package sqlite

import (
	"context"
	"fmt"

	"github.com/stashapp/stash/pkg/auth"
	"github.com/stashapp/stash/pkg/models"
)

// Per-user state overlay.
//
// The base tables keep their rating / favorite / organized / resume_time
// columns exactly as upstream defines them. Each `<entity>_user_state` table
// holds one row per (object, user) with that user's own values, and a NULL
// column there means "inherit the base table's value".
//
// Reads go through userCol(), which produces
//
//	COALESCE((SELECT <col> FROM <state> WHERE <fk> = <base>.id AND user_id = N), <base>.<col>)
//
// so a user who has never touched an object sees the library default, and the
// local administrator (user 1) sees exactly what they saw before migration 86.

type userStateSpec struct {
	// StateTable is the overlay table, e.g. "scenes_user_state".
	StateTable string
	// BaseTable is the upstream table, e.g. "scenes".
	BaseTable string
	// FKColumn is the overlay's foreign key to the base table, e.g. "scene_id".
	FKColumn string
}

var (
	sceneUserState     = userStateSpec{"scenes_user_state", "scenes", "scene_id"}
	imageUserState     = userStateSpec{"images_user_state", "images", "image_id"}
	galleryUserState   = userStateSpec{"galleries_user_state", "galleries", "gallery_id"}
	performerUserState = userStateSpec{"performers_user_state", "performers", "performer_id"}
	studioUserState    = userStateSpec{"studios_user_state", "studios", "studio_id"}
	tagUserState       = userStateSpec{"tags_user_state", "tags", "tag_id"}
	groupUserState     = userStateSpec{"groups_user_state", "groups", "group_id"}
)

// userCol returns a SQL expression resolving col for the context's user,
// falling back to the base table's column.
func (s userStateSpec) userCol(ctx context.Context, col string) string {
	uid := auth.UserID(ctx)
	return fmt.Sprintf(
		"COALESCE((SELECT `%s`.`%s` FROM `%s` WHERE `%s`.`%s` = `%s`.`id` AND `%s`.`user_id` = %d), `%s`.`%s`)",
		s.StateTable, col,
		s.StateTable,
		s.StateTable, s.FKColumn, s.BaseTable,
		s.StateTable, uid,
		s.BaseTable, col,
	)
}

// sortJoin returns a join that attaches the context user's overlay row, for
// use when sorting on an overlaid column.
func (s userStateSpec) sortJoin(ctx context.Context) join {
	uid := auth.UserID(ctx)
	return join{
		sort:  true,
		as:    s.StateTable,
		table: s.StateTable,
		onClause: fmt.Sprintf("`%s`.`%s` = `%s`.`id` AND `%s`.`user_id` = %d",
			s.StateTable, s.FKColumn, s.BaseTable, s.StateTable, uid),
		joinType: joinTypeLeft,
	}
}

// sortCol returns the expression to ORDER BY once sortJoin has been added.
func (s userStateSpec) sortCol(col string) string {
	return fmt.Sprintf("COALESCE(`%s`.`%s`, `%s`.`%s`)", s.StateTable, col, s.BaseTable, col)
}

// ---------------------------------------------------------------------------
// criterion handlers
// ---------------------------------------------------------------------------

// userIntCriterionHandler is the per-user equivalent of intCriterionHandler.
// It resolves the column expression at query time, when the context — and
// therefore the user id — is available.
func userIntCriterionHandler(c *models.IntCriterionInput, s userStateSpec, col string, addJoinFn func(f *filterBuilder, joinType joinType)) criterionHandlerFunc {
	return func(ctx context.Context, f *filterBuilder) {
		if c == nil {
			return
		}
		if addJoinFn != nil {
			joinType := joinTypeInner
			if c.Modifier == models.CriterionModifierIsNull {
				joinType = joinTypeLeft
			}
			addJoinFn(f, joinType)
		}
		clause, args := getIntCriterionWhereClause(s.userCol(ctx, col), *c)
		f.addWhere(clause, args...)
	}
}

// userBoolCriterionHandler is the per-user equivalent of boolCriterionHandler.
func userBoolCriterionHandler(c *bool, s userStateSpec, col string, addJoinFn func(f *filterBuilder, joinType joinType)) criterionHandlerFunc {
	return func(ctx context.Context, f *filterBuilder) {
		if c == nil {
			return
		}
		if addJoinFn != nil {
			addJoinFn(f, joinTypeInner)
		}
		v := "0"
		if *c {
			v = "1"
		}
		f.addWhere(fmt.Sprintf("%s = %s", s.userCol(ctx, col), v))
	}
}

// userFloatCriterionHandler is the per-user equivalent of
// floatCriterionHandler.
func userFloatCriterionHandler(c *models.FloatCriterionInput, s userStateSpec, col string, addJoinFn func(f *filterBuilder, joinType joinType)) criterionHandlerFunc {
	return func(ctx context.Context, f *filterBuilder) {
		if c == nil {
			return
		}
		if addJoinFn != nil {
			addJoinFn(f, joinTypeInner)
		}
		clause, args := getFloatCriterionWhereClause(s.userCol(ctx, col), *c)
		f.addWhere(clause, args...)
	}
}

// ---------------------------------------------------------------------------
// writes
// ---------------------------------------------------------------------------

// setUserState upserts one column of the context user's overlay row.
//
// Writes always land in the overlay, never in the base table, so one user
// rating a scene cannot change what anyone else sees. Metadata that is a
// property of the object rather than of the viewer — title, date, studio,
// tags — continues to be written to the base table by upstream's code, gated
// by the edit_metadata permission.
func setUserState(ctx context.Context, s userStateSpec, objectID int, col string, value interface{}) error {
	uid := auth.UserID(ctx)
	q := fmt.Sprintf(
		"INSERT INTO `%s` (`%s`, `user_id`, `%s`, `updated_at`) VALUES (?, ?, ?, datetime('now')) "+
			"ON CONFLICT(`%s`, `user_id`) DO UPDATE SET `%s` = excluded.`%s`, `updated_at` = excluded.`updated_at`",
		s.StateTable, s.FKColumn, col, s.FKColumn, col, col)
	_, err := dbWrapper.Exec(ctx, q, objectID, uid, value)
	return err
}

// clearUserState removes the context user's overlay row, restoring the
// library default for that object.
func clearUserState(ctx context.Context, s userStateSpec, objectID int) error {
	uid := auth.UserID(ctx)
	q := fmt.Sprintf("DELETE FROM `%s` WHERE `%s` = ? AND `user_id` = ?", s.StateTable, s.FKColumn)
	_, err := dbWrapper.Exec(ctx, q, objectID, uid)
	return err
}
