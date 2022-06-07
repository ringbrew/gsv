package server

import (
	"context"
	"fmt"
	"github.com/ringbrew/gsv/service"
	"github.com/ringbrew/gsv/tracex"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"net/http"
)

func gatewayRecoverMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if r := recover(); r != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(fmt.Sprintf("%v", r)))
			}
		}()
		h.ServeHTTP(w, r)
	})
}

func gatewayTraceMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		bags, spanCtx := tracex.HttpExtract(ctx, propagation.HeaderCarrier(r.Header))
		ctx = baggage.ContextWithBaggage(ctx, bags)

		tracer := tracex.NewConfig().TracerProvider.Tracer(
			tracex.InstrumentationName,
		)
		fullPath := r.URL.Path

		name, attr := tracex.SpanInfo(fullPath, tracex.PeerFromCtx(ctx))
		ctx, span := tracer.Start(
			trace.ContextWithRemoteSpanContext(ctx, spanCtx),
			name,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attr...),
			trace.WithAttributes(semconv.NetAttributesFromHTTPRequest("tcp", r)...),
			trace.WithAttributes(semconv.HTTPServerAttributesFromHTTPRequest("grpc-gateway", fullPath, r)...),
		)
		defer span.End()

		r = r.WithContext(ctx)

		h.ServeHTTP(w, r)
	})
}

func gatewayTraceyInterceptor() grpc.UnaryClientInterceptor {
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
