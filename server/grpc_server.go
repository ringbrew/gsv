package server

import (
	"context"
	"errors"
	"fmt"
	grpcMiddleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/ringbrew/gsv/discovery"
	"github.com/ringbrew/gsv/logger"
	"github.com/ringbrew/gsv/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/stats"
	"google.golang.org/protobuf/encoding/protojson"
	"log"
	"net"
	"net/http"
	"sync"
)

type grpcServer struct {
	name               string
	host               string
	port               int
	proxyPort          int
	gSrv               *grpc.Server
	streamInterceptors []grpc.StreamServerInterceptor
	unaryInterceptors  []grpc.UnaryServerInterceptor
	statHandler        stats.Handler
	register           discovery.NodeRegister

	enableGateway bool
	gSrvGateway   *http.Server
	gatewayMux    *runtime.ServeMux
	serviceList   []service.Service

	WaitGroup sync.WaitGroup
}

func newGrpcServer(opt Option) *grpcServer {
	s := &grpcServer{
		name:               opt.Name,
		host:               opt.Host,
		port:               opt.Port,
		proxyPort:          opt.ProxyPort,
		streamInterceptors: opt.StreamInterceptors,
		unaryInterceptors:  opt.UnaryInterceptors,
		statHandler:        opt.StatHandler,
		register:           opt.ServerRegister,
		enableGateway:      opt.EnableGrpcGateway,
	}

	if s.host == "" {
		s.host = s.findListenOn()
	}

	opts := make([]grpc.ServerOption, 0)

	if len(s.unaryInterceptors) > 0 {
		opts = append(opts, grpcMiddleware.WithUnaryServerChain(opt.UnaryInterceptors...))
	}

	if len(s.streamInterceptors) > 0 {
		opts = append(opts, grpcMiddleware.WithStreamServerChain(opt.StreamInterceptors...))
	}

	if s.statHandler != nil {
		opts = append(opts, grpc.StatsHandler(opt.StatHandler))
	}

	s.gSrv = grpc.NewServer(opts...)

	if s.enableGateway {
		jpo := &runtime.JSONPb{
			MarshalOptions: protojson.MarshalOptions{
				UseEnumNumbers: true,
			},
		}
		m := runtime.NewServeMux(runtime.WithMarshalerOption("application/json", jpo))
		httpMux := http.NewServeMux()
		httpMux.Handle("/", m)

		hs := &http.Server{
			Addr:    fmt.Sprintf(":%d", s.proxyPort),
			Handler: gatewayRecoverMiddleware(gatewayTraceMiddleware(httpMux)),
		}

		s.gatewayMux = m
		s.gSrvGateway = hs
	}

	return s
}

func (gs *grpcServer) Register(srv service.Service) error {
	desc := srv.Description()
	if !desc.Valid {
		return errors.New("invalid service description")
	}

	if len(desc.GrpcServiceDesc) == 0 {
		return errors.New("not invalid grpc service desc")
	}

	gs.serviceList = append(gs.serviceList, srv)

	return nil
}

func (gs *grpcServer) Run(ctx context.Context) {
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithChainUnaryInterceptor(gatewayTraceyInterceptor())}

	conn, err := grpc.Dial(fmt.Sprintf("127.0.0.1:%d", gs.port), opts...)
	if err != nil {
		log.Fatal(err.Error())
	}

	for i := range gs.serviceList {
		desc := gs.serviceList[i].Description()

		for _, v := range desc.GrpcServiceDesc {
			gs.gSrv.RegisterService(&v, gs.serviceList[i])
		}

		for _, f := range desc.GrpcGateway {
			if err := f(context.Background(), gs.gatewayMux, conn); err != nil {
				log.Fatal(err.Error())
			}
		}
	}

	gs.WaitGroup = sync.WaitGroup{}

	if gs.enableGateway {
		gs.WaitGroup.Add(1)
		go func() {
			if err := gs.runGateway(ctx); err != nil {
				log.Fatal(fmt.Errorf("server gateway run error:%s", err.Error()))
			}
		}()
	}

	gs.WaitGroup.Add(1)
	go func() {
		if err := gs.run(ctx); err != nil {
			log.Fatal(fmt.Errorf("server run error:%s", err.Error()))
		}
	}()

	gs.WaitGroup.Wait()
}

