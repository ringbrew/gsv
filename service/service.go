package service

import (
	"context"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"net/http"
)

type Service interface {
	/*
		Name 服务名称
	*/
	Name() string
	/*
		Remark 服务说明
	*/
	Remark() string
	/*
		Description 服务描述
	*/
	Description() Description
}

type Description struct {
	Valid           bool
	GrpcServiceDesc []grpc.ServiceDesc
	GrpcGateway     []GatewayRegister
	HttpRoute       HttpRouteCollector
}

type GatewayRegister func(ctx context.Context, mux *runtime.ServeMux, conn *grpc.ClientConn) error

const MethodAll = "all"
const ContentTypeJSON = "application/json"

type HttpRoute struct {
	Path    string
	Method  string
	Handler http.HandlerFunc
	Meta    HttpMeta
}

func NewHttpRoute(method string, path string, handler http.HandlerFunc, meta ...HttpMeta) HttpRoute {
	result := HttpRoute{
		Path:    path,
		Method:  method,
		Handler: handler,
	}

	if len(meta) > 0 {
		result.Meta = meta[0]
	}

	return result
}

type HttpMeta struct {
	ContentType string
	Request     interface{}
	Response    interface{}
	Remark      string
}

type HttpRouteCollector []HttpRoute

func (c *HttpRouteCollector) append(method string, path string, handler http.HandlerFunc, meta ...HttpMeta) {
	r := HttpRoute{
		Method:  method,
		Path:    path,
		Handler: handler,
	}

	if len(meta) > 0 {
		r.Meta = meta[0]
	}

	*c = append(*c, r)
}

func (c *HttpRouteCollector) Map(path string, handler http.HandlerFunc, meta ...HttpMeta) {
	c.append(MethodAll, path, handler, meta...)
}

func (c *HttpRouteCollector) MapMethods(method string, path string, handler http.HandlerFunc, meta ...HttpMeta) {
	c.append(method, path, handler, meta...)
}

func (c *HttpRouteCollector) Get(path string, handler http.HandlerFunc, meta ...HttpMeta) {
	c.MapMethods(http.MethodGet, path, handler, meta...)
}

func (c *HttpRouteCollector) Post(path string, handler http.HandlerFunc, meta ...HttpMeta) {
	c.MapMethods(http.MethodPost, path, handler, meta...)
}

func (c *HttpRouteCollector) Put(path string, handler http.HandlerFunc, meta ...HttpMeta) {
	c.MapMethods(http.MethodPut, path, handler, meta...)
}

func (c *HttpRouteCollector) Delete(path string, handler http.HandlerFunc, meta ...HttpMeta) {
	c.MapMethods(http.MethodDelete, path, handler, meta...)
}
