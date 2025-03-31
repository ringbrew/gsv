package server

import (
	"context"
	"fmt"
	"github.com/ringbrew/gsv/discovery"
	"github.com/ringbrew/gsv/logger"
	"github.com/ringbrew/gsv/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
	"runtime/debug"
)

type Type string

const (
	GRPC Type = "grpc"
	HTTP Type = "http"
)

const TagExternal = "external"

type Option struct {
	Name           string
	Host           string
	External       []string
	Port           int
	ProxyPort      int
	ServerRegister discovery.NodeRegister
	CertFile       string
	KeyFile        string
	NodeId         string

	//grpc option.
	EnableGrpcGateway  bool
	StreamInterceptors []grpc.StreamServerInterceptor
	UnaryInterceptors  []grpc.UnaryServerInterceptor
	StatHandler        stats.Handler

	//http option
	HttpMiddleware []Handler
	HttpOption     HttpOption
}

func Classic() Option {
	return Option{
		Port:      3000,
		ProxyPort: 3001,
		StreamInterceptors: []grpc.StreamServerInterceptor{
			RecoverStreamInterceptor(func(panic interface{}) {
				logger.Error(logger.NewEntry().WithMessage(fmt.Sprintf("server panic:[%v] with stack[%s]", panic, string(debug.Stack()))))
			}),
			TraceStreamServerInterceptor(),
		},
		UnaryInterceptors: []grpc.UnaryServerInterceptor{
			RecoverUnaryInterceptor(func(panic interface{}) {
				logger.Error(logger.NewEntry().WithMessage(fmt.Sprintf("server panic:[%v] with stack[%s]", panic, string(debug.Stack()))))
			}),
			TraceUnaryInterceptor(),
			LogUnaryInterceptor(),
		},
		HttpMiddleware: []Handler{
			NewHttpRecovery(),
			NewHttpTracer(),
			NewHttpLogger()},
	}
}

type ServicePatcher interface {
	Patch(svc service.Service) error
}

type GetKeyer interface {
	GetKey() string
}

type Server interface {
	Register(service service.Service) error
	Run(ctx context.Context)
	Doc() []DocService
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