func (gs *grpcServer) run(ctx context.Context) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", gs.port))
	if err != nil {
		return err
	}

	var node *discovery.Node
	go func() {
		defer gs.WaitGroup.Done()
		select {
		case <-ctx.Done():
			if gs.register != nil && node != nil {
				if err := gs.register.Deregister(node); err != nil {
					logger.Error(logger.NewEntry().WithMessage(fmt.Sprintf("node[%s]-[%s]-[%d] deregister error %s", node.Name, node.Host, node.Port, err.Error())))
				} else {
					logger.Info(logger.NewEntry().WithMessage(fmt.Sprintf("node[%s]-[%s]-[%d] success deregister", node.Name, node.Host, node.Port)))
				}
			}
			gs.gSrv.GracefulStop()
			logger.Info(logger.NewEntry().WithMessage(fmt.Sprintf("rpc server stop listen on: [%d]", gs.port)))
		}
	}()

	if gs.register != nil && gs.name != "" && gs.host != "" {
		node = discovery.NewNode(gs.name, gs.host, gs.port, discovery.GRPC)
		if err := gs.register.Register(node); err != nil {
			return err
		}

		go func() {
			defer func() {
				if p := recover(); p != nil {
					logger.Error(logger.NewEntry().WithMessage(fmt.Sprintf("server[%s] keep alive panic:%v", gs.name, p)))
				}
			}()
			if err := gs.register.KeepAlive(node); err != nil {
				logger.Error(logger.NewEntry().WithMessage(fmt.Sprintf("server[%s] keep alive error:%v", gs.name, err.Error())))
			}
		}()
	}

	logger.Info(logger.NewEntry().WithMessage(fmt.Sprintf("rpc server start listen on: [%d]", gs.port)))

	if err := gs.gSrv.Serve(lis); err != nil {
		return err
	}

	return nil
}

func (gs *grpcServer) runGateway(ctx context.Context) error {
	if gs.gSrvGateway == nil {
		return nil
	}

	go func() {
		defer gs.WaitGroup.Done()
		<-ctx.Done()
		logger.Info(logger.NewEntry().WithMessage(fmt.Sprintf("rpc server gateway stop listen on: [%d]", gs.proxyPort)))

		if err := gs.gSrvGateway.Shutdown(context.Background()); err != nil {
			logger.Fatal(logger.NewEntry().WithMessage(fmt.Sprintf("failed to shutdown http server: %s", err.Error())))
		}
	}()

	if gs.register != nil && gs.name != "" && gs.host != "" {
		node := discovery.NewNode(gs.name, gs.host, gs.proxyPort, discovery.HTTP)
		if err := gs.register.Register(node); err != nil {
			return err
		}
		go func() {
			defer func() {
				if p := recover(); p != nil {
					logger.Error(logger.NewEntry().WithMessage(fmt.Sprintf("server[%s] gateway keep alive panic:%v", gs.name, p)))
				}
			}()
			if err := gs.register.KeepAlive(node); err != nil {
				logger.Error(logger.NewEntry().WithMessage(fmt.Sprintf("server[%s] gateway keep alive error:%v", gs.name, err.Error())))
			}
		}()
	}
	logger.Info(logger.NewEntry().WithMessage(fmt.Sprintf("rpc server gateway start listen on: [%d]", gs.proxyPort)))

	if err := gs.gSrvGateway.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}

func (gs *grpcServer) findListenOn() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		logger.Warn(logger.NewEntry().WithMessage(fmt.Sprintf("failed to get server host, msg[%v]", err.Error())))
		return ""
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP.String()
}

func (gs *grpcServer) Doc() []DocService {
	return nil
}
