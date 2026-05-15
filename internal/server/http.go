package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	v1 "github.com/juanfont/headscale-v2/api/proto/v1"
	"github.com/juanfont/headscale-v2/internal/conf"
	"github.com/juanfont/headscale-v2/internal/derp"
	"github.com/juanfont/headscale-v2/internal/ip"
	"github.com/juanfont/headscale-v2/internal/mapper"
	"github.com/juanfont/headscale-v2/internal/noise"
	"github.com/juanfont/headscale-v2/internal/policy"
	"github.com/juanfont/headscale-v2/internal/poll"
	"github.com/juanfont/headscale-v2/internal/service"
	"github.com/juanfont/headscale-v2/internal/state"
	"github.com/juanfont/headscale-v2/internal/types"
	"github.com/juanfont/headscale-v2/internal/util"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/gorilla/handlers"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	"tailscale.com/types/views"
)

const (
	AuthPrefix         = "Bearer "
	updateInterval     = 5 * time.Second
	privateKeyFileMode = 0o600
)

var (
	errSTUNAddressNotSet = errors.New("STUN address not set")
	errEmptyInitialDERPMap = errors.New("initial DERPMap is empty")
)

type HeadscaleServer struct {
	cfg             *types.Config
	httpServer      *kratoshttp.Server
	noiseServer     *noise.NoiseServer
	state           *state.State
	policyMan       policy.PolicyManager
	batcher         *poll.Batcher
	derpManager     *derp.DERPManager
	mapper          *mapper.Mapper
	ipAllocator     *ip.IPAllocator
	logger          *log.Helper
	noisePrivateKey *key.MachinePrivate

	haProber        *state.HAHealthProber
	ephemeralGC     *state.EphemeralGarbageCollector

	clientStreamsOpen sync.WaitGroup
}

func NewHeadscaleServer(
	c *conf.Server,
	headscale *service.HeadscaleService,
	logger log.Logger,
) (*HeadscaleServer, error) {
	helper := log.NewHelper(logger)

	cfg := &types.Config{
		ServerURL:  "http://localhost:8080",
		Addr:       ":8080",
		BaseDomain: "headscale.net",
	}

	privateKey, err := readOrCreatePrivateKey("")
	if err != nil {
		return nil, fmt.Errorf("reading or creating Noise protocol private key: %w", err)
	}

	prefix4 := netip.MustParsePrefix("100.64.0.0/10")
	prefix6 := netip.MustParsePrefix("fd7a:115c:a1e0::/48")
	ipAlloc := ip.NewIPAllocator(&prefix4, &prefix6, types.IPAllocationStrategySequential)

	stateCfg := &state.StateConfig{
		IPAlloc:      ipAlloc,
		BatchSize:    20,
		BatchTimeout: time.Second,
	}
	st := state.NewState(stateCfg, logger)

	polMan, err := policy.NewPolicyManager(nil, []types.User{}, views.SliceOf([]types.NodeView{}))
	if err != nil {
		return nil, fmt.Errorf("creating policy manager: %w", err)
	}

	derpMan := derp.NewDERPManager(&cfg.DERP, logger)
	st.SetDERPMap(derpMan.GetDERPMap())

	batcher := poll.NewBatcher(logger)
	go batcher.Run(context.Background())

	mapr := mapper.NewMapper(st, cfg, logger)

	noiseCfg := &noise.Config{
		ServerURL:  cfg.ServerURL,
		ListenAddr: cfg.Addr,
		PrivateKey: *privateKey,
	}
	noiseSrv := noise.NewNoiseServer(noiseCfg, st, logger)

	var opts = []kratoshttp.ServerOption{
		kratoshttp.Middleware(
			recovery.Recovery(),
		),
	}
	if c.Http.Network != "" {
		opts = append(opts, kratoshttp.Network(c.Http.Network))
	}
	if c.Http.Addr != "" {
		opts = append(opts, kratoshttp.Address(c.Http.Addr))
	}
	if c.Http.Timeout != nil {
		opts = append(opts, kratoshttp.Timeout(c.Http.Timeout.AsDuration()))
	}

	corsHandler := handlers.CORS(
		handlers.AllowedOrigins([]string{"*"}),
		handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
		handlers.AllowedHeaders([]string{"Content-Type", "Authorization"}),
	)
	opts = append(opts, kratoshttp.Filter(corsHandler))

	httpSrv := kratoshttp.NewServer(opts...)
	v1.RegisterHeadscaleServiceHTTPServer(httpSrv, headscale)

	return &HeadscaleServer{
		cfg:             cfg,
		httpServer:      httpSrv,
		noiseServer:     noiseSrv,
		state:           st,
		policyMan:       polMan,
		batcher:         batcher,
		derpManager:     derpMan,
		mapper:          mapr,
		ipAllocator:     ipAlloc,
		logger:          helper,
		noisePrivateKey: privateKey,
	}, nil
}

