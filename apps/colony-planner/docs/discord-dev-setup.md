# Setting up Beny Bot Dev (the test Discord bot)

You only need this for checklist items marked **(Discord)**. Everyday local work doesn't need Discord:
leave the Discord lines in `.env` empty and use **Dev sign-in**. Beny Bot then stays off and the
"Sign in with Discord" button is hidden.

Beny Bot is **one shared bot** for every campaign on the site. Each DM invites it to their own Discord
server and links that server to their campaign with a one-time code and `/link`. Locally you test all of
it with one private test server.

This guide walks you through five things, once:

1. a private test server in Discord;
2. a separate Discord application called **Beny Bot Dev**;
3. inviting Beny Bot Dev to the test server;
4. putting Beny Bot Dev's values in `.env`;
5. linking the test server to Sample Colony with `/link`.

It takes about 15 minutes. Coming from M1? Read "Coming from M1" first.

> **Never use the real Beny Bot here.** Don't use the production Beny Bot application, its bot token or its
> client secret, and don't copy anything from the old party-planner `.env` files. The old Beny Bot keeps
> serving your real table, untouched. If two running programs share one bot token, both answer every
> command. Beny Bot Dev is only for testing and never becomes the production bot.

## What you'll collect

| `.env` name | What it is | Where you find it | Secret? |
|---|---|---|---|
| `DISCORD_CLIENT_ID` | Beny Bot Dev's application ID | Developer Portal → OAuth2 | no |
| `DISCORD_CLIENT_SECRET` | Lets the site finish a Discord sign-in | Developer Portal → OAuth2 → Reset Secret | **yes** |
| `DISCORD_BOT_TOKEN` | Beny Bot Dev's password for Discord | Developer Portal → Bot → Reset Token | **yes** |
| `SITE_OWNER_DISCORD_ID` | Your own Discord user ID, which makes you the site owner (you approve new campaigns) | Right-click your name → Copy User ID | no |

There is no server ID and no channel ID to collect: the server is linked with `/link`, and channels are
picked from a list on the campaign's Settings page.

Discord shows the secret and the token only once, when you reset them. Paste each one straight into
`.env` (step 6). Don't paste them anywhere else: not in a chat (AI assistants included), a commit, an
issue or a screenshot. If one leaks, reset it on the same page and update `.env`.

## Coming from M1

M1.5 names the site owner with `SITE_OWNER_DISCORD_ID` and replaces M1's `DISCORD_GUILD_ID`.

1. Put your own Discord user ID in `SITE_OWNER_DISCORD_ID` in `.env` (step 5 says how to copy it).
2. **Keep `DISCORD_GUILD_ID` for one start of Beny Bot.** M1 registered `/beny` in that one server only;
   now Beny Bot registers its commands for every server. On the first `mise run dev` with Beny Bot on,
   it clears the old server-only `/beny` and logs `old guild commands cleared; delete DISCORD_GUILD_ID
   from .env`. Without this step your test server would show `/beny` twice.
3. After you see that line, delete the `DISCORD_GUILD_ID` line from `.env`. Nothing else reads it.

Then skip to step 4 of "Try it" below: your application, token and invite already exist. Only the invite
permissions changed (step 4 below); your existing invite keeps working, and you can re-invite Beny Bot
Dev with the new link to drop the permission it no longer needs.

## 1. Turn on Developer Mode in Discord

Developer Mode adds **Copy ID** to Discord's right-click menus.

1. In the Discord app, open **User Settings** (the gear next to your name, bottom left).
2. Go to **Advanced** and switch on **Developer Mode**.

## 2. Create a private test server

1. In the server list on the left, click **+** (Add a Server).
2. Choose **Create My Own**, then **For me and my friends**.
3. Name it something like `Colony Planner Test` and click **Create**. As its creator you have the
   **Manage Server** permission, which `/link` needs.
4. Make a text channel for the DM's notices: next to **Text Channels** click **+**, name it `dm-desk`,
   and click **Create Channel**. Only you are in this server, so it doesn't need to be private.

## 3. Create the Beny Bot Dev application

1. Open https://discord.com/developers/applications and sign in with your Discord account.
2. Click **New Application**, name it `Beny Bot Dev`, accept the terms, and click **Create**.
3. Optional: on **General Information**, give it an icon so you can tell it apart from the real Beny Bot.

### OAuth2 page: client ID, secret and redirect

