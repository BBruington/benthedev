# Construction Phase 2 checklist: output, Idle, storage and Add building

The design is `docs/design/construction.md` (§4, §11's Phase 2 details and K19). Phase 1's checklist comes
first; this one picks up from a fresh database.

## Start it

1. Make sure Docker Desktop is running. If `mise run dev` is running, press Control-C in its tab. The server
   has to restart to pick up migration `0012_construction_output.sql`.
2. **The next command erases your local data.** Sample Colony's Granary now stores 200 more food, and there is
   a revealed **Mill** (materials 25, 30 Builder-days with at most 3 Builders, food +6 and materials -1 a day).

   ```bash
   cd ~/myCodeSpace/benthedev/apps/colony-planner
   mise run fresh
   mise run dev
   ```

   Wait for `dev: open http://localhost:5173`. If a page doesn't load on the first start, press Control-C and
   run `mise run dev` once more.

Do the steps in order. A private window counts as a second person.

## Try it

1. **Output on blueprints.** Sign in as **Sample DM** and open **Buildings**.
   - The Mill's card reads "Each day: Food +6, Materials -1" and the Granary's "Stores 200 more Food".
   - **Edit** the Palisade and add an Output line, Morale +1 a day. Typing letters is refused on its field; a
     blank line shows a hint. Save, then remove the line again and save.
2. **Add building.** On the Mill, press **Add building** and confirm.
   - A Mill appears under **Standing** on today's day. No materials or gold were taken, and no Builders moved.
   - The timeline has a PUBLIC "Mill added by the DM" entry; in a private window, **Ana** sees it too.
   - Press **Undo** on the board: the Mill is gone. Add it again, and add a second Mill: both stand, since a
     colony can hold any number of one building.
3. **Daily output.** Open **Advance** and type 5.
   - The preview's Food line counts the two Mills' 12 a day against upkeep, and Materials drops by 2 a day.
   - Confirm: the board's numbers match the preview.
4. **Idle.** On one Mill press **Set Idle**: it shows an **Idle** badge and "Idle, so it gives none of its
   output." Advance 1 day: only one Mill's output counts. Press **Set Active** on it again. Ana sees the badge
   but no buttons, and her timeline shows neither change.
5. **Storage.** On **Settings**, set Food's maximum to 100 (Adjust food under 100 first if the form refuses).
   - Add a Granary (or start one and advance until it finishes). The board's Food tile reads "out of 300", and
     Settings reads "max 100, +200 from buildings".
   - Adjust food up to 250, then set the Granary **Idle**. Food stays 250 and the tile reads "over its max of
     100". Advance a few days: the Mills add nothing until food falls under 100, while upkeep still eats.
   - Set the Granary **Active** again.
6. **Demolish keeps the cap honest.** With food at its cap, **Demolish** the Granary with a food refund: food
   doesn't rise past the cap left without the Granary.
7. **Archive guard.** On Settings, try to archive Materials: it is refused, naming a Mill that uses it.
8. **Hidden blueprints.** Give the hidden Warded Tower an output line and **Add building** it. Ana sees the
   stock change but no Warded Tower anywhere and no "added by the DM" entry. Undo it.
9. **Phone width.** At 360 px, Ana's Buildings page shows output and Idle badges with no sideways scrolling.

## Not in this phase

Output that scales with workers, minimum build times, prerequisites, damage and repair, Beny Bot and the
Foundry bridge (design §10).
