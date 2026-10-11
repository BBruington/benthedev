# Colony Planner: v1 Specification

**Status:** final spec for the author's review, revised with the author's follow-up decisions F1–F8 (below). **Date:** 2026-10-10.
**Build target:** `/Users/bibruington/myCodeSpace/colony-planner`, a new, empty, clean-room repo that runs **only on the author's Mac** (Postgres 17 in Docker) until the optional cutover, M6.
**Reference only:** `/Users/bibruington/myCodeSpace/party-planner`, read-only. Port from it only where named: the Beny Bot bot core, the rate limiter, the Discord message texts, the terrain colours, the pin styling and the Eldari art. No data is imported from it.
**Basis:** the author's twelve decisions, plus the follow-up decisions below, which override the twelve where they differ. Where a design choice had real alternatives, Appendix A gives the ruling and the reason. Anything the author hasn't decided is an open question (§11), never a rule.

**Follow-up decisions**
- **F1. Local only.** No hosting, domain or production database during M0–M5. Development runs against Postgres 17 in Docker. Deployment to the author's existing domain, replacing party-planner, happens only in the last milestone, **M6: Cutover (optional, only after your approval)**.
- **F2. No data import.** The old app holds only throwaway test data. `colony seed` fills the local database with fictional sample data, and `colony reset` wipes and re-migrates it (§9). Only the Eldari art carries over. A future cutover starts fresh.
- **F3. Auth confirmed:** direct Discord OAuth2 in Go with server-side sessions; no Clerk. Local development uses dev sign-in. Real Discord sign-in and Beny Bot are tested only with a separate test application, **Beny Bot Dev**, in a private test server. The production Beny Bot token is never used by this project.
- **F4. Joining:** anyone can sign in and ask, and the DM approves. There is no Discord-server membership check. Denied or removed members can't ask again; the DM can re-admit them.
- **F5. API toolchain:** huma/OpenAPI on a **trial** basis. The transport layer stays thin and isolated, so switching back to protobuf/Connect rewrites only that layer (§8.6).
- **F6. Extras:** keep Undo last change, View as player and Record cargo. **Cut hex map history entirely:** no edit log, attribution, stroke undo or revert. Keep the PWA manifest and data-defined resources and roles.
- **F7. Process:** the author checks in after every milestone, trying it locally with its checklist and then approving it. The author makes every git commit; agents never commit.
- **F8. Multi-campaign (M1.5).** The site hosts several campaigns, each sealed from the others: other DMs ask to run a campaign and the site owner (the author, `SITE_OWNER_DISCORD_ID`) approves it; players join each campaign by its invite link; one shared Beny Bot serves every linked server. The design, including the author's decisions C1–C6, is `docs/design/multi-campaign.md`; where it differs from this spec, it wins. This spec's single-campaign wording describes one campaign.

---

## 1. Product summary and design principles

### 1.1 What it is

Colony Planner is a private website for colony campaigns; the author's **Colony Campaign** is the first (F8). Each campaign has one DM and about 4–6 players, who already play over Discord with the Beny Bot bot. The colony sits on a new world that has no map yet, and ships supply it from **Eldari, the mainland**. The site has two halves:

- **The DM's console.** The day clock, stockpile, shipments, workforce, quests, patrons and favor, NPCs, places, sessions, and who is let in.
- **The players' board.** Players watch the colony change live, browse what their characters know, ask patrons for help, and chart the new world themselves on a shared hex map.

### 1.2 What a session feels like

- **Before.**
  - The Discord server's event list shows the next game.
  - Beny Bot posts a reminder about 24 hours ahead.
  - The game is also in each player's calendar through the ICS link.
- **Advancing time.** The DM taps **+3** on the board. A preview reads "Day 41 → 44 · Food 160 → 121 · Shipment arrives Day 42", and the DM confirms.
  - Every open phone shows a toast: "3 days pass · Day 44. Food −39 · Shipment arrived Day 42".
  - The food tile counts down and shows a "−39" chip.
  - The DM's own toast has an **Undo** button.
- **Requests.**
  - A player asks Lady Vex for a *Grain Barge* from their phone.
  - Beny Bot posts the request to the DM's private channel with **Grant** and **Decline** buttons.
  - The DM taps Grant without leaving Discord. Food goes up by 7, favor goes down by 3, and everyone's screen updates.
- **Charting the map.** Players switch the hex map to **Paint**, drag a stroke of forest along a coast they just explored, and label a cove. Other players see the stroke appear.
- **After.**
  - The timeline shows a band reading "Session 14 · Days 36–44".
  - The DM writes a recap, and players read it on the session page.

### 1.3 Design principles

1. **Many campaigns, each sealed (F8).** One DM and one colony per campaign; every campaign-owned row carries `campaign_id`; the site owner manages campaigns but can't read them.
2. **The server is the only gatekeeper, and it fails closed.**
   - Every route declares who may call it, and the server won't start if one doesn't.
   - A response carries only what the viewer may see. Hiding something in the UI doesn't count.
3. **The DM is the game engine.**
   - The app keeps the books and never invents consequences.
   - Nothing happens when food reaches 0 or morale is low.
   - Undecided rules are data or open questions, never code.
4. **Each colony change is one transaction.**
   - It writes its timeline entries and keeps a before-snapshot, so the latest change can be undone.
   - The timeline is both the story log and the audit trail. Auto-logged entries can be edited but never deleted.
5. **Settled rules live in one pure Go package.** That package is `internal/game`. The worked examples in §5.12 are its table tests, verbatim.
6. **Single sources of truth.**
   - One deployable.
   - One schema source: SQL migrations, with sqlc generating Go from them, closed sets included.
   - One API contract: Go structs, with OpenAPI and TypeScript generated from them (huma, on trial: F5).
   - Nothing is defined twice by hand.
7. **Nothing polls the database on a timer, and an idle open tab doesn't touch it.** So once the site is hosted (M6), Neon can scale to zero between sessions (§8.8).
8. **Phone-first for players.** They are on Discord during the game, often on a phone. Every player screen works one-handed at 360 px wide.
9. **Every dependency must earn its place** at the scale of a few campaigns of 1 DM and about 6 players.

**Non-goals for v1:**
- co-DMs or handing a campaign to another DM (C1);
- a public campaign directory;
- campaign export or copy;
- a public marketing site;
- AI features;
- Foundry sync;
- Google Calendar OAuth (Open Question 16);
- the player store;
- any new colony mechanic;
- deployment, a domain or a hosted database before the optional M6 cutover (F1);
- importing the old app's data (F2);
- hex map history: who charted what, stroke undo, revert (F6).

---

## 2. Users, roles, permissions and visibility

### 2.1 Identities

| Identity | How it is established | Can do |
|---|---|---|
| **Visitor** | Not signed in | See `/login` only |
| **Account** | Signed in with Discord (`account` row); may have no memberships | The picker `/`, invite links, ask to run a campaign (`/request`), sign out |
| **Pending** (per campaign) | `campaign_member.status = PENDING` in that campaign | See `/c/{slug}/pending` ("Waiting for the DM"), which opens live when approved; stop waiting (leave) |
| **Player** (per campaign) | `campaign_member.status = ACTIVE`, role PLAYER | The Player column in §2.3, in that campaign only |
| **DM** (per campaign) | `campaign_member.role = DM` (stored; exactly one per campaign, always ACTIVE) | Everything in that campaign |
| **Removed** (per campaign) | `campaign_member.status = REMOVED` (removed, denied, or left on their own) | That campaign's routes answer 403 and its tab shows "You don't have access" at once; their session and other campaigns keep working. Only the DM can re-admit them. |
| **Site owner** | `account.discord_user_id == SITE_OWNER_DISCORD_ID` (env var, derived per request, never stored) | Approve or deny campaign requests; list, suspend, unsuspend and delete campaigns on `/admin`. Nothing inside a campaign they aren't a member of (404). |

- **The DM role is stored per campaign** (F8). No API operation writes `role`: only campaign approval and the seed insert a DM membership, and a partial unique index keeps it to one.
- **The DM decides who gets in (decision 3), per campaign.**
  - Joining is by the campaign's invite link `/join/{inviteCode}`; asking makes a PENDING membership, and Beny Bot pings that campaign's DM channel. A first sign-in only creates the account.
  - Pings are limited to 5 an hour per campaign. Requests beyond that still appear on `/c/{slug}/players`.
  - At most 20 requests can be pending at once per campaign. Past that, joining answers "Not accepting new players right now".
  - Sign-in doesn't require membership in the Discord server (F4).
  - The DM approves or denies them on `/players`.
  - A denied or removed member can't ask again: the invite link changes nothing and sends no ping. Only the DM can re-admit them, from `/c/{slug}/players`.
  - A player may leave on their own (C5), from PENDING or ACTIVE; that is REMOVED, shown as "Left".
- **View as player.** The DM can turn this on from the profile menu.
  - While it is on, the server answers every request with the player projection and rejects DM-only writes.
  - A banner shows that it is on.

### 2.2 Server-side enforcement (one mechanism each)

1. **Auth middleware.** On every request it resolves the `__Host-colony_session` cookie to an account, using the in-memory session cache (§8.5), and builds a `Caller{AccountID, DiscordUserID, IsSiteOwner, ViewAsPlayer}`. For a campaign route the access layer then reads the `{campaignId}` path parameter before huma validates input and calls `core.EnterCampaign`, one membership read, which builds the `Viewer` (proof of membership; no owner flag). No membership, an unknown or an unparsable ID all answer **404 "No such campaign."**, the site owner included. The role is never cached in a cookie.
2. **Fail-closed access.** There are seven access levels (multi-campaign design §4.1):
   - `public`, and `token` (a secret in the URL, M5);
   - `account`: any signed-in account (the picker, invites, campaign requests, sign out);
   - `owner`: the site owner, only under `/api/admin/` and on the docs routes;
   - `pending`: any membership in this campaign, whatever its status;
   - `member`: ACTIVE members of an ACTIVE campaign;
   - `dm`: this campaign's DM, not viewing as player, campaign ACTIVE.

   A route is `pending`, `member` or `dm` if and only if its path has `{campaignId}` and is outside `/api/admin/`; the server refuses to start otherwise.

   How each kind of route gets its level:
   - **huma operations** are registered through one helper that requires a level, stored as the operation's `x-access` OpenAPI extension.
   - **Every other route** comes from one route table that also names a level:
     - `/img/*` needs `member`;
     - `/auth/logout` needs `pending`;
     - `/calendar/{token}.ics` is authorized by its token and serves only data players can already see;
     - `/auth/discord/*`, `/healthz` (which returns no data), the SPA shell and its assets, and `/auth/dev` in devauth builds (§8.5) are public.
   - **huma's built-in routes** (`/openapi.json`, `/schemas`, `/docs`) are turned off by leaving `OpenAPIPath`, `SchemasPath` and `DocsPath` empty. The OpenAPI document and the docs page are registered again as `dm` routes.

   Enforcement:
   - At startup the server walks the OpenAPI document and the route table, and **exits** if anything lacks a level.
   - One middleware enforces the level.
   - Only `GET .../me`, `GET .../stream` and `POST .../leave` under `/api/campaigns/{campaignId}` accept `pending`.
3. **Rows are filtered in SQL.**
   - Narrative tables have `revealed boolean`, and player-facing queries include `WHERE revealed OR @is_dm`. Archived patrons and actions are also excluded for players.
   - Timeline queries use `visibility = 'PUBLIC' AND undone_at IS NULL OR @is_dm`.
   - References to hidden rows (an NPC's place, a quest's giver) are joined with the same predicate, so they come back `null`.
4. **Fields are redacted centrally.**
   - Every DM-only field lives in a nested `dm` object on its DTO, for example `Npc.Dm *NpcDm \`json:"dm,omitempty"\``. The generated TypeScript type shows it as optional.
   - One huma response **Transformer** copies the response body into an addressable value. It sets every `dm` object to `nil` at any depth (structs, pointers, slices, maps) unless the viewer is the DM and not viewing as a player, then returns the copy.
   - The walker itself is a plain function in `internal/core`. The Transformer only calls it, so it survives a switch of API toolchain (§8.6).
   - M0 proves this mechanism with the canary test (item 6). If reflection over huma's body values turns out to be unreliable, the fallback is a response middleware that deletes every `dm` key from the encoded JSON. `dm` is a reserved key name.
   - Beny Bot builds every reply and post from the player projection, whoever runs the command (§7.2).
   - No handler calls redaction itself.
5. **Text never leaks.**
   - Timeline text is rendered when the event is written, using player-facing names.
   - A PUBLIC event never names a hidden NPC, quest giver or patron. If the subject is hidden, the event is written with DM visibility, or the hidden detail goes into the event's `dm.data`.
6. **Secrecy and access test (part of `mise run check`, §8.12).**
   - Each feature plants a canary string in every `dm` field and every hidden row it stores (a per-feature fixture in `internal/app/secrecy_fixtures_test.go`), and names a real row for each path parameter its routes use.
   - It then calls **every operation in the OpenAPI document and every route in the route table** as a player, the DM viewing as player, a pending member, a removed member and a visitor, with path parameters pointing at the fixtures' rows and required query parameters filled from their schemas:
     - `dm` routes must return 403;
     - an active member's member-level read must succeed, so its data path really runs;
     - every `/api/` and `/auth/` response carries `Cache-Control: no-store`;
     - every other response must contain no canary.
   - It also replays every SSE message and every Beny Bot command reply against the same canaries.
   - New endpoints are swept without editing the test, but their DM-only data is covered only once their feature plants canaries: plant one for every new `Dm` field or hidden row.
   - **Across campaigns (M1.5):** fixtures plant into two campaigns (`canaryA`, `canaryB`, `canaryBdm`, plus invite and link codes as literals). The sweep adds players of B, a DM elsewhere playing here, non-members and the owner (404 everywhere), A's DM with B's real IDs and keys in every path and body field, a suspended campaign and the owner's routes; B's rows are fingerprinted and must not change (multi-campaign design §4.4).
7. **CSRF.**
   - The mux is wrapped in Go's stdlib `http.CrossOriginProtection`.
   - The cookie is `SameSite=Lax` and `__Host-` prefixed.
   - Mutations accept JSON bodies only.

### 2.3 Permission matrix

| Data / action | Player | DM |
|---|---|---|
| Colony day, resources, colonist total, role headcounts, shipment countdown | read | read; changed only by the operations below |
| Advance N days, adjust stock, shipment schedule, record cargo, correct day, **undo last change** | — | yes |
| Workforce transfers and counts, named colonists | read role totals; named colonists listed only if revealed | yes |
| NPCs | revealed only; `displayName` only, with no true name or `dm_notes` (§5.9) | full CRUD and reveal |
| Quests | revealed only; giver shown only if revealed; no `dm_notes` | CRUD, reveal, complete, fail, reopen a failed quest |
| Patrons and favor | visible patrons, their favor, visible actions | create, edit, archive, adjust favor, define actions |
| Patron action requests | create for visible actions; withdraw own pending ones; see all party requests | approve (optionally tweaked), decline with a note, "use action now" |
| Timeline | PUBLIC entries that have not been undone | everything; add CUSTOM entries; edit text and visibility of any entry; delete CUSTOM entries only |
| Maps and places | revealed only; no `dm_notes` | CRUD, pins, image uploads |
| Hex map | paint, label, erase (no history: F6) | same |
| Sessions, recaps, ICS link | read | series CRUD, cancel or restore dates, recaps, mark played, regenerate the ICS token |
| Members | active members' display and character names | approve, deny, remove, re-admit, set character names |
| Settings | — | yes |
| Own profile | character name, theme, sign out, sign out everywhere | same, plus view as player |

The **site owner** (F8) has a third column: nothing inside a campaign they aren't a member of (404); ask to join through an invite link like anyone; their own campaign request is approved at once; see open campaign requests and approve or deny them; see every campaign's directory data (name, slug, DM, player and pending counts, Discord linked, status, created date); suspend, unsuspend, and delete a suspended campaign. A DM may also delete their own campaign (C3), and invite, reset the link, and link or unlink a Discord server in their own campaign. A player may leave a campaign (C5). Details: multi-campaign design §4.3.

---

## 3. Scope

### 3.1 v1 (this spec)

- **Access**
  - Discord sign-in (tested with the Beny Bot Dev application, F3), plus dev sign-in for local work.
  - DM approval, with a Beny Bot ping and a pending page that opens live.
  - View as player.
- **Colony clock and books**
  - Data-defined resources.
  - **Advance N days** with quick chips, a preview, a large-N confirmation and no cap.
  - Shipment cadence with the bugs fixed, plus "Record cargo".
  - DM adjustments and day correction.
  - **Single-level undo.**
- **Population**
  - Eight roles plus **Unassigned**.
  - Colonists is the derived sum, and named NPC colonists count toward their role.
- **Quests and patrons**
  - Quests with Effects rewards, plus favor for the patron who gave them.
  - Guarded Complete, Fail, and reopening a failed quest.
  - Patron actions with the **request → approve/decline** flow, including Discord Grant and Decline buttons.
- **Timeline**
  - Auto-logged entries, DM entries and session bands.
  - Live "what changed" toasts.
- **World**
  - NPCs and places with server-side secrecy.
  - The Eldari mainland map with pins, and image uploads.
  - The hex map:
    - free player painting;
    - Look and Paint modes;
    - batched strokes;
    - patterns as well as colours;
    - live updates.
- **Table logistics**
  - Sessions with a correct timezone model.
  - Rolling Discord events, reminders and automatic logging of played sessions.
  - Recaps and an ICS feed.
- **Beny Bot:** `/beny`, `/colony`, `/nextsession` and `/npc view`, plus notices in the DM channel.
- **Shell:** a PWA manifest so players can install the site on a phone, and the "Night Sea and Parchment" visual direction.
- **Local development:** Postgres 17 in Docker, dev sign-in, and `colony seed` / `colony reset` sample data (§8.10, §9). Nothing is deployed before the optional M6 (F1).

### 3.2 Next (the architecture is ready; the rules are not invented)

- **Buildings, sickness and events** (decision 10). Each one starts from the author's own rules write-up. If those rules need them, these seams are ready (§8.6):
  - the `Effects` type;
  - the ordered daily-step pipeline;
  - open-ended timeline kinds;
  - the NPC `health` enum, which already has `SICK`;
  - DM-defined resources and roles.
- **Beny Bot extras:**
  - posting public timeline entries to a party channel;
  - `/chronicle`;
  - `/request` from Discord.
- **Shipments:** a standing shipment manifest, if the author chooses one (Open Question 7).

### 3.3 Later

**The player store** (magic items, food, colonists, special NPCs) is a separate design effort (decision 6). Its open questions are Open Question 18: who pays, whether purchases arrive at once or by ship, and who owns bought items. Nothing in v1 blocks it:

