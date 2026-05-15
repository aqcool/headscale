package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/juanfont/headscale-v2/internal/state"
	"github.com/juanfont/headscale-v2/internal/types"
	"github.com/juanfont/headscale-v2/internal/util"
	"golang.org/x/oauth2"
)

const (
	randomByteSize           = 16
	defaultOAuthOptionsCount = 3
	authCacheExpiration      = time.Minute * 15
	authCacheMaxEntries      = 1024
	cookieNamePrefixLen      = 6
)

var (
	errOIDCStateTooShort       = errors.New("oidc state parameter is too short")
	errEmptyOIDCCallbackParams = errors.New("empty OIDC callback params")
	errNoOIDCIDToken           = errors.New("extracting ID token")
	errNoOIDCRegistrationInfo  = errors.New("registration info not in cache")
	errOIDCAllowedDomains      = errors.New("authenticated principal does not match any allowed domain")
	errOIDCAllowedGroups       = errors.New("authenticated principal is not in any allowed group")
	errOIDCAllowedUsers        = errors.New("authenticated principal does not match any allowed user")
	errOIDCUnverifiedEmail     = errors.New("authenticated principal has an unverified email")
)

type OIDCAuthInfo struct {
	AuthID       types.AuthID
	Verifier     *string
	Registration bool
}

type AuthProviderOIDC struct {
	serverURL string
	cfg       *types.OIDCConfig
	state     *state.State
	logger    *log.Helper

	authCache     *expirable.LRU[string, OIDCAuthInfo]
	oidcProvider  *oidc.Provider
	oauth2Config  *oauth2.Config
}

func NewAuthProviderOIDC(
	ctx context.Context,
	serverURL string,
	cfg *types.OIDCConfig,
	st *state.State,
	logger log.Logger,
) (*AuthProviderOIDC, error) {
	helper := log.NewHelper(logger)

	oidcProvider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("creating OIDC provider: %w", err)
	}

	oauth2Config := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     oidcProvider.Endpoint(),
		RedirectURL:  strings.TrimSuffix(serverURL, "/") + "/oidc/callback",
		Scopes:       cfg.Scope,
	}

	authCache := expirable.NewLRU[string, OIDCAuthInfo](
		authCacheMaxEntries,
		nil,
		authCacheExpiration,
	)

	return &AuthProviderOIDC{
		serverURL:    serverURL,
		cfg:          cfg,
		state:        st,
		logger:       helper,
		authCache:    authCache,
		oidcProvider: oidcProvider,
		oauth2Config: oauth2Config,
	}, nil
}

func (a *AuthProviderOIDC) AuthURL(authID types.AuthID) string {
	return fmt.Sprintf("%s/auth/%s", strings.TrimSuffix(a.serverURL, "/"), authID.String())
}

func (a *AuthProviderOIDC) RegisterURL(authID types.AuthID) string {
	return fmt.Sprintf("%s/register/%s", strings.TrimSuffix(a.serverURL, "/"), authID.String())
}

func (a *AuthProviderOIDC) AuthHandler(writer http.ResponseWriter, req *http.Request) {
	a.authHandler(writer, req, false)
}

func (a *AuthProviderOIDC) RegisterHandler(writer http.ResponseWriter, req *http.Request) {
	a.authHandler(writer, req, true)
}

