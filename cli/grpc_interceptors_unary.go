package cli

import (
	"context"
	"github.com/ringbrew/gsv/logger"
	"github.com/ringbrew/gsv/service"
	"github.com/ringbrew/gsv/tracex"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"strings"
	"time"
)

var slowThreshold = time.Millisecond * 500

// LogOption configures the behavior of LogUnaryInterceptor.
type LogOption func(*logConfig)

// LogSkip describes a skip rule against a gRPC full method "/pkg.Service/Method".
// Each field is matched independently; an empty string or "*" works as a
// wildcard for that field.
//
// Examples:
//   {Package: "track", Service: "Service", Method: "ReportEvent"}  exact match
//   {Package: "track", Service: "Service"}                          whole service
//   {Package: "track"}                                              whole package
//   {Method: "Health"}                                              any service's Health
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
// suppressing the success/slow info logs.
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

func LogUnaryInterceptor(opts ...LogOption) grpc.UnaryClientInterceptor {
	cfg := &logConfig{}
	for _, o := range opts {
		o(cfg)
	}
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		callOpts ...grpc.CallOption,
	) error {
		skip := cfg.shouldSkip(method)
		if skip && !cfg.keepErrorOnSkip {
			return invoker(ctx, method, req, reply, cc, callOpts...)
		}

		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, callOpts...)
		elapsed := time.Since(start)
		if err != nil {
			// req goes to extra rather than msg so that it can be redacted
			entry := logger.NewEntry(ctx).
				WithExtra("service", cc.Target()).
				WithExtra("duration", elapsed.String()).
				WithExtra("method", method).
				WithExtra("req", req)
			logger.Error(entry.WithMessage(err.Error()))
		} else if !skip {
			entry := logger.NewEntry(ctx)
			entry.WithExtra("service", cc.Target())
			entry.WithExtra("duration", elapsed.String())
			entry.WithExtra("method", method)
			entry.WithExtra("req", req)
			entry.WithExtra("resp", reply)

			if elapsed > slowThreshold {
				logger.Warn(entry.WithMessage("rpc call slow"))
			} else {
				logger.Info(entry.WithMessage("rpc call success"))
			}
		}
		return err
	}
}

// TraceUnaryInterceptor returns a grpc.UnaryClientInterceptor suitable
// for use in a grpc.Dial call.
func TraceUnaryInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		callOpts ...grpc.CallOption,
	) error {
		requestMetadata, _ := metadata.FromOutgoingContext(ctx)
		metadataCopy := requestMetadata.Copy()

		tracer := tracex.NewConfig().TracerProvider.Tracer(
			tracex.InstrumentationName,
			trace.WithInstrumentationVersion(tracex.SemVersion()),
		)

		name, attr := tracex.SpanInfo(method, cc.Target())
		var span trace.Span
		ctx, span = tracer.Start(
			ctx,
			name,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attr...),
		)
		defer span.End()

		tracex.GrpcInject(ctx, &metadataCopy)
		ctx = metadata.NewOutgoingContext(ctx, metadataCopy)

		tracex.MessageSent.Event(ctx, 1, req)

		sc := span.SpanContext()
		rpcCtx := tracex.NewServiceContext(sc.TraceID(), sc.SpanID())
		ctx = service.NewContext(ctx, rpcCtx)

		err := invoker(ctx, method, req, reply, cc, callOpts...)

		tracex.MessageReceived.Event(ctx, 1, reply)

		if err != nil {
			s, _ := status.FromError(err)
			span.SetStatus(codes.Error, s.Message())
			span.SetAttributes(statusCodeAttr(s.Code()))
		} else {
			span.SetAttributes(statusCodeAttr(grpcCodes.OK))
		}

		return err
	}
}

// statusCodeAttr returns status code attribute based on given gRPC code.
func statusCodeAttr(c grpcCodes.Code) attribute.KeyValue {
	return tracex.GRPCStatusCodeKey.Int64(int64(c))
}
