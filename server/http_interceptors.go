package server

import (
	"fmt"
	"github.com/ringbrew/gsv/logger"
	"github.com/ringbrew/gsv/service"
	"github.com/ringbrew/gsv/tracex"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
	"go.opentelemetry.io/otel/trace"
	"net/http"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

/*
HttpRecovery fork from negroni
*/
const nilRequestMessage = "Request is nil"
const panicText = "PANIC: %s"

// PanicInformation contains all
// elements for printing stack informations.
type PanicInformation struct {
	RecoveredPanic interface{}
	Stack          []byte
	Request        *http.Request
}

// StackAsString returns a printable version of the stack
func (p *PanicInformation) StackAsString() string {
	return string(p.Stack)
}

// RequestDescription returns a printable description of the url
func (p *PanicInformation) RequestDescription() string {

	if p.Request == nil {
		return nilRequestMessage
	}

	var queryOutput string
	if p.Request.URL.RawQuery != "" {
		queryOutput = "?" + p.Request.URL.RawQuery
	}
	return fmt.Sprintf("%s %s%s", p.Request.Method, p.Request.URL.Path, queryOutput)
}

const HttpRecoveryKey = "HttpRecovery"

// HttpRecovery is a middleware that recovers from any panics and writes a 500 if there was one.
type HttpRecovery struct {
	PrintStack       bool
	PanicHandlerFunc func(*PanicInformation)
	StackAll         bool
	StackSize        int
	Formatter        PanicFormatter
}

// NewHttpRecovery returns a new instance of HttpRecovery
func NewHttpRecovery() *HttpRecovery {
	return &HttpRecovery{
		PrintStack: false,
		StackAll:   false,
		StackSize:  1024 * 8,
		Formatter:  &TextPanicFormatter{},
	}
}

// PanicFormatter is an interface on object can implement
// to be able to output the stack trace
type PanicFormatter interface {
	// FormatPanicError output the stack for a given answer/response.
	// In case the the middleware should not output the stack trace,
	// the field `Stack` of the passed `PanicInformation` instance equals `[]byte{}`.
	FormatPanicError(rw http.ResponseWriter, r *http.Request, infos *PanicInformation)
}

// TextPanicFormatter output the stack
// as simple text on os.Stdout. If no `Content-Type` is set,
// it will output the data as `text/plain; charset=utf-8`.
// Otherwise, the origin `Content-Type` is kept.
type TextPanicFormatter struct{}

func (t *TextPanicFormatter) FormatPanicError(rw http.ResponseWriter, r *http.Request, infos *PanicInformation) {
	if rw.Header().Get("Content-Type") == "" {
		rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	fmt.Fprintf(rw, panicText, infos.RecoveredPanic)
}

func (rec *HttpRecovery) GetKey() string {
	return HttpRecoveryKey
}

func (rec *HttpRecovery) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	defer func() {
		if err := recover(); err != nil {
			rw.WriteHeader(http.StatusInternalServerError)

			stack := make([]byte, rec.StackSize)
			stack = stack[:runtime.Stack(stack, rec.StackAll)]
			infos := &PanicInformation{RecoveredPanic: err, Request: r}

			if rec.PrintStack {
				infos.Stack = stack
			}

			logger.Error(logger.NewEntry(r.Context()).WithMessage(fmt.Sprintf(panicText+".stack:[%s]", err, stack)))
			rec.Formatter.FormatPanicError(rw, r, infos)

			if rec.PanicHandlerFunc != nil {
				func() {
					defer func() {
						if err := recover(); err != nil {
							logger.Error(logger.NewEntry(r.Context()).WithMessage(fmt.Sprintf("provided PanicHandlerFunc panic'd: %s, trace:\n%s", err, debug.Stack())))
						}
					}()
					rec.PanicHandlerFunc(infos)
				}()
			}
		}
	}()

	rw = NewResponseWriter(rw)

	next(rw, r)
}

type HttpLogFilter struct {
	ignore map[string][]interface{}
	sync.RWMutex
}

func (hlf *HttpLogFilter) SetIgnore(key string, value interface{}) {
	hlf.Lock()
	defer hlf.Unlock()
	hlf.ignore[key] = append(hlf.ignore[key], value)
}

func (hlf *HttpLogFilter) Ignore(entry *logger.LogEntry) bool {
	hlf.RLock()
	defer hlf.RUnlock()
	for k, v := range hlf.ignore {
		if val, exist := entry.Extra[k]; exist {
			for _, vv := range v {
				if val == vv {
					return true
				}
			}
		}
	}

	return false
}

const HttpLoggerKey = "HttpLogger"

type HttpLogger struct {
	Name string
	f    *HttpLogFilter
}

func NewHttpLogger() *HttpLogger {
	return &HttpLogger{
		f: &HttpLogFilter{
			ignore: make(map[string][]interface{}),
		},
	}
}

func (hl *HttpLogger) SetName(name string) {
	hl.Name = name
}

func (hl *HttpLogger) GetKey() string {
	return HttpLoggerKey
}

func (hl *HttpLogger) SetIgnore(key string, value interface{}) {
	hl.f.SetIgnore(key, value)
}

func (hl *HttpLogger) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	startTime := time.Now()
	next(rw, r)
	duration := time.Since(startTime)

	status := 0
	size := 0

	res, ok := rw.(ResponseWriter)
	if ok {
		status = res.Status()
		size = res.Size()
	}

	entry := logger.NewEntry(r.Context()).
		WithExtra("start_time", startTime.Format(time.RFC3339Nano)).
		WithExtra("duration", duration.String()).
		WithExtra("host", r.Host).
		WithExtra("method", r.Method).
		WithExtra("path", r.URL.Path).
		WithExtra("status", status).
		WithExtra("size", size)

	if !hl.f.Ignore(entry) {
		if status >= http.StatusBadRequest {
			logger.Error(entry.WithMessage(string(res.Dump())))
		} else {
			logger.Info(entry.WithMessage("success"))
		}
	}
}

