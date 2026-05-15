package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/juanfont/headscale-v2/internal/types"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

const (
	HTTP01ChallengeType  = "HTTP-01"
	TLSALPN01ChallengeType = "TLS-ALPN-01"
)

type TLSSettings struct {
	Cert       *tls.Certificate
	CertPath   string
	KeyPath    string
	LetsEncrypt *LetsEncryptSettings
}

type LetsEncryptSettings struct {
	Hostname      string
	CacheDir      string
	ChallengeType string
	Listen        string
	Manager       *autocert.Manager
}

func GetTLSSettings(cfg *types.Config, logger log.Logger) (*TLSSettings, error) {
	helper := log.NewHelper(logger)

	settings := &TLSSettings{
		CertPath:   cfg.TLS.CertPath,
		KeyPath:    cfg.TLS.KeyPath,
	}

	if cfg.TLS.CertPath != "" && cfg.TLS.KeyPath != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLS.CertPath, cfg.TLS.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("loading TLS certificate: %w", err)
		}
		settings.Cert = &cert
		helper.Info("TLS certificate loaded from files")
		return settings, nil
	}

	if cfg.TLS.LetsEncrypt.Hostname != "" {
		leSettings, err := setupLetsEncrypt(cfg, helper)
		if err != nil {
			return nil, fmt.Errorf("setting up Let's Encrypt: %w", err)
		}
		settings.LetsEncrypt = leSettings
		helper.Infof("Let's Encrypt configured for hostname: %s", cfg.TLS.LetsEncrypt.Hostname)
		return settings, nil
	}

	return settings, nil
}

func setupLetsEncrypt(cfg *types.Config, logger *log.Helper) (*LetsEncryptSettings, error) {
	hostname := cfg.TLS.LetsEncrypt.Hostname
	if hostname == "" {
		return nil, fmt.Errorf("Let's Encrypt hostname is required")
	}

	cacheDir := cfg.TLS.LetsEncrypt.CacheDir
	if cacheDir == "" {
		cacheDir = "/var/www/.cache"
	}

	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return nil, fmt.Errorf("creating Let's Encrypt cache directory: %w", err)
	}

	manager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(hostname),
		Cache:      autocert.DirCache(cacheDir),
	}

	settings := &LetsEncryptSettings{
		Hostname:      hostname,
		CacheDir:      cacheDir,
		ChallengeType: cfg.TLS.LetsEncrypt.ChallengeType,
		Listen:        cfg.TLS.LetsEncrypt.Listen,
		Manager:       manager,
	}

	if settings.ChallengeType == "" {
		settings.ChallengeType = HTTP01ChallengeType
	}

	return settings, nil
}

func (s *TLSSettings) GetTLSConfig() (*tls.Config, error) {
	if s.Cert != nil {
		return &tls.Config{
			Certificates: []tls.Certificate{*s.Cert},
			MinVersion:   tls.VersionTLS12,
		}, nil
	}

	if s.LetsEncrypt != nil && s.LetsEncrypt.Manager != nil {
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
			GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
				return s.LetsEncrypt.Manager.GetCertificate(hello)
			},
		}

		if s.LetsEncrypt.ChallengeType == TLSALPN01ChallengeType {
			tlsConfig.NextProtos = []string{"h2", "http/1.1", acme.ALPNProto}
		}

		return tlsConfig, nil
	}

	return nil, nil
}

func (s *TLSSettings) GetHTTPChallengeHandler(next http.Handler) http.Handler {
	if s.LetsEncrypt == nil || s.LetsEncrypt.Manager == nil {
		return next
	}

	if s.LetsEncrypt.ChallengeType != HTTP01ChallengeType {
		return next
	}

	return s.LetsEncrypt.Manager.HTTPHandler(next)
}

func (s *TLSSettings) StartTLSALPN01Listener(ctx context.Context, logger *log.Helper) error {
	if s.LetsEncrypt == nil || s.LetsEncrypt.Manager == nil {
		return nil
	}

	if s.LetsEncrypt.ChallengeType != TLSALPN01ChallengeType {
		return nil
	}

	listenAddr := s.LetsEncrypt.Listen
	if listenAddr == "" {
		listenAddr = ":443"
	}

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("listening for TLS-ALPN-01: %w", err)
	}

	tlsConfig := &tls.Config{
		NextProtos:     []string{acme.ALPNProto},
		GetCertificate: s.LetsEncrypt.Manager.GetCertificate,
		MinVersion:     tls.VersionTLS12,
	}

	tlsLn := tls.NewListener(ln, tlsConfig)

	go func() {
		<-ctx.Done()
		tlsLn.Close()
		logger.Info("TLS-ALPN-01 listener stopped")
	}()

	go func() {
		logger.Infof("TLS-ALPN-01 listener started on %s", listenAddr)
		for {
			conn, err := tlsLn.Accept()
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				logger.Warnf("TLS-ALPN-01 accept error: %v", err)
				continue
			}
			conn.Close()
		}
	}()

	return nil
}

func (s *TLSSettings) HasTLS() bool {
	return s.Cert != nil || s.LetsEncrypt != nil
}

func GetTLSCertPool(caCertPaths []string) (*x509.CertPool, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}

	for _, path := range caCertPaths {
		if path == "" {
			continue
		}

		cert, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading CA certificate from %s: %w", path, err)
		}

		if !pool.AppendCertsFromPEM(cert) {
			return nil, fmt.Errorf("failed to append CA certificate from %s", path)
		}
	}

	return pool, nil
}

func IsTLSAddr(addr string) bool {
	return strings.HasSuffix(addr, ":443") || strings.HasSuffix(addr, ":8443")
}