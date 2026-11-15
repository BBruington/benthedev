# Foundry bridge Phase 3 checklist: sending NPCs, items and coins to Colony Planner

Phase 3 is done when you can right-click an actor or an item in Foundry and find it waiting in the DM's
inbox on the site, and when the party's or a character's coins go into colony gold and come back again without ever being
lost or counted twice (`docs/design/foundry-bridge.md` §14). Foundry still changes nothing on the site by
itself: every send is an inbox item that only you accept. Colony Bridge is now 0.3.1. Coin deposits are
PF2e only (pf2e 7.12.2, which runs on Foundry 13 only), in whole gold pieces. Your dnd5e world gets the NPC
and item sends.

## Where to do it

Do Part B (local) first: it needs Foundry running on your Mac at http://localhost:30000 and
`mise run dev` running, and it costs nothing if something is wrong. Do Part A (live) after the deploy, on
your Forge world, at https://colony.benthedev.com.

The steps build on each other, so do them in order. Only the active GM's browser runs the bridge, and the
new menu entries show only there. You need two browsers: the GM's, and a private window logged in to the
same world as a player. Keep the GM logged in the whole time. In Part B, sign in with **Dev sign-in** as
Sample DM and open Sample Colony. The inbox is the Foundry card on the colony board and the list on
**Settings**. The console is Cmd+Option+K in Firefox, Cmd+Option+J in Chrome, and Cmd+Option+I then
Console in Foundry's own app.

## Part B. Local: your Mac and a local Foundry

1. **Upgrade.** Restart `mise run dev` so the server has the new code. In Foundry, open **Add-on
   Modules** and update Colony Bridge to 0.3.1, then reload the world. You don't need a new code: the
   link stays.
   - [ ] Settings' **Foundry** card shows **Colony Bridge** 0.3.1 under the world details, and "Foundry is
     up to date."
2. **Only the active GM sees the entries.** In the GM's window, right-click an NPC in the **Actors** tab,
   then an item in the **Items** tab. Do the same in the player's window.
   - [ ] The GM sees **Send to Colony Planner** (a paper plane) on both.
   - [ ] The player sees neither, and no **Deposit to colony…** anywhere.
   - [ ] Turn **Bridge on** off under **Configure Settings**: the GM's entries go too. Turn it back on.
