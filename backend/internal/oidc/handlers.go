package oidc

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ─── HTTP Handlers ───────────────────────────────────────────

// DiscoveryHandler handles the OIDC discovery endpoint.
func (p *Provider) DiscoveryHandler(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, p.DiscoveryDocument())
}

// JWKSHandler handles the JWKS endpoint.
func (p *Provider) JWKSHandler(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, p.JWKS())
}

// AuthorizationHandler handles the OAuth2 authorization endpoint.
// GET /authorize?client_id=...&redirect_uri=...&response_type=code&scope=...&state=...&code_challenge=...&code_challenge_method=...&nonce=...
func (p *Provider) AuthorizationHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	clientID := r.Form.Get("client_id")
	redirectURI := r.Form.Get("redirect_uri")
	responseType := r.Form.Get("response_type")
	scope := r.Form.Get("scope")
	state := r.Form.Get("state")
	codeChallenge := r.Form.Get("code_challenge")
	codeChallengeMethod := r.Form.Get("code_challenge_method")
	nonce := r.Form.Get("nonce")

	if clientID == "" || redirectURI == "" {
		jsonError(w, http.StatusBadRequest, "invalid_request", "client_id and redirect_uri required")
		return
	}

	if responseType != "code" {
		http.Redirect(w, r, buildRedirectURL(redirectURI, map[string]string{
			"error": "unsupported_response_type", "state": state,
		}), http.StatusFound)
		return
	}

	client, err := p.GetClient(r.Context(), clientID)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid_client", "Client not found")
		return
	}

	if !isValidRedirectURI(client, redirectURI) {
		jsonError(w, http.StatusBadRequest, "invalid_redirect_uri", "Redirect URI not registered")
		return
	}

	// Authenticate user: try POST form first, then BasicAuth header
	var username, password string
	if r.Method == "POST" {
		r.ParseForm()
		username = r.FormValue("email")
		password = r.FormValue("password")
	} else {
		u, pAuth, ok := r.BasicAuth()
		if ok {
			username, password = u, pAuth
		}
	}

	if username == "" {
		// Show login form with all OIDC parameters preserved
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, loginHTML, redirectURI, clientID, scope, state, codeChallenge, codeChallengeMethod, responseType, nonce, redirectURI)
		return
	}

	if password == "" {
		http.Redirect(w, r, buildRedirectURL(redirectURI, map[string]string{
			"error": "access_denied", "state": state,
		}), http.StatusFound)
		return
	}

	userID, err := p.AuthenticatePassword(r.Context(), username, password)
	if err != nil {
		http.Redirect(w, r, buildRedirectURL(redirectURI, map[string]string{
			"error": "access_denied", "state": state,
		}), http.StatusFound)
		return
	}

	// Parse scope
	scopeParts := strings.Split(scope, " ")
	if len(scopeParts) == 1 && scopeParts[0] == "" {
		scopeParts = []string{"openid"}
	}

	// Generate authorization code
	code := GenerateCode()
	if err := p.StoreAuthCode(r.Context(), code, clientID, userID, redirectURI, scopeParts, codeChallenge, codeChallengeMethod, nonce); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Redirect back with code
	http.Redirect(w, r, buildRedirectURL(redirectURI, map[string]string{
		"code": code, "state": state,
	}), http.StatusFound)
}

