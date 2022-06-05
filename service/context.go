package service

import "context"

const gsvCtxKey = ""

type Context interface {
	TraceId() string
	SpanId() string
	ParentId() string
	Keys() []string
	Set(key string, value interface{})
	Get(key string) interface{}
	Del(key string)
}

// NewContext returns a new Context that carries value Context.
func NewContext(ctx context.Context, gsvCtx Context) context.Context {
	return context.WithValue(ctx, gsvCtxKey, gsvCtx)
}

// FromContext returns the Context value stored in ctx, if any.
func FromContext(ctx context.Context) (Context, bool) {
	result, ok := ctx.Value(gsvCtxKey).(Context)
	return result, ok
}
