package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"

	"github.com/stashapp/stash/pkg/logger"
)

type ctxKey int

const (
	ctxUser ctxKey = iota
)

const userCookieName = "stash_user"
const userSessionKey = "uid"

// Manager is the process-wide entry point for the multi-user layer.
type Manager struct {
	Config *Config
	Users  *Store
	OIDC   *OIDC

	sessions *sessions.CookieStore
}

// NewManager builds the auth manager. sessionKey should be the same key stash
// uses for its own session cookie so that rotating one rotates both.
//
// When OIDC discovery fails at startup the manager is still returned, with
// OIDC nil; discovery is retried in the background. This keeps stash bootable
// when authentik is slower to come up than stash is.
func NewManager(ctx context.Context, cfg *Config, db *sqlx.DB, sessionKey []byte, localAdminUsername string) (*Manager, error) {
	m := &Manager{
		Config: cfg,
		Users:  NewStore(db),
	}

	if !cfg.Enabled() {
		return m, nil
	}

	if err := m.Users.EnsureLocalAdmin(ctx, localAdminUsername); err != nil {
		return nil, err
	}

	store := sessions.NewCookieStore(sessionKey)
	store.MaxAge(cfg.SessionMaxAge)
	store.Options.Path = "/"
	store.Options.HttpOnly = true
	store.Options.SameSite = http.SameSiteLaxMode
	m.sessions = store

	if cfg.Mode == ModeOIDC || cfg.Mode == ModeHybrid {
		o, err := NewOIDC(ctx, cfg)
		if err != nil {
			logger.Errorf("auth: %v — retrying discovery in the background", err)
			go m.retryDiscovery(cfg)
		} else {
			m.OIDC = o
			logger.Infof("auth: OIDC provider %s ready", cfg.Issuer)
		}
	}

	return m, nil
}

func (m *Manager) retryDiscovery(cfg *Config) {
	for attempt := 1; ; attempt++ {
		delay := time.Duration(attempt) * 5 * time.Second
		if delay > 2*time.Minute {
			delay = 2 * time.Minute
		}
		time.Sleep(delay)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		o, err := NewOIDC(ctx, cfg)
		cancel()
		if err != nil {
			logger.Debugf("auth: OIDC discovery retry %d failed: %v", attempt, err)
			continue
		}
		m.OIDC = o
		logger.Infof("auth: OIDC provider %s ready after %d attempts", cfg.Issuer, attempt)
		return
	}
}

// Enabled reports whether the multi-user layer is active.
func (m *Manager) Enabled() bool {
	return m != nil && m.Config.Enabled()
}

// ---------------------------------------------------------------------------
// context plumbing
// ---------------------------------------------------------------------------

// WithUser attaches a user to the context.
func WithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, ctxUser, u)
}

// FromContext returns the authenticated user, or nil.
func FromContext(ctx context.Context) *User {
	u, _ := ctx.Value(ctxUser).(*User)
	return u
}

// UserID returns the authenticated user's id, or LocalAdminID when the
// multi-user layer is disabled or no user is attached. Per-user state code
// uses this so that a single-user install keeps working unchanged.
func UserID(ctx context.Context) int {
	if u := FromContext(ctx); u != nil {
		return u.ID
	}
	return LocalAdminID
}

// Can reports whether the context's user holds the permission. With no user
// attached — internal tasks, CLI, plugin hooks — it returns true, because
// those paths are not user-facing requests.
func Can(ctx context.Context, p Permission) bool {
	u := FromContext(ctx)
	if u == nil {
		return true
	}
	return u.Can(p)
}

// ---------------------------------------------------------------------------
// authentication
// ---------------------------------------------------------------------------

// ErrNoIdentity means the request carried no usable credentials.
var ErrNoIdentity = errors.New("no identity")

// Authenticate resolves the user for a request. Resolution order:
//
//  1. trusted reverse-proxy identity headers (header/hybrid mode only)
//  2. personal API key (ApiKey header or ?apikey=)
//  3. the stash_user session cookie
//
// It returns ErrNoIdentity when none apply.
func (m *Manager) Authenticate(w http.ResponseWriter, r *http.Request) (*User, error) {
	if !m.Enabled() {
		return nil, ErrNoIdentity
	}
	ctx := r.Context()

	if m.Config.HeaderEnabled && m.Config.TrustsProxy(r.RemoteAddr) {
		if u, err := m.fromHeaders(ctx, r); err == nil {
			return u, nil
		} else if !errors.Is(err, ErrNoIdentity) {
			return nil, err
		}
	}

	if key := apiKeyFromRequest(r); key != "" {
		u, err := m.Users.ByAPIKey(ctx, key)
		if err == nil {
			return u, nil
		}
		if !errors.Is(err, ErrUserNotFound) {
			return nil, err
		}
		// fall through: it may be the legacy global API key, which stash's
		// own session store still handles.
	}

	return m.fromSession(w, r)
}