func (a *AuthProviderOIDC) authHandler(writer http.ResponseWriter, req *http.Request, registration bool) {
	authID, err := authIDFromRequest(req)
	if err != nil {
		httpError(writer, NewHTTPError(http.StatusBadRequest, "invalid auth ID", err))
		return
	}

	state, err := setCSRFCookie(writer, req, "state")
	if err != nil {
		httpError(writer, NewHTTPError(http.StatusInternalServerError, "setting state cookie", err))
		return
	}

	nonce, err := setCSRFCookie(writer, req, "nonce")
	if err != nil {
		httpError(writer, NewHTTPError(http.StatusInternalServerError, "setting nonce cookie", err))
		return
	}

	authInfo := OIDCAuthInfo{
		AuthID:       authID,
		Registration: registration,
	}

	extras := make([]oauth2.AuthCodeOption, 0, len(a.cfg.ExtraParams)+defaultOAuthOptionsCount)

	if a.cfg.PKCE.Enabled {
		verifier := oauth2.GenerateVerifier()
		authInfo.Verifier = &verifier
		extras = append(extras, oauth2.AccessTypeOffline)

		switch a.cfg.PKCE.Method {
		case types.PKCEMethodS256:
			extras = append(extras, oauth2.S256ChallengeOption(verifier))
		case types.PKCEMethodPlain:
			extras = append(extras,
				oauth2.SetAuthURLParam("code_challenge_method", "plain"),
				oauth2.SetAuthURLParam("code_challenge", verifier))
		}
	}

	for k, v := range a.cfg.ExtraParams {
		extras = append(extras, oauth2.SetAuthURLParam(k, v))
	}

	extras = append(extras, oidc.Nonce(nonce))

	a.authCache.Add(state, authInfo)

	authURL := a.oauth2Config.AuthCodeURL(state, extras...)
	a.logger.Debugf("redirecting to %s for authentication", authURL)

	http.Redirect(writer, req, authURL, http.StatusFound)
}

func (a *AuthProviderOIDC) OIDCCallbackHandler(writer http.ResponseWriter, req *http.Request) {
	code, state, err := extractCodeAndStateFromRequest(req)
	if err != nil {
		httpError(writer, NewHTTPError(http.StatusBadRequest, err.Error(), nil))
		return
	}

	stateCookieName := getCookieName("state", state)
	stateCookie, err := req.Cookie(stateCookieName)
	if err != nil {
		httpError(writer, NewHTTPError(http.StatusBadRequest, "state cookie not found", nil))
		return
	}

	if state != stateCookie.Value {
		httpError(writer, NewHTTPError(http.StatusForbidden, "state mismatch", nil))
		return
	}

	oauth2Token, err := a.getOauth2Token(req.Context(), code, state)
	if err != nil {
		httpError(writer, NewHTTPError(http.StatusForbidden, err.Error(), nil))
		return
	}

	idToken, err := a.extractIDToken(req.Context(), oauth2Token)
	if err != nil {
		httpError(writer, NewHTTPError(http.StatusForbidden, err.Error(), nil))
		return
	}

	if idToken.Nonce == "" {
		httpError(writer, NewHTTPError(http.StatusBadRequest, "nonce missing in ID token", nil))
		return
	}

	nonceCookieName := getCookieName("nonce", idToken.Nonce)
	nonceCookie, err := req.Cookie(nonceCookieName)
	if err != nil {
		httpError(writer, NewHTTPError(http.StatusBadRequest, "nonce cookie not found", nil))
		return
	}

	if idToken.Nonce != nonceCookie.Value {
		httpError(writer, NewHTTPError(http.StatusForbidden, "nonce mismatch", nil))
		return
	}

	nodeExpiry := a.determineNodeExpiry(idToken.Expiry)

	var claims types.OIDCClaims
	if err := idToken.Claims(&claims); err != nil {
		httpError(writer, NewHTTPError(http.StatusInternalServerError, "decoding claims", err))
		return
	}

	userinfo, err := a.oidcProvider.UserInfo(req.Context(), oauth2.StaticTokenSource(oauth2Token))
	if err != nil {
		a.logger.Warnf("could not get userinfo: %v", err)
	} else {
		var userinfo2 types.OIDCUserInfo
		if userinfo.Claims(&userinfo2) == nil && userinfo2.Sub == claims.Sub {
			claims.Email = firstNonEmpty(userinfo2.Email, claims.Email)
			if userinfo2.EmailVerified {
				claims.EmailVerified = true
			}
			claims.Username = firstNonEmpty(userinfo2.PreferredUsername, claims.Username)
			claims.Name = firstNonEmpty(userinfo2.Name, claims.Name)
			claims.ProfilePictureURL = firstNonEmpty(userinfo2.Picture, claims.ProfilePictureURL)
			if userinfo2.Groups != nil {
				claims.Groups = userinfo2.Groups
			}
		}
	}

	err = a.doOIDCAuthorization(&claims)
	if err != nil {
		httpError(writer, NewHTTPError(http.StatusUnauthorized, err.Error(), err))
		return
	}

	user, err := a.createOrUpdateUser(&claims)
	if err != nil {
		httpError(writer, NewHTTPError(http.StatusInternalServerError, "creating user", err))
		return
	}

	authInfo := a.getAuthInfoFromState(state)
	if authInfo == nil {
		httpError(writer, NewHTTPError(http.StatusGone, "session expired", nil))
		return
	}

	if authInfo.Registration {
		a.renderRegistrationConfirm(writer, req, authInfo.AuthID, user, nodeExpiry)
		return
	}

	authReq, ok := a.state.GetAuthCacheEntry(authInfo.AuthID)
	if !ok {
		httpError(writer, NewHTTPError(http.StatusGone, "auth session expired", nil))
		return
	}

	authReq.FinishAuth(types.AuthVerdict{})

	content := renderAuthSuccessTemplate(user)
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(content.Bytes())
}

