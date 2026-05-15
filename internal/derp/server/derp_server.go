package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/juanfont/headscale-v2/internal/types"
	"github.com/juanfont/headscale-v2/internal/util"
	"tailscale.com/derp"
	"tailscale.com/derp/derpserver"
	"tailscale.com/net/stun"
	"tailscale.com/net/wsconn"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
)

const (
	fastStartHeader  = "Derp-Fast-Start"
	DerpVerifyScheme = "headscale-derp-verify"
)

type DERPServer struct {
	serverURL     string
	key           key.NodePrivate
	cfg           *types.DERPConfig
	tailscaleDERP *derpserver.Server
	logger         *log.Helper
}

func NewDERPServer(
	serverURL string,
	derpKey key.NodePrivate,
	cfg *types.DERPConfig,
	logger log.Logger,
) (*DERPServer, error) {
	helper := log.NewHelper(logger)
	helper.Debug("creating new embedded DERP server")

	server := derpserver.New(derpKey, util.TSLogfWrapper())

	if cfg.ServerVerifyClients {
		server.SetVerifyClientURL(DerpVerifyScheme + "://verify")
		server.SetVerifyClientURLFailOpen(false)
	}

	return &DERPServer{
		serverURL:     serverURL,
		key:           derpKey,
		cfg:           cfg,
		tailscaleDERP: server,
		logger:        helper,
	}, nil
}

func (d *DERPServer) GenerateRegion() (tailcfg.DERPRegion, error) {
	serverURL, err := url.Parse(d.serverURL)
	if err != nil {
		return tailcfg.DERPRegion{}, err
	}

	var (
		host    string
		port    int
		portStr string
	)

	host, portStr, err = net.SplitHostPort(serverURL.Host)
	if err != nil {
		if serverURL.Scheme == "https" {
			host = serverURL.Host
			port = 443
		} else {
			host = serverURL.Host
			port = 80
		}
	} else {
		port, err = strconv.Atoi(portStr)
		if err != nil {
			return tailcfg.DERPRegion{}, err
		}
	}

	region := tailcfg.DERPRegion{
		RegionID:   d.cfg.ServerRegionID,
		RegionCode: d.cfg.ServerRegionCode,
		RegionName: d.cfg.ServerRegionName,
		Avoid:      false,
		Nodes: []*tailcfg.DERPNode{
			{
				Name:     strconv.Itoa(d.cfg.ServerRegionID),
				RegionID: d.cfg.ServerRegionID,
				HostName: host,
				DERPPort: port,
				IPv4:     d.cfg.IPv4,
				IPv6:     d.cfg.IPv6,
			},
		},
	}

	if d.cfg.STUNAddr != "" {
		_, portSTUNStr, err := net.SplitHostPort(d.cfg.STUNAddr)
		if err != nil {
			return tailcfg.DERPRegion{}, err
		}

		portSTUN, err := strconv.Atoi(portSTUNStr)
		if err != nil {
			return tailcfg.DERPRegion{}, err
		}

		region.Nodes[0].STUNPort = portSTUN
	}

	d.logger.Infof("generated DERP region: %+v", region)

	return region, nil
}

func (d *DERPServer) DERPHandler(writer http.ResponseWriter, req *http.Request) {
	d.logger.Debugf("/derp request from %v", req.RemoteAddr)
	upgrade := strings.ToLower(req.Header.Get("Upgrade"))

	if upgrade != "websocket" && upgrade != "derp" {
		if upgrade != "" {
			d.logger.Warnf("No Upgrade header in DERP server request. If headscale is behind a reverse proxy, make sure it is configured to pass WebSockets through.")
		}

		writer.Header().Set("Content-Type", "text/plain")
		writer.WriteHeader(http.StatusUpgradeRequired)
		_, _ = writer.Write([]byte("DERP requires connection upgrade"))
		return
	}

	if strings.Contains(req.Header.Get("Sec-Websocket-Protocol"), "derp") {
		d.serveWebsocket(writer, req)
	} else {
		d.servePlain(writer, req)
	}
}

func (d *DERPServer) serveWebsocket(writer http.ResponseWriter, req *http.Request) {
	websocketConn, err := websocket.Accept(writer, req, &websocket.AcceptOptions{
		Subprotocols:   []string{"derp"},
		OriginPatterns: []string{"*"},
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		d.logger.Errorf("Failed to upgrade websocket request: %v", err)
		writer.Header().Set("Content-Type", "text/plain")
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte("Failed to upgrade websocket request"))
		return
	}
	defer websocketConn.Close(websocket.StatusInternalError, "closing")

	if websocketConn.Subprotocol() != "derp" {
		websocketConn.Close(websocket.StatusPolicyViolation, "client must speak the derp subprotocol")
		return
	}

	wc := wsconn.NetConn(req.Context(), websocketConn, websocket.MessageBinary, req.RemoteAddr)
	brw := bufio.NewReadWriter(bufio.NewReader(wc), bufio.NewWriter(wc))
	d.tailscaleDERP.Accept(req.Context(), wc, brw, req.RemoteAddr)
}

