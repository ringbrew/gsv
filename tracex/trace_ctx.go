package tracex

import (
	"github.com/ringbrew/gsv/service"
	"go.opentelemetry.io/otel/trace"
	"sync"
)

type rpcCtx struct {
	traceId  trace.TraceID
	spanId   trace.SpanID
	parentId trace.SpanID
	extra    map[string]interface{}
	sync.RWMutex
}

func NewServiceContext(traceId trace.TraceID, spanId trace.SpanID, parentId ...trace.SpanID) service.Context {
	result := &rpcCtx{
		traceId: traceId,
		spanId:  spanId,
	}

	if len(parentId) > 0 {
		result.parentId = parentId[0]
	}

	return result
}

func (r *rpcCtx) TraceId() string {
	return r.traceId.String()
}

func (r *rpcCtx) SpanId() string {
	return r.spanId.String()
}

func (r *rpcCtx) ParentId() string {
	return r.parentId.String()
}

func (r *rpcCtx) Keys() []string {
	r.RWMutex.RLock()
	defer r.RWMutex.RUnlock()
	result := make([]string, 0, len(r.extra))
	for k := range r.extra {
		result = append(result, k)
	}
	return result
}

func (r *rpcCtx) Set(key string, value interface{}) {
	r.RWMutex.Lock()
	defer r.RWMutex.Unlock()
	if r.extra == nil {
		r.extra = make(map[string]interface{})
	}
	r.extra[key] = value
}

func (r *rpcCtx) Get(key string) interface{} {
	r.RWMutex.RLocker()
	defer r.RWMutex.RUnlock()
	if r.extra == nil {
		return nil
	} else {
		return r.extra[key]
	}
}

func (r *rpcCtx) Del(key string) {
	r.RWMutex.Lock()
	defer r.RWMutex.Unlock()
	if r.extra == nil {
		return
	} else {
		delete(r.extra, key)
	}
}
