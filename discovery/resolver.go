package discovery

import (
	"fmt"
	"github.com/ringbrew/gsv/logger"
	"google.golang.org/grpc/resolver"
	"time"
)

const SchemeName = "gsv"

func Register(nd NodeDiscover) {
	resolver.Register(NewResolverBuilder(nd))
}

type ResolverBuilder struct {
	nd NodeDiscover
}

func NewResolverBuilder(nd NodeDiscover) *ResolverBuilder {
	return &ResolverBuilder{
		nd: nd,
	}
}

func (rb *ResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	eventChan := rb.nd.Watch(target.URL.Path, GRPC)

	nodeList := rb.nd.Node(target.URL.Path, GRPC)

	r := &gsvResolver{
		target:    target,
		cc:        cc,
		cache:     map[string]Node{},
		eventChan: eventChan,
	}

	for _, v := range nodeList {
		r.cache[v.Id] = v
	}

	go func() {
		defer func() {
			if p := recover(); p != nil {
				logger.Error(logger.NewEntry().WithMessage(fmt.Sprintf("service[%s] checker panic:%v", target.URL.Path, p)))
			}
		}()
		ticker := time.NewTicker(time.Minute)
		for range ticker.C {
			nl := rb.nd.Node(target.URL.Path, GRPC)
			eventChan <- NodeEvent{
				Event: NodeEventSync,
				Node:  nl,
			}
		}
	}()

	go func() {
		defer func() {
			if p := recover(); p != nil {
				logger.Error(logger.NewEntry().WithMessage(fmt.Sprintf("service[%s] watch panic:%v", target.URL.Path, p)))
			}
		}()
		r.watch()
	}()
	return r, nil
}

func (*ResolverBuilder) Scheme() string { return SchemeName }

type gsvResolver struct {
	target    resolver.Target
	cc        resolver.ClientConn
	cache     map[string]Node
	eventChan chan NodeEvent
}

func (r *gsvResolver) watch() {
	updateState := func() {
		resolverAddr := make([]resolver.Address, 0, len(r.cache))
		for _, v := range r.cache {
			endpoint := fmt.Sprintf("%s:%d", v.Host, v.Port)
			resolverAddr = append(resolverAddr, resolver.Address{Addr: endpoint})
		}
		_ = r.cc.UpdateState(resolver.State{Addresses: resolverAddr})
	}

	for event := range r.eventChan {
		switch event.Event {
		case NodeEventAdd:
			for _, node := range event.Node {
				r.cache[node.Id] = node
			}
			updateState()
		case NodeEventRemove:
			for _, node := range event.Node {
				delete(r.cache, node.Id)
			}
			updateState()
		case NodeEventSync:
			r.cache = make(map[string]Node)
			for _, node := range event.Node {
				r.cache[node.Id] = node
			}
			updateState()
		}
	}
}

func (*gsvResolver) ResolveNow(o resolver.ResolveNowOptions) {}

func (*gsvResolver) Close() {}
