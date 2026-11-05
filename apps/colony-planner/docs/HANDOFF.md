# Handoff: Colony Planner

**Last updated:** 2026-11-04, after Polls were built. M0–M5 and M6's deployment build are merged (M6 in [PR #8](https://github.com/BBruington/colony-planner/pull/8)); the cutover itself is the author's to run with `docs/cutover.md`. The Market and Polls await the author's check-in.
**Where it lives:** the benthedev monorepo (`~/myCodeSpace/benthedev`, GitHub `BBruington/benthedev`, private), in `apps/colony-planner`, with its full history. The Go module path stays `github.com/BBruington/colony-planner`. The old `BBruington/colony-planner` repository and its checkout at `~/myCodeSpace/colony-planner` are no longer worked in; the author archives the repository (read-only on GitHub) once the monorepo is pushed. Its PR links below still work.
**For:** the author, and any future session or AI agent picking this project up cold.

This document carries the context that isn't in the code: why the project exists, what the author decided and why, how the author likes to work, what's pending, and what bit us along the way. Read it with:

1. [`CLAUDE.md`](../CLAUDE.md): commands, layout, boundaries, invariants, recipes. Agents load it automatically.
2. [`docs/SPEC.md`](SPEC.md): the product and technical spec. §11 lists the open questions and their answers.
3. [`docs/design/multi-campaign.md`](design/multi-campaign.md): the M1.5 design. Where it and SPEC.md differ, this one wins.
4. [`docs/checklists/`](checklists/): one checklist per milestone, written for the author to run on their Mac.

---

## 1. The project in one paragraph

Colony Planner is a website plus a Discord bot (**Beny Bot**) for running Dungeons & Dragons **colony campaigns**. The author runs one; other DMs the author approves can run their own on the same site. Each campaign's colony runs on a day clock with a data-defined stockpile (gold, food, materials, morale and anything the DM adds), a workforce of roles plus Unassigned, shipments from the mainland, an undoable timeline, and, in later milestones, NPCs, quests, patrons with favor, a player-drawn hex map and session scheduling. The DM keeps the books and decides everything; the app never invents game consequences.

It replaces `~/myCodeSpace/party-planner`: the author's earlier, unfinished, generic DM tool with an Nx monorepo, Clerk, an oRPC → ConnectRPC double hop, Drizzle plus hand-written SQL, and a forgeable RSA cookie. That repo is **reference only**; never modify it.

---

## 2. Where things stand

