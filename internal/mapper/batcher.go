package mapper

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juanfont/headscale-v2/internal/types"
	"github.com/puzpuzpuz/xsync/v4"
	"tailscale.com/tailcfg"
)

var (
	ErrInitialMapSendTimeout = errors.New("sending initial map: timeout")
	ErrBatcherShuttingDown   = errors.New("batcher shutting down")
)

type work struct {
	changes  []types.Change
	nodeID   types.NodeID
	resultCh chan<- workResult
}

type workResult struct {
	mapResponse *tailcfg.MapResponse
	err         error
}

type Batcher struct {
	tick    *time.Ticker
	mapper  *Mapper
	workers int

	nodes *xsync.Map[types.NodeID, *multiChannelNodeConn]

	workCh   chan work
	done     chan struct{}
	doneOnce sync.Once

	wg sync.WaitGroup

	started atomic.Bool

	totalNodes      atomic.Int64
	workQueuedCount atomic.Int64
	workProcessed   atomic.Int64
}

func NewBatcher(batchTime time.Duration, workers int, mapper *Mapper) *Batcher {
	return &Batcher{
		mapper:  mapper,
		workers: workers,
		tick:    time.NewTicker(batchTime),
		workCh:  make(chan work, workers*200),
		done:    make(chan struct{}),
		nodes:   xsync.NewMap[types.NodeID, *multiChannelNodeConn](),
	}
}

func (b *Batcher) Start() {
	if b.started.CompareAndSwap(false, true) {
		b.wg.Add(1)
		go b.run()
	}
}

func (b *Batcher) Close() {
	b.doneOnce.Do(func() {
		close(b.done)
	})
	b.wg.Wait()
	b.tick.Stop()

	b.nodes.Range(func(id types.NodeID, nc *multiChannelNodeConn) bool {
		nc.close()
		return true
	})
}

func (b *Batcher) run() {
	defer b.wg.Done()

	for {
		select {
		case <-b.done:
			return
		case <-b.tick.C:
			b.processBatch()
		}
	}
}

func (b *Batcher) processBatch() {
	b.nodes.Range(func(id types.NodeID, nc *multiChannelNodeConn) bool {
		if !nc.hasActiveConnections() {
			return true
		}

		pending := nc.drainPending()
		if len(pending) == 0 {
			return true
		}

		b.workCh <- work{
			changes: pending,
			nodeID:  id,
		}
		return true
	})
}

func (b *Batcher) AddNode(
	id types.NodeID,
	c chan<- *tailcfg.MapResponse,
	version tailcfg.CapabilityVersion,
	stop func(),
) error {
	connID := generateConnectionID()
	now := time.Now()

	newEntry := &connectionEntry{
		id:      connID,
		c:       c,
		version: version,
		created: now,
		stop:    stop,
	}
	newEntry.lastUsed.Store(now.Unix())

	nodeConn, loaded := b.nodes.LoadOrStore(id, newMultiChannelNodeConn(id, b.mapper))
	if !loaded {
		b.totalNodes.Add(1)
	}

	nodeConn.addConnection(newEntry)

	// Send initial map response
	initialMap, err := b.mapper.BuildMapResponse(nil, id, tailcfg.MapRequest{Version: version})
	if err != nil {
		nodeConn.removeConnectionByChannel(c)
		return err
	}

	if err := nodeConn.send(initialMap); err != nil {
		nodeConn.removeConnectionByChannel(c)
		return err
	}

	nodeConn.updateSentPeers(initialMap)

	// Notify peers of new node
	b.notifyPeersOfChange(id)

	return nil
}

func (b *Batcher) RemoveNode(id types.NodeID, c chan<- *tailcfg.MapResponse) {
	nodeConn, ok := b.nodes.Load(id)
	if !ok {
		return
	}

	nodeConn.removeConnectionByChannel(c)

	if !nodeConn.hasActiveConnections() {
		nodeConn.markDisconnected()
		b.notifyPeersOfChange(id)
	}
}

func (b *Batcher) notifyPeersOfChange(changedNodeID types.NodeID) {
	b.nodes.Range(func(id types.NodeID, nc *multiChannelNodeConn) bool {
		if id == changedNodeID {
			return true
		}

		nc.appendPending(types.Change{OriginNode: changedNodeID})
		return true
	})
}

func (b *Batcher) SendChange(nodeID types.NodeID, change types.Change) {
	nodeConn, ok := b.nodes.Load(nodeID)
	if !ok {
		return
	}

	nodeConn.appendPending(change)
}

func (b *Batcher) Broadcast(change types.Change) {
	b.nodes.Range(func(id types.NodeID, nc *multiChannelNodeConn) bool {
		nc.appendPending(change)
		return true
	})
}

func (b *Batcher) MapResponseFromChange(nodeID types.NodeID, change types.Change) (*tailcfg.MapResponse, error) {
	nodeConn, ok := b.nodes.Load(nodeID)
	if !ok {
		return nil, ErrNodeNotFound
	}

	return b.mapper.BuildMapResponse(nil, nodeID, tailcfg.MapRequest{Version: nodeConn.version()})
}

func (b *Batcher) TotalNodes() int64 {
	return b.totalNodes.Load()
}

func (b *Batcher) WorkProcessed() int64 {
	return b.workProcessed.Load()
}