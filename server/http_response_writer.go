package server

import "github.com/urfave/negroni"

type HttpResponseWriter interface {
	negroni.ResponseWriter
	Dump() []byte
}
