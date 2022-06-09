package server

import (
	"context"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/ringbrew/gsv/logger"
	"github.com/ringbrew/gsv/service"
	"github.com/urfave/negroni"
	"net/http"
)

type httpServer struct {
	host        string
	port        int
	router      *mux.Router      //需要验证身份的路由
	srv         *negroni.Negroni //negroni服务器
	certFile    string           //证书路径
	keyFile     string           //证书路径
	serviceList []service.Service
}

func newHttpServer(opts ...Option) *httpServer {
	opt := Classic()
	if len(opts) > 0 {
		opt = opts[0]
	}

	s := negroni.New()

	for i := range opt.HttpMiddleware {
		m := opt.HttpMiddleware[i]
		if opt.Name != "" {
			if sn, ok := m.(SetNamer); ok {
				sn.SetName(opt.Name)
			}
		}
		s.Use(m)
	}

	return &httpServer{
		host:   opt.Host,
		port:   opt.Port,
		router: mux.NewRouter(),
		srv:    s,
	}
}

func (s *httpServer) Register(svc service.Service) error {
	s.serviceList = append(s.serviceList, svc)
	for i := range s.serviceList {
		desc := s.serviceList[i].Description()
		for ii := range desc.HttpRoute {
			routeInfo := desc.HttpRoute[ii]
			if routeInfo.Method == service.MethodAll {
				s.router.HandleFunc(routeInfo.Path, routeInfo.Handler)
			} else {
				s.router.HandleFunc(routeInfo.Path, routeInfo.Handler).Methods(routeInfo.Method)
			}
		}
	}
	return nil
}

func (s *httpServer) Run(ctx context.Context) {
	s.srv.UseHandler(s.router)

	hs := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: s.srv,
	}
	go func() {
		<-ctx.Done()
		logger.Info(logger.NewEntry().WithMessage(fmt.Sprintf("http server stop listen on: [%d]", s.port)))

		if err := hs.Shutdown(context.Background()); err != nil {
			logger.Fatal(logger.NewEntry().WithMessage(fmt.Sprintf("failed to shutdown http server: %s", err.Error())))
		}
	}()

	if s.certFile != "" && s.keyFile != "" {
		logger.Info(logger.NewEntry().WithMessage(fmt.Sprintf("http server start listen tls on: [%d]", s.port)))

		if err := hs.ListenAndServeTLS(s.certFile, s.keyFile); err != nil && err != http.ErrServerClosed {
			logger.Fatal(logger.NewEntry().WithMessage(fmt.Sprintf("http server listen tls error: %s", err.Error())))
		}
	} else {
		logger.Info(logger.NewEntry().WithMessage(fmt.Sprintf("http server start listen on: [%d]", s.port)))

		if err := hs.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal(logger.NewEntry().WithMessage(fmt.Sprintf("http server listen error: %s", err.Error())))
		}
	}
}
