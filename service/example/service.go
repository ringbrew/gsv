package example

import (
	"github.com/ringbrew/gsv/service"
	"github.com/ringbrew/gsv/service/example/export/example"
	"google.golang.org/grpc"
)

type Service struct {
	example.UnimplementedServiceServer
}

func NewService() service.Service {
	return &Service{}
}

func (s *Service) Name() string {
	return "example"
}

func (s *Service) Remark() string {
	return "remark"
}

func (s *Service) Description() service.Description {
	return service.Description{
		Valid:           true,
		GrpcServiceDesc: []grpc.ServiceDesc{example.Service_ServiceDesc},
		GrpcGateway:     nil,
	}
}
