# Construction Phase 1 checklist: blueprints, crews and finishes

The design is `docs/design/construction.md` (§11 says what Phase 1 holds). Daily output, Idle and storage
bonuses are Phase 2, so a finished building here is a record plus its one-time completion Effects.

## Start it

1. Make sure Docker Desktop is running. If `mise run dev` is running, press Control-C in its tab. The server
   has to restart to pick up construction (migration `0011_construction.sql`).
2. **The next command erases your local data.** It loads Sample Colony's blueprints: a revealed **Granary**
   (materials 30 and gold 20, 40 Builder-days with at most 4 Builders, morale +5 when finished), a revealed
   **Palisade** under construction (10 of 60 Builder-days, 3 Builders on it) and a hidden **Warded Tower**
   (30 Builder-days and 10 Mage-days). Second Sample has a **Mine Hoist**.

   ```bash
   cd ~/myCodeSpace/benthedev/apps/colony-planner
   mise run fresh
   mise run dev
   ```

   Wait for `dev: open http://localhost:5173`. If a page doesn't load on the first start, press Control-C and
   run `mise run dev` once more.

The steps build on each other, so do them in order. A private window counts as a second person.

## Try it

1. **The page.** Sign in as **Sample DM** and open Sample Colony.
   - **Buildings** is in the navigation after Shipments (on a phone, in the More sheet); the address ends in
     `/buildings`.
   - The colonists card shows Builders: headcount, how many are on projects and how many are idle.
   - **Under construction** shows the Palisade at 10 / 60 Builder-days with its crew and a finish day.
   - **Blueprints** lists the Granary, the Palisade and the Warded Tower, which carries a **Hidden** badge.
   - **Undo** on the board names "Palisade crews: Builders 0 → 3".
2. **Secrecy.** In a private window sign in as **Ana**. Buildings shows the Granary and the Palisade but no
   Warded Tower. As Sample DM, switch the Warded Tower's **Revealed** on: it appears for Ana without a reload.
   Switch it off again: it disappears.
3. **Crews.** As Ana, open **Set crews** on the Palisade.
   - Asking for more Builders than the colony has free is refused with "Only N Builders are free."
   - Lower the crew by one and save: the finish day moves later and the idle count goes up by one.
   - As Sample DM, the timeline has a DM-only "Palisade crews" entry; Ana's timeline has none.
4. **Workforce guard.** As Sample DM, on **People**, try to transfer every Builder to Farmers: it is refused
   with "N Builders are on projects; take M off a project first." Nothing changes.
5. **Start.** As Ana, press **Start** on the Granary.
   - The sheet shows its cost against the stockpile. Pick 1 or 2 Builders (only the free ones) and start:
     materials drop by 30 and gold by 20, and the toast says only the DM sees it on the timeline.
   - As Sample DM, edit the Warded Tower's materials cost above what the colony holds and open its Start
     sheet: it names the shortfall and Start stays off. Put the cost back.
   - As Sample DM, press **Undo** on the board: the Granary project disappears, the stock comes back, and the
     UNDO entry reads "The DM rewound a change to the colony". Start the Granary again.
6. **A finish.** As Sample DM, open **Advance** and type enough days for the Granary to finish (its card
   shows the day).
   - The preview has a line "The Granary is finished on Day N." beside the other lines.
   - Confirm. The Granary moves to **Standing** with its finish day, its Builders are idle again, morale rises
     by up to 5 (capped at 100), and the timeline has a PUBLIC "The Granary is finished" entry on Day N that
     Ana also sees.
   - Press **Undo**: the Granary is under construction again with its old progress and crew, and morale goes
     back. Advance again to finish it.
7. **Cancel.** As Sample DM, press **Cancel** on the Palisade. The refund sheet starts at its full cost
   (materials 40); change it to 25 and confirm. Materials rise by 25, the Palisade moves to **Past**, its
   Builders are idle, and the timeline has a DM-only "Construction cancelled: Palisade" entry.
8. **Demolish.** As Sample DM, press **Demolish** on the standing Granary. The refund sheet starts empty;
   confirm with nothing. It moves to **Past** and the timeline has a DM-only "Granary demolished" entry.
9. **Blueprints.** As Sample DM, press **New blueprint**.
   - It starts with one labor line for the construction role (Builder). A name the campaign already uses is
     refused on the Name field.
   - Save "Watchtower" with a cost, 20 Builder-days and no crew cap. It starts hidden; Ana can't see it.
   - **Edit** it, then **Archive** it: it moves to the DM's archived group and leaves Ana's list for good.
   - On **Settings**, the Construction card's role picker changes which role a new blueprint starts with.
10. **View as player.** As Sample DM, turn on **View as player**: Buildings looks exactly like Ana's, with no
    DM controls, hidden blueprints or archived ones. Turn it off.
11. **Phone width.** On a phone (or a 360 px window), Ana can start a project and set crews one-handed with no
    sideways scrolling.

## Not in Phase 1

Daily production and upkeep, Idle and Active, storage bonuses and the resource archive guard come in Phase 2.
Beny Bot and the Foundry bridge know nothing about buildings.