- `request.kind` is text validated in Go, so adding `STORE_PURCHASE` needs no migration;
- `request.cost` and `request.effects` hold any price and any goods as snapshots, and approval re-checks them under the colony lock;
- bought colonists land in Unassigned (decision 8);
- an `npc` row can carry a role, so a bought "special NPC" can become a named colonist;
- the catalog and owned items get their own tables, and `request` gains one nullable column that points at the catalog (one additive migration).

### 3.4 Disposition of every old feature

| Area | Old feature | Disposition | Reason |
|---|---|---|---|
| Colony | Initialize Colony | **Drop** | Approving a campaign creates its colony, resources and roles. |
| Colony | Resource tiles + shipment countdown | **Redesign** | Tiles come from resource rows. The countdown can never go negative. Live delta chips. |
| Colony | Edit resources / shipment interval | **Redesign** | Becomes Adjust (deltas, or "set to" values converted under the lock) plus a separate shipment schedule that can be cleared. Fixes the NaN save block and the overwrite of absolute values. |
| Colony | Advance Day | **Redesign** | Becomes Advance N days with a preview, chips and a version guard. Fixes the off-by-one and the `== 0` stall. Undoable. |
| Colony | Colonist Roles headcounts | **Redesign** | Roles define the colonist total. Unassigned added. The DM moves people between roles. |
| Colony | Named colonists + job | **Port** | `npc.colony_role_key`. A named colonist counts toward its role. |
| Colony | Patrons & favor (schema only) | **Build** | Patrons, favor, actions, requests. |
| Colony | Colony "Map" tab placeholder | **Drop** | The hex map is its own screen, `/explore`. |
| Colony | Region ↔ colony link | **Drop** | The new world has no DM map. |
| Colony | Remove colony | **Drop** | The colony is a permanent singleton. |
| Colony | Colony timeline (uncommitted) | **Redesign** | Becomes the auto-logged ledger: typed kinds, text rendered at write time, links, visibility, session bands, Day 0 rendered, opens on today. |
| Map | Hex exploration map | **Redesign** | Leaflet with a canvas layer; Look and Paint modes; grows as it is painted; label kept on repaint; live; usable on a phone. No edit history (F6). |
| World | Regions with Leaflet image maps + pins | **Redesign** | Becomes Maps, with Eldari seeded as the Mainland. Pins are normalized to 0..1, so they survive an image swap. |
| World | Region map image (URL only) | **Redesign** | The DM uploads into the Postgres image store. An external URL is still allowed. |
| World | Locations (player vs DM notes) | **Port + fix** | Renamed Places. Adds a Visible switch, on by default as today (Open Question 9). `dm_notes` is stripped on the server. The region becomes an optional map. |
| NPCs | Roster/detail/edit, secrecy | **Port + fix** | Secrecy is enforced on the server. Places render as names, not UUIDs. Aliases stay visible to players, as today (Open Question 8). |
| NPCs | "First Encountered" session (`session_encountered_id`) | **Port + fix** | Becomes `npc.first_session_id`, shown as the session's title and date instead of the raw UUID the old page showed (Open Question 10). |
| NPCs | Foundry actor fields | **Drop** | Never used. |
| Quests | List, edit, reward + loot, Complete | **Port + fix** | Status guard; one transaction; typed Effects reward plus favor; status can't be set from the edit form; edits replace the full record (no more wiped descriptions). Visible switch, on by default as today (Open Question 9). |
| Quests | Quest giver (NPC or patron) | **Port + fix** | Can be set at any time. A patron giver receives the reward favor. Patrons are archived, never deleted. |
| Characters | `player_character` | **Drop** | Replaced by `member.character_name`. Sheets stay in D&D Beyond or Foundry. |
| Sessions | Series scheduling | **Redesign** | Local wall time plus one IANA timezone, expanded by one Go function. Multi-weekday series allowed. |
| Sessions | Cancel / restore an occurrence | **Port** | Becomes a `session` row with status CANCELLED, keyed by local date. |
| Sessions | Session log + recap | **Port + fix** | The recap is shown. The session is stamped with the colony days it covered. |
| Discord | Announce to Discord (recurring event) | **Redesign** | A rolling single event for each series' next occurrence. Avoids the API's recurrence limits. Edits and cancellations are exact. |
| Discord | Auto-log played session when the event goes live | **Port** | Also stamps colony-day start and end and writes `SESSION_PLAYED`. |
| Discord | Daily reminders | **Port + fix** | In-memory timers. Respect weekdays, one-offs and the timezone. Claim-then-send. |
| Discord | Date polls (backend only) | **Drop** | Unused. Native Discord polls can be posted by hand. |
| Discord | `/beny` | **Port** | "Boy!" |
| Discord | `/npc view` | **Port + fix** | Revealed NPCs only, matched by display name; ephemeral; built from the player projection. |
| Discord | `/npc set` | **Drop** | Editing belongs on the site. |
| Discord | `/registercampaign` | **Redesign** (F8) | `/link` with a one-time code from the campaign's Settings. |
| Discord | `/nextsession` | **Port + fix** | Rebuilt on `occurrences()`, using `<t:…>` timestamps. |
| Discord | `/schedule` | **Drop** | Dead since #96. |
| Integrations | Discord OAuth install + channel settings | **Redesign** | The bot is invited once by URL. Channel settings live on `/settings`, and every one has a consumer. |
| Integrations | Google Calendar connect / conflicts / add series | **Drop** (Open Question 16) | An ICS subscription feed puts sessions in any calendar, with no OAuth and no stored tokens. **Lost:** the conflict warning (checking members' calendars for clashes while the DM schedules) and per-user calendar sync. |
| Integrations | Foundry VTT card | **Drop** | Never built. |
| Shell | Create campaign, multi-campaign, active-campaign cookie | **Redesign** (F8) | Owner-approved campaigns, path-scoped URLs, no cookie. |
| Shell | Email invites → `/accept` → `/join` | **Redesign** (F8) | An invite link per campaign, `/join/{inviteCode}`, then DM approval. No Resend. |
| Shell | Campaign settings, members, kick, archive | **Redesign** | `/settings` (singleton) and `/players` (approve, remove, character names). |
| Shell | "Join with code" | **Redesign** (F8) | The picker accepts a pasted invite link or code. |
| Shell | Campaign switcher / role change | **Redesign** (F8) | A switcher in the top bar; no role change (one DM per campaign, C1). |
| Shell | Profile menu, `/settings` (Google) | **Redesign** | Profile menu: character name, theme, view as player (DM), sign out, sign out everywhere. |
| Public | Landing page | **Redesign** | A sign-in page over the Eldari art, with a one-line privacy note. |
| Public | Beny Bot Terms / Privacy pages | **Drop**, reconsider at M6 | Locally the bot is private. M6 decides on a short privacy page now that friends' data could be hosted. |
| (infra) | Clerk + webhook, RSA `planner_auth` cookie, oRPC BFF, Drizzle, Nx, `packages/*` | **Drop** | Replaced by the single binary (§8). |
| (data) | The old app's campaign data in Neon | **Drop** | Throwaway test data; no importer (F2). Only the Eldari art carries over. |

---

## 4. Domain model

All tables live in one Postgres database:

- **Keys** are `uuid` unless noted.
- **Timestamps** are `timestamptz`, in UTC.
- **Campaign-owned tables** (F8) carry `campaign_id uuid NOT NULL REFERENCES campaign ON DELETE CASCADE`, first in the primary key or in `UNIQUE (campaign_id, id)` and first in every index; text keys are unique per campaign; a column that references another campaign row is a composite foreign key `(campaign_id, x_id)`; every query takes `@campaign_id` (`db/scope_test.go`); IDs and keys inside jsonb or request bodies are loaded by core with the Viewer's campaign. The tables below are written as for one campaign; each also has `campaign_id` (multi-campaign design §2.3, §2.6).
- **One-row-per-campaign tables** (`settings`, `colony`) use `campaign_id` as their primary key.
- **Closed sets** are Postgres `ENUM` types. sqlc turns each one into a Go string type with an `AllXValues()` function, which feeds the API schema and the TypeScript types (§8.4). So each set is written once, in a migration.
- **Open-ended kinds** (timeline kind, request kind) are `text`, validated against one Go list. Adding one needs no migration.
- **Colony-day columns** are `integer`.

### 4.0 Directory tables (F8; global, read without a campaign by design)

| Table | Key fields | Notes |
|---|---|---|
| `account` | `id`, `discord_user_id text UNIQUE`, `discord_username`, `display_name`, `avatar_url`, `created_at`, `last_seen_at` (written at most once a day) | One per Discord identity. |
| `campaign` | `id`, `slug` (3–40 of `a-z 0-9 -`, UNIQUE, fixed after approval, C2), `name`, `status campaign_status` (ACTIVE, SUSPENDED), `suspended_at`, `invite_code` UNIQUE, `created_at` | The invite code is served only to the DM. |
| `campaign_request` | `id`, `account_id`, `campaign_name`, `slug`, `note`, `status campaign_request_status` (PENDING, APPROVED, DENIED, WITHDRAWN), `decided_at`, `campaign_id NULL`, `created_at` | One open request per account and per slug. |
| `discord_guild_link` | `campaign_id PK`, `guild_id UNIQUE`, `guild_name`, `linked_by_discord_user_id`, `linked_at` | One server per campaign and one campaign per server. Link codes live in memory. |

### 4.1 Access

| Table | Key fields | Notes |
|---|---|---|
| `campaign_member` | `id`, `campaign_id`, `account_id`, `role campaign_role` (DM, PLAYER), `status member_status` (PENDING, ACTIVE, REMOVED), `character_name NULL`, `left_on_own bool` (only when REMOVED, C5), `created_at`, `approved_at`; `UNIQUE (campaign_id, account_id)` | One DM per campaign (partial unique index), always ACTIVE. No API writes `role`. |
| `web_session` | `token_hash bytea PK` (SHA-256 of a 32-byte random token), `account_id → account ON DELETE CASCADE`, `view_as_player bool`, `created_at`, `expires_at`, `last_seen_at` (written at most once a day) | Per account, so one sign-in serves every campaign. Removal from a campaign no longer touches sessions. |
| `settings` (one per campaign) | `campaign_id PK` (the name is `campaign.name`), `timezone text` (IANA), `discord_announce_channel_id`, `discord_reminder_channel_id`, `discord_dm_channel_id`, `discord_event_location text DEFAULT 'Forge + Discord Voice'`, `ics_token` | Every column has a consumer. |

### 4.2 Colony

| Table | Key fields | Notes |
|---|---|---|
| `colony` (one per campaign) | `campaign_id PK`, `name`, `day int ≥ 0`, `version bigint`, `shipment_interval_days int NULL CHECK (≥ 1)`, `next_shipment_day int NULL`, `last_change jsonb NULL`, `updated_at` | `CHECK ((shipment_interval_days IS NULL) = (next_shipment_day IS NULL))` and `CHECK (next_shipment_day > day)`, so the stall is impossible. `version` increments on every colony mutation. `last_change` holds the undo snapshot (§5.1). Every member reads `name` and the campaign name on the board; the DM edits both on `/settings`, which is not a colony mutation. |
| `resource` | `PRIMARY KEY (campaign_id, key)`, `label`, `icon colony_icon` (a curated lucide list, as a Postgres enum), `amount int`, `min_amount int DEFAULT 0`, `max_amount int NULL`, `sort_order`, `is_system bool`, `archived bool` | `CHECK (amount >= min_amount AND (max_amount IS NULL OR amount <= max_amount))`. Seeds: `gold`, `food` (system; used by upkeep), `materials`, and `morale` (max 100, starts at 100). The DM adds resources with no code change. |
| `role` | `PRIMARY KEY (campaign_id, key)`, `label`, `icon`, `sort_order`, `unnamed_count int CHECK (≥ 0)`, `is_system bool`, `archived bool` | Seeds use the old icons: FARMER (Wheat), HEALER (HeartPulse), BLACKSMITH (Hammer), SOLDIER (Sword), MINER (Pickaxe), BUILDER (HardHat), SCHOLAR (BookOpen), MAGE (Wand2, stored as `WandSparkles`, lucide's current name for it), plus the system role **UNASSIGNED** (Users). A role can be archived only when its headcount is 0. |

The **colonist total is never stored**. `game.Colonists` and this query compute it:

```sql
SELECT (SELECT COALESCE(SUM(unnamed_count), 0) FROM role WHERE campaign_id = @campaign_id)
     + (SELECT COUNT(*) FROM npc WHERE campaign_id = @campaign_id AND colony_role_key IS NOT NULL)
```

### 4.3 Narrative

| Table | Key fields | Notes |
|---|---|---|
| `npc` | `id`, `name` (true name), `known_name NULL`, `revealed bool DEFAULT false`, `aliases text[]`, `labels text[]`, `race`, `character_class`, `level`, `age`, `appearance`, `personality`, `backstory`, `player_notes`, `dm_notes`, `status`, `health`, `relation` (old enums kept), `avatar_url`, `current_place_id → place SET NULL`, `origin_place_id → place SET NULL`, `first_session_id → session SET NULL`, `colony_role_key → role NULL`, timestamps | `colony_role_key IS NOT NULL` means the NPC is a colonist in that role. Such an NPC can't be deleted until they leave the colony (§5.5). |
| `quest` | `id`, `title`, `description`, `kind quest_kind` (MAINLAND, COLONY), `status quest_status` (ACTIVE, COMPLETED, FAILED), `revealed bool DEFAULT true`, `giver_npc_id → npc SET NULL`, `patron_id → patron RESTRICT`, `reward_effects jsonb`, `reward_favor int DEFAULT 0`, `loot jsonb` (`[{name, quantity, description}]`, display only), `dm_notes`, `resolved_day NULL`, `resolved_at NULL`, timestamps | `CHECK (reward_favor = 0 OR patron_id IS NOT NULL)`. Patrons are never deleted, so a delete can't break this check. Status changes only through actions (§5.6). |
| `patron` | `id`, `name`, `description`, `revealed bool DEFAULT false`, `archived bool DEFAULT false`, `favor int DEFAULT 0`, `player_notes`, `dm_notes`, `sort_order`, timestamps | Archived, never deleted. There is no delete operation, and foreign keys to it are RESTRICT, so a stray SQL delete fails loudly. |
| `patron_action` | `id`, `patron_id → patron RESTRICT`, `name`, `description`, `favor_cost int CHECK (≥ 0)`, `effects jsonb`, `revealed bool DEFAULT false`, `archived bool`, `sort_order`, timestamps | Archived, never deleted. Example: "Grain Barge", cost 3, effects `{"resources":{"food":7}}`. |
| `request` | `id`, `kind text` (validated in Go; v1 has only `PATRON_ACTION`), `title text` (snapshot of what was asked for, e.g. "Grain Barge"), `patron_id → patron NULL SET NULL`, `patron_name text NULL` (snapshot), `patron_action_id → patron_action NULL SET NULL`, `cost jsonb` (snapshot, e.g. `{"favor":3}`), `effects jsonb` (snapshot), `applied_effects jsonb NULL`, `requested_by → member`, `on_behalf_of → member NULL`, `requested_day`, `note`, `status request_status` (PENDING, APPROVED, DECLINED, WITHDRAWN), `decided_by NULL`, `decided_at NULL`, `decision_note NULL`, `event_group_id NULL`, `discord_message_id NULL`, `created_at` | `CHECK (kind <> 'PATRON_ACTION' OR patron_name IS NOT NULL)`. Snapshots never change, so history reads correctly after the action is edited or archived. `applied_effects` records a DM tweak. A future kind fills `title`, `cost` and `effects` the same way and adds its own nullable reference column (§3.3). |
| `timeline_event` | `id`, `group_id uuid NULL` (shared by every entry one mutation writes), `sequence` (write order), `day int`, `kind text`, `title`, `body`, `visibility timeline_visibility` (PUBLIC, DM), `deltas jsonb` (applied Effects only), `data jsonb` (kind-specific), `quest_id`, `patron_id`, `request_id`, `npc_id`, `session_id` (all `SET NULL`), `undone_at NULL`, `created_by → member NULL`, `created_at`, `edited_at NULL`; index `(day, created_at)` | Named `timeline_event` so it isn't confused with the future Events mechanic. Text is rendered at write time. M1 creates it without the five link columns: each arrives with its table (`quest_id` in M3, `npc_id` in M2, and so on). `sequence` orders entries within one transaction, which share `created_at`. `CARGO_RECORDED` links to its arrival in `data` (`{arrivalEntryId}`); `SHIPMENT_ARRIVED` keeps `{first, every, count}` there. |

**`Effects` JSON** is one Go type and one generated TypeScript type, used everywhere:

```json
{ "resources": { "food": 7, "gold": -10 }, "colonists": 2 }
```

- `resources` keys must name existing, unarchived resources.
- `colonists ≥ 0`, and colonists always land in UNASSIGNED.
- Effects has no favor field in v1. Favor changes only through:
  - a quest's `reward_favor`, paid to its patron giver;
  - an action's favor cost;
  - DM adjustments.

  Whether an action, cargo or a store item may also grant favor is Open Question 12. If the answer is yes, `favor` becomes one more optional Effects field, with no migration.

### 4.4 World

| Table | Key fields | Notes |
|---|---|---|
| `map` | `id`, `name`, `image_url` (`/eldari.webp`, `/img/{id}` or an external URL), `width`, `height` (natural pixels), `is_mainland bool`, `revealed bool DEFAULT true`, `sort_order`, timestamps | Seeded with "Mainland: Eldari" (1920×1080, revealed). |
| `place` | `id`, `name`, `description`, `player_notes`, `dm_notes`, `revealed bool DEFAULT true`, `map_id → map SET NULL`, `pin_x real NULL`, `pin_y real NULL` (0..1, y measured **from the top**), timestamps | `CHECK ((pin_x IS NULL) = (pin_y IS NULL))`. Replaces regions and locations. |
| `hex` | `q int`, `r int`, `PRIMARY KEY (q, r)`, `terrain terrain` (enum), `label text NULL` (≤ 40 chars), `updated_at` | No game boundary (decision 9). The only check is a technical bound against bad input, `abs(q) <= 100000 AND abs(r) <= 100000`. Pointy-top axial. No row means uncharted. No attribution and no history (F6). **The migrations seed no hexes.** |
| `image` | `id`, `content_type`, `bytes bytea` (≤ 5 MB), `sha256 UNIQUE`, `width`, `height`, `uploaded_by → member`, `created_at` | Served at `/img/{id}` to members, with `Cache-Control: immutable`. |

### 4.5 Table logistics

| Table | Key fields | Notes |
|---|---|---|
| `session_series` | `id`, `title`, `description`, `weekdays smallint[]` (0 = Sunday … 6 = Saturday), `every_weeks smallint NULL CHECK IN (1, 2)` (NULL means a one-off on `first_date`), `first_date date`, `last_date date NULL`, `start_local time`, `duration_minutes CHECK 15..480`, `announce bool DEFAULT true`, `remind bool DEFAULT true`, timestamps | Local wall time, interpreted in `settings.timezone`. |
| `session` | `id`, `series_id → session_series SET NULL`, `local_date date`, `starts_at timestamptz`, `duration_minutes`, `title`, `status session_status` (SCHEDULED, CANCELLED, PLAYED), `discord_event_id NULL`, `discord_claimed_at NULL` (a lease, §7.3), `reminded_at NULL`, `recap text`, `colony_day_start NULL`, `colony_day_end NULL`, timestamps; `UNIQUE (series_id, local_date)` | Created on demand (cancel, Discord event, reminder, played, recap) with `INSERT … ON CONFLICT`. It replaces the old exceptions table and the history-only session rows. |

### 4.6 Relationships

```
campaign (1) ─ (n) everything below (campaign_id); account (1) ─ (n) campaign_member ─ (1) campaign
colony (1) ─ day clock, version, shipments, last_change (undo slot)
resource (n) ─ stockpile              role (n) ─ unnamed headcounts (UNASSIGNED included)
npc.colony_role_key → role            (named colonists count toward their role)
npc.first_session_id → session        (where the party first met them)
quest ─ giver_npc_id → npc, patron_id → patron     (complete → Effects + reward_favor → patron)
patron (1) ─ (n) patron_action (1) ─ (n) request ─ member (requested_by, on_behalf_of)
timeline_event ─ group_id (one mutation) ─ optional links to quest / patron / request / npc / session
map (1) ─ (n) place ← npc.current_place_id / origin_place_id
hex (q,r)                               (player-drawn; linked to nothing, not even its painter)
session_series (1) ─ (n) session (colony_day_start / colony_day_end)
account (1) ─ (n) web_session      campaign_member (1) ─ (n) request, timeline_event.created_by, image.uploaded_by
```

---

## 5. Rules and behaviors

All rules live in `internal/game` as pure functions with table tests. That package has no database access, no network calls, no `time.Now` and no randomness, and `mise run check` enforces this (§8.12). Handlers load state, call the rule, save, write events and publish.

### 5.1 The write path and single-level undo

Every **colony mutation** follows the same path. The colony mutations are:

- advance;
- adjust;
- record cargo;
- correct day;
- shipment schedule;
- workforce changes, and named colonists joining and leaving;
- quest complete, fail and reopen;
- request approve and "use action now";
- favor adjust;
- undo.

```
tx:
  colony := SELECT … FROM colony WHERE campaign_id = @campaign_id FOR UPDATE   -- per campaign; always locked first
  if op carries expectedVersion and colony.version != expectedVersion
      → 409 "The colony changed (now Day {day}). Review and retry."
  before := snapshot of every row this op will touch           -- amounts, counts, favor, day/shipments, quest or request status
  result := game.X(state, input)                               -- pure
  save rows; insert timeline_event rows sharing one group_id (applied deltas only)
  UPDATE colony SET version = version + 1,
         last_change = {group_id, label, version_after, before}
commit → stream.Publish(topics…) → best-effort Discord post
```

- **Guards.**
  - Mutations started from a form carry `expectedVersion`: advance, adjust, cargo, correct day, schedule, workforce and favor. This stops double clicks and two devices from overwriting each other.
  - Quest and request decisions are guarded by their status instead (`WHERE status = 'ACTIVE'` or `'PENDING'`), so a Discord button needs no version.
- **Undo (DM).**
  - Allowed only while `colony.version == last_change.version_after`, meaning nothing has happened since.
  - In one transaction it:
    - restores the `before` rows;
    - sets `undone_at` on that group's timeline entries (they are kept, never deleted);
    - writes an `UNDO` entry ("The DM rewound: 3 days pass · Day 44");
    - increments `version`;
    - clears `last_change`.
  - There is only one level: after an undo, there is nothing further to undo.
  - Undo does not recall Discord posts. If a granted request is undone, Beny Bot edits that request's message to say "Undone".
- **Not colony mutations:** hex painting; creating and withdrawing requests; edits to NPCs, places, quests and sessions; reveals. They don't touch `version`. Deleting an NPC who is a colonist is refused until they leave (§5.5).

### 5.2 Advance N days (DM)

**Input:**
- `days`: any integer ≥ 1. The site enforces **no limit** (decision 4). The only check is an overflow guard: the new `day` and, when a schedule exists, the new `next_shipment_day` must both fit in a Postgres `integer` (2,147,483,647). Otherwise the server returns 422 "That many days can't be stored". Go does the arithmetic in `int64`.
- `expectedVersion`.

**Daily-step pipeline.** Steps run in a fixed order each day, and that order is a rule:

```go
var DailySteps = []Step{Upkeep{}, Shipments{}} // future steps are appended only if their rules need one

type Step interface {
    // Apply this step to every day in (from, to], given that no other step changes
    // anything this step reads inside that span.
    Span(s *State, from, to int, log *Log)
    // The next day after `after` on which this step changes something another step reads.
    NextBoundary(s State, after int) (day int, ok bool)
}
```

- `Advance` walks from boundary to boundary. On each span it runs every step in order, so on any given day upkeep still runs before shipments.
- **In v1 there are no boundaries.** Shipments deliver nothing automatically and colonists are constant during an advance, so any N is a single span:
  - **Upkeep:** `food = max(food − colonists × N, food.min_amount)`. This is the old rule of 1 food per colonist per day (`game.FoodPerColonistPerDay`), in closed form.
  - **Shipments:** the arrivals in `(old, new]` are the arithmetic run `D, D+I, …`, counted rather than looped. Afterwards `D` becomes the first value greater than the new day.
- A future step that changes state every day reports a boundary every day, and the walk becomes a day-by-day loop.
- **Writes are bounded for any N:**
  - one `DAYS_ADVANCED` entry at the new day, e.g. "3 days pass · Day 44" with body "Food −39 · Shipment arrived Day 42";
  - one `SHIPMENT_ARRIVED` entry per arrival day when there are **12 or fewer arrivals**;
  - otherwise **one folded** `SHIPMENT_ARRIVED` entry on the last arrival day: "142,857 shipments arrived, every 7 days, Days 42 – 1,000,034". Its `data` holds `{first, every, count}`.
- **Preview.** `GET /api/colony/advance-preview?days=N` runs the same function without writing and returns the `version` it saw. The UI always shows the preview before Confirm, for example "Day 12 → 17 · Food 140 → 80 · 1 shipment arrival (Day 14)". It always shows the arrival count.
- **UI.**
  - Quick chips: **+1, +3, +7, Custom N**.
  - If N > 365, the confirm step asks the DM to type the number again. This is a typo guard, not a cap.
- **Guarantees.** All of these are tested:
  - advancing one day N times gives the same state as advancing N days once;
  - `Advance(a+b) == Advance(Advance(a), b)`;
  - the span engine matches a naive day-by-day reference implementation.
- **Nothing else happens** (decisions 4 and 5):
  - workers produce nothing;
  - there are no consequences at 0 food or low morale;
  - nothing records a shortfall.

### 5.3 Shipment cadence

- **State.** Interval `I` (≥ 1) and next arrival day `D` (> `day`) are either both NULL or both set.
  - The countdown is `D − day`, which is always ≥ 1 between advances.
  - The board shows "Next shipment: Day D (in X days)".
- **Set schedule (DM).**
  - The DM sets `I`, plus the next arrival day.
  - The next arrival day defaults to the current `D`, or to `day + I` when there is none. It must be greater than `day` and must fit in `integer`.
  - Setting interval 7 on day 0 gives arrivals on days 7, 14, 21, exactly every 7 days. **This fixes the off-by-one.**
  - Writes `SHIPMENT_SCHEDULE_CHANGED`, PUBLIC.
- **Clear (DM).** Sets both to NULL and hides the countdown. **This fixes "can't clear".**
- **Validation.**
  - Interval 0 is rejected (422).
  - An empty optional number input sends `null`, never NaN. **This fixes the NaN form**, which used to block every stock edit.
- **No stall.** Arrivals are matched inside the advance, and the database CHECK keeps `D` in the future. **This fixes the `== 0` stall.**
- **Correct day (DM).**
  - Sets `day` directly, with no upkeep and no arrivals. The overflow guard in §5.2 applies.
  - If `D ≤` the new day, `D` rolls forward to the first `D + k·I` after it. Moving the day backward leaves `D` alone.
  - Writes `DAY_CORRECTED` with DM visibility, e.g. "Day 10 → 30 (3 shipment dates skipped)".
- **Cargo.** Shipments carry nothing automatically, which is today's behavior.
  - Every `SHIPMENT_ARRIVED` entry, folded ones included, offers the DM **Record cargo**.
  - It opens Adjust (§5.4) prefilled with `source = shipment`, with public logging switched on.
  - It writes `CARGO_RECORDED`, linked to the arrival.
  - Recorded colonists land in Unassigned (decision 8).
  - So that cargo counts toward later upkeep, the DM advances to the arrival day, records the cargo, then continues. That is why the preview names arrival days.
  - A standing manifest is Open Question 7.

### 5.4 Effects, adjustments and favor

- **CLAMP** applies to quest rewards, DM adjustments and cargo.
  - Each resource becomes `clamp(old + Δ, min, max)`.
  - **Only the applied delta is logged.** There is no requested-versus-applied record.
  - The preview shows a cap before the DM confirms, e.g. "Morale 98 → 100 (capped)".
- **STRICT** applies to what a player's request spends:
  - the favor cost (spending can never take favor below 0);
  - by default, any negative resource deltas in the action's effects. Whether those should clamp instead is Open Question 12.
  - If anything would fall short, the whole operation is rejected with the shortfall named, e.g. "Not enough favor: Lady Vex has 2, needs 3" or "Not enough food: the colony has 4, needs 10".
  - Positive deltas still cap at the maximum.
  - The request flow, and so a future store, uses this mode.
- `colonists: n` adds n to `UNASSIGNED.unnamed_count`. Negative colonist deltas are not supported; removals go through the workforce screen (§5.5).
- **Adjust stock (DM).**
  - Input is deltas, or "set to" values that the server converts to deltas under the lock, plus a reason.
  - It always writes `STOCK_ADJUSTED`, which keeps the audit trail. Visibility is **DM by default**, with a "Log on the public timeline" switch.
- **Adjust favor (DM).**
  - Takes a delta or a set-to value. DM adjustments are unbounded, as today (Open Question 12).
  - Writes `FAVOR_ADJUSTED`, DM by default with the same public switch. It can never be PUBLIC while the patron is hidden.

### 5.5 Colonists, roles and Unassigned (decision 8, settled)

- **Colonists = Σ over all roles of (unnamed count + named NPC colonists in that role)**, Unassigned included. Food upkeep uses this number.
- **New colonists always land in UNASSIGNED**, whether they come from quest rewards, recorded cargo, approved actions or, later, the store.
- **Workforce operations (DM)** write `WORKFORCE_CHANGED` with **DM visibility by default** (opt-in PUBLIC):
  - **Transfer:** move n unnamed colonists from role A to role B. Requires `A.unnamed_count ≥ n`; the total is unchanged.
  - **Set count:** set a role's unnamed count directly. This is a correction, and it is how deaths and departures are recorded.
- **Named NPC colonists.** The joined and left entries are PUBLIC if the NPC is revealed, otherwise DM.
  - **Join colony as role R.** Two choices are offered:
    - *new colonist* (the default): total +1, writes `COLONIST_JOINED`;
    - *promote an existing unnamed R*: decrements `R.unnamed_count`, so the total is unchanged.
  - **Change role:** the total is unchanged.
  - **Leave colony.** Two choices are offered:
    - *remove* (the default): total −1, writes `COLONIST_LEFT`;
    - *return to the unnamed pool*: increments the role's unnamed count.
  - Marking an NPC DEAD changes no counts. The UI suggests removing them.
  - **Deleting an NPC who is a colonist is refused** with 409 "Brannoc is a colonist. Remove them from the colony first." Leave is a colony mutation with its own entry and undo, so the colonist total never changes silently.
- Players see every role's total. Named colonists are listed only if they are revealed; hidden ones still count.

### 5.6 Quests

- **Create (DM).** Starts as `status = ACTIVE`.
  - The create form has a **Visible to players** switch. It starts on, as today (Open Question 9).
  - The reward is `reward_effects` + `reward_favor` + `loot`.
  - Favor requires a patron giver, and the form says so.
- **Posting.** When a quest becomes visible, whether it was created visible or revealed later, a PUBLIC `QUEST_POSTED` entry is written ("New quest: Clear the Wolf Den"). A checkbox can suppress it.
- **Edit (DM).** A PUT replaces the whole editable record. **Status can't be changed through edit.**
- **Complete (DM)** opens a preview sheet, e.g. "Food 40 → 60 · +3 to Unassigned · Lady Vex favor 1 → 3". Confirming runs, in one transaction:
  1. Lock the colony.
  2. `UPDATE quest SET status='COMPLETED', resolved_day=colony.day, resolved_at=now() WHERE id=$1 AND status='ACTIVE' RETURNING *`. If no row is returned: **409 "Already resolved"**. A second click pays nothing.
  3. Apply `reward_effects` in CLAMP mode, with colonists going to Unassigned.
  4. Add `reward_favor` to the patron.
  5. Write `QUEST_COMPLETED`.
     - It is PUBLIC if the quest is revealed, otherwise DM.
     - The favor line appears in PUBLIC text only if the patron is revealed; otherwise it goes in `dm.data`.
  - A quest with an empty reward completes fine.
- **Fail (DM).** ACTIVE → FAILED. **No effects**, as today. Writes `QUEST_FAILED`.
- **Reopen (DM).** FAILED → ACTIVE. Writes `QUEST_REOPENED`.
  - By default a completed quest doesn't reopen. A mistaken Complete is fixed with Undo (while it is the latest change) or an adjustment.
  - Whether completed quests can reopen, and whether completing one again pays again, is Open Question 14.
- **Other rules.**
  - Loot is display-only.
  - Missions are treated as quests (Open Question 13).
  - `kind` (MAINLAND or COLONY) is a label with no logic.
- **Delete (DM).** A hard delete after confirmation. Its timeline entries keep their text, and `quest_id` becomes NULL.

### 5.7 Patrons, favor and the request → approve flow (decision 7)

- **Gaining favor:** quests completed for that patron (§5.6), plus DM adjustments (§5.4).
- **Defining actions (DM):** name, description, `favor_cost`, `effects`, and a Visible switch that starts off (Open Question 9).
- **Archiving (DM).** Patrons and actions are archived, never deleted. Archived ones disappear from pickers and from the players' lists. Their history, requests and timeline links stay intact.
- **Request (player).** `POST /api/requests {actionId, note}`.
  - The action and its patron must be visible and not archived.
  - The request must be affordable right now in STRICT terms, otherwise 422 "Not enough favor: Lady Vex has 2, needs 3". The UI never offers an impossible request.
  - The server snapshots the title, the patron's name, the cost and the effects. Later edits to the action don't change what was asked for.
  - Status becomes PENDING.
  - After commit, Beny Bot posts to `discord_dm_channel_id` with **Grant** and **Decline** buttons (§7.5). The in-app badge backs this up if Discord is down.
- **Withdraw (requester).** PENDING → WITHDRAWN.
- **Approve (DM)**, from the approval sheet (§6.5) or the Discord button. In one transaction:
  1. Lock the colony, then the patron.
  2. Require `status = 'PENDING'`.
  3. Apply the cost and the effects in STRICT mode. The effects are the snapshot, or the DM's per-approval tweak stored in `applied_effects`. The snapshot itself is never changed.
  4. If anything is now short, return 409 with the shortfall text, e.g. "Not enough favor: Lady Vex has 2, needs 3". The DM can adjust favor and approve again.
  5. Mark the request APPROVED.
  6. Write `PATRON_ACTION`, e.g. "Thorin called in a Grain Barge from Lady Vex: Food +7 (favor −3)". It is PUBLIC if the patron is revealed, otherwise DM.
  - Nothing is reserved while a request is pending. Several pending requests can't overdraw, because approval checks again.
- **Decline (DM).** Optional note. DECLINED, and no timeline entry. The party sees the status and the note (Open Question 12).
- **Use action now (DM).** Creates and approves a request in one step, with an optional **on_behalf_of** player, so the timeline credits the right character.
- **No cooldowns, limits or quotas.** None have been designed (Open Question 12).

### 5.8 Timeline

| Kind | Written by | Day | Default visibility |
|---|---|---|---|
| `DAYS_ADVANCED` | Advance | new day | PUBLIC |
| `SHIPMENT_ARRIVED` (one per arrival, or folded beyond 12) | Advance | arrival day | PUBLIC |
| `SHIPMENT_SCHEDULE_CHANGED` | Schedule set or clear | current | PUBLIC |
| `CARGO_RECORDED` | Record cargo | current | PUBLIC (switchable) |
| `STOCK_ADJUSTED` | Adjust | current | **DM** (opt-in PUBLIC) |
| `FAVOR_ADJUSTED` | Favor adjust | current | **DM** (opt-in PUBLIC if the patron is revealed) |
| `DAY_CORRECTED` | Correct day | new day | DM |
| `WORKFORCE_CHANGED` | Transfer or set count | current | **DM** (opt-in PUBLIC) |
| `COLONIST_JOINED` / `COLONIST_LEFT` | Named NPC changes | current | PUBLIC if the NPC is revealed, else DM |
| `QUEST_POSTED` | Quest becomes visible (create or reveal) | current | PUBLIC (suppressible) |
| `QUEST_COMPLETED` / `QUEST_FAILED` / `QUEST_REOPENED` | Quest actions | current | PUBLIC if the quest is revealed, else DM |
| `PATRON_ACTION` | Approve, use action | current | PUBLIC if the patron is revealed, else DM |
| `SESSION_PLAYED` | Discord event ACTIVE, or Mark played | `colony_day_start` | PUBLIC |
| `UNDO` | Undo | current | PUBLIC if any undone entry was PUBLIC, else DM |
| `CUSTOM` | DM | any day (past or future) | chosen by the DM |

- **Editing.**
  - Auto-logged entries **can't be deleted**. The DM can edit their title, body and visibility, and editing never reverses effects.
  - CUSTOM entries can be deleted.
  - Undone entries stay, struck through for the DM and hidden from players.
- **Page behavior.**
  - Days are paged seven at a time, labelled "Days 36–42". This is pagination only, not a game unit.
  - Day 0 shows as "Founding".
  - The page opens on the page containing today, with today ringed. A "Jump to day" box handles very large days.
  - **Session bands** span `colony_day_start`–`colony_day_end`, e.g. "Session 14 · Days 36–41".
  - The DM has filter chips by kind and a "Hide DM-only" switch.

### 5.9 Secrecy: NPCs, places, quests, patrons

- **Hidden rows** (`revealed = false`) never reach players. This covers lists, detail pages, search, the colony roster, quest givers, place references, timeline links, SSE and Beny Bot.
- **NPC naming.**
  - Players get only `displayName`, which is `known_name` if it is non-empty, otherwise `name`.
  - `name`, `known_name`, `revealed` and `dm_notes` live in `NpcDm`.
  - Avatar initials are computed from `displayName`.
  - Player search matches `displayName` and aliases.
- **Every other NPC field keeps today's visibility:**
  - aliases (Open Question 8 asks whether to hide them);
  - appearance, personality, backstory;
  - age, class, level, race;
  - status, health, relation;
  - labels, player_notes;
  - places (when revealed);
  - the first-encountered session;
  - colony role.
- **`dm_notes`** on quests, patrons and places is DM-only.
- **Starting visibility.** Every create form has a **Visible to players** switch. It starts where today's app behaved, and where nothing existed before it starts hidden (Open Question 9):
  - NPCs start hidden, as today (`is_known_to_party` defaulted to false);
  - quests, places and maps start visible, because players saw every quest, location and region before;
  - patrons and patron actions start hidden, because players have never seen patrons.
- The DM reveals or hides a row with the switch, and a "Hidden" badge marks hidden rows in the DM's view.

### 5.10 Hex map (decision 9; history cut by F6)

- **Who and what.**
  - Any active member can paint, label or erase.
  - There is **no fog of war, no DM layer, no DM hex notes, and no app-placed content**. The migrations seed no hexes; the players draw the new world themselves. (`colony seed` paints a few sample hexes in local development only, §9.)
- **Terrains.** One Postgres enum, generated into Go and into a TypeScript union: forest, plains, water, mountain, desert, swamp, tundra, volcanic.
  - The colours are ported from the old `map-page.tsx`: forest #15803d, plains #a3e635, water #3b82f6, mountain #78716c, desert #eab308, swamp #064e3b, tundra #e2e8f0, volcanic #c2410c.
  - Each terrain also has a **pattern**, so colour-blind players can read the map.
- **Writes.**
  - `POST /api/hexes/paint {cells:[{q, r, terrain}]}` takes up to 200 cells; longer strokes are split. **Painting preserves the label.**
  - `PUT /api/hexes/{q}/{r}/label` sets or clears the label (≤ 40 characters).
  - `POST /api/hexes/erase {cells:[{q, r}]}` removes those hexes, terrain and label together.
  - Every write sets `updated_at` and publishes the `hexes` topic.
- **No history (F6).** The table holds only the current map. Last write wins. Nothing records who charted a hex, and there is no stroke undo or revert, so an overwritten or erased hex is simply gone (an accepted risk, §12).
- **Reads and live updates.**
  - `GET /api/hexes` returns every charted hex.
  - On a `hexes` invalidation (§8.8), clients refetch it, at most once a second while strokes stream in.
  - If the 5,000-hex check in M4 shows full refetches are too heavy, add `?since=<updated_at>`. Nothing orders commits, so the client asks from a few seconds before its last fetch and re-reads rather than misses a slow concurrent write; erase then keeps a `terrain NULL` row so erasures show up.
- **Extent.**
  - With nothing charted, a radius-5 starter area of empty outlines is drawn.
  - Otherwise the map shows charted hexes plus a dashed **frontier ring** two hexes deep, so the map grows as players explore.
  - There is no boundary. The server rejects only coordinates beyond ±100,000, a technical guard against bad input that painting never reaches.

### 5.11 Sessions and time

- **One timezone for the table:** `settings.timezone`. A series stores:
  - `first_date` and an optional `last_date`;
  - `start_local`, e.g. 20:00;
  - `weekdays[]` (several are allowed);
  - `every_weeks`: 1, 2 or NULL for a one-off.
- **One Go function, `occurrences(series, tz, from, to)`.**
  - It returns UTC instants built with `time.Date(y, m, d, hh, mm, 0, 0, loc)`, so the local time stays fixed across DST changes.
  - Every-other-week parity counts from the week of `first_date`.
  - The web, ICS, reminders, Discord events and `/nextsession` all call it.
- **Session rows** are created on demand, keyed by `(series_id, local_date)`.
  - **Cancel** creates or updates the row as CANCELLED. **Restore** sets it back to SCHEDULED.
  - Because a cancellation is keyed by local date, it survives a change to the series' start time.
- **Played.**
  - When the Discord event goes ACTIVE, or the DM clicks **Mark played**, the session becomes PLAYED, `colony_day_start` is set to the current day, and `SESSION_PLAYED` is written.
  - When the Discord event COMPLETES, or the DM clicks **End session**, `colony_day_end` is stamped and the band is updated.
  - The DM can edit both stamps.
- **Recap.** Written by the DM and shown to everyone on the session page.
- **ICS.** `/calendar/{ics_token}.ics` lists SCHEDULED occurrences for the next 6 months and the past 30 days. There is one shared token, and the DM can regenerate it.

### 5.12 Worked examples (encoded verbatim in `internal/game/spec_examples_test.go`)

**A. Exact cadence (off-by-one fix)**

| Action | Day after | D | Countdown | Arrival logged |
|---|---|---|---|---|
| Set interval 7 on day 0 | 0 | 7 | 7 | none |
| Advance 6 | 6 | 7 | 1 | none |
| Advance 1 | 7 | 14 | 7 | Day 7 |
| Advance 7 | 14 | 21 | 7 | Day 14 |

The old app recorded arrivals on days 8, 16 and 24.

**B. Advance 10 with the food floor**

Start: Day 0, food 100. Roles: Farmer 3 unnamed + Brannoc (named) = 4, Builder 3, Soldier 2, Unassigned 3, so **12 colonists**. I = 7, D = 7.

| Day | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 |
|---|---|---|---|---|---|---|---|---|---|---|
| Food | 88 | 76 | 64 | 52 | 40 | 28 | 16 (arrival) | 4 | 0 | 0 |

- End: Day 10, food 0, colonists 12, D = 14 (in 4).
- Entries: `SHIPMENT_ARRIVED` on Day 7; `DAYS_ADVANCED` on Day 10, with `deltas {food: −100}`.
- Nothing else is recorded at 0 food.

**B2. The same week, recording cargo on arrival**

- Advance 7: food 16 on Day 7.
- **Record cargo** on that arrival: +30 food, 4 colonists. Food 46; Unassigned 7, colonists 16.
- Advance 3: food 30, 14, then 0.
- End: Day 10, food 0, D = 14. Entries:
  - `SHIPMENT_ARRIVED` on Day 7;
  - `CARGO_RECORDED` on Day 7;
  - `DAYS_ADVANCED` on Day 7 with food −84;
  - `DAYS_ADVANCED` on Day 10 with food −46.

**C. One advance crosses several shipments**

- Start: Day 3, I = 7, D = 7, food 500, 10 colonists. **Advance 30** (days 4–33).
- Arrivals on Days 7, 14, 21 and 28; D = 35 (in 2).
- Food = 500 − 300 = **200**.
- Entries: four `SHIPMENT_ARRIVED` and one `DAYS_ADVANCED`.

**D. Stall, clear, override, correction**

- **D1.** Day 20, the first schedule ever set, interval 7: D defaults to 27. The old app showed −13 forever.
- **D2.** Day 10, D = 14, I = 7. The DM lowers I to 3; D stays 14 and then runs 17, 20, … The old app stalled.
- **D3.** Day 10: the DM types D = 11, which is accepted. D = 10 is rejected (422).
- **D4.** Clear sets I and D to NULL and hides the countdown; later advances log no arrivals. Re-enabling with I = 7 on Day 30 defaults D to 37.
- **D5.** Day 10, D = 14, I = 7. Correct day to 30: D = 35, no arrivals, no upkeep, and a DM-only `DAY_CORRECTED` entry: "3 shipment dates skipped".

**E. Double submit.**
- `Advance {days: 10, expectedVersion: v}` succeeds: Day 20, version v+1.
- An identical second click gets 409 "The colony changed (now Day 20). Review and retry." Nothing changes.

**F. Completing a quest.**

Day 23. Quest "Clear the Wolf Den" (revealed), patron giver Lady Vex (revealed).
- Reward: `reward_effects {food: +20, morale: +5, colonists: 3}`, `reward_favor 2`, loot "Wolf pelts ×6".
- Before: food 0, morale 98, favor 1, Unassigned 0, colonists 16.
- After: food 20, morale 100 (applied +2; the preview said "capped"), favor 3, Unassigned 3, colonists 19.
- PUBLIC `QUEST_COMPLETED`: "Food +20 · Morale +2 · 3 colonists (Unassigned) · Lady Vex favor +2".
- A second Complete gets 409 "Already resolved".

**G. Request and approval.**

Day 12. Lady Vex has favor 5; food is 20. The action "Grain Barge" costs 3 favor and gives food +7.
- Ana (character Thorin) requests it: 5 ≥ 3, so it becomes PENDING with a snapshot, and Beny Bot posts Grant and Decline.
- The DM taps Grant: favor 2, food 27. PUBLIC `PATRON_ACTION`.
- Ana requests again: 422 "Not enough favor: Lady Vex has 2, needs 3".
- **Variant:** two requests are both made while favor is 5. The first approval succeeds (favor 2). The second gets 409 "Not enough favor: Lady Vex has 2, needs 3", and the DM declines it with the note "Vex is busy this week."

**H. Huge N.**

Day 40, I = 7, D = 42, food 300, 20 colonists.
- The DM types 1,000,000 and has to retype it to confirm.
- The preview reads "Day 40 → 1,000,040 · Food 300 → 0 · 142,857 shipment arrivals (Days 42 – 1,000,034, every 7)".
- End: D = 1,000,041 (in 1). Exactly **two** entries: `DAYS_ADVANCED` and one folded `SHIPMENT_ARRIVED` with `{first: 42, every: 7, count: 142857}`.

**I. Undo.**
- After A's "Advance 1" (Day 6 → 7), the DM taps Undo.
- Result: Day 6, D = 7, food restored. Both of that change's entries are marked undone, and a PUBLIC `UNDO` entry is written.
- If a request had been approved after the advance, only the approval could be undone (the version check).

**Properties, fuzzed:**
- the span engine equals the day-by-day reference implementation for random states and N ≤ 2,000;
- `Advance(a+b) == Advance(Advance(a), b)`;
- every balance stays within its bounds;
- `D > day` whenever a schedule exists;
- colonists always equal the role sum;
- a STRICT failure writes nothing;
- each quest and each request resolves at most once.

---

## 6. Screens and navigation

### 6.1 Shell

- **Phone (< 768 px).**
  - A bottom tab bar: **Colony · Explore · Quests · Timeline · More**.
  - "More" holds People, Patrons, Requests, NPCs, Places, Mainland and Sessions, plus Players and Settings for the DM.
- **Tablet and desktop.** A left sidebar with the same items.
- **Top bar:**
  - the colony name;
  - a **Live** dot, grey while reconnecting;
  - for the DM: a requests badge and the **View as player** toggle.
- **Editing.** The DM edits in place, with dialogs and bottom sheets. There are no `/edit` routes. Hidden rows show a "Hidden" badge to the DM and never appear for players.
- **PWA.** `manifest.webmanifest` (name, icons, `display: standalone`, theme colour) lets players add the site to a phone's home screen. There is no service worker and no offline mode.

### 6.2 Routes

Since M1.5 (F8) every route below lives under `/c/{slug}` (`/c/{slug}/timeline`, `/c/{slug}/players`, and the board at `/c/{slug}/`), and `/pending` is `/c/{slug}/pending`. Outside campaigns: `/` (the picker of your campaigns, an invite-link box and "Run your own campaign"), `/join/{inviteCode}`, `/request` and `/admin` (site owner). These pages have no stream and refetch on focus. The top bar's title becomes a campaign switcher when you have more than one campaign, and the profile menu adds "All campaigns", "Leave this campaign…" (players) and "Site admin" (owner). Settings adds the Campaign name ("The site owner sees this name"), Invite link, Discord server (link code, linked server, channel pickers) and Danger zone (delete, C3) cards. Multi-campaign design §5 has the screens and flows.

| Route | DM sees and does | Player sees and does |
|---|---|---|
| `/login` | "Sign in with Discord" over darkened Eldari art; one-line privacy note. Devauth builds add a **Dev sign-in** list of members (§8.5). | same |
| `/pending` | — | "Waiting for the DM to let you in". Opens live when approved. "You don't have access" if removed or denied, including after signing in again. Sign out. |
| `/` Colony board | Day dial ("Day 44 · next ship Day 47, in 3 days"); resource tiles with live delta chips; colonists strip ("62 colonists · 5 unassigned"); **Advance** chips (+1 / +3 / +7 / Custom) → preview sheet → confirm; Adjust; Shipments; Correct day; **Undo last change** (shows what it undoes); pending requests with Review; last 5 timeline entries; next session; last recap snippet | Same information, read-only; "Ask a patron"; my requests |
| `/people` | Role table (unnamed + named = total), Unassigned highlighted when > 0; transfer and set-count dialogs; named roster with join, change role and leave | Role totals; revealed named colonists |
| `/timeline` | All entries (DM-only marked with an eye-slash; undone struck through); session bands; add a CUSTOM entry on any day; edit text and visibility; delete CUSTOM entries; filters; jump to day | PUBLIC entries, session bands, "Jump to today" |
| `/quests`, `/quests/$questId` | All quests by status; create (with the Visible switch and "Post to timeline"); reveal; reward editor (Effects + favor + loot); giver pickers; **Complete** (preview sheet), **Fail**, **Reopen** (failed only); `dm_notes` | Visible quests: Active / Done / Failed; description; reward; giver if revealed |
| `/patrons`, `/patrons/$patronId` | All patrons with favor; edit; archive; adjust favor; define and archive actions; request history; **Use action now** (on behalf of a player) | Visible patrons; favor; action cards (cost → effects) with **Request** (with a note); request history |
| `/requests` | Approval queue (pending first); approval sheet (§6.5); history | My requests (withdraw while pending); party history with decision notes |
| `/explore` | Hex map (§6.4), with the same tools as players | Hex map (§6.4) |
| `/maps/$mapId` (Mainland = Eldari) | Image map; Edit pins mode (place, drag, remove); upload or replace the image (pins survive) | Image map with revealed pins; tap a pin → sheet → place |
| `/places`, `/places/$placeId` | List and detail; CRUD; reveal; `dm_notes`; pin | Visible places, redacted; NPCs who are here |
| `/npcs`, `/npcs/$npcId` | Roster with search and filters; full edit; reveal; colony role; first-encountered session | Revealed NPCs under their display name, redacted |
| `/sessions`, `/sessions/$sessionId` | Series CRUD; upcoming dates (cancel or restore); history; recap editor; mark played or end session; edit day stamps; ICS link | Upcoming dates (cancelled ones struck through) in local time; history with recaps and "Days 36–41"; ICS link |
| `/players` | Pending (approve or deny); active (character names, remove); removed, left or denied, each labeled so (re-admit) | 403 |
| `/settings` | Campaign and colony name; timezone; resources (add, rename, icon, bounds, archive); roles; Discord channels and event location; bot invite link; ICS token | 403 |
| Profile menu | Character name, theme, **View as player**, sign out, sign out everywhere | Character name, theme, sign out, sign out everywhere |

### 6.3 Live at the table

**After the DM acts:**
- Every viewer's open queries refetch within about a second.
- Tiles animate to their new values (400 ms, or instantly under reduced motion) and show delta chips such as +20 or −39 for 15 s. The client computes the chips from the old and new values.
- A **toast** shows each new PUBLIC timeline entry's text, e.g. "3 days pass · Day 44. Food −39 · Shipment arrived Day 42". Toasts are built **only from PUBLIC timeline text**, so each toast is exactly as secret as its entry.
- The DM's own toast carries **Undo**.

**When a request is decided:**
- The requester's list updates.
- The PUBLIC `PATRON_ACTION` entry toasts for everyone.

**When something goes wrong:**
- **Two of the DM's devices race:** the second gets "The colony changed (now Day 44). Review and retry." with a fresh preview. Nothing is overwritten.
- **Reconnect after phone sleep:** the client refetches only if something changed while it was away (§8.8). The Live dot goes grey and then green.

### 6.4 Charting the hex map on a phone

- **Look mode (default).**
  - One finger pans; pinch zooms.
  - Tapping a hex opens a bottom sheet with its terrain and label; the label can be edited there.
- **Paint mode.**
  - Toggled by a large brush button.
  - Tap to paint; drag one finger to paint a stroke.
  - Strokes are sent every 250 ms and on lift as one batched call.
  - **Two fingers always pan and zoom.** One-finger map dragging is turned off in this mode. This gesture split is verified on a real phone at the start of M4 (§8.3).
  - The label is never touched.
- **Palette.** A horizontally scrolling chip bar: the 8 terrains, each with its pattern, plus **Unchart**. The selected chip is enlarged.
- **Frontier.** Dashed outlines ring the charted area. Painting one charts it, so the map grows outward.
- **Find.** Label search zooms to the hex. "Fit" shows everything that has been charted.
- **Labels** appear from a mid zoom level upward and are truncated with an ellipsis; the full text is in the sheet.
- **Desktop shortcuts:** `1`–`8` pick a terrain, `P` toggles Paint, `E` selects Unchart.
- **Live.** Other members' strokes appear within about a second.
- **Performance target:** smooth panning at 5,000 charted hexes on a mid-range phone, checked in M4 with `colony seed --hexes 5000`.

### 6.5 The DM's approval sheet

The sheet is full-screen on a phone, one tap away from the badge, the toast or the Discord message. Top to bottom:

1. The requester's avatar, character name, and note (in italics).
2. The action's cost and effects.
3. **Before → after:** "Favor 12 → 9 · Food 40 → 47".
4. A **validity line**: green "Affordable", or red "Not enough favor: Lady Vex has 2, needs 3".
5. **Tweak this grant** (collapsed): an editable effects list that applies to this approval only.
6. An optional note to the player.
7. Two large buttons: **Grant** and **Decline**.

### 6.6 Visual direction: "Night Sea and Parchment"

The palette comes from the Eldari map: dark teal sea, parchment land, gilt lettering, umber mountains.

- **Tokens** are defined on `:root` with light and dark variants.
- The theme follows the device setting, and the profile menu can force either.

| Token | Dark | Light ("Parchment") | Use |
|---|---|---|---|
| `--bg` | `#0E1A1D` | `#F3EAD6` | Page |
| `--surface` | `#15262A` | `#FBF5E8` | Cards |
| `--surface-raised` | `#1C3136` | `#FFFFFF` | Sheets, popovers |
| `--ink` | `#EDE3CC` | `#2A1E12` | Text |
| `--ink-muted` | `#B9AD92` | `#6B5A44` | Secondary text |
| `--gilt` | `#D4A84B` | `#7A5A1C` | Primary, focus, hairlines |
| `--verdigris` | `#4E8F86` | `#2F6D65` | Secondary actions, colonists |
| `--moss` | `#6E9A4C` | `#4F7A33` | Success, food |
| `--ember` | `#B5432F` | `#A0352A` | Danger, decline, failed |
| `--umber` | `#3A2614` | `#D9C7A0` | Pins, borders |

- **Type.** Self-hosted with `@fontsource`; no runtime Google Fonts.
  - **Cinzel** for display: titles, the day dial, the colony name.
  - **Alegreya Sans** for the UI, with tabular figures for every number.
  - **Alegreya Italic** for flavor text: timeline narration and request notes.
- **Components.** shadcn/ui on Radix, re-tokened with the palette above, plus lucide icons:
  - StatTile (with a delta chip);
  - DayDial;
  - BottomSheet;
  - TimelineEntry (with a typed glyph);
  - TerrainChip;
  - the Live dot.
- **Pins** keep the old dark-brown teardrop with a gold star.
- **Accessibility.**
  - AA contrast in both themes.
  - Touch targets of at least 44 px.
  - Motion under 400 ms, and none under `prefers-reduced-motion`.
  - Terrain patterns as well as colours.

---

## 7. Discord integration (Beny Bot)

### 7.1 Setup

- **In-process bot.** Beny Bot runs inside the binary using **discordgo v0.29.0**.
- **One application per running service.** The bot token and the OAuth2 client used for login come from the same Discord application.
  - **No two running processes ever share a bot token.** If they did, both would receive every interaction and both would answer.
  - Add `${PUBLIC_URL}/auth/discord/callback` as a redirect URI on whichever application the service uses. Locally that is `http://localhost:5173/auth/discord/callback`.
- **Development (M0–M5) uses only Beny Bot Dev (F3).** It is a separate test application that the author creates and invites to the author's private test server.
  - Local development normally leaves the Discord variables unset. Beny Bot is then off, the Discord button is hidden, and dev sign-in replaces it (§8.10).
  - To try real sign-in and Beny Bot, the author puts Beny Bot Dev's values in the local `.env`. `docs/discord-dev-setup.md`, written in M0, walks through creating the application.
  - The production Beny Bot application and its token are never used by this project. The old bot keeps serving the real table, untouched.
- **M6 (optional)** puts a production application in the real guild after the old bot is stopped for good (§10, Open Question 19). Beny Bot Dev is never promoted to production.
- **One shared bot, many servers (F8).** Each DM invites Beny Bot to their own server with the URL on Settings, and links that server to their campaign: Settings → **Get a link code** (`BENY-XXXXX-XXXXX`, 15 minutes, kept in memory, used up only by the `/link` that succeeds), then `/link code:` run in the server by a member with Manage Server. One server per campaign and one campaign per server. `/unlink`, Settings → Unlink…, or removing the bot (GuildDelete without Unavailable, or a server missing from a Ready that lists other servers; a Ready that lists none unlinks nothing) unlinks. Invite:
  - scopes `bot applications.commands`;
  - permissions: View Channel, Send Messages, Embed Links, Manage Events (8589954048). No Read Message History: no feature reads messages.
- **Commands are global** (`ApplicationCommandBulkOverwrite(app, "", ...)` on Ready). M1's guild-scoped commands are cleared once while the legacy `DISCORD_GUILD_ID` is still set; then it is deleted.
- **Routing.** Every interaction goes guild → linked campaign → membership (`core.ViewerForGuild`): a direct message, an unlinked server, a suspended campaign, a non-member and a not-yet-let-in member each get their own ephemeral line. Settings lists the linked server's text channels (`core.DiscordDirectory`), and every post goes through one function that checks the channel belongs to the linked server.
- **Intents:** `Guilds | GuildScheduledEvents`.

### 7.2 Port, fix, add, drop

| Item | Action | Details |
|---|---|---|
| Bot core, interaction routing (`internal/bot/bot.go`) | **Port** | Simplified; every interaction resolves guild → linked campaign → membership (`core.ViewerForGuild`, §7.1), and commands receive that `Viewer`. |
| Per-user rate limiter (`ratelimit.go`) | **Port as is** | Burst 3, one token every 2 s. |
| Command registration | **Fix** | Since M1.5: global `ApplicationCommandBulkOverwrite` on Ready, and a one-time clear of the legacy guild's commands (§7.1). M1 did: guild-scoped `ApplicationCommandBulkOverwrite`, **plus** a global overwrite with an empty list, so the application never carries stale global commands. The old application's global `/schedule`, `/registercampaign` and `/available` leave the real guild at M6, when that application is removed from it after the old bot is stopped for good. If the author keeps the existing application instead (Open Question 19), the empty global overwrite removes them, and nothing re-adds them because the old bot no longer runs. |
| Caller identity | **Add** | Commands answer only ACTIVE members of the server's linked campaign; others get an ephemeral line saying why (§7.1). |
| `/beny` | **Port** | "Boy!" |
| `/colony` | **Add** | Ephemeral embed: day, resources, colonists (with Unassigned), next shipment, next session, site link. Built from the player projection. |
| `/nextsession` | **Port + fix** | The next SCHEDULED occurrence from `occurrences()`, shown with `<t:…:F>` and `<t:…:R>`. |
| `/npc view name:` | **Port + fix** | Autocomplete and prefix search over revealed NPCs' display names. Ephemeral reply built from the player projection. A hidden true name finds nothing, and true names and DM notes never appear in Discord. |
| Pending sign-in notice | **Add** | "<name> (@user) is waiting to join: <url>/c/<slug>/players", posted to that campaign's DM channel, 5 an hour per campaign (§2.1). A campaign request DMs the site owner, 5 an hour. |
| Request prompt | **Add** | Posted to the DM channel with Grant and Decline buttons (§7.5). |
| Discord scheduled events | **Redesign** | Rolling single events (§7.4). The old recurrence code, including its `end` field (which apps can't set), is **not** ported. |
| Played-session auto-log | **Port + extend** | ACTIVE → PLAYED and `colony_day_start`; COMPLETED → `colony_day_end`. |
| Reminders | **Port + fix** | Timers (§7.3); message text ported. |
| Cancellation notice | **Port** | Old text, posted to the reminder channel, falling back to the announce channel. |
| `/npc set`, `/registercampaign`, `/schedule`, polls, multi-guild OAuth install | **Drop** | See §3.4. |
| Timeline posts to a channel, `/chronicle`, `/request` | **Next** | Open Question 17. |

- **Every Beny Bot output uses the player projection, and every command reply is ephemeral**, whoever runs it (Open Question 11). DM-channel posts carry only member names and the request itself.
- **No outbox.** Discord posts are sent after commit and are best-effort. A failed post is logged, and the in-app badge and pages still show everything.

### 7.3 Scheduler: in-memory timers, no polling

- **Planner.**
  - Reads series, session rows and settings.
  - Computes the earliest due instant among the jobs below, including the expiry of any event lease.
  - Sets one `time.Timer`.
- **It re-plans:**
  - at startup;
  - after any series, session or settings write (an in-process call after commit);
  - after every fire.
- **Jobs:**
  1. **Discord event.** For each series with `announce`, its next SCHEDULED occurrence must have an event. If it is missing, the job is due now. After that occurrence ends, it rolls to the next one.
  2. **Reminder.** Due at `starts_at − 24 h` for the next SCHEDULED occurrence of each series with `remind`.
- **Claim, then send.** Each job claims its row before calling Discord, so a restart or a deploy overlap can't double-post:
  - **Reminders:** `UPDATE session SET reminded_at = now() WHERE id = $1 AND reminded_at IS NULL RETURNING …`.
    - On a known send failure, `reminded_at` is cleared so the next plan retries.
    - A crash between claim and send loses that one reminder rather than risking a duplicate.
  - **Discord events use a lease:** `UPDATE session SET discord_claimed_at = now() WHERE id = $1 AND discord_event_id IS NULL AND (discord_claimed_at IS NULL OR discord_claimed_at < now() - interval '5 minutes') RETURNING …`.
    - On a known failure the claim is cleared.
    - After a crash, the lease expires after 5 minutes and the planner tries again.
    - Before creating, the bot lists the guild's scheduled events and adopts any it created with the same name and start time. A crash after Discord accepted the event therefore can't produce a duplicate.
- **Database load.** The database is touched only when a timer fires or someone uses the app, so a weekly game causes a handful of wake-ups a week. If the service was down when a reminder was due, the reminder is sent at startup, as long as the game hasn't started yet.

### 7.4 Discord event lifecycle (rolling single events)

- **Create.**
  - One external event for each series' next occurrence: name = title, location = `settings.discord_event_location`, end = start + duration.
  - Its ID is stored on the `session` row.
  - Then post "**<title>** is scheduled for <t:…:F> (<t:…:R>)" to the announce channel.
- **Cancel a date.** Delete its event if it exists and post the cancellation notice. The planner then creates the event for the following occurrence.
- **Restore.** The planner recreates the event if this is now the next occurrence.
- **Edit a series.** Update the existing event to the new next occurrence.
- **Delete a series.** Delete its event.
- **ACTIVE / COMPLETED** updates on events the bot created drive the played stamps (§5.11). Other events are ignored.
- **Why single events.** Through the API, a WEEKLY recurring event can use only one weekday, and apps can't set its end. Single events avoid both limits, make cancel, restore and edit exact, and allow series on several weekdays.

### 7.5 Grant and Decline buttons

- **Format.**
  - Custom IDs are `req:{id}:grant` and `req:{id}:decline`.
  - The message reads: "Thorin requests **Grain Barge** from Lady Vex (3 favor → Food +7). Favor now 5."
- **Click handling.**
  - The handler routes the click through the server's linked campaign and checks the clicker is that campaign's DM; core loads the request with that campaign. Anyone else gets an ephemeral "Only the DM can decide".
  - It sends a **deferred update** within Discord's 3-second window.
  - It calls the same decide function as the website, applying the snapshot with no tweak and no note.
  - It then edits the message to show the outcome:
    - "Granted · Food +7 · favor 5 → 2";
    - "Declined";
    - "Not enough favor: Lady Vex has 2, needs 3. Open in app.", with the buttons kept.
- **Decisions made on the website** also edit the Discord message, via `request.discord_message_id`.
- **Deploy overlap.** Two processes may both receive a click. The status guard makes the second one a harmless no-op.

---

## 8. Architecture and stack

### 8.1 Shape

During M0–M5 everything runs on the author's Mac (F1):

```
Browser ──http://localhost:5173──▶ Vite dev server (web/, hot reload)
                                     │ proxies /api, /auth, /img, /calendar, /healthz; keeps the Host header
                                     ▼
                                   colony serve (one Go binary, devauth build, localhost:8080)
                                     ├─ /api/*        huma v2 JSON API (+ /api/campaigns/{id}/stream SSE)
                                     ├─ /auth/*       dev sign-in; Discord OAuth2 through Beny Bot Dev (optional)
                                     ├─ /img/*        images from Postgres (members only)
                                     ├─ /calendar/*   ICS feed (token)
                                     ├─ Beny Bot (discordgo gateway; off unless Beny Bot Dev's variables are set)
                                     └─ planner (in-memory timers)
                                           │ pgx/v5 pool
                                           ▼
                                   Postgres 17 in Docker (docker compose; goose migrations applied at boot)
```

- The same binary also embeds the built SPA and `/eldari.webp` (`mise run build`, then `colony serve`). That one-port, one-origin shape is what M6 would deploy, and it can be tried locally at any time.
- There is one service and one origin, so there is no CORS, no BFF and no second private port.
- Decision 11 says the old Go API's RPC port was private on Railway, and only the Clerk-webhook port had a public domain. Once deployed, the new service's single port is **public by design**. So from M0, every route declares its access (§2.2), and Go authenticates every request that touches data.

### 8.2 Repo layout (`colony-planner/`)

```
CLAUDE.md  AGENTS.md → CLAUDE.md    single agent/dev guide (kept short)
mise.toml                            pins go, bun, sqlc; tasks: dev, dev:phone, gen, test, check, build,
                                     seed, reset, fresh
compose.yaml                         Postgres 17 for development and tests (bound to 127.0.0.1, named volume)
.env.example                         variable names only; .env is gitignored
go.mod / go.sum
cmd/colony/main.go                   subcommands: serve | migrate | openapi | seed | reset
internal/game/                       pure rules + table/property tests (advance.go, steps.go, effects.go,
                                     workforce.go, occurrences.go, spec_examples_test.go)
internal/core/                       transport-neutral operations and DTOs, one file per feature: colony.go,
                                     people.go, quests.go, patrons.go, requests.go, timeline.go, npcs.go,
                                     places.go, maps.go, explore.go, sessions.go, members.go, settings.go;
                                     plus undo.go, redact.go (the `dm` walker), broker.go (SSE topics), errors.go
internal/app/                        HTTP transport only (huma): one thin handler file per feature
                                     (decode → call core → encode), access.go, transform.go (redaction
                                     Transformer), stream.go (SSE endpoint), app.go (route table)
internal/auth/                       Discord OAuth2, sessions + session cache, CSRF wrap; devauth.go (build tag `devauth`)
internal/store/                      sqlc output (generated; never hand-edited)
internal/bot/                        bot.go, commands.go, buttons.go, planner.go, ratelimit.go (calls internal/core)
internal/seed/                       the sample campaign for `colony seed` (§9)
db/migrations/                       0001_init.sql … (goose SQL: THE schema, enums included)
db/queries/                          colony.sql, quests.sql, … (sqlc input)
sqlc.yaml
web/                                 Vite SPA: package.json, vite.config.ts (dev proxy), index.html, src/,
                                     src/api/ (generated types + query hooks: the only web code that knows
                                     the transport), public/{eldari.webp, manifest.webmanifest, icons/}
web/embed.go                         //go:embed all:dist
scripts/check-docs.sh                every path named in CLAUDE.md must exist
docs/SPEC.md  docs/discord-dev-setup.md
.github/workflows/ci.yml             runs `mise run check` when the author pushes; deploys nothing before M6
```

`Dockerfile`, `.dockerignore` and `railway.toml` are added only in M6 (§10).

### 8.3 Stack and versions

These versions were checked on 2026-10-10, and every one is at least two weeks old. Any dependency without a version here gets its latest release that is at least two weeks old, pinned in `go.mod` or `bun.lock` at M0. M0 re-checks each version before pinning (`mise ls-remote go`, `bun info <package>`); if one isn't available, it uses the newest suitable release at least two weeks old and notes the change.

| Concern | Choice | Version | Why it earns its place |
|---|---|---|---|
| Backend | Go | 1.27 (toolchain go1.27.1; move to go1.27.2's security fixes once it is two weeks old, about 2026-10-22) | The author's direction. The bot, stdlib `CrossOriginProtection`, `embed`, `slog`. |
| HTTP / API | huma v2 on `net/http` ServeMux (humago) | v2.39.1 | Go structs become the contract: OpenAPI 3.1, struct-tag validation, problem+json, response Transformers. Default doc routes disabled (§2.2). **On trial (F5)**, and kept inside `internal/app` (§8.6). |
| DB driver | pgx/v5 (pgxpool: MaxConns 5, MinConns 0, MaxConnIdleTime 1 min) | v5.11.0 (must be ≥ 5.9.2 for GHSA-j88v-2chj-qfwx) | Standard; sqlc targets it. The pool holds no connections while idle. |
| Queries | sqlc (`sql_package: pgx/v5`, `emit_pointers_for_null_types`, `emit_all_enum_values`) | 1.31.1 | Compile-checked SQL and Go enums, generated from the migrations. Installed by mise, not globally. |
| Migrations | goose v3 library, SQL embedded | v3.28.0 | sqlc reads goose files. No goose CLI to install, and no migration tool in the runtime image. |
| Discord | discordgo | v0.29.0 | Existing bot code ports directly. |
| OAuth client | golang.org/x/oauth2 | v0.37.0 | Small and official. |
| Logging, timers | `log/slog`, `time.Timer` | stdlib | Replaces charmbracelet/log and robfig/cron. |
| Package manager | Bun (installs and scripts only; not a production runtime) | 1.4.2 | The author's tool. |
| Bundler | Vite | 8.3.1 | Standard SPA build. |
| UI | React | 19.3.0 | |
| Routing | TanStack Router, file-based, + `@tanstack/router-plugin` | 1.170.38 / 1.168.39 | Typed params; already familiar. No SSR behind a login. |
| Server state | TanStack Query | 5.103.2 | Caching, plus invalidation driven by SSE. |
| API client | openapi-fetch + openapi-react-query; openapi-typescript (dev) | 0.17.0 / 0.5.4 / 7.13.0 | Typed paths and bodies from huma's spec. If the pre-1.0 openapi-react-query becomes a problem, replace it with plain `queryOptions` helpers over openapi-fetch. |
| Styling | Tailwind + `@tailwindcss/vite`; shadcn/ui (radix-ui), lucide-react, sonner, `@fontsource` (Cinzel, Alegreya, Alegreya Sans) | Tailwind 4.3.3 | Familiar. Copied components add no runtime framework. |
| Maps | Leaflet + react-leaflet, `CRS.Simple`, for both the Eldari image map and the hex map | 1.9.4 / 5.0.0 | One pan, zoom and pinch stack. Leaflet 2.0 is still pre-release. react-hexgrid and honeycomb-grid are both dormant and dropped. **To verify at the start of M4:** two-finger pan and pinch still work while one-finger dragging is off in Paint mode. If Leaflet's touch-zoom handler doesn't pan, a small pointer handler drives `setView`. |
| Hex drawing | One custom Leaflet canvas layer: fills, `createPattern` patterns, labels by zoom; hit-testing by axial rounding of the pointer | — | `L.Polygon` per hex can't carry patterns and costs an object per hex. |
| Forms | Native `<form>` + controlled inputs; server 422 field errors mapped onto inputs | — | No Zod or react-hook-form mirrors. The server is the validator. |
| Lint / format | Biome (TS); `gofmt`, `go vet`, golangci-lint v2 with depguard + forbidigo (Go) | Biome 2.5.14 | One tool per language; keeps the rules package pure. |
| Typecheck | `tsc --noEmit` | TypeScript 7.0.2 (fall back to 6.0.3 if editor tooling lags) | Fast agent feedback loops. |
| Database | Postgres 17: the `postgres:17` image via docker compose during M0–M5; a new Neon database `colony` at M6 | 17 | F1. The old database is never read (F2). |
| Hosting | None before M6; then Railway, one service, 1 replica | — | F1; decision 11 applies from M6. |

**Rejected:**
- **ConnectRPC + protobuf, for now.** A second schema language, `buf`, numeric enums and Timestamp mapping: 5 touches per field instead of 4. huma is on trial (F5); if the author prefers Connect, the switch rewrites only the transport layer (§8.6).
- **TanStack Start / SSR.**
- **Clerk** (§8.5).
- **Zod mirrors.**
- **Nx.**
- **testcontainers.** The compose Postgres, and CI's service container, are enough.
- **An object-storage bucket** (until the image budget in §8.9 is reached).
- **rrule-go.**
- **Resend.**
- **An outbox table and advisory-lock leader election.**

### 8.4 API contract, access and codegen

- **Operations.** Handlers are huma operations, registered through `app.Register(api, access, op, handler)`. The `access` value is required and is written as `x-access`. Non-huma routes come from the route table in `app.go`, which also requires an access level. Startup exits if anything lacks one (§2.2).
- **Types.**
  - Request and response types are Go structs in `internal/core/<feature>.go`; the huma handlers use them directly as bodies (§8.6).
  - DM-only fields live in a `Dm` pointer sub-struct.
  - Closed sets are the sqlc-generated enum types. Their huma `SchemaProvider` lists the values from the generated `AllXValues()`, so they arrive in TypeScript as string unions. A migration is the only place a value is written.
- **Redaction** happens in one response Transformer (§2.2).
- **Updates.**
  - Edits replace the whole record with PUT. This removes the old COALESCE bugs ("can't clear a field", "description wiped").
  - Colony numbers never use PUT; they change only through the delta operations in §5.
- **One command: `mise run gen`.**
  1. `sqlc generate`.
  2. `go run ./cmd/colony openapi > web/src/api/openapi.json`.
  3. `bunx openapi-typescript web/src/api/openapi.json -o web/src/api/schema.d.ts`.

  `mise run check` runs it; CI, when the author pushes, also fails on any diff.
- **Docs.** huma's default routes are off. Interactive API docs at `/api/docs` and the OpenAPI document at `/api/openapi.json` are registered as `owner` routes (were `dm` before M1.5).

### 8.5 Auth: direct Discord OAuth2 (confirmed, F3)

| | Clerk + Discord social login | **Direct Discord OAuth2 in Go + server sessions (chosen)** |
|---|---|---|
| Vendors | Clerk **and** a Discord app (Clerk production needs your own Discord OAuth credentials anyway) | The Discord application the bot already uses |
| Moving parts | Clerk SDK, JWT/JWKS verification, a public webhook plus svix to sync users | Two handlers, one table, one middleware |
| Discord ID for Beny Bot | Indirect, through Clerk's external accounts | The primary identity |
| DM approval | Still needs our own member table | The same table |
| Revoking access | Waits for JWT expiry, plus our own check | A membership status change, read on every campaign request: immediate |
| Local development and tests | Needs a Clerk dev instance | Offline, via the `devauth` build tag |

**Flow**

1. `/auth/discord/login`:
   - sets a 10-minute `__Host-oauth_state` cookie;
   - redirects to Discord with scope **`identify` only**. There is no `guilds` scope, because there is no server check (F4).
   - PKCE isn't relied on. Whether Discord honors it for a confidential client is unverified, and the state cookie plus the client secret protect the exchange.
2. The callback:
   - checks the state;
   - exchanges the code;
   - reads `GET /users/@me`;
   - discards the Discord token.
3. It upserts the `account` (since M1.5; memberships are made by invite links, §2.1, and nobody is pinged). A removed member who signs in again stays REMOVED in that campaign.
4. It creates a session.
   - 32 random bytes go into **`__Host-colony_session`**: HttpOnly, Secure, SameSite=Lax, Path=/, 30 days.
   - Only the SHA-256 is stored.
   - The expiry and `last_seen_at` are written at most once a day.
   - Plain-http local development uses `colony_session` without the prefix.

**Session cache**
- The middleware caches session lookups in memory (token hash → account, Discord ID and view-as-player) for up to 24 hours. Status is per campaign and read by `EnterCampaign` on each campaign request, so it is not cached.
- Sign out, sign out everywhere and the view-as-player toggle evict the affected entries.
- One replica keeps the cache coherent. During a deploy overlap, the old container lives only seconds.
- So an ordinary request or SSE reconnect runs no session query.

**Removal and sign-out**
- Removing a member from a campaign rescopes their streams in that campaign to `me`; their session survives, so their other campaigns keep working.
- "Sign out everywhere" deletes the caller's sessions, evicts them and closes their streams.

**Dev sign-in**
- A `devauth` build tag adds `/auth/dev?as=owner|dm|player|pending|<account id>` (dm, player and pending mean Sample Colony's) and `?as=new&name=`, and `/login` then lists every account once with its roles under **Dev sign-in**. `mise run dev` always builds with it, so local work needs no Discord account. Integration tests use it too.
- The seeded Sample DM takes `SITE_OWNER_DISCORD_ID` when it is set, so real sign-in through Beny Bot Dev lands on the same account. When it is unset, a devauth build treats the seeded `seed:dm` account as the site owner (§9).
- The binary refuses to start with the tag when `APP_ENV=production`, and the M6 Dockerfile never sets it.

**Secrets:** the app has no signing keys, so there is nothing like the old RSA keypair to leak.

### 8.6 Backend layering, swappable transport and extension seams

**Layers**
- **`internal/game`** holds the deterministic rules: `Advance`, the `Step` pipeline, `ApplyEffects(mode)`, `Colonists`, `Transfer`, `Occurrences`, and the undo-snapshot shapes. It does no I/O.
- **`internal/store`** is sqlc's output. All SQL lives in `db/queries/`.
- **`internal/core/<feature>.go`** holds every operation as a plain Go function. It takes a context, the `Viewer` and an input struct, and returns a DTO (with its `Dm` sub-struct) or a typed error: conflict, invalid, forbidden or not found. Colony mutations follow the write path in §5.1: transaction, game call, store, timeline, publish. Beny Bot and `colony seed` call these same functions.
- **`internal/app`** is the transport and nothing else. Each huma handler decodes, calls one core function and encodes. One function maps core's typed errors to 409, 422, 403 and 404. Access levels (`x-access`), the redaction Transformer, the route table and the SSE endpoint live here too.
- **`internal/auth`** handles sign-in mechanics only: the Discord OAuth2 redirect, the session cookie and the in-memory session cache. Member and session data are core's (`internal/core/sessions.go`). Since M1.5 removing a player touches no session: core changes the membership and rescopes its streams, so their other campaigns keep working.
- There is no repository layer. Interfaces exist only where a seam needs one: the Discord client, `core.Notifier` (core → Beny Bot) and `core.SessionEvictor` (core → the session cache).

**Enforced boundaries.** Dependencies point inward: `cmd/colony → app → auth → core → store`, `cmd/colony → bot → core`, and `seed → core, store`. `mise run check` fails when:
- any package other than `internal/core` imports `internal/store`, pgx, `database/sql` or `db` (allow-listed exceptions: `internal/seed`, a fixture loader; `cmd/colony`; tests). The rule covers the whole module, so packages added later are included;
- `internal/core` imports net, `net/http`, huma, discordgo, oauth2 or any of its callers;
- huma appears anywhere outside `internal/app`; auth imports app; or app, auth and bot import each other in any other direction;
- core's exported API mentions a store or pgx type (`internal/core/boundary_test.go`, since Go's type inference would otherwise let a returned `*store.Queries` leak past every import rule);
- a Go file uses a build tag other than devauth, which would hide it from the two builds the checks run.

On the web, Biome allows the server transports (fetch, EventSource, XMLHttpRequest, WebSocket, bare or through window/globalThis/self, navigator.sendBeacon) and the openapi-fetch / openapi-react-query client libraries only in `web/src/api/`, the one seam to the server.

**Swapping the API toolchain (F5).** huma is on trial. If the author switches back to protobuf/Connect, the rewrite is confined to:
- `internal/app` (handlers, error mapping, access declarations, the redaction hook) and the transport half of the secrecy test;
- the codegen step in `mise run gen` (`buf generate` instead of the OpenAPI export);
- `web/src/api/`, the only web code that imports the generated client. Screens use its hooks and type names, so they change only where field shapes differ.

`internal/game`, `internal/core`, `internal/auth`, `internal/store`, the migrations, Beny Bot and the seed stay as they are. depguard keeps huma out of every package but `internal/app`, so this stays true (§8.12).

**Seams for decision 10.** No rules are invented now. Each future feature uses whichever of these seams its rules call for.

| Future feature | Seams it may use, if its rules call for them | Existing tables changed |
|---|---|---|
| Buildings | New tables; `BUILDING_*` timeline kinds; `Effects` for anything that changes stock; a `Step` if buildings act over days | none expected |
| Sickness | NPC `health` (already has SICK); DM-defined resources or roles; a `Step` if it changes over days; `SICKNESS_*` kinds. If per-colonist state is needed, `game.Colonists` is the one function to revisit. | none expected |
| Events | `EVENT_*` kinds; `Effects` for outcomes; a `Step` if events trigger on their own | none expected |
| Store | `request.kind = STORE_PURCHASE`; the `cost` and `effects` snapshots; catalog and owned-item tables; colonists to Unassigned | `request` gains one nullable catalog column (additive) |
| New resource or role | `/settings` | **no code** |

### 8.7 How many places to add one field

To add a player-visible NPC field `voice`, make **4 hand edits**:

1. `db/migrations/00NN_npc_voice.sql`: `ALTER TABLE npc ADD COLUMN voice text`.
2. `db/queries/npcs.sql`: add it to `UpdateNpc`'s SET list. Reads use `sqlc.embed(npc)`.
3. `internal/core/npcs.go`: add it to `Npc` (or `NpcDm` if it is secret), `toNpc()` and `NpcInput`. The huma handler passes these structs through unchanged. A secret field also gets a canary in the NPC fixture of `internal/app/secrecy_fixtures_test.go`, or the secrecy test can't see it leak.
4. `web/src/routes/npcs.$npcId.tsx`: display and input.

Then run `mise run gen`. Compiler errors point at anything missed.

The old stack needed about 10 edits in 4 languages. A new resource or role touches 0 files. A new entity is about 6 files: a migration, a queries file, a core file, a thin handler file, a route file and a nav entry, plus a game file if it has rules.

### 8.8 Live updates

- **`GET /api/campaigns/{campaignId}/stream`** (`pending` level) is a Server-Sent Events stream behind session auth, with an in-process broker. There is one stream per open campaign tab; pages outside campaigns have none. Broadcasts reach only that campaign's ACTIVE streams, and sequence numbers and the 256-message ring are per campaign (F8).
- **Messages are invalidate-only:** `event: invalidate` with `data: {"topics":["colony","timeline"]}`. They carry no data and no summaries.
  - Each message carries `id: <bootId>-<seq>`. `bootId` is random per process start, and `seq` increments with every published message.
  - Pending and removed sessions receive only their own `me` topic, which is how `/pending` opens live.
- **Timing.**
  - A heartbeat comment every 20 s. Heartbeats touch no database.
  - The server closes each stream at about 10 minutes, under the 15-minute proxy cap of the M6 host (Railway), and `EventSource` reconnects on its own with `Last-Event-ID`. Locally this costs nothing, and it keeps behavior the same before and after M6.
  - The client also closes the stream while the tab is hidden (`document.hidden`). When the tab is visible again, it reopens the stream and passes its last ID as `?last=`.
- **Reconnect without refetch.** The broker remembers its last 256 messages and who each one reached (every active stream, one member, or one session). On connect, it compares the client's last ID with its own:
  - same boot and same sequence: nothing happened, so it sends nothing;
  - same boot, an older sequence the remembered messages still reach back to: one invalidate for the topics this stream would have heard since then, or nothing when none of them was for it (a view-as-player switch in the DM's browser, a message to another member);
  - any other case (too far behind, a new boot, or no ID): one invalidate for everything.
- **A reconnect runs exactly one read**, the membership (`EnterCampaign`), and no write (since M1.5; there is no membership cache). The session comes from the in-memory cache (§8.5), and `last_seen_at` is written at most once a day. An idle open tab runs no query. A tab left open between sessions therefore won't keep a hosted database awake (M6).
- **Client.**
  - Maps topics to query-key prefixes and invalidates them.
  - After a `timeline` invalidation, it fetches the newest PUBLIC entries and toasts them.
- **Hexes** refetch the whole map on a `hexes` invalidation, at most once a second while strokes stream in (§5.10).
- **Fallback.** If the M6 host's proxy misbehaves, `refetchInterval` can be switched on per query; the keys are the same. Polling is never the default, because polling keeps a hosted database awake.

### 8.9 Images and assets

- **Eldari**
  - The 3.7 MB `Eldari.png` from the old repo, the only asset carried over (F2), is converted to `web/public/eldari.webp` (about 1 MB) and shipped inside the binary at `/eldari.webp`.
  - The seeded map row stores its natural 1920×1080 size.
- **DM uploads**
  - Map images and NPC portraits go through `POST /api/images` into the `image` table: up to 5 MB each, image types only, width and height read on upload.
  - They are served to members at `/img/{id}` with `Cache-Control: immutable`.
- **Storage budget.** Image bytes are budgeted at 150 MB. Locally this costs nothing; it matters once the site is hosted (M6).
  - Neon's free plan allows 0.5 GB of storage per project, history retention included.
  - Storage is checked on Neon's usage page along with CU-hours during M6.
  - Past the budget, images move to a bucket behind the same `/img/{id}` URL, or the database moves to a paid plan.

### 8.10 Local development (M0–M5)

- **Prerequisites on the Mac:** Docker Desktop, running, and mise. `mise install` fetches the pinned Go, Bun and sqlc; nothing is installed globally.
- **Database.** `compose.yaml` runs `postgres:17`, bound to `127.0.0.1` only, with a named volume so data survives restarts. It holds two databases: `colony` for the app and `colony_test` for tests. Its password comes from `.env`.
- **`mise run dev`** starts the database if needed, then runs two processes:
  - `colony serve`, built with the `devauth` tag, on `localhost:8080`. It applies migrations at boot under goose's session locker; `colony migrate up|status` is kept for manual use.
  - The Vite dev server on `localhost:5173`, with hot reload. It proxies `/api`, `/auth`, `/img`, `/calendar` and `/healthz` to Go and keeps the browser's Host header, so cookies, `CrossOriginProtection` and SSE all see one origin.

  The author opens http://localhost:5173. Plain http uses the unprefixed `colony_session` cookie (§8.5).
- **`mise run dev:phone`** does the same, but also listens on the Mac's Wi-Fi address and prints it, so a phone on the same network can test touch gestures and live updates. Dev sign-in works there, which means anyone on that network could use it: use it at home. Real Discord sign-in is tested on the Mac only.
- **Sign-in.** Dev sign-in by default (§8.5). Real Discord sign-in and Beny Bot need Beny Bot Dev's values in `.env` (§7.1); without them, the Discord button is hidden and Beny Bot is off.
- **`.env`** is gitignored. `.env.example` lists only the names: `DATABASE_URL`, `TEST_DATABASE_URL`, `POSTGRES_PASSWORD`, `PUBLIC_URL`, `APP_ENV`, `DISCORD_CLIENT_ID`, `DISCORD_CLIENT_SECRET`, `DISCORD_BOT_TOKEN`, `SITE_OWNER_DISCORD_ID` (M1.5 removed `DISCORD_GUILD_ID`).
  - Logs carry IDs, kinds, counts and statuses, never campaign content or message text.
  - Locally, `PUBLIC_URL` is `http://localhost:5173` and `APP_ENV` is `development`.
  - Discord values, when set, are Beny Bot Dev's and the private test server's.
  - No production credential ever goes in it, and nothing is copied from the old repo's `.env` or `.pem` files.
- **Sample data:** `mise run seed`, `mise run reset` and `mise run fresh` (§9).
- **One-binary preview:** `mise run build`, then `colony serve`, serves the embedded SPA on one port, the shape M6 would deploy.

Deployment (Dockerfile, Railway, Neon, environment and cost) is designed under M6 (§10) and built only if the author approves it.

### 8.11 Testing

| Layer | Tests |
|---|---|
| `internal/game` | §5.12 examples A–I verbatim. Property and fuzz tests: span engine = day-by-day reference; `Advance(a+b) == Advance(Advance(a), b)`; bounds; `D > day`; colonist derivation; the overflow guard on day and next shipment day. CLAMP vs STRICT; transfer below 0 rejected; undo snapshot round-trip. `occurrences()` across both DST changes in America/New_York, biweekly parity, multi-weekday, one-offs, cancelled dates. |
| `internal/core` and `internal/app` (real Postgres via `TEST_DATABASE_URL`: the compose `colony_test` database, fresh schema per test, `devauth` sessions) | **Access + secrecy test** over every OpenAPI operation, every route-table route, SSE payloads and Beny Bot replies (§2.2). Double Complete pays once. Two concurrent advances with the same `expectedVersion` give exactly one 409. Request → approve applies the snapshot; a tweak lands in `applied_effects`; a short approval gives 409 and writes nothing. Undo restores, and is refused once something newer exists. A removed member is locked out of that campaign at once (403, and its streams rescoped to `me`) but keeps their account, their session and their other campaigns; signing in again leaves them removed there. A reconnect with an up-to-date last ID sends nothing and runs exactly one read, the membership, and no write. Two concurrent paint calls both land, and a refetch after their invalidations shows both. Repainting keeps a hex's label. Deleting an NPC colonist is refused. Reminder claim is single-send. `colony seed` succeeds on a fresh database and refuses a used one; `seed` and `reset` refuse `APP_ENV=production` and a non-local `DATABASE_URL`. |
| Migrations and SQL (M1.5) | `db/migrations_test.go`: an empty database migrates with no rows (no campaign until one is approved) and migrates down and up again; the schema itself keeps campaigns apart (composite foreign keys, keys per campaign, one active DM, `left_on_own` only when removed, one open request per account and per slug, a deleted campaign takes its rows with it). `db/migrate_test.go`: `serve`, `migrate` and `seed` refuse a database that records a migration this build doesn't have, and `reset` (which drops the schema before migrating) rebuilds it. `db/scope_test.go`: the SQL scope lint and its bad-query cases. |
| `internal/bot` | Guild routing (every refusal), `/link` and `/unlink` refusals (a refused `/link` keeps its code; two at once with one code link once), unlinking on GuildDelete and on a Ready that lists other servers (a Ready that lists none unlinks nothing), posting only into the linked server; button DM check, deferred reply then edit, status-guard no-op on a second click; planner due-time computation (fake clock); event lease expiry and orphan adoption. |
| Web | `tsc`, `biome ci`, `vite build`; `bun test` for hex math and the topic → query-key map. An optional Playwright smoke run (dev sign-in, advance, paint, request then grant). |

### 8.12 Checks (`mise run check`)

One task runs every check locally, and every milestone checklist ends with it:

1. `mise run gen`, so generated code is never stale.
2. `go vet ./...`.
3. `golangci-lint`:
   - depguard keeps `internal/game` free of `internal/store`, `net/*`, `database/*`, pgx, discordgo and `math/rand`, and keeps huma out of `internal/core`;
   - forbidigo bans `time.Now` in `internal/game`.
4. `go test ./...` against the compose database `colony_test`.
5. Web checks: `bun install --frozen-lockfile && bun run typecheck && bunx biome ci && bun run build`.
6. `scripts/check-docs.sh`.

It ends by printing "All checks passed" or the first failure. `.github/workflows/ci.yml` runs the same task with a `postgres:17` service container, plus `git diff --exit-code` after `gen`, whenever the author pushes. Nothing deploys from it before M6, which adds `docker build` and the deploy gate (§10).

### 8.13 Agent docs

There is one root `CLAUDE.md`, kept as short as its contents allow (M1.5 dropped the 150-line target); `AGENTS.md` is a symlink to it. There are no per-layer files and no ADR folder. It contains:

- the commands (`mise run dev|dev:phone|gen|test|check|seed|reset|fresh`);
- the layout map;
- **invariants:**
  1. game rules only in `internal/game`; SQL only in `db/queries/` (sqlc → `internal/store`); handlers in `internal/app` only decode, call one `internal/core` function, and encode;
  2. every colony change takes the colony lock, runs one transaction, writes grouped timeline entries, records the undo snapshot, and publishes;
  3. every route declares its access (huma operations via `x-access`, the rest via the route table), and secrets go in `Dm` sub-structs;
  4. never hand-edit `internal/store`, `openapi.json` or `schema.d.ts`;
  5. no PATCH semantics: full-record PUT or delta operations only;
  6. UTC instants, and local-time schedules expanded only by `occurrences()`;
  7. single replica;
  8. no timer that polls the database, and no per-request bookkeeping write;
  9. local only until the author approves M6: no deploy work, no production credential, and no secret value read, printed or copied from any `.env` or `.pem` file in either repo;
  10. (since M1.5) every campaign-owned table carries `campaign_id` and every query on one filters by it (`db/scope_test.go`); a `core.Viewer` is proof of membership and only core builds one; broadcasts are per campaign; logs carry IDs, kinds, counts and statuses, never campaign content;
- the **4-step add-a-field** and **add-an-entity** recipes, and "add a daily step";
- the **"do not invent game mechanics"** rule;
- the process: stop after each milestone for the author's check-in; **the author makes every commit, and agents never commit**;
- the author's conventions: no `any`, no `console.*`, no nested ternaries, full variable names, conventional commit messages.

`scripts/check-docs.sh` fails `mise run check` if a path named in CLAUDE.md doesn't exist.

---

## 9. Sample data: `colony seed` and `colony reset`

There is no importer (F2). The old app holds only throwaway test data, so nothing is read from its database and no old credential is needed. The one asset carried over is the Eldari art (§8.9). If the site is ever deployed (M6), production starts fresh.

**`colony seed`** (`mise run seed`) fills the local database with believable, clearly fictional sample campaigns (two since M1.5: Sample Colony, `/c/sample-colony`, with M1's numbers, and Second Sample, `/c/second-sample`, DM'd by Hollis, who also plays Pell in Sample Colony; Ana plays in both; plus the site owner and Wren's pending campaign request), so every screen has something to show and every checklist in §10 has something to act on.
- It grows with each milestone: each one adds the sample rows its screens need (table below).
- Wherever an operation exists, the seed plays its sample history through the same `internal/core` operations the API uses (advancing days, recording cargo, completing a quest, granting a request). Timeline entries, versions and the undo slot are therefore exactly what real play produces. Rows with no operation, such as members, are inserted directly.
- It is fictional on its face. The campaign is called "Sample Colony", the cast reuses this spec's example names (Lady Vex, Brannoc, Thorin, the Grain Barge, Clear the Wolf Den), and seeded members have Discord IDs like `seed:ana`, which no real account can have. The seeded Sample DM is the one exception: it takes `SITE_OWNER_DISCORD_ID` when that is set (§8.5).
- It runs only on a freshly migrated database. Otherwise it stops with "This database already has data. Run `mise run reset` first."
- `--hexes N` paints N sample hexes instead of the default handful, for the 5,000-hex check in M4.

**`colony reset`** (`mise run reset`) drops everything and re-runs the migrations. The result is a fresh install with no campaign: a campaign's colony, settings, resources and roles are created when the owner approves it.

**`mise run fresh`** runs reset, then seed. Every checklist starts from it.

**Safety.** `seed` and `reset` refuse to run when `APP_ENV=production` or when `DATABASE_URL` points anywhere but `localhost` or `127.0.0.1`. Tests never touch this database; they use `colony_test`, with a fresh schema per test (§8.11).

| Milestone | What `colony seed` adds |
|---|---|
| M0 | Settings (campaign name, timezone America/New_York); the DM; four active players with character names, Thorin among them; two pending players; one removed player. |
| M1 | A colony some weeks in: day, gold, food, materials and morale; a 7-day shipment schedule; a short history (a few advances, an arrival with recorded cargo, a DM-only adjustment). Role headcounts are set directly, so upkeep has colonists before the People screen exists. |
| M2 | Headcounts across the eight roles, with some Unassigned; NPCs, including hidden ones, one whom players know by a different name, and named colonists such as Brannoc the farmer; a few places. |
| M3 | Quests: active, completed, failed, one hidden, and one from Lady Vex with a favor reward. Patrons: Lady Vex (revealed, with favor), one hidden and one archived, with actions such as the Grain Barge. Requests: pending, granted, and declined with a note. |
| M4 | A small charted area on the hex map with a few labels; pins for the sample places on the Eldari map. |
| M5 | A weekly series with one cancelled date; two played sessions with recaps and colony-day stamps, so the timeline shows session bands. |

---

## 10. Build plan

Each milestone is a vertical slice that runs on the author's Mac (F1). It ends with a **local acceptance checklist** written for someone who doesn't write code. The author works through it and then either approves the milestone or lists what to fix; the next milestone starts only after approval (F7). Agents never commit; the author makes every commit.

**How to try a milestone** (the same every time):
1. Open Docker Desktop and wait until it says it is running.
2. In Terminal, in the `colony-planner` folder, run `mise install` (it does nothing if everything is already installed), then `mise run fresh` to load the sample campaign, then `mise run dev`.
3. Open http://localhost:5173. Use **Dev sign-in** to be the DM or any sample player. A private window, or a second browser, is a second person.
4. Work through the checklist. The last item is always `mise run check`, run in a second Terminal tab, which must end with "All checks passed".

For phone items, run `mise run dev:phone` instead of `mise run dev` and open the address it prints on a phone on the same Wi-Fi. Items marked **(Discord)** need Beny Bot Dev set up once (`docs/discord-dev-setup.md`, from M0) and happen in the private test server.

**The real table meanwhile.** The old site and the old Beny Bot keep serving the table, untouched, through M5. Nothing moves to the new site before M6, and M6 happens only if the author asks for it.

**M0: Skeleton, sign-in, local dev.**
- **Scope:**
  - repo, mise and its tasks (§8.10), docker compose Postgres, `.env.example`, the goose, sqlc and OpenAPI → TypeScript pipelines, `mise run check`;
  - the access startup check over huma operations and the route table;
  - `/healthz`;
  - dev sign-in; Discord login through Beny Bot Dev; DM bootstrap; Pending → approve → Active, deny, remove, re-admit, with the ping limit and the pending cap;
  - session cache;
  - Beny Bot core with `/beny` and the pending ping, in the private test server; `docs/discord-dev-setup.md`;
  - view as player;
  - theme tokens and fonts;
  - an empty Colony page and the settings singleton, including the Discord channels;
  - `colony seed` (members) and `colony reset`.
- **Acceptance:**
  - [ ] http://localhost:5173 shows the sign-in page over the Eldari art.
  - [ ] Dev sign-in as the DM lands on the Colony page.
  - [ ] In a private window, dev sign-in as a pending sample player shows "Waiting for the DM". I approve them on `/players`, and their page opens without a refresh.
  - [ ] I remove that player. Their window is signed out at once, and signing in as them again shows "You don't have access". Re-admitting them lets them back in. (Since M1.5 they stay signed in: the campaign shows "You don't have access" at once and their other campaigns keep working.)
  - [ ] (Discord) "Sign in with Discord" with my own account lands me on the Colony page as DM. Signing in with a second Discord account in a private window shows "Waiting for the DM", and Beny Bot Dev posts in my test server's DM channel.
  - [ ] (Discord) `/beny` answers "Boy!" in the test server.
  - [ ] `mise run check` ends with "All checks passed".

**M1: Colony clock and books.**
- **Scope:**
  - data-driven resources;
  - Advance N (chips, preview, large-N confirm), shipments, record cargo, Adjust, Correct day;
  - **undo**;
  - timeline page;
  - SSE with toasts and delta chips, Last-Event-ID resume, closing the stream while the tab is hidden;
  - the sample colony and its short history in `colony seed`.
- **Acceptance:**
  - [ ] After `mise run fresh`, the board shows the sample colony's day, stock and next shipment, and the timeline shows its history, with DM-only entries marked.
  - [ ] I set the shipment interval to 7 with the next arrival 7 days out. Advancing 6 days shows "in 1 day"; advancing 1 more logs the arrival on exactly that day, and the next one is due 7 days later (Example A).
  - [ ] Every advance shows a preview first, and the result matches it.
  - [ ] I type 1,000,000 days: I have to type it again, the preview counts the arrivals, and afterwards the timeline has exactly two new entries (Example H). Undo puts everything back.
  - [ ] Clicking Confirm twice quickly, or confirming the same advance in two tabs, changes the colony once; the second says "The colony changed" (Example E).
  - [ ] After an advance, Undo puts everything back. After I make another change, Undo offers only that newer change.
  - [ ] Record cargo on an arrival raises food and the Unassigned colonists, and the timeline links the cargo to the arrival.
  - [ ] Correcting the day forward past shipment dates writes a DM-only entry saying how many were skipped (Example D5).
  - [ ] Clicking "Clear schedule" hides the countdown. With no schedule, editing food and saving works.
  - [ ] I advance 3 days, and a player window shows the toast and the food chip within about 2 seconds, without reloading.
  - [ ] In view-as-player, I see no Advance, Adjust or Undo buttons and no DM-only timeline entries.
  - [ ] `mise run check` ends with "All checks passed". It runs examples A–I from §5.12 verbatim, and checks that an idle open tab and a reconnect run no database query.

**M1.5: Multi-campaign.**
- **Scope:** `docs/design/multi-campaign.md` with the author's decisions C1–C6: accounts and per-campaign memberships, owner-approved campaigns, invite links, path-scoped URLs, the seven access levels, per-campaign broadcasts, the cross-campaign secrecy sweep, one shared Beny Bot with `/link`, suspend and delete (the DM's own delete, C3), leaving (C5), the multi-campaign schema and two sample campaigns.
- **Acceptance:** `docs/checklists/M1.5.md` (multi-campaign design §10 plus the C3 and C5 steps).

**M2: People and secrecy.**
- **Scope:**
  - per campaign (F8): `npc` and `place` carry `campaign_id`; the role-key join includes `campaign_id`; the colonist count filters both tables; `/npc view` routes by the server's campaign; fixtures plant into both campaigns;
  - roles + Unassigned, transfers, named colonists;
  - NPC roster and detail with server-side secrecy and the first-encountered session;
  - places (unpinned);
  - `/npc view`;
  - the secrecy test in `mise run check`;
  - sample roles, NPCs and places in `colony seed`.
- **Acceptance:**
  - [ ] The Colonists tile always equals the sum of the role column.
  - [ ] Adding an NPC "as a new colonist" raises Colonists by 1; "promote an existing Farmer" leaves it unchanged.
  - [ ] Moving 3 people from Unassigned to Farmer works; moving more than exist is refused with a message.
  - [ ] Deleting an NPC who is a colonist is refused until I remove them from the colony.
  - [ ] In view-as-player, and signed in as a sample player, the NPCs marked Hidden appear nowhere (roster, colony list, search), and an NPC with a known name shows only that name, never the true one.
  - [ ] (Discord) `/npc view` with a hidden NPC's true name finds nothing, and every reply is visible only to me.
  - [ ] `mise run check` ends with "All checks passed", including the secrecy test.

**M3: Quests, patrons, requests.**
- **Scope:**
  - per campaign (F8): quests, patrons, actions and requests carry `campaign_id`; request IDs in bodies and buttons are loaded with the Viewer's campaign; Grant and Decline check the server's campaign and its DM; prompts go through `postToCampaign`;
  - quests with Effects rewards, favor and givers; QUEST_POSTED when a quest becomes visible; complete (preview), fail, reopen a failed quest;
  - patrons, favor, actions, archiving;
  - requests with the approval sheet;
  - Beny Bot Grant and Decline buttons;
  - sample quests, patrons, actions and requests in `colony seed`.
- **Acceptance:**
  - [ ] Completing a quest shows a preview, pays its reward and the patron's favor, and posts to the timeline (Example F). Clicking Complete twice, or from two tabs, pays once.
  - [ ] As a sample player, I ask Lady Vex for a Grain Barge. As the DM, I grant it on the approval sheet, and food and favor change on both screens. Once favor is too low, the player can't ask again, and the reason names the shortfall (Example G).
  - [ ] (Discord) The same request also appears in my test server's DM channel, and tapping **Grant** there works.
  - [ ] I can tweak a grant's food amount on the approval sheet; the request still shows what was asked.
  - [ ] "Use action now" on behalf of Thorin credits Thorin on the timeline.
  - [ ] Hidden or archived quests, patrons and actions never appear in view-as-player.
  - [ ] `mise run check` ends with "All checks passed".

**M4: World maps.**
- **Scope:**
  - per campaign (F8): maps, pins, hexes and images carry `campaign_id`; images are served under `/api/campaigns/{campaignId}/images/`; 50 MB of images and 50,000 hexes per campaign. **The Eldari art belongs to the author's campaign only (C6):** it is not seeded or offered to other campaigns as a starting map; other DMs upload their own mainland art;
  - Explore (canvas layer; Look and Paint modes; batched strokes; frontier; patterns; label search; live), starting with the two-finger gesture check on a real phone;
  - Mainland (Eldari) with pins; image uploads;
  - PWA manifest;
  - sample hexes and pins in `colony seed`, plus `--hexes 5000` for the performance check.
- **Acceptance:**
  - [ ] After `mise run fresh`, Explore shows the sample charted area inside its frontier ring, and the Mainland shows Eldari with the sample pins.
  - [ ] On my phone (`mise run dev:phone`), as a sample player, I switch to Paint, drag a forest stroke, pan with two fingers and label a hex. A player window on the Mac shows it within about 2 seconds.
  - [ ] Two players painting at the same moment both end up seeing both strokes.
  - [ ] Repainting a labelled hex keeps its label, and Unchart removes a hex.
  - [ ] Painting at the frontier makes the map grow.
  - [ ] I place and drag pins, then replace the Eldari image with a larger copy; the pins stay put.
  - [ ] After `mise run reset` and `mise run seed -- --hexes 5000`, panning and zooming stay smooth on my phone.
  - [ ] Chrome on the Mac offers to install the site. (Installing on a phone needs HTTPS, so that is checked in M6.)
  - [ ] `mise run check` ends with "All checks passed".

**M5: Sessions and Beny Bot.**
- **Scope:**
  - per campaign (F8): series, sessions and the ICS token per campaign; one planner across ACTIVE, linked campaigns (its queries `-- unscoped: planner`); events in each campaign's linked server; reminders through `postToCampaign`;
  - series with the timezone; `occurrences()`; cancel and restore;
  - rolling Discord events; timer-driven reminders; played auto-log with day stamps and timeline bands;
  - recaps; ICS;
  - `/colony` and `/nextsession`;
  - a sample series and played sessions in `colony seed`.
- **Acceptance:**
  - [ ] After `mise run fresh`, the timeline shows the sample sessions as bands, and their pages show recaps and their colony days.
  - [ ] A Friday 8:00 PM New York series lists every upcoming Friday at 8:00 PM, including the first Friday after the November DST change. (Discord) The next one has a Discord event at 8:00 PM.
  - [ ] (Discord) Cancelling the next date removes its Discord event and posts a notice, and the following week's event appears. Restoring brings the original back.
  - [ ] (Discord) A one-off session starting in 23 hours gets exactly one reminder right away, and restarting `mise run dev` doesn't send another.
  - [ ] (Discord) When I start and then end that session's Discord event, the session shows its colony days and the timeline shows the band.
  - [ ] Subscribing to the ICS link in Apple Calendar on the Mac shows the sessions. (Google Calendar needs a public address, so that is checked in M6.)
  - [ ] (Discord) `/colony` and `/nextsession` match the website.
  - [ ] `mise run check` ends with "All checks passed".

**M6: Cutover (optional, only after your approval).**

Nothing here happens unless, after approving M5, the author says the app meets their standards and asks for the cutover. Until then nothing is deployed, and the old site and the old Beny Bot keep serving the table.

- **Scope:**
  - **Multi-campaign (F8):** one production Discord application shared by every DM; `SITE_OWNER_DISCORD_ID` in Railway; budget for several campaigns on Neon; decide row-level security and whether a membership cache is worth it; a short privacy page now that friends' data is hosted;
  - **Hosting:** add the deployment files below; create the Railway service and a new Neon database `colony` (Postgres 17). The author enters the environment variables in Railway.
  - **A fresh start (F2):** production starts from the migrations alone, and `colony seed` refuses to run there. The DM enters the colony's real state through the normal screens: Correct day, Adjust, the shipment schedule, workforce, NPCs, quests, patrons, places and series.
  - **Domain:** move the author's existing domain from party-planner to the new service; `PUBLIC_URL` becomes that address.
  - **Discord:** stop the old bot for good, then delete the old recurring Discord event. Create the production Discord application (Open Question 19), add `${PUBLIC_URL}/auth/discord/callback` as its redirect URI, and invite it to the real guild. Remove the old Beny Bot application from the guild, which also removes its global `/schedule`, `/registercampaign` and `/available`. Set the channels on `/settings`. Production credentials go straight into Railway; agents never handle them.
  - **CI gate:** CI adds `docker build`, and Railway deploys `main` only after CI passes.
  - **Players** sign in with Discord, and the DM approves them.
  - **Retire the old app:** stop the old Railway services, and keep them and the old Neon database for 30 days, so rolling back means restarting them; the author deletes them when ready. Rotate or destroy the old Neon credentials, the RSA keypair and the old bot token.
- **Deployment design:**
  - **One service, built from a repo-root `Dockerfile`:**
    1. `oven/bun`, at the Bun version mise pins, builds `web/dist`.
    2. `golang`, at the Go version mise pins, runs `CGO_ENABLED=0 go build -trimpath ./cmd/colony` without the `devauth` tag, producing a static binary with the SPA and migrations embedded.
    3. `gcr.io/distroless/static-debian12` runs the binary as non-root.

    A `.dockerignore` keeps `.env` and other local files out of the build.
  - **Migrations** run at boot, before serving, under goose's Postgres session locker, as they do locally. A failed migration exits non-zero, so the deploy never goes healthy and Railway keeps the old version running.
  - **`railway.toml`:**
    - health check `/healthz`, which Railway calls only during deploys;
    - **1 replica**: Beny Bot, the planner, the session cache and the SSE broker live in-process, and CLAUDE.md documents this;
    - restart on failure;
    - deploys wait for GitHub CI.
  - **Deploy overlap.** For a few seconds two containers can run. Every side effect is idempotent (claim-then-send, leases, status guards, `ON CONFLICT`), so no leader lock is used.
  - **Environment variables:** the names in `.env.example` (§8.10) except `TEST_DATABASE_URL` and `POSTGRES_PASSWORD`. `DATABASE_URL` is the Neon **direct** endpoint (`sslmode=require`), `APP_ENV` is `production`, and the Discord values are the production application's.
  - **Cost.**
    - Railway Hobby runs one small always-on Go process.
    - Neon wakes only when someone acts or a timer is due. Open tabs, heartbeats and reconnects don't wake it (§8.8), so the compute can scale to zero between sessions.
    - Whether usage fits Neon's free plan (100 CU-hours a month and 0.5 GB of storage) is checked on Neon's usage page during M6, not assumed.
- **Acceptance:**
  - [ ] Pushing to `main` deploys only after a green CI run.
  - [ ] At my domain, "Sign in with Discord" lands me on the Colony page as DM, and there is no Dev sign-in (`/auth/dev` doesn't exist).
  - [ ] Every player has signed in and been approved.
  - [ ] `/schedule`, `/registercampaign` and `/available` are gone from Discord and stay gone after the new service restarts.
  - [ ] The next session has exactly one Discord event and gets exactly one reminder.
  - [ ] The ICS link works in Google Calendar, and the site can be added to a phone's home screen.
  - [ ] With a player tab left open overnight, Neon's dashboard shows the compute suspended. CU-hours and storage are checked on the usage page.
  - [ ] I run one full session using only the new site and Beny Bot.
  - [ ] The old site no longer serves the app. The old Neon credentials, the RSA keypair and the old bot token are rotated or destroyed.

---

## 11. Open questions for the author

These are in priority order. Each has a default, so building isn't blocked. Answered questions keep their numbers and say **Resolved**.

**Before M0**

1. **API toolchain.** **Resolved (F5):** huma/OpenAPI, on a trial basis. The transport layer stays thin, so switching back to Connect/protobuf rewrites only `internal/app`, the codegen step and `web/src/api/` (§8.6).
2. **Site address.** **Resolved (F1):** nothing is hosted before M6. At M6, the author's existing domain moves from party-planner to the new service, so bookmarks keep working.
3. **Import and handover.** **Resolved (F2):** no import and no handover. The old data is throwaway; local development uses `colony seed`, and a future cutover starts fresh.
4. **Who can ask to join.** **Resolved (F4):** anyone with the link can sign in with Discord and ask, with pings rate-limited and pending requests capped. There is no server-membership check, so no `guilds` scope. A denied or removed player can't ask again; you re-admit from `/players`.

**Before M1**

5. **What players see.** Should players see exact stockpile numbers, as today? Should they see each visible patron's exact favor? That would be new, since players have never seen patrons or favor. *Default: yes to both; they need favor to make requests.*
6. **Population import.** **Resolved (F2):** there is no import, so there is nothing to reconcile. Named NPC colonists count on top of each role's unnamed headcount (§5.5).
7. **Shipment cargo.** Should shipments stay "countdown + Record cargo when it lands", or have a standing manifest applied on each arrival (e.g. +20 food, +2 colonists, landing in Unassigned)? Could a patron change it? *Default: Record cargo.*

**Before M2**

8. **NPC secrecy depth.** Aliases stay visible to players, as today. Should they become DM-only, since an alias can give away a secret identity? Should backstory or personality also become DM-only? *Default: hide only the true name and DM notes; everything else stays visible as today.*
9. **Starting visibility.** New NPCs start hidden, and new quests, places and maps start visible, as today.
   - Should new quests and places start hidden instead, so you reveal them when the party learns of them?
   - Patrons and actions are new, so they start hidden, because players have never seen patrons. Is that right?

   *Default: as described.*
10. **First-encountered session.** Keep the NPC "first encountered in session" link? The old page showed it as a raw ID. *Default: keep it, as a link to the session.*
11. **Beny Bot replies.** Should Beny Bot's command replies always be ephemeral and show only what players see, even when you run them? *Default: yes; DM detail stays on the site.*

**Before M3**

12. **Patron rules.**
    - Any limits, cooldowns, or "one pending request per action"?
    - May your manual adjustments push favor below 0? Spending never can.
    - If an action's effects would take a resource below its minimum, should the request be refused with the shortfall named, or clamp?
    - Can anything besides a quest for that patron grant favor: an action, shipment cargo, a store item?
    - Should a decline note be visible to the whole party or only the requester?

    *Default: no limits; yes; refuse; no (Effects gains a `favor` field if you say yes); the whole party.*
13. **Missions.** Decision 7 says "quests & missions". Are missions different from quests, for example short patron tasks with their own flow? *Default: missions are quests; a patron-given quest pays favor to that patron.*
14. **Failures, departures and reopening.**
    - Should a failed quest ever cost something?
    - When colonists die or leave, do they come out of a specific role?
    - Can a completed quest be reopened, and would completing it again pay again?

    *Default: no failure effects; you use Set count; only failed quests reopen (Undo or Adjust fixes a mistaken Complete).*

**Before M5**

15. **The table.**
    - Is the game still a fixed weekly or every-other-week slot, and in which timezone?
    - Is "Forge + Discord Voice" still the right event location? It suggests Foundry on The Forge, but that is an inference.
    - Do you want a "Join the game" link?

    *Default: you enter the slot and timezone when you create the series and on `/settings`; the location is editable; no link.*
16. **Google Calendar.** The ICS feed puts sessions in anyone's calendar. It loses the old conflict warning (checking members' calendars for clashes while you schedule) and per-user calendar sync. Is ICS enough? *Default: ICS only.*

**Later**

17. **Beny Bot announcements.** Should Beny Bot post public timeline entries (advances, shipments, quests, grants) to a party channel? Do you want `/chronicle` and `/request`? *Default: off; Next.*
18. **Store details** (decision 6, a separate design effort).
    - Do purchases arrive at once or with the next shipment?
    - Who pays: colony gold, a party purse, or individual characters?
    - Who owns a bought magic item, now that player characters aren't tracked?

    *Default: decided in the store design; nothing in v1 depends on the answers.*

**Multi-campaign (M1.5), all resolved on 2026-10-11 (multi-campaign design, last section)**

20. **Co-DMs or hand-off.** **Resolved (C1):** no. One DM per campaign, no hand-off.
21. **Changing a campaign's address.** **Resolved (C2):** no. The owner can adjust the slug at approval; then it is fixed.
22. **A DM deleting their own campaign.** **Resolved (C3):** yes, at any time, after typing its slug; it cascades like the owner's delete. The owner can still suspend any campaign and delete a suspended one.
23. **A denied requester asking again.** **Resolved (C4):** yes, one open request at a time.
24. **A player leaving on their own.** **Resolved (C5):** yes, only themselves, from PENDING or ACTIVE, never the DM. Leaving is REMOVED, recorded as their own choice so `/players` shows "Left"; the invite link can't bring them back and pings nobody; only the DM re-admits.
25. **The Eldari art for other campaigns.** **Resolved (C6):** no. It belongs to the author's campaign; M4 must not offer it to other campaigns.

**Before M6 (only if you go ahead)**

19. **Production Discord application.** F3 keeps the production Beny Bot token out of this project. So by default M6 creates a new production application (it can still be called Beny Bot), invites it to the real guild, and removes the old Beny Bot application once the old bot is stopped. Players then see a new bot account; the old bot's past messages stay. Would you rather the deployed site take over the existing Beny Bot application, with you entering its credentials in Railway yourself? *Default: a new production application.*

---

## 12. Risks

| Risk | Likelihood / impact | Mitigation |
|---|---|---|
| The rebuild stalls before it replaces the old app | Medium / high | Vertical milestones, each tried on the Mac and approved by the author before the next (F7); sample data so every slice can be tried at once; lean scope; the old app and its bot keep serving the table, untouched, until the optional M6. |
| A secret reaches players through a new endpoint, SSE or Beny Bot | Medium / high | Fail-closed access on huma operations and the route table; SQL row filters; one redaction Transformer over `Dm` sub-structs; invalidate-only SSE; write-time text with player names only; ephemeral Beny Bot replies from the player projection; a canary test over every route, SSE and Beny Bot; view-as-player. |
| Hand-rolled auth or session bugs | Low / high | Small surface; hashed tokens; `__Host-` cookie; SameSite=Lax; stdlib `CrossOriginProtection`; role derived on every request; explicit cache eviction; tests for 401, 403, lockout and CSRF; devauth disabled in production. |
| Undo restores the wrong thing | Low / medium | Single level only; refused unless the version is unchanged; snapshot round-trip tests; entries marked undone, never deleted. |
| Something that works on localhost fails once hosted (Secure cookies, the host's stream cap, HTTPS-only phone install, Google Calendar fetching the ICS feed) | Medium / low | The one-binary shape runs locally at any time (`mise run build`, then `colony serve`); the 10-minute stream rotation is built from M1; M6's checklist re-checks each HTTPS-only item. |
| Neon usage exceeds the free plan after M6 (CU-hours or storage) | Low / medium | No polling timers; heartbeats and reconnects touch no database; in-memory session cache; daily-at-most bookkeeping writes; a pool with no idle connections; image budget; usage checked during M6, with Neon's paid tier as the fallback. |
| A huge or mistyped Advance | Low / low | Closed-form span engine; folded arrival entries (at most 2 + 12 rows); preview shows the arrival count; typed confirm above 365; overflow guard on day and next shipment; one-tap undo. |
| Single process (bot, planner, SSE, session cache) and deploy overlap | Low / medium | 1 replica documented; claim-then-send; event leases with orphan adoption; status guards; `ON CONFLICT`; goose session locker; the second click on a button is a no-op. |
| SSE dropped by the M6 host's proxy (Railway's 15-minute cap) or phone sleep | Medium / low | 20 s heartbeats; 10-minute rotation; resume by last event ID, refetching only after a change; Live dot; per-query polling switch. |
| huma/OpenAPI is new to the author; openapi-react-query is pre-1.0 | Medium / low | On trial (F5); the transport is confined to `internal/app` and `web/src/api/`, so switching back to Connect leaves rules, operations, SQL and screens alone; plain Go structs and JSON; generated TS types; `/api/docs`; recipes in CLAUDE.md; `queryOptions` fallback. |
| Hex map performance or gestures on older phones | Low / medium | One canvas layer, not an object per hex; labels by zoom; batched strokes; throttled refetch, with `?since=` in reserve; gesture check at the start of M4; 5,000-hex check with `colony seed --hexes 5000` in M4. |
| Accidental or deliberate wipes of the player map have no history | Medium / low | **Accepted (F6).** Last write wins, and nothing records who painted what or what was there before. Only active members can paint, the DM can remove a member at once, and players redraw from what they remember. |
| Leaked old secrets (RSA keys, `apps/web/.env` and `packages/database/.env` in the jj snapshot; the public key shipped to browsers until #100) | Known / high | The old repo is read-only reference: no `.env` or `.pem` value is read or copied, and its database is never connected to (F2); the new app has no signing keys and uses only local credentials until M6; M6 rotates or destroys the old Neon credentials, RSA keys and bot token; never push jj keep-refs. |
| Game design keeps changing (buildings, sickness, events, store) | High / medium | Rules isolated in `internal/game` with spec-example tests; resources, roles, actions and effects are data; the `Step` pipeline; open timeline kinds; generic request snapshots; "do not invent mechanics" in CLAUDE.md. |
| Old and new Beny Bot answering the same interactions | Low / medium | No two processes share a token: development uses only Beny Bot Dev in a private test server, and the production Beny Bot token is never used by this project (F3); at M6 the old bot is stopped for good before the production application goes live; command-cleanup check in M6 acceptance. |
| TypeScript 7 tooling gaps | Low / low | Pin 6.0.3 instead; it is used only for `tsc --noEmit`. |
| A query misses its campaign filter (a leak, or an update to every campaign's `gold`) (F8) | Medium / high | The Viewer type; the SQL scope lint; composite foreign keys; tokens in every value of the second campaign; its fingerprint after the DM and mixed-ID passes. |
| A body or jsonb ID from another campaign is accepted (F8) | Medium / high | Core loads every body and jsonb ID and key with the Viewer's campaign; sweep pass 4 puts the other campaign's real IDs and keys into every body field. |
| Roles judged against the wrong membership (a DM of B gets DM powers in A) (F8) | Low / high | A Viewer is built per campaign with no owner flag; a DM elsewhere is swept as a player; the owner-flag table test. |
| "The owner can't read" is only an app promise (F8) | Known / medium | Said on the request form; owner routes read directory tables only; no Read Message History; logs carry no content; row-level security decided at M6. |
| Beny Bot posts into the wrong server, or a server that removed it (F8) | Low / medium | Channels picked from the linked server; every post checks the channel's server; GuildDelete and Ready unlink. |
| A database migrated before M1.5's migrations were squashed skips a later migration that reuses a version (F8) | Low / medium | `serve`, `migrate` (up and status) and `seed` refuse a database that records a migration the build doesn't have and say to run `mise run fresh`; `reset` drops the schema before migrating, so `mise run reset` and `fresh` rebuild such a database; tested. |

---

## Appendix A. Design choices and alternatives

| # | Question | Ruling | Why |
|---|---|---|---|
| 1 | Undo in v1, and how? (A change table with before-state, no undo at all, or one snapshot) | **Single-level undo**, with the snapshot in **one `colony.last_change` column** and a `group_id` on timeline entries. No change or ledger tables. Undone entries are marked, never deleted. | One level needs one snapshot, not a table. The DM gets a safety net at the table, and the timeline stays an append-only story. |
| 2 | When CLAMP bites, log requested and applied, or applied only? | **Applied deltas only.** The preview shows "capped" before confirming. | Decision 5: nothing is tracked when food hits 0, and a requested-versus-applied record for upkeep would be exactly that. |
| 3 | Discord Grant and Decline buttons in v1, or later? | **v1.** `/request` and party posts come later. | One handler on the existing decide path, status-guarded and idempotent. It lets the DM decide mid-session without leaving Discord. |
| 4 | Migrations at boot, or as a separate step (at M6, a Railway pre-deploy step)? | **At boot**, locally and at M6, under goose's session locker. The `colony migrate` subcommand is kept, and `colony reset` re-migrates a wiped local database. | Locally, `mise run dev` needs no separate migrate step. At M6 it needs no extra Railway config and avoids pre-deploy hangs that others have reported. The lock is held only during migration, so a Neon suspend doesn't matter. |
| 5 | Hex history: an append-only edit log with stroke undo and revert, or none? | **None (F6).** The `hex` table holds only `q`, `r`, `terrain`, `label` and `updated_at`, and last write wins. | The author cut it to keep the map simple. Accidental wipes are an accepted risk (§12). |
| 6 | Aliases: visible to players, or DM-only? | **Visible, as today.** | Hiding them would change what players see without the author's say. Open Question 8 asks. |
| 7 | Stock adjustments: off the timeline, or on it as the audit log? | **Always written, DM visibility by default**, opt-in PUBLIC. Workforce and favor adjustments work the same way. | Keeps the audit trail without filling players' timeline with typo fixes. |
| 8 | Double-submit guard: `expectedDay` or `expectedVersion`? | **`expectedVersion` on every form-driven colony mutation**; status guards on decisions. | It covers the double-click case, also protects Adjust and the other operations, and the same version gates undo. |
| 9 | Unbounded N vs bounded writes | **Closed-form span engine + arrivals folded beyond 12 + only an integer-overflow guard.** | No cap (decision 4); a typo writes at most 14 rows; property tests prove it matches day-by-day. |
| 10 | Sessions: a horizon job that materializes rows, or rows created on demand? | **Rows created on demand**, keyed by `(series_id, local_date)`. The planner uses in-memory timers. | No horizon job and no polling; cancellations survive a time change. |
| 11 | Closed sets (including hex terrains): `text` + `CHECK` with Go lists, or Postgres enums? | **Postgres enums**, generated into Go by sqlc and into TypeScript through the OpenAPI schema. Terrain colours and patterns live in TypeScript. | One definition per set (principle 6). Terrain isn't an undecided game rule. |
| 12 | Hex rendering | **One custom canvas layer inside Leaflet `CRS.Simple`.** | `L.Polygon` can't carry the colour-blind patterns, and an object per hex slows phones. |
| 13 | Redaction mechanism | **`Dm` sub-structs + one huma response Transformer**; Beny Bot always uses the player projection. | Central rather than per handler, typed, and covered by the canary test. A JSON-level fallback exists if reflection proves unreliable. |
| 14 | Pending-member notification | **A rate-limited ping with a link**, no Admit buttons. | Approving on `/players` is one tap. |
| 15 | DM notes on hexes, a landing hex | **None.** | Decision 9: only the players map the new world. `colony seed` paints sample hexes in local development only. |
| 16 | The old app's data: import it, or start fresh? | **Start fresh (F2).** No importer: `colony seed` loads fictional sample data locally, and `colony reset` wipes and re-migrates. Only the Eldari art carries over. | The old app holds only throwaway test data. Dropping the importer also drops the `import_mark` table, nullable "Imported" attribution, the shipment-countdown mapping and population reconciliation. |
| 17 | Which Discord application does the new site run as during development? | **Beny Bot Dev, in a private test server, for all of M0–M5; the production Beny Bot token is never used by this project (F3).** At M6 a production application goes live only after the old bot is stopped for good (Open Question 19). | Two processes on one token would both answer every interaction, and the old bot re-registers its global commands on every start. Keeping the old bot whole means the table never loses reminders or its event, and keeping production credentials out of development removes a way to leak them. |
| 18 | Multi-campaign: how does a request know its campaign? (F8) | **Path-scoped URLs** (`/c/{slug}` in the SPA, `/api/campaigns/{campaignId}` in the API); no active-campaign cookie or header. | Explicit in every request, linkable from Discord, two tabs can sit in two campaigns, cache keys separate by themselves, and the secrecy sweep fills it like any path parameter. |
| 19 | One DM or several per campaign? (F8) | **One, stored**, with a partial unique index; no API writes `role` (C1). | Keeps "the DM" unambiguous; co-DMs later mean dropping one index and adding a promote operation. |
| 20 | Row-level security? (F8) | **Not now**; the Viewer type, the SQL scope lint, composite keys and the fingerprinted sweep instead. Decided at M6. | RLS needs a second login role and a transaction per read (four round trips to Neon), and still misses jsonb and body IDs. |
| 21 | A membership cache? (F8) | **No**: one indexed membership read per campaign request. | No stale grant to evict; a reconnect costs one read. Revisit at M6 only if Neon wake-ups show up in the bill. |
| 22 | Where do link codes live? (F8) | **In memory**, 15 minutes, single use, one per campaign. | Single replica (Invariant 8); a restart just means a new code. |
| 23 | Guild or global Beny Bot commands? (F8) | **Global**, with a one-time clear of M1's guild commands. | One shared bot serves every linked server; routing goes through the server's campaign. |
