# Colony Planner

A private website for colony campaigns (several DMs, each with about six players; the author's campaign is
the first) plus Beny Bot, its one shared Discord bot. One Go binary serves a huma JSON API, the embedded
React SPA, live updates over SSE and the bot, on Postgres. The design is `docs/SPEC.md`, and M1.5's
multi-campaign design is `docs/design/multi-campaign.md`: read the sections your task touches before
changing behavior. Starting a new session? Read `docs/HANDOFF.md` first: the author's decisions and
working preferences, current status, pending items, and how to test without touching the author's setup.
It lives in the benthedev monorepo as apps/colony-planner, and every command and path here is relative
to this folder; the monorepo's own CLAUDE.md covers what spans apps.

## Commands

Run `mise run setup` once (pinned tools, `bun install`, and a `.env` with a random local database password).

| Command | What it does |
|---|---|
| `mise run dev` | Postgres, the Go server (devauth build) and Vite; open http://localhost:5173 |
| `mise run dev:phone` | the same, with Vite also on this Mac's Wi-Fi address |
| `mise run gen` | sqlc, then `web/src/api/openapi.json`, then `web/src/api/schema.d.ts` |
| `mise run test` | Go tests against the `colony_test` database, then web unit tests |
| `mise run lint` | the build-tag check, golangci-lint for both builds, then Biome (CI runs it as its own job) |
| `mise run typecheck` | go vet for both builds, then the web's tsc (CI runs it as its own job) |
| `mise run check` | every check; it must end with "All checks passed" |
| `mise run seed` / `reset` / `fresh` | load the two sample campaigns / wipe and re-migrate / both |
| `mise run build` | the SPA into web/dist/spa and bin/colony (no devauth); then bin/colony serve is the one-origin shape |
| `mise run db:up` / `db:down` | start / stop the Postgres container (and the database browser) |
| `mise run db:studio` | DbGate, a web database browser on http://localhost:54330, connected to `colony`; edits skip the app's rules |

Ports: Vite 5173; Go 8080 (bound to 127.0.0.1 outside production); DbGate 54330; Postgres 54329 (container
`colony-planner-db`, databases `colony` and `colony_test`). Vite proxies /api, /auth, /img, /calendar
and /healthz to Go and keeps the Host header. Web edits hot-reload; restart `mise run dev` after Go edits.
`reset`/`fresh` may run while dev runs: the server reconnects and everyone signs in again. Outside
production Go answers only a Host that is localhost, an IP address or PUBLIC_URL's host (DNS rebinding);
production answers only PUBLIC_URL's host, plus Railway's health check on /healthz.
The author's PATH has a Homebrew Go first: mise tasks use the pinned Go (`mise.toml` sets
activate_aggressive); in a bare shell use `mise exec -- go ...`.

## Layout

