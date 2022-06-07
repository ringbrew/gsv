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
	HttpRoute       []HttpRoute
}

type GatewayRegister func(ctx context.Context, mux *runtime.ServeMux, conn *grpc.ClientConn) error

type HttpRoute struct {
	Path    string
	Method  string
	Handler http.HandlerFunc
}