// TokenHandler handles the OAuth2 token endpoint.
// POST /token with grant_type=authorization_code|client_credentials|refresh_token|urn:ietf:params:oauth:grant-type:device_code
func (p *Provider) TokenHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid_request", "Could not parse form")
		return
	}

	grantType := r.FormValue("grant_type")
	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")

	// Client authentication (for confidential clients)
	var client *ClientRecord
	var err error
	if clientSecret != "" || clientID != "" {
		client, err = p.ValidateClient(r.Context(), clientID, clientSecret)
		if err != nil {
			jsonError(w, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
			return
		}
	}

	switch grantType {
	case "authorization_code":
		if client == nil {
			jsonError(w, http.StatusUnauthorized, "invalid_client", "Client authentication required")
			return
		}
		p.handleAuthCodeGrant(w, r, client)
	case "client_credentials":
		if client == nil {
			jsonError(w, http.StatusUnauthorized, "invalid_client", "Client authentication required")
			return
		}
		p.handleClientCredentialsGrant(w, r, client)
	case "refresh_token":
		if client == nil {
			jsonError(w, http.StatusUnauthorized, "invalid_client", "Client authentication required")
			return
		}
		p.handleRefreshTokenGrant(w, r, client)
	case "urn:ietf:params:oauth:grant-type:device_code":
		if client == nil {
			jsonError(w, http.StatusUnauthorized, "invalid_client", "Client authentication required")
			return
		}
		p.handleDeviceCodeGrant(w, r, client)
	default:
		jsonError(w, http.StatusBadRequest, "unsupported_grant_type", grantType+" not supported")
	}
}

func (p *Provider) handleAuthCodeGrant(w http.ResponseWriter, r *http.Request, client *ClientRecord) {
	code := r.FormValue("code")
	redirectURI := r.FormValue("redirect_uri")
	codeVerifier := r.FormValue("code_verifier")

	if code == "" {
		jsonError(w, http.StatusBadRequest, "invalid_grant", "code required")
		return
	}

	entry, err := p.ConsumeAuthCode(r.Context(), code, client.ClientID, redirectURI, codeVerifier)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid_grant", err.Error())
		return
	}

	// Issue tokens
	accessToken, _ := p.SignAccessToken(entry.UserID, client.ClientID, strings.Join(entry.Scope, " "))
	refreshToken, _ := p.SignRefreshToken(entry.UserID, client.ClientID, strings.Join(entry.Scope, " "))

	// Get user claims for ID token
	claims := p.GetUserClaims(r.Context(), entry.UserID)
	idToken, _ := p.SignIDToken(entry.UserID, client.ClientID, entry.Nonce, claims)

	// Store refresh token
	if err := p.StoreRefreshToken(r.Context(), refreshToken, client.ClientID, entry.UserID, entry.Scope); err != nil {
		jsonError(w, http.StatusInternalServerError, "server_error", "Failed to store refresh token")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"access_token":  accessToken,
		"token_type":    "Bearer",
		"expires_in":    int(p.accessTTL.Seconds()),
		"refresh_token": refreshToken,
		"id_token":      idToken,
		"scope":         strings.Join(entry.Scope, " "),
	})
}

func (p *Provider) handleClientCredentialsGrant(w http.ResponseWriter, r *http.Request, client *ClientRecord) {
	scope := r.FormValue("scope")
	if scope == "" {
		scope = "api"
	}

	sub := "client:" + client.ClientID
	accessToken, _ := p.SignAccessToken(sub, client.ClientID, scope)

	jsonResponse(w, http.StatusOK, map[string]any{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   int(p.accessTTL.Seconds()),
		"scope":        scope,
	})
}

func (p *Provider) handleRefreshTokenGrant(w http.ResponseWriter, r *http.Request, client *ClientRecord) {
	refreshToken := r.FormValue("refresh_token")
	scope := r.FormValue("scope")

	if refreshToken == "" {
		jsonError(w, http.StatusBadRequest, "invalid_grant", "refresh_token required")
		return
	}

	entry, err := p.ConsumeRefreshToken(r.Context(), refreshToken, client.ClientID)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid_grant", err.Error())
		return
	}

	// Revoke old refresh token (token rotation)
	p.RevokeRefreshToken(r.Context(), refreshToken)

	newScope := strings.Split(scope, " ")
	if len(newScope) == 1 && newScope[0] == "" {
		newScope = entry.Scope
	}

	accessToken, _ := p.SignAccessToken(entry.UserID, client.ClientID, strings.Join(newScope, " "))
	newRefreshToken, _ := p.SignRefreshToken(entry.UserID, client.ClientID, strings.Join(newScope, " "))
	p.StoreRefreshToken(r.Context(), newRefreshToken, client.ClientID, entry.UserID, newScope)

	jsonResponse(w, http.StatusOK, map[string]any{
		"access_token":  accessToken,
		"token_type":    "Bearer",
		"expires_in":    int(p.accessTTL.Seconds()),
		"refresh_token": newRefreshToken,
		"scope":         strings.Join(newScope, " "),
	})
}

