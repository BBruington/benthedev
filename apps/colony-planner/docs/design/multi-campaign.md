# M1.5: Multi-campaign Colony Planner

**Status:** approved by the author on 2026-10-11 with the decisions in "Decisions confirmed by the author" (they override §0–§10 where they differ). **Date:** 2026-10-10.
**Builds after:** M1 is approved and committed. **Builds before:** M2.
**Replaces:** SPEC.md §1.3 principle 1 ("One campaign, one DM, one colony"). Everything else in SPEC.md
stands unless this document says otherwise; §9 lists every SPEC.md and CLAUDE.md edit.
**Audience:** the author (developer and DM) first, builders second.
**Note:** the M1 → M1.5 upgrade scaffolding (migration 0003's conversion, `DM_DISCORD_ID`'s refusal, `mise run env:upgrade`) was removed before merge: 0001–0003 were squashed into one `0001_init.sql` with the same final schema, and a database migrated before that is refused until `mise run fresh` rebuilds it. The text below keeps the history.

## 0. The decisions on one page

| # | Decision |
|---|---|
| D1 | Three identities: an **account** (one Discord identity, global), a **membership** (account in one campaign: role DM or PLAYER, status, character name), and the **site owner** (`SITE_OWNER_DISCORD_ID`, derived on every request, never stored). `DM_DISCORD_ID` and `DISCORD_GUILD_ID` are removed. |
| D2 | Exactly **one DM per campaign**, stored as `campaign_member.role = 'DM'` and enforced by a partial unique index. No API operation writes `role`. No co-DMs and no hand-off in M1.5. |
| D3 | **Path-scoped URLs.** SPA: `/c/{slug}/...`. API: `/api/campaigns/{campaignId}/...`. No "active campaign" cookie or header. |
| D4 | **A `core.Viewer` is proof of membership.** Only core builds one, from a fresh membership read; it carries the campaign in an unexported field and has no site-owner flag. Campaign functions take a Viewer; account functions take a `core.Caller`. |
| D5 | **Every campaign-owned table carries `campaign_id`** (`NOT NULL REFERENCES campaign ON DELETE CASCADE`); singletons become one row per campaign; text keys become `(campaign_id, key)`; links between campaign rows are composite foreign keys. IDs inside jsonb and request bodies are checked by core, not the database. |
| D6 | **Guards, no RLS:** the Viewer type, a SQL scope lint, composite foreign keys, per-campaign broadcasts, and a cross-campaign secrecy sweep (bodies included) whose second campaign is all canary and must stay unchanged. Row-level security is an M6 decision. |
| D7 | **Owner manages, never reads.** Owner routes live only under `/api/admin/` and read only directory tables. A non-member, the owner included, gets **404** from every campaign route. |
| D8 | **New DMs ask; the owner approves** on `/admin`. Approval creates the campaign, its DM membership, colony, settings, default resources and roles in one transaction. A denied account may ask again. |
| D9 | **Joining is by invite link** `/join/{inviteCode}`; the DM approves per campaign, as today. Reset kills the old link at once. Removal is per campaign and no longer signs anyone out. There is no self-service leave: a player asks the DM, as today. |
| D10 | **One shared Beny Bot**, commands registered globally. A server is linked to one campaign with a one-time code from the campaign's Settings that a member with Manage Server runs as `/link`. Every interaction routes guild → campaign → membership. |
| D11 | **Suspend** closes the campaign to everyone (site, stream, bot, invite link, timers) with data intact. **Delete** is offered only for a suspended campaign, after typing its slug, and cascades. |
| D12 | **SSE is per campaign only:** `GET /api/campaigns/{campaignId}/stream`. Pages outside a campaign refetch on focus. `Broker.Publish` requires a campaign; sequence numbers and the resume ring are per campaign. |
| D13 | **Migration 0003** turns the author's local data into the first campaign, DM'd by the account whose Discord ID is `SITE_OWNER_DISCORD_ID` (or `seed:dm`). On an empty database it creates no campaign. Every `colony` subcommand refuses to run while `DM_DISCORD_ID` is set. |
| D14 | **Seed: two sample campaigns** that share players, including a DM of one who plays in the other, plus the owner and a pending DM request. |
| D15 | Limits: 1 open DM request per account; 20 pending joiners per campaign; pings 5 an hour per campaign and 5 an hour to the owner. Owner approval is the gate for everything else. |

What does **not** change: the game rules, `internal/game`, Undo, View as player, Record cargo, exact stock
for players, huma on trial, direct Discord OAuth2 with server-side sessions, local only until M6, the
author makes every commit.

---

## 1. Concepts and identities

### 1.1 Terms

| Term | What it is | Stored where |
|---|---|---|
| **Account** | A person, identified by their Discord user ID. One per Discord identity. | `account` (today's `member`, renamed and slimmed) |
| **Campaign** | One table's game: one DM, one colony, its own players, timeline, settings, Discord server. | `campaign` |
| **Membership** | An account's place in one campaign: `role` (DM, PLAYER), `status` (PENDING, ACTIVE, REMOVED), `character_name`. | `campaign_member` |
| **DM** | The membership with `role = 'DM'`. Always ACTIVE. One per campaign. | `campaign_member` |
| **Site owner** | The account whose Discord ID equals `SITE_OWNER_DISCORD_ID`. Manages campaigns; reads none it isn't a member of. | env var, derived per request |
| **Campaign request** | An account asking to run a campaign; the owner approves or denies it. | `campaign_request` |
| **Invite link** | `/join/{inviteCode}`: asks to join that campaign. | `campaign.invite_code` |
| **Guild link** | The one Discord server a campaign uses with Beny Bot. | `discord_guild_link` |

### 1.2 Rules

- **The site owner is never stored.** `SITE_OWNER_DISCORD_ID` is read at startup and compared on every
  request, as `DM_DISCORD_ID` is today. In devauth builds with it unset, the seeded `seed:dm` account is the
  owner (`auth.EffectiveSiteOwnerDiscordID`, replacing `EffectiveDmDiscordID`). Being owner adds only the
  `owner` access level (§4); it confers nothing inside any campaign.
- **The DM is now stored**, per campaign. That gives up "nothing in the database can escalate a player"
  for something narrower and checkable: **no API operation writes `role`.** Only `ApproveCampaignRequest`
  (and the seed) inserts a DM membership. The partial unique index keeps it to one; a CHECK keeps the DM
  ACTIVE; core refuses to remove or deny the DM. Only direct database access (DbGate) can change a role.
- **Co-DMs and hand-off: not in M1.5** (C1). The schema keeps the door open: co-DMs later mean dropping one
  partial index and adding a promote operation.
- **One person, several campaigns.** An account has any number of memberships, at most one per campaign
  (`UNIQUE (campaign_id, account_id)`), each with its own role, status and character name. Ana can be
  Thorin in the author's campaign and Brisa in a friend's; the friend can play in the author's campaign
  while DMing their own. Sessions belong to the account, so one sign-in serves every campaign.
- **View as player stays one boolean per session** (`web_session.view_as_player`, unchanged). It applies in
  every campaign where the caller is DM and does nothing elsewhere. It only lowers privileges and the
  banner always shows.
- **Removal is per campaign.** Removing Ana from campaign B sets her B membership to REMOVED and rescopes
  her B streams to `me`. **Her session survives**, so her other campaigns keep working; her B tab drops to
  "You don't have access" at once. (M0 signed removed members out of the whole site; with several
  campaigns that would punish them everywhere.)
- **A REMOVED membership is final until the DM re-admits it**, exactly as F4 says today. Opening the invite
  link again changes nothing and pings nobody. There is no "leave" (C5): a player who wants out asks the DM
  to remove them, so no path leads from REMOVED back to PENDING.

---

## 2. Data model

### 2.1 Directory tables (global; queried without a campaign by design)

```sql
CREATE TYPE campaign_role AS ENUM ('DM', 'PLAYER');
CREATE TYPE campaign_status AS ENUM ('ACTIVE', 'SUSPENDED');
CREATE TYPE campaign_request_status AS ENUM ('PENDING', 'APPROVED', 'DENIED', 'WITHDRAWN');

-- today's member, minus everything that is per campaign (status, character_name, approved_at)
ALTER TABLE member RENAME TO account;
-- account: id, discord_user_id UNIQUE, discord_username, display_name, avatar_url, created_at, last_seen_at

CREATE TABLE campaign (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    -- In the SPA's URLs; chosen in the request, adjustable by the owner at approval, then fixed (C2).
    slug text NOT NULL UNIQUE CHECK (length(slug) BETWEEN 3 AND 40 AND slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    -- Directory data: the owner sees it. The DM edits it on Settings (was settings.campaign_name).
    name text NOT NULL CHECK (name <> '' AND length(name) <= 80),
    status campaign_status NOT NULL DEFAULT 'ACTIVE',
    suspended_at timestamptz,
    -- 16 random bytes, base64url. Stored plainly so the DM can copy it again; it only lets someone ask.
    -- Served only by the dm-level GET .../invite (§5.3), never in any other DTO.
    invite_code text NOT NULL UNIQUE CHECK (invite_code ~ '^[A-Za-z0-9_-]{22,64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT campaign_suspended_at_matches CHECK ((status = 'SUSPENDED') = (suspended_at IS NOT NULL))
);

-- Holds directory data only: no colony name, timezone or other campaign content.
CREATE TABLE campaign_request (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL REFERENCES account (id),
    campaign_name text NOT NULL CHECK (campaign_name <> '' AND length(campaign_name) <= 80),
    slug text NOT NULL,
    note text NOT NULL DEFAULT '' CHECK (length(note) <= 500),   -- to the owner
    status campaign_request_status NOT NULL DEFAULT 'PENDING',
    decided_at timestamptz,
    campaign_id uuid REFERENCES campaign (id) ON DELETE SET NULL,  -- set on approval
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX campaign_request_one_open ON campaign_request (account_id) WHERE status = 'PENDING';
CREATE UNIQUE INDEX campaign_request_open_slug ON campaign_request (slug) WHERE status = 'PENDING';

-- web_session: member_id → account_id (REFERENCES account ON DELETE CASCADE); view_as_player unchanged.

CREATE TABLE discord_guild_link (
    campaign_id uuid PRIMARY KEY REFERENCES campaign (id) ON DELETE CASCADE,  -- one server per campaign
    guild_id text NOT NULL UNIQUE CHECK (guild_id ~ '^[0-9]{17,20}$'),        -- one campaign per server
    guild_name text NOT NULL,               -- snapshot for the DM's Settings card
    linked_by_discord_user_id text NOT NULL,
    linked_at timestamptz NOT NULL DEFAULT now()
);
```

Link codes (§6.2) live in memory in core, not in a table (single replica, Invariant 8).

### 2.2 Campaign-owned tables (every query takes the campaign)

```sql
CREATE TABLE campaign_member (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    account_id uuid NOT NULL REFERENCES account (id),
    role campaign_role NOT NULL DEFAULT 'PLAYER',
    status member_status NOT NULL DEFAULT 'PENDING',     -- PENDING, ACTIVE, REMOVED (unchanged enum)
    character_name text,
    created_at timestamptz NOT NULL DEFAULT now(),
    approved_at timestamptz,
    UNIQUE (campaign_id, account_id),
    UNIQUE (campaign_id, id),
    CONSTRAINT campaign_member_dm_active CHECK (role <> 'DM' OR status = 'ACTIVE')
);
CREATE UNIQUE INDEX campaign_member_one_dm ON campaign_member (campaign_id) WHERE role = 'DM';
CREATE INDEX campaign_member_account_idx ON campaign_member (account_id);
```

How each existing table changes:

| Table | Today | After 0003 |
|---|---|---|
| `settings` | `id smallint PK CHECK (id = 1)`, `campaign_name`, timezone, 3 channel IDs | `campaign_id uuid PK → campaign ON DELETE CASCADE`; `id` and `campaign_name` dropped (name moved to `campaign.name`). Channel IDs must belong to the linked server (§6.4). |
| `colony` | `id smallint PK CHECK (id = 1)` | `campaign_id uuid PK → campaign CASCADE`; `id` dropped. The colony lock becomes `... WHERE campaign_id = @campaign_id FOR UPDATE`, so campaigns never wait on each other. |
| `resource` | `key text PK` | `PRIMARY KEY (campaign_id, key)`. Every campaign has its own `gold` and `food`. |
| `role` | `key text PK` | `PRIMARY KEY (campaign_id, key)`. |
| `timeline_event` | no campaign | `campaign_id NOT NULL → campaign CASCADE`; `UNIQUE (campaign_id, id)`; `created_by` → `FOREIGN KEY (campaign_id, created_by) REFERENCES campaign_member (campaign_id, id) ON DELETE SET NULL (created_by)`; indexes become `(campaign_id, day, created_at)` and `(campaign_id, group_id) WHERE group_id IS NOT NULL`. `sequence` stays a global identity (it only orders within a query that already filters by campaign). |
| `member` | the person and their status | becomes `account` (§2.1); status, character name and approval move to `campaign_member`. |
| `web_session` | `member_id`, `view_as_player` | `account_id`, `view_as_player` (unchanged). |

### 2.3 The rules every campaign-owned table follows (now and in M2–M5)

1. `campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE`, as the **first** column of its
   primary key or of a `UNIQUE (campaign_id, id)` beside a uuid key, and the first column of every index.
2. A **column** that references another campaign-owned row is a **composite foreign key** `(campaign_id,
   x_id) → parent (campaign_id, id)` (`ON DELETE SET NULL (x_id)` where the spec says SET NULL). Such a
   column can never point into another campaign.
3. Text keys (resource, role, and anything like them later) are unique **per campaign**, and joins on them
   always include `campaign_id`.
4. Every query on the table takes `@campaign_id` (§3.3 lint).
5. **References the database can't see** get no such protection: IDs and keys inside jsonb, and IDs and
   keys in request bodies. Core must load each one with `campaign_id = viewer.CampaignID()` and treat
   "not found" as a field error. In M1 these are: `AdjustInput.arrivalEntryId` and the
   `timeline_event.data.arrivalEntryId` it becomes (`arrivalFor` → `GetTimelineEvent`), `UndoInput.groupId`
   and `colony.last_change` (`groupId`, `before`; `MarkTimelineGroupUndone`), and resource and role keys
   in `Effects` and in adjust, cargo and resource bodies. M3 adds request IDs in bodies. Sweep pass 4
   (§4.4) puts the other campaign's real IDs and keys into every one of them.

### 2.4 Migration `0003_campaigns.sql`

The author's database has members, a colony, history and maybe sessions. Tests and `mise run reset`
start empty. One migration handles both.

1. Create the enums, `campaign`, `campaign_request`, `campaign_member`, `discord_guild_link`; rename
   `member` to `account`.
2. Add a **nullable** `campaign_id` to `settings`, `colony`, `resource`, `role`, `timeline_event`.
3. Read both variables through goose's environment substitution (goose v3.28; only this statement has
   ENVSUB on, because `$$` in a DO block would be expanded). There is no default here; the DO block decides:

   ```sql
   -- +goose ENVSUB ON
   SELECT set_config('colony.site_owner_discord_id', '${SITE_OWNER_DISCORD_ID:-}', true),
          set_config('colony.legacy_dm_discord_id', '${DM_DISCORD_ID:-}', true);
   -- +goose ENVSUB OFF
   ```