func apiKeyFromRequest(r *http.Request) string {
	if k := r.Header.Get("ApiKey"); k != "" {
		return k
	}
	if k := r.URL.Query().Get("apikey"); k != "" {
		return k
	}
	if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(a, "Bearer "))
	}
	return ""
}

func (m *Manager) fromHeaders(ctx context.Context, r *http.Request) (*User, error) {
	username := strings.TrimSpace(r.Header.Get(m.Config.HeaderUser))
	if username == "" {
		return nil, ErrNoIdentity
	}

	rawGroups := r.Header.Get(m.Config.HeaderGroups)
	var groups []string
	if rawGroups != "" {
		sep := m.Config.HeaderGroupSep
		if sep == "" {
			sep = "|"
		}
		for _, g := range strings.Split(rawGroups, sep) {
			if g = strings.TrimSpace(g); g != "" {
				groups = append(groups, g)
			}
		}
	}

	role := m.Config.RoleForGroups(groups)
	if role == RoleNone {
		return nil, fmt.Errorf("proxy-authenticated user %q matched no stash group", username)
	}

	return m.Users.Upsert(ctx, Identity{
		Username:    username,
		Email:       strings.TrimSpace(r.Header.Get(m.Config.HeaderEmail)),
		DisplayName: strings.TrimSpace(r.Header.Get(m.Config.HeaderName)),
		Role:        role,
	}, m.Config.AutoCreate)
}

func (m *Manager) fromSession(w http.ResponseWriter, r *http.Request) (*User, error) {
	if m.sessions == nil {
		return nil, ErrNoIdentity
	}
	sess, err := m.sessions.Get(r, userCookieName)
	if err != nil || sess.IsNew {
		return nil, ErrNoIdentity
	}
	id, ok := sess.Values[userSessionKey].(int)
	if !ok || id == 0 {
		return nil, ErrNoIdentity
	}

	u, err := m.Users.ByID(r.Context(), id)
	if err != nil {
		// stale cookie referring to a deleted user
		m.ClearSession(w, r)
		return nil, ErrNoIdentity
	}
	if u.Disabled {
		m.ClearSession(w, r)
		return nil, ErrUserDisabled
	}

	// slide the expiry
	_ = sess.Save(r, w)
	return u, nil
}

// StartSession writes the session cookie for the given user.
func (m *Manager) StartSession(w http.ResponseWriter, r *http.Request, u *User) error {
	if m.sessions == nil {
		return errors.New("session store not initialised")
	}
	sess, _ := m.sessions.Get(r, userCookieName)
	sess.Values[userSessionKey] = u.ID
	sess.Options.Secure = r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	return sess.Save(r, w)
}

// ClearSession removes the session cookie.
func (m *Manager) ClearSession(w http.ResponseWriter, r *http.Request) {
	if m.sessions == nil {
		return
	}
	sess, _ := m.sessions.Get(r, userCookieName)
	delete(sess.Values, userSessionKey)
	sess.Options.MaxAge = -1
	_ = sess.Save(r, w)
}

// ---------------------------------------------------------------------------
// routes
// ---------------------------------------------------------------------------

// Routes returns the OIDC endpoints, to be mounted at /oidc.
func (m *Manager) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/login", func(w http.ResponseWriter, req *http.Request) {
		if m.OIDC == nil {
			http.Error(w, "OIDC is not configured or the provider is unreachable", http.StatusServiceUnavailable)
			return
		}
		m.OIDC.HandleLogin(false)(w, req)
	})

	r.Get("/callback", func(w http.ResponseWriter, req *http.Request) {
		if m.OIDC == nil {
			http.Error(w, "OIDC is not configured", http.StatusServiceUnavailable)
			return
		}
		res, err := m.OIDC.HandleCallback(w, req)
		if err != nil {
			logger.Warnf("auth: OIDC callback rejected: %v", err)
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}

		u, err := m.Users.Upsert(req.Context(), res.Identity, m.Config.AutoCreate)
		if err != nil {
			logger.Errorf("auth: could not provision %q: %v", res.Identity.Username, err)
			http.Error(w, "your account could not be provisioned in stash", http.StatusForbidden)
			return
		}

		if err := m.StartSession(w, req, u); err != nil {
			http.Error(w, "could not start session", http.StatusInternalServerError)
			return
		}

		logger.Infof("auth: %s signed in as %s", u.Username, u.Role)

		dest := res.ReturnURL
		if dest == "" {
			dest = "/"
		}
		http.Redirect(w, req, dest, http.StatusFound)
	})

	r.Get("/logout", func(w http.ResponseWriter, req *http.Request) {
		m.ClearSession(w, req)
		if m.OIDC != nil {
			if u := m.OIDC.EndSessionURL(""); u != "" {
				http.Redirect(w, req, u, http.StatusFound)
				return
			}
		}
		http.Redirect(w, req, "/", http.StatusFound)
	})

	return r
}
