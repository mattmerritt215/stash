package api

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/pkg/auth"
)

// toStashUser converts a store user into the GraphQL model.
func toStashUser(u *auth.User) *StashUser {
	if u == nil {
		return nil
	}
	ret := &StashUser{
		ID:          strconv.Itoa(u.ID),
		Username:    u.Username,
		Role:        UserRole(strings.ToUpper(u.Role)),
		Disabled:    u.Disabled,
		HasAPIKey:   u.APIKeyHash != nil && *u.APIKeyHash != "",
		CreatedAt:   u.CreatedAt,
		Email:       u.Email,
		DisplayName: u.DisplayName,
	}
	if u.LastLoginAt != nil {
		t := *u.LastLoginAt
		ret.LastLoginAt = &t
	}
	return ret
}

func authManager() *auth.Manager {
	return manager.GetInstance().Auth
}

// Me returns the caller's identity and effective permissions. It is the only
// user query a non-admin can run, and it is what the frontend uses to decide
// which navigation entries and settings panels to render.
func (r *queryResolver) Me(ctx context.Context) (*CurrentUser, error) {
	am := authManager()

	ret := &CurrentUser{
		AuthMode:  "disabled",
		LogoutURL: "/logout",
	}

	if am == nil || !am.Enabled() {
		// Single-user stash: report the full permission set so the UI is
		// identical to upstream.
		for _, p := range auth.AllPermissions {
			ret.Permissions = append(ret.Permissions, string(p))
		}
		return ret, nil
	}

	ret.AuthMode = string(am.Config.Mode)
	if am.Config.Mode == auth.ModeOIDC || am.Config.Mode == auth.ModeHybrid {
		ret.LogoutURL = "/oidc/logout"
	}

	u := auth.FromContext(ctx)
	if u == nil {
		return ret, nil
	}

	ret.User = toStashUser(u)
	for _, p := range u.GetRole().Permissions() {
		ret.Permissions = append(ret.Permissions, string(p))
	}
	return ret, nil
}

// Users lists every stash user. Gated to manage_server by the GraphQL gate.
func (r *queryResolver) Users(ctx context.Context) ([]*StashUser, error) {
	am := authManager()
	if am == nil || !am.Enabled() {
		return nil, errors.New("the multi-user layer is disabled")
	}
	users, err := am.Users.List(ctx)
	if err != nil {
		return nil, err
	}
	ret := make([]*StashUser, 0, len(users))
	for _, u := range users {
		ret = append(ret, toStashUser(u))
	}
	return ret, nil
}

func (r *mutationResolver) UserUpdate(ctx context.Context, input UserUpdateInput) (*StashUser, error) {
	am := authManager()
	if am == nil || !am.Enabled() {
		return nil, errors.New("the multi-user layer is disabled")
	}
	id, err := strconv.Atoi(input.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid user id %q", input.ID)
	}

	if input.Role != nil {
		role := auth.ParseRole(string(*input.Role))
		if role == auth.RoleNone {
			return nil, fmt.Errorf("invalid role %q", *input.Role)
		}
		if err := am.Users.SetRole(ctx, id, role); err != nil {
			return nil, err
		}
	}
	if input.Disabled != nil {
		if err := am.Users.SetDisabled(ctx, id, *input.Disabled); err != nil {
			return nil, err
		}
	}

	u, err := am.Users.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return toStashUser(u), nil
}

func (r *mutationResolver) UserDestroy(ctx context.Context, id string) (bool, error) {
	am := authManager()
	if am == nil || !am.Enabled() {
		return false, errors.New("the multi-user layer is disabled")
	}
	uid, err := strconv.Atoi(id)
	if err != nil {
		return false, fmt.Errorf("invalid user id %q", id)
	}
	if caller := auth.FromContext(ctx); caller != nil && caller.ID == uid {
		return false, errors.New("you cannot delete your own account")
	}
	if err := am.Users.Destroy(ctx, uid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) UserGenerateAPIKey(ctx context.Context, id string) (string, error) {
	am := authManager()
	if am == nil || !am.Enabled() {
		return "", errors.New("the multi-user layer is disabled")
	}
	uid, err := strconv.Atoi(id)
	if err != nil {
		return "", fmt.Errorf("invalid user id %q", id)
	}
	return am.Users.GenerateAPIKey(ctx, uid)
}

func (r *mutationResolver) GenerateOwnAPIKey(ctx context.Context) (string, error) {
	am := authManager()
	if am == nil || !am.Enabled() {
		return "", errors.New("the multi-user layer is disabled")
	}
	u := auth.FromContext(ctx)
	if u == nil {
		return "", errors.New("not signed in")
	}
	return am.Users.GenerateAPIKey(ctx, u.ID)
}

func (r *mutationResolver) RevokeOwnAPIKey(ctx context.Context) (bool, error) {
	am := authManager()
	if am == nil || !am.Enabled() {
		return false, errors.New("the multi-user layer is disabled")
	}
	u := auth.FromContext(ctx)
	if u == nil {
		return false, errors.New("not signed in")
	}
	if err := am.Users.RevokeAPIKey(ctx, u.ID); err != nil {
		return false, err
	}
	return true, nil
}
