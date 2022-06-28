package discovery

import (
	"github.com/google/uuid"
)

type Type string

const (
	GRPC Type = "grpc"
	HTTP Type = "http"
)

type Node struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
	Type Type   `json:"type"`
}

func NewNode(name, host string, port int, t Type) *Node {
	return &Node{
		Id:   uuid.New().String(),
		Name: name,
		Host: host,
		Port: port,
		Type: t,
	}
}
