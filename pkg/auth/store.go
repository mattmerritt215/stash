package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
)

// ErrUserNotFound is returned when no user matches the lookup.
var ErrUserNotFound = errors.New("user not found")

// ErrUserDisabled is returned when a matching user exists but is disabled.
var ErrUserDisabled = errors.New("user is disabled")

// LocalAdminID is the row id of the seeded local administrator. It owns all
// pre-existing per-user state migrated from the single-user schema.
const LocalAdminID = 1

// User is a stash identity.
type User struct {
	ID          int        `db:"id"`
	Username    string     `db:"username"`
	Subject     *string    `db:"subject"`
	Email       *string    `db:"email"`
	DisplayName *string    `db:"display_name"`
	Role        string     `db:"role"`
	Disabled    bool       `db:"disabled"`
	APIKeyHash  *string    `db:"api_key_hash"`
	UIConfig    string     `db:"ui_config"`
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"`
	LastLoginAt *time.Time `db:"last_login_at"`
}

// GetRole returns the parsed role, defaulting to RoleNone for junk values.
func (u *User) GetRole() Role {
	if u == nil {
		return RoleNone
	}
	return ParseRole(u.Role)
}

// Can reports whether the user holds the permission. A nil or disabled user
// holds nothing.
func (u *User) Can(p Permission) bool {
	if u == nil || u.Disabled {
		return false
	}
	return u.GetRole().Can(p)
}

// Name is the best available human-readable label for the user.
func (u *User) Name() string {
	if u == nil {
		return ""
	}
	if u.DisplayName != nil && *u.DisplayName != "" {
		return *u.DisplayName
	}
	return u.Username
}

// UI decodes the user's personal UI configuration map.
func (u *User) UI() map[string]interface{} {
	out := map[string]interface{}{}
	if u == nil || strings.TrimSpace(u.UIConfig) == "" {
		return out
	}
	if err := json.Unmarshal([]byte(u.UIConfig), &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

// Store is the persistence layer for users. It holds its own handle on the
// stash database so that it can be constructed before the main repository is
// wired up.
type Store struct {
	db *sqlx.DB

	mu    sync.RWMutex
	cache map[string]*User // keyed by "sub:<subject>" and "user:<username>"
}

// NewStore returns a Store backed by the supplied database handle.
func NewStore(db *sqlx.DB) *Store {
	return &Store{db: db, cache: map[string]*User{}}
}

const userColumns = `id, username, subject, email, display_name, role, disabled,
	api_key_hash, ui_config, created_at, updated_at, last_login_at`

func (s *Store) invalidate(u *User) {
	if u == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cache, "user:"+strings.ToLower(u.Username))
	if u.Subject != nil {
		delete(s.cache, "sub:"+*u.Subject)
	}
}

func (s *Store) cached(key string) *User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache[key]
}

func (s *Store) store(key string, u *User) *User {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[key] = u
	return u
}

// ByID looks a user up by row id.
func (s *Store) ByID(ctx context.Context, id int) (*User, error) {
	var u User
	err := s.db.GetContext(ctx, &u, `SELECT `+userColumns+` FROM users WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// ByUsername looks a user up by username, case-insensitively.
func (s *Store) ByUsername(ctx context.Context, username string) (*User, error) {
	key := "user:" + strings.ToLower(username)
	if u := s.cached(key); u != nil {
		return u, nil
	}
	var u User
	err := s.db.GetContext(ctx, &u,
		`SELECT `+userColumns+` FROM users WHERE username = ? COLLATE NOCASE`, username)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.store(key, &u), nil
}

// BySubject looks a user up by OIDC subject identifier.
func (s *Store) BySubject(ctx context.Context, subject string) (*User, error) {
	key := "sub:" + subject
	if u := s.cached(key); u != nil {
		return u, nil
	}
	var u User
	err := s.db.GetContext(ctx, &u,
		`SELECT `+userColumns+` FROM users WHERE subject = ?`, subject)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.store(key, &u), nil
}

// List returns every user, ordered by username.
func (s *Store) List(ctx context.Context) ([]*User, error) {
	var out []*User
	err := s.db.SelectContext(ctx, &out,
		`SELECT `+userColumns+` FROM users ORDER BY username COLLATE NOCASE`)
	return out, err
}

// Identity is the set of claims resolved from an authentication event.
type Identity struct {
	Subject     string
	Username    string
	Email       string
	DisplayName string
	Role        Role
}

// Upsert reconciles an authenticated identity with the users table. When
// autoCreate is false and no user exists, ErrUserNotFound is returned.
//
// Role is refreshed on every login so that group changes in the IdP take
// effect at the next sign-in. The local administrator (id 1) is never
// demoted, so that a broken group mapping cannot lock you out.
func (s *Store) Upsert(ctx context.Context, id Identity, autoCreate bool) (*User, error) {
	if id.Username == "" {
		return nil, fmt.Errorf("identity has no username")
	}

	now := time.Now().UTC()

	var existing *User
	var err error
	if id.Subject != "" {
		existing, err = s.BySubject(ctx, id.Subject)
		if err != nil && !errors.Is(err, ErrUserNotFound) {
			return nil, err
		}
	}
	if existing == nil {
		existing, err = s.ByUsername(ctx, id.Username)
		if err != nil && !errors.Is(err, ErrUserNotFound) {
			return nil, err
		}
	}

	if existing == nil {
		if !autoCreate {
			return nil, ErrUserNotFound
		}
		if id.Role == RoleNone {
			return nil, fmt.Errorf("refusing to create user %q with no role: check group mapping", id.Username)
		}
		res, err := s.db.ExecContext(ctx, `
			INSERT INTO users
				(username, subject, email, display_name, role, disabled, ui_config, created_at, updated_at, last_login_at)
			VALUES (?, ?, ?, ?, ?, 0, '{}', ?, ?, ?)`,
			id.Username, nullable(id.Subject), nullable(id.Email), nullable(id.DisplayName),
			string(id.Role), now, now, now)
		if err != nil {
			return nil, fmt.Errorf("creating user %q: %w", id.Username, err)
		}
		newID, _ := res.LastInsertId()
		return s.ByID(ctx, int(newID))
	}

	role := existing.Role
	if id.Role != RoleNone && existing.ID != LocalAdminID {
		role = string(id.Role)
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE users SET
			username = ?,
			subject = COALESCE(?, subject),
			email = COALESCE(?, email),
			display_name = COALESCE(?, display_name),
			role = ?,
			updated_at = ?,
			last_login_at = ?
		WHERE id = ?`,
		id.Username, nullable(id.Subject), nullable(id.Email), nullable(id.DisplayName),
		role, now, now, existing.ID)
	if err != nil {
		return nil, fmt.Errorf("updating user %d: %w", existing.ID, err)
	}

	s.invalidate(existing)
	updated, err := s.ByID(ctx, existing.ID)
	if err != nil {
		return nil, err
	}
	if updated.Disabled {
		return nil, ErrUserDisabled
	}
	return updated, nil
}

