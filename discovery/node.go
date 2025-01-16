package discovery

import (
	"github.com/google/uuid"
	"sync"
)

type Type string

const (
	GRPC Type = "grpc"
	HTTP Type = "http"
)

type Node struct {
	Id    string   `json:"id"`
	Name  string   `json:"name"`
	Host  string   `json:"host"`
	Port  int      `json:"port"`
	Type  Type     `json:"type"`
	Extra sync.Map `json:"-"`
}

func NewNode(name, host string, port int, t Type, id ...string) *Node {
	n := &Node{
		Name: name,
		Host: host,
		Port: port,
		Type: t,
	}

	if len(id) > 0 && id[0] != "" {
		n.Id = id[0]
	} else {
		n.Id = uuid.New().String()
	}

	return n
}