func (s *HeadscaleServer) Start(ctx context.Context) error {
	go func() {
		if err := s.noiseServer.Start(ctx); err != nil {
			s.logger.Errorf("Noise server error: %v", err)
		}
	}()
	go s.scheduledTasks(ctx)
	return s.httpServer.Start(ctx)
}

func (s *HeadscaleServer) Stop(ctx context.Context) error {
	s.state.Stop()
	return s.httpServer.Stop(ctx)
}

func (s *HeadscaleServer) scheduledTasks(ctx context.Context) {
	expireTicker := time.NewTicker(updateInterval)
	defer expireTicker.Stop()

	derpUpdateChan := make(chan struct{})
	go s.derpManager.StartUpdateLoop(ctx)

	haProbeChan := make(chan struct{})
	if s.cfg.Node.Routes.HA.ProbeInterval > 0 {
		haTicker := time.NewTicker(s.cfg.Node.Routes.HA.ProbeInterval)
		defer haTicker.Stop()
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-haTicker.C:
					haProbeChan <- struct{}{}
				}
			}
		}()
	}

	s.logger.Info("scheduled task worker started")

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scheduled task worker is shutting down")
			s.derpManager.Stop()
			if s.haProber != nil {
				s.haProber.Stop()
			}
			return

		case <-expireTicker.C:
			s.checkExpiredNodes()

		case <-derpUpdateChan:
			s.updateDERPMap(ctx)

		case <-haProbeChan:
			if s.haProber != nil {
				s.haProber.ProbeOnce(ctx, func(c types.Change) {
					s.Change(c)
				})
			}
		}
	}
}

func (s *HeadscaleServer) updateDERPMap(ctx context.Context) {
	if err := s.derpManager.UpdateNow(); err != nil {
		s.logger.Warnf("failed to update DERP map: %v", err)
		return
	}

	newMap := s.derpManager.GetDERPMap()
	oldMap := s.state.DERPMap()

	if newMap != nil && oldMap != nil && !derpMapsEqual(oldMap, newMap) {
		s.state.SetDERPMap(newMap)
		s.Change(types.Change{SendAllPeers: true})
		s.logger.Info("DERP map updated")
	}
}

func derpMapsEqual(a, b *tailcfg.DERPMap) bool {
	if a == nil || b == nil {
		return a == b
	}
	if len(a.Regions) != len(b.Regions) {
		return false
	}
	for id := range a.Regions {
		if _, ok := b.Regions[id]; !ok {
			return false
		}
	}
	return a.OmitDefaultRegions == b.OmitDefaultRegions
}

func (s *HeadscaleServer) checkExpiredNodes() {
	now := time.Now()
	changes := []types.Change{}

	nodes := s.state.ListNodes()
	for _, node := range nodes {
		if !node.IsExpired() {
			continue
		}

		expiryOpt := node.Expiry()
		var expiry time.Time
		if expiryOpt.Valid() {
			expiry = expiryOpt.Get()
		} else {
			continue
		}

		s.logger.Infof("Node %s (%d) expired at %v", node.Hostname, node.ID(), expiry)

		if expiry.Before(now.Add(-24 * time.Hour)) {
			change, err := s.state.DeleteNode(node)
			if err != nil {
				s.logger.Warnf("failed to delete expired node %d: %v", node.ID(), err)
				continue
			}
			changes = append(changes, change)
			s.logger.Infof("Deleted long-expired node %d", node.ID())
		} else {
			changes = append(changes, types.Change{
				TargetNode:    node.ID(),
				SendAllPeers:  true,
				IncludeSelf:   true,
			})
		}
	}

	if len(changes) > 0 {
		s.Change(changes...)
	}
}

func (s *HeadscaleServer) Change(cs ...types.Change) {
	for _, c := range cs {
		s.batcher.SendChange(c)
	}
}

func (s *HeadscaleServer) NoisePublicKey() key.MachinePublic {
	return s.noisePrivateKey.Public()
}

func (s *HeadscaleServer) GetState() *state.State {
	return s.state
}

func (s *HeadscaleServer) HTTPHandler() http.Handler {
	return s.httpServer
}

