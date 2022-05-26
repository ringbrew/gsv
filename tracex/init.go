package tracex

import (
	"context"
	"github.com/ringbrew/gsv/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
)

type Exporter string

const (
	ExporterZipkin = "zipkin"
)

func Init(opts ...Option) error {
	opt := Option{
		Sampler: 1,
	}

	if len(opts) > 0 {
		opt = opts[0]
	}

	traceOpts := []trace.TracerProviderOption{
		// Set the sampling rate based on the parent span to 100%
		trace.WithSampler(trace.ParentBased(trace.TraceIDRatioBased(opt.Sampler))),
	}

	tp := trace.NewTracerProvider(traceOpts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.Error(logger.NewEntry().WithMessage(err.Error()))
	}))

	return nil
}

func newExporter(ctx context.Context, opt Option) (trace.SpanExporter, error) {
	switch opt.Exporter {
	case ExporterZipkin:
		if opt.Endpoint != "" {
			return zipkin.New(opt.Endpoint)
		}
	}
	return nil, nil
}
