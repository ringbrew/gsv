package tracex

import (
	"context"
	"github.com/ringbrew/gsv/service"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"net"
	"strings"
)

const (
	InstrumentationName = "github.com/ringbrew/gsv"
)

// Version is the current release version of the gsv instrumentation.
func Version() string {
	return "1.0.0"
}

// SemVersion is the semantic version to be supplied to tracer/meter creation.
func SemVersion() string {
	return "semver:" + Version()
}

// Config is a group of options for this instrumentation.
type Config struct {
	Propagators    propagation.TextMapPropagator
	TracerProvider trace.TracerProvider
}

// Option applies an option value for a config.
//type Option interface {
//	apply(*config)
//}

// NewConfig returns a config configured with all the passed Options.
func NewConfig() *Config {
	c := &Config{
		Propagators:    otel.GetTextMapPropagator(),
		TracerProvider: otel.GetTracerProvider(),
	}
	//for _, o := range opts {
	//	o.apply(c)
	//}
	return c
}

type Option struct {
	Endpoint string  `json:"endpoint"`
	Exporter string  `json:"exporter"`
	Sampler  float64 `json:"sampler"`
	Debug    bool    `json:"debug"`
}

type metadataSupplier struct {
	metadata *metadata.MD
}

// assert that metadataSupplier implements the TextMapCarrier interface.
var _ propagation.TextMapCarrier = &metadataSupplier{}

func (s *metadataSupplier) Get(key string) string {
	values := s.metadata.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (s *metadataSupplier) Set(key string, value string) {
	s.metadata.Set(key, value)
}

func (s *metadataSupplier) Keys() []string {
	out := make([]string, 0, len(*s.metadata))
	for key := range *s.metadata {
		out = append(out, key)
	}
	return out
}

// GrpcInject injects correlation context and span context into the gRPC
// metadata object. This function is meant to be used on outgoing
// requests.
func GrpcInject(ctx context.Context, md *metadata.MD) {
	c := NewConfig()
	c.Propagators.Inject(ctx, &metadataSupplier{
		metadata: md,
	})
}

// GrpcExtract returns the correlation context and span context that
// another service encoded in the gRPC metadata object with Inject.
// This function is meant to be used on incoming requests.
func GrpcExtract(ctx context.Context, md *metadata.MD) (baggage.Baggage, trace.SpanContext) {
	c := NewConfig()
	ctx = c.Propagators.Extract(ctx, &metadataSupplier{
		metadata: md,
	})
	return baggage.FromContext(ctx), trace.SpanContextFromContext(ctx)
}

func HttpInject(ctx context.Context, hc propagation.HeaderCarrier) {
	c := NewConfig()
	c.Propagators.Inject(ctx, hc)
}

func HttpExtract(ctx context.Context, hc propagation.HeaderCarrier) (baggage.Baggage, trace.SpanContext) {
	c := NewConfig()
	ctx = c.Propagators.Extract(ctx, hc)
	return baggage.FromContext(ctx), trace.SpanContextFromContext(ctx)
}

// SpanInfo returns a span name and all appropriate attributes from the gRPC
// method and peer address.
func SpanInfo(fullMethod, peerAddress string) (string, []attribute.KeyValue) {
	attrs := []attribute.KeyValue{RPCSystemGRPC}
	name, mAttrs := ParseFullMethod(fullMethod)
	attrs = append(attrs, mAttrs...)
	attrs = append(attrs, PeerAttr(peerAddress)...)
	return name, attrs
}

// PeerAttr returns attributes about the peer address.
func PeerAttr(addr string) []attribute.KeyValue {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return []attribute.KeyValue(nil)
	}

	if host == "" {
		host = "127.0.0.1"
	}

	return []attribute.KeyValue{
		semconv.NetPeerIPKey.String(host),
		semconv.NetPeerPortKey.String(port),
	}
}

// PeerFromCtx returns a peer address from a context, if one exists.
func PeerFromCtx(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return ""
	}
	return p.Addr.String()
}

// ParseFullMethod returns a span name following the OpenTelemetry semantic
// conventions as well as all applicable span attribute.KeyValue attributes based
// on a gRPC's FullMethod.
func ParseFullMethod(fullMethod string) (string, []attribute.KeyValue) {
	name := strings.TrimLeft(fullMethod, "/")
	parts := strings.SplitN(name, "/", 2)
	if len(parts) != 2 {
		// Invalid format, does not follow `/package.service/method`.
		return name, []attribute.KeyValue(nil)
	}

	var attrs []attribute.KeyValue
	if service := parts[0]; service != "" {
		attrs = append(attrs, semconv.RPCServiceKey.String(service))
	}
	if method := parts[1]; method != "" {
		attrs = append(attrs, semconv.RPCMethodKey.String(method))
	}
	return name, attrs
}

func NewTraceSpanContext(ctx context.Context, spanName string) (context.Context, trace.Span) {
	tracer := NewConfig().TracerProvider.Tracer(
		InstrumentationName,
		trace.WithInstrumentationVersion(SemVersion()),
	)
	ctx, span := tracer.Start(
		ctx,
		spanName,
		trace.WithSpanKind(trace.SpanKindServer),
	)
	sc := span.SpanContext()
	rpcCtx := NewServiceContext(sc.TraceID(), sc.SpanID())
	ctx = service.NewContext(ctx, rpcCtx)
	return ctx, span
}
