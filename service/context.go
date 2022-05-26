package service

import (
	"context"
	"go.opentelemetry.io/otel/trace"
)

type Context interface {
	TraceId() string
	SpanId() string
	ParentId() string
	Extra() map[string]string
	Set(key string, value string)
	Get(key string) string
}

type rpcCtx struct {
	traceId  trace.TraceID
	spanId   trace.SpanID
	parentId trace.SpanID
	extra    map[string]string
}

func newRpcCtx(ctx context.Context) *rpcCtx {
	// todo get context info from open-telemetry.

	return &rpcCtx{
		traceId:  trace.TraceID{},
		spanId:   trace.SpanID{},
		parentId: trace.SpanID{},
		extra:    nil,
	}
}

func (r *rpcCtx) TraceId() string {
	//TODO implement me
	panic("implement me")
}

func (r *rpcCtx) SpanId() string {
	//TODO implement me
	panic("implement me")
}

func (r *rpcCtx) ParentId() string {
	//TODO implement me
	panic("implement me")
}

func (r *rpcCtx) Extra() map[string]string {
	//TODO implement me
	panic("implement me")
}

func (r *rpcCtx) Set(key string, value string) {
	//TODO implement me
	panic("implement me")
}

func (r *rpcCtx) Get(key string) string {
	//TODO implement me
	panic("implement me")
}