| Milestone | What | Status |
|---|---|---|
| M0 | Skeleton, direct Discord OAuth2 sign-in, server sessions, DM approval of players, app shell, Beny Bot core | On `main`, committed by the author (`47e58c1`) |
| M1 | Colony clock and books: Advance N days with preview, shipments (exact cadence), Adjust, Record cargo, Correct day, single-level Undo, timeline, live updates; plus the M0 review hardening | Merged ([PR #1](https://github.com/BBruington/colony-planner/pull/1)) |
| M1.5 | Multi-campaign: accounts and memberships, site owner, invite links, DM requests and owner approval, one shared Beny Bot with `/link`, cross-campaign isolation; then a cleanup pass that squashed migrations into one `0001` | Merged ([PR #2](https://github.com/BBruington/colony-planner/pull/2)) |
| M2 | People and secrecy: `/people` (transfers, set counts, named colonists joining and leaving), NPC roster with server-side secrecy, places (unpinned), Settings roles card, Beny Bot `/npc view` | Merged ([PR #3](https://github.com/BBruington/colony-planner/pull/3)) |
| M3 | Quests (Effects rewards, favor to patron givers, an optional failure cost), patrons and actions, player requests with the DM's approval sheet, Beny Bot Grant/Decline buttons | Merged ([PR #4](https://github.com/BBruington/colony-planner/pull/4)) |
| M4 | World maps: the Explore hex map (players chart the new world; no fog of war, no history), each DM's uploaded Mainland and other maps with pins, image uploads, NPC portraits, the PWA manifest | Merged ([PR #5](https://github.com/BBruington/colony-planner/pull/5)) |
| M5 | Sessions: series in the table's time zone, cancel and restore, Mark played and End session with day stamps and timeline bands, recaps, the ICS feed, the NPC's first session; Beny Bot's rolling Discord events, reminders, cancellation notices, `/colony` and `/nextsession`; the legacy guild-command cleanup removed | Merged ([PR #6](https://github.com/BBruington/colony-planner/pull/6), [PR #7](https://github.com/BBruington/colony-planner/pull/7)) |
| M6 | Deploy at colony.benthedev.com, replacing party-planner | Deployment merged ([PR #8](https://github.com/BBruington/colony-planner/pull/8)): the Dockerfile, `railway.toml`, Go 1.27.2, the production host check, the privacy page (`/privacy`), the runbook (`docs/cutover.md`) and `docs/checklists/M6.md`. Then moved into the benthedev monorepo, with the runbook rewritten for the new subdomain. **The cutover is pending the author's runbook steps** (Railway, Neon, Cloudflare and Discord are the author's alone) |
| Market | The player store (SPEC §3.3, Open Question 18 resolved 2026-11-03): `/c/{slug}/shipments` ("Shipments" in the nav; the code and API keep the name market) with the DM's catalog, one shared party cart, up to the DM's ship limit of open orders (default 1) that each dock after the campaign's voyage days, payment in colony gold at the docks (STRICT, undoable by the DM), bought colonists into their role, four PUBLIC timeline kinds, the board's goods-ship line and the Advance preview's docking line. Reworked after the author's first look (2026-11-04): gold and morale refused as goods, a slimmer cart line, and Adjust stock without a Reason field. Reworked again (2026-11-04): supplies and colonists are standing offers (one per resource, one per role at 30 gp, a new role included) that players buy in bulk by typing units, and "New special item" handles special items only. Reworked a third time (2026-11-05): the page is Shipments with Ship(s), the Party Shipment Draft and collapsible catalog groups, the DM's ship limit sits beside the voyage, and the board's dial reads "Next shipment arrives at Day N". `0006_market.sql` changed in place, so a local database that has it needs `mise run fresh` | Built, not committed: **pending the author's check-in** with `docs/checklists/market.md` |
| Polls | Questions for the table (SPEC §3.5, designed with the author 2026-11-04, reworked to native Discord polls 2026-11-06): `/c/{slug}/polls` ("Polls" in the nav after Shipments) where the DM starts, closes and deletes polls. A campaign with a linked server and an announcements channel gets `DISCORD` polls: Beny Bot posts a native Discord poll there, players vote only in Discord, and Beny Bot syncs the voters back (players named, everyone else counted as "N others"); otherwise polls are `SITE` polls with a vote card on the colony board until a player votes. Single or several answers, changeable until close; the DM never votes on the site; closing writes one PUBLIC `POLL_CLOSED` entry; a required duration (1 hour to 32 days) is kept by Beny Bot's planner. New migration `0007_polls.sql` | Built, not committed: **pending the author's check-in** with `docs/checklists/polls.md` |

Each milestone's scope and acceptance criteria are in SPEC.md §10. Scope that M1.5 changed is in the design document.

---

## 3. The author's decisions (authoritative)

Most of these came from conversations, not documents. When code or docs disagree with this list, ask the author before changing anything.

### The game

- **The colony sits on a new world with no DM-provided map.** **Eldari** (`web/public/eldari.webp`) is the **mainland** that ships come from. The hex map is the players' own tool to chart the new world: free painting, **no fog of war, no hex history**.
- **Eldari belongs to the author's campaign only.** Never offer it to other campaigns (C6); another DM would ask the author.
- **M5 answers (2026-10-12, SPEC §11 questions 15–16):** a fixed weekly or every-other-week slot entered as a series (one-off sessions still possible); the time zone is set on `/settings`. The Discord event is an **external** event with a text location the DM edits on `/settings`, not a voice-channel event; the default location is **"Online"** for every campaign. No "Join the game" link. ICS only, no Google Calendar, and the feed **keeps played dates** (past 30 days onward); only cancelled dates leave it. Played sessions only stamp colony days; nothing else happens automatically.
- **M4 answers (2026-10-12):** no map is seeded in production. Each DM uploads their own Mainland art, and the author uploads Eldari like any DM; a map's art never points at the bundled `/eldari.webp` (it serves the sign-in page). `colony seed` uploads the Eldari bytes as Sample Colony's Mainland for local testing only. The two-finger gesture split in Paint mode is checked by the author on a real phone in the M4 checklist, with the pointer-handler fallback built in from the start.
- **Don't invent game mechanics.** The author is still designing the campaign. Advance Day does only what SPEC §5.2 says: Days +1, and upkeep of 1 food per colonist per day, floored at the minimum. Nothing happens when food hits 0 or morale is low. Undecided rules become DM-editable data or open questions, never code.
- **The DM advances time by hand**, any number of days, with no limit enforced by the site.
- **Colonists = the sum of role headcounts**, with named NPC colonists counting toward their role. New colonists land in **Unassigned**.
- **Patrons:** favor is earned by doing quests and missions for a patron. Patrons offer actions that spend favor ("spend X favor → +7 food"). **Players request, the DM approves.**
- **M3 answers (2026-10-12, SPEC §11 questions 5, 9, 12–14):** quests start visible (with a "Post to timeline" checkbox); patrons and actions start hidden. Missions are quests. Players see each visible patron's exact favor. No limits, cooldowns or quotas on requests; a request must be affordable (STRICT) when made and again at approval, refused with the shortfall named. Spending never takes favor below 0; the DM's adjustments may. Only completed quests and DM adjustments grant favor (Effects has no favor field). **A decline note is seen only by the requester and the DM.** **A quest may have an optional failure cost** (resources, CLAMP; plus favor lost with its patron giver, which may go below 0), DM-only until it fails. Reopening a failed quest is status only; completed quests don't reopen.
- **The player store is the Market** (SPEC §3.3; the author's answers of 2026-11-03 close Open Question 18): each order has its own voyage (a DM setting, default 14 days) and the supply-ship cadence is unchanged; the ship waits at the docks until someone pays in full from colony gold, and while every allowed ship is out (the DM's ship limit, default 1, added 2026-11-05) no new order can be placed (the DM can cancel; Advance is never blocked); one shared party cart that any active member edits and confirms; a bought Farmer lands in Farmers (the Market's one exception to decision 8); magic and unique items are recorded only, with no inventory or characters; 1 gp = 2 food.
- **Polls** (SPEC §3.5, answered 2026-11-04, reworked 2026-11-06): Beny Bot posts a native Discord poll (the author chose it over the earlier button message), so a poll is voted in exactly one place, fixed when it is created: Discord when the campaign has a linked server and an announcements channel, else the site. Discord's limits apply everywhere (a 300-character question, 2 to 10 options of 55 characters, a duration of 1 hour to 32 days); the DM chooses single or several answers per poll; a vote can change until the poll closes; counts and players' names are public to the campaign and the Discord channel; only active players' votes are named, and other Discord voters are only counted, never stored; the DM closes by hand or the duration runs out; the close's timeline entry lists the counts and names no winner; after posting, a poll can only be closed or deleted; closed polls stay only on the Polls page.
- **Coming soon after the milestones above:** buildings, sickness, events. Leave seams; don't design their rules.
- **Players see exact stockpile numbers** (open question 5, answered 2026-10-10). A role hierarchy where, say, only a treasurer sees gold is a "maybe, probably not": don't build it, don't rule it out. Players also see each visible patron's exact favor (answered 2026-10-12).
- **No random or fuzz property tests for the game rules for now.** Keep the fixed worked examples (§5.12). Leave the randomness ban in `.golangci.yml` as it is.

### People and access

- **Many campaigns, each sealed off from the others.** One DM per campaign; no co-DMs, no hand-off (C1).
- **Anyone may sign in with Discord and ask to run a campaign; the site owner approves.** A denied requester may ask again, with one open request at a time (C4).
- **Players join with the DM's invite link; the DM approves.** No Discord-server membership check.
- **The site owner manages but never reads.** The owner approves DM requests, lists campaigns, suspends, unsuspends and deletes. A non-member, the owner included, gets 404 from every campaign route.
- **A campaign's address (slug) can be changed only by the owner at approval, then never** (C2).
- **A DM can delete their own campaign** after typing its slug (C3).
- **A player can leave, removing only themselves** (C5). Leaving is REMOVED, recorded as their own choice so `/players` shows "Left". The invite link can't bring them back; only the DM re-admits.
- **One shared Beny Bot**, invited by each DM to their own server and linked with a one-time `/link` code (one campaign per server).

### Technology

- **One Go backend** (Go 1.27) with Beny Bot in the same process; **React 19 + Vite + TanStack Router/Query**; **Postgres 17** (Docker locally, Neon at M6). One schema source: SQL migrations, with sqlc generating Go.
- **huma/OpenAPI is on trial.** The author may switch back to protobuf/Connect, so handlers stay thin: decode, call one `internal/core` function, encode. Only `internal/app` knows huma.
- **Direct Discord OAuth2 with server-side sessions. No Clerk.**
- **Local only until M6.** No Railway, domain or production-database work before then. The production Beny Bot token is never used by this project; local testing uses the author's dev bot.
- **M6 answers (2026-10-14, SPEC §11 question 19 and §10 "As built in M6"):** the live site uses the existing development application, **Beny Bot Dev**, for now (a separate production application may come later), so **its token runs in one place at a time**: while Railway has it, the author's `.env` has `DISCORD_BOT_TOKEN=` blank (it has to be in `.env`: mise's `.env` values win over a variable typed before `mise run dev`). Go live at https://colony.benthedev.com from the first deploy and run a test session there (changed on 2026-10-17 from "Railway's own address first, then party-planner's domain"); on switch day party-planner stops and its address redirects to colony's. The old site serves until that switch, and is retired over 30 days (old Neon credentials and old bot token reset on switch day). Go 1.27.2 before going live. **No row-level security now**, and **no membership cache** (counted per path: only a tab visible on a screen that stays on wakes Neon, once per 10-minute stream rotation; the trigger to add one is in the SPEC note).
- **No data import from party-planner.** Its data was test data. `colony seed` loads sample data instead.
- **Keep:** Undo, View as player, Record cargo. **Cut:** hex map history.
- **The seam-based architecture is enforced, not just documented.** See CLAUDE.md "Boundaries": depguard allow-lists, `internal/core/boundary_test.go`, Biome rules that keep network calls in `web/src/api/`, and the only-`devauth`-build-tag check.
- **The bot's full name is "Beny Bot"** everywhere a person reads it. The slash command stays `/beny`.

---

## 4. How the author likes to work

- **Ask before building; propose before deciding.** The author answers product questions directly. Recommend a default, and don't push for decisions while they're still asking questions.
- **Check in after every milestone,** with a plain checklist the author can run (`docs/checklists/M*.md`), before starting the next one.
- **Commits and PRs.** By default the author commits. When they ask for a PR, commit on a `claude/<name>` branch (their branch prefix), push, and open a PR against `main` with the attribution lines from the session. Never commit to `main`. The author merges.
- **Honesty about time and cost.** M0 took 12 hours, mostly stalled agents, and the author called it out. Since then: time limits on every agent, a stall watchdog, and progress updates. Give realistic estimates and say so when a stage overruns.
- **Verify before claiming.** The author expects you to check things yourself (run the checks, click through screens in the browser) and to say plainly what wasn't verified.
- **Conventions:**
  - full descriptive names in every language (no `ctx`, `req`, `res`, `cfg`; Go's `err` is fine);
  - TypeScript: no `any`, no `console.*`, no nested ternaries, `import.meta.env` only in `web/src/env.ts`;
  - conventional commits.
- **Plain language in user-facing text and in reports to the author.**

---

## 5. Pending and open items

Roughly in order:

- [ ] **Run the cutover** with `docs/cutover.md`, ticking `docs/checklists/M6.md` as you go. Start with `mise install` and `mise run check` in `apps/colony-planner`, then push the monorepo's `main` with green checks (Part 0: Railway builds `main` from this folder), then Part 4's same-token rule before the first live deploy. Agents never do these steps: every one touches Railway, Neon, Discord or DNS.
- [ ] **M6 choices to confirm** (SPEC.md "As built in M6"): the live campaign is linked to the private test server until switch day, so the real server never sees two bots; the privacy page promises account deletion on request, done by hand with the runbook's SQL (it keeps what the person wrote and shows them as "Former member", "Left"); an account-delete operation in the app instead?; the M6 checklist asks for Google Calendar and the phone's home screen at colony.benthedev.com, which never changes, so nothing is re-added on switch day.
- [ ] **Try the Market** with `docs/checklists/market.md`. It starts with `mise run fresh`, which **erases local data** and loads Sample Colony's market items, a cart and an order at sea. Then, if the author asks for a PR, commit on a `claude/` branch.
- [ ] **Market choices to confirm** (SPEC §3.3): the voyage defaults to 14 days; the ship limit defaults to 1 (1 to 100), and lowering it below the open count cancels nothing, it only blocks new orders; every campaign starts with Food (2 for 1 gp) and Materials (1 for 5 gp) offers and a 30 gp colonist offer for every role, Unassigned included (named "Colonist"), and a role the DM creates is offered at 30 gp at once; a supply offer brings 1 to 1,000,000 units a purchase; other resources aren't for sale until the DM sets a price; a new row is visible to players at once, with no hidden switch, and Stop selling (archiving) is the only way to withdraw one; the cart's 10,000 limit counts purchases (lots), not units; gold is checked only at payment, so confirming with too little gold only warns; payment refuses, rather than skips, a grant whose resource or role was archived since the order was placed; a cancelled order's lines don't return to the cart; only the DM cancels, so players can't cancel an order at sea; the catalog groups start open; Beny Bot is unchanged (a "Market order" field on `/colony` would be a small follow-up).
- [ ] **Try Polls** with `docs/checklists/polls.md`. It starts with `mise run fresh`, which **erases local data** and loads the sample polls; a local database that ran the earlier button version of `0007_polls.sql` needs it anyway, since the migration changed in place. The Discord step (the posted native poll, voting there, the deadline close, closing early and delete) is the author's to run, after re-inviting Beny Bot Dev with the Send Polls permission; agents checked Beny Bot only with fakes. Then, if the author asks for a PR, commit on a `claude/` branch.
- [ ] **Polls choices to confirm** (SPEC §3.5): Discord's limits on every poll, site ones too (2 to 10 options, a 300-character question, 55-character options, a duration of 1 hour to 32 days), and 10 open polls per campaign; a campaign whose link or announcements channel is missing at creation gets a site poll that stays one; the DM's Discord vote counts only as an unnamed other vote; a poll whose time ran out shows as closed at once, but without Beny Bot its "Poll closed" entry is written only by the campaign's next poll write (10 minutes after a posted poll's deadline); deleting a poll keeps its "Poll closed" entry; the board's line says which option leads ("3 votes, Beside the harbor leads"), which the timeline never does; a poll has no hidden or DM-only parts, so a DM can't run a secret ballot, and a Discord poll's link shows players the server's and announcements channel's IDs.
- [ ] **Try M5** with `docs/checklists/M5.md`. First **delete the `DISCORD_GUILD_ID` line from `.env`** (nothing reads it any more; agents never touch `.env`). The checklist starts with `mise run fresh`, which **erases local data** and loads the sample series and played sessions. The Discord steps (events, the cancellation notice, one reminder across a restart, Start and End Event, `/colony` and `/nextsession`) and the Apple Calendar subscription are the author's to run; agents checked Discord only with fakes. Then, if the author asks for a PR, commit on a `claude/` branch.
- [ ] **M5 choices to confirm** (SPEC.md "As built in M5" notes): every member, players included, can read the ICS link, and only the DM regenerates it; Mark played, End session and Discord's stamps aren't undoable (they leave the undo slot alone); the server lets the DM mark a future date played; a date nobody marked played leaves `/sessions` once it ends; the table's time zone is readable by every member on the colony board (it has no secret), so players see "Table time 8:00 PM EDT (New York)"; restoring a date earlier than the one whose event stood in makes the restored date a new event and removes the later one (only an edit that leaves a date off the series moves its event); reminders use the cancellation notice's channel (reminder, falling back to announce); the cancellation notice ignores the series' Announce and Remind switches and skips past dates; an event someone deletes or cancels by hand in Discord isn't replaced until the series is edited or the date ends; unlinking deletes Beny Bot's events for upcoming dates in the old server, but after a kick it can't, so those stay; removing a player doesn't change the calendar link, so a removed player who copied it keeps reading the schedule until the DM presses **New link…** (the removal confirmation says so; rotate it on removal instead?).
- [ ] **M4 choices to confirm** (SPEC.md "As built in M4" notes): terrain values are uppercase like every other enum; the external-URL map art is kept (https only, not counted in the budget, fetched by players' browsers from that host; no screen for it yet); an upload nothing uses within an hour is deleted by the campaign's next upload; at most 200 maps per campaign; a label needs a charted hex (labelling an uncharted one is a 404); deleting a map unpins its places; the Mainland isn't listed first (the DM orders maps by hand); in Paint mode the pointer handler always drives two-finger pan and pinch, and a mouse can't drag-pan there.
- [ ] **M3 choices to confirm** (SPEC.md "As built in M3" notes): a request's own note is visible to the whole party (only the decision note is per viewer); undoing a grant puts the request back to waiting; a `PATRON_ACTION` entry is public only when both the patron and the action are revealed; a hidden patron's favor line goes in the entry's DM-only note; a request the DM used while its patron or action was hidden stays DM-only even after both are revealed; the DM can't send a request (only Use action now); undoing Use action now writes "The DM rewound a change to the colony" rather than naming the action, in the DM's timeline too; Use action now's note is visible to players who can see the request (its field says so), and the action carries the board version, so a repeated submit is refused instead of spending twice.
- [x] **Go 1.27.2**: the author decided on 2026-10-14 to take it before going live (M6 moved `mise.toml`, `go.mod` and the Dockerfile's builder, and `scripts/vulncheck.sh` drops the 10 advisories it fixes).
- [x] **Go 1.27.3**: the author decided on 2026-11-04 to take it during the Market check-in, so govulncheck passes again (GO-2026-6809); `mise.toml`, `go.mod` and the Dockerfile's builder moved.
- [ ] **Offered, not yet decided:**
  - hide unbuilt sections from the navigation until their milestone lands (about 15 minutes);
  - make Vite scan every page at startup so a dependency change never needs a second `mise run dev` restart (about 5 minutes);
  - turn on Auto-fix for PR CI.
- [ ] **Known small things, fine for now** (from the M1.5 debrief):
  - Old M1 URLs (`/timeline`, `/players`) now 404, with no redirects.
  - The web has unit tests for helpers only, not for the new routes and flows. A few spots match mutation keys by string.
  - `go test ./internal/bot` needs the local Postgres.
  - Real-Discord behavior (global commands, `/link`, kicked-bot cleanup) has been checked only with fakes; checklist M1.5 steps 17–19 cover it by hand.
  - The picker's "Run your own campaign" card keeps saying "Sample Colony is ready" after the owner's own request was approved.
  - The owner-ping limiter counts a failed DM toward its hourly cap.
  - The broker's `memberActive` map is never pruned (tiny memory growth).
  - A campaign with no DM row (only possible by editing the database directly) disappears from Site admin.
- [ ] **Known small things, fine for now** (from the M5 build):
  - Discord behavior (events, reminders, the notice, Start and End Event, `/colony`, `/nextsession`) was checked only with the discordgo fakes and a fake clock.
  - In `schema.d.ts`, `Npc.firstSession`, `NextSession.session` and `LatestRecap.session` are typed as never null (huma can't mark an object nullable), so the web annotates them as nullable itself.
  - Live updates between two browsers on the session screens weren't checked, and Firefox wasn't tried.
- [ ] **Known small things, fine for now** (from the M4 build):
  - The two-finger gesture split was checked only with synthetic touch events in Chromium's mobile emulation, and Firefox wasn't tried; the frame timings (about 0.1 ms a frame panning 5,000 hexes once the zoomed-out map is cached) come from the Mac. The real phone decides.
  - Zoomed out on a big map, a stroke ends with one or two full repaints of the cached map image (about 20 ms each on the Mac), which may be a short hitch on a phone after lifting the finger.
  - Keyboard users can pan Explore with the arrow keys but can't open a hex's sheet, which needs a tap or click.
  - The server reads only an upload's header, so a file cut off after its header is stored and shows as a broken image (the DM's own upload; players' phones are protected by the 16,384-pixel and 50-megapixel caps).
  - The hexes read is about 234 KB of uncompressed JSON at 5,000 hexes (18 KB gzipped). Live updates no longer abort a read in flight, so a slow connection still gets one read a second while someone paints; M6 resolved this: `internal/app/compress.go` gzips JSON, so the read is 233,412 bytes plain and 13,554 gzipped at 5,000 hexes (the reserved `?since=` is still unbuilt).
  - Lists that carry an NPC reference (named colonists on People, a place's NPCs who are here) show initials, not portraits.
  - Upload shows "Uploading…", not a progress bar (fetch reports no upload progress).
  - Sandboxes on plain `localhost` share one cookie host, so two at once sign each other out; give each its own host (`PUBLIC_URL=http://<key>.localhost:<port>`).
- [ ] **Known small things, fine for now** (from the M3 build):
  - Beny Bot stores a request prompt's message ID but not its channel. If the DM picks another DM channel after a prompt is posted, that prompt can't be edited and keeps its buttons (clicks still answer correctly). Fix: store the channel with the message.
  - There is no exported "already decided" error, so the bot reads the request once more after any Conflict to tell it from a shortfall.
  - The pending-request count shows on the top-bar badge only, not on the sidebar or the phone's More tab.
  - Use action now has no server preview; its sheet checks the shortfall in the browser.
  - A request made with Use action now is the DM's, so it shows "Yours" to the DM and isn't in the credited player's "My requests".
  - Web tests still cover helpers only (`web/src/lib/ventures.ts`, `web/src/lib/clipboard.ts`), not the M3 screens (the approval sheet, the quest and patron sheets, Use action now). Screen tests need a component-test library, such as Testing Library on happy-dom; adding one is the author's call.
  - Real-Discord behavior of request prompts (posting to the DM channel, Grant and Decline, the edited outcome) has been checked only with fakes; the checklist's Discord step is the author's to run.
  - Add loot and Copy invite link over `mise run dev:phone` (plain http, where `crypto.randomUUID` and the clipboard are missing) were fixed but checked only in Chromium on localhost, not on a real phone.
- [ ] **Known small things, fine for now** (from the M2 debrief):
  - A narrow race: setting an NPC's known name in one tab while joining that NPC to the colony in another can write a PUBLIC entry that uses the true name.
  - Renames only ever move an NPC's entries to DM-only and never move them back. Changing a revealed NPC's known name also hides entries that use the old name, which the party did know.
  - Web tests still cover helpers only, not screens (the NPC and place sheets and pages, People, the timeline's NPC links).
  - Beny Bot's `/npc view` needs the exact known name and doesn't match aliases, and NPCs who share a display name can't be reached there.

The full open-question list, with defaults, is SPEC.md §11. The "Before M2" questions were answered on 2026-10-11 (all defaults), the "Before M3" ones on 2026-10-12, and the "Before M5" ones (15–16) on 2026-10-12.

M2 sequencing decided on 2026-10-11: columns arrive with the tables they point at. `npc.first_session_id` comes with sessions in M5; NPC portraits (`npc.avatar_url`) and place pins (`place.map_id`, `pin_x`, `pin_y`) come with images and maps in M4. M2 shows initials avatars. As built in M4, the portrait column is `npc.portrait_image_id` (one of the campaign's images).

---

## 6. Working in this repo

### The author's machine

- **The author's own `mise run dev`** runs Go on **8080** and Vite on **5173** against their **`colony`** database, with their **real dev-bot Discord credentials** in `.env`. Never stop it, never use those ports, never write to that database, and never read, print or copy `.env` or `*.pem` values. Checking whether a key is *present* (by name) is fine.
- **Their other processes:** the old party-planner may be running on 3000, 8000 and 8001. Leave them alone.
- **Docker:** container `colony-planner-db` (Postgres 17 on `127.0.0.1:54329`, databases `colony` and `colony_test`). `mise run db:studio` runs **DbGate** on `http://localhost:54330`, a Drizzle-Studio-like browser. After any migration, DbGate needs **"refresh DB model"**: the clock in its bottom status bar. It caches the structure server-side.
- **The author's screenshots come from a Firefox-based browser**, but agents test in a Chromium pane. Keep that gap in mind for browser-specific bugs.

### Testing without touching the author's setup (the sandbox)

Run your own copy on other ports with its own database:

1. Build the current code: `mise exec -- go build -tags devauth -o <scratch>/colony-<key> ./cmd/colony`. It embeds `web/dist/spa`; create a placeholder `index.html` there if it's missing.
2. `docker exec colony-planner-db psql -U colony -d postgres -c 'CREATE DATABASE colony_<key>'`.
3. Run inside `mise exec -- bash -c '…'`, so `.env` loads, then:
   - Point `DATABASE_URL` at your database: `sed` the `/colony` database name to `/colony_<key>`, and never print the URL.
   - Set `PUBLIC_URL=http://localhost:<vite-port or go-port>`, `PORT=<go-port>` and `APP_ENV=development`.
   - Blank the Discord values (`DISCORD_BOT_TOKEN= DISCORD_CLIENT_ID= DISCORD_CLIENT_SECRET= SITE_OWNER_DISCORD_ID=`).
   - Run `migrate up`, then `seed` (skip it for an empty database), then `serve`.
4. Optional UI: from `web/`, run `COLONY_DEV_GO_URL=http://127.0.0.1:<go-port> mise exec -- bunx vite --port <vite-port> --strictPort`.
5. Sign in with `<url>/auth/dev?as=owner|dm|player|pending|<account id>|new&name=X`. With curl, add `-H 'Sec-Fetch-Site: same-origin'`.
6. Always stop your servers and `DROP DATABASE` afterwards.

Empty `SITE_OWNER_DISCORD_ID` in a devauth build makes the seeded `seed:dm` account the owner. The seed loads two campaigns:
- **Sample Colony** ("Port Eldari", Day 22), run by Sample DM. Hollis plays Pell here.
- **Second Sample** ("Ridgehold", Day 9), run by Hollis.

Ana plays in both. Wren has a pending request to run a campaign. These sandbox scripts lived in a temporary scratch folder during the M0–M1.5 sessions. If they'll be used again, consider adding them to `scripts/`.

### Checks

- **`mise run check`** is the gate: codegen drift, the build-tag check, vet and lint for both builds, govulncheck, Go tests (real Postgres), typecheck, Biome, web tests, build, and `check-docs`. **Never run two at once**: it rebuilds `web/dist`, and concurrent runs race.
- **The secrecy and isolation sweep** (`internal/app/secrecy_test.go`) only catches values someone planted. **Every new DM-only field or hidden row needs canaries** in `secrecy_fixtures_test.go`, in both campaigns.
- **Never renumber or delete a merged migration.** M1.5's squash was a one-time, pre-deploy exception.

### Lessons from M0–M1.5 multi-agent runs

- **Stalls, not work, are the big time sink.** Give every agent a time limit, and run a watchdog that flags any agent silent for more than about 12 minutes. A stuck agent can be stopped and the run resumed, keeping finished agents' results.
- **Sequence overlapping work.** Contract first (migrations, DTOs, stub operations, generated types), then backend and web in parallel with strict file ownership, then integration, then review and fix. Let only one stage run `mise run check`.
- **The browser pane reports `document.hidden = true` when it's hidden.** The app then closes its live stream on purpose and shows "Connecting". Override `document.hidden` with JavaScript and dispatch `visibilitychange` to test live updates.
- **After dependency changes, the first `mise run dev` start can break lazy pages** while Vite re-bundles. Restart once.
- **Reviewers find real issues.** Two independent reviewers plus a fixer caught bugs in every milestone. Still, click through new screens yourself: integrators have run out of time for browser checks more than once.
- **The permission system sometimes blocks compound shell commands** that include `rm` or inline `bash -c` scripts. Use script files and the dedicated tools instead, and never work around a denial.

---

## 7. History, briefly

- **2026-10-09 to 10:** analyzed party-planner and wrote a verified brief and stack assessment. The author chose a clean-room rebuild in a new folder, keeping Go. A judge panel of three drafts, two judges and a critic produced SPEC.md. Decisions followed: local only, no import, direct Discord OAuth, huma on trial.
- **2026-10-10:** M0 built; it took 12 hours because of stalled agents. The bot was renamed "Beny" → "Beny Bot". Seams were closed and enforced by lint and tests. The repo was created as the private `BBruington/colony-planner`, and the Go module path changed from `TheBenDev` to `BBruington`. M0 was reviewed (go-with-fixes). M1 was built in about 1½ hours. DbGate was added.
- **2026-10-11:** the author decided to support other DMs. The multi-campaign design was written, critiqued and approved with C1–C6. M1.5 was built in about 2 hours, with independent browser verification. A cleanup pass squashed migrations (schema proven byte-identical), removed the upgrade scaffolding and applied the pre-merge fixes. This handoff was written. PR #2 was merged.
- **2026-10-11 (evening):** the author answered the "Before M2" questions (all defaults). M2 was built in about 1 hour 40 minutes (9 agents). The first review found 11 issues, including a hidden NPC's true name reaching players through a timeline entry; all were fixed, then a second adversarial round re-checked the fixes.
- **2026-10-12:** M2 went up as PR #3. After the debrief the author approved follow-up fixes for it: an NPC's or place's visibility now changes only through the switch on its page (edits and stale edit sheets can't reveal it again), a server field error no longer keeps Save disabled after it's corrected, timeline entries about an NPC link to their page, and some wording and switch polish.
- **2026-10-12 (later):** M2 merged. The author answered the "Before M3" questions and added the failure cost. M3 was built in stages (core, app, web, Beny Bot, then integration), with the secrecy sweep extended to quests, patrons and per-viewer decision notes, and walked on a sandbox with curl and the browser.
- **2026-10-12 (evening):** the M3 debrief's fixes: undo after a quest delete, action-named `PATRON_ACTION` titles, SPEC and migration corrections (`CHECK (reward_favor >= 0)` in `0003`), unarchived-only patron and action limits with an unarchive guard and a ceiling counting archived rows, Add loot and copying on plain http, archived resources in the effects editor, field errors and hints in Use action now and the approval sheet, and broader live-update topics.
- **2026-10-12 (night):** M3 merged (PR #4). M4 was built in stages (core, app, the Explore hex map and the image maps in parallel, then integration): per-campaign images with a 50 MB budget, maps with pins, hexes with a 50,000 limit, NPC portraits and the PWA manifest, with the secrecy sweep extended to hidden maps, pins and images, and walked on a sandbox with curl and the browser (desktop and phone width).
- **2026-10-13:** M4 merged (PR #5). M5 was built in stages (core, then the HTTP layer, the web screens and Beny Bot in parallel, then integration): series and on-demand session rows per campaign, one planner across linked campaigns with claim-then-send reminders and a 5-minute event lease, the ICS feed, the NPC's first session, and the legacy guild-command cleanup removed; the secrecy sweep covers sessions and the ICS token, and the screens were walked on a sandbox with curl and the browser.
- **2026-10-13 (later):** the M5 review's fixes: every-other-week parity counts whole weeks from the first date (a Saturday keeps the Sunday after it), a start inside the spring-forward gap begins after the skipped hour, a series deleted or a date cancelled while Beny Bot makes its event leaves no event or announcement behind, a reminder whose channel couldn't be read is retried, malformed calendar tokens read as a missing route, the sweep forbids the calendar token to pending and removed members, an in-play session keeps its band on later timeline pages, editing a session's days moves its "Session played" entry, First met takes only played sessions, and players see no reconstructed table time on played sessions.
- **2026-10-13 (later still):** the second M5 review and the author's 2026-10-12 decisions: the ICS feed keeps played dates; the default event location is "Online" and a Settings body without it is refused rather than resetting it; editing a series moves its existing Discord event (RSVPs kept) and a new location updates it; unlinking deletes the campaign's upcoming events in the old server and forgets them, so a relinked server gets its own; the time zone is on the colony board for players; a date restored mid-game no longer deletes the next date's event; a one-off sharing a title and start with the weekly game gets its own event; a fall-back start east of UTC is the first one; the planner's back-off follows the wall clock; event claims check the campaign is still linked to that server; non-canonical calendar URLs are redacted in the log; the unused ComingSoon page is gone.
- **2026-10-14:** M5 merged (PRs #6 and #7). The author asked for M6 and decided: Beny Bot Dev serves the live site for now (with the same-token rule), Railway's own address first and then the domain, Go 1.27.2 before going live, no row-level security now, and the membership cache decided from evidence (none). The deployment half was built in stages (the Dockerfile, `railway.toml`, CI and the production host check; then the privacy page, the runbook and the docs).
- **2026-10-17:** colony-planner moved into the benthedev monorepo as `apps/colony-planner`, history included (rewritten with git filter-repo, so every commit ID changed and bare PR numbers in commit messages now read `BBruington/colony-planner#N`). CI moved to the monorepo's `colony-planner` workflow, and the cutover now goes live at colony.benthedev.com from the first deploy, with party-planner's address redirected there on switch day. No Go or web code changed.
- **2026-11-03:** the author answered Open Question 18 (store details) and approved the Market design: per-order voyages, payment at the docks in colony gold, one shared party cart, typed colonists into their role, special items recorded only, 1 gp = 2 food. It was built in stages (migration, rules, core and HTTP; the web screens; seed, tests and the secrecy sweep; docs) and awaits the author's check-in.
- **2026-11-04:** the author designed Polls in two rounds of questions (SPEC §3.5). Built in stages (migration, rules, core and HTTP; then the web screens, Beny Bot and the seed with the secrecy sweep in parallel; docs) and awaits the author's check-in.
- **2026-11-06:** Polls reworked from Beny Bot's button message to native Discord polls with Discord's limits: a poll is voted only in Discord or only on the site, fixed at creation, and Beny Bot syncs Discord's voters back. `0007_polls.sql` changed in place, so a local database that ran it needs `mise run fresh`. Awaits the author's check-in.