const HttpTracerKey = "HttpTracer"

type HttpTracer struct {
	Name string
}

func NewHttpTracer() *HttpTracer {
	return &HttpTracer{}
}

func (ht *HttpTracer) SetName(name string) {
	ht.Name = name
}

func (ht *HttpTracer) GetKey() string {
	return HttpTracerKey
}

func (ht *HttpTracer) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	ctx := r.Context()

	bags, spanCtx := tracex.HttpExtract(ctx, propagation.HeaderCarrier(r.Header))
	ctx = baggage.ContextWithBaggage(ctx, bags)

	tracer := tracex.NewConfig().TracerProvider.Tracer(
		tracex.InstrumentationName,
		trace.WithInstrumentationVersion(tracex.SemVersion()),
	)
	fullPath := r.URL.Path

	name, attr := tracex.SpanInfo(fullPath, tracex.PeerFromCtx(ctx))
	ctx, span := tracer.Start(
		trace.ContextWithRemoteSpanContext(ctx, spanCtx),
		name,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attr...),
		trace.WithAttributes(semconv.NetAttributesFromHTTPRequest("tcp", r)...),
		trace.WithAttributes(semconv.HTTPServerAttributesFromHTTPRequest(ht.Name, fullPath, r)...),
	)
	defer span.End()

	sc := span.SpanContext()
	rpcCtx := tracex.NewServiceContext(sc.TraceID(), sc.SpanID())
	ctx = service.NewContext(ctx, rpcCtx)

	r = r.WithContext(ctx)

	traceId := span.SpanContext().TraceID()
	if traceId.IsValid() {
		rw.Header().Set("x-trace-id", traceId.String())
	}

	next(rw, r)

	status := 0
	if res, ok := rw.(ResponseWriter); ok {
		status = res.Status()
	}

	attrs := semconv.HTTPAttributesFromHTTPStatusCode(status)
	spanStatus, spanMessage := semconv.SpanStatusFromHTTPStatusCode(status)
	span.SetAttributes(attrs...)
	span.SetStatus(spanStatus, spanMessage)
}

type HttpLogOption = func(l *HttpLogger)

func WithHttpLoggerIgnore(key string, value interface{}) HttpLogOption {
	return func(l *HttpLogger) {
		l.SetIgnore(key, value)
	}
}

func WithHttpLoggerName(name string) HttpLogOption {
	return func(l *HttpLogger) {
		l.SetName(name)
	}
}

type HttpTraceOption = func(l *HttpTracer)

func WithHttpTracerName(name string) HttpTraceOption {
	return func(l *HttpTracer) {
		l.SetName(name)
	}
}

type HttpOption struct {
	TraceOptions []HttpTraceOption
	LogOptions   []HttpLogOption
}

func (ho HttpOption) Exec(handler Handler) {
	if k, ok := handler.(GetKeyer); !ok {
		return
	} else {
		switch k.GetKey() {
		case HttpLoggerKey:
			if hlo, ok := handler.(*HttpLogger); !ok {
				return
			} else {
				for i := range ho.LogOptions {
					ho.LogOptions[i](hlo)
				}
			}
		case HttpTracerKey:
			if ht, ok := handler.(*HttpTracer); !ok {
				return
			} else {
				for i := range ho.TraceOptions {
					ho.TraceOptions[i](ht)
				}
			}
		}
	}
}
