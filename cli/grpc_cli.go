package cli

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Option struct {
	Secure             bool
	StreamInterceptors []grpc.StreamClientInterceptor
	UnaryInterceptors  []grpc.UnaryClientInterceptor
}

func Classic() Option {
	return Option{
		Secure: false,
		StreamInterceptors: []grpc.StreamClientInterceptor{
			TraceStreamInterceptor(),
		},
		UnaryInterceptors: []grpc.UnaryClientInterceptor{
			TraceUnaryInterceptor(),
			LogUnaryInterceptor(),
		},
	}
}

type Client interface {
	Conn() *grpc.ClientConn
}

type client struct {
	conn *grpc.ClientConn
}

func (c *client) Conn() *grpc.ClientConn {
	return c.conn
}

func NewClient(target string, opts ...Option) (Client, error) {
	opt := Classic()

	if len(opts) > 0 {
		opt = opts[0]
	}

	dialOpts := make([]grpc.DialOption, 0)

	if !opt.Secure {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	if len(opt.UnaryInterceptors) > 0 {
		dialOpts = append(dialOpts, grpc.WithChainUnaryInterceptor(opt.UnaryInterceptors...))
	}

	if len(opt.StreamInterceptors) > 0 {
		dialOpts = append(dialOpts, grpc.WithChainStreamInterceptor(opt.StreamInterceptors...))
	}

	conn, err := grpc.Dial(target, dialOpts...)
	if err != nil {
		return nil, err
	}

	c := &client{
		conn: conn,
	}

	return c, nil
}