func (a *AuthProviderOIDC) RegisterConfirmHandler(writer http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		httpError(writer, errMethodNotAllowed)
		return
	}

	authID, err := authIDFromRequest(req)
	if err != nil {
		httpError(writer, NewHTTPError(http.StatusBadRequest, "invalid auth ID", nil))
		return
	}

	req.Body = http.MaxBytesReader(writer, req.Body, 4*1024)
	if err := req.ParseForm(); err != nil {
		httpError(writer, NewHTTPError(http.StatusBadRequest, "invalid form", nil))
		return
	}

	authReq, ok := a.state.GetAuthCacheEntry(authID)
	if !ok {
		httpError(writer, NewHTTPError(http.StatusGone, "registration session expired", nil))
		return
	}

	pending := authReq.PendingConfirmation()
	if pending == nil {
		httpError(writer, NewHTTPError(http.StatusForbidden, "registration not authorized", nil))
		return
	}

	user, err := a.state.GetUserByID(types.UserID(pending.UserID))
	if err != nil {
		httpError(writer, NewHTTPError(http.StatusInternalServerError, "user lookup failed", err))
		return
	}

	_, err = a.handleRegistration(user, authID, pending.NodeExpiry)
	if err != nil {
		httpError(writer, NewHTTPError(http.StatusInternalServerError, err.Error(), err))
		return
	}

	http.SetCookie(writer, &http.Cookie{
		Name:     "headscale_register_confirm",
		Value:    "",
		MaxAge:   -1,
		Secure:   req.TLS != nil,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	content := renderRegistrationSuccessTemplate(user, true)
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(content.Bytes())
}

func (a *AuthProviderOIDC) determineNodeExpiry(idTokenExpiration time.Time) *time.Time {
	if a.cfg.UseExpiryFromToken {
		return &idTokenExpiration
	}
	return nil
}

func (a *AuthProviderOIDC) getOauth2Token(ctx context.Context, code, state string) (*oauth2.Token, error) {
	var exchangeOpts []oauth2.AuthCodeOption

	if a.cfg.PKCE.Enabled {
		regInfo, ok := a.authCache.Get(state)
		if !ok {
			return nil, errNoOIDCRegistrationInfo
		}
		if regInfo.Verifier != nil {
			exchangeOpts = []oauth2.AuthCodeOption{oauth2.VerifierOption(*regInfo.Verifier)}
		}
	}

	oauth2Token, err := a.oauth2Config.Exchange(ctx, code, exchangeOpts...)
	if err != nil {
		return nil, fmt.Errorf("exchanging code: %w", err)
	}
	return oauth2Token, nil
}

func (a *AuthProviderOIDC) extractIDToken(ctx context.Context, oauth2Token *oauth2.Token) (*oidc.IDToken, error) {
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return nil, errNoOIDCIDToken
	}

	verifier := a.oidcProvider.Verifier(&oidc.Config{ClientID: a.cfg.ClientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("verifying ID token: %w", err)
	}
	return idToken, nil
}

func (a *AuthProviderOIDC) doOIDCAuthorization(claims *types.OIDCClaims) error {
	if len(a.cfg.AllowedGroups) > 0 {
		if err := validateOIDCAllowedGroups(a.cfg.AllowedGroups, claims); err != nil {
			return err
		}
	}

	trustEmail := !a.cfg.EmailVerifiedRequired || bool(claims.EmailVerified)
	hasEmailTests := len(a.cfg.AllowedDomains) > 0 || len(a.cfg.AllowedUsers) > 0

	if !trustEmail && hasEmailTests {
		return errOIDCUnverifiedEmail
	}

	if len(a.cfg.AllowedDomains) > 0 {
		if err := validateOIDCAllowedDomains(a.cfg.AllowedDomains, claims); err != nil {
			return err
		}
	}

	if len(a.cfg.AllowedUsers) > 0 {
		if err := validateOIDCAllowedUsers(a.cfg.AllowedUsers, claims); err != nil {
			return err
		}
	}

	return nil
}

func validateOIDCAllowedDomains(allowedDomains []string, claims *types.OIDCClaims) error {
	if len(allowedDomains) > 0 {
		at := strings.LastIndex(claims.Email, "@")
		if at < 0 || !slices.Contains(allowedDomains, claims.Email[at+1:]) {
			return errOIDCAllowedDomains
		}
	}
	return nil
}

func validateOIDCAllowedGroups(allowedGroups []string, claims *types.OIDCClaims) error {
	for _, group := range allowedGroups {
		if slices.Contains(claims.Groups, group) {
			return nil
		}
	}
	return errOIDCAllowedGroups
}

func validateOIDCAllowedUsers(allowedUsers []string, claims *types.OIDCClaims) error {
	if !slices.Contains(allowedUsers, claims.Email) {
		return errOIDCAllowedUsers
	}
	return nil
}

func (a *AuthProviderOIDC) getAuthInfoFromState(state string) *OIDCAuthInfo {
	authInfo, ok := a.authCache.Get(state)
	if !ok {
		return nil
	}
	return &authInfo
}

func (a *AuthProviderOIDC) createOrUpdateUser(claims *types.OIDCClaims) (*types.User, error) {
	identifier := claims.Identifier()

	user, err := a.state.GetUserByProviderIdentifier(identifier)
	if err != nil && !errors.Is(err, state.ErrUserNotFound) {
		return nil, fmt.Errorf("looking up user: %w", err)
	}

	if user == nil {
		user = &types.User{
			Name:     claims.Username,
			Email:    claims.Email,
			DisplayName: claims.Name,
			ProfileURL:  claims.ProfilePictureURL,
			Provider:    "oidc",
		}
		if a.cfg.EmailVerifiedRequired && !bool(claims.EmailVerified) {
			user.Email = ""
		}
		user.ProviderIdentifier.String = identifier
		user.ProviderIdentifier.Valid = true

		created, err := a.state.CreateUser(user.Name)
		if err != nil {
			return nil, fmt.Errorf("creating user: %w", err)
		}
		user = created
	} else {
		user.Name = claims.Username
		user.DisplayName = claims.Name
		user.ProfileURL = claims.ProfilePictureURL
		if !a.cfg.EmailVerifiedRequired || bool(claims.EmailVerified) {
			user.Email = claims.Email
		}
	}

	return user, nil
}

func (a *AuthProviderOIDC) handleRegistration(user *types.User, authID types.AuthID, expiry *time.Time) (bool, error) {
	regData, ok := a.state.GetRegistrationData(authID)
	if !ok {
		return false, errors.New("registration data not found")
	}

	node := &types.Node{
		MachineKey:     regData.MachineKey,
		NodeKey:        regData.NodeKey,
		Hostname:       regData.Hostname,
		Hostinfo:       regData.Hostinfo,
		RegisterMethod: "oidc",
		Expiry:         expiry,
	}

	if user != nil {
		uid := uint(user.ID)
		node.UserID = &uid
	}

	addedNode, err := a.state.AddNode(context.Background(), node)
	if err != nil {
		return false, fmt.Errorf("registering node: %w", err)
	}

	return addedNode.Valid(), nil
}

func (a *AuthProviderOIDC) renderRegistrationConfirm(writer http.ResponseWriter, req *http.Request, authID types.AuthID, user *types.User, expiry *time.Time) {
	authReq, ok := a.state.GetAuthCacheEntry(authID)
	if !ok {
		httpError(writer, NewHTTPError(http.StatusGone, "session expired", nil))
		return
	}

	csrf, err := util.GenerateRandomStringURLSafe(32)
	if err != nil {
		httpError(writer, NewHTTPError(http.StatusInternalServerError, "generating CSRF", err))
		return
	}

	authReq.SetPendingConfirmation(&types.PendingRegistrationConfirmation{
		UserID:     uint(user.ID),
		NodeExpiry: expiry,
		CSRF:       csrf,
	})

	http.SetCookie(writer, &http.Cookie{
		Name:     "headscale_register_confirm",
		Value:    csrf,
		Path:     "/register/confirm/" + authID.String(),
		MaxAge:   int(authCacheExpiration.Seconds()),
		Secure:   req.TLS != nil,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	regData, _ := a.state.GetRegistrationData(authID)

	info := RegistrationConfirmInfo{
		FormAction: "/register/confirm/" + authID.String(),
		CSRFToken:  csrf,
		User:       user.Display(),
		Hostname:   regData.Hostname,
	}

	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte(renderRegistrationConfirmPage(info)))
}

type RegistrationConfirmInfo struct {
	FormAction string
	CSRFToken  string
	User       string
	Hostname   string
	OS         string
}

func renderRegistrationConfirmPage(info RegistrationConfirmInfo) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>Headscale - Confirm Registration</title></head>
<body>
<h1>Confirm Node Registration</h1>
<p>User: %s</p>
<p>Hostname: %s</p>
<form method="POST" action="%s">
<input type="hidden" name="headscale_register_confirm" value="%s">
<button type="submit">Confirm Registration</button>
</form>
</body>
</html>`, info.User, info.Hostname, info.FormAction, info.CSRFToken)
}

func renderAuthSuccessTemplate(user *types.User) *bytes.Buffer {
	return bytes.NewBufferString(fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>Headscale - Auth Success</title></head>
<body>
<h1>SSH Session Authorized</h1>
<p>User: %s</p>
<p>You may return to your terminal.</p>
</body>
</html>`, user.Display()))
}