1. Open **OAuth2** in the left sidebar.
2. Under **Client information**, copy the **Client ID**. It goes in `DISCORD_CLIENT_ID`.
3. Click **Reset Secret**, confirm, and copy the new secret. It goes in `DISCORD_CLIENT_SECRET`.
4. Under **Redirects**, click **Add Redirect** and enter exactly:

   ```
   http://localhost:5173/auth/discord/callback
   ```

   Then click **Save Changes**. It must match character for character: `http` (not `https`),
   `localhost` (not `127.0.0.1`), port `5173` (the address you open the site at, which is
   `PUBLIC_URL` in `.env`), and no slash at the end.

   If you ever try Discord sign-in on the one-binary preview at http://localhost:8080, add
   `http://localhost:8080/auth/discord/callback` as a second redirect, and set `PUBLIC_URL` to
   `http://localhost:8080` while you do.

### Bot page: the token

1. Open **Bot** in the left sidebar.
2. Click **Reset Token**, confirm (Discord may ask for your password or two-factor code), and copy the
   token. It goes in `DISCORD_BOT_TOKEN`.
3. Leave all three **Privileged Gateway Intents** switched off. Beny Bot doesn't need them.
4. Leave **Requires OAuth2 Code Grant** switched off.
5. Recommended: switch off **Public Bot** so nobody else can add Beny Bot Dev to their server. If Discord
   won't save that, open **Installation**, set **Install Link** to **None**, save, and try again.

## 4. Invite Beny Bot Dev to the test server

Beny Bot needs the scopes `bot` and `applications.commands`, and four permissions: **View Channels**,
**Send Messages**, **Embed Links** and **Manage Events**. Together those permissions are the number
`8589954048`. It doesn't ask for **Read Message History**: no feature reads messages, and the site owner,
who runs Beny Bot, must not be able to read a campaign's server through it.

1. Paste this address into your browser, replacing `YOUR_CLIENT_ID` with the Client ID from step 3:

   ```
   https://discord.com/oauth2/authorize?client_id=YOUR_CLIENT_ID&scope=bot+applications.commands&permissions=8589954048
   ```

   You can build the same link on **OAuth2** → **OAuth2 URL Generator**: tick `bot` and
   `applications.commands`, tick the four permissions, and choose **Guild Install**. The link it shows
   should end in `permissions=8589954048`. Every DM's Settings page shows this same link (with the
   Client ID filled in) under **Discord server**.
2. Pick **Colony Planner Test** as the server, check that the four permissions are listed, and click
   **Authorize**.
3. Beny Bot Dev now appears in the server's member list, offline until the site runs with its token.

## 5. Copy your user ID

With Developer Mode on (step 1), right-click (or Control-click) your own name or avatar in Discord →
**Copy User ID**. It goes in `SITE_OWNER_DISCORD_ID`. Whoever has this ID is the site owner: they approve
new campaigns on Site admin, and `mise run fresh` makes them the DM of Sample Colony.

## 6. Fill in `.env`

`.env` is in the `colony-planner` folder. `mise run setup` created it, and git ignores it. Finder hides
files whose names start with a dot, so open it from your code editor.

Fill in the four lines that are already there, one value after each `=`, with no spaces or quotes:

```
DISCORD_CLIENT_ID=
DISCORD_CLIENT_SECRET=
DISCORD_BOT_TOKEN=
SITE_OWNER_DISCORD_ID=
```

Leave every other line as it is. `PUBLIC_URL` stays `http://localhost:5173`.

## 7. Try it

1. In Terminal, in the `colony-planner` folder, run `mise run fresh`. It reloads the two sample campaigns
   (wiping local data) so the sample DM gets your Discord ID. Then run `mise run dev`.
2. Among the first lines, look for `beny_bot=true` and `discord_sign_in=true`. A moment later a line says
   `Beny Bot is online`. Beny Bot's commands are registered for every server it is in; Discord can take a
   minute to show new ones.
3. Open http://localhost:5173 and click **Sign in with Discord**. You land on Sample Colony as its DM, and
   the profile menu has **Site admin**.
4. **Link the server.** In Sample Colony open **Settings** → **Discord server** → **Get a link code**. In
   the test server, type `/link` and paste the code into its `code` box (`/link code:BENY-XXXXX-XXXXX`).
   Beny Bot answers "Linked this server to **Sample Colony**. Pick channels in the campaign's Settings."
   A code links one server, within 15 minutes; a refused `/link` doesn't use it up. A restart of the site
   forgets it, so get a new one if Beny Bot says it doesn't work.
   Only members with **Manage Server** see `/link` and `/unlink`.
5. Settings now shows "Linked to Colony Planner Test". Pick `dm-desk` from the **DM channel** list and
   save.
6. In the test server, type `/beny`. Beny Bot Dev answers "Boy!", and Discord marks it "Only you can see
   this". Every Beny Bot reply is private like that. If `/beny` isn't offered yet, wait a minute and
   reload Discord (Command-R).
