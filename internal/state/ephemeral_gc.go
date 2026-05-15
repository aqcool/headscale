package state

import (
	"context"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/juanfont/headscale-v2/internal/types"
)

type EphemeralGarbageCollector struct {
	mu          sync.Mutex
	scheduled   map[types.NodeID]*scheduledDeletion
	state       *State
	timeout     time.Duration
	logger      *log.Helper
	cancel      context.CancelFunc
	running     bool
}

type scheduledDeletion struct {
	timer     *time.Timer
	nodeID    types.NodeID
}

func NewEphemeralGarbageCollector(
	st *State,
	timeout time.Duration,
	logger log.Logger,
) *EphemeralGarbageCollector {
	return &EphemeralGarbageCollector{
		scheduled: make(map[types.NodeID]*scheduledDeletion),
		state:     st,
		timeout:   timeout,
		logger:    log.NewHelper(logger),
	}
}

func (gc *EphemeralGarbageCollector) Start(ctx context.Context) {
	gc.mu.Lock()
	if gc.running {
		gc.mu.Unlock()
		return
	}
	gc.running = true
	gc.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	gc.cancel = cancel

	gc.logger.Info("ephemeral garbage collector started")

	for {
		select {
		case <-ctx.Done():
			gc.logger.Info("ephemeral garbage collector stopped")
			gc.Stop()
			return
		}
	}
}

func (gc *EphemeralGarbageCollector) Stop() {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	if gc.cancel != nil {
		gc.cancel()
	}

	for nodeID, deletion := range gc.scheduled {
		if deletion.timer != nil {
			deletion.timer.Stop()
		}
		gc.scheduled[nodeID] = nil
		delete(gc.scheduled, nodeID)
	}

	gc.running = false
	gc.logger.Info("ephemeral garbage collector stopped, all timers cancelled")
}

func (gc *EphemeralGarbageCollector) Schedule(node types.NodeView) {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	if !node.IsEphemeral() {
		return
	}

	nodeID := node.ID()

	if existing, exists := gc.scheduled[nodeID]; exists {
		if existing.timer != nil {
			existing.timer.Stop()
		}
	}

	timer := time.AfterFunc(gc.timeout, func() {
		gc.deleteNode(nodeID)
	})

	gc.scheduled[nodeID] = &scheduledDeletion{
		timer:  timer,
		nodeID: nodeID,
	}

	gc.logger.Debugf("scheduled ephemeral node %d for deletion after %s", nodeID, gc.timeout)
}

func (gc *EphemeralGarbageCollector) CancelSchedule(nodeID types.NodeID) {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	if deletion, exists := gc.scheduled[nodeID]; exists {
		if deletion.timer != nil {
			deletion.timer.Stop()
		}
		delete(gc.scheduled, nodeID)
		gc.logger.Debugf("cancelled scheduled deletion for ephemeral node %d", nodeID)
	}
}

func (gc *EphemeralGarbageCollector) IsScheduled(nodeID types.NodeID) bool {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	return gc.scheduled[nodeID] != nil
}

func (gc *EphemeralGarbageCollector) deleteNode(nodeID types.NodeID) {
	gc.mu.Lock()
	delete(gc.scheduled, nodeID)
	gc.mu.Unlock()

	node, ok := gc.state.nodeStore.GetNode(nodeID)
	if !ok {
		gc.logger.Debugf("ephemeral node %d no longer exists", nodeID)
		return
	}

	if !node.IsEphemeral() {
		gc.logger.Debugf("node %d is no longer ephemeral, skipping deletion", nodeID)
		return
	}

	isOnline := node.IsOnline()
	if isOnline.Valid() && isOnline.Get() {
		gc.logger.Debugf("ephemeral node %d is online, skipping deletion", nodeID)
		gc.Schedule(node)
		return
	}

	gc.logger.Infof("deleting ephemeral node %d due to inactivity", nodeID)

	change, err := gc.state.DeleteNode(node)
	if err != nil {
		gc.logger.Errorf("failed to delete ephemeral node %d: %v", nodeID, err)
		return
	}

	gc.logger.Infof("ephemeral node %d deleted", nodeID)
	_ = change
}

func (gc *EphemeralGarbageCollector) ScheduledCount() int {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	return len(gc.scheduled)
}

func (gc *EphemeralGarbageCollector) ProcessNodes(nodes []types.NodeView) {
	for _, node := range nodes {
		if node.IsEphemeral() {
			isOnline := node.IsOnline()
			if !isOnline.Valid() || !isOnline.Get() {
				gc.Schedule(node)
			} else {
				gc.CancelSchedule(node.ID())
			}
		}
	}
}