| Path | Holds |
|---|---|
| `cmd/colony/main.go` | subcommands serve, migrate up/status, openapi, seed, reset; `cmd/colony/config.go` reads every env var |
| `internal/game/` | pure rules with table tests: no I/O, no clock, no randomness (depguard and forbidigo enforce it); `internal/game/sessions.go` holds occurrences() (`Occurrences`) and the planner's rules (`PlanSessions`); `internal/game/market.go` holds an order's total, its docking day and `Unload` (the payment) |
| `internal/core/` | transport-neutral operations and DTOs, one file per feature; `internal/core/redact.go`, `internal/core/errors.go`, `internal/core/notify.go`, `internal/core/broker.go`; game sessions are `internal/core/game_sessions.go` (series, dates, stamps, recaps, the ICS feed) and `internal/core/session_planner.go` (the planner's reads, claims and leases, Discord event status), apart from sign-in sessions in `internal/core/sessions.go`; `internal/core/market.go` is the Market, shown as Shipments (items, the party cart, orders, the voyage and ship limit settings) |
| `internal/app/` | HTTP only: `internal/app/access.go` (levels, Register, startup check), `internal/app/app.go` (route table, security headers), `internal/app/compress.go` (gzip for JSON, html, css, js and svg; never the event stream, images, fonts or the ICS feed), `internal/app/transform.go`, `internal/app/errors.go`, `internal/app/enums.go`, one thin file per feature |
| `internal/auth/` | Discord OAuth2, the session cookie and its in-memory cache, the Caller middleware; `internal/auth/devauth.go` (build tag). Session data itself lives in `internal/core/sessions.go` |
| `internal/store/` | sqlc output |
| `internal/bot/` | Beny Bot (discordgo), in-process; `internal/bot/directory.go` lists a linked server's channels, `postToCampaign` in `internal/bot/notify.go` sends every campaign post, `internal/bot/npc.go` is `/npc view`, `internal/bot/requests.go` is the request prompts with Grant and Decline, `internal/bot/planner.go` is the one timer planner (Discord events, reminders, the cancellation notice, event status updates) and `internal/bot/colony.go` is `/colony` and `/nextsession` |
| `internal/seed/` | the two fictional sample campaigns for `colony seed`; `internal/seed/world.go` uploads the Eldari art as Sample Colony's Mainland, pins its places and paints the sample hexes (`--hexes N`); `internal/seed/sessions.go` plays the sample series and their bands; `internal/seed/market.go` adds Sample Colony's market items, cart and open order |
| `db/migrations/` | goose SQL, the one schema source (enums included), embedded by `db/embed.go` and applied by `db/migrate.go` |
| `db/dbtest/` | `dbtest.New(t)`: a fresh, migrated schema per test in TEST_DATABASE_URL, with a query counter |
| `db/scope_test.go` | the SQL scope lint: every query on a campaign-owned table filters it by campaign_id (part of `go test`) |
| `db/queries/` | sqlc input, one file per feature |
| `web/` | Vite SPA; `web/src/api/` is the only web code that knows the transport; `web/embed.go` embeds the build; feature folders `web/src/colony/`, `web/src/people/` (workforce, roles), `web/src/npcs/`, `web/src/places/`, `web/src/ventures/` (quests, patrons, requests, the approval sheet), `web/src/maps/` (image maps, pins, the upload button; `web/src/npcs/npc-portrait.tsx` and `web/src/places/place-pin.tsx` use it), `web/src/explore/` (the hex map: canvas layer, Paint-mode gestures, stroke queue), `web/src/sessions/` (the series sheet, session times, the calendar link card, the board's next game night; helpers in `web/src/lib/sessions.ts`) and `web/src/market/` (the Shipments page's catalog, party shipment draft and ship list, and its item, shipping, place-order and pay sheets; helpers in `web/src/lib/market.ts`) hold the pieces their route files render; `web/src/api/images.ts` uploads images and builds their URLs |
| `scripts/` | `scripts/dev.sh`, `scripts/gen.sh`, `scripts/check.sh`, `scripts/check-docs.sh`, `scripts/check-build-tags.sh`, `scripts/vulncheck.sh` (plain govulncheck: it fails only on a vulnerability our code reaches, and accepts none) |
| `docs/` | `docs/SPEC.md` (the design), `docs/design/multi-campaign.md` (M1.5), `docs/discord-dev-setup.md` (Beny Bot Dev, the test bot that also serves the live site for now), `docs/cutover.md` (the author's M6 runbook) and `docs/checklists/` |

## Deployment (M6)

- **Railway:** its own Railway project, deployed from the benthedev monorepo (BBruington/benthedev) at
  https://colony.benthedev.com from the first deploy. The service's root directory is apps/colony-planner
  and its watch path /apps/colony-planner/**, so only changes in this folder redeploy it. CI is the
  monorepo's colony-planner workflow (.github/workflows/colony-planner.yml at the repo root), filtered to
  the same folder; "Wait for CI" makes Railway deploy main only after it passes. The settings live in the
  project's dashboard, and `docs/cutover.md` lists every one.
- `Dockerfile` builds the one production image: Bun builds the SPA, Go builds `colony` without devauth
  (static, with the SPA and migrations embedded), and distroless runs it as non-root with
  `APP_ENV=production`. `.dockerignore` is an allow-list, so `.env` and key files never reach a build.
- `railway.toml` keeps the service's settings on record: the Dockerfile builder, the `/healthz` health
  check, 1 replica, restart on failure. A new Railway project ignores it, so `docs/cutover.md` sets the
  same values by hand in the dashboard; change both together.
- **1 replica, always** (Invariant 8): Beny Bot, the planner, the session cache and the SSE broker live
  in the process, and a second replica would run a second bot and split the cache and the broker. Railway's
  app sleeping stays off. The few seconds of deploy overlap are safe because every side effect is
  idempotent, so there is no leader lock.
- Production variables, entered in Railway by the author only: `DATABASE_URL` (Neon's direct endpoint,
  never a `-pooler` host: goose's migration lock needs a session; `sslmode=verify-full`, because pgx's
  `require` accepts any certificate, and serve refuses anything less in production), `PUBLIC_URL`
  (https; production answers only its host, `internal/auth/host.go`, and sends HSTS), `PORT`, `DISCORD_CLIENT_ID`, `DISCORD_CLIENT_SECRET`,
  `DISCORD_BOT_TOKEN` and `SITE_OWNER_DISCORD_ID` (serve refuses production without it). The image sets
  `APP_ENV`.
- **The same-token rule.** The live site runs as Beny Bot Dev (SPEC §11, question 19), so its bot token
  runs in exactly one place: two processes on one token both answer every command and both run planners.
  While Railway has the token, the author's `.env` has `DISCORD_BOT_TOKEN=` blank, set in `.env` itself
  because mise's `.env` values win over a variable typed before `mise run dev`; local Discord testing
  removes Railway's first (`docs/cutover.md` Part 4). Sandboxes and agent servers always blank every
  Discord value.
- Railway's own HTTP logs record every request's full path, IP address and browser, outside the app's
  redaction, so they show invite codes and ICS tokens: never share them (the app's Deploy Logs are safe).
- `/privacy` (`web/src/routes/privacy.tsx`) states what the migrations store and who sees it. A new
  column or table holding personal data, or a new outside service, updates it in the same change.

## Boundaries (enforced by `mise run check`)

```
cmd/colony -> app -> auth -> core -> store      cmd/colony -> bot -> core      seed -> core, store
web/src/** -> web/src/api -> the Go API
```

- Only `internal/core` touches data. No other package may import `internal/store`, pgx, database/sql or
  `db` (exceptions: `internal/seed`, a fixture loader; `cmd/colony`, which wires the pool; tests).
  This is an allow-list over the whole module, so new packages are covered automatically.
- `internal/core` knows none of its callers: no net, net/http, huma, discordgo or oauth2, and no import of
  app, auth, bot or seed. It reaches outward only through `core.Notifier`, `core.RequestNotifier` (request
  prompts and their outcomes), `core.SessionEvictor`, `core.DiscordDirectory` (Beny Bot's channel list) and
  `core.SessionPlanner` (the planner's re-plan signal and the cancellation notice), all passed in
  `core.Options` at construction.
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
   server refuses to start if anything lacks a level. Levels: public, token, account, owner, pending,
   member, dm. A route is pending, member or dm if and only if its path has `{campaignId}` and is outside
   /api/admin/; owner appears only under /api/admin/ and on /api/openapi.json and /api/docs. Exactly
   three routes admit pending members: `.../me`, `.../stream` and `.../leave`.
4. Secrets go in a `Dm` pointer sub-struct (`json:"dm,omitempty"`). The one Transformer strips every `dm`
   object unless the viewer is the DM and not viewing as a player. No handler redacts; `dm` is a reserved key.
5. Never hand-edit `internal/store`, `web/src/api/openapi.json`, `web/src/api/schema.d.ts` or
   `web/src/routeTree.gen.ts`. Run `mise run gen` (the router file regenerates on dev and build).
6. No PATCH semantics: full-record PUT, or delta operations for colony numbers.
7. Instants are UTC. Local-time schedules are expanded only by occurrences() in `internal/game`.
8. Single replica: Beny Bot, the planner, the session cache and the SSE broker live in-process.
9. No timer polls the database and no ordinary request writes bookkeeping (last_seen_at at most daily).
   A visible tab's stream still reconnects every 10 minutes, one membership read each (SPEC §10, M6).
10. Agents never touch production: no production credential, no deploy, and no Railway, Neon or Discord
    Developer Portal action (the author runs `docs/cutover.md`); and never read, print or copy a secret
    value from any .env or .pem file, here or in the old party-planner repo.
11. Every campaign-owned table carries `campaign_id NOT NULL REFERENCES campaign ON DELETE CASCADE` (first
    in its primary key or in `UNIQUE (campaign_id, id)`, and first in every index); text keys are unique
    per campaign; column references between campaign rows are composite foreign keys. Every query on a
    campaign-owned table takes `@campaign_id` and filters each such table by `campaign_id`, or says
    `-- unscoped: <reason>` on the line above `-- name:` (`db/scope_test.go`). IDs and keys from jsonb or
    request bodies are loaded with the Viewer's campaign, and "not found" is a field error.
12. A `core.Viewer` is proof of membership: only core builds one (`EnterCampaign`, `ViewerForGuild`), its
    fields are unexported and it has no owner flag. Campaign functions take a Viewer; account functions
    take a `core.Caller`; owner functions check `Caller.IsSiteOwner` and read only directory tables
    (account, campaign, campaign_request, discord_guild_link, counts). A non-member, the site owner
    included, gets 404 "No such campaign." from every campaign route.
13. Broadcasts are per campaign: `Broker.Publish(campaignID, topics...)`; there is no unscoped publish.
    Sequence numbers and the resume ring are per campaign.
14. Logs carry IDs, kinds, counts and statuses, never campaign content or message text.
15. A hidden row (`revealed = false`: NPCs, places, quests, patrons, patron actions, maps) never reaches
    a player: reads answer 404 exactly like a missing row, and it is absent from lists, search, named
    colonists, a place's npcsHere, quest givers, requests, map pins, the timeline and Beny Bot. A hidden named
    colonist still counts, as unnamed, in the player's headcounts. Players get an NPC only by
    `displayName`; the true name, known name, revealed flag and DM notes live in `NpcDm`.
    Timeline text names NPCs by display name, which is the true name while known_name is blank. So an
    entry about an NPC carries npc_id and npc_name (the display name it was written with, kept after a
    delete), reaches players only while its NPC is revealed, and its PUBLIC text (made public, edited, an
    UNDO repeating it) never names the NPC by npc_name once that isn't the display name, or by the true
    name while a known name is set (`npcEntryRefusal` in `internal/core/timeline.go`). A rename that does
    this moves the NPC's PUBLIC entries to DM, and deleting a hidden NPC moves theirs to DM. Workforce
    entries count in role totals, never unnamed.
    Beny Bot builds every reply from the player projection, even for the DM. The one exception is a
    request prompt: it goes only to the campaign's DM channel (through `postToCampaign`), so core builds
    it from the DM's projection.
16. M3 secrecy (quests, patrons, actions, requests). Visibility and archiving change only through their
    own switch endpoints, never a full-record PUT (an edit body carrying revealed, archived, status or
    favor is a 422), and create takes the starting visibility. Archived patrons and actions leave players'
    lists and every picker, but their history stays. Players get a quest's giver and patron only while
    those are revealed, and a request whose action or patron is hidden now, or was hidden when it was made
    (`made_while_revealed`), is absent for them. Timeline
    entries carry quest_id, patron_id and request_id, and reach players only while the linked quest,
    patron and action are revealed; a hidden patron's favor line goes in the entry's DM-only `dm.note`.
    An UNDO carries the links of what it undid, DM-only ones too, and never names an action whose use
    it undid (that undo deletes the request, the action's only link).
    Deleting a hidden quest moves its PUBLIC entries to DM before the link clears. A quest's failure cost
    (failure_effects, failure_favor) and dm_notes live in `QuestDm` until the quest fails. A request's
    decision note is per viewer, not DM-only: core returns it only to the requester and the DM (not in
    view as player), because it isn't in a `dm` object and the Transformer won't strip it
    (`mayReadDecisionNote` in `internal/core/requests.go`).
17. M4 secrecy (maps, pins, images). A hidden map is a hidden row like those in 15: players get 404
    "That map doesn't exist." and never its ID, name or art. Visibility, the Mainland flag, order and
    art each have their own endpoint; a map's edit body is only its name. Players' reads filter in SQL:
    a map's pins leave out hidden places, and a place's `pin` is null while its map is hidden, so a
    hidden map's ID and name never travel on a place. An image (`GET .../images/{imageId}`, member level)
    reaches a player, or the DM in view as player, only while a revealed map's art or a revealed NPC's
    portrait uses it (`GetImageView` in `db/queries/images.sql`); any other image, an unused upload
    included, is the same 404 as a missing one. Images are served with `Cache-Control: private,
    no-cache` and their ID as the ETag (the same URL is 404 for a player, so a browser shared with the DM
    must ask before reusing the DM's copy; a revalidation is a 304), nosniff and `Content-Security-Policy: default-src 'none'; sandbox`, and
    the type is sniffed from the bytes (PNG, JPEG, WebP, GIF; never SVG), with at most 16,384 pixels on a
    side and 50 megapixels in all. A map's art is this campaign's image or an https address on another
    site: core refuses PUBLIC_URL's host (`core.Options.PublicURL`) and any host name that isn't plain
    ASCII (a browser maps those first), so never the bundled `/eldari.webp` (C6). The colony board's faint
    backdrop is the Mainland from the viewer's own map list, so it follows the Mainland's visibility, and
    nothing when there is none. Hexes have no secrets: any active member, view as player included, may paint, label or erase.
    Maps, pins, portraits, images and hexes write no timeline entries.
18. M5 sessions. Series and session rows have no secrets and no hidden rows: every active member reads
    them, view as player included, and an NPC's first session (`firstSession`) is player-visible. The
    ICS token is the campaign's secret against outsiders and other campaigns, readable by its members,
    regenerated only by the DM; `GET /calendar/{token}.ics` is token level, an unknown, regenerated or
    suspended campaign's token answers exactly like a missing route, and logs show `/calendar/{icsToken}`.
    The one Beny Bot planner spans campaigns, so its campaign list is the only query marked
    `-- unscoped: planner`; the scope lint requires such a query to join discord_guild_link, filter
    `campaign.status = 'ACTIVE'` and use no OR or outer join. Every other planner read and write takes
    the campaign. Claim before calling Discord: a reminder claims `reminded_at` (released only when
    the post failed: Discord refused it or the campaign's server couldn't be read), an event create takes a 5-minute lease on `discord_claimed_at` that
    `game.PlanSessions` and `ClaimSessionDiscordEvent` both call over at exactly 5 minutes, and the bot
    adopts its own event with the same name and start (one no other date holds) before creating one. The
    event claim and its record hold only while the campaign is ACTIVE and linked to the plan's server, and
    unlinking forgets the events of unplayed dates (Beny Bot deletes the upcoming ones there, best
    effort). The table's time zone has no secret: the colony board carries it. Core signals the planner
    (`SessionPlanner.SessionsChanged`) after commit of every series, session and settings write, link and
    unlink, and event status update, but never after a released claim, so a failing Discord can't spin
    it. Mark played, End session and Discord's ACTIVE/COMPLETED stamps take the colony lock and write a
    PUBLIC `SESSION_PLAYED` entry, but are not undoable and leave the version and undo slot alone. Played
    sessions only stamp colony days; nothing else happens.
19. The Market. Items, the cart and orders have no secrets and no hidden rows: every active member reads
    them, view as player included, and an archived item reaches only the DM's projection (archiving is
    the only way to withdraw one; items are never deleted). A campaign has at most `market_ship_limit`
    UNPAID orders (a DM setting, default 1, checked under the colony lock by Place and by undoing a
    payment); at sea and docked are both UNPAID, and docked is derived from the colony day.
    Any active member edits the cart, places an order and pays; editing items, shipping and Cancel are
    the DM's. Place and Cancel take the colony lock and write a PUBLIC entry but, like Mark played, leave
    the version and undo slot alone. Pay is a member-level colony change (`runColonyMutation` takes the
    access level): STRICT gold, grants capped at their max, bought colonists into their item's role
    (decision 8's one exception), and the DM can undo it, which puts the order back at the docks.

API details that keep the contract single-sourced:
- Closed sets are Postgres enums. Add each to `enumTypes` in `internal/app/enums.go` so TypeScript gets a union.
- Lists are never null: core returns empty slices, not nil ones.
- Core returns typed errors (core.Invalid, Forbidden, NotFound, Conflict, ErrSessionEnded); `internal/app/errors.go` maps them.
- The DM is the campaign's `campaign_member` with role DM: exactly one per campaign (a partial unique
  index), always ACTIVE, and no API operation writes `role` (only campaign approval and the seed insert a
  DM). No co-DMs and no hand-off. The site owner is `Caller.IsSiteOwner`, from `SITE_OWNER_DISCORD_ID` on
  every request (the seeded seed:dm account in devauth builds when it is unset); a Viewer has no owner flag.
- Decisions C1–C6 (`docs/design/multi-campaign.md`, last section) are settled: one DM, no hand-off; a slug
  is fixed after approval; a DM may delete their own campaign after typing its slug, and the owner deletes
  only a suspended one; a denied requester may ask again (one open request each); a player may leave
  (their own membership, from PENDING or ACTIVE, never the DM), which sets REMOVED with `left_on_own`, so
  the invite link can't bring them back and only the DM re-admits; the Eldari art belongs to the author's
  campaign only, so M4 must not offer it to other campaigns as a starting map.
- The campaign is in the path: SPA `/c/{slug}/...`, API `/api/campaigns/{campaignId}/...`; no active-campaign
  cookie. `internal/app/access.go` resolves `{campaignId}` before huma parses input (an unparsable ID is the
  same 404), so every campaign input struct declares its CampaignID field itself (huma ignores path
  parameters in embedded structs).
- `serve`, `migrate` (up and status) and `seed` refuse a database whose goose_db_version records a
  migration this build doesn't embed (`CheckMigrationHistory` in `db/migrate.go`), because goose would
  skip a later migration that reuses its version; the message says a hosted database needs a build with
  those migrations and a local one `mise run fresh`. `reset` drops
  the schema before migrating, so `mise run reset` and `fresh` rebuild such a database. Never renumber or
  delete a migration once it is merged.
- Mutations accept JSON bodies only, behind stdlib http.CrossOriginProtection. The one exception is the
  image upload, `POST /api/campaigns/{campaignId}/images`, which must be multipart/form-data with one file
  field `image` (`requireJSONBodies` in `internal/app/app.go`, `isImageUpload` in `internal/app/images.go`).
  A body over 5 MiB + 64 KiB with a Content-Length is a 413 before anything reads it, and every upload
  body goes through http.MaxBytesReader. After the access check, `readImageUpload` in
  `internal/app/images.go` reads the form under a 2-minute deadline (408 when it stalls) and gives a
  chunked body over the cap the same 413. Every other request has 30 seconds to arrive whole
  (`HTTPServer` in `internal/app/app.go`, the server `cmd/colony` runs: ReadHeaderTimeout plus a ReadTimeout
  that covers the body, so a stalled body is cut off even when it's answered unread); there is no
  WriteTimeout, which would end event streams. The app has no app-wide Content-Security-Policy; if one is ever
  added, its img-src needs https: for maps whose art is an external address.
- Sessions belong to the account and are cached in memory (`internal/auth/cache.go`). Sign-out and
  sign-out-everywhere evict through `core.SessionEvictor`; a membership change (approve, deny, remove,
  re-admit, leave) touches no session and rescopes that membership's streams instead. After commit an
  operation publishes topics to the broker, never data.
- Dev sign-in: `/auth/dev?as=owner|dm|player|pending|<account id>`, or `?as=new&name=...` for a first-time
  sign-in; dm, player and pending mean Sample Colony's. It refuses requests another site starts
  (Sec-Fetch-Site), since a GET escapes CrossOriginProtection.
- Tests use TEST_DATABASE_URL (`colony_test`), never DATABASE_URL.

## Recipes

Add a field (four hand edits), then `mise run gen` and follow the compiler errors:
1. a new migration in `db/migrations/` (ALTER TABLE);
2. the column in the feature's queries file in `db/queries/`;
3. the DTO field, its mapping and its input field in the feature's file in `internal/core/` (a secret
   field goes in the Dm sub-struct); free text goes through `cleanText` (`internal/core/text.go`), and
   core, not a huma tag, checks lengths and formats so the person reads core's message;
4. its display and input in the screen under web/src/routes/_signedIn/c.$campaignSlug/_shell/, or in the
   feature folder's pieces that screen renders. For an NPC: display in `npcs.$npcId.tsx` (prose) or
   `web/src/npcs/npc-profile.tsx` (short facts), input in `web/src/npcs/npc-sheet.tsx`.
A secret field also needs a canary: plant a canary for every new Dm field or hidden row in its feature's
fixture in `internal/app/secrecy_fixtures_test.go`, into both campaigns. M2's people, NPCs and places
fixture already plants every M2 secret (true and known names, NPC and place DM notes, a hidden NPC, a hidden
place used as a revealed NPC's current place, a hidden named colonist, a PUBLIC entry about an NPC hidden
since), and hidden rows' IDs are forbidden
values, so a new link to one is caught too. M3's quests, patrons and requests fixture plants a hidden
quest, patron and action with a canary in every field, an action of the hidden patron, a QUEST_POSTED
entry and a PUBLIC favor entry whose quest and patron were hidden since, a request for an action hidden
since, Use action now on hidden actions (one renamed and revealed since), an undone Use action now whose
action is hidden since, a hidden quest's completion, a hidden patron's favor change and a hidden action's
use each undone with the UNDO made PUBLIC, the revealed quest's and patron's DM notes, the revealed quest's
failure cost, a quest completed for the hidden patron (its favor line is in `dm.note`), and archived
patrons and actions. A dedicated requester account no sweep caller uses owns a declined and a granted
request whose decision notes are canaries, so no other player and no DM viewing as player may see them.
M4's maps, pins, hexes and images fixture plants a revealed Mainland (the {mapId} row, its art the
{imageId} row) pinning the hidden place, a hidden map with a canary name and its own art pinning the
revealed {placeId} place (whose `pin` must be null for players), a portrait on the hidden NPC, an upload
nothing uses, and hex labels carrying canaryB in B. The hidden map's ID and every image only hidden
things use are forbidden values. Image bytes aren't JSON, so `TestPlayersGetHiddenImagesAsMissing` checks
that players and view as player get exactly the missing-image 404 for each of those images (use distinct
images for hidden and visible things: an image both use is rightly readable).
M5's sessions fixture plants a weekly series with a played first date and a recap (canaryB in every
text of B, since sessions have no DM-only values), sets that session as the first session of both the
hidden and the revealed NPC, and keeps each campaign's ICS token: B's token is forbidden wherever canaryB
is, and the outside-campaign routes sweep A's feed for B's values.
The secrecy sweep only sees a leak of a planted value. A new ID or map field in a request body also goes
in `mixedIDFields` in `internal/app/secrecy_test.go` (kind "uuid", "uuids" for a list of IDs, "key" or
"map"; M5 added `seriesId` and `sessionId`), so the sweep puts the other campaign's real IDs and keys into it.

Add an entity, then `mise run gen`:
1. a migration (the table carries `campaign_id` per Invariant 11) and a queries file in `db/queries/`
   whose every query takes `@campaign_id`;
2. a core file in `internal/core/`, and its live-update topic: a `Topic` constant in
   `internal/core/broker.go` listed in `AllTopics()` (a test fails if it isn't), published after commit;
3. for a new Postgres enum: a re-export of the type and every value in `internal/core/` (as
   `internal/core/member_status.go` does), each name in `allowedDataReferences` in
   `internal/core/boundary_test.go`, and one line in `enumTypes` in `internal/app/enums.go`;
4. a thin handler file in `internal/app/` registered from `app.New`;
5. a secrecy fixture in `internal/app/secrecy_fixtures_test.go`: plant into both campaigns (`canaryA` in
   A's DM-only values, `canaryB` in every value of B, `canaryBdm` in B's DM-only values; SPEC §8.11 has the
   details), name a real row for every path parameter, and list a GET where the DM sees the canary (the
   sweep runs DELETE routes last, so a delete can't remove a row a later GET needs);
6. its reads in `web/src/api/queries.ts` (factories that take the campaignId) and its topic's entry in
   `topicQueryKeys` in `web/src/api/topics.ts`;
7. a route file under web/src/routes/_signedIn/c.$campaignSlug/_shell/ (screens get the campaign from
   `useCampaign()` in `web/src/hooks/use-campaign.ts`), a nav entry, and a rules file in `internal/game/`
   if it has rules.

Add a non-huma route: one route-table line in `internal/app/app.go` with its access level.

Add a daily step (M1 onward): one Step in the `internal/game` pipeline, with table tests; no other wiring.

## Rules of the game

Do not invent game mechanics. The app keeps the books and the DM is the engine: nothing happens when food
reaches 0 or morale is low. An undecided rule is data or an open question (`docs/SPEC.md` §11), never code.

## Process

- Work one milestone at a time (`docs/SPEC.md` §10) and stop after each for the author's check-in.
- The author makes commits. Agents commit or push (git or jj) only when the author asks for a PR: then
  on a claude/<name> branch, never main, and the author merges. Messages use conventional prefixes:
  feat:, fix:, chore:, refactor:, docs:, test:.
- Pin exact dependency versions, choosing the newest release that is at least two weeks old.

## Conventions

- Full descriptive names in every language: campaignId, request, response, integration. Go's err is
  the only short name allowed; receivers are spelled out too (service *Service).
- TypeScript: no any (use unknown and type guards), no console.*, no process.env, no nested ternaries.
  Build tooling config (`web/vite.config.ts`) is the exception: it runs in Node, Biome allows process.env
  in *.config.ts, and COLONY_DEV_GO_URL there points a second dev server at another Go server.
  import.meta.env is read only in `web/src/env.ts`; a Biome plugin (`web/biome-plugins/no-import-meta-env.grit`)
  enforces it.
- Comments explain why, never what.
- Web files: a route file under web/src/routes holds the page and its data wiring (queries, mutations,
  toasts). A piece used by more than one screen, or big enough to stand alone, goes in
  `web/src/components/` (or a feature folder under web/src when a feature grows several), and takes
  its data and callbacks as props.
- An action that removes or disables its own trigger (a row that moves, a banner or sheet that closes)
  moves focus to a stable target when it settles, with `focusFirstVisible` from `web/src/lib/focus.ts`
  (a heading takes tabIndex={-1}); see Let in on /c/{slug}/players and Back to DM view. BottomSheet and
  AlertDialogContent hand focus back to whatever opened them (`web/src/hooks/use-focus-return.ts`); a
  confirmed action that removes its opener prevents the default in onCloseAutoFocus and moves focus itself.
- Web UI: shadcn/ui components (added with the shadcn CLI per `web/components.json`), `cn` from
  `web/src/lib/utils.ts`, palette tokens and fonts in `web/src/styles.css` ("Night Sea and Parchment").
  Players use phones: every player screen works one-handed at 360 px wide.
- Leaflet maps: the map's container isolates its stacking context (`isolation: isolate`, as
  `web/src/maps/image-map.css` and the Explore map's wrapper do), because Leaflet's panes use z-indexes up
  to 1000 and would otherwise cover bottom sheets, dialogs and the shell's bars.
