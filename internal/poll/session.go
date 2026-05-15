package poll

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/juanfont/headscale-v2/internal/state"
	"github.com/juanfont/headscale-v2/internal/types"
	"tailscale.com/tailcfg"
	"tailscale.com/util/zstdframe"
)

const (
	keepAliveInterval = 50 * time.Second
	keepAliveJitter   = 10 * time.Second
	reservedResponseHeader = 4
)

type contextKey string

const nodeNameContextKey = contextKey("nodeName")

// MapSession handles a long-polling session with a node
type MapSession struct {
	state       *state.State
	req         tailcfg.MapRequest
	ctx         context.Context
	capVer      tailcfg.CapabilityVersion

	ch             chan *tailcfg.MapResponse
	cancelCh       chan struct{}
	cancelChClosed atomic.Bool

	keepAlive       time.Duration
	keepAliveTicker *time.Ticker

	node types.NodeView
	w    http.ResponseWriter

	logger *log.Helper
}

// NewMapSession creates a new map session
func NewMapSession(
	ctx context.Context,
	st *state.State,
	req tailcfg.MapRequest,
	w http.ResponseWriter,
	node types.NodeView,
	logger log.Logger,
) *MapSession {
	// Add jitter to keepalive (0-9 seconds)
	ka := keepAliveInterval + time.Duration(rand.Intn(9000))*time.Millisecond

	return &MapSession{
		state:     st,
		ctx:       ctx,
		req:       req,
		w:         w,
		node:      node,
		capVer:    req.Version,
		ch:        make(chan *tailcfg.MapResponse, 100),
		cancelCh:  make(chan struct{}),
		keepAlive: ka,
		logger:    log.NewHelper(logger),
	}
}

func (m *MapSession) isStreaming() bool {
	return m.req.Stream
}

func (m *MapSession) isEndpointUpdate() bool {
	return !m.req.Stream && m.req.OmitPeers
}

func (m *MapSession) resetKeepAlive() {
	if m.keepAliveTicker != nil {
		m.keepAliveTicker.Reset(m.keepAlive)
	}
}

func (m *MapSession) stopFromBatcher() {
	if m.cancelChClosed.CompareAndSwap(false, true) {
		close(m.cancelCh)
	}
}