// SetRole changes a user's role. The local administrator cannot be demoted.
func (s *Store) SetRole(ctx context.Context, id int, role Role) error {
	if id == LocalAdminID && role != RoleAdmin {
		return errors.New("the local administrator cannot be demoted")
	}
	if !role.Valid() {
		return fmt.Errorf("invalid role %q", role)
	}
	u, err := s.ByID(ctx, id)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE users SET role = ?, updated_at = ? WHERE id = ?`, string(role), time.Now().UTC(), id)
	s.invalidate(u)
	return err
}

// SetDisabled enables or disables a user. The local administrator cannot be
// disabled.
func (s *Store) SetDisabled(ctx context.Context, id int, disabled bool) error {
	if id == LocalAdminID && disabled {
		return errors.New("the local administrator cannot be disabled")
	}
	u, err := s.ByID(ctx, id)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE users SET disabled = ?, updated_at = ? WHERE id = ?`, disabled, time.Now().UTC(), id)
	s.invalidate(u)
	return err
}

// Destroy removes a user and, by foreign key cascade, all of their per-user
// state. The local administrator cannot be removed.
func (s *Store) Destroy(ctx context.Context, id int) error {
	if id == LocalAdminID {
		return errors.New("the local administrator cannot be deleted")
	}
	u, err := s.ByID(ctx, id)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	s.invalidate(u)
	return err
}

// SetUIConfig replaces a user's personal interface configuration.
func (s *Store) SetUIConfig(ctx context.Context, id int, cfg map[string]interface{}) error {
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	u, err := s.ByID(ctx, id)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE users SET ui_config = ?, updated_at = ? WHERE id = ?`, string(b), time.Now().UTC(), id)
	s.invalidate(u)
	return err
}

// GenerateAPIKey issues a fresh personal API key for the user, stores its
// SHA-256 hash and returns the plaintext key. The plaintext is never
// persisted and cannot be recovered afterwards.
func (s *Store) GenerateAPIKey(ctx context.Context, id int) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	key := fmt.Sprintf("stash_%d_%s", id, base64.RawURLEncoding.EncodeToString(raw))
	sum := sha256.Sum256([]byte(key))
	hash := hex.EncodeToString(sum[:])

	u, err := s.ByID(ctx, id)
	if err != nil {
		return "", err
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE users SET api_key_hash = ?, updated_at = ? WHERE id = ?`, hash, time.Now().UTC(), id); err != nil {
		return "", err
	}
	s.invalidate(u)
	return key, nil
}

// RevokeAPIKey clears the user's personal API key.
func (s *Store) RevokeAPIKey(ctx context.Context, id int) error {
	u, err := s.ByID(ctx, id)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE users SET api_key_hash = NULL, updated_at = ? WHERE id = ?`, time.Now().UTC(), id)
	s.invalidate(u)
	return err
}

// ByAPIKey resolves a plaintext API key to its owner in constant time with
// respect to the key value.
func (s *Store) ByAPIKey(ctx context.Context, key string) (*User, error) {
	if key == "" {
		return nil, ErrUserNotFound
	}
	sum := sha256.Sum256([]byte(key))
	hash := hex.EncodeToString(sum[:])

	var users []*User
	if err := s.db.SelectContext(ctx, &users,
		`SELECT `+userColumns+` FROM users WHERE api_key_hash IS NOT NULL`); err != nil {
		return nil, err
	}
	for _, u := range users {
		if u.APIKeyHash == nil {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(*u.APIKeyHash), []byte(hash)) == 1 {
			if u.Disabled {
				return nil, ErrUserDisabled
			}
			return u, nil
		}
	}
	return nil, ErrUserNotFound
}

// EnsureLocalAdmin makes sure row 1 exists and matches the username
// configured in stash's config.yml. It is safe to call on every startup.
func (s *Store) EnsureLocalAdmin(ctx context.Context, username string) error {
	if username == "" {
		username = "admin"
	}
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (id, username, role, disabled, ui_config, created_at, updated_at)
		VALUES (?, ?, 'admin', 0, '{}', ?, ?)
		ON CONFLICT(id) DO UPDATE SET username = excluded.username, role = 'admin', disabled = 0, updated_at = excluded.updated_at`,
		LocalAdminID, username, now, now)
	if err != nil {
		return fmt.Errorf("ensuring local admin: %w", err)
	}
	s.mu.Lock()
	s.cache = map[string]*User{}
	s.mu.Unlock()
	return nil
}

func nullable(s string) interface{} {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
