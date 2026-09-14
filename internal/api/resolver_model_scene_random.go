package api

import (
	"context"

	"github.com/stashapp/stash/pkg/models"
)

// RandomExcluded reports whether the current user has excluded this scene
// from random selection. See pkg/sqlite/scene_random_state.go.
func (r *sceneResolver) RandomExcluded(ctx context.Context, obj *models.Scene) (bool, error) {
	var ret bool
	if err := r.withReadTxn(ctx, func(ctx context.Context) error {
		v, err := r.repository.Scene.GetRandomExcluded(ctx, obj.ID)
		if err != nil {
			return err
		}
		ret = v
		return nil
	}); err != nil {
		return false, err
	}

	return ret, nil
}
