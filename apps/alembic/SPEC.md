# Alembic Spec

The Casper Studios take-home build. Rules live in `Brewing rules.md`, and this spec decides what the app around them is.

## Problem

Crafting potions in PF2e is bookkeeping, not play. Jacob is moving his colony campaign from 5e to PF2e and wants potions brewed by measuring ingredients:

- each ingredient adds four tradition values (Arcane, Primal, Occult, Divine), PF2e's four magical traditions (see `Brewing rules.md` §1);
- a potion forms when the totals land near its hidden target;
- PF2e's rarity, feat, level and Crafting-check gates still apply.

Alembic is no longer a sandbox: ingredients are a finite, per-campaign inventory, and every brew uses up what goes into it. Exact-primary matching and real campaign content come later, after the move into benthedev.

## Users & Core Flow

**Primary user: the party's crafter (a player).** The secondary user is the GM, reached through a "Viewing as" toggle on the same account.

**The 60-second tour, which must work on the deployed URL:**

1. Sign in with GitHub, Google or Discord.
2. The workshop opens on:
   - Wren Ashby, a level 5 Alchemist;
   - 16 ingredients showing their tradition values, one of each in stock;
   - a codex with Healing Potion (Minor) already Known, plus a mentor's note: "Heartmoss + Marsh Toad Gland".
3. Brew the note. The totals are 7/26/1/8, which forms Healing Potion (Minor) against DC 15. The result shows it was off by 14, with arrows: Arcane on, Primal on, Occult low, Divine low.
4. Follow the arrows: restock Heartmoss and Marsh Toad Gland on the shelf, then brew them with Glowcap. The totals become 7/26/10/13, which is Perfect: 1 + 1d3 potions. The codex marks it Perfected.
5. Experiment toward the 10 undiscovered potions. A blocked brew names its gate; for example, Truth Potion is level 6 and Wren is level 5.
6. Flip to GM view. The answer key shows every target and perfect recipe. Reveal Invisibility Potion, flip back, and the codex shows it as Known with its name, rarity, DC and odds, but no target.

## Scope

- Clerk sign-in
- the workshop
- the ingredient inventory
- the brew bench
- server-side brew resolution
- the result card
- the codex
- the brew log
- the Viewing-as toggle
- the GM answer key
- GM reveal
- the crafter edit sheet
- colony-planner's palette
- the house-rules panel
- Railway deploy and the README

**Cut order if behind at T+70:** the crafter edit sheet goes first. GM view is the last thing cut.

## Requirements

**R1 Sign-in.** Clerk (`@clerk/tanstack-react-start`) on the Alchemyst app's development instance, with GitHub, Google and Discord through Clerk's shared dev credentials. There is no guest sign-in.

- A signed-out visitor sees a landing page with a two-sentence explainer and a sign-in button.
- Sign-in and sign-up live at `/sign-in` and `/sign-up`, and a successful sign-in lands on `/workshop`.
- Every app route and every oRPC procedure requires a session. The route guard redirects a signed-out visitor to `/sign-in` and is only UX; the oRPC middleware, which answers 401, is the real boundary.
- The app shows its own dev badge whenever the Clerk instance is a development one.

**R2 Workshop bootstrap.** The first authenticated request creates the user's workshop, in one idempotent transaction:

- a campaign with the user as its GM, viewing as player;
- Wren: level 5, Int +4, expert Crafting, Alchemical Crafting, expanded lab;
- Healing Potion (Minor) Known, with the mentor's note;
- one unit of every catalog ingredient in stock.

**R3 Ingredient shelf.** All 16 catalog ingredients, each with:

- name, rarity, type and form, biome;
- the four tradition values.

Each ingredient shows its stock with +/- buttons that change it by one (0-999). Players and the GM can both change it; each click is an atomic adjustment of the `ingredient_inventory` row, one per campaign and ingredient. Campaigns created before inventory existed are backfilled to one of each.

**R4 Brew bench.**

- Pick 1-4 units, repeats allowed, with +/- per ingredient and a hard cap of 4 in total.
- Four live total bars, one per tradition colour.
- An optional "table d20" field that accepts only 1-20, and a Brew button that is disabled while a brew is pending.
- No ingredient can be added past its stock, repeats included. A mix needing more than is in stock names every short ingredient and cannot be brewed.

**R5 Brew resolution.** `brew.attempt` resolves on the server in one write, exactly per `Brewing rules.md` §4:

1. Sum the units.
2. Shortlist potions whose primary value is within its room.
3. Apply the secondary budget.
4. Apply the rarity gate and any required ingredient.
5. Apply the crafter gates: feat, level, rank.
6. Roll the Crafting check against DC = the level DC plus the rarity adjustment.
7. Apply the outcome table.

