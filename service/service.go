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

type HttpRoute struct {
	Path    string
	Method  string
	Handler http.HandlerFunc
}

type HttpRouteCollector []HttpRoute

func (c *HttpRouteCollector) append(method string, path string, handler http.HandlerFunc) {
	r := HttpRoute{
		Method:  method,
		Path:    path,
		Handler: handler,
	}
	*c = append(*c, r)
}

func (c *HttpRouteCollector) MapMethods(method string, path string, handler http.HandlerFunc) {
	c.append(method, path, handler)
}

func (c *HttpRouteCollector) Get(path string, handler http.HandlerFunc) {
	c.MapMethods(http.MethodGet, path, handler)
}

func (c *HttpRouteCollector) Post(path string, handler http.HandlerFunc) {
	c.MapMethods(http.MethodPost, path, handler)
}

func (c *HttpRouteCollector) Put(path string, handler http.HandlerFunc) {
	c.MapMethods(http.MethodPut, path, handler)
}

func (c *HttpRouteCollector) Delete(path string, handler http.HandlerFunc) {
	c.MapMethods(http.MethodDelete, path, handler)
}
