package discovery

import (
	"github.com/ringbrew/gsv/service"
	"github.com/ringbrew/gsv/utils"
)

type Type string

const (
	GRPC Type = "grpc"
	HTTP Type = "http"
)

type Node struct {
	Id   string
	Host string
	Port int
	Type Type
	Svc  service.Service
}

func NewNode(host string, port int, t Type, svc service.Service) *Node {
	return &Node{
		Id:   utils.NewNodeId(),
		Host: host,
		Port: port,
		Type: t,
		Svc:  svc,
	}
}
