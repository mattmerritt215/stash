package api

import (
	"context"

	"github.com/stashapp/stash/internal/manager"
)

func (r *queryResolver) SystemStatus(ctx context.Context) (*manager.SystemStatus, error) {
	return scopeSystemStatus(ctx, manager.GetInstance().GetSystemStatus()), nil
}
