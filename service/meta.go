package service

import "context"

// MetaKeyCaller carries the calling service name in gRPC metadata.
const MetaKeyCaller = "x-gsv-caller"

// HeaderCaller carries the calling service name in HTTP requests.
const HeaderCaller = "X-Gsv-Caller"

type serverNameCtxKey struct{}

// WithServerName returns a context carrying the name of the serving service.
func WithServerName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, serverNameCtxKey{}, name)
}

// ServerNameFromContext returns the serving service name, or "" if unset.
func ServerNameFromContext(ctx context.Context) string {
	name, _ := ctx.Value(serverNameCtxKey{}).(string)
	return name
}
