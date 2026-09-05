// Package auth provides multi-user identity, OIDC single sign-on and
// role-based permission enforcement for stash.
//
// Configuration is read from the process environment (systemd
// EnvironmentFile=/etc/stash/env) rather than from stash's config.yml, so that
// upstream's config schema is left untouched and secrets never land in the
// stash data directory.
package auth

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// Mode determines how a request's identity is established.
type Mode string

const (
	// ModeDisabled turns the multi-user layer off entirely. Stash behaves as
	// upstream does: a single implicit admin.
	ModeDisabled Mode = "disabled"
	// ModeOIDC authenticates browsers via the OIDC authorization code flow.
	ModeOIDC Mode = "oidc"
	// ModeHeader trusts identity headers set by an upstream reverse proxy.
	ModeHeader Mode = "header"
	// ModeHybrid accepts either; trusted-proxy headers are checked first and
	// OIDC is used for anything that arrives without them.
	ModeHybrid Mode = "hybrid"
)

// Config is the fully-resolved auth configuration for the process.
type Config struct {
	Mode Mode

	// OIDC
	Issuer             string
	ClientID           string
	ClientSecret       string
	RedirectURL        string
	Scopes             []string
	UsernameClaim      string
	EmailClaim         string
	NameClaim          string
	GroupsClaim        string
	AdminGroups        []string
	UserGroups         []string
	AutoCreate         bool
	EndSession         bool
	InsecureSkipVerify bool

	// Trusted-proxy header mode
	HeaderEnabled  bool
	HeaderUser     string
	HeaderGroups   string
	HeaderEmail    string
	HeaderName     string
	HeaderGroupSep string
	TrustedProxies []*net.IPNet

	// LocalLogin keeps upstream's username/password form available as a
	// break-glass path. The local user is always an admin.
	LocalLogin bool

	// DefaultRole is assigned to a user that authenticates successfully but
	// matches neither AdminGroups nor UserGroups. Empty means "reject".
	DefaultRole Role

	// SessionMaxAge is the cookie lifetime in seconds for OIDC sessions.
	SessionMaxAge int
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return b
}

func envInt(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	i, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return i
}

