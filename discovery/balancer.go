package discovery

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"hash/maphash"
	"sync"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/serviceconfig"

	"github.com/ringbrew/gsv/discovery/hashring"
)

var gl = grpclog.Component("consistenthashring")

type ctxKey string

const (
	BalancerName = "consistent-hashring"

	cKey ctxKey = "consistent-hashring-key"

	DefaultReplicationFactor = 100

	DefaultSpread = 1
)

var ConsistentServiceConfigJSON = (&BalancerConfig{
	ReplicationFactor: DefaultReplicationFactor,
	Spread:            DefaultSpread,
}).MustServiceConfigJSON()

type BalancerConfig struct {
	serviceconfig.LoadBalancingConfig `json:"-"`
	ReplicationFactor                 uint16 `json:"replicationFactor,omitempty"`
	Spread                            uint8  `json:"spread,omitempty"`
}

func (c *BalancerConfig) ServiceConfigJSON() (string, error) {
	type wrapper struct {
		Config []map[string]*BalancerConfig `json:"loadBalancingConfig"`
	}

	out := wrapper{Config: []map[string]*BalancerConfig{{BalancerName: c}}}

	j, err := json.Marshal(out)
	if err != nil {
		return "", err
	}

	return string(j), nil
}

func (c *BalancerConfig) MustServiceConfigJSON() string {
	o, err := c.ServiceConfigJSON()
	if err != nil {
		panic(err)
	}
	return o
}

func NewBuilder(hashfn hashring.HashFunc) Builder {
	return &builder{hashfn: hashfn}
}

type subConnMember struct {
	balancer.SubConn
	key string
}

func (s subConnMember) Key() string { return s.key }

var _ hashring.Member = (*subConnMember)(nil)

type builder struct {
	sync.Mutex
	hashfn hashring.HashFunc
	config BalancerConfig
}

type Builder interface {
	balancer.Builder
	balancer.ConfigParser
}

var _ Builder = (*builder)(nil)

func (b *builder) Name() string { return BalancerName }

func (b *builder) Build(cc balancer.ClientConn, _ balancer.BuildOptions) balancer.Balancer {
	bal := &ringBalancer{
		cc:       cc,
		subConns: resolver.NewAddressMap(),
		scStates: make(map[balancer.SubConn]connectivity.State),
		csEvltr:  &balancer.ConnectivityStateEvaluator{},
		state:    connectivity.Connecting,
		hasher:   b.hashfn,
		picker:   base.NewErrPicker(balancer.ErrNoSubConnAvailable),
	}

	return bal
}

func (b *builder) ParseConfig(js json.RawMessage) (serviceconfig.LoadBalancingConfig, error) {
	var lbCfg BalancerConfig
	if err := json.Unmarshal(js, &lbCfg); err != nil {
		return nil, fmt.Errorf("wrr: unable to unmarshal LB policy config: %s, error: %w", string(js), err)
	}

	gl.Infof("parsed balancer config %s", js)

	if lbCfg.ReplicationFactor == 0 {
		lbCfg.ReplicationFactor = DefaultReplicationFactor
	}

	if lbCfg.Spread == 0 {
		lbCfg.Spread = DefaultSpread
	}

	b.Lock()
	b.config = lbCfg
	b.Unlock()

	return &lbCfg, nil
}

type ringBalancer struct {
	state    connectivity.State
	cc       balancer.ClientConn
	picker   balancer.Picker
	csEvltr  *balancer.ConnectivityStateEvaluator
	subConns *resolver.AddressMap
	scStates map[balancer.SubConn]connectivity.State

	config   *BalancerConfig
	hashring *hashring.Ring
	hasher   hashring.HashFunc

	resolverErr error // the last error reported by the resolver; cleared on successful resolution
	connErr     error // the last connection error; cleared upon leaving TransientFailure
}

var _ balancer.Balancer = (*ringBalancer)(nil)

func (b *ringBalancer) ResolverError(err error) {
	b.resolverErr = err
	if b.subConns.Len() == 0 {
		b.state = connectivity.TransientFailure
		b.picker = base.NewErrPicker(errors.Join(b.connErr, b.resolverErr))
	}

	if b.state != connectivity.TransientFailure {
		// The picker will not change since the balancer does not currently
		// report an error.
		return
	}

	b.cc.UpdateState(balancer.State{
		ConnectivityState: b.state,
		Picker:            b.picker,
	})
}

func (b *ringBalancer) getAddrId(addr resolver.Address) string {
	id := ""
	if addr.Attributes != nil {
		if idAny := addr.Attributes.Value("id"); idAny != nil {
			if idStr, hasId := idAny.(string); hasId {
				id = idStr
			}
		}
	}
	return id
}

