# Market checklist: the party's shipment orders

## Start it

1. Make sure Docker Desktop is running. If `mise run dev` is running, press Control-C in its tab. The server has to restart to pick up the Market.
2. **The next command erases your local data.** The market's migration changed in place, so a database that already has it must be rebuilt. The command loads the new sample data: Sample Colony's market sells Food, Materials and colonists for every role (Mages at 80 gp), plus Boots of Striding (1 per order), has a few lines in the party cart, and one order at sea.

   ```bash
   cd ~/myCodeSpace/benthedev/apps/colony-planner
   mise run fresh
   mise run dev
   ```

   Wait for `dev: open http://localhost:5173`. If a page doesn't load on the first start, press Control-C and run `mise run dev` once more.

The steps build on each other, so do them in order. A private window counts as a second person; to switch who is signed in there, sign out and use **Dev sign-in** again.

## Try it

1. **The sample market.** Sign in as **Sample DM** and open Sample Colony.
   - **Shipments** is in the Colony group after People (on a phone, in the More sheet). The page's title is "Shipments" with "Order additional supplies from the Mainland" under it, and the address ends in `/shipments`.
   - **Ship(s)** has one card: the order is at sea, the day it docks and how many days that is, its lines, and the gold due on arrival beside the gold the colony has. It has **Cancel order**.
   - **Catalog** has three sections: **Supplies** (Food, 2 food for 1 gp; Materials, 1 for 5 gp), **Colonists** (every role, 30 gp each, Mage 80 gp) and **Special items** (Boots of Striding, 1 per order). Each section's title folds it away and opens it again (all start open). Supplies and colonists each have a number box and **Add**; the Boots have **Add**. Each priced row has an edit button, Special items has **New special item**, and the header has **Shipping · 14 days, 1 ship**.
   - The cart is titled **Party Shipment Draft**.
   - The **Colony** board's day dial reads "Next shipment arrives at Day N", its arc filled by how far the ship has come. There is no Market link under it.
2. **A player sees the same, without DM tools.** In a private window, sign in as **Ana** and open Sample Colony.
   - **Shipments** shows the same ships, draft and catalog, with no Cancel order, New special item, edit or Set price buttons, archived list or Shipping button.
   - **Place order** is disabled with "The ship is out. The next docks Day N (in N days). Pay for it there, or ask the DM to cancel one, to order again."
3. **Cancel the sample order.** As Sample DM, press **Cancel order** and confirm.
   - Ship(s) says no ship is ordered and orders take 14 days, and focus lands on the Shipments heading. The board's dial goes back to the supply ships' line ("no ships scheduled" when none are).
   - Open **New special item** and close it with its X, then Escape, then **Cancel**, then a tap outside it: each one closes it.
   - Ana's window updates on its own. The cart is unchanged.
   - **Timeline** has a public "Order cancelled" entry.
4. **The DM's tools.** As Sample DM:
   - Open **Shipping** and set the voyage to **3** days. 0 and 3651 are refused, and so are 0 and 101 ships.
   - **New special item**: "Boots of Striding II" at 120 gp, **Most per order** 1. The sheet has only Name, Description, Price in gold and Most per order. It lists under Special items, and Ana sees it at once.
   - Edit **Healer**'s price to 40 gp: it reads "40 gp each". Try a negative price (refused).
   - Edit **Food**: set 4 food per purchase for 3 gp, then put it back to 2 for 1 gp. 0 food per purchase is refused.
   - **Stop selling** Mage: it leaves the catalog, in both windows, and Ana no longer sees it. Its row shows **Set price** for you only; set it to 80 gp and Ana sees it again.
   - In People, create a role "Fisher". The catalog lists Fisher under Colonists at 30 gp each right away.