func (p *Provider) handleDeviceCodeGrant(w http.ResponseWriter, r *http.Request, client *ClientRecord) {
	deviceCode := r.FormValue("device_code")
	if deviceCode == "" {
		jsonError(w, http.StatusBadRequest, "invalid_request", "device_code required")
		return
	}

	entry, err := p.PollDeviceCode(r.Context(), deviceCode, client.ClientID)
	if err != nil {
		if err.Error() == "authorization_pending" {
			jsonError(w, http.StatusBadRequest, "authorization_pending", "User hasn't authorized yet")
			return
		}
		jsonError(w, http.StatusBadRequest, "invalid_grant", err.Error())
		return
	}

	// Issue tokens
	accessToken, _ := p.SignAccessToken(*entry.UserID, client.ClientID, strings.Join(entry.Scope, " "))
	refreshToken, _ := p.SignRefreshToken(*entry.UserID, client.ClientID, strings.Join(entry.Scope, " "))

	claims := p.GetUserClaims(r.Context(), *entry.UserID)
	idToken, _ := p.SignIDToken(*entry.UserID, client.ClientID, "", claims)

	p.StoreRefreshToken(r.Context(), refreshToken, client.ClientID, *entry.UserID, entry.Scope)

	jsonResponse(w, http.StatusOK, map[string]any{
		"access_token":  accessToken,
		"token_type":    "Bearer",
		"expires_in":    int(p.accessTTL.Seconds()),
		"refresh_token": refreshToken,
		"id_token":      idToken,
		"scope":         strings.Join(entry.Scope, " "),
	})
}

// DeviceAuthorizationHandler handles the device authorization endpoint (RFC 8628).
// POST /device_authorization
func (p *Provider) DeviceAuthorizationHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid_request", "Could not parse form")
		return
	}

	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")
	scope := r.FormValue("scope")

	// Authenticate client
	client, err := p.ValidateClient(r.Context(), clientID, clientSecret)
	if err != nil {
		jsonError(w, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
		return
	}

	scopeParts := strings.Split(scope, " ")
	if len(scopeParts) == 1 && scopeParts[0] == "" {
		scopeParts = []string{"openid"}
	}

	entry, err := p.CreateDeviceCode(r.Context(), client.ClientID, scopeParts)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "server_error", "Failed to create device code")
		return
	}

	// Verification URI - use issuer as base
	verificationURI := p.issuer + "/device"
	verificationURIComplete := verificationURI + "?user_code=" + entry.UserCode

	jsonResponse(w, http.StatusOK, map[string]any{
		"device_code":              entry.DeviceCode,
		"user_code":                entry.UserCode,
		"verification_uri":         verificationURI,
		"verification_uri_complete": verificationURIComplete,
		"expires_in":               int(time.Until(entry.ExpiresAt).Seconds()),
		"interval":                 5,
	})
}

// DeviceVerificationHandler renders the device verification page.
// GET /device?user_code=XXXX-XXXX
func (p *Provider) DeviceVerificationHandler(w http.ResponseWriter, r *http.Request) {
	userCode := r.URL.Query().Get("user_code")

	if r.Method == "POST" {
		r.ParseForm()
		submittedCode := r.FormValue("user_code")
		email := r.FormValue("email")
		password := r.FormValue("password")

		if submittedCode != userCode {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, deviceHTML, userCode, "Invalid user code", userCode)
			return
		}

		userID, err := p.AuthenticatePassword(r.Context(), email, password)
		if err != nil {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, deviceHTML, userCode, "Invalid credentials", userCode)
			return
		}

		if err := p.ApproveDeviceCode(r.Context(), userCode, userID); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, deviceSuccessHTML, userCode)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, deviceHTML, userCode, "", userCode)
}

