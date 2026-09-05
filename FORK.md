# stash-multiuser

A hard fork of [stashapp/stash](https://github.com/stashapp/stash), forked at
`c1fa78e0b28ac1d5164421f31e2d105dcaa2ea40` (2026-08-25), adding OIDC/reverse-proxy
authentication, role-based permissions, and per-user personalisation on top of
what is otherwise stock, single-user Stash.

Upstream closed the OIDC feature request ([#6115](https://github.com/stashapp/stash/issues/6115))
as *not planned*, so there's no path other than forking.

This repo used to be carried as a patch series applied at build time (a
`scripts/apply.sh` that cloned upstream and replayed patches). It's now a
normal git fork: the `multiuser` branch is upstream's history plus the commits
below, and pulling in a new upstream release is `git fetch upstream && git
merge upstream/master`, not a patch-reapply exercise.

## What changed, and why

New, conflict-free files (never touch upstream code, so they can never
conflict on merge):

- `pkg/auth/` — config, session/OIDC manager, role & permission tables,
  user store
- `pkg/sqlite/user_state.go` + `pkg/sqlite/migrations/86_multiuser.up.sql` —
  per-user overlay tables for `rating`/`favorite`/`organized`/`resume_time`;
  a missing row means "inherit the library default". Migration 86 seeds user
  row 1 (the local admin) with every pre-existing value, so upgrading a
  single-user install is lossless and reversible.
- `internal/api/resolver_user.go`, `config_scope.go`, `system_status_scope.go`
- `graphql/schema/types/user.graphql`
- `ui/v2.5/src/hooks/permissions.tsx` + two frontend `.graphql` documents

Modified upstream files — see `git log` on `multiuser` for the full,
individually-explained commit history, one commit per file/concern:

| File | What changes |
| --- | --- |
| `graphql/schema/schema.graphql` | adds `me`, `users`, `userUpdate`, `userDestroy`, `userGenerateApiKey`, `generateOwnApiKey`, `revokeOwnApiKey` |
| `internal/api/authentication.go` | unknown credentials fall through to the auth layer instead of a bare 401; marks local sessions |
| `internal/api/resolver_mutation_configure.go` | non-admin `configureUI`/`configureUISetting`/`configureInterface` write to the caller's own store |
| `internal/api/resolver_mutation_migrate.go`, `internal/manager/task/migrate.go` | keep the auth layer enabled across a schema migration instead of needing a restart |
| `internal/api/resolver_query_configuration.go` | routes `configuration` through per-user scoping |
| `internal/api/resolver_query_metadata.go` | routes `systemStatus` through scoping — see the bug note below |
| `internal/api/server.go` | mounts `/oidc`, installs the auth middleware and the gqlgen permission gate |
| `internal/manager/manager.go`, `init.go` | holds and builds `Manager.Auth` |
| `pkg/sqlite/database.go` | schema version 85→86; exposes `SQLDB()` |
| `pkg/sqlite/saved_filter.go` | saved filters scoped per user |
| `ui/v2.5/src/App.tsx` | wraps the tree in `PermissionProvider`, ahead of `PluginsLoader` |
| `ui/v2.5/src/components/MainNavbar.tsx` | logout target comes from the server |
| `ui/v2.5/src/components/Settings/Settings.tsx` | tab gating + first-allowed-tab landing |
| `ui/v2.5/src/plugins.tsx` | fixes an infinite error-toast loop for non-admins |
| `ui/v2.5/src/core/createClient.ts` | no-op error handler on a permission-gated subscription |
| `pkg/auth/roles.go` | `metadataScan` and `findJob` reclassified from `run_tasks` to `edit_metadata` — see below |

## Two real bugs worth knowing about if you rebase this

**`systemStatus` blocked the entire app for non-admins.** `App.tsx`'s
`renderContent()` won't render *any* route until `systemStatus` resolves. It
was classified `manage_server`-only, so every non-admin sign-in hung on a bare
loading spinner forever, no console error, no failed network request to point
at. Fixed by reclassifying to `view_library` and redacting the sensitive
fields (paths, OS, ffmpeg location) in the resolver for non-admins, mirroring
the existing `configuration` redaction pattern. If you add a new field to
`SystemStatus` upstream, decide whether it needs the same redaction.

**`metadataScan` / `findJob` were `run_tasks`, blocking curators (and service
accounts) from finishing an add-by-file-path flow.** A curator can create
scenes/tags directly, so being unable to call `metadataScan` — the mechanism
for picking up a file that landed on disk outside GraphQL (rsync, yt-dlp,
etc.) — or `findJob` to poll that scan to completion, was pure friction, not a
real security boundary. Both reclassified to `edit_metadata`. The genuinely
admin-only jobs — `metadataClean` (destructive), `metadataIdentify` (external
stash-box calls), `metadataGenerate`/`metadataAutoTag` (whole-library),
`stopJob`/`stopAllJobs` (can kill someone else's job), `jobQueue` (enumerates
every job on the server) — stay at `run_tasks`.

**The lesson for future permission classification:** `pkg/auth/roles.go`
fails closed — an unclassified field defaults to `manage_server`. That's the
property that makes this fork safe to merge upstream: a new mutation added by
a future release is admin-only until someone classifies it. When a real user
hits a spurious "not permitted", get the exact denied `query.X`/`mutation.X`
name from the journal before changing anything, then ask what capability that
field *actually* represents — enumeration/whole-library/destructive/external
stays high; "finish something I was already trusted to start" belongs at the
same tier as the thing that started it.

## Architecture decisions

**Config lives in `/etc/stash/env`, not `config.yml`.** Nothing in stash's
viper schema is touched, so upstream config changes never collide, and the
OIDC client secret stays out of the stash data directory. `STASH_AUTH_MODE`
unset → the binary is stock stash.

**Enforcement is a gqlgen `AroundOperations` middleware, not a schema
directive.** A directive would mean regenerating `generated_exec.go`. The
middleware checks root field names against two tables in `pkg/auth/roles.go`.

**`configuration` and `systemStatus` are readable by everyone, then redacted
in the resolver**, because the frontend cannot render without them.

**Per-user state is an overlay, not a column move.** Base tables keep their
columns exactly as upstream defines them; `<entity>_user_state` tables hold
each user's own value.

**Auth modes: `oidc`, `header`, `hybrid`, `disabled`.** OIDC is the
authorization-code flow with PKCE, run by stash itself. Header mode
(reverse-proxy forward-auth) requires `STASH_AUTH_TRUSTED_PROXIES`; the
process refuses to start with headers enabled and an empty list.

**Break-glass.** OIDC discovery failure is non-fatal — stash starts and
retries in the background. The built-in `/login` still works from
`config.yml` credentials and maps to user row 1 as admin, who can never be
demoted, disabled or deleted.

## Roles

| Permission | guest | user | curator | admin |
| --- | :-: | :-: | :-: | :-: |
| `view_library` | ● | ● | ● | ● |
| `track_activity` | ● | ● | ● | ● |
| `personalise` | | ● | ● | ● |
| `edit_metadata` | | | ● | ● |
| `scrape` | | | ● | ● |
| `delete_content` | | | | ● |
| `run_tasks` | | | | ● |
| `manage_server` | | | | ● |

Group mapping (`STASH_OIDC_ADMIN_GROUPS` / `STASH_OIDC_USER_GROUPS`) reaches
`admin` and `user`; `curator` and `guest` are assigned via `userUpdate` or
`STASH_AUTH_DEFAULT_ROLE`. Roles are re-evaluated on every sign-in.

## Status by tier

| | Status |
| --- | --- |
| Interface settings, ui config, front page layout | **working**, verified |
| Saved filters, default filters | schema done; resolver scoping is next |
| Play count, o-counter, resume position, ratings, favourites, organized | schema done (overlay tables); filter/sort rewiring is next |
| Library visibility (who sees which scenes) | **not implemented, deliberately** — every authenticated user sees the whole library. Gating this properly means enforcing it in the SQL filter layer *and* on `/scene/`, `/image/`, `/downloads/`, which stream files without going through GraphQL. Half-doing it is worse than not doing it. |

`pkg/sqlite/user_state.go` carries the machinery for the unfinished tiers:
`userCol()` builds the `COALESCE(overlay, base)` expression, `sortJoin()`/
`sortCol()` handle `ORDER BY`, and `userIntCriterionHandler`/
`userBoolCriterionHandler`/`userFloatCriterionHandler` are drop-in
replacements for upstream's handlers. Finishing it is mechanical — swap call
sites in the seven `*_filter.go` files, one entity at a time, checking filter
*and* sort, plus writing through `setUserState(ctx, ...)` instead of the base
column.

## Building

Same as upstream stash: build the frontend first (`cd ui/v2.5 && pnpm install
&& pnpm run gqlgen && pnpm run build`), then `go generate ./cmd/stash && go
build ./cmd/stash`.

## Merging future upstream releases

```sh
git remote add upstream https://github.com/stashapp/stash.git   # once
git fetch upstream
git merge upstream/master   # or a specific tag
```

Most likely to conflict or need attention, in order:

- **New mutations/queries in `schema.graphql`** — fails closed automatically
  (unclassified fields need `manage_server`), but users will see spurious
  "not permitted" until you classify them in `pkg/auth/roles.go`. Get the
  exact denied field name from the journal, decide what capability it really
  represents, classify it.
- **`internal/api/authentication.go`** — touched whenever upstream changes the
  login/session flow.
- **`ui/v2.5/src/components/Settings/Settings.tsx`** — the `SettingsTabGate`
  wrapper straddles the whole nav, so a new upstream settings tab lands
  inside it and defaults to visible unless added to `settingsTabPermissions`.
- **`internal/api/resolver_query_configuration.go` / `config_scope.go`** — a
  new config section must be added to the redaction list or non-admins start
  seeing it.
- **`internal/api/resolver_query_metadata.go` / `system_status_scope.go`** —
  same redaction concern for any new `SystemStatus` field.

## authentik provider setup (or any OIDC provider)

Provider: OAuth2/OpenID, confidential, redirect URI
`https://<your-host>/oidc/callback` (strict), subject mode "based on the
user's hashed ID". Scopes must include the `groups` scope mapping — without
it the ID token carries no groups, the userinfo fallback is empty, and every
sign-in is refused with "not a member of any group that grants access to
stash". That refusal is deliberate: the fork will not default anyone into
access.
