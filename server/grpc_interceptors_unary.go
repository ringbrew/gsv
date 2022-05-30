package server

import (
	"context"
	grpcLogging "github.com/grpc-ecosystem/go-grpc-middleware/logging"
	"github.com/ringbrew/gsv/tracex"
	"github.com/ringbrew/gsvcore/logger"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"log"
	"strconv"
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

func LogUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		startTime := time.Now()

		span := trace.SpanFromContext(ctx)
		spanCtx := span.SpanContext()
		log.Println(spanCtx.TraceID().String(), spanCtx.SpanID().String())

		resp, err := handler(ctx, req)
		duration := time.Since(startTime)

		code := grpcLogging.DefaultErrorToCode(err)
		entry := logger.NewEntry(ctx)
		entry = entry.WithExtra("method", info.FullMethod).
			WithExtra("latency", duration.String()).
			WithExtra("codeString", code.String()).
			WithExtra("code", strconv.Itoa(int(code)))

		if err != nil {
			logger.Error(entry.WithMessage(err.Error()))
		} else {
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
