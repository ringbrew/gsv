package server

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/ringbrew/gsv/logger"
	"github.com/ringbrew/gsv/service"
	"github.com/urfave/negroni"
	"net/http"
	"reflect"
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
	desc := svc.Description()
	middlewares := s.srv.Handlers()

	for ii := range desc.HttpRoute {
		routeInfo := desc.HttpRoute[ii]
		if routeInfo.Method == service.MethodAll {
			s.router.HandleFunc(routeInfo.Path, routeInfo.Handler)
		} else {
			s.router.HandleFunc(routeInfo.Path, routeInfo.Handler).Methods(routeInfo.Method)
		}
	}

	for i := range middlewares {
		m := middlewares[i]
		if patcher, ok := m.(ServicePatcher); ok {
			if err := patcher.Patch(svc); err != nil {
				return err
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

func (s *httpServer) Doc() []DocService {
	result := make([]DocService, 0, len(s.serviceList))
	for i := range s.serviceList {
		hds := DocService{
			Key:  s.serviceList[i].Name(),
			Name: s.serviceList[i].Remark(),
		}

		if hds.Name == "" {
			continue
		}
		desc := s.serviceList[i].Description()
		for ii := range desc.HttpRoute {
			dhr := desc.HttpRoute[ii]
			hda := DocApi{
				Name:        dhr.Meta.Remark,
				Path:        dhr.Path,
				Method:      dhr.Method,
				ContentType: dhr.Meta.ContentType,
			}

			if dhr.Meta.Request != nil {
				apiReq := structInfo(reflect.TypeOf(dhr.Meta.Request))
				hda.Request = append(hda.Request, apiReq...)
				hda.RequestExample = s.newExample(dhr.Meta.Request)
			}

			if dhr.Meta.Response != nil {
				apiResp := structInfo(reflect.TypeOf(dhr.Meta.Response))
				hda.Response = append(hda.Response, apiResp...)
				hda.ResponseExample = s.newExample(dhr.Meta.Response)
			}

			hds.Api = append(hds.Api, hda)
		}
		result = append(result, hds)
	}

	return result
}

func (s *httpServer) newExample(object interface{}) string {
	var example string
	rt := reflect.TypeOf(object)
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem() // use Elem to get the pointed-to-type
	}

	if rt.Kind() == reflect.Slice {
		sv := reflect.New(reflect.TypeOf([]interface{}{})).Elem()

		objectType := rt.Elem() // use Elem to get type of slice's element

		if objectType.Kind() == reflect.Ptr {
			objectType = objectType.Elem()
		}

		ssv := reflect.Append(sv, reflect.New(objectType))

		e, _ := json.MarshalIndent(ssv.Interface(), "", "	")
		example = string(e)
	} else {
		e, _ := json.MarshalIndent(reflect.New(rt).Interface(), "", "	")
		example = string(e)
	}

	return example
}
