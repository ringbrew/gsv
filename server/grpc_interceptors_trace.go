package server

import (
	"context"
	"github.com/ringbrew/gsv/tracex"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func traceUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		requestMetadata, _ := metadata.FromIncomingContext(ctx)
		metadataCopy := requestMetadata.Copy()

		bags, spanCtx := tracex.GrpcExtract(ctx, &metadataCopy)
		ctx = baggage.ContextWithBaggage(ctx, bags)

		tracer := tracex.NewConfig().TracerProvider.Tracer(
			tracex.InstrumentationName,
			//trace.WithInstrumentationVersion(SemVersion()),
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
