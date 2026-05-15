package state

import (
	"sync"
	"time"

	"github.com/juanfont/headscale-v2/internal/types"
	"github.com/juanfont/headscale-v2/internal/util"
)

const pingIDLength = 16

type pingTracker struct {
	mu      sync.Mutex
	pending map[string]*pendingPing
}

type pendingPing struct {
	nodeID     types.NodeID
	startTime  time.Time
	responseCh chan time.Duration
}

func newPingTracker() *pingTracker {
	return &pingTracker{
		pending: make(map[string]*pendingPing),
	}
}

func (pt *pingTracker) register(nodeID types.NodeID) (string, <-chan time.Duration) {
	pingID, _ := util.GenerateRandomStringDNSSafe(pingIDLength)
	ch := make(chan time.Duration, 1)

	pt.mu.Lock()
	pt.pending[pingID] = &pendingPing{
		nodeID:    nodeID,
		startTime: time.Now(),
		responseCh: ch,
	}
	pt.mu.Unlock()

	return pingID, ch
}

func (pt *pingTracker) complete(pingID string) bool {
	pt.mu.Lock()
	pp, ok := pt.pending[pingID]
	if ok {
		delete(pt.pending, pingID)
	}
	pt.mu.Unlock()

	if ok {
		pp.responseCh <- time.Since(pp.startTime)
		close(pp.responseCh)
		return true
	}
	return false
}

func (pt *pingTracker) cancel(pingID string) {
	pt.mu.Lock()
	delete(pt.pending, pingID)
	pt.mu.Unlock()
}

func (pt *pingTracker) drain() {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	for id, pp := range pt.pending {
		close(pp.responseCh)
		delete(pt.pending, id)
	}
}