package cli

import (
	"context"
	"fmt"
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
	"time"
)

var slowThreshold = time.Millisecond * 500

func LogUnaryInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		callOpts ...grpc.CallOption,
	) error {
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, callOpts...)
		if err != nil {
			logger.Error(logger.NewEntry(ctx).WithMessage(fmt.Sprintf("call service[%s]-method[%s] req[%v], error[%s]", cc.Target(), method, req, err.Error())))
		} else {
			elapsed := time.Since(start)
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