func renderRegistrationSuccessTemplate(user *types.User, newNode bool) *bytes.Buffer {
	verb := "Reauthenticated"
	if newNode {
		verb = "Registered"
	}
	return bytes.NewBufferString(fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>Headscale - Registration Success</title></head>
<body>
<h1>Node %s</h1>
<p>User: %s</p>
<p>You can now close this window.</p>
</body>
</html>`, verb, user.Display()))
}

func authIDFromRequest(req *http.Request) (types.AuthID, error) {
	pathParts := strings.Split(req.URL.Path, "/")
	if len(pathParts) < 3 {
		return "", errors.New("invalid path")
	}
	authIDStr := pathParts[len(pathParts)-1]
	authID := types.AuthID(authIDStr)
	if err := authID.Validate(); err != nil {
		return "", err
	}
	return authID, nil
}

func extractCodeAndStateFromRequest(req *http.Request) (string, string, error) {
	code := req.URL.Query().Get("code")
	state := req.URL.Query().Get("state")

	if code == "" || state == "" {
		return "", "", errEmptyOIDCCallbackParams
	}

	if len(state) < cookieNamePrefixLen {
		return "", "", errOIDCStateTooShort
	}

	return code, state, nil
}

func getCookieName(baseName, value string) string {
	return fmt.Sprintf("%s_%s", baseName, value[:cookieNamePrefixLen])
}

func setCSRFCookie(w http.ResponseWriter, r *http.Request, name string) (string, error) {
	val, err := util.GenerateRandomStringURLSafe(64)
	if err != nil {
		return "", err
	}

	http.SetCookie(w, &http.Cookie{
		Path:     "/oidc/callback",
		Name:     getCookieName(name, val),
		Value:    val,
		MaxAge:   int(time.Hour.Seconds()),
		Secure:   r.TLS != nil,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	return val, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}