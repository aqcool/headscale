package state

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/juanfont/headscale-v2/internal/types"
)

type HAHealthProber struct {
	mu          sync.Mutex
	state       *State
	probeInterval time.Duration
	probeTimeout  time.Duration
	logger      *log.Helper
	cancel      context.CancelFunc
	running     bool
}

func NewHAHealthProber(
	st *State,
	probeInterval, probeTimeout time.Duration,
	logger log.Logger,
) *HAHealthProber {
	return &HAHealthProber{
		state:         st,
		probeInterval: probeInterval,
		probeTimeout:  probeTimeout,
		logger:        log.NewHelper(logger),
	}
}

func (p *HAHealthProber) Start(ctx context.Context) {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	p.running = true
	p.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	p.cancel = cancel

	if p.probeInterval <= 0 {
		p.logger.Info("HA health probing disabled (probe_interval <= 0)")
		return
	}

	p.logger.Infof("HA health prober started, interval: %v, timeout: %v", p.probeInterval, p.probeTimeout)

	ticker := time.NewTicker(p.probeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("HA health prober stopped")
			return
		case <-ticker.C:
			p.ProbeOnce(ctx, nil)
		}
	}
}

func (p *HAHealthProber) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cancel != nil {
		p.cancel()
	}
	p.running = false
}

func (p *HAHealthProber) ProbeOnce(ctx context.Context, changeFn func(types.Change)) {
	nodes := p.state.ListNodes()

	for _, node := range nodes {
		if !node.IsSubnetRouter() {
			continue
		}

		healthy := p.probeNode(ctx, node)

		p.mu.Lock()
		isUnhealthy := false
		p.state.UpdateNode(node.ID(), func(n *types.Node) {
			n.Unhealthy = !healthy
			isUnhealthy = n.Unhealthy
		})
		p.mu.Unlock()

		if healthy && isUnhealthy {
			p.logger.Infof("HA: node %d (%s) became healthy", node.ID(), node.Hostname())
			if changeFn != nil {
				changeFn(types.NodeAdded(node.ID()))
			}
		} else if !healthy && !isUnhealthy {
			p.logger.Warnf("HA: node %d (%s) became unhealthy", node.ID(), node.Hostname())
			if changeFn != nil {
				changeFn(types.NodeAdded(node.ID()))
			}
		}
	}
}

func (p *HAHealthProber) probeNode(ctx context.Context, node types.NodeView) bool {
	routes := node.SubnetRoutes()
	if len(routes) == 0 {
		return true
	}

	probeTimeout := p.probeTimeout
	if probeTimeout <= 0 {
		probeTimeout = 5 * time.Second
	}

	client := &http.Client{
		Timeout: probeTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodHead, fmt.Sprintf("http://100.100.100.100:%d/health", 80), nil)
	if err != nil {
		p.logger.Debugf("HA: failed to create probe request for node %d: %v", node.ID(), err)
		return true
	}

	resp, err := client.Do(req)
	if err != nil {
		p.logger.Debugf("HA: probe failed for node %d: %v", node.ID(), err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode < 500
}

func (p *HAHealthProber) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}