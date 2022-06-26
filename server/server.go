package server

import (
	"context"
	"fmt"
	"github.com/ringbrew/gsv/discovery"
	"github.com/ringbrew/gsv/logger"
	"github.com/ringbrew/gsv/service"
	"github.com/urfave/negroni"
	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
)

type Type string

const (
	GRPC Type = "grpc"
	HTTP Type = "http"
)

type Option struct {
	Name           string
	Host           string
	Port           int
	ProxyPort      int
	ServerRegister discovery.Register
	CertFile       string
	KeyFile        string

	//grpc option.
	EnableGrpcGateway  bool
	StreamInterceptors []grpc.StreamServerInterceptor
	UnaryInterceptors  []grpc.UnaryServerInterceptor
	StatHandler        stats.Handler

	//http option
	HttpMiddleware  []negroni.Handler
	EnableDocServer bool
}

func Classic() Option {
	return Option{
		Port:      3000,
		ProxyPort: 3001,
		StreamInterceptors: []grpc.StreamServerInterceptor{
			RecoverStreamInterceptor(func(panic interface{}) {
				logger.Error(logger.NewEntry().WithMessage(fmt.Sprintf("server panic:[%v]", panic)))
			}),
			TraceStreamServerInterceptor(),
		},
		UnaryInterceptors: []grpc.UnaryServerInterceptor{
			RecoverUnaryInterceptor(func(panic interface{}) {
				logger.Error(logger.NewEntry().WithMessage(fmt.Sprintf("server panic:[%v]", panic)))
			}),
			TraceUnaryInterceptor(),
			LogUnaryInterceptor(),
		},
		HttpMiddleware: []negroni.Handler{
			NewHttpRecovery(),
			NewHttpTracer(),
			NewHttpLogger()},
	}
}

type SetNamer interface {
	SetName(name string)
}

type Server interface {
	Register(service service.Service) error
	Run(ctx context.Context)
}

func NewServer(t Type, opts ...*Option) Server {
	opt := Classic()

	if len(opts) > 0 && opts[0] != nil {
		opt = *opts[0]
	}

	switch t {
	case GRPC:
		return newGrpcServer(opt)
	case HTTP:
		return newHttpServer(opt)
	default:
		return nil
	}
}
