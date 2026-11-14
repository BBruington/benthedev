# Foundry bridge: Colony Planner and a Forge-hosted Foundry world

**Status:** approved by the author on 2026-11-12, after answering §9's questions (eight defaults, plus a
deposit action for question 7). Phase 1 is built to §12's wire format; Phase 2's wire format is in §13
(2026-11-14). **Date:** 2026-11-12.
**Builds after:** the Market and Polls (merged). Phase 1's Forge check needs the M6 cutover, since a Forge
game can't reach the author's Mac. **Builds before:** nothing depends on it.
**Changes:** SPEC.md §2.2 (the `token` level gains a header-carried secret), §8.8 (a second kind of stream)
and Invariant 9's wording; §10 lists every edit. Everything else in SPEC.md and the multi-campaign design
stands.
**Audience:** the author (developer and DM) first, builders second.
**Research behind it:** a four-part web study done on 2026-11-12 (outbound, inbound, platform and policy,
data mapping). The key sources are in §11.

## 0. The proposed decisions on one page

| # | Proposal |
|---|---|
| B1 | **A small Foundry module plus new routes on Colony Planner.** No relay service, no headless browser, no direct socket.io client. The module runs in the active GM's browser and is the only Foundry-side code. |
| B2 | **The bridge works only while a GM has the world open.** The Forge runs no customer code on its servers, has no webhooks and no API that edits a running world, and its ToS §8 bans scripts that keep a game awake. Colony Planner queues changes and the module catches up when a GM connects. |
| B3 | **One bridge link per campaign**, made like Beny Bot's `/link`: the DM presses **Connect Foundry** on Settings, gets a one-time code that lasts 10 minutes, and types it into the module's settings in Foundry. The module trades the code for a bridge token. |
| B4 | **The bridge token is a bearer header, never a cookie or a URL.** Opaque random bytes, stored only as a SHA-256 hash, scoped to one campaign, revoked by the DM or by unlinking. In Foundry it lives in a `client`-scope setting (that browser's local storage), because world settings reach every player's browser. |
| B5 | **Bridge routes live under `/api/bridge/` at the `token` level** and never read the session cookie. The token picks the campaign, as the ICS token does. Core builds a `BridgeCaller` from it, which only bridge operations accept. |
| B6 | **Colony to Foundry is an outbox.** Core writes outbox rows in the same transaction as the change they describe, built from the **player projection**. The module reads them in order and acknowledges them, so nothing is lost while no GM is online or across a redeploy. |
| B7 | **The bridge stream stays invalidate-only**, like the site's: it tells the module "the outbox grew" and the module fetches the rows. The stream reads the database only on connect and when the broker publishes, so no timer polls it. |
| B8 | **Foundry to Colony never changes the books by itself.** Events from Foundry arrive as items in a DM inbox (for example "Foundry moved 3 days: advance the colony?"), and the DM confirms them with the existing sheets. The app still invents no game consequences. |
| B9 | **System-agnostic core, thin PF2e adapter.** Journals, chat messages, actors, items, scenes, notes and world time work in any system. The PF2e adapter adds coins and party inventory. Anything else falls back to journal and chat only. |
| B10 | **No DM-only content crosses**, in either direction, until §9's open check says Foundry keeps unobservable documents out of player browsers. The one exception is a paid special item's compendium UUID in its DELIVERY row (§13.6), sent once the item is the party's. |
| B11 | **Colony Planner serves the module itself**: `/foundry/module.json` and its zip, public, embedded in the binary, so the module's version always matches the server's. The Forge installs it with "Install From Manifest". |
| B12 | **Three phases** (§8): the plumbing with one kind each way, then Colony to Foundry content, then Foundry to Colony content. Stop for the author's check-in after each. |

What does **not** change: the game rules, `internal/game`, Undo, View as player, the secrecy model, the
single replica, Beny Bot, and the author makes every commit.

---

## 1. What the platform allows (the research, briefly)

- **All Foundry package code runs in the browser.** The Foundry server only relays a module's socket
  messages between connected clients. So every path in or out of a running world goes through a logged-in
  client, in practice the GM's tab.
- **The Forge adds nothing for live data.** Its API (an `Access-Key` header) covers assets, starting,
  stopping and idling games, and installing worlds. Nothing edits documents in a running world and nothing
  pushes events out.
- **The network path is open.** A header check of Forge's demo game on 2026-11-12 found no
  Content-Security-Policy, so a module can `fetch` and stream to colony.benthedev.com. The only gate is
  CORS, which Colony Planner answers. Forge custom domains only redirect, so the page origin is always
  `https://<subdomain>.forge-vtt.com`, which makes an exact allow-list possible. Only the `/join` page was
  checked, not the signed-in `/game` page (§9).
- **Forge games sleep** after an undocumented idle time (users report one to two hours). A sleeping world
  has no clients and therefore no bridge.
- **Forge ToS §8:** "You may not use scripts or other tools to simulate activity, if you are not actually
  using the server." An always-on headless GM is exactly that, so B2 accepts the gap instead.
- **Foundry v14 is stable** (14.370, 2026-11-02), but in April 2026 the Forge said it didn't yet recommend
  v14 there, and about two thirds of users were still on v13 in May. The module targets minimum 13,
  verified 14, ApplicationV2 and no jQuery.
- **Foundry's licence and ToS allow a module that calls an outside service.** A hand-built socket.io client
  sits in a grey zone under the no-reverse-engineering clause, which is one more reason for B1.
- **Content:** PF2e code is Apache 2.0 and its mechanics are OGL and ORC; Paizo's content is in Foundry
  under a Paizo and Foundry agreement that doesn't cover other websites. Colony Planner stores compendium
  UUIDs and numbers from Foundry, never art, icons or rules text.

The alternatives the research weighed, and why they lose for now:

| Option | Why not now |
|---|---|
| ThreeHats Foundry REST API with a self-hosted relay | A second Railway service, a generic payload shape, no PF2e adapter, one maintainer. It's the same GM-browser pattern underneath; its only extra is a headless GM, which B2 rules out on Forge. |
| Headless browser logged in as a bot GM | Forge's User Manager logs players in through forge-vtt.com accounts, so a bot needs its own Forge account and a scripted login; kept online, it breaks ToS §8. |
| Direct socket.io client from Go | An undocumented protocol that changed in v14, never shown through Forge's proxy, and it skips the client-side logic PF2e runs on every create and update. |
| socketlib, Discord as the bus, Assets Library snapshots, world export ZIPs | Client-to-client only, chat-text parsing, public-URL leaks, and one-off imports respectively. |

---

## 2. Concepts

| Term | What it is | Stored where |
|---|---|---|
| **Bridge link** | One campaign's connection to one Foundry world: the token's hash, the Forge origin, when it was made, the last contact day, revoked or not. At most one active per campaign. | `bridge_link` |
| **Pairing code** | A one-time code from Settings that the module trades for a token. 10 minutes, single use, one open per campaign. | `bridge_pairing_code` (hash only) |
| **Bridge token** | The secret the module sends as `Authorization: Bearer <token>` on every bridge request. | its hash in `bridge_link`; the token itself only in the GM's browser |
| **Outbox row** | One thing Foundry should show: a kind, a target key, a player-projection payload, a per-campaign sequence number, and whether it was applied. | `bridge_outbox` |
| **Inbound event** | One thing Foundry reports: a kind, a module-made ID (for de-duplication) and a small payload. | `bridge_event` |
| **DM inbox item** | An inbound event waiting for the DM's decision, shown on the colony board and Settings. | `bridge_event` (status PENDING) |
| **BridgeCaller** | Core's principal for a bridge request, built only by core from a live token. Not a Viewer and not a Caller. | in memory, per request |

Every table is campaign-owned and follows Invariant 11 (`campaign_id` first in every key and index,
composite foreign keys, every query takes `@campaign_id`). The one exception is the token lookup itself,
`-- unscoped: bridge token lookup` on a unique index of the token hash, exactly as the ICS feed finds its
campaign by token today.

---

## 3. Access and identity

### 3.1 Pairing

1. The DM presses **Connect Foundry** on `/c/{slug}/settings`:
   `POST /api/campaigns/{campaignId}/bridge/codes` (`dm`). The site shows a code like `K7Q-4XM` and how
   long it lasts. A new code replaces an unused one.
2. In Foundry, the GM opens the module's settings menu (GM-only), types the code and presses **Connect**.
   The module calls `POST /api/bridge/pair` with the code in the body.
3. Core checks the code (hash, unexpired, unused), records the request's `Origin` header as the link's
   origin (a page script can't forge `Origin`; it must match `https://<label>.forge-vtt.com`, or
   `http://localhost:30000` in devauth builds), revokes any earlier link of that campaign, and returns the
   token once. The site's Settings card shows "Connected to <subdomain>.forge-vtt.com".
4. The module stores the token in a `client`-scope setting. The GM's other browsers, and every player's,
   never have it. Connecting from a second browser means a new code, which replaces the link (B3).

Limits: 5 codes an hour per campaign, and a wrong code costs the same as a right one (constant-time
compare) and counts toward a per-IP limit.

### 3.2 The `token` level, widened

SPEC §2.2 defines `token` as "a secret in the URL". The bridge adds a second form: a secret in the
`Authorization` header, because Railway's HTTP logs record full paths (CLAUDE.md, Deployment), and a
bridge token allows writes, unlike the ICS token. Both stay outside `{campaignId}`, so Invariant 3's rule
holds as written.

For `/api/bridge/*`:

- The auth middleware **never reads the session cookie** there. A request carrying one is treated as if it
  didn't. This is what makes the CrossOriginProtection bypass below safe: no ambient credential exists on
  these routes.
- Core's `EnterBridge(token)` does one indexed read of `bridge_link` joined to an ACTIVE campaign and
  returns a `BridgeCaller`. A missing, revoked or malformed token, or a suspended campaign, is the same
  401 with no body detail.
- `last_contact_on` is written at most once a day, as `last_seen_at` is (Invariant 9).

### 3.3 Browser rules the Go side needs

- **CORS** only on `/api/bridge/*`. The preflight (`OPTIONS`, `Access-Control-Max-Age: 7200`) carries no
  token and the 401 says nothing, so both answer any origin allowed to pair (§3.1), which lets a revoked
  or replaced link's module read its 401 and ask the GM to reconnect. Any other real response echoes
  `Access-Control-Allow-Origin` only when the request's `Origin` equals **that token's** link origin, with `Vary: Origin`, `Allow-Headers: Authorization, Content-Type`, and no
  `Allow-Credentials`. Never `*.forge-vtt.com`: every Forge customer runs their own JavaScript on their own
  subdomain.
- **CrossOriginProtection** rejects cross-site POSTs, so `/api/bridge/` gets
  `AddInsecureBypassPattern`. Safe only because §3.2 ignores cookies there; a test asserts both.
- **JSON bodies only** still apply (`requireJSONBodies`). Bodies are capped at 256 KiB, batches at 100
  events.
- **Rate limits** in memory (single replica): 120 requests a minute per link, 30 a minute per IP for
  `/pair`. Over the limit is 429 with `Retry-After`.
- **Logs** carry the link ID, kinds, counts and statuses, never payloads (Invariant 14). Tokens are in
  headers, so the path redaction list needs no new entry.

---

## 4. Colony to Foundry: the outbox

### 4.1 Writing rows

When a campaign has an active link, core operations that Foundry should mirror insert outbox rows in their
own transaction, after the change and before commit, so a committed change always has its rows and a
rolled-back one never does. Invariant 2's write path gains one step ("enqueues bridge rows"), and the
operations outside it (Mark played, poll close, Place order) call the same helper.

Each row has:

- `seq`: per campaign, increasing.
- `kind`: an enum, for example `COLONY_SUMMARY`, `NPC_UPSERT`, `NPC_WITHDRAW`, `DELIVERY`, `POLL_POSTED`,
  `SESSION_UPSERT`.
- `target_key`: what it describes (`colony`, `npc:<id>`, `order:<id>`).
- `payload`: JSON built from the **player projection** (as Beny Bot's replies are), so the `dm` objects
  and hidden rows of Invariants 4 and 15 to 17 can't reach it by construction.

Two rules keep it small and safe to replay:

- **State, not deltas.** Most kinds carry the whole current state of their target ("the colony summary is
  now this"), so applying one twice, or out of a long gap, gives the same result. An unapplied row for a
  target is replaced by a newer one for the same target (one unapplied row per `target_key`). Undo then
  needs nothing special: it changes state and enqueues the new state.
- **One-shot kinds** (a delivery, a posted poll) carry an ID the module stores in a flag, so a repeat does
  nothing. Undoing one enqueues its counterpart (§9, question 6).

Applied rows older than 30 days are deleted by the campaign's next acknowledgement, never by a timer (the
same lazy pattern as unused uploads). At most 10,000 unapplied rows per campaign; past that, the oldest
one-shot rows go first, once per transaction after its rows are written, and the server logs how many.
State rows are never trimmed: each is its target's only waiting row, so a trimmed upsert would lose its
target until it next changed, and a trimmed withdraw would leave it visible. The newest 1,000 one-shot
rows always stay, so one-shot kinds keep the larger of 1,000 and 10,000 minus the waiting state rows, and
a new delivery or poll row is never trimmed in its own transaction. Live targets stay under 5,001
(§13.5); only withdraws of rows deleted while Foundry is away can pass that, and then the outbox may hold
more than 10,000 rows.

### 4.2 Reading rows

| Route | Level | Does |
|---|---|---|
| `GET /api/bridge/stream` | token | Invalidate-only SSE (B7): `event: outbox` with the newest `seq`, sent on connect when rows are waiting and after each broker publish of the campaign's new `bridge` topic. Heartbeat comment every 20 s; the server closes it at about 10 minutes like the site's stream (Railway's 15-minute proxy cap). |
| `GET /api/bridge/outbox?after={seq}` | token | Up to 100 unapplied rows after `seq`, oldest first. The cursor isn't secret, so a URL is fine. |
| `POST /api/bridge/acks` | token | `{"through": seq, "failed": [{"seq": n, "reason": "..."}]}`. Marks rows applied; a failed row is kept for the DM's status card with its reason, never retried in a loop. |

`EventSource` can't send an `Authorization` header, so the module reads the stream with `fetch` and a
stream reader. The stream holds no transaction and reads only on connect and on a publish (Invariant 9).
The `bridge` topic joins `AllTopics()`, and like every publish it carries no data (Invariant 13 holds).

### 4.3 What Foundry gets

| Colony Planner | Kind | Foundry, any system | PF2e adapter | Phase |
|---|---|---|---|---|
| Colony day, stockpile, workforce, next shipment | `COLONY_SUMMARY` | One journal page, "Colony", replaced in place | Same | 1 |
| Colony day advanced | `DAYS_ADVANCED` | A chat message; optional `game.time.advance` (§9, question 2) | Same | 1 |
| Revealed NPC, portrait, display name | `NPC_UPSERT` / `NPC_WITHDRAW` | A journal entry in a "Colony NPCs" folder (portrait as the page image) | Optionally a minimal `npc` actor | 2 |
| Revealed place and its map pin | `PLACE_UPSERT` | A journal entry; a Note on a scene the DM picks once | Same | 2 |
| Posted quest | `QUEST_UPSERT` | A journal entry in "Colony Quests" | Same | 2 |
| Paid shipment | `DELIVERY` | A chat message listing the goods | Physical items on the party actor, from compendium UUIDs the DM set on catalog items | 2 |
| Poll posted or closed | `POLL_POSTED` / `POLL_CLOSED` | A chat message with the question and, on close, the counts | Same | 2 |
| Next game night | `SESSION_UPSERT` | Part of the summary page | Same | 2 |
| A dismissed coin deposit (§5) | `DEPOSIT_REFUND` | Nothing without a coin adapter | The coins added back to the party actor | 3 |

The hex map isn't bridged: Foundry has no core hex state, and Paizo's paid Kingmaker module owns that in
PF2e. Withdraw kinds (an NPC hidden again) hide the Foundry document rather than delete it, so the GM keeps
anything they added there; §13.7 says what a withdraw blanks.

---

## 5. Foundry to Colony: inbound events

`POST /api/bridge/events` (token) takes a batch of events, each with a module-made UUID. Core records each
ID per campaign and ignores a repeat, because two GM browsers can both think they're the active GM for a
moment during reconnects. Seen IDs older than 30 days are pruned by the next batch.

Each event becomes one of two things:

- **Status**, which changes nothing: the world's Foundry and system versions, the module's version, and
  "connected" (sent on the module's `ready`). These feed the Settings card.
- **A DM inbox item** (status PENDING), which changes nothing until the DM acts on it with an existing
  sheet. Confirming runs that sheet's normal operation, with the colony lock, timeline, undo and publish
  of Invariant 2. Dismissing marks it DISMISSED.

| Foundry | Detected by | Becomes | Phase |
|---|---|---|---|
| World time moved forward N whole days | `updateWorldTime` (GM client only; the module keeps the last whole day it reported) | Inbox: "Foundry moved N days." **Advance** opens the Advance sheet pre-filled with N | 1 |
| GM deposits party coins into colony gold (a **Deposit to colony** button on the party sheet) | An explicit GM action | Inbox: "The party deposited 40 gp." **Accept** adds it to colony gold; **Dismiss** returns the coins | 3 |
| GM sends an NPC actor to Colony Planner (a button on the actor sheet) | An explicit GM action | Inbox: **Import as NPC** opens the NPC sheet pre-filled, hidden | 3 |
| GM sends a compendium item to the catalog | An explicit GM action | Inbox: **Add to Shipments** opens the item sheet pre-filled | 3 |

A deposit holds the coins in escrow so neither side can lose them. The module removes them from the party
actor first (`removeCoins`, which refuses when the party is short, so no event is sent), then sends the
event. **Accept** is an ordinary colony change: the colony lock, a STRICT gold grant capped at its max, a
timeline entry and undo. **Dismiss** enqueues a `DEPOSIT_REFUND` row that adds the coins back. Undoing an
accepted deposit takes the gold back out of the colony only, with a GM-only chat note, as for an undone
shipment (§9, question 6). Deposits are in whole gold pieces at 1 gp to 1 colony gold, and only where an
adapter can move coins (PF2e, or Item Piles once its API is checked).

Payloads from Foundry are untrusted: every player can run code in their own browser, and other modules can
read the GM's local storage. Core validates every field as it does a request body (lengths through
`cleanText`, IDs loaded with the link's campaign), and a bridge request can never do more than add inbox
items and status, which bounds what a leaked token can do.

The inbox is Phase 1's one new screen: a card on the colony board for the DM only (a `dm` object on the
board DTO, so view as player strips it) and a list on Settings.

---

## 6. The Foundry module

- **Where it lives:** `apps/colony-planner/foundry/`, its own Bun project and Biome root, like `web/`.
  `mise run build` builds it, and the Go binary embeds the zip and a `module.json` generated with the
  server's version (B11). Public routes `GET /foundry/module.json` and `GET /foundry/colony-bridge.zip`
  serve them; the module holds no secrets.
- **Shape:** one package ID (`colony-bridge`), `esmodules`, `compatibility.minimum` 13 and `verified` 14,
  no `maximum`. A GM-only settings menu on ApplicationV2 with native DOM. World settings hold nothing
  secret (whether the bridge is on, the scene for place pins). The token is `client` scope. The Colony
  Planner address is fixed at build time, so a script can't point the module at another server.
- **Who acts:** only the client where `game.users.activeGM?.id === game.user.id` holds the stream, sends
  events and applies rows. Players' clients load the module and do nothing.
- **Applying rows:** through the normal Document API (`JournalEntry.create`, `actor.update`,
  `ChatMessage.create`, `inventory.add`), so PF2e's own create and update logic runs. Every document the
  bridge makes carries `flags.colony-bridge.externalId`, which is how a repeat finds its target. Writes the
  bridge makes pass `{colonyBridge: true}` in their options, and the module's own hooks skip those, so a
  change never echoes back.
- **Adapters:** chosen at `ready` from `game.system.id`: `pf2e` (coins, party inventory, compendium items),
  else Item Piles if it's active (its currency API is unverified, §9), else journal and chat only.
- **Reconnects:** exponential backoff from 1 s to 60 s, a fresh `after` cursor each time, and a status
  event on every successful connect.
- **Tests:** unit tests against small fakes of the Foundry globals (no Foundry install in CI). Real Foundry
  and Forge behavior is the author's checklist (§8), as real Discord behavior is today.

---

## 7. Privacy and secrecy

- `/privacy` gains the Foundry bridge: the Forge address and the last contact day, the Foundry document
  IDs and names of what the GM sends (Phase 3), and that Colony Planner sends the colony's player-visible
  data to that Foundry world. Same change as the migration, per CLAUDE.md.
- **The secrecy sweep** gains bridge callers: campaign A's token against every bridge route must never
  return B's values, and B's token must never see A's. Every outbox payload is replayed against the
  canaries, as SSE messages and Beny Bot replies are, so a DM-only field that slips into a projection fails
  `mise run check`. A revoked token is swept like a removed member (401 everywhere).
- Hidden rows never get outbox rows; revealing one enqueues it, hiding it enqueues its withdraw kind.
- A withdraw leaves players nothing to read, even in their browser's data: it sets the entry's default
  and every player's own ownership to None, renames it "Hidden on Colony Planner" and blanks the bridge's
  page (§13.7). A failed withdraw is never acknowledged, so it is retried and later rows wait, and a link
  that ends hides every entry it made.
- Players can make chat messages and, with the permission, journal entries carrying the module's flags,
  so the module trusts a flagged chat message only when a GM posted it, and a flagged entry or folder only
  when its last editor was a GM and no player has a level of their own above None (§13.7).
- New `kind` and `status` enums go in `enumTypes` and the core re-exports, per the recipe.

---

## 8. Phases

Each phase ends with a checklist in `docs/checklists/` for the author, who runs the Foundry and Forge steps;
agents test against a local Foundry only if the author installs one, otherwise with fakes.

1. **Plumbing.** Migration (`bridge_link`, `bridge_pairing_code`, `bridge_outbox`, `bridge_event`), core
   (`EnterBridge`, pairing, outbox writes, ingest, inbox), the `/api/bridge/*` routes with CORS, the bypass
   and rate limits, the Settings card (connect, status, pending rows, failed rows, unlink), the board's
   inbox card, the module with the core adapter, `COLONY_SUMMARY`, `DAYS_ADVANCED`, and the time-advance
   inbox item. The sweep extended. Done means: the author connects the live site to their Forge world,
   advances a day on each side, and watches both show up, including after closing Foundry and reopening it.
2. **Colony to Foundry content.** NPCs, places and pins, quests, deliveries (with the PF2e adapter and the
   catalog's optional compendium UUID field), polls, sessions.
3. **Foundry to Colony content.** Coin deposits with their refunds, NPC import, catalog import.

---

## 9. Open questions for the author

All nine were answered by the author on 2026-11-12.

1. **Which direction matters most?** **Resolved:** both, phased as in §8 (both directions' plumbing, then
   Colony to Foundry content, then the reverse).
2. **Who owns time?** **Resolved:** the colony owns its days. Foundry's moves arrive as inbox items the DM
   confirms; pushing colony days into Foundry's clock is a per-link switch, off by default.
3. **DM-only content in Foundry** (hidden NPCs, DM notes) as GM-only documents? **Resolved:** never, until
   the check in §9.1 shows Foundry keeps a document a player can't observe out of that player's browser;
   then the author decides again.
4. **PF2e's kingdom model.** Mirror the stockpile into the party actor's kingdom commodities? **Resolved:**
   no; the colony summary journal page only.
5. **Where deliveries land.** **Resolved:** the party actor's inventory in PF2e, from the compendium items
   linked on the catalog; a chat message in other systems.
6. **Undoing a paid shipment** after its items reached Foundry. **Resolved:** a GM-only chat note naming
   what to remove; the module never deletes items the party may have moved.
7. **Party coins and colony gold.** **Resolved:** they stay separate pools, joined by a **Deposit to
   colony** action (§5): the GM moves coins out of the party actor, and the DM accepts them as colony gold
   on the site or dismisses them, which returns the coins. No automatic coin notice.
8. **Module hosting.** **Resolved:** Colony Planner serves the manifest and zip (B11).
9. **Other DMs.** **Resolved:** every campaign's DM can connect their own world; each link is per campaign
   and DM-only.

### 9.1 To check on the author's own world before Phase 1

- **Done 2026-11-12:** the signed-in `/game` page on the author's Forge server has no
  Content-Security-Policy header or meta tag, and a `no-cors` fetch to `colony.benthedev.com/healthz` from
  it reached the site.
- **Done 2026-11-12:** that server runs Foundry 13.351. The world checked is a dnd5e 5.2.5 practice
  world; the campaign moves to PF2e, which stays the target adapter. Phase 1 uses only the core adapter, so
  it can be proven in the 5e world.
- **Done 2026-11-12:** the author's PF2e world runs pf2e 7.12.2 on Foundry 13.351. `game.actors.party`
  returns the party actor, its `inventory.addCoins` is a function and `inventory.coins.copperValue` reads
  its coins. An `equipment-srd` item has `system.price` as `{value, per, sizeSensitive}`, with the coins in
  `value`, and `system.quantity` as a number. Item Piles isn't installed, which the PF2e adapter doesn't need.
- Whether a player's browser receives journal entries whose ownership is None for that player (decides
  question 3).
- Unverified, needed only for a non-PF2e world in Phase 2 or 3: Item Piles' currency API. PF2e's kingdom
  data path no longer matters, since question 4 keeps the stockpile out of it.

---

## 10. Document edits when approved

- **SPEC.md:** §2.2 (the `token` level's header form and the cookie-free bridge prefix), §3 (a Foundry
  bridge subsection pointing here), §8.8 (the bridge stream), §10 (the three phases), §11 (these questions).
- **CLAUDE.md:** Layout (`foundry/`, the bridge files), Invariants (a bridge invariant: player projection
  only, cookie-free prefix, outbox in the change's transaction, inbox never applies itself; Invariant 2's
  extra step), the CrossOriginProtection exception beside the image upload's.
- **HANDOFF.md:** status row and pending items.

---

## 11. Sources

- Foundry: [module development](https://foundryvtt.com/article/module-development/),
  [v13 Users (`activeGM`)](https://foundryvtt.com/api/v13/classes/foundry.documents.collections.Users.html),
  [v13 `updateWorldTime`](https://foundryvtt.com/api/v13/functions/hookEvents.updateWorldTime.html),
  [v14 ClientSettings](https://foundryvtt.com/api/v14/classes/foundry.helpers.ClientSettings.html),
  [14.370 release](https://foundryvtt.com/releases/14.370), [licence](https://foundryvtt.com/article/license/),
  [ToS](https://foundryvtt.com/article/terms-of-service/).
- The Forge: [ToS](https://forge-vtt.com/tos), [April 2026 update (v14)](https://blog.forge-vtt.com/april-2026-developer-update/),
  [custom API requests (2020, unofficial)](https://forums.forge-vtt.com/t/how-to-control-your-games-using-custom-api-requests-and-reset-your-worlds-periodically/5982),
  [domain redirects](https://forums.forge-vtt.com/t/how-can-i-redirect-my-domain-to-the-forge-can-i-point-my-dns-record-to-a-forge-game/16635),
  [Game Manager and User Manager](https://forums.forge-vtt.com/t/an-introduction-to-the-game-manager-user-manager/12357).
- Prior art: [ThreeHats Foundry REST API](https://github.com/ThreeHats/foundryvtt-rest-api) and
  [relay](https://github.com/ThreeHats/foundryvtt-rest-api-relay) (its
  [authentication notes](https://github.com/ThreeHats/foundryvtt-rest-api-relay/blob/main/docs/md/authentication.md)
  on what reaches player clients), [FoundryToDiscord](https://github.com/therealguy90/FoundryToDiscord).
- PF2e: [system repo](https://github.com/foundryvtt/pf2e), [party actor](https://github.com/foundryvtt/pf2e/blob/master/src/module/actor/party/document.ts),
  [inventory and coins](https://github.com/foundryvtt/pf2e/blob/master/src/module/actor/inventory/index.ts),
  [kingdom schema](https://github.com/foundryvtt/pf2e/blob/master/src/module/actor/party/kingdom/schema.ts).
- Web: [MDN CORS](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/CORS),
  [OWASP REST Security](https://cheatsheetseries.owasp.org/cheatsheets/REST_Security_Cheat_Sheet.html).

---

## 12. Phase 1 wire format

Fixed before the build so the server, the site and the module are built side by side. JSON keys are
camelCase, instants are RFC 3339 UTC, and every list is `[]`, never null.

### 12.1 Site routes (session cookie, campaign in the path)

| Route | Level | Body | Answer |
|---|---|---|---|
| `GET /api/campaigns/{campaignId}/bridge` | dm | | `BridgeStatus` below |
| `POST /api/campaigns/{campaignId}/bridge/codes` | dm | `{}` | 201 `{"code": "K7Q-4XM", "expiresAt": "..."}`; replaces an unused code |
| `DELETE /api/campaigns/{campaignId}/bridge` | dm | | 204; revokes the link and its unused code, keeps the rows for the record |
| `POST /api/campaigns/{campaignId}/bridge/failed/clear` | dm | `{}` | 204; deletes the rows Foundry couldn't apply (added 2026-11-14, Settings' **Clear**) |
| `POST /api/campaigns/{campaignId}/bridge/inbox/{eventId}/dismiss` | dm | `{}` | 204; PENDING to DISMISSED, else 409 |
| `POST /api/campaigns/{campaignId}/colony/advance` | dm | the existing body plus optional `bridgeEventId` | as today; with `bridgeEventId`, the same transaction marks that PENDING `TIME_ADVANCED` item ACCEPTED (409 when it isn't PENDING), and undoing that advance sets it PENDING again |

`BridgeStatus`: `{"link": null | {"origin", "linkedAt", "lastContactOn", "foundryVersion", "systemId",
"systemVersion", "moduleVersion", "worldTitle", "unappliedRows", "failedRows": [{"seq", "kind", "reason",
"failedAt"}]}, "code": null | {"expiresAt"}, "inbox": [InboxItem], "moduleManifestUrl"}`. The version
fields are `""` until the module's first status event.

`InboxItem`: `{"id", "kind": "TIME_ADVANCED", "receivedAt", "days"}`. The colony board's `dm` object gains
`bridgeInbox: [InboxItem]`, so View as player strips it with the rest.

### 12.2 Bridge routes (`Authorization: Bearer <token>`, no cookie, campaign from the token)

| Route | Level | Body | Answer |
|---|---|---|---|
| `POST /api/bridge/pair` | public, own per-IP limit | `{"code"}` | 200 `{"token", "campaignName"}`; a bad, used or expired code is 400 `"That code didn't work. Make a new one on Colony Planner's Settings."`; no `Origin` or one outside §3.1's pattern is 403 |
| `GET /api/bridge/stream` | token | | SSE: `event: outbox` with `data: {"seq": N}` (the newest unapplied seq) on connect when rows wait and after each `bridge` publish; `: ping` every 20 s; closes after about 10 minutes |
| `GET /api/bridge/outbox?after=N` | token | | `{"rows": [OutboxRow], "more": bool}`, at most 100 unapplied rows with `seq > N`, oldest first |
| `POST /api/bridge/acks` | token | `{"through": N, "failed": [{"seq", "reason"}]}` | 204; rows `<= through` not in `failed` are applied; `reason` is at most 300 characters |
| `POST /api/bridge/events` | token | `{"events": [InboundEvent]}`, 1 to 100 | `{"accepted": n, "duplicates": n}` |

A missing, malformed, revoked or unknown token, or one whose campaign isn't ACTIVE, is 401 with the body
`{"title": "Unauthorized"}`. `OutboxRow`: `{"seq", "kind", "targetKey", "payload"}`.
`InboundEvent`: `{"id": uuid, "kind", "occurredAt", "payload"}`.

### 12.3 Phase 1 kinds

Outbox (`bridge_outbox_kind`):

- `COLONY_SUMMARY`, target `colony`, state:
  `{"campaignName", "day", "resources": [{"name", "amount"}], "workforce": [{"role", "count"}],
  "nextShipmentDay": int | null, "nextGoodsShipDay": int | null, "siteUrl"}`, from the player projection.
  Enqueued on link and by every colony change, and its undo.
- `DAYS_ADVANCED`, one-shot, target `advance:<seq>`: `{"fromDay", "toDay", "days"}`. Enqueued by Advance
  only, never by its undo (the summary row carries the undo). The module posts a chat message, and moves
  `game.time` by `days * 86400` seconds only when its world setting "Move Foundry's clock with colony days"
  is on (off by default; question 2).

Inbound (`bridge_event_kind`):

- `STATUS`: `{"foundryVersion", "systemId", "systemVersion", "moduleVersion", "worldTitle"}`, each at most
  100 characters. Stored on the link; never an inbox item.
- `TIME_ADVANCED`: `{"days"}`, a whole number from 1 to 3650. Becomes a PENDING inbox item.

Unknown kinds are 422 for the whole batch. Inbox status (`bridge_event_status`): `PENDING`, `ACCEPTED`,
`DISMISSED`; `STATUS` events are stored as `ACCEPTED` only for de-duplication.

### 12.4 Serving the module

`GET /foundry/module.json` and `GET /foundry/colony-bridge.zip` (public). Go embeds `foundry/dist/` the
way `web/embed.go` embeds the SPA, and fills the manifest's `version` with the server's build version and
its `manifest` and `download` with PUBLIC_URL's. The module finds Colony Planner from its own installed
manifest's origin (`game.modules.get("colony-bridge").manifest`), so nothing is baked in at build time.

---

## 13. Phase 2 wire format

Fixed on 2026-11-14 so the server and the module are built side by side, under §12's conventions (camelCase
keys, RFC 3339 UTC instants, lists `[]` and never null). It replaces §4.3's Phase 2 rows with the author's
2026-11-14 decisions: text only (no portraits, no map art); places are journal entries only (no scene Notes,
so no scene world setting); in PF2e an NPC stays a journal entry (no actor); polls post a chat message when
they open and one with the counts when they close. `SESSION_UPSERT` is dropped: the next game night rides in
the summary (§13.3). Nothing DM-only or hidden crosses (B10, Invariant 21).

### 13.1 Kinds and target keys

| Kind | Target key | Shape | Payload |
|---|---|---|---|
| `NPC_UPSERT` | `npc:<npcId>` | state | `NpcPayload` |
| `NPC_WITHDRAW` | `npc:<npcId>` | state | `{}` |
| `PLACE_UPSERT` | `place:<placeId>` | state | `PlacePayload` |
| `PLACE_WITHDRAW` | `place:<placeId>` | state | `{}` |
| `QUEST_UPSERT` | `quest:<questId>` | state | `QuestPayload` |
| `QUEST_WITHDRAW` | `quest:<questId>` | state | `{}` |
| `DELIVERY` | `delivery:<paidGroupId>` | one-shot | `DeliveryPayload` |
| `DELIVERY_UNDONE` | `delivery-undone:<paidGroupId>` | one-shot | `DeliveryUndonePayload` |
| `POLL_POSTED` | `poll:<pollId>:posted` | one-shot | `PollPostedPayload` |
| `POLL_CLOSED` | `poll:<pollId>:closed` | one-shot | `PollClosedPayload` |

- An upsert and its withdraw share one key, so each replaces the other while unapplied (one unapplied row
  per key, §4.1): hiding then revealing before Foundry reads leaves only the upsert.
- `<paidGroupId>` is the payment's timeline group (`market_order.paid_group_id`, the undo slot's
  `groupId`), not the order's ID: an order paid, undone and paid again is two deliveries, and an
  `order:<id>` key would make the second one find the first one's flag and do nothing.
- Every ID in a key is one players could read when the row was written. A withdraw's key names a row that
  is now hidden or deleted and that Foundry already had; its `{}` payload doesn't say which, as a 404
  doesn't.

### 13.2 Payloads

Each is built in the change's transaction from the player projection (`readNpc`, `readPlace` and
`readQuest` with `dmProjection` false, `loadPolls`, the board's reads), never from a DM read with fields
dropped. A projection read that finds nothing (the row is hidden) is what makes an operation write a
withdraw or nothing. `siteUrl` is PUBLIC_URL plus the row's page: `/c/<slug>/npcs/<id>`, `/places/<id>`,
`/quests/<id>`.

`NpcPayload`, from the player projection's `Npc`:

| Field | Type | From |
|---|---|---|
| `displayName` | string | `displayName`; never `dm.name` or `dm.knownName` |
| `aliases`, `labels` | string[] | same |
| `race`, `characterClass`, `age` | string | same |
| `level` | int or null | same |
| `appearance`, `personality`, `backstory`, `playerNotes` | string, may hold line breaks | same |
| `status` | `ALIVE`, `DEAD`, `MISSING` or `UNKNOWN` | same |
| `health` | `HEALTHY`, `INJURED`, `SICK`, `UNKNOWN` or `DEAD` | same |
| `relation` | `ALLY`, `NEUTRAL`, `SUSPICIOUS`, `ENEMY` or `UNKNOWN` | same |
| `currentPlace`, `originPlace` | string or null | the place reference's `name`; null when unset or hidden (the projection's join) |
| `colonyRole` | string or null | `colonyRole.label`; null when not a colonist |
| `siteUrl` | string | |

Left out: the portrait (text only), every place, role and session ID, and `firstSession` (a session title
edit would then re-upsert NPCs; it can come later as an additive field).

`PlacePayload`: `{"name", "description", "playerNotes", "siteUrl"}`, all strings. Left out: `pin` (no
scene Notes, and a pin names a map) and `npcsHere` (each NPC's entry names its place; listing them here
would re-upsert a place on every NPC change).

`QuestPayload`, from the player projection's `Quest`:

| Field | Type | From |
|---|---|---|
| `title`, `description` | string | same |
| `kind` | `MAINLAND` or `COLONY` | same |
| `status` | `ACTIVE`, `COMPLETED` or `FAILED` | same |
| `giver` | string or null | `giverNpc.displayName`; null when unset or hidden |
| `patron` | string or null | `patron.name`; null when unset or hidden |
| `reward` | string | `rewardEffects` rendered by `appliedEffectsText` in board order; `""` when none |
| `rewardFavor` | int | same; 0 while `patron` is null, as the projection gives it |
| `loot` | `[{"name": string, "quantity": int, "description": string}]` | same |
| `resolvedDay` | int or null | same |
| `siteUrl` | string | |

Never: `dm.dmNotes`, `dm.failureEffects` or `dm.failureFavor`, not even after a failure (players read the
cost only on the QUEST_FAILED entry, Invariant 16), nor `revealed` or a patron's favor total.

`DeliveryPayload`: `{"paidDay": int, "totalGold": int, "summary": string, "items": [{"name": string,
"quantity": int, "compendiumUuid": string | null}]}`.

- `summary` is `orderSummaryText` over every line ("20 food, 3 Farmers, Boots of Striding").
- `items` are the order's special-item lines only (no resources, no colonists), in line order, since
  supplies and colonists land in the colony's books, not in Foundry. `name` and `quantity` are the line's
  snapshot; `compendiumUuid` is the line's item's current `compendium_uuid`, read at Pay, so a UUID the DM
  sets while the ship sails still counts.

`DeliveryUndonePayload`: `{"summary": string, "items": [{"name": string, "quantity": int}]}`, the same
order's summary and special-item lines.

`PollPostedPayload`: `{"question": string, "options": [string], "allowMultiple": bool, "voting": "SITE" |
"DISCORD", "closesAt": instant, "pollsUrl": string}`. Options in position order; `pollsUrl` is PUBLIC_URL
plus `/c/<slug>/polls`. No `discordUrl`: Beny Bot posts after commit, so it is still null at create.

`PollClosedPayload`: `{"question": string, "closedAt": instant, "options": [{"label": string, "count":
int, "otherVotes": int}]}`, in position order. `closedAt` is the recorded `closed_at` (the deadline for an
overdue close). `count` is what `closePoll` writes into the POLL_CLOSED entry (`CountPollVotesByOption`,
other votes included) and `otherVotes` the option's `discord_other_votes`. No winner, no voter names.

### 13.3 The next game night in COLONY_SUMMARY

`COLONY_SUMMARY` gains `"nextSession": null | {"title": string, "startsAt": instant, "endsAt": instant,
"localDate": "YYYY-MM-DD", "timezone": string}`: what `NextSession` returns when the row is written (the
next SCHEDULED date not yet started, within two years), with the board's `timezone`. Null when none.

- It is a snapshot. Nothing rewrites it when the start time passes; Mark played, End session and Discord's
  stamps re-enqueue the summary, which moves it on to the following date.
- A 0.1.x module ignores it: `isColonySummaryPayload` checks only §12's fields, so the page renders as
  before, without a game night line. No other summary field changes.

### 13.4 What enqueues what

Every row below is written only while the campaign has an active link (checked once per transaction, so an
unlinked campaign makes no projection reads), in the operation's own transaction, and never as an upsert
of a row that is hidden after the change. "Its quests" means the revealed quests whose giver is that NPC
or whose patron is that patron; "its NPCs" means the revealed NPCs whose current or origin place it is.

| Operation | Rows |
|---|---|
| `CreateNpc`, `UpdateNpc` | NPC_UPSERT when revealed. An `UpdateNpc` that changes the display name also writes QUEST_UPSERT for its quests: a new known name makes the old display name, the true name, a secret (Invariant 15) |
| `SetNpcRevealed` | to revealed NPC_UPSERT, to hidden NPC_WITHDRAW; either way QUEST_UPSERT for its quests. No change, nothing |
| `DeleteNpc` | a revealed NPC: NPC_WITHDRAW, then QUEST_UPSERT for the quests it gave (the foreign key nulls the giver). A hidden one: nothing |
| `JoinColony`, `ChangeColonyRole`, `LeaveColony` | NPC_UPSERT when revealed, beside the summary |
| `CreatePlace`, `UpdatePlace` | PLACE_UPSERT when revealed. An `UpdatePlace` that changes the name also writes NPC_UPSERT for its NPCs |
| `SetPlaceRevealed` | to revealed PLACE_UPSERT, to hidden PLACE_WITHDRAW; either way NPC_UPSERT for its NPCs. No change, nothing |
| `DeletePlace` | a revealed place: PLACE_WITHDRAW, then NPC_UPSERT for its NPCs (the foreign key nulls their place). It gains a transaction that reads the place first |
| `CreateQuest`, `UpdateQuest` | QUEST_UPSERT when revealed |
| `SetQuestRevealed` | to revealed QUEST_UPSERT, to hidden QUEST_WITHDRAW. No change, nothing |
| `DeleteQuest` | QUEST_WITHDRAW when it was revealed |
| `CompleteQuest`, `FailQuest`, `ReopenQuest` | QUEST_UPSERT when revealed |
| `SetPatronRevealed` (a change), `UpdatePatron` (a revealed patron's name changes) | QUEST_UPSERT for its quests |
| `UndoLastChange` | NPC_UPSERT for each revealed NPC in `before.npcColonyRoles`; QUEST_UPSERT for each revealed quest in `before.quests`; for an undone Pay (`before.marketOrders`), it deletes the unapplied DELIVERY of `delivery:<groupId>` when one waits (so a module that hasn't fetched it never delivers it) and always writes DELIVERY_UNDONE: unapplied means unacknowledged, and a module that already fetched the DELIVERY still applies it |
| `PayMarketOrder` | DELIVERY, beside the summary |
| `CreatePoll` | POLL_POSTED (every poll is player-visible, Invariant 20) |
| `closePoll` (the DM's Close, the deadline timer, the lazy close) | POLL_CLOSED, only when this call recorded the close |
| `DeletePoll` | no new row; it deletes the poll's unapplied POLL_POSTED and POLL_CLOSED when they wait, so Foundry never posts a deleted poll or its close. It gains a transaction |
| every session write that calls `afterSessionWrite` (series create, edit and delete; cancel and restore a date; Mark played; End session; days, recap and title; `RecordDiscordEventStatus`) | COLONY_SUMMARY. A single-statement write gains a transaction |
| `PairBridge` | COLONY_SUMMARY, then the backfill (§13.5) |

Nothing beyond Phase 1's summary rows: portraits, first sessions, pins, maps, patron archiving and favor,
requests, votes, Discord vote syncs, `RecordPollMessage`, catalog edits (the UUID is read at Pay), and
resource or role renames (an NPC's `colonyRole` and a quest's `reward` keep the labels of their last
upsert).

### 13.5 Backfill on pairing

`PairBridge`, after the summary and in the same transaction, writes PLACE_UPSERT for every revealed place,
then NPC_UPSERT for every revealed NPC, then QUEST_UPSERT for every revealed quest, each in its list's
order, so an existing campaign shows up whole. Before the summary it deletes every unapplied, unfailed
COLONY_SUMMARY, NPC, place and quest row an earlier link left waiting (one-shot rows stay), since an old
upsert of a row hidden while unlinked would otherwise show it again after Connect's withdrawal. NPCs,
places and quests have no archive (only patrons and market items do), so revealed is the whole filter.
The rows are written in three statements whatever their number: one `UPDATE ... RETURNING` reserves
their consecutive seqs, one `DELETE` takes back the waiting rows of their keys, and one `INSERT ...
SELECT unnest(...)` writes them in order. Every operation's rows are written the same way at the end of
its transaction, and its cascades read their NPCs and quests with one list query each.

- The cap: live state targets are at most 1 + 2,000 NPCs + 1,000 places + 2,000 quests = 5,001
  (`NpcLimit`, `PlaceLimit`, `QuestLimit`), so a backfill always fits under 10,000 and leaves at least
  4,999 rows for one-shot kinds. Withdraws of rows deleted while Foundry is away also wait as state rows
  and can pass that; one-shot kinds then still keep their newest 1,000 (§4.1). Open question 1 is the
  trim rule this depends on.
- A full backfill is about 51 pages and as many acks, close to the 120 a minute limit. A 429 ends that sync;
  the next stream event or reconnect resumes from the cursor.
- Polls already open when the link is made aren't posted; their close still posts POLL_CLOSED.
- On a successful **Connect**, before its first sync, the module withdraws (as NPC_WITHDRAW does) every
  journal entry it flagged `npc:`, `place:` or `quest:`. The backfill shows the revealed ones again, so a
  row hidden or deleted while unlinked, or under an earlier link, leaves players' view.

### 13.6 The catalog's Foundry item

- Migration 0009 adds `market_item.compendium_uuid text` (nullable) with CHECKs: `compendium_uuid IS NULL
  OR (resource_key IS NULL AND role_key IS NULL)`, `length(compendium_uuid) <= 200`, and the pattern below.
- Pattern: `^Compendium\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.Item\.[A-Za-z0-9]{16}$`, Foundry v13's full
  form as **Copy UUID** gives it. Core trims it, turns `""` into null, and checks the pattern and length
  itself (field `compendiumUuid`, message "Paste the item's UUID from Foundry, like
  Compendium.pf2e.equipment-srd.Item.abcdefghij123456.").
- Only special items carry one. Supply offers (a resource) and colonist offers (a role) can't: their
  endpoints don't take it and the CHECK refuses it.
- `MarketItemInput` (the DM's POST and PUT of a special item) gains `compendiumUuid: string | null`
  (`required:"false"`, nullable). It is a full-record PUT, so omitted is null, and the item sheet sends it
  on every save. `MarketItem` gains `compendiumUuid: string | null` in its `dm` object (`MarketItemDm`),
  so only the DM's projection reads it (Invariant 19): a disguised item's UUID would reveal what it really
  is. The secrecy fixture plants canaryA and canaryBdm in it. It isn't personal data, so `/privacy` only
  notes that the DM-only UUIDs go to Foundry.
- The web item sheet gains one optional field, "Foundry item UUID". DELIVERY lines carry it (§13.2), read
  from the database at Pay: the one DM-only value an outbox row carries, read only by the active GM's
  module, which adds the item it names and never shows the UUID.

### 13.7 The module

Version 0.2.0 in `foundry/package.json`. `wire.ts` gains a type and guard per payload; a payload that fails
its guard throws, so the row fails with its reason. As in §12, every Foundry write gets its own fresh
`bridgeWriteOptions()` object, never one shared between calls. `foundry-types.ts` gains the shapes used
below: folders, `game.folders`, `game.users` as a list, `game.actors.party`, an actor's `items` and
`createEmbeddedDocuments("Item", ...)`, `fromUuid`, a chat message's `whisper` and `author`, and an entry's
or folder's `ownership` and `_stats.lastModifiedBy`.

Core adapter (every system):

- **Trust.** Players can create chat messages, and journal entries when the world allows it, carrying any
  flag, and an owner can change an entry's flags. So every lookup by `flags.colony-bridge.externalId`
  below counts a chat message only when `message.author?.isGM` (v13's `author`, a User; the server lets
  only a GM post as someone else), and a journal entry or folder only when its `_stats.lastModifiedBy`
  (server-set) is a GM's, or unset, and no non-GM user has an `ownership` level of their own above None.
  v13 records no creator, so the last editor stands in for one. Anything else is ignored, so a planted
  document blocks nothing: the bridge makes its own. Withdraws are the exception (below).
- **Folders.** "Colony NPCs", "Colony Places" and "Colony Quests" are `JournalEntry` folders the module
  creates when missing and finds by `flags.colony-bridge.externalId` (`folder:npcs`, `folder:places`,
  `folder:quests`), so the GM may rename or move them.
- **Upserts** find their entry by `externalId` equal to the target key. Missing: create it in its folder
  with `ownership.default` 2 (Observer) and one text page flagged with the same key. Found: update the
  entry's name and the bridge's own page, leaving its folder, ownership and other pages as the GM set
  them; when the entry has `flags.colony-bridge.withdrawn` true, also set `ownership.default` 2 and the
  flag false. The name is `displayName`, `name` or `title`.
- **Withdraws** apply to every entry flagged with the key, trusted or not (hiding can't leak): each gets
  `ownership.default` 0 (None), every non-GM user's own level 0, the name "Hidden on Colony Planner" and
  `withdrawn` true, and then the bridge's own page is blanked; they succeed when no entry exists. They
  never delete, and keep the GM's levels and the pages the GM added. The next upsert restores the name and
  the page (and `ownership.default` 2), not the players' own levels. A withdraw applier that throws is
  never acknowledged: the sync acknowledges the rows before it and stops, so the row is retried after the
  reconnect backoff and every later row waits, and the GM gets one notification per failing run.
- **Connect and unlink.** On a successful Connect (before its first sync) and when the module learns its
  link was revoked (a 401), it withdraws every entry flagged `npc:`, `place:` or `quest:` in one
  `JournalEntry.updateDocuments`, then blanks each bridge page that still has text. The 401 path runs once,
  after any sync still in flight and before the token is forgotten, best effort: a failure is a GM
  notification and the token is forgotten anyway (the next Connect withdraws again). It makes no request,
  so it can't meet another 401.
- **Rendering:** every payload string escaped, line breaks as `<br>`, blank strings, empty lists and
  `UNKNOWN` values skipped, and a link to `siteUrl` when it is a web address, as the summary does. The
  summary renders `nextSession` when it isn't null.
- **Chat kinds** post one message as `speaker.alias` "Colony Planner", flagged with the target key, and
  post nothing when a GM's message with that flag exists (the DAYS_ADVANCED pattern).
  - DELIVERY: "Goods unloaded on Day N, paid G gold: <summary>", then the items to hand out.
  - DELIVERY_UNDONE: whispered to the GMs (`whisper` is the IDs of the users with `isGM`): the payment was
    undone, and any of these items that reached the party should be removed by hand. It names only
    player-visible things; the whisper keeps the GMs' cleanup out of players' chat. It posts only when
    this world holds the delivery: a GM's chat message flagged `delivery:<paidGroupId>`, the paid group read
    from its own target key. Otherwise (the DELIVERY was taken back before this module fetched it, or
    failed before posting) the row succeeds with no message.
  - POLL_POSTED: the question, the options, "Pick one" or "Pick any", the close time, and "Vote on Colony
    Planner" linking `pollsUrl` (SITE) or "Vote in Discord" (DISCORD).
  - POLL_CLOSED: "Poll closed: <question>", then each option and its count in order, naming no winner.

PF2e adapter (`systemAdapters.pf2e`: the core appliers, with its own DELIVERY). The items go first and the
message last, so chat never names an item that didn't arrive:

1. When a GM's chat message flagged with the target key exists, stop: the row is done.
2. When `game.actors.party` exists, for each line the party doesn't hold yet (an item flagged
   `externalId` `<targetKey>:<index>`), resolve its `compendiumUuid` (the §13.6 pattern only) by
   `fromUuid`; a line is addable when the result is an Item whose `system.quantity` is a number. Each
   addable line's compendium item's `toObject()` source without `_id` (its folder, sort and `_stats`
   kept, and `_stats.compendiumSource` not set, a possible follow-up), with `system.quantity` set to the
   line's quantity and that flag, is created by one `party.createEmbeddedDocuments("Item", sources,
   bridgeWriteOptions())`.
3. Post the message: the lines with a compendium UUID that the party now holds under "Added to <party
   name>'s inventory", every other line (no UUID, nothing resolved, not a physical item, no party actor,
   or the create failed) under "Hand these out yourself". The message carries only the target key.
4. When step 2's create threw, the message still posts (step 3), and then the row fails with Foundry's
   reason. A replay before the message posted (a lost acknowledgement, or a failed post) adds only the
   lines still missing, so no item is added twice; once the message exists nothing more happens.

The module never deletes an item. DELIVERY_UNDONE in PF2e is the core adapter's whisper, which also posts
when the delivery's message is missing but the party holds items flagged `<targetKey>:<index>` for it
(step 4's failed post), and then names only those lines.

### 13.8 Enums and migration

- `bridge_outbox_kind` gains `NPC_UPSERT`, `NPC_WITHDRAW`, `PLACE_UPSERT`, `PLACE_WITHDRAW`,
  `QUEST_UPSERT`, `QUEST_WITHDRAW`, `DELIVERY`, `DELIVERY_UNDONE`, `POLL_POSTED` and `POLL_CLOSED`, in
  migration 0009 with `ALTER TYPE ... ADD VALUE` (no new value is used in that migration). Each gets a
  constant in `internal/core/bridge_enums.go` and a name in `allowedDataReferences`; the type is already
  in `enumTypes`. Phase 2 adds no inbound kinds, so `bridge_event_kind` and `bridge_event_status` stay.
- The down migration drops the column, deletes rows of the new kinds and rebuilds the enum without them.

### 13.9 The secrecy sweep

- `TestBridgeCallersSeeOnlyTheirCampaign` reads the whole outbox (following `more`), checks every row's
  target key as well as its payload against the forbidden values, and requires NPC_UPSERT, PLACE_UPSERT
  and QUEST_UPSERT rows. The bridge fixture pairs after every other plant, so this replays the backfill:
  no row may name a `hiddenIDs` value, the hidden place (the revealed NPC whose current place it is gets
  `currentPlace` null), the hidden NPC or patron on a quest, the true name of an NPC with a known name,
  a DM note or the revealed quest's failure cost.
- New plants: a special item in each campaign, its compendium UUID's package segment canaryA in A and
  canaryBdm in B (the check lets only DELIVERY carry the campaign's own), and a docked order of it.
- After planting, on each side, the test (as it already advances) pays the order (DELIVERY with the
  UUID), acknowledges it and undoes the payment (DELIVERY_UNDONE); creates and closes a poll (B's question
  and options carry canaryB); marks a session played (`nextSession`, B's title canaryB); sets a known name
  on the revealed NPC who gives the revealed quest (its NPC_UPSERT and that QUEST_UPSERT carry the known
  name, never the true one); and hides, then deletes, a place and an NPC it made revealed (withdraws with
  `{}`).
- A `hiddenIDs` value may appear only as the target key of a withdraw of its own kind with payload `{}`.
  The fixture's hidden rows are never revealed while linked, so today none does.
- In its own test, with a link: creating, editing, renaming, hiding again and deleting a hidden NPC, place
  and quest, renaming a hidden patron, and a hidden NPC joining the colony write no row but the summaries a
  colony mutation already writes.

### 13.10 Open questions

The author accepted all three proposals on 2026-11-14. The first two are built as proposed; the third was
changed by that day's security review (below).

1. **Trim.** §4.1 says that past 10,000 unapplied rows "the oldest state rows go first, since a newer row
   of every live target is still queued", and `TrimBridgeOutbox` drops the oldest unapplied rows of any
   kind. With one unapplied row per target there is no newer row, so a trimmed upsert loses that NPC,
   place or quest until it next changes, and after a backfill those are the oldest rows. Proposed: trim
   only one-shot rows, oldest first; state rows can't pass 5,001 (§13.5).
2. **Updating the module.** A 0.1.x module fails every Phase 2 row ("doesn't know NPC_UPSERT rows"), a
   failed row is never offered again, and the Forge updates the module only when the GM does. Proposed:
   no server gating; Phase 2's checklist has the author update the module in Foundry, press **Clear** on
   Settings and **Connect Foundry** again, whose backfill restores everything. The alternative holds
   Phase 2 rows until the link's `moduleVersion` is 0.2.0 or later and backfills on the first such STATUS.
3. **What a withdraw leaves behind.** Decided: a withdraw blanks. The author accepted keeping the name and
   page until §9.1's check, but the 2026-11-14 security review showed that isn't safe: a GM's explicit
   per-player levels survived a default of None, and if players' browsers receive None-ownership
   documents, a withdrawn entry would keep showing its last name and page, which may be an NPC's true
   name that is now a secret. So a withdraw also sets every non-GM user's own level to None, renames the
   entry "Hidden on Colony Planner" and blanks the bridge's page, whatever §9.1 finds (§13.7). The GM's own
   pages stay, and the next upsert restores the name and page.
