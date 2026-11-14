# Foundry bridge Phase 2 checklist: NPCs, places, quests, deliveries and polls

Phase 2 is done when the players' side of your Foundry world shows what the site has revealed, and stays
quiet about what it hasn't (`docs/design/foundry-bridge.md` §13). It sends text only, no images. Colony
Bridge is now 0.2.0. Your PF2e world (pf2e 7.12.2) gets the item delivery; your dnd5e world gets
everything except that, which is chat only there.

## Where to do it

Do Part B (local) first: it needs Foundry running on your Mac at http://localhost:30000 and
`mise run dev` running, and it costs nothing if something is wrong. Do Part A (live) after the deploy, on
your Forge world, at https://colony.benthedev.com.

The steps build on each other, so do them in order. Only the active GM's browser runs the bridge. You need
two browsers: the GM's, and a private window logged in to the same world as a player. Keep the GM logged
in the whole time: switching that window to the player leaves no GM, so nothing reaches Foundry. In Part B, sign in
with **Dev sign-in** as Sample DM and open Sample Colony. The console is Cmd+Option+K in Firefox,
Cmd+Option+J in Chrome, and Cmd+Option+I then Console in Foundry's own app.

## Part B. Local: your Mac and a local Foundry

1. **Upgrade and reconnect.** Restart `mise run dev` so the server has the new code. In Foundry, open
   **Add-on Modules** and update Colony Bridge to 0.2.0, then reload the world. On the site's **Settings**,
   look at the **Foundry** card.
   - [ ] **Colony Bridge** shows 0.2.0 under the world details.
   - [ ] If a "Foundry couldn't apply these:" list shows, press **Clear**. Phase 2 rows fail on an old
     module and failed rows are never retried, so clear them before reconnecting.
   - [ ] Press **Disconnect…** and confirm, then **Make a new code** (or **Connect Foundry**) and connect
     as in Phase 1: in **Configure Settings**, **Colony Planner**, **Connect**, type the code into
     **Pairing code**. A link made before 0.1.2 needs a new code anyway.
2. **The backfill.** Before this step, have on the site at least one revealed NPC, one revealed place and
   one revealed quest, and one hidden of each. Connecting in step 1 sends them. In Foundry's Journal tab:
   - [ ] Three folders exist: "Colony NPCs", "Colony Places" and "Colony Quests".
   - [ ] Each revealed NPC, place and quest is an entry in its folder, with a page named "Colony Planner".
   - [ ] In the player's window the same entries open and read fully.
   - [ ] None of the hidden NPCs, places or quests appear, for you or the player.
3. **Reveal an NPC.** On the site, create an NPC, or take a hidden one, and reveal it.
   - [ ] A new entry appears in "Colony NPCs" within a few seconds, with no reload, and the player can
     open it.
4. **Rename its known name.** Give an NPC whose real name differs a **known name** (the name players
   see), then change the known name again.
   - [ ] The entry's name and heading update to the new known name.
   - [ ] The true name appears nowhere in the entry, its page, or any quest that names this NPC.
5. **Hide the NPC.** Hide it on the site.
   - [ ] The player can no longer open the entry, and it leaves their Journal list.
   - [ ] As GM you still see the entry: it was not deleted. It is named "Hidden on Colony Planner", its
     "Colony Planner" page is empty, and any page you added to it is still there.
   - [ ] Reveal it again: the player sees it again, with the latest text.
6. **A place.** Reveal a place, rename it, hide it.
   - [ ] It appears in "Colony Places", the rename shows, and after hiding the player cannot open it but
     the entry is still in your Journal.
   - [ ] An NPC whose current place is that place shows "Now at" with the new name, and after hiding the
     place it no longer names it.
7. **A quest.** Reveal a quest, then **Complete** it, then reopen it and **Fail** it.
   - [ ] It appears in "Colony Quests". After Complete the entry shows Status "Completed" and the day
     under Resolved. After Fail it shows "Failed".
   - [ ] No failure cost, no failure favor and no DM notes appear in the entry, even after failing.
   - [ ] Hide it: the player cannot open it, and you still can.
8. **A hidden giver.** Make a quest revealed whose giver is a hidden NPC.
   - [ ] The quest's entry has no "Given by" line, and the hidden NPC's name appears nowhere in it.
   - [ ] Reveal the NPC: the quest entry now shows "Given by" with the NPC's name. Hide the NPC: it goes
     again.
9. **The next game night.** Schedule a game night on the site's Sessions page, then look at the Colony
   journal entry's page.
   - [ ] It shows "Next game night:" with the date, the start and end times and the time zone name.
10. **A poll opens.** On the site, create a poll with a few options.
    - [ ] Foundry gets a chat message from "Colony Planner": "New poll:", the question, the options,
      "Pick one" or "Pick any", when it closes, and "Vote on Colony Planner" (or "Vote in Discord").
11. **A poll closes.** Add a few votes, then press **Close** on the poll.
    - [ ] Foundry gets "Poll closed:" with the question and each option's vote count in order.
    - [ ] It names no winner and no voter.
