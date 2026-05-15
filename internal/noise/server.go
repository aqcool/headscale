package noise

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/juanfont/headscale-v2/internal/state"
	"tailscale.com/control/controlbase"
	"tailscale.com/types/key"
)

const (
	earlyPayloadMagic = "\xff\xff\xffTS"
	protocolVersion   = 1
)

var (
	ErrInvalidConn        = errors.New("invalid connection")
	ErrInvalidMachineKey  = errors.New("invalid machine key")
	ErrInvalidNodeKey     = errors.New("invalid node key")
	ErrRegistrationFailed = errors.New("registration failed")
)

type NoiseServer struct {
	cfg        *Config
	state      *state.State
	privateKey key.MachinePrivate
	httpServer *http.Server
	conns      sync.Map
	logger     *log.Helper
}

type Config struct {
	ServerURL  string
	ListenAddr string
	PrivateKey key.MachinePrivate
}

func NewNoiseServer(cfg *Config, st *state.State, logger log.Logger) *NoiseServer {
	return &NoiseServer{
		cfg:        cfg,
		state:      st,
		privateKey: cfg.PrivateKey,
		logger:     log.NewHelper(logger),
	}
}

func (s *NoiseServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ts2021", s.NoiseUpgradeHandler)
	mux.HandleFunc("/machine/register", s.handleRegisterHTTP)
	mux.HandleFunc("/machine/map", s.handleMapHTTP)

	s.httpServer = &http.Server{
		Addr:    s.cfg.ListenAddr,
		Handler: mux,
	}

	s.logger.Infof("Noise server starting on %s", s.cfg.ListenAddr)
	return s.httpServer.ListenAndServe()
}

func (s *NoiseServer) Stop(ctx context.Context) error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// ServeHTTP implements http.Handler interface
func (s *NoiseServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ts2021", s.NoiseUpgradeHandler)
	mux.HandleFunc("/machine/register", s.handleRegisterHTTP)
	mux.HandleFunc("/machine/map", s.handleMapHTTP)
	mux.ServeHTTP(w, r)
}

func (s *NoiseServer) NoiseUpgradeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	conn, err := s.upgradeToNoise(r)
	if err != nil {
		s.logger.Errorf("Noise upgrade failed: %v", err)
		http.Error(w, "Noise upgrade failed", http.StatusBadRequest)
		return
	}
	defer conn.Close()

	s.handleConn(r.Context(), conn)
}

func (s *NoiseServer) upgradeToNoise(r *http.Request) (*controlbase.Conn, error) {
	hijacker, ok := r.Context().Value(http.Hijacker(nil)).(http.Hijacker)
	if !ok {
		return nil, ErrInvalidConn
	}

	netConn, rw, err := hijacker.Hijack()
	if err != nil {
		return nil, fmt.Errorf("hijack failed: %w", err)
	}

	if rw != nil {
		if err := rw.Flush(); err != nil {
			netConn.Close()
			return nil, fmt.Errorf("flush failed: %w", err)
		}
	}

	// Create Noise connection using controlbase.Server
	conn, err := controlbase.Server(r.Context(), netConn, s.privateKey, nil)
	if err != nil {
		netConn.Close()
		return nil, fmt.Errorf("noise handshake failed: %w", err)
	}

	return conn, nil
}

func (s *NoiseServer) handleConn(ctx context.Context, conn *controlbase.Conn) {
	s.logger.Infof("New Noise connection")

	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg, err := readMsg(conn)
			if err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
					s.logger.Debugf("Connection closed")
					return
				}
				s.logger.Errorf("Read error: %v", err)
				return
			}

			s.handleMessage(ctx, conn, msg)
		}
	}
}

func (s *NoiseServer) handleMessage(ctx context.Context, conn *controlbase.Conn, msg []byte) {
	if len(msg) < 1 {
		return
	}

	msgType := msg[0]
	data := msg[1:]

	switch msgType {
	case messageTypeRegister:
		s.handleRegisterMessage(ctx, conn, data)
	case messageTypeMapRequest:
		s.handleMapRequest(ctx, conn, data)
	default:
		s.logger.Warnf("Unknown message type: %d", msgType)
	}
}

const (
	messageTypeRegister   = 1
	messageTypeMapRequest = 2
)

func readMsg(r io.Reader) ([]byte, error) {
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

func writeMsg(w io.Writer, msg []byte) error {
	if err := binary.Write(w, binary.BigEndian, uint32(len(msg))); err != nil {
		return err
	}
	_, err := w.Write(msg)
	return err
}
