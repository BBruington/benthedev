# Colony Planner

A private website for one D&D campaign (one DM, about six players) plus Beny Bot, its Discord bot. One Go
binary serves a huma JSON API, the embedded React SPA, live updates over SSE and the bot, on Postgres.
The design is `docs/SPEC.md`: read the sections your task touches before changing behavior.

## Commands

Run `mise run setup` once (pinned tools, `bun install`, and a `.env` with a random local database password).

| Command | What it does |
|---|---|
| `mise run dev` | Postgres, the Go server (devauth build) and Vite; open http://localhost:5173 |
| `mise run dev:phone` | the same, with Vite also on this Mac's Wi-Fi address |
| `mise run gen` | sqlc, then `web/src/api/openapi.json`, then `web/src/api/schema.d.ts` |
| `mise run test` | Go tests against the `colony_test` database, then web unit tests |
| `mise run check` | every check; it must end with "All checks passed" |
| `mise run seed` / `reset` / `fresh` | load the sample campaign / wipe and re-migrate / both |
| `mise run build` | the SPA into web/dist/spa and bin/colony (no devauth); then bin/colony serve is the one-origin shape |
| `mise run db:up` / `db:down` | start / stop the Postgres container |

Ports: Vite 5173; Go 8080 (bound to 127.0.0.1 outside production); Postgres 54329 (container
`colony-planner-db`, databases `colony` and `colony_test`). Vite proxies /api, /auth, /img, /calendar
and /healthz to Go and keeps the Host header. Web edits hot-reload; restart `mise run dev` after Go edits.
`reset`/`fresh` may run while dev runs: the server reconnects and everyone signs in again. Outside
production Go answers only a Host that is localhost, an IP address or PUBLIC_URL's host (DNS rebinding).
The author's PATH has a Homebrew Go first: mise tasks use the pinned Go (`mise.toml` sets
activate_aggressive); in a bare shell use `mise exec -- go ...`.

## Layout

| Path | Holds |
|---|---|
| `cmd/colony/main.go` | subcommands serve, migrate up/status, openapi, seed, reset; `cmd/colony/config.go` reads every env var |
| `internal/game/` | pure rules with table tests: no I/O, no clock, no randomness (depguard and forbidigo enforce it) |
| `internal/core/` | transport-neutral operations and DTOs, one file per feature; `internal/core/redact.go`, `internal/core/errors.go`, `internal/core/notify.go`, `internal/core/broker.go` |
| `internal/app/` | HTTP only: `internal/app/access.go` (levels, Register, startup check), `internal/app/app.go` (route table), `internal/app/transform.go`, `internal/app/errors.go`, `internal/app/enums.go`, one thin file per feature |
| `internal/auth/` | Discord OAuth2, the session cookie and its in-memory cache, the Viewer middleware; `internal/auth/devauth.go` (build tag). Session data itself lives in `internal/core/sessions.go` |
| `internal/store/` | sqlc output |
| `internal/bot/` | Beny Bot (discordgo), in-process |
| `internal/seed/` | the fictional sample campaign for `colony seed` |
| `db/migrations/` | goose SQL, the one schema source (enums included), embedded by `db/embed.go` and applied by `db/migrate.go` |
| `db/dbtest/` | `dbtest.New(t)`: a fresh, migrated schema per test in TEST_DATABASE_URL, with a query counter |
| `db/queries/` | sqlc input, one file per feature |
| `web/` | Vite SPA; `web/src/api/` is the only web code that knows the transport; `web/embed.go` embeds the build |
| `scripts/` | `scripts/dev.sh`, `scripts/gen.sh`, `scripts/check.sh`, `scripts/check-docs.sh`, `scripts/check-build-tags.sh` |
| `docs/` | `docs/SPEC.md` (the design) and `docs/discord-dev-setup.md` (Beny Bot Dev, the private test bot) |

## Boundaries (enforced by `mise run check`)

```
cmd/colony -> app -> auth -> core -> store      cmd/colony -> bot -> core      seed -> core, store
web/src/** -> web/src/api -> the Go API
```

- Only `internal/core` touches data. No other package may import `internal/store`, pgx, database/sql or
  `db` (exceptions: `internal/seed`, a fixture loader; `cmd/colony`, which wires the pool; tests).
  This is an allow-list over the whole module, so new packages are covered automatically.
- `internal/core` knows none of its callers: no net, net/http, huma, discordgo or oauth2, and no import of
  app, auth, bot or seed. It reaches outward only through `core.Notifier` and `core.SessionEvictor`.
- core's exported API never mentions a store or pgx type, because Go infers types and a returned
  `*store.Queries` would leak the database without an import. `internal/core/boundary_test.go` enforces
  this; re-export only enum and value types (see `internal/core/member_status.go`), never queries or pools.
- huma appears only in `internal/app`, so switching the API toolchain rewrites only that package. app may
  import auth (middleware, sign-in routes); auth never imports app. app, auth and bot meet only in `cmd/colony`.
- On the web, only `web/src/api/` talks to the server: fetch, EventSource, XMLHttpRequest, WebSocket (bare
  or via window/globalThis/self), navigator.sendBeacon, and openapi-fetch / openapi-react-query.
