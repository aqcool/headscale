package server

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/juanfont/headscale-v2/internal/types"
	"tailscale.com/control/controlbase"
	"tailscale.com/control/controlhttp/controlhttpserver"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
)

var (
	ErrUnsupportedClientVersion = errors.New("unsupported client version")
	ErrMissingURLParameter      = errors.New("missing URL parameter")
	ErrNoAuthSession            = errors.New("no auth session found")
)

const (
	ts2021UpgradePath  = "/ts2021"
	earlyPayloadMagic  = "\xff\xff\xffTS"
	noiseBodyLimit     = 1048576 // 1 MiB
	reservedResponseHeaderSize = 4
)

// noiseServer handles Noise protocol connections.
type noiseServer struct {
	headscale       *Headscale
	conn            *controlbase.Conn
	machineKey      key.MachinePublic
	nodeKey         key.NodePublic
	challenge       key.ChallengePrivate
	protocolVersion int
	logger          *log.Helper
}

// NoiseUpgradeHandler handles the TS2021 protocol upgrade.
func (h *Headscale) NoiseUpgradeHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Debugf("Noise upgrade request from %s", r.RemoteAddr)

	upgrade := r.Header.Get("Upgrade")
	if upgrade == "" {
		h.logger.Warnf("No upgrade header in TS2021 request")
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	ns := &noiseServer{
		headscale: h,
		challenge: key.NewChallenge(),
		logger:    h.logger,
	}

	noiseConn, err := controlhttpserver.AcceptHTTP(
		r.Context(),
		w,
		r,
		*h.noisePrivateKey,
		ns.earlyNoise,
	)
	if err != nil {
		httpError(w, fmt.Errorf("upgrading noise connection: %w", err))
		return
	}

	ns.conn = noiseConn
	ns.machineKey = ns.conn.Peer()
	ns.protocolVersion = ns.conn.ProtocolVersion()

	// Set up router for Noise connection
	router := chi.NewRouter()
	router.Use(middleware.Recoverer)

	router.Route("/machine", func(r chi.Router) {
		r.Post("/register", ns.RegistrationHandler)
		r.Post("/map", ns.PollNetMapHandler)
		r.Get("/ssh/action/{src_node_id}/to/{dst_node_id}", ns.SSHActionHandler)
	})

	// Serve HTTP/2 over Noise connection
	ns.serveNoiseConn(router)
}

func (ns *noiseServer) serveNoiseConn(handler http.Handler) {
	// Simple echo loop for now - full HTTP/2 would need http2.Server
	for {
		msg, err := readNoiseMessage(ns.conn)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			ns.logger.Errorf("Read error: %v", err)
			return
		}

		// Process message
		response, err := ns.handleMessage(context.Background(), msg)
		if err != nil {
			ns.logger.Errorf("Handle error: %v", err)
			continue
		}

		if response != nil {
			if err := writeNoiseMessage(ns.conn, response); err != nil {
				ns.logger.Errorf("Write error: %v", err)
				return
			}
		}
	}
}

func (ns *noiseServer) handleMessage(ctx context.Context, msg []byte) ([]byte, error) {
	// Parse as MapRequest or RegisterRequest
	var req struct {
		NodeKey key.NodePublic `json:"NodeKey"`
	}

	if err := json.Unmarshal(msg, &req); err != nil {
		return nil, err
	}

	// Return keepalive for now
	return json.Marshal(&tailcfg.MapResponse{KeepAlive: true})
}

func (ns *noiseServer) earlyNoise(protocolVersion int, writer io.Writer) error {
	earlyJSON, err := json.Marshal(&tailcfg.EarlyNoise{
		NodeKeyChallenge: ns.challenge.Public(),
	})
	if err != nil {
		return err
	}

	var notH2Frame [5]byte
	copy(notH2Frame[:], earlyPayloadMagic)

	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(earlyJSON)))

	if _, err := writer.Write(notH2Frame[:]); err != nil {
		return err
	}
	if _, err := writer.Write(lenBuf[:]); err != nil {
		return err
	}
	if _, err := writer.Write(earlyJSON); err != nil {
		return err
	}

	return nil
}

