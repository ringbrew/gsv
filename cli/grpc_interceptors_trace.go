package cli

import (
	"context"
	"github.com/ringbrew/gsv/tracex"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

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
			//trace.WithInstrumentationVersion(SemVersion()),
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
