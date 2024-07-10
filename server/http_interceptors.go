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

type HttpLogger struct {
	Name string
}

func NewHttpLogger() *HttpLogger {
	return &HttpLogger{}
}

func (hl *HttpLogger) SetName(name string) {
	hl.Name = name
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

	if hl.Name != "" {
		entry = entry.WithExtra("name", hl.Name)
	}

	if status >= http.StatusBadRequest {
		logger.Error(entry.WithMessage(string(res.Dump())))
	} else {
		logger.Info(entry.WithMessage("success"))
	}
}

type HttpTracer struct {
	Name string
}

func NewHttpTracer() *HttpTracer {
	return &HttpTracer{}
}

func (ht *HttpTracer) SetName(name string) {
	ht.Name = name
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
