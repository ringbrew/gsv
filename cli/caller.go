package cli

import (
	"context"
	"sync/atomic"

	"github.com/ringbrew/gsv/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var defaultName atomic.Value

// SetDefaultName sets the calling service name sent to servers by every
// client whose interceptors do not specify one.
func SetDefaultName(name string) {
	defaultName.Store(name)
}

func DefaultName() string {
	name, _ := defaultName.Load().(string)
	return name
}

// CallerUnaryInterceptor sends name as the calling service in metadata. An
// empty name falls back to DefaultName at call time.
func CallerUnaryInterceptor(name string) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		callOpts ...grpc.CallOption,
	) error {
		return invoker(withCaller(ctx, name), method, req, reply, cc, callOpts...)
	}
}

// CallerStreamInterceptor is the stream counterpart of CallerUnaryInterceptor.
func CallerStreamInterceptor(name string) grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		callOpts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		return streamer(withCaller(ctx, name), desc, cc, method, callOpts...)
	}
}

// withCaller overwrites rather than appends, since services that forward
// incoming metadata would otherwise pass on their own caller's name.
func withCaller(ctx context.Context, name string) context.Context {
	if name == "" {
		name = DefaultName()
	}
	if name == "" {
		return ctx
	}
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set(service.MetaKeyCaller, name)
	return metadata.NewOutgoingContext(ctx, md)
}
