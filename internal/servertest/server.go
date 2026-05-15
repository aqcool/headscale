// Package servertest provides an in-process test harness for Headscale's
// control plane.
package servertest

import (
	"net"
	"net/http"
	"net/netip"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	hscontrol "github.com/juanfont/headscale-v2/internal/server"
	"github.com/juanfont/headscale-v2/internal/state"
	"github.com/juanfont/headscale-v2/internal/types"
	"tailscale.com/net/memnet"
	"tailscale.com/tailcfg"
)

// TestServer is an in-process Headscale control server.
type TestServer struct {
	App *hscontrol.Headscale
	URL string

	memNet     *memnet.Network
	ln         net.Listener
	httpServer *http.Server
	st         *state.State
}

// ServerOption configures a TestServer.
type ServerOption func(*serverConfig)

type serverConfig struct {
	batchDelay       time.Duration
	bufferedChanSize int
	ephemeralTimeout time.Duration
	nodeExpiry       time.Duration
	batcherWorkers   int
}

func defaultServerConfig() *serverConfig {
	return &serverConfig{
		batchDelay:       50 * time.Millisecond,
		bufferedChanSize: 30,
		batcherWorkers:   1,
		ephemeralTimeout: 30 * time.Second,
	}
}

// WithBatchDelay sets the batcher's change coalescing delay.
func WithBatchDelay(d time.Duration) ServerOption {
	return func(c *serverConfig) { c.batchDelay = d }
}

// WithBufferedChanSize sets the per-node map session channel buffer.
func WithBufferedChanSize(n int) ServerOption {
	return func(c *serverConfig) { c.bufferedChanSize = n }
}

// WithEphemeralTimeout sets the ephemeral node inactivity timeout.
func WithEphemeralTimeout(d time.Duration) ServerOption {
	return func(c *serverConfig) { c.ephemeralTimeout = d }
}

// WithNodeExpiry sets the default node key expiry duration.
func WithNodeExpiry(d time.Duration) ServerOption {
	return func(c *serverConfig) { c.nodeExpiry = d }
}

// NewServer creates and starts a Headscale test server.
func NewServer(tb testing.TB, opts ...ServerOption) *TestServer {
	tb.Helper()

	sc := defaultServerConfig()
	for _, o := range opts {
		o(sc)
	}

	tmpDir := tb.TempDir()

	prefixV4 := netip.MustParsePrefix("100.64.0.0/10")
	prefixV6 := netip.MustParsePrefix("fd7a:115c:a1e0::/48")

	cfg := &types.Config{
		ServerURL:           "http://localhost:0",
		NoisePrivateKeyPath: tmpDir + "/noise_private.key",
		PrefixV4:            &prefixV4,
		PrefixV6:            &prefixV6,
		IPAllocation:        types.IPAllocationStrategySequential,
	}

	app, err := hscontrol.NewHeadscale(cfg, log.DefaultLogger)
	if err != nil {
		tb.Fatalf("servertest: NewHeadscale: %v", err)
	}

	// Set a minimal DERP map
	app.GetState().SetDERPMap(&tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{
			900: {
				RegionID:   900,
				RegionCode: "test",
				RegionName: "Test Region",
				Nodes: []*tailcfg.DERPNode{{
					Name:     "test0",
					RegionID: 900,
					HostName: "127.0.0.1",
					IPv4:     "127.0.0.1",
					DERPPort: -1,
				}},
			},
		},
	})

	// Start in-memory network
	var memNetwork memnet.Network

	ln, err := memNetwork.Listen("tcp", "127.0.0.1:443")
	if err != nil {
		tb.Fatalf("servertest: memnet Listen: %v", err)
	}

	httpServer := &http.Server{
		Handler:           app.HTTPHandler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go httpServer.Serve(ln)

	serverURL := "http://" + ln.Addr().String()

	ts := &TestServer{
		App:        app,
		URL:        serverURL,
		memNet:     &memNetwork,
		ln:         ln,
		httpServer: httpServer,
		st:         app.GetState(),
	}

	tb.Cleanup(ts.Close)

	return ts
}

// State returns the server's state manager.
func (s *TestServer) State() *state.State {
	return s.st
}

// Close shuts down the in-memory HTTP server.
func (s *TestServer) Close() {
	s.httpServer.Close()
	s.ln.Close()
}

// MemNet returns the in-memory network.
func (s *TestServer) MemNet() *memnet.Network {
	return s.memNet
}

// CreateUser creates a test user.
func (s *TestServer) CreateUser(tb testing.TB, name string) *types.User {
	tb.Helper()

	u, err := s.st.CreateUser(name)
	if err != nil {
		tb.Fatalf("servertest: CreateUser(%q): %v", name, err)
	}

	return u
}

// CreatePreAuthKey creates a reusable pre-auth key for the given user.
func (s *TestServer) CreatePreAuthKey(tb testing.TB, userID uint64) string {
	tb.Helper()

	pak, err := s.st.CreatePreAuthKey(userID, true, false, nil)
	if err != nil {
		tb.Fatalf("servertest: CreatePreAuthKey: %v", err)
	}

	return pak.Key
}

// CreateEphemeralPreAuthKey creates an ephemeral pre-auth key.
func (s *TestServer) CreateEphemeralPreAuthKey(tb testing.TB, userID uint64) string {
	tb.Helper()

	pak, err := s.st.CreatePreAuthKey(userID, false, true, nil)
	if err != nil {
		tb.Fatalf("servertest: CreateEphemeralPreAuthKey: %v", err)
	}

	return pak.Key
}
