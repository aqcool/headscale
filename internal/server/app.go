package server

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"time"

	"github.com/juanfont/headscale-v2/internal/derp"
	"github.com/juanfont/headscale-v2/internal/ip"
	"github.com/juanfont/headscale-v2/internal/mapper"
	"github.com/juanfont/headscale-v2/internal/noise"
	"github.com/juanfont/headscale-v2/internal/policy"
	"github.com/juanfont/headscale-v2/internal/poll"
	"github.com/juanfont/headscale-v2/internal/state"
	"github.com/juanfont/headscale-v2/internal/types"

	"github.com/go-kratos/kratos/v2/log"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/gorilla/handlers"
	"tailscale.com/types/key"
	"tailscale.com/types/views"
)

type Headscale struct {
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

	haProber    *state.HAHealthProber
	ephemeralGC *state.EphemeralGarbageCollector
}

func NewHeadscale(
	cfg *types.Config,
	logger log.Logger,
) (*Headscale, error) {
	helper := log.NewHelper(logger)

	privateKey, err := readOrCreatePrivateKey(cfg.NoisePrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("reading or creating Noise protocol private key: %w", err)
	}

	prefix4 := cfg.PrefixV4
	prefix6 := cfg.PrefixV6
	if prefix4 == nil {
		p := netip.MustParsePrefix("100.64.0.0/10")
		prefix4 = &p
	}
	if prefix6 == nil {
		p := netip.MustParsePrefix("fd7a:115c:a1e0::/48")
		prefix6 = &p
	}

	ipAlloc := ip.NewIPAllocator(prefix4, prefix6, cfg.IPAllocation)

	peersFunc := func(nodes []types.NodeView) map[types.NodeID][]types.NodeView {
		return buildPeersMap(nodes)
	}

	stateCfg := &state.StateConfig{
		IPAlloc:      ipAlloc,
		BatchSize:    cfg.Tuning.NodeStoreBatchSize,
		BatchTimeout: cfg.Tuning.NodeStoreBatchTimeout,
		PeersFunc:    peersFunc,
	}
	st := state.NewState(stateCfg, logger)

	st.SetIPAllocator(ipAlloc)

	derpMan := derp.NewDERPManager(&cfg.DERP, logger)
	st.SetDERPMap(derpMan.GetDERPMap())

	batcher := poll.NewBatcher(logger)
	go batcher.Run(context.Background())

	mapr := mapper.NewMapper(st, cfg, logger)

	polMan, err := policy.NewPolicyManager(nil, []types.User{}, views.SliceOf([]types.NodeView{}))
	if err != nil {
		return nil, fmt.Errorf("creating policy manager: %w", err)
	}
	st.SetPolicyManager(polMan)

	noiseCfg := &noise.Config{
		ServerURL:  cfg.ServerURL,
		ListenAddr: cfg.Addr,
		PrivateKey: *privateKey,
	}
	noiseSrv := noise.NewNoiseServer(noiseCfg, st, logger)

	var opts []kratoshttp.ServerOption
	if cfg.Addr != "" {
		opts = append(opts, kratoshttp.Address(cfg.Addr))
	}
	opts = append(opts, kratoshttp.Middleware())

	corsHandler := handlers.CORS(
		handlers.AllowedOrigins([]string{"*"}),
		handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
		handlers.AllowedHeaders([]string{"Content-Type", "Authorization"}),
	)
	opts = append(opts, kratoshttp.Filter(corsHandler))

	httpSrv := kratoshttp.NewServer(opts...)

	registerOIDCHandlers(httpSrv, cfg, st, helper)

	registerNoiseHandlers(httpSrv, noiseSrv)

	haProber := state.NewHAHealthProber(st, cfg.Node.Routes.HA.ProbeInterval, cfg.Node.Routes.HA.ProbeTimeout, logger)
	ephemeralGC := state.NewEphemeralGarbageCollector(st, cfg.Node.Ephemeral.InactivityTimeout, logger)

	app := &Headscale{
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
		haProber:        haProber,
		ephemeralGC:     ephemeralGC,
	}

	app.state.SetNotifier(app.notifyChange)

	return app, nil
}

func buildPeersMap(nodes []types.NodeView) map[types.NodeID][]types.NodeView {
	result := make(map[types.NodeID][]types.NodeView, len(nodes))
	for _, node := range nodes {
		var peers []types.NodeView
		for _, n := range nodes {
			if n.ID() != node.ID() {
				peers = append(peers, n)
			}
		}
		result[node.ID()] = peers
	}
	return result
}

func (h *Headscale) notifyChange(c types.Change) {
	h.batcher.SendChange(c)
}

func (h *Headscale) HTTPServer() *kratoshttp.Server {
	return h.httpServer
}

func (h *Headscale) GetState() *state.State {
	return h.state
}

func (h *Headscale) NoisePublicKey() key.MachinePublic {
	return h.noisePrivateKey.Public()
}

func (h *Headscale) HTTPHandler() http.Handler {
	return h.httpServer
}

func (h *Headscale) Start(ctx context.Context) error {
	go func() {
		if err := h.noiseServer.Start(ctx); err != nil {
			h.logger.Errorf("Noise server error: %v", err)
		}
	}()

	go h.scheduledTasks(ctx)

	go h.haProber.Start(ctx)

	go h.ephemeralGC.Start(ctx)

	h.logger.Info("Headscale server started")
	return h.httpServer.Start(ctx)
}

func (h *Headscale) Stop(ctx context.Context) error {
	h.state.Stop()
	h.haProber.Stop()
	h.ephemeralGC.Stop()
	h.derpManager.Stop()
	return h.httpServer.Stop(ctx)
}

func (h *Headscale) scheduledTasks(ctx context.Context) {
	expireTicker := time.NewTicker(updateInterval)
	defer expireTicker.Stop()

	h.logger.Info("scheduled task worker started")

	for {
		select {
		case <-ctx.Done():
			h.logger.Info("scheduled task worker is shutting down")
			return

		case <-expireTicker.C:
			h.checkExpiredNodes()
		}
	}
}

func (h *Headscale) checkExpiredNodes() {
	now := time.Now()
	nodes := h.state.ListNodes()
	for _, node := range nodes {
		if !node.IsExpired() {
			continue
		}
		expiryOpt := node.Expiry()
		if !expiryOpt.Valid() {
			continue
		}
		expiry := expiryOpt.Get()

		h.logger.Infof("Node %s (%d) expired at %v", node.Hostname(), node.ID(), expiry)

		if expiry.Before(now.Add(-24 * time.Hour)) {
			change, err := h.state.DeleteNode(node)
			if err != nil {
				h.logger.Warnf("failed to delete expired node %d: %v", node.ID(), err)
				continue
			}
			h.notifyChange(change)
			h.logger.Infof("Deleted long-expired node %d", node.ID())
		} else {
			h.notifyChange(types.Change{
				TargetNode:   node.ID(),
				SendAllPeers: true,
				IncludeSelf:  true,
			})
		}
	}
}

func loadPolicyFromFile(path string) ([]byte, error) {
	return nil, fmt.Errorf("policy loading not implemented")
}

func registerOIDCHandlers(srv *kratoshttp.Server, cfg *types.Config, st *state.State, logger *log.Helper) {
	if cfg.OIDC.Issuer == "" {
		return
	}
	logger.Info("OIDC handlers registered")
}

func registerNoiseHandlers(srv *kratoshttp.Server, noiseSrv *noise.NoiseServer) {

}