# Settings rework

Split the one settings page into a rider's own **profile** and an
**admin-only** service area, and make the server enforce the split. Nothing
here has been applied. Every claim names the file it was read from at
`527d8c3`; line numbers will drift, shapes will not.

## Where things stand

**One settings surface, no admin gate on it.** `internal/httpapi/handler.go`
registers `GET /v1/settings` and nine `PUT /v1/settings/*` sections, plus
`PUT /v1/tasks/{name}/schedule` and `POST /v1/tasks/{name}/run`. All of them
sit behind `gated` (any valid session) and nothing checks `identity.Admin`.
The only admin checks today are target scoping: `RunTask` for
`sync:target`/`sync:clear`, `StartOAuth` over a named target, and the owner
column in `GET /v1/status`. Any signed-in rider can therefore rewrite the
Wahoo application, the source credentials, the basemaps, or the schedule.
`docs/specs/service.md` §"Settings" describes these as what "an operator may
change" but never says who an operator is.

**Everything server-side is shared.** `internal/runtimeconfig.Values` is one
whole-object snapshot backed by the singleton `runtime_settings` row and its
list tables (`internal/sqlite/migrations/000027_current_schema.up.sql`).
There is no per-subject settings storage of any kind.

**What is already per rider.** Exactly one thing: the Wahoo target, owned via
`targets.owner_subject` (migration 30), created by `GET /oauth/wahoo/start`
and scoped in `/v1/status` by `targetIDs` (`handler.go`). Units and theme are
browser-local (`useUnitSystem`, `themeChoice` in `App.tsx`) and stay that way.

**Identity reaches the browser.** `GET /v1/webui/config` returns
`identity.{display,admin}`; `TargetConvergenceCard.tsx` and `UserPill.tsx`
already read it. The session's `Admin` flag comes from the Auth0 Action claim
(`internal/session/service.go`), so no new identity plumbing is needed.

**UI shape.** `features/settings/SettingsPage.tsx` stacks: display
preferences (local) → `ServiceSettings` (nine shared cards) → a link to
`/settings/tasks` → `DataSources` credits. `MenuBar.tsx` has one "Settings"
entry. The rider's own target only appears on `/sync`, mixed into the fleet
view an admin sees.

## Target shape

| Surface | Who | Holds |
| --- | --- | --- |
| `/settings` | every rider | units, theme (local); **own Wahoo account**: connection state, connect / re-authorise; credits (`DataSources`) |
| `/admin` | admin only | today's `ServiceSettings` cards, link to tasks |
| `/admin/tasks` | admin only | today's `TasksPage` |

No new storage. The profile's Wahoo card is the rider's own row from
`/v1/status` (already scoped) plus the existing `/oauth/wahoo/start` link;
reuse `TargetRow`. Disconnecting a target does not exist today and is not
added here.

Server rule: a non-admin session gets `403` from every shared write and from
the admin-only reads. Not `404`: the paths are published in
`api/openapi.yaml`, so hiding them buys nothing, and `404` is reserved for
"which targets exist" as the spec already states.

Admin-only after the change:

- `GET /v1/settings` and every `PUT /v1/settings/*` (it carries the Wahoo
  client ID, source URLs and which secrets are set — nothing a rider needs
  once `/v1/webui/config` keeps serving basemaps and source base URLs).
- `GET /v1/tasks`, `GET /v1/tasks/runs`, `PUT /v1/tasks/{name}/schedule`.
- `POST /v1/tasks/{name}/run` for every task except `sync:target` over the
  caller's own subject. Today a rider can trigger the source inventory or a
  surface rebuild; `SyncControls.tsx` shows both buttons to everyone, so the
  UI hides them for non-admins in the same change.
- The `/admin` and `/admin/tasks` documents (the Go page handlers check
  `identity.Admin` and answer not found; the SPA route also redirects).

Stays for every rider: `/v1/status`, `/v1/sync/runs`, `/v1/routes*`,
`/v1/weather`, `/v1/webui/config`, the reprocess POST (check: the spec ties
it to origin only, decide whether a rider may reprocess a shared route), and
`sync:target` over their own subject.

## Steps

Ordered so each is reviewable alone. Step 1 is a safety fix and should land
first regardless of the rest.

1. **Admin gate on the server.** One helper in `internal/httpapi` (an
   `adminOnly` wrapper around the handler funcs above, answering the existing
   error shape with `403 forbidden`). Regression tests in
   `routes_settings_test.go` / `routes_tasks_test.go` with a non-admin
   identity, following `gate_test.go`'s pattern. Add `403` to each affected
   operation in `api/openapi.yaml` and regenerate. Update
   `docs/specs/service.md` (define the admin subject once, next to the
   sign-in gate around line 155, and mark each endpoint) and the safety list
   in `AGENTS.md`. Run `test-race`: the session gate is touched.
2. **Profile page.** `SettingsPage.tsx` drops `ServiceSettings` and the tasks
   card, gains a "Wahoo account" card built from the caller's own target in
   `/v1/status` (reuse `TargetRow`; `ConnectPrompt` from
   `TargetConvergenceCard.tsx` when none). `SyncControls.tsx` hides the
   source/surface run buttons for non-admins. Vitest for both states.
3. **Admin page.** New `features/admin/AdminPage.tsx` wrapping
   `ServiceSettings` and the tasks link; move `TasksPage` under
   `/admin/tasks`; `App.tsx` routes with a `Navigate` from `/settings/tasks`;
   `MenuBar.tsx` adds "Admin" when `identity.admin`; Go page handlers for
   `/admin` and `/admin/tasks`. Storybook stories move with the components.
   Update `service.md` §"Browser UI" route list and the spec's "state-changing
   HTTP" sentence in `AGENTS.md` if the path list there changes.

Steps 2 and 3 can be one PR; step 1 is its own.

## Decisions taken

- Profile scope is the Wahoo connection only. Units and theme stay
  browser-local; per-rider notifications and ride-model inputs are out until
  something consumes them.
- Two routes, not tabs or hidden cards: the admin area has its own URL, so
  the server can refuse the document as well as the API.
- `403`, not `404`, for the admin gate.

## Open

- Whether a rider may `POST .../reprocess` on a shared route. Leaning no:
  it spends the elevation and surface budget everyone shares.
- Whether the dev session (`DOMESTIQUE_DEV_SESSION`) should be admin by
  default. It must be for `ui-dev` to reach `/admin`; confirm in
  `cmd/domestique/providers.go` when doing step 1.
