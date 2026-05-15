package servertest

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/juanfont/headscale-v2/internal/types"
	"tailscale.com/control/controlclient"
	"tailscale.com/health"
	"tailscale.com/net/netmon"
	"tailscale.com/net/tsdial"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	"tailscale.com/types/netmap"
	"tailscale.com/types/persist"
	"tailscale.com/util/eventbus"
)

// TestClient wraps a Tailscale controlclient.Direct.
type TestClient struct {
	Name string

	server  *TestServer
	direct  *controlclient.Direct
	authKey string
	user    *types.User

	pollCtx    context.Context
	pollCancel context.CancelFunc
	pollDone   chan struct{}

	mu      sync.RWMutex
	netmap  *netmap.NetworkMap
	history []*netmap.NetworkMap
	updates chan *netmap.NetworkMap

	bus     *eventbus.Bus
	dialer  *tsdial.Dialer
	tracker *health.Tracker
}

// ClientOption configures a TestClient.
type ClientOption func(*clientConfig)

type clientConfig struct {
	ephemeral bool
	hostname  string
	tags      []string
	user      *types.User
}

// WithEphemeral makes the client register as ephemeral.
func WithEphemeral() ClientOption {
	return func(c *clientConfig) { c.ephemeral = true }
}

// WithHostname sets the client's hostname.
func WithHostname(name string) ClientOption {
	return func(c *clientConfig) { c.hostname = name }
}

// WithTags sets ACL tags.
func WithTags(tags ...string) ClientOption {
	return func(c *clientConfig) { c.tags = tags }
}

// WithUser sets the user for the client.
func WithUser(user *types.User) ClientOption {
	return func(c *clientConfig) { c.user = user }
}

// NewClient creates a TestClient and registers it.
func NewClient(tb testing.TB, server *TestServer, name string, opts ...ClientOption) *TestClient {
	tb.Helper()

	cc := &clientConfig{hostname: name}
	for _, o := range opts {
		o(cc)
	}

	user := cc.user
	if user == nil {
		user = server.CreateUser(tb, "user-"+name)
	}

	var authKey string
	switch {
	case cc.ephemeral:
		authKey = server.CreateEphemeralPreAuthKey(tb, uint64(user.ID))
	case len(cc.tags) > 0:
		authKey = server.CreatePreAuthKey(tb, uint64(user.ID))
	default:
		authKey = server.CreatePreAuthKey(tb, uint64(user.ID))
	}

	bus := eventbus.New()
	tracker := health.NewTracker(bus)
	dialer := tsdial.NewDialer(netmon.NewStatic())
	dialer.SetBus(bus)
	dialer.SetSystemDialerForTest(server.MemNet().Dial)

	machineKey := key.NewMachine()

	direct, err := controlclient.NewDirect(controlclient.Options{
		Persist:              persist.Persist{},
		GetMachinePrivateKey: func() (key.MachinePrivate, error) { return machineKey, nil },
		ServerURL:            server.URL,
		AuthKey:              authKey,
		Hostinfo: &tailcfg.Hostinfo{
			BackendLogID: "servertest-" + name,
			Hostname:     cc.hostname,
		},
		DiscoPublicKey: key.NewDisco().Public(),
		Logf:           tb.Logf,
		HealthTracker:  tracker,
		Dialer:         dialer,
		Bus:            bus,
	})
	if err != nil {
		tb.Fatalf("servertest: NewDirect(%s): %v", name, err)
	}

	tc := &TestClient{
		Name:    name,
		server:  server,
		direct:  direct,
		authKey: authKey,
		user:    user,
		updates: make(chan *netmap.NetworkMap, 64),
		bus:     bus,
		dialer:  dialer,
		tracker: tracker,
	}

	tb.Cleanup(func() { tc.cleanup() })

	tc.register(tb)
	tc.startPoll(tb)

	return tc
}

func (c *TestClient) register(tb testing.TB) {
	tb.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	url, err := c.direct.TryLogin(ctx, controlclient.LoginDefault)
	if err != nil {
		tb.Fatalf("servertest: TryLogin(%s): %v", c.Name, err)
	}

	if url != "" {
		tb.Fatalf("servertest: TryLogin(%s): unexpected auth URL", c.Name)
	}
}

func (c *TestClient) startPoll(tb testing.TB) {
	tb.Helper()

	c.pollCtx, c.pollCancel = context.WithCancel(context.Background())
	c.pollDone = make(chan struct{})

	go func() {
		defer close(c.pollDone)
		_ = c.direct.PollNetMap(c.pollCtx, c)
	}()
}

// UpdateFullNetmap implements controlclient.NetmapUpdater.
func (c *TestClient) UpdateFullNetmap(nm *netmap.NetworkMap) {
	c.mu.Lock()
	c.netmap = nm
	c.history = append(c.history, nm)
	c.mu.Unlock()

	select {
	case c.updates <- nm:
	default:
	}
}

func (c *TestClient) cleanup() {
	if c.pollCancel != nil {
		c.pollCancel()
	}

	if c.pollDone != nil {
		select {
		case <-c.pollDone:
		case <-time.After(5 * time.Second):
		}
	}

	if c.direct != nil {
		c.direct.Close()
	}

	if c.dialer != nil {
		c.dialer.Close()
	}

	if c.bus != nil {
		c.bus.Close()
	}
}

// Netmap returns the latest NetworkMap.
func (c *TestClient) Netmap() *netmap.NetworkMap {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.netmap
}

// WaitForPeers blocks until the client sees at least n peers.
func (c *TestClient) WaitForPeers(tb testing.TB, n int, timeout time.Duration) {
	tb.Helper()

	deadline := time.After(timeout)

	for {
		if nm := c.Netmap(); nm != nil && len(nm.Peers) >= n {
			return
		}

		select {
		case <-c.updates:
		case <-deadline:
			nm := c.Netmap()
			got := 0
			if nm != nil {
				got = len(nm.Peers)
			}
			tb.Fatalf("servertest: WaitForPeers(%s, %d): timeout (got %d)", c.Name, n, got)
		}
	}
}

// WaitForUpdate blocks until the next netmap update.
func (c *TestClient) WaitForUpdate(tb testing.TB, timeout time.Duration) *netmap.NetworkMap {
	tb.Helper()

	select {
	case nm := <-c.updates:
		return nm
	case <-time.After(timeout):
		tb.Fatalf("servertest: WaitForUpdate(%s): timeout", c.Name)
		return nil
	}
}

// String implements fmt.Stringer.
func (c *TestClient) String() string {
	nm := c.Netmap()
	if nm == nil {
		return fmt.Sprintf("TestClient(%s, no netmap)", c.Name)
	}
	return fmt.Sprintf("TestClient(%s, %d peers)", c.Name, len(nm.Peers))
}
