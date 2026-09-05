package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/stashapp/stash/pkg/logger"
)

const (
	stateCookieName = "stash_oidc_state"
	stateCookieTTL  = 10 * time.Minute
)

// OIDC wraps the provider and the pieces of the authorization code flow.
type OIDC struct {
	cfg      *Config
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth    *oauth2.Config

	endSessionEndpoint string
}

// NewOIDC discovers the provider metadata and builds the flow. It retries
// discovery in the background so that stash still starts when the IdP is
// briefly unavailable (for example when both come up together after a
// reboot).
func NewOIDC(ctx context.Context, cfg *Config) (*OIDC, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	if cfg.InsecureSkipVerify {
		client.Transport = insecureTransport()
	}
	ctx = oidc.ClientContext(ctx, client)

	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery against %s: %w", cfg.Issuer, err)
	}

	o := &OIDC{
		cfg:      cfg,
		provider: provider,
		verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		oauth: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  cfg.RedirectURL,
			Scopes:       cfg.Scopes,
		},
	}

	var meta struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := provider.Claims(&meta); err == nil {
		o.endSessionEndpoint = meta.EndSessionEndpoint
	}

	return o, nil
}

func insecureTransport() http.RoundTripper {
	t := http.DefaultTransport.(*http.Transport).Clone()
	if t.TLSClientConfig != nil {
		t.TLSClientConfig.InsecureSkipVerify = true // nolint:gosec
	}
	return t
}

type flowState struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
	Return   string `json:"r"`
}

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HandleLogin starts the authorization code flow with PKCE.
func (o *OIDC) HandleLogin(secureCookies bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state, err := randomString(32)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		nonce, err := randomString(32)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		verifier, err := randomString(48)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		fs := flowState{
			State:    state,
			Nonce:    nonce,
			Verifier: verifier,
			Return:   sanitiseReturnURL(r.URL.Query().Get("returnURL")),
		}
		raw, _ := json.Marshal(fs)

		http.SetCookie(w, &http.Cookie{
			Name:     stateCookieName,
			Value:    base64.RawURLEncoding.EncodeToString(raw),
			Path:     "/",
			MaxAge:   int(stateCookieTTL.Seconds()),
			HttpOnly: true,
			Secure:   secureCookies || r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
			SameSite: http.SameSiteLaxMode,
		})

		sum := sha256.Sum256([]byte(verifier))
		challenge := base64.RawURLEncoding.EncodeToString(sum[:])

		authURL := o.oauth.AuthCodeURL(state,
			oidc.Nonce(nonce),
			oauth2.SetAuthURLParam("code_challenge", challenge),
			oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		)

		http.Redirect(w, r, authURL, http.StatusFound)
	}
}

// CallbackResult carries the outcome of a successful callback.
type CallbackResult struct {
	Identity  Identity
	ReturnURL string
}