// IntrospectionHandler handles the OAuth2 introspection endpoint (RFC 7662).
// POST /introspect
func (p *Provider) IntrospectionHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid_request", "Could not parse form")
		return
	}

	token := r.FormValue("token")
	// Authenticate client (optional but recommended)
	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")
	if clientID != "" {
		if _, err := p.ValidateClient(r.Context(), clientID, clientSecret); err != nil {
			jsonError(w, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
			return
		}
	}

	if token == "" {
		jsonError(w, http.StatusBadRequest, "invalid_request", "token required")
		return
	}

	// Try parsing as access token first
	tok, claims, err := p.ParseToken(token)
	result := map[string]any{"active": false}

	if err == nil && tok.Valid {
		sub, _ := claims["sub"].(string)
		aud, _ := claims["aud"].(string)
		exp, _ := claims["exp"].(float64)
		scope, _ := claims["scope"].(string)
		tokenType, _ := claims["type"].(string)

		result = map[string]any{
			"active":    true,
			"sub":       sub,
			"aud":       aud,
			"exp":       exp,
			"scope":     scope,
			"token_type": tokenType,
			"iss":       claims["iss"],
			"iat":       claims["iat"],
			"jti":       claims["jti"],
		}

		// For refresh tokens, don't expose user claims
		if tokenType != "refresh" {
			userClaims := p.GetUserClaims(r.Context(), sub)
			for k, v := range userClaims {
				result[k] = v
			}
		}
	}

	jsonResponse(w, http.StatusOK, result)
}

// RevocationHandler handles the OAuth2 token revocation endpoint (RFC 7009).
// POST /revoke
func (p *Provider) RevocationHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid_request", "Could not parse form")
		return
	}

	token := r.FormValue("token")
	tokenTypeHint := r.FormValue("token_type_hint")

	// Authenticate client
	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")
	if clientID != "" {
		if _, err := p.ValidateClient(r.Context(), clientID, clientSecret); err != nil {
			// Per RFC 7009, we don't return error for invalid client on revocation
			// to prevent token enumeration attacks
		}
	}

	if token == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Try to parse token
	tok, claims, err := p.ParseToken(token)
	if err != nil || !tok.Valid {
		// Token might be a refresh token (opaque) - try to revoke by hash
		p.RevokeRefreshToken(r.Context(), token)
		w.WriteHeader(http.StatusOK)
		return
	}

	tokenType, _ := claims["type"].(string)
	if tokenType == "refresh" {
		p.RevokeRefreshToken(r.Context(), token)
	} else if tokenTypeHint == "refresh_token" {
		p.RevokeRefreshToken(r.Context(), token)
	}

	// For access tokens, we can't revoke them (they're stateless JWTs)
	// In production, you'd add to a revocation list / use short TTL
	w.WriteHeader(http.StatusOK)
}

// UserInfoHandler returns claims about the authenticated user.
// GET /userinfo with Authorization: Bearer <access_token>
func (p *Provider) UserInfoHandler(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		jsonError(w, http.StatusUnauthorized, "invalid_token", "Bearer token required")
		return
	}

	tokenStr := strings.TrimPrefix(auth, "Bearer ")
	_, claims, err := p.ParseToken(tokenStr)
	if err != nil {
		jsonError(w, http.StatusUnauthorized, "invalid_token", err.Error())
		return
	}

	userID, _ := claims["sub"].(string)
	userClaims := p.GetUserClaims(r.Context(), userID)
	if userClaims == nil {
		userClaims = map[string]any{}
	}
	userClaims["sub"] = userID

	jsonResponse(w, http.StatusOK, userClaims)
}

// ─── Helpers ─────────────────────────────────────────────────

func jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, status int, errCode, description string) {
	jsonResponse(w, status, map[string]string{
		"error":             errCode,
		"error_description": description,
	})
}

