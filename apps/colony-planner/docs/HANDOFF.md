# Handoff: Colony Planner

**Last updated:** 2026-10-12, after the M2 debrief fixes. M2 is [PR #3](https://github.com/BBruington/colony-planner/pull/3) (branch `claude/m2-people-and-secrecy`), awaiting the author's checklist run and merge.
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
| M2 | People and secrecy: `/people` (transfers, set counts, named colonists joining and leaving), NPC roster with server-side secrecy, places (unpinned), Settings roles card, Beny Bot `/npc view` | In [PR #3](https://github.com/BBruington/colony-planner/pull/3) (branch `claude/m2-people-and-secrecy`) with the debrief fixes, awaiting the author's checklist run (`docs/checklists/M2.md`) and merge |
| M3 | Quests (Effects rewards, favor to patron givers), patrons and actions, player requests with DM approval (Discord Grant/Decline buttons) | Planned |
| M4 | Hex map (players chart the new world; no fog of war, no history), the mainland map with pins | Planned |
| M5 | Sessions (local time + time zone), Discord events, reminders, recaps, ICS feed | Planned |
| M6 | Deploy to the author's existing domain, replacing party-planner. **Optional, and only after the author says the app meets their standards** | Not started |

Each milestone's scope and acceptance criteria are in SPEC.md §10. Scope that M1.5 changed is in the design document.

---

## 3. The author's decisions (authoritative)

Most of these came from conversations, not documents. When code or docs disagree with this list, ask the author before changing anything.

### The game

- **The colony sits on a new world with no DM-provided map.** **Eldari** (`web/public/eldari.webp`) is the **mainland** that ships come from. The hex map is the players' own tool to chart the new world: free painting, **no fog of war, no hex history**.
- **Eldari belongs to the author's campaign only.** Never offer it to other campaigns (C6); another DM would ask the author.
- **Don't invent game mechanics.** The author is still designing the campaign. Advance Day does only what SPEC §5.2 says: Days +1, and upkeep of 1 food per colonist per day, floored at the minimum. Nothing happens when food hits 0 or morale is low. Undecided rules become DM-editable data or open questions, never code.
- **The DM advances time by hand**, any number of days, with no limit enforced by the site.
- **Colonists = the sum of role headcounts**, with named NPC colonists counting toward their role. New colonists land in **Unassigned**.
- **Patrons:** favor is earned by doing quests and missions for a patron. Patrons offer actions that spend favor ("spend X favor → +7 food"). **Players request, the DM approves.**
- **A player store is planned for later** (magic items, food, colonists, special NPCs). It's a separate design effort; the data model must not block it.
- **Coming soon after the milestones above:** buildings, sickness, events. Leave seams; don't design their rules.
- **Players see exact stockpile numbers** (open question 5, answered 2026-10-10). A role hierarchy where, say, only a treasurer sees gold is a "maybe, probably not": don't build it, don't rule it out. Whether players see each patron's exact favor is still open (part of OQ 5); the spec's default is yes.
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

- [ ] **Try M2** with `docs/checklists/M2.md` (it starts with `mise run fresh`, which **erases local data** and loads the new sample NPCs and places), then merge PR #3. Step 15 (`/npc view` in real Discord) can only be checked by the author.
- [ ] **Remove Beny Bot's legacy guild-command cleanup.** That's `DISCORD_GUILD_ID` / `LegacyGuildID` in `internal/bot/bot.go`, `cmd/colony/serve.go` and `cmd/colony/config.go`, plus its tests and docs. Waiting on the author: if `/beny` appears only once in their Discord test server, the old command is gone. Then delete `DISCORD_GUILD_ID` from `.env` and remove the code.
- [ ] **Bump Go to 1.27.2 on or after 2026-10-22** (in `mise.toml` and `go.mod`), then remove the 10 accepted stdlib advisories from `scripts/vulncheck.sh`. The author was asked whether to take these security fixes earlier and hasn't answered.
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
- [ ] **Known small things, fine for now** (from the M2 debrief):
  - A narrow race: setting an NPC's known name in one tab while joining that NPC to the colony in another can write a PUBLIC entry that uses the true name.
  - Renames only ever move an NPC's entries to DM-only and never move them back. Changing a revealed NPC's known name also hides entries that use the old name, which the party did know.
  - Web tests still cover helpers only, not screens (the NPC and place sheets and pages, People, the timeline's NPC links).
  - Beny Bot's `/npc view` needs the exact known name and doesn't match aliases, and NPCs who share a display name can't be reached there.

The full open-question list, with defaults, is SPEC.md §11. The "Before M2" questions were answered on 2026-10-11 (all defaults). Ask the "Before M3" ones, plus the quest half of question 9, when M3 starts.

M2 sequencing decided on 2026-10-11: columns arrive with the tables they point at. `npc.first_session_id` comes with sessions in M5; NPC portraits (`npc.avatar_url`) and place pins (`place.map_id`, `pin_x`, `pin_y`) come with images and maps in M4. M2 shows initials avatars.

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
   - Blank the Discord values (`DISCORD_BOT_TOKEN= DISCORD_CLIENT_ID= DISCORD_CLIENT_SECRET= SITE_OWNER_DISCORD_ID=`) and `unset DISCORD_GUILD_ID`.
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