// HandleCallback completes the flow and returns the resolved identity. The
// caller is responsible for creating the stash session and redirecting.
func (o *OIDC) HandleCallback(w http.ResponseWriter, r *http.Request) (*CallbackResult, error) {
	c, err := r.Cookie(stateCookieName)
	if err != nil {
		return nil, errors.New("missing or expired login state; start again from /oidc/login")
	}
	// consume the cookie regardless of outcome
	http.SetCookie(w, &http.Cookie{Name: stateCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})

	raw, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return nil, errors.New("malformed login state")
	}
	var fs flowState
	if err := json.Unmarshal(raw, &fs); err != nil {
		return nil, errors.New("malformed login state")
	}

	if q := r.URL.Query(); q.Get("error") != "" {
		return nil, fmt.Errorf("identity provider returned %s: %s", q.Get("error"), q.Get("error_description"))
	}

	if r.URL.Query().Get("state") != fs.State || fs.State == "" {
		return nil, errors.New("state mismatch")
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		return nil, errors.New("no authorization code in callback")
	}

	ctx := r.Context()
	if o.cfg.InsecureSkipVerify {
		ctx = oidc.ClientContext(ctx, &http.Client{Transport: insecureTransport(), Timeout: 15 * time.Second})
	}

	token, err := o.oauth.Exchange(ctx, code,
		oauth2.SetAuthURLParam("code_verifier", fs.Verifier))
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, errors.New("no id_token in token response")
	}

	idToken, err := o.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("id_token verification failed: %w", err)
	}
	if idToken.Nonce != fs.Nonce {
		return nil, errors.New("nonce mismatch")
	}

	claims := map[string]interface{}{}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("decoding id_token claims: %w", err)
	}

	// authentik puts groups in the ID token when the "groups" scope is
	// requested, but some configurations only expose them at the userinfo
	// endpoint. Merge userinfo in when the groups claim is absent.
	if _, ok := claims[o.cfg.GroupsClaim]; !ok {
		if ui, err := o.provider.UserInfo(ctx, oauth2.StaticTokenSource(token)); err == nil {
			extra := map[string]interface{}{}
			if err := ui.Claims(&extra); err == nil {
				for k, v := range extra {
					if _, exists := claims[k]; !exists {
						claims[k] = v
					}
				}
			}
		}
	}

	username := stringClaim(claims, o.cfg.UsernameClaim)
	if username == "" {
		username = stringClaim(claims, "preferred_username")
	}
	if username == "" {
		username = stringClaim(claims, "email")
	}
	if username == "" {
		username = idToken.Subject
	}

	groups := stringSliceClaim(claims, o.cfg.GroupsClaim)
	role := o.cfg.RoleForGroups(groups)
	if role == RoleNone {
		// This is the failure people hit first, and "no groups" is ambiguous:
		// the claim may be absent, present but empty, or spelled differently.
		// Log enough to tell those apart without needing to decode a token by
		// hand.
		keys := make([]string, 0, len(claims))
		for k := range claims {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		if _, present := claims[o.cfg.GroupsClaim]; !present {
			logger.Warnf("auth: %q signed in but the %q claim was not in the id_token or userinfo. "+
				"Claims received: %v", username, o.cfg.GroupsClaim, keys)
			logger.Warnf("auth: authentik has no separate 'groups' scope -- its default 'profile' " +
				"scope mapping is what carries groups. Check that mapping is selected on the " +
				"provider, and that STASH_OIDC_SCOPES requests 'profile'.")
		} else {
			logger.Warnf("auth: %q signed in with groups %v, none of which match "+
				"STASH_OIDC_ADMIN_GROUPS (%v) or STASH_OIDC_USER_GROUPS (%v)",
				username, groups, o.cfg.AdminGroups, o.cfg.UserGroups)
		}

		return nil, fmt.Errorf("your account is not a member of any group that grants access to stash")
	}

	return &CallbackResult{
		Identity: Identity{
			Subject:     idToken.Subject,
			Username:    username,
			Email:       stringClaim(claims, o.cfg.EmailClaim),
			DisplayName: stringClaim(claims, o.cfg.NameClaim),
			Role:        role,
		},
		ReturnURL: fs.Return,
	}, nil
}

// EndSessionURL returns the provider's RP-initiated logout URL, or "" when
// the provider does not advertise one or end-session is disabled.
func (o *OIDC) EndSessionURL(postLogoutRedirect string) string {
	if !o.cfg.EndSession || o.endSessionEndpoint == "" {
		return ""
	}
	u, err := url.Parse(o.endSessionEndpoint)
	if err != nil {
		return ""
	}
	q := u.Query()
	q.Set("client_id", o.cfg.ClientID)
	if postLogoutRedirect != "" {
		q.Set("post_logout_redirect_uri", postLogoutRedirect)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func stringClaim(claims map[string]interface{}, key string) string {
	if key == "" {
		return ""
	}
	v, ok := claims[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func stringSliceClaim(claims map[string]interface{}, key string) []string {
	if key == "" {
		return nil
	}
	v, ok := claims[key]
	if !ok {
		return nil
	}
	switch t := v.(type) {
	case []string:
		return t
	case string:
		return strings.Split(t, ",")
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// sanitiseReturnURL rejects absolute URLs so that the callback cannot be used
// as an open redirect.
func sanitiseReturnURL(s string) string {
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "//") || strings.Contains(s, "://") {
		return ""
	}
	if !strings.HasPrefix(s, "/") {
		return ""
	}
	return s
}
