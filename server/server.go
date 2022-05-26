package server

import (
	"context"
	"github.com/ringbrew/gsv/discovery"
	"github.com/ringbrew/gsv/logger"
	"github.com/ringbrew/gsv/service"
	"github.com/ringbrew/gsv/tracex"
	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
	"log"
	"net/http"
)

type Type string

const (
	GRPC Type = "grpc"
)

type Option struct {
	Port               int
	ProxyPort          int
	Logger             logger.Logger
	StreamInterceptors []grpc.StreamServerInterceptor
	UnaryInterceptors  []grpc.UnaryServerInterceptor
	StatHandler        stats.Handler
	HttpInterceptors   []http.HandlerFunc
	ServerRegister     discovery.Register
	EnableGrpcGateway  bool
	TraceOption        tracex.Option
}

func Classic() Option {
	return Option{
		Port:               3000,
		ProxyPort:          3001,
		Logger:             logger.NewDefaultLogger(),
		StreamInterceptors: []grpc.StreamServerInterceptor{},
		UnaryInterceptors: []grpc.UnaryServerInterceptor{
			recoverUnaryInterceptor(),
			traceUnaryInterceptor(),
			logUnaryInterceptor(),
		},
		HttpInterceptors: []http.HandlerFunc{},
		TraceOption: tracex.Option{
			Endpoint: "",
			Exporter: "",
			Sampler:  1,
		},
	}
}

type Server interface {
	Register(service service.Service) error
	Run(ctx context.Context)
}

func NewServer(t Type, opts ...Option) Server {
	if err := tracex.Init(); err != nil {
		log.Fatal(err.Error())
	}

	opt := Classic()

	if len(opts) > 0 {
		opt = opts[0]
	}

	switch t {
	case GRPC:
		return newGrpcServer(opt)
	default:
		return nil
	}
}
