package api

import (
	"context"
	"strconv"

	"github.com/stashapp/stash/pkg/models"
)

// SceneSetRandomExcluded sets whether the caller excludes a scene from
// random selection. This is per-user state (pkg/sqlite/scene_random_state.go),
// so it deliberately does not go through the sceneUpdate/SceneUpdateInput
// path, which writes the base scenes table.
func (r *mutationResolver) SceneSetRandomExcluded(ctx context.Context, id string, value bool) (*models.Scene, error) {
	sceneID, err := strconv.Atoi(id)
	if err != nil {
		return nil, err
	}

	var ret *models.Scene
	if err := r.withTxn(ctx, func(ctx context.Context) error {
		if err := r.repository.Scene.SetRandomExcluded(ctx, sceneID, value); err != nil {
			return err
		}

		ret, err = r.repository.Scene.Find(ctx, sceneID)
		return err
	}); err != nil {
		return nil, err
	}

	return ret, nil
}