5. **Fill the cart.** As Ana, type **500** in Food's box: "250 gp" shows beside it before you press Add. Type **501** instead: "Food comes in lots of 2." and nothing is added. Add 500 food (the cart already had 10, so its line reads "510 food"), type **1** under Farmer and press Enter. The cart already holds the Boots, whose **+** stops at 1.
   - Food's cart box is prefilled "510"; change it to 500 and press **Update** (disabled until the number changes). The trash button removes a line.
   - The cart shows each line's total, the order's total and "Last changed by Ana", and your Sample DM window follows along.
6. **Place the order.** As Ana, press **Place order**.
   - The sheet lists the lines, the total and "Docks Day N, in 3 days". If the colony has less gold than the total, an ember note says how much it will need; confirming still works.
   - Confirm. The cart empties, the ship card says it's at sea and docks in 3 days, and the board's day dial says the same.
   - **Timeline**: a public "Order placed: …" entry naming the lines and the docking day.
   - Fill the draft again and try **Place order**: it's disabled. (A second confirm from a stale sheet is refused with "The ship is still out. Pay for it at the docks or cancel it before placing another order.")
   - As Sample DM, open **Shipping** and set **Ships** to 2: the header reads "Shipping · 3 days, 2 ships" and Ana's Place order works again. Place the second order as Ana: Ship(s) lists both, soonest first, the board's dial still follows the first, and Place order is disabled with "All 2 ships are out. …". Cancel the second order as Sample DM and set Ships back to 1.
7. **The ship docks.** As Sample DM, open **Advance** and advance 3 days.
   - The preview has the line "The goods ship docks on Day N."
   - After advancing, the timeline has "Goods ship docked · N gold due" on that day, the ship card says it's docked, and the board's dial reads "Shipment has arrived and is awaiting payment" with a full arc.
8. **Short on gold.** As Sample DM, use **Adjust** to set gold below the order's total.
   - As Ana, press **Pay**: the sheet shows gold, food and Farmers before and after, a box "Not enough gold: the colony has …, needs …", and Pay is disabled.
   - Advance one more day as Sample DM: Advance isn't blocked, and the ship keeps waiting.
9. **Pay.** As Sample DM, put gold back above the total. As Ana, open **Pay** again and pay.
   - Gold drops by the total, food rises by the food in the order (capped at its max if it would pass it), and **Farmers** has one more colonist (not Unassigned).
   - **Timeline**: a public "Goods unloaded · paid N gold" entry naming the food in units, the Farmer and the Boots.
   - Ship(s) is empty, **Past orders** lists the order as paid, and Ana can place a new order.
   - Ana's toast has no Undo.
10. **Undo the payment.** On your Sample DM board, the box under the clock says "Last change: Goods unloaded · paid N gold". Tap **Undo** and confirm.
    - Gold, food and Farmers are back, and the order is back at the docks with the gold due, in both windows.
    - As Ana, pay again: the ship is free.
    - As Ana, place a new order. Then, as Sample DM, tap **Undo** under the clock (the last change is still that payment): it is refused with "The ship is still out. Pay for it at the docks or cancel it before undoing this payment." Cancel the new order to finish.
11. **On a phone.** Narrow a window to 360 px (or open the site on your phone with `mise run dev:phone`). The catalog's and cart's number boxes, **Add** and **Update**, the Boots' **−** and **+**, Place order and the Pay sheet all work one-handed, every control sits centered in its row, and nothing scrolls sideways.
12. **Campaigns stay apart.** Sign in as **Hollis** and open Second Sample. Its market has only the starting offers (Food, Materials where it has that resource, and its own roles at 30 gp), an empty cart and no ship: no Boots, nothing from Sample Colony.
13. **Checks.** In a second Terminal tab, run `mise run check`. It ends with "All checks passed" after a few minutes. The secrecy sweep now plants market items, an archived item, a cart and an open order in both sample campaigns, and checks that no outsider or other campaign's DM ever sees them and that players never see an archived item.

## When you're done

- Press Control-C in the `mise run dev` tab. `mise run db:down` stops Postgres; your data is kept.
