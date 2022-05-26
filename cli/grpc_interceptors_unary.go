package cli

import (
	"context"
	"fmt"
	"github.com/ringbrew/gsv/logger"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"log"
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

		span := trace.SpanFromContext(ctx)
		spanCtx := span.SpanContext()
		log.Println(spanCtx.TraceID().String(), spanCtx.SpanID().String())

		err := invoker(ctx, method, req, reply, cc, callOpts...)
		if err != nil {
			logger.Error(logger.NewEntry(ctx).WithMessage(fmt.Sprintf("call service[%s]-method[%s] error[%s]", cc.Target(), method, err.Error())))
		} else {
			elapsed := time.Since(start)
			if elapsed > slowThreshold {
				logger.Warn(logger.NewEntry(ctx).WithMessage(fmt.Sprintf("call service[%s]-method[%s] slow duration[%s], - %v - %v", cc.Target(), method, elapsed, req, reply)))
			} else {
				logger.Debug(logger.NewEntry(ctx).WithMessage(fmt.Sprintf("call service[%s]-method[%s] success duration[%s], - %v - %v", cc.Target(), method, elapsed, req, reply)))
			}
		}
		return err
	}
}
