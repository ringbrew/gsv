package cli

import (
	"github.com/ringbrew/gsv/discovery"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type LoadBalancePolicy int

const (
	LoadBalancePolicyRoundRobin LoadBalancePolicy = iota
	LoadBalancePolicyRingHash
)

type Option struct {
	// Name is the calling service name sent to servers. It overrides the name
	// given to Classic and SetDefaultName.
	Name               string
	Secure             bool
	LoadBalancePolicy  LoadBalancePolicy
	StreamInterceptors []grpc.StreamClientInterceptor
	UnaryInterceptors  []grpc.UnaryClientInterceptor
}

// Classic returns the default client option. The optional name is sent to
// servers as the calling service; without it SetDefaultName is used.
func Classic(name ...string) Option {
	caller := ""
	if len(name) > 0 {
		caller = name[0]
	}
	return Option{
		Secure: false,
		StreamInterceptors: []grpc.StreamClientInterceptor{
			CallerStreamInterceptor(caller),
			TraceStreamInterceptor(),
		},
		UnaryInterceptors: []grpc.UnaryClientInterceptor{
			CallerUnaryInterceptor(caller),
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

	unary := opt.UnaryInterceptors
	stream := opt.StreamInterceptors
	if opt.Name != "" {
		// appended last so it runs after, and wins over, any caller interceptor in the list
		unary = append(unary[:len(unary):len(unary)], CallerUnaryInterceptor(opt.Name))
		stream = append(stream[:len(stream):len(stream)], CallerStreamInterceptor(opt.Name))
	}

	if len(unary) > 0 {
		dialOpts = append(dialOpts, grpc.WithChainUnaryInterceptor(unary...))
	}

	if len(stream) > 0 {
		dialOpts = append(dialOpts, grpc.WithChainStreamInterceptor(stream...))
	}

	switch opt.LoadBalancePolicy {
	case LoadBalancePolicyRoundRobin:
		dialOpts = append(dialOpts, grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`))
	case LoadBalancePolicyRingHash:
		dialOpts = append(dialOpts, grpc.WithDefaultServiceConfig(discovery.ConsistentServiceConfigJSON))
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