func (b *ringBalancer) UpdateClientConnState(s balancer.ClientConnState) error {
	if gl.V(2) {
		gl.Info("got new ClientConn state: ", s)
	}
	b.resolverErr = nil

	if s.BalancerConfig != nil {
		svcConfig := s.BalancerConfig.(*BalancerConfig)
		if b.config == nil || svcConfig.ReplicationFactor != b.config.ReplicationFactor {
			b.hashring = hashring.MustNew(b.hasher, svcConfig.ReplicationFactor)
			b.config = svcConfig
		}
	}

	if b.hashring == nil {
		b.picker = base.NewErrPicker(errors.Join(b.connErr, b.resolverErr))
		b.cc.UpdateState(balancer.State{ConnectivityState: b.state, Picker: b.picker})

		return fmt.Errorf("no hashring configured")
	}

	addrSet := resolver.NewAddressMap()
	for _, addr := range s.ResolverState.Addresses {
		addrSet.Set(addr, nil)

		if _, ok := b.subConns.Get(addr); !ok {
			sc, err := b.cc.NewSubConn([]resolver.Address{addr}, balancer.NewSubConnOptions{HealthCheckEnabled: false})
			if err != nil {
				gl.Warningf("base.baseBalancer: failed to create new SubConn: %v", err)
				continue
			}

			b.subConns.Set(addr, sc)
			b.scStates[sc] = connectivity.Idle
			b.csEvltr.RecordTransition(connectivity.Shutdown, connectivity.Idle)
			sc.Connect()

			key := addr.ServerName + addr.Addr
			if id := b.getAddrId(addr); id != "" {
				key = id
			}

			if err := b.hashring.Add(subConnMember{
				SubConn: sc,
				key:     key,
			}); err != nil {
				return fmt.Errorf("couldn't add to hashring")
			}
		}
	}

	for _, addr := range b.subConns.Keys() {
		sci, _ := b.subConns.Get(addr)
		sc := sci.(balancer.SubConn)
		if _, ok := addrSet.Get(addr); !ok {
			b.cc.RemoveSubConn(sc)
			b.subConns.Delete(addr)
			key := addr.ServerName + addr.Addr
			if id := b.getAddrId(addr); id != "" {
				key = id
			}
			if err := b.hashring.Remove(subConnMember{
				SubConn: sc,
				key:     key,
			}); err != nil {
				return fmt.Errorf("couldn't add to hashring")
			}
		}
	}

	if gl.V(2) {
		gl.Infof("%d hashring members found", len(b.hashring.Members()))

		for _, m := range b.hashring.Members() {
			gl.Infof("hashring member %s", m.Key())
		}
	}

	if len(s.ResolverState.Addresses) == 0 {
		b.ResolverError(errors.New("produced zero addresses"))
		return balancer.ErrBadResolverState
	}

	if b.state == connectivity.TransientFailure {
		b.picker = base.NewErrPicker(errors.Join(b.connErr, b.resolverErr))
	} else {
		b.picker = &picker{
			hashring: b.hashring,
			spread:   b.config.Spread,
		}
	}

	b.cc.UpdateState(balancer.State{ConnectivityState: b.state, Picker: b.picker})

	return nil
}

func (b *ringBalancer) UpdateSubConnState(sc balancer.SubConn, state balancer.SubConnState) {
	s := state.ConnectivityState
	if gl.V(2) {
		gl.Infof("base.baseBalancer: handle SubConn state change: %p, %v", sc, s)
	}

	oldS, ok := b.scStates[sc]
	if !ok {
		if gl.V(2) {
			gl.Infof("base.baseBalancer: got state changes for an unknown SubConn: %p, %v", sc, s)
		}

		return
	}

	if oldS == connectivity.TransientFailure &&
		(s == connectivity.Connecting || s == connectivity.Idle) {
		if s == connectivity.Idle {
			sc.Connect()
		}

		return
	}

	b.scStates[sc] = s

	switch s {
	case connectivity.Idle:
		sc.Connect()
	case connectivity.Shutdown:
		delete(b.scStates, sc)
	case connectivity.TransientFailure:
		b.connErr = state.ConnectionError
	}

	b.state = b.csEvltr.RecordTransition(oldS, s)

	b.cc.UpdateState(balancer.State{ConnectivityState: b.state, Picker: b.picker})
}

func (b *ringBalancer) Close() {
}

func (b *ringBalancer) ExitIdle() {
}

type picker struct {
	hashring *hashring.Ring
	spread   uint8
}

var _ balancer.Picker = (*picker)(nil)

func (p *picker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	var key []byte
	if k := info.Ctx.Value(cKey); k != nil {
		if kk, ok := k.([]byte); ok {
			key = kk
		}
	}

	if key == nil || len(key) == 0 {
		key = make([]byte, 10)
		_, err := rand.Read(key)
		if err != nil {
			return balancer.PickResult{}, err
		}
	}

	members, err := p.hashring.FindN(key, p.spread)
	if err != nil {
		return balancer.PickResult{}, err
	}

	index := 0
	if p.spread > 1 {
		index = intn(p.spread)
	}

	chosen := members[index].(subConnMember)

	return balancer.PickResult{SubConn: chosen.SubConn}, nil
}

var intn = func(n uint8) int {
	out := int(new(maphash.Hash).Sum64())
	if out < 0 {
		out = -out
	}
	return out % int(n)
}

func BalanceKey(ctx context.Context, val []byte) context.Context {
	return context.WithValue(ctx, cKey, val)
}