3. **Send an NPC.** Pick a PF2e NPC (not a character) whose **Public Notes** you can edit. Write a line
   there, then add a GM secret block (the editor's **Secret** button) holding a second line. Write a line
   in its **Private Notes**. Right-click it in the Actors tab and choose **Send to Colony Planner**.
   - [ ] Foundry says "Queued <name> for Colony Planner's inbox."
   - [ ] Within a few seconds the inbox reads "Foundry sent an NPC: <name>." with **Import as NPC** and
     **Dismiss**.
4. **Import it.** Press **Import as NPC**.
   - [ ] The NPC sheet opens with the actor's name as the true name, its level, its Public Notes line as
     Appearance, and DM notes holding the Private Notes line then the secret line.
   - [ ] The secret line is not in Appearance. No formatting tags (`<p>`, `&amp;`) show anywhere.
   - [ ] The visibility switch is off. Save: the NPC is created hidden, and the inbox item is gone.
   - [ ] As the player on the site, the NPC doesn't appear.
5. **A character and a compendium NPC.** Send a PF2e player character from the Actors tab, then open a
   bestiary compendium window and send a creature from it, then one whose level is -1 (a rat swarm or
   similar).
   - [ ] The character arrives with its ancestry as Race and its class as Class, its biography's
     Appearance as Appearance, and empty DM notes.
   - [ ] The compendium creature arrives with its name and level; Race and Class are blank (PF2e NPCs have
     only traits).
   - [ ] The level -1 creature arrives with no level.
   - [ ] Right-click the party actor: it has no **Send to Colony Planner**. In a compendium of party
     actors (if you have one), sending one says "A party can't become an NPC."
   - [ ] Right-click the character: it has **Send to Colony Planner** and **Deposit to colony…**. An NPC
     has no **Deposit to colony…**, and neither does anything in a compendium window.
6. **Send an item from a compendium.** Open the PF2e equipment compendium. Pick an item priced in whole
   gold pieces with a description (Boots of Striding or similar). Right-click it and choose **Send to
   Colony Planner**.
   - [ ] The inbox reads "Foundry sent an item: <name>." with **Add to Shipments** and **Dismiss**.
   - [ ] **Add to Shipments** opens the item sheet with the name, the price in gold, and **Foundry item
     UUID** holding `Compendium.pf2e.equipment-srd.Item.` and 16 letters and numbers.
   - [ ] **The description check.** The sheet's description holds the item's description text from
     Foundry. The module reads it from `system.description.value`, a path nobody has checked in the PF2e
     and dnd5e sources yet. If the description is blank while the Foundry item has one, note it: the path
     is wrong for that system. Write down what you found for `docs/design/foundry-bridge.md` §14.11.
   - [ ] Save: the item is in the catalog, and the inbox item is gone.
7. **Prices that aren't whole gold.** Send a torch (priced in copper) and an item priced in silver that
   isn't a multiple of 10 sp, then an item sold in a bundle (arrows, priced per 10).
   - [ ] Each opens with the price blank and the hint "Foundry's price wasn't a whole number of gold. Type
     one.", and Save stays off until you type a number.
   - [ ] An item priced 1 pp opens at 10 gold; one priced 30 sp opens at 3.
8. **World items.** Drag an item from the compendium into the Items tab and send that copy. Then create a
   brand new item in the Items tab and send it.
   - [ ] The dragged copy fills **Foundry item UUID** with the compendium item's UUID.
   - [ ] The new item leaves **Foundry item UUID** empty.
9. **Dismiss an import.** Send any actor and press **Dismiss** on it.
   - [ ] The site says "Dismissed. Nothing was added." and nothing was created.
10. **Deposit from the party.** Make sure the party actor has some coins (at least 50 gp worth).
    Right-click the party in the Actors tab and choose **Deposit to colony…** (coins icon). Type 40 into
    **Gold pieces to deposit** (the hint reads "Paid with any coins worth exactly this much.") and confirm.
    - [ ] The party's coins drop by exactly 40 gp worth. If it had no gold pieces, it paid in platinum
      and got change back in smaller coins.
    - [ ] Foundry says "Queued <party name>'s 40 gp deposit for Colony Planner's inbox."
    - [ ] The inbox reads "<party name> deposited 40 gp." with **Accept** and **Dismiss**.
11. **Accept.** Note colony gold, then press **Accept**.
    - [ ] The toast reads "40 gold added to the colony." with **Undo**, and colony gold rose by 40.
    - [ ] The timeline has "Coins deposited · 40 gold", and it names no party. A player can read it.
12. **Undo the deposit.** Press **Undo** on the toast (or the board's undo).
    - [ ] Colony gold drops by 40 and the deposit waits in the inbox again.
    - [ ] Only a GM sees a whisper in Foundry: "Colony Planner undid accepting <party>'s 40 gp deposit.
      The gold left the colony, and the deposit waits in the DM's inbox again: dismissing it there returns
      the coins." The player sees nothing.
    - [ ] The party's coins did not change.
13. **Dismiss the deposit.** Press **Dismiss** on it.
    - [ ] The site says "Dismissed. Foundry returns the coins to whoever paid them."
    - [ ] Within a few seconds the party gains 40 gp, as gold pieces (not the coins it paid with).
    - [ ] Only a GM sees "Colony Planner returned the 40 gp <party> deposited."
    - [ ] Reload the world: the party doesn't gain the coins again, and the whisper doesn't post again.
14. **Deposit from a character, and refund it.** Give a PF2e player character at least 20 gp worth of
    coins and note the party's coins. Right-click the character in the Actors tab, choose **Deposit to
    colony…** and deposit 12.
    - [ ] The character's coins drop by exactly 12 gp worth, and the party's coins don't change.
    - [ ] Foundry says "Queued <character name>'s 12 gp deposit for Colony Planner's inbox.", and the
      inbox reads "<character name> deposited 12 gp."
    - [ ] Press **Accept**, then **Undo**: colony gold rises by 12 and drops again, and only a GM sees
      the whisper naming the character.
    - [ ] Press **Dismiss**: within a few seconds the character, not the party, gains 12 gp as gold
      pieces, and only a GM sees "Colony Planner returned the 12 gp <character name> deposited."
    - [ ] Deposit more than the character has: "<character name> has only N gp worth of coins." and
      nothing changes.
15. **Deposit failures worth trying by hand.** Each should change nothing and lose nothing.
    - [ ] **Not enough coins.** Deposit more than the party has: "<party name> has only N gp worth of
      coins." The coins and the inbox are unchanged.
    - [ ] **Bad amounts.** Type 0, then 2.5, then 2000000, then leave it empty: each warns "Type a whole
      number of gold pieces from 1 to 1,000,000." and nothing happens. Closing the dialog does nothing.
    - [ ] **Past gold's max.** On Settings, lower colony gold's max to just above today's gold, deposit
      more than the room left, and press **Accept**: "Colony gold would pass its max of N. Raise the max on
      Settings, or dismiss the deposit to return the coins." Gold is unchanged and the item still waits.
      Dismiss it: the coins come back. Put the max back.
    - [ ] **Dismiss with no link.** Deposit 5 gp. On Settings press **Disconnect…**: the confirmation adds
      "N deposits wait in the inbox; dismissing one needs a connected world." with N the count. Disconnect, then
      **Dismiss** the deposit: "Connect Foundry first, so the coins can go back to the actor that paid." It stays in
      the inbox. Connect again with a new code, then Dismiss: the coins come back.
    - [ ] **Colony Planner is down.** Stop `mise run dev` (Control-C), deposit 5 gp, and watch the coins
      leave the party. Start it again: within a minute the deposit is in the inbox, once.
    - [ ] **The GM's tab closes while it waits.** Stop `mise run dev`, deposit 5 gp, close the GM's Foundry
      tab, start the server, and reopen the world as GM: the deposit arrives in the inbox, once.
    - [ ] **Two tabs.** Open the board in two browser tabs and press **Accept** in both: the second says
      "That Foundry inbox item was already handled."
    - [ ] Dismiss whatever deposits are left, so their coins go back.
    - [ ] **The link changes while a deposit waits.** Stop `mise run dev`, deposit 5 gp, and press
      **Disconnect** in **Configure Settings**, **Colony Planner**. Start the server, make a new code on
      Settings and connect with it. Foundry shows an error that this world's link changed before Colony
      Planner confirmed the deposit, and the deposit never reaches the inbox. The menu now lists it under
      "Deposits that stopped part way": add the 5 gp back to the party by hand, then press **Forget**, and
      the list is gone.
    - [ ] Otherwise, no "Deposits that stopped part way" list shows in that menu.
16. **A dnd5e world.** Connect the dnd5e world (update it to 0.3.1 first), then repeat steps 2 to 9 there
    with a dnd5e NPC and character and dnd5e items.
    - [ ] No actor has **Deposit to colony…**.
    - [ ] A character arrives with its level, its race and every class name ("Fighter, Wizard"), its
      Public Biography as Appearance and its full Biography as DM notes. An NPC arrives with no level.
    - [ ] An item priced in gp, sp or pp arrives with that price in gold when it is whole; one priced in
      electrum (ep) arrives with the price blank.
17. **A player's browser does nothing.** In the player's window, with the Network tab open, right-click
    actors and items in their sidebar.
    - [ ] No new menu entries, and no request to the site.

## Part A. Live: the site and your Forge world

Repeat in this order, with the live site and your Forge world. Where Part B said "Dev sign-in", sign in
normally. Use throwaway coins and test actors you don't mind seeing in your real campaign's inbox, and
dismiss or delete what you made afterwards.

1. **Upgrade.** Update Colony Bridge to 0.3.1 in Forge's **Add-on Modules** (Forge may need its own
   refresh of the module list), then reload.
   - [ ] Settings shows **Colony Bridge** 0.3.1 and "Foundry is up to date."
2. **Sends.** Part B steps 2 to 9.
   - [ ] Only the active GM sees **Send to Colony Planner**; NPCs import hidden with secrets only in DM
     notes; items fill price and **Foundry item UUID**; the description check from step 6 gives the same
     answer as locally.
3. **Deposit, accept, undo, dismiss.** Part B steps 10 to 14 on your PF2e world.
   - [ ] The coins leave the party or the character, Accept adds them to colony gold, Undo whispers the
     GMs only, and Dismiss puts them back on whichever paid, as gold pieces, once.
4. **Failures.** Part B step 15's "Not enough coins", "Past gold's max" and "Two tabs".
   - [ ] Each changes nothing and loses nothing.
5. **dnd5e and players.** Part B steps 16 and 17 on your dnd5e world.
   - [ ] No deposits in dnd5e, and a player's browser does nothing. This is the done criterion.

## Known limits

- Text only: no portraits. Formatting becomes plain text, and an enricher link like `@UUID[...]{Name}`
  becomes its label.
- Deposits are PF2e only and in whole gold pieces, and pf2e 7.12.2 runs on Foundry 13 only, so deposits
  are untested on Foundry 14. The menu entries are built for Foundry 14's field names too, but nobody has
  seen them there yet.
- A refund comes back as gold pieces, not as the coins that paid it.
- Undoing an accepted deposit leaves the coins with Colony Planner until you accept again or dismiss it.
- If Foundry stops while moving coins (the browser closes during a deposit or a refund), or the link
  changes before Colony Planner confirms a deposit, the module never guesses: the deposit is listed under "Deposits that stopped part way" in Colony Bridge's
  settings, and you fix the paying actor's coins by hand, then press **Forget**.
- Clearing Foundry's chat log can let a whisper about a deposit post again; the coins never move twice.
- A GM secret block, PF2e's GM-only text, an enricher with no label (such as `@Check[fortitude|dc:25]`)
  and an unlabelled inline roll (such as `[[/gmr 1d20]]`) never reach an item's description or an NPC's Appearance; an NPC's go to its DM notes instead.

## When you're done

- Part B only: press Control-C in the `mise run dev` tab. `mise run db:down` stops Postgres; your data is
  kept.
- Record step 6's description check in `docs/design/foundry-bridge.md` §14.11 (it says the path was not
  checked in the systems' sources).