func readOrCreatePrivateKey(path string) (*key.MachinePrivate, error) {
	if path == "" {
		path = "/var/lib/headscale/noise_private.key"
	}

	dir := filepath.Dir(path)
	if err := util.EnsureDir(dir); err != nil {
		return nil, fmt.Errorf("ensuring private key directory: %w", err)
	}

	privateKey, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		machineKey := key.NewMachine()
		machineKeyStr, err := machineKey.MarshalText()
		if err != nil {
			return nil, fmt.Errorf("converting private key to string: %w", err)
		}

		err = os.WriteFile(path, machineKeyStr, privateKeyFileMode)
		if err != nil {
			return nil, fmt.Errorf("saving private key: %w", err)
		}

		return &machineKey, nil
	} else if err != nil {
		return nil, fmt.Errorf("reading private key file: %w", err)
	}

	trimmedPrivateKey := strings.TrimSpace(string(privateKey))
	var machineKey key.MachinePrivate
	if err = machineKey.UnmarshalText([]byte(trimmedPrivateKey)); err != nil {
		return nil, fmt.Errorf("parsing private key: %w", err)
	}

	return &machineKey, nil
}

func (s *HeadscaleServer) HTTPAuthenticationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")

		s.logger.Debugf("HTTP auth invoked: client=%s, path=%s", r.RemoteAddr, r.URL.Path)

		writeUnauthorized := func(statusCode int, msg string) {
			s.logger.Warnf("HTTP auth failed: client=%s, status=%d, reason=%s", r.RemoteAddr, statusCode, msg)
			w.WriteHeader(statusCode)
			_, _ = w.Write([]byte("Unauthorized: " + msg))
		}

		if authHeader == "" {
			writeUnauthorized(http.StatusUnauthorized, "missing Authorization header")
			return
		}

		if !strings.HasPrefix(authHeader, AuthPrefix) {
			writeUnauthorized(http.StatusUnauthorized, "missing Bearer prefix")
			return
		}

		token := strings.TrimSpace(strings.TrimPrefix(authHeader, AuthPrefix))
		if token == "" {
			writeUnauthorized(http.StatusUnauthorized, "empty token")
			return
		}

		valid, err := s.state.ValidateAPIKey(token)
		if err != nil {
			s.logger.Errorf("validating API key: %v", err)
			writeUnauthorized(http.StatusInternalServerError, "internal error")
			return
		}

		if !valid {
			writeUnauthorized(http.StatusUnauthorized, "invalid token")
			return
		}

		s.logger.Debugf("HTTP auth succeeded: client=%s", r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-XSS-Protection", "1; mode=block")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

func (s *HeadscaleServer) WaitForSignal() {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	)

	for sig := range sigc {
		switch sig {
		case syscall.SIGHUP:
			s.logger.Info("Received SIGHUP, reloading ACL policy")
			s.reloadPolicy()
		default:
			s.logger.Infof("Received signal %v, shutting down", sig)
			return
		}
	}
}

func (s *HeadscaleServer) reloadPolicy() {
	if s.cfg.Policy.Path == "" {
		s.logger.Debug("No policy file configured, skipping reload")
		return
	}

	policyData, err := os.ReadFile(s.cfg.Policy.Path)
	if err != nil {
		s.logger.Warnf("Failed to read policy file: %v", err)
		return
	}

	users, err := s.state.ListUsers()
	if err != nil {
		s.logger.Warnf("Failed to list users for policy reload: %v", err)
		return
	}

	nodes := s.state.ListNodes()
	nodeViews := make([]types.NodeView, len(nodes))
	for i, n := range nodes {
		nodeViews[i] = n
	}

	if err := s.state.SetPolicy(policyData, users, views.SliceOf(nodeViews)); err != nil {
		s.logger.Warnf("Failed to reload policy: %v", err)
		return
	}

	changes, err := s.state.ReloadPolicy()
	if err != nil {
		s.logger.Warnf("Failed to apply policy changes: %v", err)
		return
	}

	s.Change(changes...)
	s.logger.Infof("Policy reloaded successfully from %s", s.cfg.Policy.Path)
}

func NewHTTPServer(c *conf.Server, headscale *service.HeadscaleService, logger log.Logger) *kratoshttp.Server {
	var opts = []kratoshttp.ServerOption{
		kratoshttp.Middleware(
			recovery.Recovery(),
		),
	}
	if c.Http.Network != "" {
		opts = append(opts, kratoshttp.Network(c.Http.Network))
	}
	if c.Http.Addr != "" {
		opts = append(opts, kratoshttp.Address(c.Http.Addr))
	}
	if c.Http.Timeout != nil {
		opts = append(opts, kratoshttp.Timeout(c.Http.Timeout.AsDuration()))
	}

	corsHandler := handlers.CORS(
		handlers.AllowedOrigins([]string{"*"}),
		handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
		handlers.AllowedHeaders([]string{"Content-Type", "Authorization"}),
	)
	opts = append(opts, kratoshttp.Filter(corsHandler))

	srv := kratoshttp.NewServer(opts...)
	v1.RegisterHeadscaleServiceHTTPServer(srv, headscale)
	return srv
}
