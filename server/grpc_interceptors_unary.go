package server

import (
	"context"
	"fmt"
	grpcLogging "github.com/grpc-ecosystem/go-grpc-middleware/logging"
	"github.com/ringbrew/gsv/logger"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"log"
	"strconv"
	"time"
)

func recoverUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (_ interface{}, err error) {
		defer func() {
			if p := recover(); p != nil {
				logger.Fatal(logger.NewEntry(ctx).WithMessage(fmt.Sprintf("server panic: %v", p)))
				err = status.Errorf(codes.Internal, "%s", p)
			}
		}()

		return handler(ctx, req)
	}
}

func logUnaryInterceptor() grpc.UnaryServerInterceptor {
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
