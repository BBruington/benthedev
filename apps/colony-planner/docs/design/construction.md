# Construction: buildings inside the colony

**Status:** designed with the author and approved on 2026-11-15, with §10's defaults. Built in two phases
(§11); Phase 1 is in progress.
**Date:** 2026-11-15.
**Builds after:** nothing it depends on is unbuilt. It uses the existing resource and role tables, `Effects`,
the daily-step pipeline (`internal/game/advance.go`), the colony write path and undo slot
(`internal/core/colony.go`) and the timeline.
**Changes:** SPEC.md §3.2 (buildings move from "Next" to designed), §4.2 (the `resource` CHECK, §6.2 here),
§5.1 (new colony mutations), §5.2 (two new daily steps), §5.4 (a positive delta never lowers an amount above
its cap), §5.8 (new timeline kinds) and §8.6's seams table. Everything else stands.
**Audience:** the author (developer and DM) first, builders second.

The rule from CLAUDE.md still holds: the app keeps the books and the DM is the engine. Every rule below is one
the author chose in the interview; anything not chosen is data the DM types or an open question in §10, never
code.

## 0. The decisions on one page

| # | Decision |
|---|---|
| K1 | **The DM keeps a catalog of blueprints.** A blueprint is a building type: name, description, DM notes, a material cost, labor, an optional max crew per labor line, optional one-time completion Effects, and optional ongoing output (daily production or upkeep, storage bonuses). |
| K2 | **Material cost is a map keyed on the resource table**, `{resourceKey: amount}`, stored as rows with a composite foreign key to `resource`, as `market_item` does. Moving from `materials` to `wood`, `stone` and `ore` is a data change on `/settings` and the blueprints, never code. |
| K3 | **The full cost is taken when a project starts, STRICT.** Short on anything, Start is refused with the shortfall named. No reserved stock. |
| K4 | **Cancel refunds what the DM types.** The cancel sheet is prefilled with the full cost; the DM edits it (0 is fine) and the refund applies CLAMP. The app has no salvage rate. |
| K5 | **Labor is a map keyed on the role table**, `{roleKey: role-days}`, mirroring the cost map. A new blueprint starts with one line for the campaign's construction role (a setting, Builder by default); the DM may add lines for other roles (a Mage line for a warded tower). |
| K6 | **Each labor line has its own crew**, a count of that role's colonists. Each crew member adds 1 role-day to its line per day. The project is finished when every line reaches its effort. |
| K7 | **Crews are explicit.** Across all unfinished projects, a role's crews never exceed its headcount (unnamed plus named colonists). Idle colonists of each role are shown. |
| K8 | **A workforce change that would strand crews is refused** ("5 Builders are on projects; take 2 off a project first"). Nothing shrinks a crew silently. |
| K9 | **A line may cap its crew** (`max_crew`, blank for no cap), so 40 builders can't raise a hut in a day. There is no minimum duration. |
| K10 | **Advance finishes a project on the exact day** its last line reaches its effort, frees each line's crew on the day that line finishes, and names finishes in its preview. Undoing the Advance un-finishes it. |
| K11 | **A finished building is a record, plus what its blueprint opts into:** one-time completion Effects (CLAMP, like a quest reward), flat daily output per resource (positive is production, negative is upkeep, CLAMP), and storage bonuses that add to a resource's cap. |
| K12 | **The DM can switch a finished building Idle or Active.** Idle stops its daily output and its storage bonus without demolishing it. |
| K13 | **Storage bonuses add to the cap set on `/settings`**, and only while the building is Active. Losing a bonus never destroys stock: the amount stays, and only new gains are refused until it falls under the cap. |
| K14 | **Building output runs before colonists eat**, so today's harvest feeds today. |
| K15 | **Any active member starts a project and moves crews** (a member-level colony change, like Market Pay). Cancel, demolish, Idle and Active, and blueprints are the DM's. The DM can undo a player's change like any other. |
| K16 | **Blueprints start hidden**, with a reveal switch, like patron actions. A hidden blueprint, and every project and building of it, is a hidden row for players. The DM may build a hidden blueprint. |
| K17 | **Players' timeline shows only finishes** ("The Granary is finished"). Every other construction change still writes an entry, DM-only, so each stock change is explained and Undo has a label. |
| K18 | **A started build is a snapshot; ongoing output is live.** Cost, labor, max crews and completion Effects are copied when a project starts. Daily output and storage bonuses are read from the blueprint, so rebalancing the Mill changes every standing Mill from the next Advance. |
| K19 | **The DM can add a building instantly, for free** (decided 2026-11-15): Add building on a blueprint places a FINISHED, Active building on today's day with no cost, no labor and no completion Effects. It is a colony change the DM can undo, writing a `BUILDING_ADDED` entry that players see while the blueprint is revealed. A colony may hold any number of buildings of one blueprint, built or added. |

