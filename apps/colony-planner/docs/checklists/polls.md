# Polls checklist: questions for the table

## Start it

1. Make sure Docker Desktop is running. If `mise run dev` is running, press Control-C in its tab. The server has to restart to pick up Polls.
2. **The next command erases your local data**, and a local database that ran the earlier button version of Polls needs it anyway (its migration changed in place). It loads the new sample polls, all site polls because the sample campaigns have no linked server: Sample Colony has a closed poll about the fishing boat, an open several-answer poll about festivals that closes in 3 days (Ana has voted), and an open single-answer poll about the watchtower that closes in 7 days (Ana hasn't). Second Sample has one open poll about its tunnels.

   ```bash
   cd ~/myCodeSpace/benthedev/apps/colony-planner
   mise run fresh
   mise run dev
   ```

   Wait for `dev: open http://localhost:5173`. If a page doesn't load on the first start, press Control-C and run `mise run dev` once more.

The steps build on each other, so do them in order. A private window counts as a second person; to switch who is signed in there, sign out and use **Dev sign-in** again.

## Try it

1. **The sample polls.** Sign in as **Sample DM** and open Sample Colony.
   - **Polls** is in the navigation after Shipments (on a phone, in the More sheet), and the address ends in `/polls`.
   - **Open** lists the watchtower poll first, then the festivals poll. Each card shows "Pick one" or "Pick any", the voter count, the close time, every option's count with a bar and the names of who picked it, and **Close poll** and **Delete**. You have no vote buttons.
   - **Closed** lists the fishing boat poll with its final counts and "Closed" with the date.
   - The **Colony** board has a line for each open poll with its question and a summary such as "3 votes, Beside the harbor leads", and an **All polls** link.
   - **Timeline** has a public "Poll closed" entry for the fishing boat with every option's count and no winner.
2. **A player votes on the board.** In a private window, sign in as **Ana** and open Sample Colony.
   - The board shows the watchtower poll as a vote card (Ana hasn't voted) and the festivals poll as a results line (she has).
   - Pick **The north ridge** and press **Vote**: "Vote saved.", and the card turns into the results line. The Sample DM window updates on its own, with Thorin under The north ridge.
3. **Change and withdraw.** As Ana, open **Polls**.
   - On the watchtower poll, pick **On the old shrine hill** and press **Change vote**: the counts move, and Thorin's name moves with them.
   - On the festivals poll, tick a third festival and press **Change vote**: all three show "Your vote".
   - Press **Withdraw vote** on the watchtower poll: its voter count drops by one, and the board shows its vote card again.
4. **The DM never votes.** As Sample DM, turn on **View as player**: the polls show results only, with no vote buttons. Turn it off.
5. **Start a poll.** As Sample DM, on **Polls**, press **New poll**.
   - Close the sheet with its X, then Escape, then a tap outside it: each one closes it.
   - Try to start with an empty question, one option and two identical options: each is refused with its reason. **Add option** stops at 10 options, the question stops at 300 characters and each option at 55 (Discord's limits). Every poll needs a duration, from 1 hour to 32 days.
   - Start "Who should lead the next expedition?" with three options, **Allow several answers** off, and the shortest duration, 1 hour. "Poll started." shows, focus lands on the Polls heading, and Ana sees it at once on her board and her Polls page, with a vote card (no server is linked, so it is a site poll).
6. **The open-poll limit.** Starting an 11th open poll is refused with "There are already 10 open polls. Close one before starting another." (Start throwaway ones and delete them afterwards, or skip this.)
7. **It closes on time.** As Ana, vote on the expedition poll. Wait an hour for its closing time (or skip this).
   - Both windows stop offering a vote once the time passes, and the poll moves to **Closed** (within 30 seconds on the Polls page).
   - Without Beny Bot running locally, the close is recorded by the campaign's next poll write: as Ana, vote on another open poll, and the expedition poll's "Poll closed" entry appears on the timeline.
8. **Close and delete.** As Sample DM, press **Close poll** on the watchtower poll and confirm: "Poll closed.", focus lands on its heading, it moves to Closed, the board drops it, and the timeline has its "Poll closed" entry with every count. Then press **Delete** on the fishing boat poll and confirm: it disappears in both windows, and its timeline entry stays.
9. **On a phone.** Narrow a window to 360 px (or open the site on your phone with `mise run dev:phone`). As Ana, the board's vote card, the Polls page's option cards, **Vote**, **Change vote** and **Withdraw vote** all work one-handed, and nothing scrolls sideways. As Sample DM, the New poll sheet's fields, **Add option**, the remove buttons and the duration picker work the same way.
10. **Campaigns stay apart.** Sign in as **Hollis** and open Second Sample. Its Polls page has only the tunnel poll, with Tamsin's vote: nothing from Sample Colony.
11. **Beny Bot (yours to run, with a real Discord server).** Follow the same-token rule first (`docs/cutover.md` Part 4: take the token off Railway before running it locally). Re-invite Beny Bot Dev with the invite link on Settings so it has the new **Send Polls** permission, link a test server and pick an announcements channel. Then:
    - **New poll** now makes a Discord poll: Beny Bot posts it in the announcements channel as a native Discord poll with the question, the options as answers (several answers when allowed) and its duration, under one line linking the Polls page. The site's close time changes to Discord's.
    - On the site, the poll shows **Vote in Discord** instead of a vote card (and "Waiting for Beny Bot to post this in Discord." for the moment before it is posted); the link opens the poll's message. A vote sent to the site anyway is refused with "Vote on this poll in Discord."
    - Voting in Discord as a linked player shows up on the site within a few seconds, under your member name; changing or removing it there does the same. A vote from the DM or anyone outside the campaign shows up only as "and 1 other" and in the counts, never by name.
    - Stop Beny Bot, vote in Discord, start it again: the vote reaches the site once it reconnects.
    - A poll that runs out closes in Discord and on the site, and the timeline gets its entry with the final counts, other votes included, without anyone voting on the site. **Close poll** on the site ends the Discord poll early too.
    - **Delete** removes the Discord poll's message.
    - Unlink the server (or clear the announcements channel) and start another poll: it is a site poll with a vote card, and stays one after you link again.
12. **Checks.** In a second Terminal tab, run `mise run check`. It ends with "All checks passed" after a few minutes. The secrecy sweep plants open and closed polls with votes in both sample campaigns, a Discord poll with other votes in each, and checks that no outsider or other campaign ever sees them and that another campaign's poll or option IDs are refused.

## When you're done

- Press Control-C in the `mise run dev` tab. `mise run db:down` stops Postgres; your data is kept.
