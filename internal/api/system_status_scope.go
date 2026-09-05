package api

import (
	"context"

	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/pkg/auth"
)

// scopeSystemStatus redacts the server-internal fields of SystemStatus for
// callers without manage_server.
//
// systemStatus is not an admin-only concern in practice: the frontend's
// top-level router blocks rendering *anything* until this query resolves,
// using status to detect the needs-migration state. It has to be readable by
// every authenticated role (see pkg/auth/roles.go) for the app to load at
// all. What it must not do for a non-administrator is leak filesystem paths,
// the host OS, or the ffmpeg binary location -- so, same pattern as
// scopeConfigResult in config_scope.go, strip those and keep only what the
// frontend's bootstrap logic actually needs: status and the schema versions.
func scopeSystemStatus(ctx context.Context, ret *manager.SystemStatus) *manager.SystemStatus {
	am := authManager()
	if am == nil || !am.Enabled() {
		return ret
	}

	u := auth.FromContext(ctx)
	if u == nil || u.Can(auth.PermManageServer) {
		return ret
	}

	return &manager.SystemStatus{
		DatabaseSchema: ret.DatabaseSchema,
		AppSchema:      ret.AppSchema,
		Status:         ret.Status,
		// Os / WorkingDir / HomeDir are non-nullable in the schema, so an
		// empty string rather than omission -- the frontend's bootstrap path
		// never reads them, only Status.
	}
}