The app rolls the d20 with crypto unless a table d20 was given. Every brew adds 1 to the crafter's downtime-day counter and subtracts its units from the inventory. The server refuses a brew the inventory can't cover before rolling, so nothing is logged, consumed or counted.

**R6 Result card.** It has three variants:

- **Formed:** the potion, how many were made (including 1d3 on perfect), the d20 arithmetic, how far off the mix was and an arrow per tradition. A critical success also shows the exact target of the worst-off axis.
- **Inert:** at most one hint.
- **Beyond:** names every failing gate.

Every brew also raises a small top-right toast naming the outcome (potion and count, inert, or beyond reach).

**R7 Codex.**

- A potion becomes Known when it forms or the GM reveals it.
- It becomes Perfected after any formed brew with a perfect mix, whatever the check's result.
- Known potions show name, rarity, level, DC, the crafter's odds and the best off-by so far.
- An undiscovered count shows how many remain.

**R8 Brew log.** Every brew, newest first: the mix, the totals, the roll arithmetic, the outcome and the time.

**R9 Viewing-as toggle.** It switches between GM and Player and is stored on the server. It can only lower the user's role, never raise it.

**R10 GM answer key.** Shows every potion's target, both rooms, any required ingredient and the perfect recipe.

- The player view never receives any of it, because one server-side projection strips it.
- A leak test asserts that.

**R11 GM reveal.** Reveal and unreveal a potion for the player. Revealing shows its name, rarity and DC, never its target.

**R12 Crafter edit sheet** (React Hook Form + Zod). Fields:

- level 1-20;
- Int from -1 to +7;
- Crafting rank;
- Alchemical Crafting, which requires trained Crafting;
- tools.

The DC odds and the gates update after saving.

**R13 Look.** Colony-planner's "Night Sea and Parchment" tokens are mapped into the shadcn variables, with one colour per tradition.

**R14 House rules and licence.** A panel lists the homebrew rules. The footer carries the ORC / Community Use notice and a "sample content is fictional" line.

**R15 Delivery.** Deployed on Railway with Neon. The README covers what works, what is incomplete, how to run it and the 60-second tour.

## Constraints & Assumptions

- **Stack:** TanStack Start, Bun, shadcn, Neon, Drizzle, oRPC, React Hook Form, Zod, and Clerk.
- **Timebox:** 2 hours, and all code is written inside it.
  - `brewing-engine/` is an answer key to diff against, not code to paste.
  - The catalog values are seed data and may be transcribed.
- **Schedule:**
  - Jacob does the account setup during T+0-10 while agents scaffold: Neon, then Railway, then the Clerk keys, then GitHub.
  - Every requirement is live by T+75.
  - T+75-100 is Jacob's revision window, and everything freezes at T+100.
- **The public repo holds only fictional content.** Run the `rg` sweep for real campaign names before pushing.
- **Clerk:** the Alchemyst app's development instance runs on the Railway domain with its dev keys. A production instance on a benthedev subdomain comes with the migration.
  - Env vars: `VITE_CLERK_PUBLISHABLE_KEY`, `CLERK_SECRET_KEY` (never `VITE_`-prefixed), `VITE_CLERK_SIGN_IN_URL=/sign-in` and `VITE_CLERK_SIGN_UP_URL=/sign-up`. Railway needs them as runtime variables, since `.env.local` never reaches it. Locally the app runs on `localhost`, not `127.0.0.1`.
  - Dashboard: GitHub, Google and Discord SSO on for all users, email address the only required identifier (any extra required field stalls an OAuth sign-up at `/sign-up/continue`), and access mode Open. The dashboard labels are unverified.
  - Users are keyed by the Clerk user id as plain text, with no user table and no webhook. The workshop is created on the user's first authenticated request.
  - What a reviewer sees, all acceptable: a sub-second hop through `clerk.accounts.dev` on first load, a "Development mode" label on Clerk's components, `accounts.dev`-branded OAuth consent screens, and a 100-user cap, plus the app's own dev badge.

## Success Criteria

- A stranger completes the 60-second tour on the deployed URL without reading the README.
- Every cell of the rules' outcome table can be reached from the UI. The table d20 field forces each degree of success.
- Unit tests pin the worked examples in `Brewing rules.md` §12, and the player-view leak test passes.
- `bun test`, the typecheck and Biome are clean at submission.
- Everything is live by T+75, and Jacob makes at least 15 minutes of his own revisions inside the timer.