## 1. Costs

- A blueprint's cost is zero or more `(resource_key, amount)` rows, `amount` 1 to `game.MaxStored`. Any
  unarchived resource may appear, gold included; the DM decides what makes sense (morale as a cost is allowed).
- **Start** (any active member) is a colony change carrying `expectedVersion`:
  - it snapshots the blueprint's cost into the project (`cost jsonb`, in `Effects` resource shape, negated);
  - it applies the cost with `game.ApplyEffects(state, cost, game.Strict)`, so every shortfall is named in
    one refusal ("Not enough stone: the colony has 40, needs 60");
  - a cost naming a resource archived since the blueprint was written refuses Start, naming it, as Market Pay
    refuses an archived grant. The DM edits the blueprint or unarchives the resource.
- **Cancel** (DM, an unfinished project): a sheet prefilled with the snapshotted cost as positive deltas;
  the DM edits any line, down to 0, and confirms. The refund applies CLAMP. A line naming a resource archived
  since is left out of the prefill (Effects refuse archived keys) and the sheet says so.
- **Demolish** (DM, a finished building): the same sheet, prefilled empty. Whatever the DM types applies CLAMP.
  The building's output and storage bonus stop at once.
- Gold is just another resource here. Buying materials stays the Market's job; construction never buys.

## 2. Builders and labor

- `settings.construction_role_key` names the role a new blueprint's first labor line uses. Campaign approval
  sets it to `BUILDER` (seeded at approval today); the DM may change or clear it on `/settings`. It is a
  convenience for new blueprints only: existing blueprints keep their lines.
- A blueprint's labor is one or more `(role_key, effort, max_crew)` rows: `effort` 1 to `game.MaxStored`
  role-days, `max_crew` 1 or more, or NULL for no cap. A blueprint with no labor line can't be saved (a
  building that needs no work is a DM Adjust, not a project).
- A project copies its blueprint's labor lines at Start, each with `progress = 0` and the crew the starter
  chose (0 or more, at most `max_crew`).
- **Crews.** For every role R: the sum of R's crews over every unfinished project is at most R's headcount
  (`unnamed_count + named colonists`, as `game.Colonists` counts them). Set crews (any active member) checks
  this under the colony lock and refuses past it ("Only 2 Mages are free"), and refuses past `max_crew`.
- **Headcount drops** (Transfer, Set count, Change role or Leave colony of a named colonist) are refused while
  they would leave a role with fewer colonists than its crews: "5 Builders are on projects; take 2 off a
  project first." The check lives beside the existing workforce rules in `internal/game/workforce.go`, so the
  workforce functions take the crews per role as input.
- **Idle colonists** per role (headcount minus crews) show on the Buildings page and beside each role on
  People. Nothing makes idle colonists do anything else (decision 4: workers produce nothing).
- Named NPC colonists count toward their role's headcount and so toward crews, but crews are counts: the app
  doesn't say which colonists are on which project.

## 3. Build time

- A line with crew `c > 0` and `r` role-days left finishes `ceil(r / c)` days into a span. A line with crew 0
  never finishes; its project shows "Needs Mages" instead of a finish day.
- Work starts the day after Start: Advance covers `(from, to]`, as upkeep does.
- A line's crew is set to 0 at the end of the day that line finishes (its colonists become idle; nothing
  reassigns them). The project is **finished** on the day its last line finishes: status FINISHED,
  `finished_day` set, and from the next day it counts as a standing building.