- The only Go build tag is devauth, because check builds with and without it (`scripts/check-build-tags.sh`).
- Rules live in `.golangci.yml` (depguard), `web/biome.json` and `web/biome-plugins/transport-only-in-api.grit`.
  Never loosen a rule to make a change compile: move the code to the side of the seam it belongs on.

## Invariants

1. Game rules live only in `internal/game`. SQL lives only in `db/queries/` (sqlc generates `internal/store`).
   Handlers in `internal/app` only decode, call one `internal/core` function, and encode.
2. Every colony change takes the colony lock, runs one transaction, writes grouped timeline entries,
   records the undo snapshot, and publishes (spec §5.1).
3. Every route declares its access. huma operations go through `app.Register(api, level, operation, handler)`,
   which writes x-access; everything else is one line in the route table in `internal/app/app.go`. The
   server refuses to start if anything lacks a level. Levels: public, token, pending, member, dm.
4. Secrets go in a `Dm` pointer sub-struct (`json:"dm,omitempty"`). The one Transformer strips every `dm`
   object unless the viewer is the DM and not viewing as a player. No handler redacts; `dm` is a reserved key.
5. Never hand-edit `internal/store`, `web/src/api/openapi.json`, `web/src/api/schema.d.ts` or
   `web/src/routeTree.gen.ts`. Run `mise run gen` (the router file regenerates on dev and build).
6. No PATCH semantics: full-record PUT, or delta operations for colony numbers.
7. Instants are UTC. Local-time schedules are expanded only by occurrences() in `internal/game`.
8. Single replica: Beny Bot, the planner, the session cache and the SSE broker live in-process.
9. No timer polls the database and no ordinary request writes bookkeeping (last_seen_at at most daily).
10. Local only until the author approves M6: no deploy work, no production credential, and never read,
    print or copy a secret value from any .env or .pem file, here or in the old party-planner repo.

API details that keep the contract single-sourced:
- Closed sets are Postgres enums. Add each to `enumTypes` in `internal/app/enums.go` so TypeScript gets a union.
- Lists are never null: core returns empty slices, not nil ones.
- Core returns typed errors (core.Invalid, Forbidden, NotFound, Conflict, ErrSessionEnded); `internal/app/errors.go` maps them.
- The DM is `Viewer.IsDM`, derived from DM_DISCORD_ID on every request (or the seeded seed:dm member in
  devauth builds when it is unset). Nothing stores a role.
- Mutations accept JSON bodies only, behind stdlib http.CrossOriginProtection.
- Sessions are cached in memory (`internal/auth/cache.go`). An operation that changes a member's status or
  sessions evicts through `core.SessionEvictor`; after commit it publishes topics to the broker, never data.
- Dev sign-in: `/auth/dev?as=dm|player|pending|<member id>`, or `?as=new&name=...` for a first-time sign-in.
  It refuses requests another site starts (Sec-Fetch-Site), since a GET escapes CrossOriginProtection.
- Tests use TEST_DATABASE_URL (`colony_test`), never DATABASE_URL.

## Recipes

Add a field (four hand edits), then `mise run gen` and follow the compiler errors:
1. a new migration in `db/migrations/` (ALTER TABLE);
2. the column in the feature's queries file in `db/queries/`;
3. the DTO field, its mapping and its input field in the feature's file in `internal/core/` (a secret
   field goes in the Dm sub-struct);
4. its display and input in the screen under web/src/routes.

Add an entity: a migration, a queries file, a core file, a thin handler file in `internal/app/` registered
from `app.New`, a route file under web/src/routes, a nav entry, and a rules file in `internal/game/` if it
has rules.

Add a non-huma route: one route-table line in `internal/app/app.go` with its access level.

Add a daily step (M1 onward): one Step in the `internal/game` pipeline, with table tests; no other wiring.

## Rules of the game

Do not invent game mechanics. The app keeps the books and the DM is the engine: nothing happens when food
reaches 0 or morale is low. An undecided rule is data or an open question (`docs/SPEC.md` §11), never code.

## Process

- Work one milestone at a time (`docs/SPEC.md` §10) and stop after each for the author's check-in.
- The author makes every commit. Agents never commit or push (git or jj). The author's messages use
  conventional prefixes: feat:, fix:, chore:, refactor:, docs:, test:.
- Pin exact dependency versions, choosing the newest release that is at least two weeks old.

## Conventions

- Full descriptive names in every language: campaignId, request, response, integration. Go's err is
  the only short name allowed; receivers are spelled out too (service *Service).
- TypeScript: no any (use unknown and type guards), no console.*, no process.env, no nested ternaries.
  import.meta.env is read only in `web/src/env.ts`; a Biome plugin (`web/biome-plugins/no-import-meta-env.grit`)
  enforces it.
- Comments explain why, never what.
- Web UI: shadcn/ui components (added with the shadcn CLI per `web/components.json`), `cn` from
  `web/src/lib/utils.ts`, palette tokens and fonts in `web/src/styles.css` ("Night Sea and Parchment").
  Players use phones: every player screen works one-handed at 360 px wide.
