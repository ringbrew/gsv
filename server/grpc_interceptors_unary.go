package server

import (
	"context"
	grpcLogging "github.com/grpc-ecosystem/go-grpc-middleware/logging"
	"github.com/ringbrew/gsv/logger"
	"github.com/ringbrew/gsv/service"
	"github.com/ringbrew/gsv/tracex"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"strings"
	"time"
)

func RecoverUnaryInterceptor(f func(panic interface{})) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (_ interface{}, err error) {
		defer func() {
			if p := recover(); p != nil {
				if f != nil {
					f(p)
				}
				err = status.Errorf(grpcCodes.Internal, "%s", p)
			}
		}()

		return handler(ctx, req)
	}
}

// LogOption configures the behavior of LogUnaryInterceptor.
type LogOption func(*logConfig)

// LogSkip describes a skip rule against a gRPC full method "/pkg.Service/Method".
// Each field is matched independently; an empty string or "*" works as a
// wildcard for that field.
//
// Examples:
//   {Package: "task", Service: "Service", Method: "Create"}  exact match
//   {Package: "task", Service: "Service"}                     whole service
//   {Package: "task"}                                         whole package
//   {Method: "Health"}                                        any service's Health
type LogSkip struct {
	Package string
	Service string
	Method  string
}

type logConfig struct {
	skipRules       []LogSkip
	skipFunc        func(fullMethod string) bool
	keepErrorOnSkip bool
}

// WithLogSkip skips logging for any call matching one of the given rules.
//
// By default both success and error logs are skipped for matched calls;
// use WithLogSkipKeepError to keep error logs.
func WithLogSkip(rules ...LogSkip) LogOption {
	return func(c *logConfig) {
		c.skipRules = append(c.skipRules, rules...)
	}
}

// WithLogSkipFunc lets the caller decide whether to skip logging based on
// the gRPC full method name (e.g. "/pkg.Service/Method"). Returning true
// skips the log for that call.
func WithLogSkipFunc(fn func(fullMethod string) bool) LogOption {
	return func(c *logConfig) {
		c.skipFunc = fn
	}
}

// WithLogSkipKeepError makes skipped methods still emit error logs while
// suppressing the success info logs.
func WithLogSkipKeepError() LogOption {
	return func(c *logConfig) {
		c.keepErrorOnSkip = true
	}
}

// parseFullMethod splits "/pkg.Service/Method" into its three components.
// The package is everything before the last "." in the service descriptor.
// If the input doesn't fit the standard shape, all returned strings may be
// empty.
func parseFullMethod(fullMethod string) (pkg, svc, method string) {
	if len(fullMethod) == 0 || fullMethod[0] != '/' {
		return "", "", ""
	}
	rest := fullMethod[1:]
	i := strings.IndexByte(rest, '/')
	if i < 0 {
		return "", "", ""
	}
	pkgSvc := rest[:i]
	method = rest[i+1:]
	if dot := strings.LastIndexByte(pkgSvc, '.'); dot >= 0 {
		pkg, svc = pkgSvc[:dot], pkgSvc[dot+1:]
	} else {
		svc = pkgSvc
	}
	return
}

func matchSkipField(pattern, actual string) bool {
	return pattern == "" || pattern == "*" || pattern == actual
}

func (r LogSkip) matches(pkg, svc, method string) bool {
	return matchSkipField(r.Package, pkg) &&
		matchSkipField(r.Service, svc) &&
		matchSkipField(r.Method, method)
}

func (c *logConfig) shouldSkip(fullMethod string) bool {
	if c == nil {
		return false
	}
	if len(c.skipRules) > 0 {
		pkg, svc, method := parseFullMethod(fullMethod)
		for _, r := range c.skipRules {
			if r.matches(pkg, svc, method) {
				return true
			}
		}
	}
	if c.skipFunc != nil {
		return c.skipFunc(fullMethod)
	}
	return false
}

func LogUnaryInterceptor(opts ...LogOption) grpc.UnaryServerInterceptor {
	cfg := &logConfig{}
	for _, o := range opts {
		o(cfg)
	}
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		skip := cfg.shouldSkip(info.FullMethod)
		if skip && !cfg.keepErrorOnSkip {
			return handler(ctx, req)
		}

		startTime := time.Now()

		resp, err := handler(ctx, req)
		duration := time.Since(startTime)

		code := grpcLogging.DefaultErrorToCode(err)
		entry := logger.NewEntry(ctx)
		entry = entry.WithExtra("method", info.FullMethod).
			WithExtra("duration", duration.String()).
			WithExtra("codeString", code.String()).
			WithExtra("code", code)

		if err != nil {
			logger.Error(entry.WithMessage(err.Error()))
		} else if !skip {
			logger.Info(entry.WithMessage("success"))
		}

		return resp, err
	}
}

func TraceUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		requestMetadata, _ := metadata.FromIncomingContext(ctx)
		metadataCopy := requestMetadata.Copy()

		bags, spanCtx := tracex.GrpcExtract(ctx, &metadataCopy)
		ctx = baggage.ContextWithBaggage(ctx, bags)

		tracer := tracex.NewConfig().TracerProvider.Tracer(
			tracex.InstrumentationName,
			trace.WithInstrumentationVersion(tracex.SemVersion()),
		)

		name, attr := tracex.SpanInfo(info.FullMethod, tracex.PeerFromCtx(ctx))
		ctx, span := tracer.Start(
			trace.ContextWithRemoteSpanContext(ctx, spanCtx),
			name,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attr...),
		)
		defer span.End()

		tracex.MessageReceived.Event(ctx, 1, req)

		sc := span.SpanContext()
		rpcCtx := tracex.NewServiceContext(sc.TraceID(), sc.SpanID())
		ctx = service.NewContext(ctx, rpcCtx)

		resp, err := handler(ctx, req)
		if err != nil {
			s, _ := status.FromError(err)
			span.SetStatus(codes.Error, s.Message())
			span.SetAttributes(statusCodeAttr(s.Code()))
			tracex.MessageSent.Event(ctx, 1, s.Proto())
		} else {
			span.SetAttributes(statusCodeAttr(grpcCodes.OK))
			tracex.MessageSent.Event(ctx, 1, resp)
		}

		return resp, err
	}
}

// statusCodeAttr returns status code attribute based on given gRPC code.
func statusCodeAttr(c grpcCodes.Code) attribute.KeyValue {
	return tracex.GRPCStatusCodeKey.Int64(int64(c))
}