7. To see the waiting-to-join notice, copy Sample Colony's invite link (**Settings** → **Invite link** →
   **Copy**), open a private browser window, sign in as a new person (Dev sign-in → "A new person", or
   a **second** Discord account), open the link and tap **Ask to join**. Beny Bot Dev posts "*name*
   (@*username*) is waiting to join: http://localhost:5173/c/sample-colony/players" in `dm-desk`.
8. To see the site owner's notice, have a new person ask to run a campaign (**Run your own campaign**).
   Beny Bot Dev sends you a direct message: "*name* (@*username*) asks to run *campaign*:
   http://localhost:5173/admin". Discord delivers it because you share the test server with Beny Bot Dev.
9. To unlink, use **Unlink…** on Settings, or run `/unlink` in the server. Removing Beny Bot Dev from the
   server unlinks it too, even while the site is stopped: it notices on its next start. Re-inviting it
   then needs a new `/link`.

Only one copy of the site may run with this token at a time. Stop one `mise run dev` before starting
another.

## Turning Beny Bot off again

Empty `DISCORD_CLIENT_ID`, `DISCORD_CLIENT_SECRET` and `DISCORD_BOT_TOKEN` in `.env` (keep the names) and
restart `mise run dev`. Beny Bot goes offline, the Discord button disappears, and Dev sign-in works as
before. `SITE_OWNER_DISCORD_ID` can stay: it only names the site owner. A linked server stays linked
while Beny Bot is off; its channel lists are empty until it is back.

## When something doesn't work

| What you see | What to check |
|---|---|
| Discord says "Invalid OAuth2 redirect_uri" | The redirect in step 3 must be exactly `http://localhost:5173/auth/discord/callback` and saved. |
| No **Sign in with Discord** button | `DISCORD_CLIENT_ID` or `DISCORD_CLIENT_SECRET` is empty, or `mise run dev` wasn't restarted after editing `.env`. |
| No **Site admin** in the profile menu | `SITE_OWNER_DISCORD_ID` isn't your user ID, or `mise run dev` wasn't restarted after editing `.env`. |
| The first lines say `beny_bot=false` | `DISCORD_BOT_TOKEN` is empty, or `mise run dev` wasn't restarted after editing `.env`. |
| An error mentioning authentication, or `4004` | The bot token is wrong or was reset. Reset it again (step 3) and paste the new one. |
| `/beny` or `/link` isn't offered | Beny Bot Dev isn't in the server (step 4), or Discord hasn't shown the new commands yet: wait a minute and reload Discord (Command-R). `/link` and `/unlink` are only offered to members with Manage Server. |
| `/beny` is offered twice | M1's server-only command is still there. Put `DISCORD_GUILD_ID` back for one start (see "Coming from M1"). |
| "The application did not respond" | The site isn't running, or it was started without the bot token. |
| "Use Beny Bot in your campaign's server." | The command was sent in a direct message. Run it in the linked server. |
| "This server isn't linked to a campaign yet." | Link it: step 4 of "Try it". |
| "That code doesn't work." | The code expired (15 minutes), already linked a server, or the site restarted since you got it. Get a new one on Settings. |
| "This server is already linked to a campaign." | Unlink it in that campaign's Settings, or run `/unlink` in the server, then run `/link` again with the same code if it hasn't expired. |
| "Unlink the other server in Settings first." | This campaign is linked to another server. Unlink the other server (on Settings, or `/unlink` there), then run `/link` again with the same code if it hasn't expired. |
| "Only someone with the Manage Server permission can do that." | Ask the server's owner, or give your role Manage Server. |
| "You're not in this campaign." | That Discord account has no membership in the campaign linked to this server. Open the campaign's invite link with it and have the DM let it in. |
| "The DM hasn't let you in yet." | The account is waiting or was removed. The DM lets it in on Players. |
| "This campaign is paused by the site owner." | The campaign is suspended on Site admin. |
| No waiting-to-join notice | The server isn't linked; no DM channel is picked on Settings; Beny Bot Dev can't see `dm-desk` (if you made it private, add Beny Bot Dev to it); the person had already asked (opening the link again pings nobody); or five notices were already sent for this campaign in the last hour. The request still appears on Players. |
| The channel lists on Settings are empty | Beny Bot is off, or the server isn't linked, or it has no text channels Beny Bot can see. |
| No direct message about a campaign request | `SITE_OWNER_DISCORD_ID` is empty or isn't your user ID; your Discord privacy settings block direct messages from members of the test server; or five were already sent in the last hour. The request still appears on Site admin. |
| A token or secret was pasted somewhere it shouldn't be | Reset it on the Developer Portal (step 3), update `.env`, and restart. |
