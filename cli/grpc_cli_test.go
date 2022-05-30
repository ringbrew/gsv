package cli

import (
	"context"
	"fmt"
	"github.com/ringbrew/gsv/service/example/export/example"
	"github.com/ringbrew/gsv/tracex"
	"github.com/ringbrew/gsvcore/logger"
	"testing"
)

func TestCli(t *testing.T) {
	tracex.Init()

	c, err := NewClient("localhost:3000")
	if err != nil {
		t.Error(err)
		return
	}

	exampleCli := example.NewServiceClient(c.Conn())
	resp, err := exampleCli.GetExample(context.Background(), &example.GetExampleReq{Name: "test"})
	if err != nil {
		t.Error(err)
		return
	}

	logger.Info(logger.NewEntry().WithMessage(fmt.Sprintf("%v", resp)))
}
