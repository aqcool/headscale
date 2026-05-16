package mapper

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/juanfont/headscale-v2/internal/types"
	"github.com/puzpuzpuz/xsync/v4"
	"tailscale.com/tailcfg"
)

var errNoActiveConnections = errors.New("no active connections")
var errConnectionClosed = errors.New("connection closed")
var ErrConnectionSendTimeout = errors.New("connection send timeout")

type connectionEntry struct {
	id       string
	c        chan<- *tailcfg.MapResponse
	version  tailcfg.CapabilityVersion
	created  time.Time
	stop     func()
	lastUsed atomic.Int64
	closed   atomic.Bool
}

type multiChannelNodeConn struct {
	id     types.NodeID
	mapper *Mapper
	log    *log.Helper

	mutex       sync.RWMutex
	connections []*connectionEntry

	pendingMu sync.Mutex
	pending   []types.Change

	workMu sync.Mutex

	closeOnce   sync.Once
	updateCount atomic.Int64

	disconnectedAt atomic.Pointer[time.Time]
	lastSentPeers  *xsync.Map[tailcfg.NodeID, struct{}]
}

var connIDCounter atomic.Uint64

func generateConnectionID() string {
	return strconv.FormatUint(connIDCounter.Add(1), 10)
}

func newMultiChannelNodeConn(id types.NodeID, mapper *Mapper) *multiChannelNodeConn {
	return &multiChannelNodeConn{
		id:            id,
		mapper:        mapper,
		lastSentPeers: xsync.NewMap[tailcfg.NodeID, struct{}](),
		log:           log.NewHelper(log.With(log.DefaultLogger, "node_id", id.Uint64())),
	}
}

func (mc *multiChannelNodeConn) close() {
	mc.closeOnce.Do(func() {
		mc.mutex.Lock()
		defer mc.mutex.Unlock()

		for _, conn := range mc.connections {
			mc.stopConnection(conn)
		}
	})
}

func (mc *multiChannelNodeConn) stopConnection(conn *connectionEntry) {
	if conn.closed.CompareAndSwap(false, true) {
		if conn.stop != nil {
			conn.stop()
		}
	}
}

func (mc *multiChannelNodeConn) addConnection(entry *connectionEntry) {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()
	mc.connections = append(mc.connections, entry)
}

func (mc *multiChannelNodeConn) removeConnectionByChannel(c chan<- *tailcfg.MapResponse) bool {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	for i, entry := range mc.connections {
		if entry.c == c {
			copy(mc.connections[i:], mc.connections[i+1:])
			mc.connections[len(mc.connections)-1] = nil
			mc.connections = mc.connections[:len(mc.connections)-1]
			mc.stopConnection(entry)
			return true
		}
	}
	return false
}

func (mc *multiChannelNodeConn) hasActiveConnections() bool {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()
	return len(mc.connections) > 0
}

func (mc *multiChannelNodeConn) send(data *tailcfg.MapResponse) error {
	if data == nil {
		return nil
	}

	mc.mutex.RLock()
	if len(mc.connections) == 0 {
		mc.mutex.RUnlock()
		return errNoActiveConnections
	}

	snapshot := make([]*connectionEntry, len(mc.connections))
	copy(snapshot, mc.connections)
	mc.mutex.RUnlock()

	var lastErr error
	var successCount int
	var failed []*connectionEntry

	for _, conn := range snapshot {
		err := conn.send(data)
		if err != nil {
			lastErr = err
			failed = append(failed, conn)
		} else {
			successCount++
		}
	}

	if len(failed) > 0 {
		mc.mutex.Lock()
		failedSet := make(map[*connectionEntry]struct{}, len(failed))
		for _, f := range failed {
			failedSet[f] = struct{}{}
		}

		clean := mc.connections[:0]
		for _, conn := range mc.connections {
			if _, isFailed := failedSet[conn]; !isFailed {
				clean = append(clean, conn)
			} else {
				mc.stopConnection(conn)
			}
		}
		mc.connections = clean
		mc.mutex.Unlock()
	}

	mc.updateCount.Add(1)

	if successCount > 0 {
		return nil
	}
	return fmt.Errorf("node %d: all connections failed: %w", mc.id, lastErr)
}

func (entry *connectionEntry) send(data *tailcfg.MapResponse) error {
	if data == nil {
		return nil
	}

	if entry.closed.Load() {
		return fmt.Errorf("connection %s: %w", entry.id, errConnectionClosed)
	}

	timer := time.NewTimer(50 * time.Millisecond)
	defer timer.Stop()

	select {
	case entry.c <- data:
		entry.lastUsed.Store(time.Now().Unix())
		return nil
	case <-timer.C:
		return fmt.Errorf("connection %s: %w", entry.id, ErrConnectionSendTimeout)
	}
}

func (mc *multiChannelNodeConn) nodeID() types.NodeID {
	return mc.id
}

func (mc *multiChannelNodeConn) version() tailcfg.CapabilityVersion {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	if len(mc.connections) == 0 {
		return 0
	}
	return mc.connections[0].version
}

func (mc *multiChannelNodeConn) updateSentPeers(resp *tailcfg.MapResponse) {
	if resp == nil {
		return
	}

	if resp.Peers != nil {
		mc.lastSentPeers.Clear()
		for _, peer := range resp.Peers {
			mc.lastSentPeers.Store(peer.ID, struct{}{})
		}
	}

	for _, peer := range resp.PeersChanged {
		mc.lastSentPeers.Store(peer.ID, struct{}{})
	}

	for _, id := range resp.PeersRemoved {
		mc.lastSentPeers.Delete(id)
	}
}

func (mc *multiChannelNodeConn) computePeerDiff(currentPeers []tailcfg.NodeID) []tailcfg.NodeID {
	currentSet := make(map[tailcfg.NodeID]struct{}, len(currentPeers))
	for _, id := range currentPeers {
		currentSet[id] = struct{}{}
	}

	var removed []tailcfg.NodeID
	mc.lastSentPeers.Range(func(id tailcfg.NodeID, _ struct{}) bool {
		if _, exists := currentSet[id]; !exists {
			removed = append(removed, id)
		}
		return true
	})

	return removed
}

func (mc *multiChannelNodeConn) appendPending(changes ...types.Change) {
	mc.pendingMu.Lock()
	mc.pending = append(mc.pending, changes...)
	mc.pendingMu.Unlock()
}

func (mc *multiChannelNodeConn) drainPending() []types.Change {
	mc.pendingMu.Lock()
	p := mc.pending
	mc.pending = nil
	mc.pendingMu.Unlock()
	return p
}

func (mc *multiChannelNodeConn) markDisconnected() {
	now := time.Now()
	mc.disconnectedAt.Store(&now)
}

func (mc *multiChannelNodeConn) markConnected() {
	mc.disconnectedAt.Store(nil)
}