func buildRedirectURL(base string, params map[string]string) string {
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	q := u.Query()
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func isValidRedirectURI(client *ClientRecord, redirectURI string) bool {
	for _, uri := range client.RedirectURIs {
		if strings.HasPrefix(redirectURI, uri) {
			return true
		}
	}
	return false
}

// Minimal login form for browser-based flows
const loginHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>ObserveID — Sign In</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:system-ui,sans-serif;background:#050508;color:#F0EFEC;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0}
body::before{content:'';position:fixed;inset:0;background:linear-gradient(rgba(255,255,255,0.01)1px,transparent 1px),linear-gradient(90deg,rgba(255,255,255,0.01)1px,transparent 1px);background-size:64px 64px;pointer-events:none}
body::after{content:'';position:fixed;inset:0;background:radial-gradient(ellipse 60%% 40%% at 50%% -10%%,rgba(245,158,11,0.04),transparent 60%%),radial-gradient(ellipse 80%% 30%% at 50%% 110%%,rgba(245,158,11,0.03),transparent 50%%);pointer-events:none}
.card{background:rgba(12,12,16,0.85);backdrop-filter:blur(32px);border:1px solid rgba(255,255,255,0.06);border-radius:16px;padding:40px;width:100%%;max-width:400px;box-shadow:0 8px 32px rgba(0,0,0,0.5)}
h1{font-size:24px;font-weight:700;margin-bottom:4px}
h1 span{background:linear-gradient(135deg,#FBBF24,#F59E0B,#D97706);-webkit-background-clip:text;-webkit-text-fill-color:transparent}
p.subtitle{font-size:13px;color:#5C5C62;margin-bottom:24px}
label{display:block;font-size:11px;color:#5C5C62;text-transform:uppercase;letter-spacing:.08em;margin-bottom:4px}
input{width:100%%;padding:10px 12px;background:rgba(255,255,255,0.03);border:1px solid rgba(255,255,255,0.06);border-radius:8px;color:#F0EFEC;font-size:14px;margin-bottom:16px;outline:none;transition:all .2s}
input:focus{border-color:rgba(245,158,11,0.4);box-shadow:0 0 0 3px rgba(245,158,11,0.08)}
button{width:100%%;padding:12px;background:linear-gradient(135deg,rgba(245,158,11,0.15),rgba(217,119,6,0.2));border:1px solid rgba(245,158,11,0.25);border-radius:8px;color:#FBBF24;font-size:14px;font-weight:600;cursor:pointer;transition:all .25s}
button:hover{background:linear-gradient(135deg,rgba(245,158,11,0.25),rgba(217,119,6,0.35));box-shadow:0 0 20px rgba(245,158,11,0.08)}
.error{color:#EF4444;font-size:12px;margin-top:8px}
</style></head>
<body>
<div class="card">
<h1><span>ObserveID</span></h1>
<p class="subtitle">Identity Fabric — Sign In</p>
<form method="post" action="%s">
<input type="hidden" name="client_id" value="%s">
<input type="hidden" name="scope" value="%s">
<input type="hidden" name="state" value="%s">
<input type="hidden" name="code_challenge" value="%s">
<input type="hidden" name="code_challenge_method" value="%s">
<input type="hidden" name="response_type" value="%s">
<input type="hidden" name="nonce" value="%s">
<input type="hidden" name="redirect_uri" value="%s">
<label>Email</label><input name="email" type="email" placeholder="you@observeid.io" autofocus>
<label>Password</label><input name="password" type="password" placeholder="Your password">
<button type="submit">Sign In</button>
</form>
</div>
</body></html>
`

// Device authorization page
const deviceHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>ObserveID — Device Authorization</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:system-ui,sans-serif;background:#050508;color:#F0EFEC;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0}
body::before{content:'';position:fixed;inset:0;background:linear-gradient(rgba(255,255,255,0.01)1px,transparent 1px),linear-gradient(90deg,rgba(255,255,255,0.01)1px,transparent 1px);background-size:64px 64px;pointer-events:none}
body::after{content:'';position:fixed;inset:0;background:radial-gradient(ellipse 60%% 40%% at 50%% -10%%,rgba(245,158,11,0.04),transparent 60%%),radial-gradient(ellipse 80%% 30%% at 50%% 110%%,rgba(245,158,11,0.03),transparent 50%%);pointer-events:none}
.card{background:rgba(12,12,16,0.85);backdrop-filter:blur(32px);border:1px solid rgba(255,255,255,0.06);border-radius:16px;padding:40px;width:100%%;max-width:400px;box-shadow:0 8px 32px rgba(0,0,0,0.5)}
h1{font-size:24px;font-weight:700;margin-bottom:4px}
h1 span{background:linear-gradient(135deg,#FBBF24,#F59E0B,#D97706);-webkit-background-clip:text;-webkit-text-fill-color:transparent}
p.subtitle{font-size:13px;color:#5C5C62;margin-bottom:8px}
.code{font-family:'JetBrains Mono',monospace;font-size:28px;font-weight:700;color:#FBBF24;text-align:center;margin:16px 0;padding:12px;background:rgba(245,158,11,0.1);border-radius:8px;border:1px solid rgba(245,158,11,0.2);letter-spacing:4px}
label{display:block;font-size:11px;color:#5C5C62;text-transform:uppercase;letter-spacing:.08em;margin-bottom:4px}
input{width:100%%;padding:10px 12px;background:rgba(255,255,255,0.03);border:1px solid rgba(255,255,255,0.06);border-radius:8px;color:#F0EFEC;font-size:14px;margin-bottom:16px;outline:none;transition:all .2s}
input:focus{border-color:rgba(245,158,11,0.4);box-shadow:0 0 0 3px rgba(245,158,11,0.08)}
button{width:100%%;padding:12px;background:linear-gradient(135deg,rgba(245,158,11,0.15),rgba(217,119,6,0.2));border:1px solid rgba(245,158,11,0.25);border-radius:8px;color:#FBBF24;font-size:14px;font-weight:600;cursor:pointer;transition:all .25s}
button:hover{background:linear-gradient(135deg,rgba(245,158,11,0.25),rgba(217,119,6,0.35));box-shadow:0 0 20px rgba(245,158,11,0.08)}
.error{color:#EF4444;font-size:12px;margin-top:8px;text-align:center}
</style></head>
<body>
<div class="card">
<h1><span>ObserveID</span></h1>
<p class="subtitle">Enter the code shown on your device</p>
<div class="code">%s</div>
%s
<form method="post" action="">
<input type="hidden" name="user_code" value="%s">
<label>Email</label><input name="email" type="email" placeholder="you@observeid.io" autofocus>
<label>Password</label><input name="password" type="password" placeholder="Your password">
<button type="submit">Authorize</button>
</form>
</div>
</body></html>
`

// Device authorization success page
const deviceSuccessHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>ObserveID — Authorized</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:system-ui,sans-serif;background:#050508;color:#F0EFEC;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0}
body::before{content:'';position:fixed;inset:0;background:linear-gradient(rgba(255,255,255,0.01)1px,transparent 1px),linear-gradient(90deg,rgba(255,255,255,0.01)1px,transparent 1px);background-size:64px 64px;pointer-events:none}
body::after{content:'';position:fixed;inset:0;background:radial-gradient(ellipse 60%% 40%% at 50%% -10%%,rgba(245,158,11,0.04),transparent 60%%),radial-gradient(ellipse 80%% 30%% at 50%% 110%%,rgba(245,158,11,0.03),transparent 50%%);pointer-events:none}
.card{background:rgba(12,12,16,0.85);backdrop-filter:blur(32px);border:1px solid rgba(255,255,255,0.06);border-radius:16px;padding:40px;width:100%%;max-width:400px;box-shadow:0 8px 32px rgba(0,0,0,0.5);text-align:center}
h1{font-size:24px;font-weight:700;margin-bottom:4px}
h1 span{background:linear-gradient(135deg,#10B981,#34D399);-webkit-background-clip:text;-webkit-text-fill-color:transparent}
p{font-size:14px;color:#5C5C62;margin-top:16px}
.icon{font-size:64px;margin-bottom:16px}
</style></head>
<body>
<div class="card">
<div class="icon">✓</div>
<h1><span>Authorized</span></h1>
<p>You have successfully authorized device code <strong>%s</strong>.<br>You may now close this window and return to your device.</p>
</div>
</body></html>
`

// GenerateCode generates a cryptographically secure authorization code.
func GenerateCode() string {
	b := make([]byte, 32)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}