func (d *DERPServer) servePlain(writer http.ResponseWriter, req *http.Request) {
	fastStart := req.Header.Get(fastStartHeader) == "1"

	hijacker, ok := writer.(http.Hijacker)
	if !ok {
		d.logger.Error("derp requires Hijacker interface from Gin")
		writer.Header().Set("Content-Type", "text/plain")
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte("HTTP does not support general TCP support"))
		return
	}

	netConn, conn, err := hijacker.Hijack()
	if err != nil {
		d.logger.Errorf("hijack failed: %v", err)
		writer.Header().Set("Content-Type", "text/plain")
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte("HTTP does not support general TCP support"))
		return
	}

	d.logger.Debugf("hijacked connection from %v", req.RemoteAddr)

	if !fastStart {
		pubKey := d.key.Public()
		pubKeyStr, _ := pubKey.MarshalText()
		fmt.Fprintf(conn, "HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: DERP\r\n"+
			"Connection: Upgrade\r\n"+
			"Derp-Version: %v\r\n"+
			"Derp-Public-Key: %s\r\n\r\n",
			derp.ProtocolVersion,
			string(pubKeyStr))
	}

	d.tailscaleDERP.Accept(req.Context(), netConn, conn, netConn.RemoteAddr().String())
}

func DERPProbeHandler(writer http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodHead, http.MethodGet:
		writer.Header().Set("Access-Control-Allow-Origin", "*")
		writer.WriteHeader(http.StatusOK)
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = writer.Write([]byte("bogus probe method"))
	}
}

func DERPBootstrapDNSHandler(derpMap tailcfg.DERPMapView) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, req *http.Request) {
		dnsEntries := make(map[string][]net.IP)

		resolvCtx, cancel := context.WithTimeout(req.Context(), time.Minute)
		defer cancel()

		var resolver net.Resolver

		for _, region := range derpMap.Regions().All() {
			for _, node := range region.Nodes().All() {
				addrs, err := resolver.LookupIP(resolvCtx, "ip", node.HostName())
				if err != nil {
					continue
				}

				dnsEntries[node.HostName()] = addrs
			}
		}

		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)

		_ = json.NewEncoder(writer).Encode(dnsEntries)
	}
}

func (d *DERPServer) ServeSTUN(ctx context.Context) error {
	if d.cfg.STUNAddr == "" {
		return nil
	}

	packetConn, err := new(net.ListenConfig).ListenPacket(ctx, "udp", d.cfg.STUNAddr)
	if err != nil {
		return fmt.Errorf("failed to open STUN listener: %w", err)
	}

	d.logger.Infof("STUN server started at %s", packetConn.LocalAddr())

	udpConn, ok := packetConn.(*net.UDPConn)
	if !ok {
		return fmt.Errorf("stun listener is not a UDP listener")
	}

	go d.serveSTUNListener(ctx, udpConn)

	return nil
}

func (d *DERPServer) serveSTUNListener(ctx context.Context, packetConn *net.UDPConn) {
	var buf [64 << 10]byte

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		bytesRead, udpAddr, err := packetConn.ReadFromUDP(buf[:])
		if err != nil {
			if ctx.Err() != nil {
				return
			}

			d.logger.Errorf("stun ReadFrom: %v", err)
			time.Sleep(time.Second)
			continue
		}

		d.logger.Debugf("stun request from %v", udpAddr)

		pkt := buf[:bytesRead]
		if !stun.Is(pkt) {
			continue
		}

		txid, err := stun.ParseBindingRequest(pkt)
		if err != nil {
			d.logger.Debugf("stun parse error: %v", err)
			continue
		}

		addr, _ := netip.AddrFromSlice(udpAddr.IP)
		res := stun.Response(txid, netip.AddrPortFrom(addr, uint16(udpAddr.Port)))

		_, err = packetConn.WriteTo(res, udpAddr)
		if err != nil {
			d.logger.Debugf("issue writing to UDP: %v", err)
			continue
		}
	}
}

func NewDERPVerifyTransport(handleVerifyRequest func(*http.Request, io.Writer) error) *DERPVerifyTransport {
	return &DERPVerifyTransport{
		handleVerifyRequest: handleVerifyRequest,
	}
}

type DERPVerifyTransport struct {
	handleVerifyRequest func(*http.Request, io.Writer) error
}

func (t *DERPVerifyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	buf := new(bytes.Buffer)

	err := t.handleVerifyRequest(req, buf)
	if err != nil {
		return nil, err
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(buf),
	}

	return resp, nil
}

func (d *DERPServer) Close() error {
	return nil
}
