package servertest

import (
	"fmt"
	"testing"
	"time"

	"github.com/juanfont/headscale-v2/internal/types"
)

// TestHarness orchestrates a TestServer with multiple TestClients.
type TestHarness struct {
	Server  *TestServer
	clients []*TestClient

	defaultUser *types.User
}

// HarnessOption configures a TestHarness.
type HarnessOption func(*harnessConfig)

type harnessConfig struct {
	serverOpts     []ServerOption
	clientOpts     []ClientOption
	convergenceMax time.Duration
}

func defaultHarnessConfig() *harnessConfig {
	return &harnessConfig{
		convergenceMax: 30 * time.Second,
	}
}

// WithServerOptions passes ServerOptions through to the TestServer.
func WithServerOptions(opts ...ServerOption) HarnessOption {
	return func(c *harnessConfig) { c.serverOpts = append(c.serverOpts, opts...) }
}

// WithDefaultClientOptions applies ClientOptions to every client.
func WithDefaultClientOptions(opts ...ClientOption) HarnessOption {
	return func(c *harnessConfig) { c.clientOpts = append(c.clientOpts, opts...) }
}

// WithConvergenceTimeout sets how long WaitForMeshComplete waits.
func WithConvergenceTimeout(d time.Duration) HarnessOption {
	return func(c *harnessConfig) { c.convergenceMax = d }
}

// NewHarness creates a TestServer and numClients connected clients.
func NewHarness(tb testing.TB, numClients int, opts ...HarnessOption) *TestHarness {
	tb.Helper()

	hc := defaultHarnessConfig()
	for _, o := range opts {
		o(hc)
	}

	server := NewServer(tb, hc.serverOpts...)

	user := server.CreateUser(tb, "harness-default")

	h := &TestHarness{
		Server:      server,
		defaultUser: user,
	}

	for i := range numClients {
		name := clientName(i)
		copts := append([]ClientOption{WithUser(user)}, hc.clientOpts...)
		c := NewClient(tb, server, name, copts...)
		h.clients = append(h.clients, c)
	}

	return h
}

// Client returns the i-th client (0-indexed).
func (h *TestHarness) Client(i int) *TestClient {
	return h.clients[i]
}

// Clients returns all clients.
func (h *TestHarness) Clients() []*TestClient {
	return h.clients
}

// DefaultUser returns the shared user.
func (h *TestHarness) DefaultUser() *types.User {
	return h.defaultUser
}

func clientName(index int) string {
	return fmt.Sprintf("node-%d", index)
}
