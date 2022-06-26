package server

import (
	"bytes"
	"context"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/ringbrew/gsv/logger"
	"github.com/ringbrew/gsv/service"
	"github.com/urfave/negroni"
	"net/http"
	"reflect"
	"text/template"
)

type httpServer struct {
	host        string
	port        int
	router      *mux.Router      //需要验证身份的路由
	srv         *negroni.Negroni //negroni服务器
	certFile    string           //证书路径
	keyFile     string           //证书路径
	serviceList []service.Service
	enableDoc   bool
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
		host:      opt.Host,
		port:      opt.Port,
		router:    mux.NewRouter(),
		srv:       s,
		enableDoc: opt.EnableDocServer,
	}
}

func (s *httpServer) Register(svc service.Service) error {
	s.serviceList = append(s.serviceList, svc)
	desc := svc.Description()
	for ii := range desc.HttpRoute {
		routeInfo := desc.HttpRoute[ii]
		if routeInfo.Method == service.MethodAll {
			s.router.HandleFunc(routeInfo.Path, routeInfo.Handler)
		} else {
			s.router.HandleFunc(routeInfo.Path, routeInfo.Handler).Methods(routeInfo.Method)
		}
	}
	return nil
}

func (s *httpServer) RunDoc(ctx context.Context) {
	http.HandleFunc("/http/service/api/docs", func(writer http.ResponseWriter, request *http.Request) {
		result := make([]HttpDocService, 0, len(s.serviceList))
		for i := range s.serviceList {
			hds := HttpDocService{
				Name: s.serviceList[i].Remark(),
			}

			if hds.Name == "" {
				continue
			}
			desc := s.serviceList[i].Description()
			for ii := range desc.HttpRoute {
				dhr := desc.HttpRoute[ii]
				hda := HttpDocApi{
					Name:        dhr.Meta.Remark,
					Path:        dhr.Path,
					Method:      dhr.Method,
					ContentType: dhr.Meta.ContentType,
				}

				if dhr.Meta.Request != nil {
					apiReq := structInfo(reflect.TypeOf(dhr.Meta.Request))
					hda.Request = append(hda.Request, apiReq...)
				}

				if dhr.Meta.Response != nil {
					apiResp := structInfo(reflect.TypeOf(dhr.Meta.Response))
					hda.Response = append(hda.Response, apiResp...)
				}

				hds.Api = append(hds.Api, hda)
			}
			result = append(result, hds)

			b := make([]byte, 0, 1024)
			buf := bytes.NewBuffer(b)
			tmpl, err := template.New("apiDocTmpl").Parse(apiDocTmpl)
			if err != nil {
				logger.Error(logger.NewEntry(request.Context()).WithMessage(fmt.Sprintf("template parse error:%s", err.Error())))
				writer.WriteHeader(http.StatusInternalServerError)
				return
			}

			if err := tmpl.Execute(buf, result); err != nil {
				logger.Error(logger.NewEntry(request.Context()).WithMessage(fmt.Sprintf("template parse error:%s", err.Error())))
				writer.WriteHeader(http.StatusInternalServerError)
				return
			}

			writer.Header().Set("Content-Type", "application/octet-stream")
			writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment;filename=http_api.md"))
			writer.Write(buf.Bytes())
		}
	})

	http.ListenAndServe(":9090", nil)
}

func (s *httpServer) Run(ctx context.Context) {
	s.srv.UseHandler(s.router)

	if s.enableDoc {
		go func() {
			defer func() {
				if p := recover(); p != nil {
					logger.Error(logger.NewEntry().WithMessage(fmt.Sprintf("http doc server panic: %v", p)))
				}
			}()
			s.RunDoc(ctx)
		}()
	}

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
