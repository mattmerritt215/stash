package sqlite

import "context"

// GetRandomExcluded reports whether the context's user has excluded this
// scene from random selection. See sceneUserState and userColOnly: a
// missing overlay row means "not excluded".
func (qb *SceneStore) GetRandomExcluded(ctx context.Context, id int) (bool, error) {
	return getUserBoolOnlyState(ctx, sceneUserState, id, "random_excluded")
}

// SetRandomExcluded sets whether the context's user excludes this scene
// from random selection.
func (qb *SceneStore) SetRandomExcluded(ctx context.Context, id int, value bool) error {
	return setUserState(ctx, sceneUserState, id, "random_excluded", value)
}
