package server

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/juanfont/headscale-v2/internal/types"
	"tailscale.com/tailcfg"
	"tailscale.com/util/zstdframe"
)

const keepAliveInterval = 50 * time.Second

type contextKey string

const nodeNameContextKey = contextKey("nodeName")

// mapSession handles long-polling MapRequest sessions.
type mapSession struct {
	h      *Headscale
	req    tailcfg.MapRequest
	ctx    context.Context
	capVer tailcfg.CapabilityVersion

	ch             chan *tailcfg.MapResponse
	cancelCh       chan struct{}
	cancelChClosed atomic.Bool

	keepAlive       time.Duration
	keepAliveTicker *time.Ticker

	node *types.Node
	w    http.ResponseWriter

	logger *log.Helper
}

func (h *Headscale) newMapSession(
	ctx context.Context,
	req tailcfg.MapRequest,
	w http.ResponseWriter,
	node *types.Node,
) *mapSession {
	ka := keepAliveInterval + (time.Duration(rand.IntN(9000)) * time.Millisecond)

	return &mapSession{
		h:      h,
		ctx:    ctx,
		req:    req,
		w:      w,
		node:   node,
		capVer: req.Version,

		ch:       make(chan *tailcfg.MapResponse, 1),
		cancelCh: make(chan struct{}),

		keepAlive:       ka,
		keepAliveTicker: nil,

		logger: h.logger,
	}
}

func (m *mapSession) isStreaming() bool {
	return m.req.Stream
}

func (m *mapSession) isEndpointUpdate() bool {
	return !m.req.Stream && m.req.OmitPeers
}

func (m *mapSession) resetKeepAlive() {
	if m.keepAliveTicker != nil {
		m.keepAliveTicker.Reset(m.keepAlive)
	}
}

func (m *mapSession) stopFromBatcher() {
	if m.cancelChClosed.CompareAndSwap(false, true) {
		close(m.cancelCh)
	}
}

func (m *mapSession) beforeServeLongPoll() {
	// Cancel ephemeral GC if applicable
}

func (m *mapSession) afterServeLongPoll() {
	// Schedule ephemeral GC if applicable
}

// serve handles non-streaming requests.
func (m *mapSession) serve() {
	// Update node from MapRequest
	_, err := m.h.state.UpdateNodeFromMapRequest(m.node.ID, m.req)
	if err != nil {
		httpError(m.w, err)
		return
	}

	if m.isEndpointUpdate() {
		m.w.WriteHeader(http.StatusOK)
	}
}

// serveLongPoll handles streaming MapRequest sessions.
func (m *mapSession) serveLongPoll() {
	m.beforeServeLongPoll()

	m.logger.Debug("long poll session started")

	defer func() {
		m.stopFromBatcher()
		m.logger.Debug("long poll session ended")
		m.afterServeLongPoll()
	}()

	ctx, cancel := context.WithCancel(m.ctx)
	defer cancel()

	m.keepAliveTicker = time.NewTicker(m.keepAlive)
	defer m.keepAliveTicker.Stop()

	// Update node state
	_, err := m.h.state.UpdateNodeFromMapRequest(m.node.ID, m.req)
	if err != nil {
		m.logger.Errorf("failed to update node from MapRequest: %v", err)
		return
	}

	// Loop through updates
	for {
		select {
		case <-m.cancelCh:
			m.logger.Debug("poll cancelled")
			return

		case <-ctx.Done():
			m.logger.Debug("poll context done")
			return

		case update, ok := <-m.ch:
			if !ok {
				m.logger.Debug("update channel closed")
				return
			}

			err := m.writeMap(update)
			if err != nil {
				m.logger.Errorf("cannot write update: %v", err)
				return
			}

			m.resetKeepAlive()

		case <-m.keepAliveTicker.C:
			err := m.writeMap(&keepAlive)
			if err != nil {
				m.logger.Errorf("cannot write keepalive: %v", err)
				return
			}

			m.resetKeepAlive()
		}
	}
}

func (m *mapSession) writeMap(msg *tailcfg.MapResponse) error {
	jsonBody, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshalling map response: %w", err)
	}

	// Apply zstd compression if requested
	if m.req.Compress == "zstd" {
		jsonBody = zstdframe.AppendEncode(nil, jsonBody, zstdframe.FastestCompression)
	}

	// Write header + body
	data := make([]byte, reservedResponseHeaderSize, reservedResponseHeaderSize+len(jsonBody))
	binary.LittleEndian.PutUint32(data, uint32(len(jsonBody)))
	data = append(data, jsonBody...)

	_, err = m.w.Write(data)
	if err != nil {
		return err
	}

	if m.isStreaming() {
		if f, ok := m.w.(http.Flusher); ok {
			f.Flush()
		}
	}

	return nil
}

var keepAlive = tailcfg.MapResponse{
	KeepAlive: true,
}