4. One DO block (`StatementBegin`/`StatementEnd`), in this order:
   - **no accounts** (fresh database, every test): `DELETE` the default rows 0001/0002 inserted (settings,
     colony, resources, roles); there is no campaign until someone is approved. RETURN. (The variables are
     ignored, so a stale shell environment can't break a test database.)
   - legacy set and owner empty: RAISE `0003: rename DM_DISCORD_ID to SITE_OWNER_DISCORD_ID in .env (keep
     the value).` Both set and different: RAISE `0003: DM_DISCORD_ID and SITE_OWNER_DISCORD_ID differ;
     delete DM_DISCORD_ID.` Neither set: the owner is `seed:dm`.
   - the value must match `^([0-9]{17,20}|seed:dm)$`, else RAISE;
   - find the account with that Discord ID. **None:** RAISE `0003: no account has Discord ID <x>. Put the
     Discord ID you used as DM into SITE_OWNER_DISCORD_ID, or run mise run fresh.`
   - insert the campaign: `name` = `settings.campaign_name`; `slug` = the name lower-cased, runs of
     anything but `a-z0-9` replaced by `-`, cut to 40, **then** leading and trailing `-` trimmed, and
     `colony` if fewer than 3 characters remain; `invite_code` = `replace(gen_random_uuid()::text, '-', '')`;
   - insert one `campaign_member` per account **reusing the account's id as the membership id**, so
     `timeline_event.created_by` stays valid without rewriting; the owner's account gets `role = 'DM'`,
     `status = 'ACTIVE'`; everyone else keeps their status, character name and approval time;
   - set `campaign_id` on every row of `settings`, `colony`, `resource`, `role`, `timeline_event`.
5. Then: `SET NOT NULL`, swap the primary keys (drop `settings.id`, `colony.id`; `(campaign_id, key)` on
   resource and role), add the foreign keys and indexes of §2.2, drop `account.status`,
   `account.character_name`, `account.approved_at`, `settings.campaign_name`, and the old
   `timeline_event.created_by → member` foreign key.
6. `-- +goose Down`: `RAISE EXCEPTION '0003 cannot be undone; run mise run reset'` (reset drops the schema
   and never runs Down).

**Every subcommand refuses first.** `loadConfig()` in `cmd/colony/config.go` (used by `serve`, `migrate`,
`seed` and `reset`) fails when `DM_DISCORD_ID` is set at all: "DM_DISCORD_ID is gone: rename it to
SITE_OWNER_DISCORD_ID in .env (keep the value)." The DO block is the backstop for anything that migrates
without `loadConfig`.

**Migration tests** (`db/migrations_test.go`, migrating a test database to 0002, inserting rows, then 0003):
the author's shape (a real-ID DM, players, history, a session) becomes one campaign with every row
attributed; an empty database ends with no campaign and no default rows; **a database with both a
`seed:dm` account and a real-ID account, `DM_DISCORD_ID` set and `SITE_OWNER_DISCORD_ID` unset, fails and
changes nothing**; a missing owner account fails; a campaign name like `"The Iron Coast of the Far Northern
Reach - Year 2"` yields a valid slug with no trailing `-`.

### 2.5 A new campaign's defaults

The defaults that 0001/0002 inserted move into `db/queries/campaigns.sql` as `CreateDefaultResources :exec`
and `CreateDefaultRoles :exec` (`INSERT ... SELECT @campaign_id, ... FROM (VALUES ...)`), with exactly
today's rows: gold, food (system), materials, morale (max 100, starts at 100); the eight roles plus
UNASSIGNED (system). They are data, not rules, and they stay in SQL. Colony: the column defaults (name
"The Colony", day 0, no shipment schedule). Settings: the column defaults (timezone UTC, no channels). The
new DM names the colony and picks the timezone on Settings; the DM's empty board says so (§5.2).

### 2.6 Effect on the M2–M5 tables in SPEC.md §4

Every table below gets the §2.3 treatment. Only the non-obvious parts are listed.

| Table (milestone) | Campaign-scoped keys and references |
|---|---|
| `npc` (M2) | `(campaign_id, colony_role_key) → role (campaign_id, key)`; `(campaign_id, current_place_id)`, `(campaign_id, origin_place_id) → place`; `(campaign_id, first_session_id) → session` (M5). The colonist count filters **both** tables: `SUM(unnamed_count) FROM role WHERE campaign_id = @campaign_id` + `COUNT(*) FROM npc WHERE campaign_id = @campaign_id AND colony_role_key IS NOT NULL`. |
| `place` (M2), `map` (M4) | `(campaign_id, map_id) → map`. **No map is seeded per campaign**, and the Eldari art is never offered to other campaigns (C6, confirmed below); other DMs upload their own. |
| `quest`, `patron`, `patron_action` (M3) | `(campaign_id, giver_npc_id)`, `(campaign_id, patron_id)`. As built, the patron and action keys are NO ACTION rather than RESTRICT, so deleting a campaign can cascade while a stray delete still fails (SPEC §5.7). |
| `request` (M3) | `requested_by`, `on_behalf_of`, `decided_by` → `campaign_member (campaign_id, id)`. Request IDs in bodies and Discord buttons are loaded with the Viewer's campaign (§2.3 rule 5, §6.3). |
| `timeline_event` link columns (M2–M5) | `(campaign_id, quest_id)` etc., `ON DELETE SET NULL (quest_id)`. |
| `hex` (M4) | `PRIMARY KEY (campaign_id, q, r)`. |
| `image` (M4) | `UNIQUE (campaign_id, sha256)` (no cross-campaign sharing); `uploaded_by → campaign_member`; served at `/api/campaigns/{campaignId}/images/{imageId}` (member level), so it obeys the path rule of §4.1. |
| `session_series`, `session` (M5) | `(campaign_id, series_id)`; the timezone is that campaign's `settings.timezone`. |
| `settings.ics_token` (M5) | globally `UNIQUE`; `/calendar/{token}.ics` resolves the campaign from the token and serves only that campaign's player data. |

---

## 3. How every request knows its campaign

### 3.1 URL scheme

| Kind | Shape | Examples |
|---|---|---|
| SPA, campaign screens | `/c/{slug}/...` | `/c/sample-colony/`, `/c/sample-colony/timeline`, `/c/sample-colony/pending` |
| SPA, account screens | top-level | `/login`, `/` (picker), `/join/{inviteCode}`, `/request`, `/admin` |
| API, campaign | `/api/campaigns/{campaignId}/...` | `GET .../colony`, `POST .../members/{memberId}/approve`, `GET .../stream` |
| API, account | `/api/...` | `GET /api/me`, `GET /api/campaigns`, `GET /api/invites/{inviteCode}`, `POST /api/invites/{inviteCode}/join`, `POST /api/campaign-requests` |
| API, owner | `/api/admin/...` | `GET /api/admin/campaigns`, `POST /api/admin/campaign-requests/{campaignRequestId}/approve`, `POST /api/admin/campaigns/{campaignId}/suspend` |
| Token | `/calendar/{token}.ics` (M5) | |

**Why path-scoped.** The campaign is explicit in every request, so there is no hidden state to go stale
(the old app's `active_campaign_id` cookie needed "clear on stale campaign" logic); two tabs can sit in two
campaigns; Beny Bot's links land in the right campaign; openapi-react-query keys contain the path
parameters, so caches of different campaigns never mix; and the secrecy sweep fills `{campaignId}` like
any other path parameter. **Rejected:** a cookie or header (invisible, can't be linked, and EventSource
can't send headers); subdomains (wildcard DNS and certificates at M6, cookies per host).

**Slug in the SPA, id in the API.** Slugs read well in a Discord message; uuids never change and are what
the sweep fills. The slug is fixed after approval (C2), so no redirect table. The SPA resolves slug → id
from `GET /api/campaigns` (the caller's memberships). A slug not in that list shows "No such campaign",
the same screen whether the campaign exists or not; the API gives the same answer (§4.1).

### 3.2 Caller, Viewer and where the campaign is resolved

```go
// Caller is who is asking, before any campaign: built by the auth middleware from the session cache.
type Caller struct {
    AccountID        uuid.UUID
    DiscordUserID    string
    IsSiteOwner      bool   // derived from SITE_OWNER_DISCORD_ID on every request
    SessionTokenHash []byte
    ViewAsPlayer     bool   // the session's toggle
}

// Viewer is someone inside one campaign. Only core builds one, from a membership read in the same call,
// so a Viewer is proof of membership. Its fields are unexported; it has no IsSiteOwner, so campaign code
// cannot consult owner status even by mistake. Its zero value names no campaign, and core refuses it.
type Viewer struct {
    accountID, memberID, campaignID uuid.UUID
    sessionTokenHash                []byte // nil for Beny Bot
    viewAsPlayer                    bool
    role                            CampaignRole
    status                          MemberStatus
    campaignStatus                  CampaignStatus
}
// CampaignID(), MemberID(), AccountID(), IsDM(), IsActive(), SeesDmFields() (IsDM && !viewAsPlayer),
// CampaignActive().

func (service *Service) EnterCampaign(ctx context.Context, caller Caller, campaignID uuid.UUID) (Viewer, error)
func (service *Service) ViewerForGuild(ctx context.Context, guildID, discordUserID string) (Viewer, error) // Beny Bot
```

- **No membership cache.** `EnterCampaign` runs one indexed read (`campaign_member` joined to `campaign` on
  `(campaign_id, account_id)`); no row → `NotFound("No such campaign.")`. Every campaign request already
  queries for its data, so the cost is one extra round trip, and there is no stale grant to evict. An idle
  open tab still runs no query (Invariant 9); a **reconnect runs exactly one read**, the membership, and no
  write (SPEC §8.8 is amended; revisit with a cache at M6 only if Neon wake-ups show up in the bill).
- **The access layer resolves the campaign, once, before the handler.** For a huma operation at a campaign
  level, the access middleware reads the `campaignId` path parameter **before huma validates input**: an
  unparsable ID answers the same 404 "No such campaign." as an unknown one (never huma's 422). It then calls
  `EnterCampaign`, decides the level against the Viewer and puts the Viewer in the context. `guardRoute`
  does the same for the stream. Handlers call `campaignViewerFrom(ctx)` and pass it to one core function.
- Every campaign-scoped core function takes a `Viewer` and passes `viewer.CampaignID()` to every query.
  `requireDm` and `requireActive` gain `requireCampaign(viewer)` (non-nil campaign, campaign ACTIVE).
  Account functions (my campaigns, invites, requests) take a `Caller`; owner functions take a `Caller` and
  call `requireSiteOwner`.
- A campaign deleted between `EnterCampaign` and a read leaves no colony or settings row: core maps that to
  `NotFound("No such campaign.")`, never a 500. A missing account row stays `sessionEnded` (today's reset
  handling); with no membership cache there is nothing else to flush.
- Beny Bot gets a Viewer only from `ViewerForGuild` (one read joining guild link, campaign, account and
  membership). The seed gets one from `EnterCampaign` with a Caller for the sample DM's account.
- **As built in M6: still no cache.** Counted per path, the membership read is the only query of a request
  only on a live-stream connect; elsewhere it rides beside the operation's own reads, so a cache would save
  a round trip but never a Neon wake-up. Hidden tabs close their stream, so only a tab visible on a screen
  that stays on wakes Neon, once per 10-minute stream rotation (about 1 CU-hour a night per such tab). The
  trigger to add a cache for stream reconnects is in SPEC.md §10 "As built in M6".

### 3.3 The structural guarantees

No single guard is enough; together they make a cross-campaign read or write hard to write and unlikely
to merge unnoticed.

1. **Types.** Campaign data is reachable only through core functions that take a `Viewer`, and a Viewer
   exists only after a membership read. The compiler finds every M1 call site that must change.
2. **SQL scope lint** (`db/scope_test.go`, part of `go test`, so of `mise run check`). It migrates a fresh
   test schema, reads `information_schema.columns` for every table with a `campaign_id` column, then reads
   each `-- name:` block in `db/queries/*.sql`. It is a **token heuristic, not a SQL parser**; it fails when:
   - a query names a campaign-owned table and has no `@campaign_id` (or `sqlc.arg(campaign_id)`) parameter;
   - a query names campaign-owned tables N times (each FROM, JOIN, subquery, CTE, and UPDATE, DELETE or
     INSERT target counts) but mentions the **column** `campaign_id` (not counting the parameter) fewer
     than N times. This catches `UPDATE ... FROM`, a subquery filtering only the outer table, and an M2
     colonist count that filters `role` but not `npc`;
   - a `JOIN` of a campaign-owned table has an `ON` or `USING` that doesn't mention `campaign_id`;

   unless the line above `-- name:` says `-- unscoped: <reason>`. The two directory tables that have a
   `campaign_id` column (`campaign_request`, `discord_guild_link`) are allow-listed with a reason each.
   Expected `-- unscoped:` queries in M1.5: `ListMembershipsForAccount`, `CampaignByInviteCode`,
   `ViewerForGuild`, the owner's counts. The lint's own test feeds it deliberately bad queries (each case
   above) and one good query of each shape. **The backstop is the fingerprinted sweep** (passes 3 and 4):
   a query the heuristic misses still has to survive A's DM exercising every operation while B's rows are
   hashed.
3. **Composite foreign keys** (§2.3 rule 2) make a cross-campaign **column** reference unstorable. They
   say nothing about jsonb or body IDs (§2.3 rule 5); pass 4 covers those.
4. **Scoped broadcasts.** `Broker.Publish(campaignID, topics...)` replaces `Publish(topics...)`; there is no
   unscoped broadcast to call.
5. **The cross-campaign sweep** (§4.4).

### 3.4 Row-level security: not now

RLS would catch a missing filter on a write path. Against it, today:
- the app connects as `colony`, the table owner and a superuser; superusers bypass RLS and owners need
  `FORCE ROW LEVEL SECURITY`, so it needs a second, non-owner login role in compose, CI, tests and M6;
- the campaign has to reach Postgres per transaction (`SET LOCAL app.campaign_id`), so a read that is one
  round trip today becomes BEGIN, set, query, COMMIT: four round trips to Neon at M6;
- directory lookups (by invite code, by guild, the owner's counts, the M5 planner) each need a policy
  exception, the same allow-list the lint keeps.

RLS would not cover jsonb or body IDs either (§2.3 rule 5). The lint, the composite keys and the
fingerprinted sweep cover the same failures at no runtime cost. **Revisit at M6** if friends' campaigns go
on the hosted database: a `colony_app` role without BYPASSRLS, policies `USING (campaign_id =
current_setting('app.campaign_id')::uuid)`, and core's `inTransaction` setting it. Nothing in M1.5 blocks it.

**As built in M6: not now.** The hosted database holds a handful of friends' campaigns, and every reason
above still holds on Neon (the app connects as the role that owns the tables). SPEC.md §10 "As built in
M6" records the decision and when to revisit it.

---

## 4. Access levels, redaction and the secrecy sweep

### 4.1 Levels

| Level | Admits | Used by |
|---|---|---|
| `public` | anyone | `/healthz`, sign-in, SPA shell, `/api/login-options`, `/auth/dev` (devauth) |
| `token` | a valid secret in the URL | `/calendar/{token}.ics` (M5) |
| `account` | any signed-in account (replaces today's `pending`) | `GET /api/me`, `GET /api/campaigns`, `GET` and `POST /api/invites/{inviteCode}...`, campaign requests, `POST /auth/logout`, `POST /auth/logout-everywhere` |
| `owner` | the site owner (view as player has no effect here) | everything under `/api/admin/`; `/api/openapi.json`, `/api/docs` (were `dm`) |
| `pending` | any membership in **this** campaign, whatever its status | exactly `GET .../me` and `GET .../stream` |
| `member` | ACTIVE membership, campaign ACTIVE | campaign reads, profile, view-as-player toggle |
| `dm` | this campaign's DM, not viewing as player, campaign ACTIVE | DM operations, including `GET .../invite` |

**Startup check (extended, fail closed):** every operation has a level (as today), and **a route uses
`pending`, `member` or `dm` if and only if it has a `{campaignId}` path parameter and is outside
`/api/admin/`**; `owner` appears only under `/api/admin/` and on the two docs routes. The server refuses
to start otherwise. (Admin routes may carry `{campaignId}`; they are `owner`.)

**Answers:** no session → 401. Signed in but no membership in the campaign (the owner included, members of
other campaigns included), or an unparsable campaign ID → **404 "No such campaign."**, identical to a
campaign that doesn't exist. Pending or removed member on a `member`/`dm` route → 403 "The DM hasn't let
you in." Suspended campaign on a `member`/`dm` route → 403 "This campaign is paused by the site owner."
`GET .../me` still answers (with `campaign.status`) so the SPA can show the right screen.

### 4.2 Redaction and what each DTO may carry

The Transformer keeps `dm` objects only when the context holds a campaign Viewer with `SeesDmFields()`.
Account and owner routes have no campaign Viewer, so their `dm` objects are always stripped; owner DTOs
are built from directory tables and have no `dm` objects at all. Beny Bot keeps building every reply from
the player projection.

The **invite code** is served only by `GET /api/campaigns/{campaignId}/invite` (`dm`) and returned by
`POST .../invite/reset` (`dm`). It is in no member, account, invite or owner DTO, so "Reset link" really
locks out anyone who had the old one, removed players included. The **link code** (§6.2) is returned only
by the `dm` operation that creates it.

### 4.3 Permission matrix (adds to SPEC.md §2.3)

| Action | Player | DM | Site owner (not a member) |
|---|---|---|---|
| Anything inside a campaign | as SPEC §2.3 | as SPEC §2.3 | **nothing: 404** |
| Join through an invite link | ask | — | ask, like anyone |
| Approve, deny, remove, re-admit joiners | — | own campaign | — |
| See or reset the invite link; link or unlink a Discord server; set channels | — | own campaign | — |
| Ask to run a campaign | any account | any account | auto-approved |
| See open campaign requests; approve or deny | — | — | yes |
| See every campaign's name, slug, DM (name and @username), player and pending counts, Discord linked yes/no, status, created date | — | — | yes |
| Suspend, unsuspend, delete a campaign | — | — | yes |

The owner who is also a DM (the author) has both columns, in their own campaign only. "Manage, not read"
is the app's promise, not a technical wall: the owner runs the server, the database and Beny Bot. The
request form says so in one line: **"The site owner runs the server and Beny Bot. The app never shows them
what happens in your campaign."** Two rules keep the operator's view small: Beny Bot is invited without
Read Message History (§6.1), and **logs carry IDs, kinds, counts and statuses, never campaign content or
message text** (a new invariant, §9.2).

### 4.4 Extending the secrecy sweep

`internal/app/harness_test.go` builds these accounts and campaigns. **B is created through the real path**
(`RequestCampaign`, then `ApproveCampaignRequest` by the owner), so the owner's request list is exercised.

| Account | Campaign A | Campaign B |
|---|---|---|
| `dmA` | DM | ACTIVE player |
| `ana` | ACTIVE player | ACTIVE player |
| `otherA`, `pendingA`, `removedA` | as today | — |
| `dmB` | ACTIVE player | DM |
| `bOnly` | — | ACTIVE player |
| `outsider` | — | — |
| `owner` (the harness's site owner) | — | — |

**Tokens.** Three, none a substring of another except where intended:
- `canaryA` in A's DM-only values (today's `canary`, renamed);
- `canaryB` in every value of B that players of B may see (colony name, resource labels, an extra resource
  `brine` that only B has, public timeline titles, character names, the undo label);
- `canaryBdm` in B's DM-only values (hidden timeline entries, notes, channel IDs). It contains `canaryB`,
  so "no `canaryB`" also forbids it.

B's directory data (campaign name, slug, DM display name and username) carries no token, because the
owner legitimately sees it. The forbidden lists also hold **literal values**: A's invite code and link
code (DM-only in A), B's invite code and link code (DM-only in B).

Passes, over every OpenAPI operation and every route-table route:

| Pass | Callers | Paths and bodies | Must hold |
|---|---|---|---|
| 1 players of A | `ana`, `otherA`, **`dmB`** (a DM elsewhere), `dmA` viewing as player, `pendingA`, `removedA`, visitor | A's | today's player rules (every `dm` route 403); no `canaryA`, no `canaryB`, no A literals |
| 1b players of B | **`dmA`** (a DM elsewhere), `ana` | B's | every `dm` route 403; no `canaryBdm`, no `canaryA`, no B literals |
| 2 non-members | `bOnly`, `outsider`, `owner` | A's | every campaign route **404**, with a malformed `campaignId` too; no token or literal |
| 3 A's DM | `dmA`, every operation including mutations | A's | no 5xx; may hold `canaryA`, **never `canaryB`** |
| 4 mixed IDs | `dmA`, every operation | `{campaignId}` = A; then, one at a time, every other path ID from B, every body field with `format: uuid` set to a real B ID of the matching kind, every resource or role key field and every `Effects.resources` key set to `brine` or a B-only role key | a 4xx (404, 409 or 422), never 2xx or 5xx; never `canaryB` |
| 5 suspended | A suspended; `dmA`, `ana`; `outsider` on A's invite | A's; `GET`/`POST /api/invites/{A's code}` | 403 except the two `pending` routes; the invite reads "paused" and the join answers 409 and creates no membership; no token |
| 6 owner | `owner` | `/api/admin/*`, including the requests list that approved B | 200 on reads; no token or literal |

Pass 4 reads body schemas from the OpenAPI document; key fields are those whose name ends in `Key` or
`key` and the property names of `Effects.resources` (a sweep test fails if an operation gains an
unclassified map field, so new ones get listed).

**B's fingerprint.** After the fixtures and pass 1b, a helper hashes: every row of every table with a
`campaign_id` column `WHERE campaign_id = B` (found from `information_schema`, so new tables are covered);
the `campaign` row `WHERE id = B`; the `account` rows of accounts whose only memberships are in B
(in the harness, `bOnly`); and those accounts' `web_session` rows,
leaving out `last_seen_at` and `expires_at`, which sign-in bookkeeping may touch. After passes 2–6 the
hash must be unchanged. Passes 3 and 4 are what catch an `UPDATE ... WHERE key = @key` that forgot the
campaign, or a body ID loaded without it.

Also:
- `TestOnlyTwoRoutesAdmitPendingMembers` lists the two campaign `pending` routes.
- `TestSiteOwnerChangesNoCampaignDecision`: a table test over every campaign level × membership state ×
  `IsSiteOwner` true/false gives the same answer either way (the owner who DMs A gets 404 on B).
- Joining: a REMOVED member's `POST /api/invites/{code}/join` leaves the membership REMOVED and sends no
  ping; a suspended campaign's join answers 409.
- Streams: `dmB` advancing B sends nothing to any stream of A (including `ana`'s and `dmB`'s own A
  streams); approval in B reaches only the joiner's B stream; stream IDs of A are unaffected by B's
  traffic (per-campaign sequence).
- Beny Bot: in a guild linked to A, a Discord user who is only in B gets "You're not in this campaign",
  with no token; `/link` refusals (§6.2) are tested; after A is deleted, `/beny` there says "not linked";
  Ready unlinks a linked guild that is missing from its guild list, and a Ready that lists no guild
  unlinks nothing.
- The startup-check tests cover the path/level rule, including an `/api/admin/campaigns/{campaignId}/...`
  route at `owner` (allowed) and a `{campaignId}` route at `account` (refused).

---

## 5. Flows and screens

### 5.1 Screens

| Route | Who | Shows |
|---|---|---|
| `/login` | visitor | as today, plus "Run your own colony campaign? Sign in and ask." Devauth: dev sign-in grouped by role (§7.3). |
| `/` | account | **Picker**: "Your campaigns" cards (name, your role or character, status: Waiting, No access, Paused); "Have an invite link? Paste it"; "Run your own campaign" (or your request's status). Goes straight to the last campaign you opened (localStorage) when you're still ACTIVE there. |
| `/join/{inviteCode}` | account (visitor → login → back) | "Join **Sample Colony**, run by Sample DM?" → **Ask to join**; or your status there ("You don't have access to Sample Colony" for REMOVED, no button); or "Sample Colony is paused by the site owner" (no button); or "This invite link doesn't work any more. Ask your DM for a new one." |
| `/request` | account | the request form, then its status |
| `/admin` | owner | **Requests** (open ones) and **Campaigns** (directory columns; ⋮ menu: Copy ID, Suspend, Unsuspend, Delete…) |
| `/c/{slug}/pending` | pending, removed member | "Waiting for Sample DM to let you in to Sample Colony" (opens live), or "You don't have access", or "This campaign is paused" |
| `/c/{slug}/...` | members | today's screens, unchanged inside; Settings gains **Campaign name** (with "The site owner sees this name"), **Invite link** and **Discord server** cards |
| Top bar | members | the campaign name becomes a switcher when you have more than one campaign. The profile menu adds "All campaigns" and "Site admin" (owner). |

Pages outside a campaign (`/`, `/join`, `/request`, `/admin`) have no stream: their queries refetch on
window focus and after their own mutations.

### 5.2 Asking to run a campaign → owner approval → campaign

1. Any signed-in account opens **Run your own campaign** (`/request`). Fields: campaign name (1–80),
   address (slug, suggested from the name, `/c/` shown in front, 3–40 of `a-z 0-9 -`), a note to the site
   owner (up to 500). Core checks: no open request by this account; the slug is free among campaigns and
   open requests. → `campaign_request` PENDING. The page reads "Waiting for the site owner", with
   **Withdraw** (to change the request, withdraw and ask again).
2. The owner hears of it: the `/admin` count on the profile menu (on focus) and, when Beny Bot is on, a
   direct message from Beny Bot to the owner ("Mira (@mira) asks to run Sky Isles: <url>/admin"), at most
   5 an hour.
3. On `/admin` → Requests, the owner sees the requester's avatar, name, @username, the campaign name,
   address and note. **Approve** (the address can be changed here) or **Deny**.
4. **Approve** runs one transaction: lock the request (must be PENDING); re-check the slug; insert
   `campaign` (ACTIVE, random invite code), the DM `campaign_member` (ACTIVE, approved now), `settings` and
   `colony` (column defaults), the default resources and roles (§2.5); mark the request APPROVED with its
   `campaign_id`. `/request` then shows "Sky Isles is ready" → **Open it**. The first visit lands on an
   empty board, Day 0, four resources and nine roles; while the timeline is empty the DM sees "Start in
   Settings: name your colony, pick your timezone, share your invite link."
5. **Deny**: the request becomes DENIED; the requester sees "The site owner said no this time" and may
   ask again (one open request per account and the owner-ping limit are enough). No decision note: the
   owner can tell them on Discord.
6. The owner's own request is approved on submit (it's their site).

### 5.3 Invite link → join request → DM approval

1. DM: Settings → **Invite link** (`GET .../invite`): `https://<site>/join/<code>`, **Copy**, **Reset
   link…** ("The old link stops working at once. People already waiting stay waiting.").
2. A friend opens the link. Not signed in: `/login` (return-to remembered) → back. Signed in:
   `GET /api/invites/{code}` returns the campaign name, its DM's display name, the campaign's status and
   the caller's membership status there (if any); unknown code → 404 "This invite link doesn't work any
   more." **Ask to join** → `POST /api/invites/{code}/join`, which under a per-campaign advisory lock:
   - campaign SUSPENDED → 409 "This campaign is paused by the site owner.";
   - existing membership → returned **unchanged**, no ping (ACTIVE → go in; PENDING → waiting; REMOVED →
     "You don't have access");
   - 20 pending already → 409 "Not accepting new players right now.";
   - otherwise a PENDING membership → `/c/{slug}/pending`.
3. Beny Bot posts "<name> (@user) is waiting to join: <url>/c/<slug>/players" to that campaign's DM channel
   in its linked server, at most 5 an hour per campaign.
4. The DM approves, denies, removes or re-admits on `/c/{slug}/players`, as today. Approval opens the
   joiner's waiting page live (it holds the campaign stream at the `pending` level).

A first Discord sign-in now only creates the account; it no longer makes anyone pending or pings anyone.

### 5.4 Switching

The top-bar switcher (or `/`) moves between campaigns. Each campaign has its own stream and caches
(keyed by campaign ID). A person who is DM in one campaign and a player in another sees DM tools only in
the first.

### 5.5 Suspend and delete (owner)

- **Suspend** (confirm: "Everyone in Sky Isles loses access until you unsuspend it. Nothing is deleted."):
  status SUSPENDED. At once: every member's campaign routes answer 403 except the two `pending` routes; the
  broker rescopes the campaign's streams to `me` and publishes `me`, so open tabs show "This campaign is
  paused by the site owner"; the invite link shows "paused" and refuses joins; the campaign's live link
  code is dropped; Beny Bot answers commands in its server with that line and posts nothing; the M5
  planner skips it; Discord events already created stay. **Unsuspend** restores everything and publishes
  `me`, so open tabs come back without a reload.
- **Delete…** appears only for a suspended campaign. The dialog lists what goes: the colony, its timeline,
  settings, resources and roles, every membership (and later NPCs, quests, patrons, requests, maps, places,
  hexes, images, sessions), the invite link and the Discord link; and what stays: everyone's account and
  their other campaigns. "This can't be undone. Type **sky-isles** to confirm." →
  `POST /api/admin/campaigns/{campaignId}/delete {"confirmSlug": "sky-isles"}`. Core checks SUSPENDED and
  the slug, `DELETE FROM campaign` (everything cascades), and the broker closes the campaign's streams and
  drops its resume ring (`Broker.CloseCampaign`). Open tabs reconnect, get 404 and show "No such
  campaign". Beny Bot stays in the server; its commands there now say "not linked".
- There is no DM-initiated delete in M1.5 (C3): the DM asks the owner.

---

## 6. Beny Bot: one shared bot

### 6.1 One application, many servers

- One Discord application (Beny Bot Dev locally; one production application at M6). Each DM invites it to
  their own server with the invite URL shown on Settings. **Permissions drop Read Message History** (no
  feature reads messages): View Channel, Send Messages, Embed Links, Manage Events.
- `DISCORD_GUILD_ID` is removed. **Commands are registered globally** on Ready
  (`ApplicationCommandBulkOverwrite(app, "", commands)`). The author's old guild-scoped `/beny` is cleared
  once: `docs/discord-dev-setup.md` tells the author to keep `DISCORD_GUILD_ID` for one run, during which
  Ready overwrites that one guild's commands with `[]` and logs "old guild commands cleared; delete
  DISCORD_GUILD_ID from .env". `loadConfig` reads it only for this.
- Intents stay `Guilds | GuildScheduledEvents`. **Stale links:** `GuildDelete` with `Unavailable == false`
  (the bot was removed) unlinks that server; and on every Ready, core unlinks every linked guild that is
  not in Ready's guild list (the bot was removed while offline, which is common locally). A Ready that
  lists no guild at all while links exist is taken as incomplete: it unlinks nothing and logs a warning.
  Either way the DM's Settings shows "Not linked", and re-inviting the bot needs a new `/link`.

### 6.2 Linking a server to a campaign

Two consents are needed: the **DM's** (a code only their Settings page shows) and the **server's** (a
member with Manage Server runs the command there). Neither side can attach the other alone.

1. DM: Settings → **Discord server** → "1. Invite Beny Bot (link). 2. **Get a link code**." Core makes a
   10-character code from `23456789ABCDEFGHJKMNPQRSTUVWXYZ` (about 49 bits), shown as `BENY-XXXXX-XXXXX`,
   and keeps it **in memory** (`internal/core/linkcodes.go`: code → campaign and expiry, one live code per
   campaign, a new one replaces it), for 15 minutes. A restart forgets it; the DM gets a new one. The card
   shows the code, its expiry and "In your server, run `/link code:BENY-...`".
2. In the server, someone runs `/link code:...`. The command is registered with
   `DefaultMemberPermissions: ManageGuild` and the handler checks again
   (`interaction.Member.Permissions & (ManageGuild | Administrator)`), because server admins can override
   command visibility. Refused in direct messages.
3. Core `LinkGuild(code, guildID, guildName, discordUserID)`: look the code up (unknown or expired: "That
   code doesn't work. Get a new one in your campaign's Settings."); then in one transaction, holding the
   campaign row's lock, refuse if the server is linked to any campaign ("This server is already linked to
   a campaign. Its DM can unlink it in Settings, or run /unlink here.", without naming it), if the
   campaign is linked to another server ("Unlink the other server in Settings first.") or if the campaign
   is suspended; insert `discord_guild_link`; clear the campaign's channel settings; and only then use the
   code up. A refused `/link` leaves the code working until it expires, and two `/link` runs with one code
   can't both link.
4. Ephemeral reply: "Linked this server to **Sky Isles**. Pick channels in the campaign's Settings." The
   Settings card (live, `settings` topic) shows "Linked to <server name>" with **Unlink…**.
5. Wrong codes: the existing per-user limiter (burst 3, one every 2 s) is enough against a 49-bit,
   15-minute, single-use code.

### 6.3 Routing every interaction

Every command, autocomplete (M2), button (M3) and scheduled-event update (M5):
1. No guild (a direct message) → "Use Beny Bot in your campaign's server."
2. `ViewerForGuild(guildID, discordUserID)`, one read, answers one of: not linked → "This server isn't
   linked to a campaign yet. A DM gets a code in their campaign's Settings and runs /link here." (except
   `/link`); suspended → "This campaign is paused by the site owner."; no account or no membership →
   "You're not in this campaign. Ask its DM for the invite link."; not ACTIVE → "The DM hasn't let you in
   yet."; otherwise a Viewer.
3. As today: player projection, ephemeral replies, no mentions.
4. Buttons (M3): `req:{requestId}:grant` decides with the guild's campaign Viewer; core loads the request
   with `campaign_id = viewer.CampaignID()`, so a button can only decide that campaign's requests, and only
   its DM may press it.

### 6.4 Posting and channels

- Pings, request prompts, reminders and event notices go through one bot function
  `postToCampaign(campaignID, channelKind, message)`. It reads the campaign's link and channel and **checks
  the channel belongs to the linked server** (gateway state, `Channel.GuildID`) before sending; otherwise it
  logs the IDs and skips. A DM can never make Beny Bot post into someone else's server.
- **Channel picker:** Settings lists the linked server's text channels instead of asking for IDs. Core
  gains one outward seam, `core.DiscordDirectory` (`TextChannels(ctx, guildID) ([]DiscordChannel, error)`),
  implemented by the bot from gateway state; `UpdateSettings` refuses a channel not in that list. Without a
  link (or with Beny Bot off) the channel fields are disabled with "Link a Discord server first". When the
  gateway can't see the linked server right now (just after a restart, during an outage, or once Beny Bot
  has left it), `TextChannels` returns `core.ErrDiscordListUnavailable`: the picker is empty and says why,
  and saving Settings keeps the stored channels and refuses only a change.
- Rate limits are per campaign: 5 waiting pings an hour per campaign, so one busy campaign never silences
  another.

### 6.5 Unlinking

The DM's **Unlink…** on Settings, `/unlink` by a Manage Server member in the server, or the bot being
removed (§6.1) each delete the link row and clear the channels. As built in M5, it also forgets the events
of the campaign's unplayed dates, and Beny Bot deletes, best effort, the ones for dates still ahead in the
old server (a bot that was removed can't, so those stay); a server linked later gets its own events.

---

## 7. Live updates, sessions, dev sign-in, seed, DbGate

### 7.1 Live updates

- One stream kind: `GET /api/campaigns/{campaignId}/stream` (`pending` level). A tab inside a campaign has
  one open; pages outside campaigns have none (§5.1).
- `Subscription` gains `campaignID`; its `memberID` is the `campaign_member` ID. `recipients` gains the
  campaign: a **broadcast reaches only ACTIVE streams of that campaign**; member messages target a
  membership; session messages (the view-as-player toggle) reach that session's streams in every campaign.
- **Per-campaign log.** The broker keeps `map[campaignID]*campaignLog`, each with its own `sequence` and
  ring of 256 messages. Stream IDs stay `bootId-seq` but count only that campaign's messages, so a player
  learns nothing of other campaigns' activity, and a busy campaign never pushes another's messages out of
  its ring. A session message goes into the ring of each campaign where that session has a stream. Logs
  are created on first use and dropped by `CloseCampaign`.
- Status: `memberStatusChanged` (approve, deny, remove, re-admit) rescopes that membership's streams as
  today; `campaignStatusChanged` (suspend, unsuspend) rescopes all of a campaign's streams; both remember
  the newest state so a stream that subscribes with a Viewer read just before the change still gets it.
- Topics are today's (`me`, `members`, `settings`, `colony`, `timeline`, `resources`); `AllTopics()` is
  unchanged.
- Web: query option factories take `campaignId` (`colonyBoardQuery(campaignId)`); `topicQueryKeys` becomes a
  function of the campaign ID returning keys with `{ params: { path: { campaignId } } }`, which TanStack
  Query matches partially, so an invalidation refetches only that campaign's queries. `lastEventId` is kept
  per stream URL.

### 7.2 Sessions

- `web_session` is per account. The session cache entry holds `accountID`, `discordUserID`, `viewAsPlayer`
  and expiry; no status (status is per campaign now).
- `core.SessionEvictor` keeps `EvictSession` and `EvictAll`; `EvictMember` becomes `EvictAccount` (sign
  out everywhere). Removal from a campaign no longer touches sessions.
- The existing idle-tab test is unchanged (no query); the reconnect test now expects exactly one read and
  no write (§3.2).

### 7.3 Dev sign-in

- `/auth/dev?as=owner` signs in as the effective site owner; `as=dm`, `as=player`, `as=pending` keep
  meaning the DM, first active player and first pending joiner of Sample Colony (so the M0 and M1
  checklists still work); `as=<account id>` any account; `as=new&name=...` a brand-new account.
- `/login`'s dev list shows each account once with its roles: "Sample DM · site owner · DM of Sample
  Colony", "Ana · Thorin in Sample Colony · Brisa in Second Sample", "Hollis · DM of Second Sample · Pell in
  Sample Colony", "Wren · asking to run a campaign".
- In devauth builds with `SITE_OWNER_DISCORD_ID` unset, `seed:dm` is the owner.

### 7.4 The seed: two campaigns

`colony seed` creates, through core wherever an operation exists:
- **Accounts:** Sample DM (`seed:dm`, or `SITE_OWNER_DISCORD_ID` when set, so real Discord sign-in lands
  on it), Ana, Bram, Cass, Dov, Emil, Fen, Gus (as today), plus Hollis, Iko, Jun and Wren.
- **Sample Colony** (`/c/sample-colony`, colony Port Eldari): today's M1 sample, DM'd by Sample DM, so the
  M1 checklist's numbers hold. **Hollis is also an ACTIVE player here (Pell)**; the Players list gains him.
- **Second Sample** (`/c/second-sample`, colony Ridgehold), created through `RequestCampaign` and
  `ApproveCampaignRequest`: DM Hollis; players Ana (Brisa) and Iko (Tamsin Vale); Jun pending. Its own
  short history on different numbers (Day 9, Food 140), an extra resource Timber, and one DM-only
  adjustment (Materials −10), so isolation is easy to see.
- **A pending campaign request** from Wren: "Sample Sky Isles" (`sample-sky-isles`) with a note.
- `--hexes N` (M4) paints into Sample Colony.

The seed refuses a database that already has accounts.

### 7.5 DbGate

No change to `compose.yaml` beyond one comment line: DbGate connects as the database owner and shows
every campaign; it is the operator's tool, local only, and outside "manage, not read". Composite foreign
keys refuse most accidental cross-campaign edits made in it.

---

## 8. Limits

Constants in `internal/core/limits.go`, each with core's message. None is a game rule. Owner approval is
the gate for new campaigns, and the DM approves every player, so nothing else is capped.

| Limit | Value | When hit |
|---|---|---|
| Open campaign requests per account | 1 | "You already asked. Withdraw it to change it." |
| Pending joiners per campaign | 20 (today's cap, now per campaign) | "Not accepting new players right now." |
| Waiting pings | 5 an hour per campaign | still listed on Players |
| Owner request pings | 5 an hour | still listed on /admin |
| Link codes | one live per campaign, 15 minutes, single use | "That code doesn't work." |
| Text | campaign name 1–80, slug 3–40, note 500 | field errors |
| Set for M4 (noted in SPEC) | image storage 50 MB per campaign; 50,000 hexes per campaign | upload or paint refused with the reason |

---

## 9. Documents, plan, files, risks, effort

### 9.1 SPEC.md edits

| Section | Edit |
|---|---|
| Header, F-list | Add **F8. Multi-campaign** (the author's four decisions) and point to this document. |
| §1.1 | "a private website for colony campaigns; the author's Colony Campaign is the first". |
| §1.3 principle 1 | Replace with: "**Many campaigns, each sealed.** One DM and one colony per campaign; every campaign-owned row carries `campaign_id`; the site owner manages campaigns but can't read them." Non-goals: remove "multiple campaigns or DMs"; add co-DMs, DM hand-off, self-service leave, a public campaign directory, campaign export or copy. Principle 9 reads "at the scale of a few campaigns of 1 DM and about 6 players". |
| §2.1 | New identities table: Visitor, Account (no memberships), Pending / Removed member (per campaign), Player, DM (stored role), Site owner (env). Joining moves to invite links; pings and caps per campaign. |
| §2.2 | Access levels of §4.1; the campaign resolved in the access layer; 404 for non-members; the sweep of §4.4. |
| §2.3 | Add the site-owner column (§4.3). |
| §3.4 | Re-dispose: "Create campaign, multi-campaign, active-campaign cookie" → **Redesign** (owner-approved campaigns, path-scoped, no cookie); "Campaign switcher / role change" → **Redesign** (switcher; no role change); "Email invites", "Join with code" → **Redesign** (invite link); "/registercampaign" → **Redesign** (`/link` with a one-time code); "Beny Bot Terms / Privacy" → reconsider at M6. |
| §4 | New §4.0 Directory tables; every table gets `campaign_id` per §2.3 (rule 5 included); "Singletons use `id CHECK (id = 1)`" becomes "one-row-per-campaign tables use `campaign_id` as primary key"; the colonist query filters both tables; §4.6 diagram gains `campaign (1) ─ (n) everything`. |
| §5.1 | The colony lock is per campaign. |
| §6 | Routes table prefixed `/c/{slug}`; the screens of §5.1; Settings cards; switcher. |
| §7 | One shared bot (§6 here); permissions without Read Message History (§7.1); global commands; `/link`, `/unlink`; routing; stale-link clean-up; channel picker; buttons check the guild's campaign and its DM instead of `DM_DISCORD_ID`. |
| §8.4–8.5, 8.8, 8.10, 8.11 | Levels; `SITE_OWNER_DISCORD_ID`; sessions per account; a reconnect runs one membership read (§3.2); per-campaign streams and rings; env list drops `DM_DISCORD_ID`, `DISCORD_GUILD_ID`, adds `SITE_OWNER_DISCORD_ID`; logs carry no campaign content; tests. |
| §9 | Two sample campaigns, the owner, a pending request (§7.4). |
| §10 | Insert **M1.5: Multi-campaign** between M1 and M2 (scope = this document; acceptance = §10 here). M2–M6 scopes gain the lines of §9.3. |
| §11 | Add the decisions of "Decisions to confirm" below with the author's answers. |
| §12, Appendix A | Add the risks of §9.5; rulings: path scoping, one DM, no RLS, no membership cache, link codes in memory, global commands. |

### 9.2 CLAUDE.md edits

CLAUDE.md is 170 lines; there is no length target. Each addition is one bullet, and the DM bullet and
`DM_DISCORD_ID` mentions they replace go.
- Opening: "A private website for colony campaigns (several DMs, each with about six players)..."
- **Boundaries:** core reaches outward through `core.Notifier`, `core.SessionEvictor` and
  `core.DiscordDirectory`.
- **Invariants**, add:
  - "Every campaign-owned table carries `campaign_id NOT NULL REFERENCES campaign ON DELETE CASCADE`; text
    keys are unique per campaign; column references between campaign rows are composite foreign keys.
    Every query on a campaign-owned table filters each such table by `campaign_id`, or says
    `-- unscoped: <reason>` (`db/scope_test.go`). IDs and keys from jsonb or request bodies are loaded
    with the Viewer's campaign."
  - "A `core.Viewer` is proof of membership: only core builds one (`EnterCampaign`, `ViewerForGuild`).
    Campaign functions take a Viewer; account functions take a Caller; owner functions check
    `IsSiteOwner` and read only directory tables."
  - "Broadcasts are per campaign: `Broker.Publish(campaignID, topics...)`."
  - "Logs carry IDs, kinds, counts and statuses, never campaign content or message text."
  - Invariant 3's level list becomes: public, token, account, owner, pending, member, dm.
- **API details:** replace the DM bullet with: "The DM is the campaign's `campaign_member` with role DM
  (exactly one; no API writes `role`). The site owner is `Caller.IsSiteOwner`, from
  `SITE_OWNER_DISCORD_ID` on every request (`seed:dm` in devauth builds when unset); a Viewer has no owner
  flag." Dev sign-in line: `?as=owner|dm|player|pending|<account id>`.
- **Recipes:** add-an-entity step 1 gains "with `campaign_id` per the invariant"; step 5 becomes "plant
  into both campaigns (`canaryA`, `canaryB`, `canaryBdm`; SPEC §8.11 has the details)".

### 9.3 The M2–M6 plan

- **M2:** `npc`, `place` per §2.6; `/npc view` routes by guild; fixtures plant into A and B; the role key
  join includes `campaign_id`; the colonist count filters both tables.
- **M3:** quests, patrons, actions, requests per §2.6; request IDs in bodies and buttons loaded with the
  Viewer's campaign (pass 4 covers them); Grant and Decline check the guild's campaign and its DM; prompts
  go through `postToCampaign`.
- **M4:** maps, pins, hexes, images per campaign; images under `/api/campaigns/{campaignId}/images/`;
  Eldari offered, not seeded (C6); the per-campaign image and hex limits.
- **M5:** series, sessions, ICS per campaign; one planner across ACTIVE, linked campaigns (its queries
  `-- unscoped: planner`); events in each campaign's linked server; reminders through `postToCampaign`.
- **M6:** one production application shared by every DM; `SITE_OWNER_DISCORD_ID` in Railway; budget for
  several campaigns on Neon; decide RLS (§3.4) and whether a membership cache is worth it (§3.2); a short
  privacy page now that friends' data is hosted (the bot stays under Discord's 100-server verification
  line). As built: the live site uses Beny Bot Dev for now (SPEC.md §11, question 19), RLS and the
  membership cache are not now (§3.2, §3.4), and the privacy page is `/privacy`; `docs/cutover.md` is the
  author's runbook.

### 9.4 File-by-file change list for M1.5

**Database**
- `db/migrations/0003_campaigns.sql` (new, §2.4); `db/migrations_test.go` (new, §2.4).
- `db/queries/campaigns.sql` (new): campaign create and read by id, slug, invite code; my memberships;
  defaults; invite read and reset; suspend, unsuspend, delete; owner listing with counts; requests; guild
  links.
- `db/queries/members.sql` → `accounts.sql` (account upsert, last seen) and `campaign_members.sql`
  (membership read for `EnterCampaign`, per-campaign list, status, character name, pending count, join lock).
- `db/queries/sessions.sql`, `settings.sql`, `colony.sql`, `resources.sql`, `roles.sql`, `timeline.sql`:
  `@campaign_id` everywhere (including `GetTimelineEvent` and `MarkTimelineGroupUndone`); `WHERE id = 1` gone.
- `db/scope_test.go` (new): the lint of §3.3 and its bad-query cases.
- `internal/store/*`: regenerated.

**Core** (`internal/core/`)
- `viewer.go`: `Caller`, `Viewer` (unexported fields, no owner flag), `EnterCampaign`, `ViewerForGuild`;
  `SeedDmDiscordID` stays as the devauth owner default.
- `core.go`: `Options.SiteOwnerDiscordID`, `Options.Directory`; `requireCampaign`, `requireSiteOwner`;
  missing colony or settings row → NotFound.
- `limits.go`, `linkcodes.go` (new).
- `campaigns.go` (new): `MyCampaigns`, `CampaignMe`, `RequestCampaign`, `MyCampaignRequest`,
  `WithdrawCampaignRequest`, `Invite`, `InviteInfo`, `JoinByInvite`, `ResetInviteLink`.
- `admin.go` (new): `ListCampaignRequests`, `ApproveCampaignRequest`, `DenyCampaignRequest`,
  `ListCampaigns`, `SuspendCampaign`, `UnsuspendCampaign`, `DeleteCampaign`.
- `discord.go` (new): `CreateLinkCode`, `LinkGuild`, `UnlinkGuild`, `ReconcileGuilds` (Ready),
  `DiscordChannels` (moved from settings.go, per campaign), `DiscordDirectory` interface.
- `members.go`, `signin.go`, `sessions.go`, `login.go`: account versus membership split; sign-in only
  upserts the account; removal no longer deletes sessions.
- `settings.go`, `colony.go` (`arrivalFor`, Undo), `resources.go`, `timeline.go`: the Viewer's campaign in
  every query and every body-ID lookup.
- `broker.go`: per-campaign logs, campaign-scoped recipients, `campaignStatusChanged`, `CloseCampaign`.
- `notify.go`: `MemberWaiting` carries the campaign; `CampaignRequested` added.
- Re-exports `CampaignRole`, `CampaignStatus`, `CampaignRequestStatus`; `boundary_test.go` allow-list;
  tests updated (`members_test.go`, `colony_test.go`, `broker_test.go`, `sessions_test.go`,
  `timeline_test.go`) plus `campaigns_test.go`, `admin_test.go`, `discord_test.go`.

**App** (`internal/app/`)
- `access.go`: the seven levels; campaign resolution before input validation in both middlewares; the
  path/level startup rule.
- `app.go`: route table (campaign stream; docs at `owner`).
- `campaigns.go`, `admin.go`, `discord.go` (new thin handler files).
- `colony.go`, `timeline.go`, `resources.go`, `settings.go`, `members.go`, `me.go`: paths under
  `/api/campaigns/{campaignId}`; `stream.go`: campaign stream; `transform.go`: campaign Viewer;
  `enums.go`: the three new enums.
- Tests: `harness_test.go` (§4.4 accounts, B through approval), `secrecy_test.go` (passes, body
  substitution, fingerprint), `secrecy_fixtures_test.go` (plant per campaign with the three tokens),
  `access_test.go`, `members_test.go`, `colony_test.go`, `host_test.go`, new `campaigns_test.go`,
  `admin_test.go`.

**Auth** (`internal/auth/`): `auth.go` (`SiteOwnerDiscordID`), `session.go` (Caller), `cache.go` (account
entries), `devauth.go` and `devauth_disabled.go` (`EffectiveSiteOwnerDiscordID`, `as=owner`), `discord.go`
(callback no longer creates a pending member), tests.

**Bot** (`internal/bot/`): `bot.go` (global registration, one-time legacy guild clean-up, guild routing,
GuildDelete, Ready reconciliation), `commands.go` (`/link`, `/unlink`), `invite.go` (no Read Message
History), `notify.go` (`postToCampaign`, per-campaign limiter, owner DM), `directory.go` (new,
`TextChannels`), tests.

**Seed and command:** `internal/seed/seed.go`, `seed_test.go` (§7.4); `cmd/colony/config.go`
(`SITE_OWNER_DISCORD_ID`; refuse `DM_DISCORD_ID` for every subcommand; legacy `DISCORD_GUILD_ID` for the
clean-up only), `serve.go`, `seed.go`; `.env.example`.

**Web** (`web/src/`)
- Routes: `routes/_signedIn.tsx` (account guard) with `index.tsx` (picker), `request.tsx`, `admin.tsx`,
  `join.$inviteCode.tsx`; `routes/_signedIn/c.$campaignSlug.tsx` (resolve slug, not-found and paused
  screens) with `pending.tsx` and a `_shell` layout holding today's `_app/*` screens; remove
  `routes/_app.tsx`, `routes/_app/*`, `routes/pending.tsx`; `login.tsx` (grouped dev list).
- `api/queries.ts` (campaign-parameterized factories, new reads, refetch on focus outside campaigns),
  `api/topics.ts` (campaign-keyed map), `api/stream.ts` (campaign stream URL).
- `hooks/use-campaign.ts`, `hooks/use-me.ts`, `lib/access.ts` (account Me and campaign Me),
  `components/shell/*` (switcher, links with `$campaignSlug`, profile menu entries), Settings cards
  (`campaign-name-card.tsx`, `invite-link-card.tsx`, `discord-link-card.tsx`), the DM's empty-board hint,
  `lib/storage.ts` (last campaign).

**Docs:** `docs/SPEC.md`, `CLAUDE.md`, `docs/discord-dev-setup.md` (one shared bot, `/link`, the one-time
guild clean-up), `docs/checklists/M1.5.md` (§10 here).

### 9.5 Risks

| Risk | Likelihood / impact | Mitigation |
|---|---|---|
| A query misses its campaign filter: a leak, or an update to every campaign's `gold` | Medium / high | Viewer type; scope lint (heuristic); composite keys; tokens in every value of B; B's fingerprint after DM and mixed-ID passes. |
| A body or jsonb ID from another campaign is accepted | Medium / high | §2.3 rule 5; pass 4 substitutes B's IDs and keys into every body field. |
| Roles judged against the wrong membership (a DM of B gets DM powers in A) | Low / high | Viewer built per campaign with no owner flag; `dmB` in pass 1 and `dmA` in pass 1b; the owner-flag table test. |
| "Owner can't read" is only an app promise | Known / medium | Said plainly on the request form; owner routes read directory tables only; no Read Message History; logs carry no content; RLS decision at M6. |
| Beny Bot posts into the wrong server, or into a server that removed it | Low / medium | Channels picked from the linked server; `postToCampaign` checks the channel's guild; GuildDelete and Ready reconciliation unlink. |
| Someone links another campaign or another server | Low / medium | Code (DM) + Manage Server (server); 15-minute single-use codes; one-to-one link. |
| Migration 0003 makes the wrong account DM | Low / high | Every subcommand refuses `DM_DISCORD_ID`; the DO block refuses mismatched variables; tested on the two-account case. |
| Global commands appear late or doubled in the test server | Medium / low | One-time legacy clean-up; the checklist allows a minute. |
| Scope creep (co-DMs, hand-off, export, directory, leave) | Medium / medium | Non-goals in SPEC; decisions to confirm. |
| Hosting cost grows with friends' campaigns (M6) | Medium / medium | Owner approval; per-campaign image and hex caps; usage checked at M6. |
| Route move breaks screens and checklists | Medium / low | `as=dm|player|pending` unchanged; the sample campaign keeps M1's numbers; `/` remembers the last campaign. |

### 9.6 Effort

About **8–11 builder-days**, a little under M1, in five slices, each ending green on `mise run check`:
1. Schema, migration 0003 and its tests, queries, scope lint (2 days).
2. Core: Caller/Viewer, broker per campaign, every M1 function scoped (body IDs included), campaigns,
   admin, link codes (2.5).
3. App: levels, resolution, routes, the extended sweep with body substitution and fingerprint (2).
4. Beny Bot: global commands, routing, `/link`, `/unlink`, reconciliation, channel picker, posting (1).
5. Web: route move, picker, join, request, admin, Settings cards, switcher; seed; docs and checklist
   (2–3).

The author's check-in: about an hour with dev sign-in, plus 30 minutes for the Discord items.

---

## 10. M1.5 acceptance checklist (becomes `docs/checklists/M1.5.md`)

### Start it

1. Make sure Docker Desktop is running. If `mise run dev` is running, press Control-C in its tab.
2. In your `.env`, rename `DM_DISCORD_ID` to `SITE_OWNER_DISCORD_ID` and keep its value. (Every `colony`
   command refuses to run until you do.) Leave `DISCORD_GUILD_ID` in place for now.
3. **Your existing data first.** In Terminal:

   ```bash
   cd ~/myCodeSpace/benthedev/apps/colony-planner
   mise install
   mise run dev
   ```

   Open http://localhost:5173 and use Dev sign-in as your DM account. You land on your colony, now at an
   address starting with `/c/`. The day, stock, timeline and players are what you had. Press Control-C.
4. **The next command erases your current local data.** To keep a copy first:

   ```bash
   docker exec colony-planner-db pg_dump -U colony colony > ~/colony-before-M1.5.sql
   ```

   Then load the two sample campaigns:

   ```bash
   mise run fresh
   mise run dev
   ```

The steps build on each other. A private window counts as a second person; a second browser (Safari next
to Chrome) is a third. Pages outside a campaign (the list, Site admin, the request page) update when you
click into their window.

### Try it

1. **Sign-in page.** The Dev sign-in list shows each person once with their roles: "Sample DM · site owner
   · DM of Sample Colony", "Ana · Thorin in Sample Colony · Brisa in Second Sample", "Hollis · DM of Second
   Sample · Pell in Sample Colony", "Wren · asking to run a campaign".
2. **Your campaign is unchanged.** Sign in as **Sample DM**. You land on Sample Colony at
   `/c/sample-colony/`: Port Eldari, Day 22, Food 63, as in the M1 checklist.
3. **One person, two campaigns.** In a private window, sign in as **Ana**. The page lists Sample Colony
   (Thorin) and Second Sample (Brisa). Open Second Sample: Ridgehold, Day 9, Food 140, a Timber tile, and
   her name is Brisa. Use the campaign name in the top bar to switch to Sample Colony: Port Eldari, Thorin.
4. **Campaigns don't hear each other.** Put Ana on Second Sample. In your window, tap **+3** and advance.
   Ana's window shows no toast and stays on Day 9. Switch Ana to Sample Colony: Day 25.
5. **You run the site, not their game.** In your window open the profile menu → **Site admin**.
   - Campaigns lists Sample Colony (you) and Second Sample (Hollis, @hollis.sample) with player and waiting
     counts and "Discord: not linked". No day, stock or timeline appears.
   - Type `localhost:5173/c/second-sample` in the address bar: "No such campaign".
   - Back on Site admin, Second Sample's ⋮ menu → **Copy ID**. Open
     `localhost:5173/api/campaigns/<paste>/colony`: the browser shows a small JSON error, status 404,
     "No such campaign." The server itself refuses you, not just the page.
6. **Saying no to a campaign.** Requests shows Wren's "Sample Sky Isles" with her note. Tap **Deny**. In a
   private window sign in as **Wren**: she sees "The site owner said no this time" and **Run your own
   campaign** is offered again. Leave it.
7. **A friend starts a campaign.** In Safari, Dev sign-in → "A new person" → **Mira**. She lands on an
   empty list with **Run your own campaign**. She asks for "Sky Isles" with a note. She sees "Waiting for
   the site owner", with **Withdraw** and no way to send a second request.
   - Click into your window: Site admin shows the request. Tap **Approve**.
   - Click into Mira's window: "Sky Isles is ready". **Open it**: The Colony, Day 0, Gold, Food, Materials
     and Morale, nine roles, the DM tools, and a hint pointing to Settings. In **Settings** she names the
     colony "Gullhaven" and picks her timezone; the board shows Gullhaven.
8. **Invite link.** As Mira, **Settings** → **Invite link** → **Copy**.
   - In a private window in Chrome, sign in as a new person **Nico** and paste the link. "Join Sky Isles,
     run by Mira?" → **Ask to join** → "Waiting for Mira".
   - Mira's **Players** shows Nico without a refresh. **Let in**: Nico's window opens Gullhaven by itself.
   - Mira taps **Reset link…**. Opening the old link now says "This invite link doesn't work any more".
9. **Removal is per campaign.** In a private window sign in as **Hollis** → Second Sample → Players →
   remove **Ana (Brisa)**. Ana's Second Sample window shows "You don't have access" at once, and her Sample
   Colony tab keeps working without signing in again.
   - Hollis copies Second Sample's invite link; Ana opens it: "You don't have access to Second Sample", no
     **Ask to join** button. Hollis's Players shows no new waiting person.
   - Hollis re-admits her.
10. **A DM elsewhere is a player here.** As Hollis, switch to Sample Colony. He's Pell, a player: no +/−
    buttons, no Players management, no Invite link or Discord cards in Settings, and the Timeline has no
    "Stock adjusted" entry for Gold −40 on Day 19.
11. **View as player.** In your Sample Colony turn on **View as player**. Open Site admin: it still works.
    Go back: the banner is still on. Turn it off.
12. **Pausing a campaign.** On Site admin, **Suspend** Second Sample. Hollis's Second Sample window shows
    "This campaign is paused by the site owner" at once; Ana's Sample Colony is untouched.
    - In a private window, a new person **Quinn** opens Second Sample's invite link (Hollis copied it in
      step 9): "Second Sample is paused by the site owner", no **Ask to join** button.
    - **Unsuspend**: Hollis's window works again without a reload.
13. **Deleting a campaign.** Suspend Sky Isles. **Delete…** lists what will go. Type `sky-isles` and
    confirm. It disappears from Site admin; Mira and Nico still sign in and see an empty list. Open Sky
    Isles' current invite link: "This invite link doesn't work any more".
14. **Database view (optional).** `mise run db:studio` → http://localhost:54330. The `campaign` table has
    two rows, and `colony`, `resource`, `role`, `timeline_event` each have a `campaign_id` column.
15. **(Discord) Link your server.** With Beny Bot Dev set up (`docs/discord-dev-setup.md`, updated): the
    first start logs "old guild commands cleared"; delete `DISCORD_GUILD_ID` from `.env` afterwards. In
    Sample Colony **Settings → Discord server**, tap **Get a link code**. In your test server run
    `/link code:BENY-...`. Settings shows "Linked to <your server>". Pick your DM channel from the list.
    Allow a minute for commands to appear.
    - In Safari, a new person opens Sample Colony's invite link and asks: Beny Bot posts in that channel.
    - `/beny` answers "Boy!". Running `/beny` as a Discord account that isn't in Sample Colony says
      "You're not in this campaign".
    - A server member without Manage Server doesn't see `/link` (or is refused if your server shows it).
    - Get a code in Second Sample (as Hollis) and run `/link` with it in the same server: refused,
      "already linked". An expired or mistyped code is refused.
    - **Unlink…** in Settings: `/beny` now says the server isn't linked.
16. **(Discord) Removing the bot unlinks.** Link the server again. Press Control-C in the `mise run dev`
    tab, remove Beny Bot Dev from your server in Discord, run `mise run dev` again: Settings shows "Not
    linked".
17. **(Discord) A request pings you.** With Beny Bot Dev on (and invited back), a new person asks to run a
    campaign: Beny Bot sends you a direct message linking to Site admin.
18. **Checks.** In a second Terminal tab run `mise run check`. It ends with "All checks passed". It includes
    the cross-campaign secrecy sweep (players who DM elsewhere, outsiders, the owner, the DM with the other
    campaign's IDs in paths and bodies, a suspended campaign; the other campaign's rows must stay
    unchanged), the SQL scope check, the migration tests, and the idle-tab and reconnect tests.

### When you're done

- Press Control-C in the `mise run dev` tab. `mise run db:down` stops Postgres; your data is kept.
- To bring back the copy from step 4, with `mise run dev` stopped:
  `docker exec colony-planner-db psql -U colony -d colony -c 'DROP SCHEMA public CASCADE; CREATE SCHEMA public;'`,
  then `docker exec -i colony-planner-db psql -U colony -d colony < ~/colony-before-M1.5.sql`.

---

## Decisions confirmed by the author (2026-10-11)

These override anything earlier in this document that says otherwise.

| # | Decision | Answer |
|---|---|---|
| C1 | Hand-off or co-DM? | **No.** One DM per campaign, no hand-off. |
| C2 | Can a campaign's address (slug) change after approval? | **No.** The owner can adjust it at approval. |
| C3 | Can a DM delete their own campaign? | **Yes.** The DM can delete their own campaign at any time (no suspension first), after typing its slug to confirm; it cascades exactly like the owner's delete. The owner can still suspend any campaign and delete a suspended one. Add `DeleteOwnCampaign` (dm level) beside the owner's delete, with the same typed-slug confirmation, and a "Delete this campaign" danger zone on the campaign's Settings. Add acceptance steps and a secrecy-sweep case (a player and a non-member cannot delete; another campaign is untouched). |
| C4 | May a denied requester ask again? | **Yes**, one open request at a time (the default). |
| C5 | Can a player leave on their own? | **Yes: a player can remove themselves, and only themselves.** "Leave this campaign" (member's own membership only) is allowed from PENDING (withdraws the request) or ACTIVE, never from REMOVED, and never for the DM (who deletes the campaign instead). Leaving sets the membership to REMOVED, exactly like the DM removing them: the invite link cannot bring them back and pings nobody; only the DM can re-admit. Record that it was self-initiated (for example `campaign_member.left_on_own boolean` or `removed_by`) so /players shows "Left" instead of "Removed". No LEFT enum value. Add tests: leave from ACTIVE and from PENDING works; leave from REMOVED is refused; the DM cannot leave; after leaving, opening the invite link changes nothing and pings nobody; the DM can re-admit. |
| C6 | Offer the Eldari art to other campaigns in M4? | **No.** Eldari belongs to the author's campaign only; another DM would ask the author. M4 must not offer it as a shared starting map; other campaigns upload their own mainland art. |

## Revision notes

- Removed member bypass: cut LEFT and the leave flow; JoinByInvite returns an existing membership unchanged and pings nobody; tests added; C5 records the rule for any later Leave.
- Role mixing: harness adds `dmA` as a player in B and `dmB` as a player in A; pass 1 and new pass 1b; tokens `canaryA`, `canaryB`, `canaryBdm`; seed makes Hollis a player (Pell) in Sample Colony; checklist step 10.
- Body IDs: §2.3 rule 5 lists jsonb and body references; pass 4 substitutes B's IDs and keys into every body field; §3.3 no longer calls cross-campaign rows unstorable.
- Suspended join: `GET /api/invites/{code}` returns the status; JoinByInvite answers 409 for a suspended campaign; pass 5 and checklist step 12 cover it.
- DM_DISCORD_ID: `loadConfig` refuses it for every subcommand; the migration reads both variables with no default and raises on a lone or mismatched legacy value; migration test for the two-account database.
- Denied requests: a denied account may ask again; "Approve anyway" and `decision_note` removed; checklist step 6 fixed.
- Request content: `colony_name` and `timezone` dropped from `campaign_request`; colony and settings start at column defaults; the DM sets them on Settings; harness creates B through approval.
- Fingerprint: also hashes B's `campaign` row, B-only accounts and their sessions (bookkeeping columns excluded).
- Invite code: served only by the dm-level `GET .../invite`; listed as forbidden literals in the sweep.
- Stale guild links: Ready unlinks guilds missing from its list; no guild map to evict (one read per interaction); delete → "not linked" test.
- Startup check: campaign level if and only if `{campaignId}` and outside `/api/admin/`; images moved under `/api/campaigns/{campaignId}/images/`; unparsable ID → 404 before validation.
- Lint: per-occurrence `campaign_id` count rule, stated as a heuristic with the sweep as backstop; bad-query cases in its own test.
- Broker: per-campaign sequence and ring.
- Slug: cut to 40 before trimming `-`, then fall back to `colony`.
- Reset staleness: no membership cache; a missing colony or settings row maps to NotFound.
- Owner promise: new request-form copy; Read Message History dropped; logging invariant added.
- Fresh erases data: bold warning, a pg_dump backup command and a restore step.
- Checklist: added the API 404 (5), removed member's invite link (9), DM-elsewhere-as-player (10), paused invite (12), deleted campaign's link (13), no Manage Server (15), bot removed while off (16).
- CLAUDE.md length: dropped the 150-line target.
- Cuts adopted: the site-wide request cap, 3-campaigns-per-DM, 30 players, 10 pending joins per account, and failed-`/link` limits; LEFT; the account stream and its topics; `view_as_player_campaign_id` (boolean kept); `discord_link_code` table (in memory); the every-Ready guild-command sweep (one-time legacy clean-up instead); the membership cache (one read per campaign request; the reconnect test now allows one read); RLS stays deferred with §3.3 corrected.
- Effort revised from 9–12 to 8–11 builder-days.
