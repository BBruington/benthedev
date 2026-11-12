# Foundry bridge Phase 1 checklist: connect your Forge world

Phase 1 is done when you connect the live site to your Forge world, advance a day on each side, and watch
both show up, including after closing Foundry and reopening it (`docs/design/foundry-bridge.md` §8, item 1).
Phase 1 uses only the core adapter, so your PF2e world (pf2e 7.12.2) and your dnd5e practice world both
work. Foundry is 13.351 on The Forge.

## Where to do it

A Forge game can't reach your Mac, so the real check needs the live site, https://colony.benthedev.com,
which exists after the M6 cutover (`docs/cutover.md`, Part 5 for the first deploy and Part 6 for signing in).
Do Part A (live) when the cutover is done.

Part B is optional and can come first: it needs Foundry running on your Mac at http://localhost:30000
(`devauth` builds accept that origin) and `mise run dev` running. Skip it if you don't run Foundry locally.
Part B follows the same steps, with these changes: sign in with **Dev sign-in** as Sample DM, open Sample
Colony, and use the manifest address that Settings shows (it points at your Mac, not the live site).

The steps build on each other, so do them in order. Only the active GM's browser runs the bridge, and the
token lives in that browser only.

## Part A. On the live site and your Forge world

1. **Install the module.** On the site, sign in and open your campaign's **Settings**, then the **Foundry**
   card. Press **Copy the address**. In Foundry: **Add-on Modules**, **Install Module**, paste into
   **Manifest URL**, **Install**.
   - [ ] Foundry lists Colony Bridge as installed, with no error.
2. **Enable it.** Open your world, then **Manage Modules**, tick Colony Bridge and save.
   - [ ] The world reloads and Colony Bridge shows as active.
3. **Make a code.** On Settings, press **Connect Foundry**.
   - [ ] A code like `K7Q-4XM` shows, with the time it works until (10 minutes).
4. **Connect.** In Foundry, open **Configure Settings**, find **Colony Planner** under Colony Bridge and
   press **Connect**. Type the code into **Pairing code** and press **Connect** again.
   - [ ] The menu says "Connected to" and your campaign name, then "Live: connected to" the same name.
5. **Settings shows it.** Reload the site's Settings.
   - [ ] The Foundry card says "Connected to" your Forge address, with **World** (your world's title),
     **Foundry** (13.351), **Game system** and **Colony Bridge** (the module version).
   - [ ] "Foundry is up to date." shows, and "Last heard from" shows today's date.
6. **The journal.** In Foundry's Journal tab:
   - [ ] A journal named "Colony" has appeared, with the day, your stockpile, your workforce, the next
     shipment and a link back to the site.
7. **Advance on the site.** Advance the colony 2 days on the colony board. Leave **Move Foundry's clock with
   colony days** off for now (it is under **Configure Settings**, in Colony Bridge's list).
   - [ ] Foundry gets a chat message from "Colony Planner": "The colony advanced 2 days, from Day X to Day Y."
   - [ ] The Colony journal shows the new day.
   - [ ] Foundry's world time did not move.
8. **The clock setting.** Turn **Move Foundry's clock with colony days** on and save. Advance 1 day on the
   site.
   - [ ] The chat message appears and Foundry's world time moves forward one day. Then turn the setting off
     again.
9. **A rename.** On the site, rename a resource (for example Timber to Lumber).
   - [ ] The Colony journal shows the new name within a few seconds, with no reload.
10. **Foundry moves time.** As GM, open the browser console (on a Mac: Cmd+Option+K in Firefox, Cmd+Option+J in
    Chrome, Cmd+Option+I then Console in Foundry's own app) and run `game.time.advance(86400 * 3)`.
    The 3 days must be a whole number of days past the last day the module saw.
    - [ ] The colony board shows a **From Foundry** card with "Foundry moved 3 days." and **Advance** and
      **Dismiss** buttons. The Settings card shows the same item.
11. **Advance from the inbox.** Press **Advance** on that item.
    - [ ] The Advance sheet opens with 3 days already filled in. Confirming it advances the colony, and the
      item leaves the board.
    - [ ] Foundry gets the chat message for it, and the journal updates.
12. **Dismiss.** Run `game.time.advance(86400)` again, then press **Dismiss** on the new item.
    - [ ] "Dismissed. The colony's clock stays where it is." shows, the item leaves, and the colony day does
      not change.
13. **Catch up.** Close the Foundry tab (leave the world). On the site, advance 1 day and rename a resource.
    Settings now says changes are waiting for Foundry (for example "2 changes are waiting for Foundry."). Reopen
    the world in Foundry.
    - [ ] Within a minute the chat message and the new journal text arrive, and Settings says "Foundry is up
      to date." This is the done criterion.
14. **View as player.** On the site, turn on **View as player**.
    - [ ] The **From Foundry** card is gone from the board. Turn View as player off. Have an item in the inbox first (run step 10 again) so there is a card to hide.
15. **A player's browser does nothing.** In a private window, log in to your Forge game as a player (not the
    GM). Open the console and run `game.time.advance(86400)` if you are allowed, or have the GM do it while
    the player window is open.
    - [ ] No "Foundry moved" item is created by the player's window, and the Network tab shows no request to
      colony.benthedev.com from it.
16. **Hidden journals (§9.1).** As GM, create a journal entry named "Bridge test" with its default ownership
    set to None for everyone. In the player's window, open the console and run
    `game.journal.getName('Bridge test')`.
    - [ ] Write down the result here: ______ . `undefined` means Foundry never sends a document a player
      can't observe to that player's browser, which answers question 3 (hidden NPCs and DM notes could then
      go to Foundry as hidden documents). An object means they must stay out. Delete the entry afterwards.
17. **Disconnect on the site.** On Settings, press **Disconnect…** and confirm **Disconnect**.
    - [ ] The card goes back to the install and connect steps.
    - [ ] In Foundry's **Colony Planner** menu (reload the GM's page first if it still says Live), the status
      says Colony Planner ended this world's link, and to make a new code and connect again.
    - [ ] The Colony journal stays in the world, unchanged.
18. **Connect again.** Make a new code, and connect with it as in steps 3 and 4.
    - [ ] Settings shows Connected again and the journal updates.

## Known limits

- The bridge works only while a GM has the world open. A Forge game keeps no code running when nobody is in it.
- After you press Disconnect on the site, the module may show a network error instead of a clear message,
  until you reload the GM's page.
- Foundry only offers the module an update when `foundry/package.json`'s version is bumped.
- "Last heard from" shows a UTC day, not your local time, and it changes at most once a day.

## When you're done

- Record the step 16 result in `docs/design/foundry-bridge.md` §9.1 (question 3).
- Part B only: press Control-C in the `mise run dev` tab. `mise run db:down` stops Postgres; your data is kept.