// serve handles non-streaming requests
func (m *MapSession) serve() {
	// Process the MapRequest to update node state
	_, err := m.state.UpdateNodeFromMapRequest(m.node.ID(), m.req)
	if err != nil {
		m.logger.Errorf("failed to update node from map request: %v", err)
		http.Error(m.w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Endpoint update - just return 200
	if m.isEndpointUpdate() {
		m.w.WriteHeader(http.StatusOK)
		return
	}
}

// serveLongPoll handles streaming long-poll requests
func (m *MapSession) serveLongPoll() {
	m.logger.Debugf("long poll session started for node %d", m.node.ID())

	defer func() {
		m.stopFromBatcher()
		m.logger.Infof("node %d disconnected", m.node.ID())
	}()

	m.keepAliveTicker = time.NewTicker(m.keepAlive)
	defer m.keepAliveTicker.Stop()

	_, err := m.state.UpdateNodeFromMapRequest(m.node.ID(), m.req)
	if err != nil {
		m.logger.Errorf("failed to update node from initial MapRequest: %v", err)
		return
	}

	connectChanges, _ := m.state.Connect(m.node.ID())
	m.logger.Infof("node %d connected", m.node.ID())

	for _, c := range connectChanges {
		resp := m.state.BuildMapResponse(m.node, c)
		if resp != nil {
			if err := m.writeMap(resp); err != nil {
				m.logger.Errorf("cannot write initial update to node %d: %v", m.node.ID(), err)
				return
			}
		}
	}

	for {
		select {
		case <-m.cancelCh:
			m.logger.Debugf("poll cancelled for node %d", m.node.ID())
			return

		case <-m.ctx.Done():
			m.logger.Debugf("poll context done for node %d", m.node.ID())
			return

		case update, ok := <-m.ch:
			if !ok {
				m.logger.Debugf("update channel closed for node %d", m.node.ID())
				return
			}

			err := m.writeMap(update)
			if err != nil {
				m.logger.Errorf("cannot write update to node %d: %v", m.node.ID(), err)
				return
			}

			m.logger.Debugf("update sent to node %d", m.node.ID())
			m.resetKeepAlive()

		case <-m.keepAliveTicker.C:
			err := m.writeMap(&keepAlive)
			if err != nil {
				m.logger.Errorf("cannot write keep alive to node %d: %v", m.node.ID(), err)
				return
			}
			m.resetKeepAlive()
		}
	}
}

// writeMap writes a MapResponse to the client
func (m *MapSession) writeMap(msg *tailcfg.MapResponse) error {
	jsonBody, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshalling map response: %w", err)
	}

	// Compress if requested
	if m.req.Compress == "zstd" {
		jsonBody = zstdframe.AppendEncode(nil, jsonBody, zstdframe.FastestCompression)
	}

	// Prepend length header
	data := make([]byte, reservedResponseHeader, reservedResponseHeader+len(jsonBody))
	binary.LittleEndian.PutUint32(data, uint32(len(jsonBody)))
	data = append(data, jsonBody...)

	_, err = m.w.Write(data)
	if err != nil {
		return err
	}

	// Flush for streaming
	if m.isStreaming() {
		if f, ok := m.w.(http.Flusher); ok {
			f.Flush()
		}
	}

	return nil
}

// SendUpdate queues an update to be sent to the client
func (m *MapSession) SendUpdate(resp *tailcfg.MapResponse) {
	select {
	case m.ch <- resp:
	default:
		m.logger.Warnf("MapSession channel full, dropping update for node %d", m.node.ID())
	}
}

// Stop stops the session
func (m *MapSession) Stop() {
	m.stopFromBatcher()
}

var keepAlive = tailcfg.MapResponse{
	KeepAlive: true,
}

// Batcher batches updates and distributes them to connected sessions
type Batcher struct {
	mu       sync.RWMutex
	sessions map[types.NodeID]*MapSession
	ch       chan batchItem
	logger   *log.Helper
}

type batchItem struct {
	nodeID types.NodeID
	resp   *tailcfg.MapResponse
}

// NewBatcher creates a new batcher
func NewBatcher(logger log.Logger) *Batcher {
	return &Batcher{
		sessions: make(map[types.NodeID]*MapSession),
		ch:       make(chan batchItem, 1000),
		logger:   log.NewHelper(logger),
	}
}

// AddSession adds a session to the batcher
func (b *Batcher) AddSession(session *MapSession) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sessions[session.node.ID()] = session
}

// RemoveSession removes a session from the batcher
func (b *Batcher) RemoveSession(nodeID types.NodeID) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.sessions, nodeID)
}

// GetSession gets a session by node ID
func (b *Batcher) GetSession(nodeID types.NodeID) (*MapSession, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	session, ok := b.sessions[nodeID]
	return session, ok
}

// Send queues an update for a specific node
func (b *Batcher) Send(nodeID types.NodeID, resp *tailcfg.MapResponse) {
	select {
	case b.ch <- batchItem{nodeID: nodeID, resp: resp}:
	default:
		b.logger.Warnf("Batcher channel full, dropping update for node %d", nodeID)
	}
}

// Broadcast sends an update to all connected nodes
func (b *Batcher) Broadcast(resp *tailcfg.MapResponse) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for nodeID := range b.sessions {
		b.Send(nodeID, resp)
	}
}

// SendChange handles a change notification by building and sending MapResponses
func (b *Batcher) SendChange(c types.Change) {
	b.logger.Debugf("SendChange: %+v", c)

	var targetSession *MapSession
	var ok bool

	b.mu.RLock()
	if c.TargetNode != 0 {
		targetSession, ok = b.sessions[c.TargetNode]
	}
	b.mu.RUnlock()

	if c.SendAllPeers {
		b.mu.RLock()
		for nodeID, session := range b.sessions {
			if c.IncludeSelf || nodeID != c.TargetNode {
				resp := session.state.BuildMapResponse(session.node, c)
				if resp != nil {
					session.SendUpdate(resp)
				}
			}
		}
		b.mu.RUnlock()
		return
	}

	if targetSession != nil && ok {
		resp := targetSession.state.BuildMapResponse(targetSession.node, c)
		if resp != nil {
			targetSession.SendUpdate(resp)
		}
	}
}

// Run runs the batcher loop
func (b *Batcher) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case item := <-b.ch:
			session, ok := b.GetSession(item.nodeID)
			if !ok {
				continue
			}
			session.SendUpdate(item.resp)
		}
	}
}
