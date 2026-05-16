package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/juanfont/headscale-v2/internal/types"
	"tailscale.com/tailcfg"
)

const (
	NoiseCapabilityVersion = 39
	verifyBodyLimit        = 4 * 1024
)

type HTTPError struct {
	Code int
	Msg  string
	Err  error
}

func (e HTTPError) Error() string {
	return fmt.Sprintf("http error[%d]: %s, %s", e.Code, e.Msg, e.Err)
}

func (e HTTPError) Unwrap() error {
	return e.Err
}

func NewHTTPError(code int, msg string, err error) HTTPError {
	return HTTPError{Code: code, Msg: msg, Err: err}
}

var errMethodNotAllowed = NewHTTPError(http.StatusMethodNotAllowed, "method not allowed", nil)

func httpError(w http.ResponseWriter, err error) {
	var herr HTTPError
	if errors.As(err, &herr) {
		http.Error(w, herr.Msg, herr.Code)
		fmt.Printf("[ERROR] http error: code=%d, msg=%s, err=%v\n", herr.Code, herr.Msg, herr.Err)
	} else {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		fmt.Printf("[ERROR] http internal server error: %v\n", err)
	}
}

func parseCapabilityVersion(req *http.Request) (tailcfg.CapabilityVersion, error) {
	clientCapabilityStr := req.URL.Query().Get("v")
	if clientCapabilityStr == "" {
		return 0, NewHTTPError(http.StatusBadRequest, "capability version must be set", nil)
	}

	clientCapabilityVersion, err := strconv.Atoi(clientCapabilityStr)
	if err != nil {
		return 0, NewHTTPError(http.StatusBadRequest, "invalid capability version", fmt.Errorf("parsing capability version: %w", err))
	}

	return tailcfg.CapabilityVersion(clientCapabilityVersion), nil
}

func (s *HeadscaleServer) KeyHandler(writer http.ResponseWriter, req *http.Request) {
	capVer, err := parseCapabilityVersion(req)
	if err != nil {
		httpError(writer, err)
		return
	}

	if capVer >= NoiseCapabilityVersion {
		resp := tailcfg.OverTLSPublicKeyResponse{
			PublicKey: s.noisePrivateKey.Public(),
		}

		writer.Header().Set("Content-Type", "application/json")

		err := json.NewEncoder(writer).Encode(resp)
		if err != nil {
			fmt.Printf("[ERROR] %s: %v\n", "failed to encode public key response", err)
		}

		return
	}

	http.Error(writer, "unsupported capability version", http.StatusBadRequest)
}

func (s *HeadscaleServer) HealthHandler(writer http.ResponseWriter, req *http.Request) {
	respond := func(err error) {
		writer.Header().Set("Content-Type", "application/health+json; charset=utf-8")

		res := struct {
			Status string `json:"status"`
		}{
			Status: "pass",
		}

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			res.Status = "fail"
		}

		encErr := json.NewEncoder(writer).Encode(res)
		if encErr != nil {
			fmt.Printf("[ERROR] failed to encode health response: %v\n", encErr)
		}
	}

	respond(nil)
}

func (s *HeadscaleServer) RobotsHandler(writer http.ResponseWriter, req *http.Request) {
	writer.Header().Set("Content-Type", "text/plain")
	writer.WriteHeader(http.StatusOK)

	_, err := writer.Write([]byte("User-agent: *\nDisallow: /"))
	if err != nil {
		fmt.Printf("[ERROR] %s: %v\n", "Failed to write robots.txt", err)
	}
}

func (s *HeadscaleServer) VersionHandler(writer http.ResponseWriter, req *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)

	versionInfo := struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
	}{
		Version: "v2.0.0",
		Commit:  "dev",
	}

	err := json.NewEncoder(writer).Encode(versionInfo)
	if err != nil {
		fmt.Printf("[ERROR] %s: %v\n", "Failed to write version response", err)
	}
}

func (s *HeadscaleServer) VerifyHandler(writer http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		httpError(writer, errMethodNotAllowed)
		return
	}

	resp := &tailcfg.DERPAdmitClientResponse{
		Allow: true,
	}

	writer.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(writer).Encode(resp)
	if err != nil {
		httpError(writer, err)
	}
}

func BlankHandler(writer http.ResponseWriter, req *http.Request) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(http.StatusOK)

	_, err := writer.Write([]byte("<html><body><h1>Headscale v2</h1></body></html>"))
	if err != nil {
		fmt.Printf("[ERROR] %s: %v\n", "Failed to write blank response", err)
	}
}

type AuthProviderWeb struct {
	serverURL string
}

func NewAuthProviderWeb(serverURL string) *AuthProviderWeb {
	return &AuthProviderWeb{
		serverURL: serverURL,
	}
}

func (a *AuthProviderWeb) RegisterURL(authID types.AuthID) string {
	return fmt.Sprintf(
		"%s/register/%s",
		strings.TrimSuffix(a.serverURL, "/"),
		authID.String())
}

func (a *AuthProviderWeb) AuthURL(authID types.AuthID) string {
	return fmt.Sprintf(
		"%s/auth/%s",
		strings.TrimSuffix(a.serverURL, "/"),
		authID.String())
}

func (a *AuthProviderWeb) AuthHandler(writer http.ResponseWriter, req *http.Request) {
	authID := req.PathValue("auth_id")
	if authID == "" {
		httpError(writer, NewHTTPError(http.StatusBadRequest, "invalid auth id", nil))
		return
	}

	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(http.StatusOK)

	html := fmt.Sprintf(`<html><body><h1>Authentication</h1><p>Run: headscale auth approve --auth-id %s</p></body></html>`, authID)
	_, err := writer.Write([]byte(html))
	if err != nil {
		fmt.Printf("[ERROR] %s: %v\n", "failed to write auth response", err)
	}
}

func (a *AuthProviderWeb) RegisterHandler(writer http.ResponseWriter, req *http.Request) {
	authID := req.PathValue("auth_id")
	if authID == "" {
		httpError(writer, NewHTTPError(http.StatusBadRequest, "invalid auth id", nil))
		return
	}

	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(http.StatusOK)

	html := fmt.Sprintf(`<html><body><h1>Node registration</h1><p>Run: headscale auth register --auth-id %s --user USERNAME</p></body></html>`, authID)
	_, err := writer.Write([]byte(html))
	if err != nil {
		fmt.Printf("[ERROR] %s: %v\n", "failed to write register response", err)
	}
}