12. **Set an item's UUID.** In Foundry, open a PF2e compendium (for example the equipment pack) and open an
    item. Copy its UUID: click the ID badge in the sheet header, or right-click it, and choose to copy the
    UUID. It looks like `Compendium.pf2e.equipment-srd.Item.` followed by 16 letters and numbers. On the
    site, open the market, edit a special item, and paste it into **Foundry item UUID**. Save. Also keep
    one special item with the field empty.
    - [ ] The item saves with no error. A malformed UUID is refused with a message under the field.
13. **Order and pay (PF2e world).** Make sure the world has a party actor. Order 2 of the item with the
    UUID and 1 of the item without one, dock the ship, then **Pay**.
    - [ ] The party actor's inventory gains the item, quantity 2.
    - [ ] A chat message from "Colony Planner" reads "Goods unloaded on Day N, paid G gold:" with the
      goods, then "Added to" the party's name "'s inventory:" listing the item, then "Hand these out
      yourself:" listing the one with no UUID.
    - [ ] Supplies and colonists in the same order are not in Foundry at all.
14. **Undo the payment.** On the site, undo the payment (**Undo**).
    - [ ] Only a GM sees a whisper: "A payment was undone on Colony Planner:" and "If any of these reached
      the party, remove them by hand:" with the goods.
    - [ ] The party's inventory is unchanged: nothing was deleted. Remove the item yourself.
15. **A dnd5e world.** Repeat steps 1 and 2, then 13 on the dnd5e world, connecting it with its own code.
    - [ ] Entries, polls and the game night work the same.
    - [ ] The delivery is a chat message only, with every item under "Hand these out yourself:", and no
      actor changed.
16. **A player's browser does nothing.** In the player's window, with the Network tab open, do something on
    the site that sends a row (such as step 3).
    - [ ] The player's window makes no request to the site. No delivery is added twice, and no duplicate
      chat messages appear while both browsers are open.
17. **Retry.** Close the GM's Foundry tab, reveal an NPC and create a poll on the site, then reopen the world.
    - [ ] Within a minute the entry and the chat message arrive, and Settings says "Foundry is up to date."

## Part A. Live: the site and your Forge world

Repeat in this order, with the live site and your Forge world. Where Part B said "Dev sign-in", sign in
normally. Use test content you don't mind players seeing, since the live backfill shows your real revealed
NPCs, places and quests.

1. **Upgrade and reconnect.** Update Colony Bridge to 0.2.0 in Forge's **Add-on Modules** (Forge may need
   its own refresh of the module list), reload, press **Clear** on Settings if failed rows show, then
   **Disconnect…** and connect again.
   - [ ] Settings shows **Colony Bridge** 0.2.0 and "Foundry is up to date."
2. **Backfill, reveal, rename, hide.** Part B steps 2 to 5.
   - [ ] The three folders exist, revealed things are readable by a player, hidden ones never appear.
   - [ ] A known-name change updates the entry, the true name is never shown, and hiding keeps the entry
     for you while the player loses it.
3. **Place, quest, hidden giver.** Part B steps 6 to 8.
   - [ ] Quest status shows; no failure cost or DM notes; a hidden giver is never named.
4. **Game night and polls.** Part B steps 9 to 11.
   - [ ] The Colony page shows the next game night; a poll opening posts a chat message; closing posts
     counts naming no winner.
5. **Delivery and undo, PF2e.** Part B steps 12 to 14 on your PF2e world.
   - [ ] The item lands in the party actor's inventory with the right quantity, chat lists the goods, goods
     without a UUID are listed in chat only, and undo whispers the GMs and deletes nothing.
6. **dnd5e and players.** Part B steps 15 and 16 on your dnd5e world.
   - [ ] Deliveries are chat only, and a player's browser does nothing.
7. **Catch up.** Part B step 17.
   - [ ] Settings says "Foundry is up to date." after reopening the world. This is the done criterion.

## Known limits

- Text only: no portraits, no images, no map pins or scene Notes.
- A withdrawn (hidden) entry is renamed "Hidden on Colony Planner" and its bridge page is blanked, so it
  holds no Colony Planner text even if a player's browser receives it. Pages you added to it keep their
  text, and players' own levels on it are set to None, so give them back by hand if you want them.
- Clearing Foundry's chat log can let a one-shot message (a poll, a delivery, a day change) post again, since
  the module finds what it already posted by looking at the chat log.
- Party items added by a delivery don't record their compendium source, so Foundry can't link them back to
  the compendium entry.
- The module never deletes anything. Removing an entry, an item or a folder is yours to do by hand.
- Renaming a resource or a colony role does not update an NPC's "Colony role" or a quest's "Reward" until
  that NPC or quest is saved again.

## When you're done

- Part B only: press Control-C in the `mise run dev` tab. `mise run db:down` stops Postgres; your data is
  kept.
- If Phase 1's step 16 result was never recorded in `docs/design/foundry-bridge.md` §9.1, record it: it
  tells whether the pages you add to a withdrawn entry stay out of players' browsers.
