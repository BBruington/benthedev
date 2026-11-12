# Foundry bridge: Colony Planner and a Forge-hosted Foundry world

**Status:** approved by the author on 2026-11-12, after answering §9's questions (eight defaults, plus a
deposit action for question 7). Phase 1 is being built to §12's wire format. **Date:** 2026-11-12.
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
| B10 | **No DM-only content crosses**, in either direction, until §9's open check says Foundry keeps unobservable documents out of player browsers. |
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

- **CORS** only on `/api/bridge/*`. The preflight (`OPTIONS`) carries no token, so it answers for any
  origin that belongs to an active link (one indexed read; `Access-Control-Max-Age: 7200`). The real
  response echoes `Access-Control-Allow-Origin` only when the request's `Origin` equals **that token's**
  link origin, with `Vary: Origin`, `Allow-Headers: Authorization, Content-Type`, and no
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
state rows go first, since a newer row of every live target is still queued.

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
PF2e. Withdraw kinds (an NPC hidden again) set the Foundry document's default ownership to None rather than
deleting it, so the GM keeps anything they added there.

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