func envList(key string, def []string) []string {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	parts := strings.FieldsFunc(v, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}

// LoadConfig resolves the auth configuration from the environment and
// validates it. It returns an error only for configurations that cannot
// possibly work; anything recoverable is defaulted.
func LoadConfig() (*Config, error) {
	c := &Config{
		Mode:               Mode(strings.ToLower(env("STASH_AUTH_MODE", string(ModeDisabled)))),
		// Do NOT normalise trailing slashes here. go-oidc's discovery requires
		// cfg.Issuer to match byte-for-byte the "issuer" field the provider's
		// well-known document returns. Authentik's always carries a trailing
		// slash; other IdPs may not. Whatever the admin puts in
		// STASH_OIDC_ISSUER is passed through unchanged -- getting that value
		// right for their provider is their job, and silently rewriting it out
		// from under them (as this used to do) makes a correct config look
		// broken.
		Issuer:             env("STASH_OIDC_ISSUER", ""),
		ClientID:           env("STASH_OIDC_CLIENT_ID", ""),
		ClientSecret:       env("STASH_OIDC_CLIENT_SECRET", ""),
		RedirectURL:        env("STASH_OIDC_REDIRECT_URL", ""),
		Scopes:             envList("STASH_OIDC_SCOPES", []string{"openid", "profile", "email", "groups"}),
		UsernameClaim:      env("STASH_OIDC_USERNAME_CLAIM", "preferred_username"),
		EmailClaim:         env("STASH_OIDC_EMAIL_CLAIM", "email"),
		NameClaim:          env("STASH_OIDC_NAME_CLAIM", "name"),
		GroupsClaim:        env("STASH_OIDC_GROUPS_CLAIM", "groups"),
		AdminGroups:        envList("STASH_OIDC_ADMIN_GROUPS", []string{"stash-admins"}),
		UserGroups:         envList("STASH_OIDC_USER_GROUPS", []string{"stash-users"}),
		AutoCreate:         envBool("STASH_OIDC_AUTO_CREATE", true),
		EndSession:         envBool("STASH_OIDC_END_SESSION", true),
		InsecureSkipVerify: envBool("STASH_OIDC_INSECURE_SKIP_VERIFY", false),

		HeaderEnabled:  envBool("STASH_AUTH_HEADER_ENABLED", false),
		HeaderUser:     env("STASH_AUTH_HEADER_USER", "X-Authentik-Username"),
		HeaderGroups:   env("STASH_AUTH_HEADER_GROUPS", "X-Authentik-Groups"),
		HeaderEmail:    env("STASH_AUTH_HEADER_EMAIL", "X-Authentik-Email"),
		HeaderName:     env("STASH_AUTH_HEADER_NAME", "X-Authentik-Name"),
		HeaderGroupSep: env("STASH_AUTH_HEADER_GROUP_SEPARATOR", "|"),

		LocalLogin:    envBool("STASH_AUTH_LOCAL_LOGIN", true),
		SessionMaxAge: envInt("STASH_AUTH_SESSION_MAX_AGE", 60*60*24*7),
	}

	switch strings.ToLower(env("STASH_AUTH_DEFAULT_ROLE", "")) {
	case string(RoleAdmin):
		c.DefaultRole = RoleAdmin
	case string(RoleUser):
		c.DefaultRole = RoleUser
	case string(RoleGuest):
		c.DefaultRole = RoleGuest
	default:
		c.DefaultRole = RoleNone
	}

	switch c.Mode {
	case ModeDisabled:
		return c, nil
	case ModeOIDC, ModeHybrid:
		if c.Issuer == "" || c.ClientID == "" || c.ClientSecret == "" {
			return nil, fmt.Errorf("auth mode %q requires STASH_OIDC_ISSUER, STASH_OIDC_CLIENT_ID and STASH_OIDC_CLIENT_SECRET", c.Mode)
		}
		if c.RedirectURL == "" {
			return nil, fmt.Errorf("auth mode %q requires STASH_OIDC_REDIRECT_URL", c.Mode)
		}
	case ModeHeader:
		c.HeaderEnabled = true
	default:
		return nil, fmt.Errorf("unknown STASH_AUTH_MODE %q (want disabled|oidc|header|hybrid)", c.Mode)
	}

	if c.Mode == ModeHybrid || c.Mode == ModeHeader {
		c.HeaderEnabled = c.HeaderEnabled || c.Mode == ModeHeader
	}

	if c.HeaderEnabled {
		raw := envList("STASH_AUTH_TRUSTED_PROXIES", nil)
		if len(raw) == 0 {
			return nil, fmt.Errorf("header auth is enabled but STASH_AUTH_TRUSTED_PROXIES is empty; refusing to trust identity headers from arbitrary peers")
		}
		for _, entry := range raw {
			n, err := parseCIDROrIP(entry)
			if err != nil {
				return nil, fmt.Errorf("STASH_AUTH_TRUSTED_PROXIES: %w", err)
			}
			c.TrustedProxies = append(c.TrustedProxies, n)
		}
	}

	return c, nil
}

func parseCIDROrIP(s string) (*net.IPNet, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "/") {
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", s, err)
		}
		return n, nil
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP %q", s)
	}
	bits := 32
	if ip.To4() == nil {
		bits = 128
	} else {
		ip = ip.To4()
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}, nil
}

// Enabled reports whether the multi-user layer is active.
func (c *Config) Enabled() bool {
	return c != nil && c.Mode != ModeDisabled
}

// TrustsProxy reports whether identity headers from remoteAddr may be trusted.
func (c *Config) TrustsProxy(remoteAddr string) bool {
	if !c.HeaderEnabled {
		return false
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return false
	}
	for _, n := range c.TrustedProxies {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// RoleForGroups maps a set of IdP group names onto a stash role.
func (c *Config) RoleForGroups(groups []string) Role {
	has := func(want []string) bool {
		for _, w := range want {
			for _, g := range groups {
				if strings.EqualFold(strings.TrimSpace(g), w) {
					return true
				}
			}
		}
		return false
	}

	if has(c.AdminGroups) {
		return RoleAdmin
	}
	if has(c.UserGroups) {
		return RoleUser
	}
	return c.DefaultRole
}
