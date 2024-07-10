package server

import (
	"log"
	"net/http"
	"os"
)

const (
	// DefaultAddress is used if no other is specified.
	DefaultAddress = ":8080"
)

// Handler is an interface that objects can implement to be registered to serve as middleware
// in the Engine middleware stack.
// ServeHTTP should yield to the next middleware in the chain by invoking the next http.HandlerFunc
// passed in.
//
// If the Handler writes to the ResponseWriter, the next http.HandlerFunc should not be invoked.
type Handler interface {
	ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc)
}

// HandlerFunc is an adapter to allow the use of ordinary functions as Engine handlers.
// If f is a function with the appropriate signature, HandlerFunc(f) is a Handler object that calls f.
type HandlerFunc func(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc)

func (h HandlerFunc) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	h(rw, r, next)
}

type middleware struct {
	handler Handler
	next    *middleware
}

func (m middleware) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	m.handler.ServeHTTP(rw, r, m.next.ServeHTTP)
}

// Wrap converts a http.Handler into a engine.Handler so it can be used as a Engine
// middleware. The next http.HandlerFunc is automatically called after the Handler
// is executed.
func Wrap(handler http.Handler) Handler {
	return HandlerFunc(func(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		handler.ServeHTTP(rw, r)
		next(rw, r)
	})
}

// WrapFunc converts a http.HandlerFunc into a engine.Handler so it can be used as a Engine
// middleware. The next http.HandlerFunc is automatically called after the Handler
// is executed.
func WrapFunc(handlerFunc http.HandlerFunc) Handler {
	return HandlerFunc(func(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		handlerFunc(rw, r)
		next(rw, r)
	})
}

// Engine is a stack of Handler Handlers that can be invoked as an http.Handler.
// Engine middleware is evaluated in the order that they are added to the stack using
// the Use and UseHandler methods.
type Engine struct {
	middleware middleware
	handlers   []Handler
}

// New returns a new Engine instance with no middleware preconfigured.
func New(handlers ...Handler) *Engine {
	return &Engine{
		handlers:   handlers,
		middleware: build(handlers),
	}
}

// With returns a new Engine instance that is a combination of the engine
// receiver's handlers and the provided handlers.
func (n *Engine) With(handlers ...Handler) *Engine {
	return New(
		append(n.handlers, handlers...)...,
	)
}

func (n *Engine) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	n.middleware.ServeHTTP(NewResponseWriter(rw), r)
}

// Use adds a Handler onto the middleware stack. Handlers are invoked in the order they are added to a Engine.
func (n *Engine) Use(handler Handler) {
	if handler == nil {
		panic("handler cannot be nil")
	}

	n.handlers = append(n.handlers, handler)
	n.middleware = build(n.handlers)
}

// UseFunc adds a Engine-style handler function onto the middleware stack.
func (n *Engine) UseFunc(handlerFunc func(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc)) {
	n.Use(HandlerFunc(handlerFunc))
}

// UseHandler adds a http.Handler onto the middleware stack. Handlers are invoked in the order they are added to a Engine.
func (n *Engine) UseHandler(handler http.Handler) {
	n.Use(Wrap(handler))
}

// UseHandlerFunc adds a http.HandlerFunc-style handler function onto the middleware stack.
func (n *Engine) UseHandlerFunc(handlerFunc func(rw http.ResponseWriter, r *http.Request)) {
	n.UseHandler(http.HandlerFunc(handlerFunc))
}

// Run is a convenience function that runs the engine stack as an HTTP
// server. The addr string, if provided, takes the same format as http.ListenAndServe.
// If no address is provided but the PORT environment variable is set, the PORT value is used.
// If neither is provided, the address' value will equal the DefaultAddress constant.
func (n *Engine) Run(addr ...string) {
	l := log.New(os.Stdout, "[engine] ", 0)
	finalAddr := detectAddress(addr...)
	l.Printf("listening on %s", finalAddr)
	l.Fatal(http.ListenAndServe(finalAddr, n))
}

func detectAddress(addr ...string) string {
	if len(addr) > 0 {
		return addr[0]
	}
	if port := os.Getenv("PORT"); port != "" {
		return ":" + port
	}
	return DefaultAddress
}

// Handlers Returns a list of all the handlers in the current Engine middleware chain.
func (n *Engine) Handlers() []Handler {
	return n.handlers
}

func build(handlers []Handler) middleware {
	var next middleware

	if len(handlers) == 0 {
		return voidMiddleware()
	} else if len(handlers) > 1 {
		next = build(handlers[1:])
	} else {
		next = voidMiddleware()
	}

	return middleware{handlers[0], &next}
}

func voidMiddleware() middleware {
	return middleware{
		HandlerFunc(func(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {}),
		&middleware{},
	}
}