// RegistrationHandler handles node registration over Noise.
func (ns *noiseServer) RegistrationHandler(w http.ResponseWriter, r *http.Request) {
	var regReq tailcfg.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&regReq); err != nil {
		httpError(w, err)
		return
	}

	ns.nodeKey = regReq.NodeKey

	resp, err := ns.headscale.handleRegister(r.Context(), regReq, ns.machineKey)
	if err != nil {
		if httpErr, ok := err.(HTTPError); ok {
			resp = &tailcfg.RegisterResponse{Error: httpErr.Msg}
		} else {
			resp = &tailcfg.RegisterResponse{Error: err.Error()}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// PollNetMapHandler handles MapRequest over Noise.
func (ns *noiseServer) PollNetMapHandler(w http.ResponseWriter, r *http.Request) {
	var mapReq tailcfg.MapRequest
	if err := json.NewDecoder(r.Body).Decode(&mapReq); err != nil {
		httpError(w, err)
		return
	}

	nv, ok := ns.headscale.state.GetNodeByNodeKey(mapReq.NodeKey)
	if !ok {
		httpError(w, NewHTTPError(http.StatusNotFound, "node not found", nil))
		return
	}

	if ns.machineKey != nv.MachineKey() {
		httpError(w, NewHTTPError(http.StatusUnauthorized, "machine key mismatch", nil))
		return
	}

	// Build MapResponse
	resp, err := ns.headscale.mapper.BuildMapResponse(r.Context(), nv.ID(), mapReq)
	if err != nil {
		httpError(w, err)
		return
	}

	// Write response
	jsonBody, _ := json.Marshal(resp)
	data := make([]byte, reservedResponseHeaderSize, reservedResponseHeaderSize+len(jsonBody))
	binary.LittleEndian.PutUint32(data, uint32(len(jsonBody)))
	data = append(data, jsonBody...)

	w.Write(data)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// SSHActionHandler handles SSH action requests.
func (ns *noiseServer) SSHActionHandler(w http.ResponseWriter, r *http.Request) {
	srcNodeID := chi.URLParam(r, "src_node_id")
	dstNodeID := chi.URLParam(r, "dst_node_id")

	ns.logger.Debugf("SSH action request: src=%s, dst=%s", srcNodeID, dstNodeID)

	// Return default action
	action := &tailcfg.SSHAction{
		Accept:                    true,
		AllowAgentForwarding:      true,
		AllowLocalPortForwarding:  true,
		AllowRemotePortForwarding: true,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(action)
}

func readNoiseMessage(r io.Reader) ([]byte, error) {
	var size uint32
	if err := binary.Read(r, binary.BigEndian, &size); err != nil {
		return nil, err
	}
	if size > 1<<20 {
		return nil, fmt.Errorf("message too large: %d", size)
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func writeNoiseMessage(w io.Writer, msg []byte) error {
	if err := binary.Write(w, binary.BigEndian, uint32(len(msg))); err != nil {
		return err
	}
	_, err := w.Write(msg)
	return err
}

// PingResponseHandler handles ping responses from nodes.
func (h *Headscale) PingResponseHandler(w http.ResponseWriter, r *http.Request) {
	pingID := r.URL.Query().Get("id")
	if pingID == "" {
		http.Error(w, "missing ping ID", http.StatusBadRequest)
		return
	}

	if h.state.CompletePing(pingID) {
		w.WriteHeader(http.StatusOK)
	} else {
		http.Error(w, "unknown or expired ping", http.StatusNotFound)
	}
}

func urlParam[T any](r *http.Request, key string) (T, error) {
	var zero T
	param := chi.URLParam(r, key)
	if param == "" {
		return zero, fmt.Errorf("%w: %s", ErrMissingURLParameter, key)
	}

	switch any(zero).(type) {
	case string:
		return any(param).(T), nil
	case types.NodeID:
		id, err := types.ParseNodeID(param)
		if err != nil {
			return zero, err
		}
		return any(id).(T), nil
	default:
		return zero, fmt.Errorf("unsupported type")
	}
}
