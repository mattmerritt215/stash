package auth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// unauthenticatedPrefixes are paths that must stay reachable without an
// identity, or the login flow cannot complete.
var unauthenticatedPrefixes = []string{
	"/oidc/",
	"/login",
	"/logout",
	"/assets",
	"/css",
	"/javascript",
	"/customlocales",
	"/favicon.ico",
	"/manifest.json",
}

func allowUnauthenticated(p string) bool {
	for _, prefix := range unauthenticatedPrefixes {
		if p == strings.TrimSuffix(prefix, "/") || strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// Middleware resolves the caller's identity and attaches it to the request
// context. Requests without an identity are redirected to the OIDC login
// endpoint (for navigations) or answered 401 (for API and asset requests).
//
// It is installed after stash's own authenticateHandler so that the legacy
// local login keeps working: when stash has already authenticated the local
// user, that identity is mapped onto the local administrator row.
//
// proxyPrefix should return the X-Forwarded-Prefix value for a request, so
// redirects work behind a subpath reverse proxy.
func (m *Manager) Middleware(proxyPrefix func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !m.Enabled() {
				next.ServeHTTP(w, r)
				return
			}

			u, err := m.Authenticate(w, r)
			switch {
			case err == nil:
				r = r.WithContext(WithUser(r.Context(), u))
				next.ServeHTTP(w, r)
				return

			case errors.Is(err, ErrUserDisabled):
				httpDeny(w, r, "your stash account is disabled", http.StatusForbidden)
				return

			case errors.Is(err, ErrNoIdentity):
				// fall through to the redirect/401 logic below

			default:
				httpDeny(w, r, err.Error(), http.StatusForbidden)
				return
			}

			if allowUnauthenticated(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			// If stash's own local login already authenticated this request,
			// adopt it as the local administrator. LocalSessionUser is set by
			// the patched authenticateHandler.
			if m.Config.LocalLogin && localSessionAuthenticated(r) {
				admin, lerr := m.Users.ByID(r.Context(), LocalAdminID)
				if lerr == nil {
					r = r.WithContext(WithUser(r.Context(), admin))
					next.ServeHTTP(w, r)
					return
				}
			}

			prefix := ""
			if proxyPrefix != nil {
				prefix = proxyPrefix(r)
			}

			// API-shaped requests get a status code, not a redirect.
			ext := path.Ext(r.URL.Path)
			isAPI := r.URL.Path == "/graphql" ||
				strings.Contains(r.Header.Get("Accept"), "application/json") ||
				(ext != "" && ext != ".html")
			if isAPI {
				w.Header().Set("WWW-Authenticate", `Bearer realm="stash"`)
				httpDeny(w, r, "authentication required", http.StatusUnauthorized)
				return
			}

			ret := url.URL{Path: prefix + r.URL.Path, RawQuery: r.URL.RawQuery}
			q := url.Values{}
			q.Set("returnURL", ret.String())
			http.Redirect(w, r, prefix+"/oidc/login?"+q.Encode(), http.StatusFound)
		})
	}
}

// RequirePermission wraps a handler so that it is only reachable by callers
// holding the given permission. Use it on non-GraphQL routes.
func RequirePermission(p Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !Can(r.Context(), p) {
				httpDeny(w, r, "not permitted", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func httpDeny(w http.ResponseWriter, r *http.Request, msg string, code int) {
	if strings.Contains(r.Header.Get("Accept"), "application/json") || r.URL.Path == "/graphql" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write([]byte(`{"errors":[{"message":` + quoteJSON(msg) + `}]}`))
		return
	}
	http.Error(w, msg, code)
}

func quoteJSON(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				continue
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// localSessionMarkerKey marks a request that stash's own session cookie (the
// built-in username/password login) already authenticated.
type localSessionMarkerKey struct{}

// MarkLocalSession records that stash's built-in login authenticated this
// request. The patched authenticateHandler calls this.
func MarkLocalSession(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), localSessionMarkerKey{}, true))
}

func localSessionAuthenticated(r *http.Request) bool {
	v, _ := r.Context().Value(localSessionMarkerKey{}).(bool)
	return v
}
