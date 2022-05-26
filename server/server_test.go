package server

import (
	"context"
	"github.com/ringbrew/gsv/service/example"
	"testing"
)

func TestGrpcServer(t *testing.T) {
	ctx := context.Background()

	s := NewServer(GRPC)

	svc := example.NewService()

	if err := s.Register(svc); err != nil {
		t.Error(err)
		return
	}

	s.Run(ctx)
}