- The project page shows each line's progress (`45 / 60 Builder-days`) and the finish day with the current
  crews ("Finishes on Day 52"). The Advance preview names every finish in its range ("The Granary is
  finished on Day 52."), as it names a goods ship docking.
- No minimum duration (the author picked max crew only). If a ritual or curing time ever matters, it is a new
  optional blueprint field (§10).

## 4. What a finished building does

| Opt-in on the blueprint | Rule | Mode |
|---|---|---|
| Completion Effects | Applied once, on the finish day, inside the Advance that finishes it. Colonists land in Unassigned (decision 8). | CLAMP |
| Daily output `{resourceKey: delta}` | Every day the building is Active and standing, each delta applies. Positive is production, negative is upkeep. When upkeep can't be paid, the resource stops at its minimum and nothing else happens (the rule food already follows). | CLAMP |
| Storage bonus `{resourceKey: +n}` | While Active, the resource's effective max is its `/settings` max plus every Active building's bonus. A resource with no max is unaffected. | n/a |

- **Idle and Active** (DM): a colony change. A new building starts Active. Idle stops its daily output and
  storage bonus from that moment.
- **Excess stays (K13).** When a cap falls (Idle, demolish, a blueprint edit, the DM lowering it), the amount
  stays where it is. A positive delta then adds nothing until the amount is under the cap; a negative delta
  applies normally. `ApplyEffects`'s CLAMP becomes `after = old + Δ` clamped to `[min, max(old, max)]` for
  positive deltas, so a gain never lowers an amount.
- A finished building is never "used up": it stands until demolished.

### 4.1 The daily steps

`DailySteps` becomes `[]Step{BuildingOutput{}, Upkeep{}, Shipments{}, Construction{}}`:

1. **BuildingOutput**: every Active standing building's daily output, CLAMP, against the effective caps.
2. **Upkeep**: unchanged (1 food per colonist, floored at the minimum).
3. **Shipments**: unchanged.
4. **Construction**: each unfinished line gains `crew` role-days, capped at its effort; lines and projects
   finish as in §3; a project finishing today applies its completion Effects and its storage bonus now, and
   its daily output from tomorrow.

Boundaries (`NextBoundary`) are the finish days of lines and projects, because a finish changes crews (read by
the next span's progress), standing buildings (read by BuildingOutput) and caps. Between boundaries the
per-day deltas are constant, so each span stays closed form even with clamps: a resource with a constant net
daily change and fixed bounds reaches a bound, or a fixed point at a bound, within a computable number of
days. The existing guarantees still hold and are tested: one day N times equals N days once,
`Advance(a+b) == Advance(Advance(a), b)`, and the span engine matches the naive day-by-day reference. N stays
unbounded (decision 4), so no step may fall back to a loop over N.

### 4.2 Worked example (for `internal/game/spec_examples_test.go`)

Day 40, food 12 (min 0, max 100), 5 colonists, 4 Builders, no Mages. A Mill (cost `materials 30`, labor
`BUILDER 20` with max crew 4, daily output `food +10`) is started on Day 40 with crew 4.

- Advance 7: Days 41 to 45 the Mill builds (20 Builder-days at 4 a day, finished on Day 45, Builders idle
  from the end of Day 45). Food: Days 41 to 45 are upkeep only, 12 → 7 → 2 → 0 → 0 → 0. Day 46: +10 then
  −5 = 5. Day 47: 5 + 10 − 5 = 10. Preview: "Day 40 → 47 · Food 12 → 10 · The Mill is finished on Day 45."
- A Granary (`food` storage +200) finished while food is 100 raises the cap to 300; set Idle later at food
  250, food stays 250 and the Mill's +10 adds nothing until food falls under 100.

## 5. Automation versus DM judgment

| The app does | The DM decides |
|---|---|
| Takes the cost, STRICT, at Start | What a blueprint costs, needs and gives |
| Counts role-days, caps crews, finishes on the exact day | Which roles a blueprint needs, and every number |
| Refuses crews past headcount or `max_crew`, and workforce changes that strand crews | Who moves when a crew must shrink |
| Applies completion Effects, daily output and storage bonuses, CLAMP | Whether a building runs (Idle or Active) |
| Shows idle colonists, progress and finish days | What a cancelled or demolished building gives back |
| Writes the timeline and the undo slot | Everything the app does nothing about: a building damaged, a shortfall's consequences, a crew's mood |

Nothing happens when a building's upkeep can't be paid, a project stalls with no crew, or colonists sit idle.

## 6. Data model sketch

All tables are campaign-owned (Invariant 11): `campaign_id` first in every key and index, composite foreign
keys, every query scoped. Blueprints are archived, never deleted, so references to them are NO ACTION, as
patrons' are.

### 6.1 Blueprints (live)

| Table | Key fields | Notes |
|---|---|---|
| `building_blueprint` | `id`, `name` (unique per campaign), `description`, `dm_notes`, `revealed bool DEFAULT false`, `archived bool DEFAULT false`, `completion_effects jsonb DEFAULT '{}'` (`Effects`), `sort_order`, timestamps | Limits are technical bounds, like `MapLimit`: 200 blueprints per campaign. |
| `building_blueprint_cost` | `(campaign_id, blueprint_id, resource_key)` PK, `amount int CHECK (> 0)` | FK `(campaign_id, resource_key) → resource`, as `market_item_resource_fkey` does. |
| `building_blueprint_labor` | `(campaign_id, blueprint_id, role_key)` PK, `effort int CHECK (> 0)`, `max_crew int NULL CHECK (> 0)` | FK `(campaign_id, role_key) → role`. At least one row per blueprint (core checks). |
| `building_blueprint_output` | `(campaign_id, blueprint_id, resource_key)` PK, `daily_delta int DEFAULT 0`, `storage_bonus int DEFAULT 0 CHECK (>= 0)`, `CHECK (daily_delta <> 0 OR storage_bonus > 0)` | FK to `resource`. Live (K18): edits change standing buildings from the next Advance. |

### 6.2 Projects and buildings (one row through its life)

| Table | Key fields | Notes |
|---|---|---|
| `building` | `id`, `blueprint_id → building_blueprint NO ACTION`, `status building_status` (CONSTRUCTING, FINISHED, CANCELLED, DEMOLISHED), `active bool DEFAULT true` (meaningful only while FINISHED), `cost jsonb` (snapshot, the Effects Start applied), `completion_effects jsonb` (snapshot), `started_day`, `finished_day NULL`, `ended_day NULL` (cancel or demolish), `refund jsonb NULL` (what Cancel or Demolish applied), `started_by → campaign_member`, timestamps | `CHECK ((status = 'FINISHED' OR status = 'DEMOLISHED') = (finished_day IS NOT NULL))`. Several buildings of one blueprint are allowed (§10). |
| `building_labor` | `(campaign_id, building_id, role_key)` PK, `effort`, `max_crew NULL`, `progress int DEFAULT 0 CHECK (progress BETWEEN 0 AND effort)`, `crew int DEFAULT 0 CHECK (crew >= 0 AND (max_crew IS NULL OR crew <= max_crew))` | Snapshot of the blueprint's labor plus live progress. FK to `role`. A line is done when `progress = effort`, and a CHECK keeps a done line's crew at 0. |

- `settings.construction_role_key text NULL`, FK to `role`; approval sets `BUILDER` where that role exists.
- New Postgres enum `building_status`: re-exported in core, listed in `enumTypes` (`internal/app/enums.go`) and
  `allowedDataReferences`, per the Add an entity recipe.
- **The one existing-table change:** `resource`'s `CHECK (amount >= min_amount AND (max_amount IS NULL OR
  amount <= max_amount))` loses its max half, because K13 lets an amount sit above a cap that fell. The max is
  enforced in `internal/game` (effective max = `max_amount` + Active bonuses), computed there from the standing
  buildings in `game.State` (§11, Phase 2).
- `game.State` gains the construction state: unfinished lines (role, effort, progress, crew) per building, and
  standing buildings (Active or Idle) with their blueprints' daily output and storage bonuses. Core loads them with the colony (`readColony`).
- Archiving guards, so Advance never meets an archived key: a resource can't be archived while a standing
  building's blueprint output names it, Idle ones included; a role can't be archived while an unfinished project's labor
  names it (crews are already 0, since its headcount must be 0) or while it is `construction_role_key`.
- A new topic `TopicConstruction` in `internal/core/broker.go`; colony changes also publish the colony topic.
- Screens: `/c/{slug}/buildings` ("Buildings" in the Colony nav group after Shipments): Standing buildings,
  Under construction (progress bars, crews, finish day, Set crews), Blueprints (players see revealed ones and
  Start; the DM edits, reveals, archives), and idle colonists per labor role. Every player screen works at
  360 px.

## 7. Secrecy and visibility

- **Blueprints start hidden** (K16), with their own reveal switch endpoint, never a full-record PUT
  (Invariant 16's pattern). `dm_notes` lives in `BuildingBlueprintDm`.
- A hidden blueprint, and every building of it in any status, is a hidden row (Invariant 15): 404 for
  players in a path, and the missing-ID field error ("That blueprint doesn't exist.") in a body (Invariant 11), absent from lists, the Advance preview's finish names and Beny Bot. Players can't Start or crew it.
- A hidden blueprint's projects still take colonists. Players see those crews only as "busy on other work" in
  idle counts, so totals stay right and nothing names the project, as a hidden named colonist counts unnamed.
- A hidden blueprint's cost still leaves the stockpile (players see exact numbers, open question 5); its Start
  entry is DM-only like every Start.
- Revealing a blueprint reveals its projects and buildings, and its `BUILDING_FINISHED` entries reach players
  from then on; hiding it again hides them.
- Secrecy fixture: a hidden blueprint with canaries in its name, description and DM notes, a project and a
  finished building of it, its `BUILDING_FINISHED` entry, a revealed blueprint's DM notes, and the DM-only
  construction entries, in both campaigns. `blueprintId` and `buildingId` go in `mixedIDFields`.
- The Foundry bridge sends nothing about buildings in this design (§10).

## 8. Undo and timeline

### 8.1 Colony changes

Every change below takes the colony lock, runs one transaction, writes its entries, records the undo
snapshot and publishes (Invariant 2). Start and Set crews carry `expectedVersion`, so two phones moving crews
can't overwrite each other.

| Change | Who | Entry kind | Visibility |
|---|---|---|---|
| Start | any active member | `BUILDING_STARTED` (cost deltas) | DM |
| Set crews | any active member | `BUILDING_CREWS_CHANGED` ("Mill: Builders 2 → 4") | DM |
| Cancel | DM | `BUILDING_CANCELLED` (refund deltas) | DM |
| Demolish | DM | `BUILDING_DEMOLISHED` (refund deltas) | DM |
| Idle, Active | DM | `BUILDING_IDLED`, `BUILDING_ACTIVATED` | DM |
| Advance across a finish | DM | `BUILDING_FINISHED` on `finished_day`, in the Advance's group, with completion Effects as deltas | PUBLIC while the blueprint is revealed, else DM |

- The DM can still switch a DM-only entry PUBLIC, as with `STOCK_ADJUSTED`, but only while its blueprint is
  revealed.
- Entries carry a `building_id` link column (SET NULL), so the timeline can hide an entry whose blueprint is
  hidden now, as `quest_id` does.
- Advance's `DAYS_ADVANCED` body includes building output in each resource's total ("Food +31"), and its
  deltas record what applied. Blueprint edits, reveals and archives aren't colony changes and write no entry.

### 8.2 Undo

- `undoSnapshot` gains `Buildings map[uuid.UUID]buildingBefore`: status, active, finished and ended days, and
  each line's progress and crew. Start creates its row, so its `buildingBefore` says `created` and undo deletes
  it (as `requestBefore.Created` does). Effective caps aren't stored, so nothing about them is snapshotted.
- Undoing an Advance restores progress, crews and statuses, un-finishes what it finished and marks its
  `BUILDING_FINISHED` entries undone.
- UNDO's visibility follows the existing rule (PUBLIC if any undone entry was PUBLIC), so undoing a player's
  Start writes a DM-only UNDO. An UNDO carries the `building_id` of what it undid.

## 9. Edge cases

- A Start or Set crews from a stale sheet is 409 "The colony changed" (version guard).
- Crew 0 on a line: the project never finishes and the preview never names it; the page says what's missing.
- A Market Pay that brings Builders only raises headcount, so it never strands crews.
- Undo of an older workforce change can't strand crews: undo reaches only the latest change.
- Effort and progress arithmetic is int64 with the `MaxStored` overflow guard (§5.2).
- A blueprint edit that removes a labor line or lowers `max_crew` doesn't touch started projects (K18).
- A blueprint archived while buildings of it stand: they keep standing and producing; it leaves players'
  Start list.
- Completion Effects naming a resource archived since Start: that one line is skipped and the DM-only note on
  the finish entry names it (a finish happens inside Advance and can't be refused).

## 10. Open questions

1. **Limits per blueprint.** **Resolved (2026-11-15):** unlimited, built or added (K19).
2. **Prerequisites and upgrades.** Does a blueprint ever need another building standing first, or replace
   one (Hut → House)? *Default: neither; the DM reveals blueprints when the party may build them.*
3. **Staffed output.** Should a building's output ever scale with workers in a role ("+2 food per Farmer, up
   to 3")? *Default: flat with Idle and Active (K12), which leaves room for this as blueprint data.*
4. **Minimum duration.** Rituals, curing, seasons. *Default: none (K9); a nullable `min_days` field if needed.*
5. **Damage and repair.** *Default: none; the DM uses Idle and Adjust.*
6. **Beny Bot.** A "Building" line on `/colony`, or finishes posted to a party channel (open question 17)?
   *Default: Beny Bot unchanged.*
7. **Foundry bridge.** Should the colony summary journal list standing buildings? *Default: no outbox change
   until the author asks; it would come only from the player projection (Invariant 21).*
8. **Players' view of their own Start.** A player who starts a project sees it on Buildings but not on the
   timeline, since Start is DM-only (K17). *Default: as decided; revisit if the table finds it confusing.*
9. **The DM lowering a cap on `/settings` below the amount.** Today's CHECK forbids it; with §6.2's CHECK change
   the amount could stay above. *Default: keep refusing a `/settings` max below the current amount, so only
   building bonuses can leave excess.*

## 11. Phases

Each phase ends with `mise run check` green, a sandbox walk-through and a checklist in `docs/checklists/`,
then stops for the author's check-in.

**Phase 1, construction** (migration `0011_construction.sql`):

- `building_blueprint`, `building_blueprint_cost`, `building_blueprint_labor`, `building`, `building_labor`,
  the `building_status` enum, `settings.construction_role_key` and `timeline_event.building_id`;
- blueprints: create, full-record edit (never `revealed` or `archived`), reveal and archive switches, DM notes;
- Start, Set crews, Cancel and Demolish (a finished building with no output is a record, so Demolish is
  only its refund and status);
- the `Construction` daily step (`DailySteps` becomes `Upkeep, Shipments, Construction`), finishes, the
  Advance preview's finish line and completion Effects;
- the crew guards on workforce changes and the archive guards on roles (§6.2);
- undo, the timeline kinds of §8.1 but Idle and Active, secrecy (§7) and its fixture, the seed;
- the Buildings page with idle colonists per labor role (People is unchanged in Phase 1).

**Phase 2, output** (migration `0012_construction_output.sql`): `building_blueprint_output`, `building.active`,
the `BuildingOutput` step ahead of Upkeep, Idle and Active, storage bonuses, the `resource` CHECK change and
K13's never-lower rule in `ApplyEffects`, and the resource archive guard. Details settled when it started
(2026-11-15):

- **Food's output runs inside Upkeep.** As built, `BuildingOutput` handles every resource but food, and
  `Upkeep` applies food's output then its eating as one day map, which keeps K14's order and lets one closed
  form cover both once a cap or floor engages.
- **One net change per resource per day.** BuildingOutput sums every Active standing building's daily delta
  for a resource and applies that sum once a day, CLAMP with K13's never-lower rule, so two buildings never
  depend on which one runs first.
- **Effective caps live in `internal/game`.** `game.State` carries the standing buildings (ID, Active, their
  blueprint's live daily deltas and storage bonuses); a resource's effective max is its `/settings` max plus
  every Active standing building's bonus, or none when the `/settings` max is none. Every rule that reads a
  max (ApplyEffects, completion Effects, BuildingOutput, the Market's grants) uses the effective one, and a
  project finishing mid-advance raises the cap from its finish day.
- **The board shows effective caps** ("out of 300"); `/settings` shows the DM's own max and the building
  bonus beside it. Changing a resource on `/settings` checks the current amount against the effective max,
  not the DM's own, so a Granary's stock never blocks renaming food.
- **The resource archive guard covers every standing building**, Idle ones too: a resource can't be archived
  while a FINISHED building's blueprint output names it. A blueprint's output, like its cost, names only
  unarchived resources when saved.
- **Idle and Active** are DM colony changes with DM-only `BUILDING_IDLED` and `BUILDING_ACTIVATED` entries;
  undo restores `active`. Demolish ends output and bonus at once.
- **Players see the numbers move** (open question 5): a hidden blueprint's output still changes stock and a
  hidden Granary's bonus still shows in the cap, as a hidden Start's cost does. The building itself stays
  hidden.
