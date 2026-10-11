# Setting up Beny Bot Dev (the test Discord bot)

You only need this for checklist items marked **(Discord)**. Everyday local work doesn't need Discord:
leave the Discord lines in `.env` empty and use **Dev sign-in**. Beny Bot then stays off and the
"Sign in with Discord" button is hidden.

This guide walks you through four things, once:

1. a private test server in Discord;
2. a separate Discord application called **Beny Bot Dev**;
3. inviting Beny Bot Dev to the test server;
4. putting Beny Bot Dev's values in `.env`.

It takes about 15 minutes.

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
| `DISCORD_GUILD_ID` | Your test server's ID | Right-click the server → Copy Server ID | no |
| `DM_DISCORD_ID` | Your own Discord user ID, which makes you the DM | Right-click your name → Copy User ID | no |

Discord shows the secret and the token only once, when you reset them. Paste each one straight into
`.env` (step 6). Don't paste them anywhere else: not in a chat (AI assistants included), a commit, an
issue or a screenshot. If one leaks, reset it on the same page and update `.env`.

## 1. Turn on Developer Mode in Discord

Developer Mode adds **Copy ID** to Discord's right-click menus.

1. In the Discord app, open **User Settings** (the gear next to your name, bottom left).
2. Go to **Advanced** and switch on **Developer Mode**.

## 2. Create a private test server

1. In the server list on the left, click **+** (Add a Server).
2. Choose **Create My Own**, then **For me and my friends**.
3. Name it something like `Colony Planner Test` and click **Create**.
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

Beny Bot needs the scopes `bot` and `applications.commands`, and five permissions: **View Channels**,
**Send Messages**, **Embed Links**, **Read Message History** and **Manage Events**. Together those
permissions are the number `8590019584`.

1. Paste this address into your browser, replacing `YOUR_CLIENT_ID` with the Client ID from step 3:

   ```
   https://discord.com/oauth2/authorize?client_id=YOUR_CLIENT_ID&scope=bot+applications.commands&permissions=8590019584
   ```

   You can build the same link on **OAuth2** → **OAuth2 URL Generator**: tick `bot` and
   `applications.commands`, tick the five permissions, and choose **Guild Install**. The link it shows
   should end in `permissions=8590019584`.
2. Pick **Colony Planner Test** as the server, check that the five permissions are listed, and click
   **Authorize**.
3. Beny Bot Dev now appears in the server's member list, offline until the site runs with its token.

## 5. Copy the IDs

With Developer Mode on (step 1), right-click (or Control-click) in Discord:

- the test server's icon → **Copy Server ID**. It goes in `DISCORD_GUILD_ID`.
- your own name or avatar → **Copy User ID**. It goes in `DM_DISCORD_ID`. Whoever has this ID is the
  DM on the site.
- the `dm-desk` channel → **Copy Channel ID**. This one does **not** go in `.env`; you paste it on the
  site's Settings page in step 7. Keep it handy.

## 6. Fill in `.env`

`.env` is in the `colony-planner` folder. `mise run setup` created it, and git ignores it. Finder hides
files whose names start with a dot, so open it from your code editor.

Fill in the five lines that are already there, one value after each `=`, with no spaces or quotes:

```
DISCORD_CLIENT_ID=
DISCORD_CLIENT_SECRET=
DISCORD_BOT_TOKEN=
DISCORD_GUILD_ID=
DM_DISCORD_ID=
```

Leave every other line as it is. `PUBLIC_URL` stays `http://localhost:5173`.

## 7. Try it

1. In Terminal, in the `colony-planner` folder, run `mise run fresh`. It reloads the sample campaign
   (wiping local data) so the sample DM gets your Discord ID. Then run `mise run dev`.
2. Among the first lines, look for `beny_bot=true` and `discord_sign_in=true`. A moment later a line says
   `Beny Bot is online`.
3. Open http://localhost:5173 and click **Sign in with Discord**. You land on the Colony page as the DM.
4. Open **Settings**, paste the `dm-desk` channel ID into the DM channel field, and save. The Discord
   section also has an **Invite Beny Bot** button with the step 4 link, for inviting Beny Bot Dev again later.
5. In the test server, type `/beny`. Beny Bot Dev answers "Boy!", and Discord marks it "Only you can see
   this". Every Beny Bot reply is private like that. If `/beny` isn't offered yet, reload Discord
   (Command-R).
6. To see the waiting-to-join notice, open a private browser window and sign in with Discord as a
   **second** Discord account. That window shows "Waiting for the DM", and Beny Bot Dev posts
   "*name* (@*username*) is waiting to join: http://localhost:5173/players" in `dm-desk`. The second
   account doesn't need to be in the test server.

Only one copy of the site may run with this token at a time. Stop one `mise run dev` before starting
another.

## Turning Beny Bot off again

Empty the five Discord values in `.env` (keep the names) and restart `mise run dev`. Beny Bot goes offline,
the Discord button disappears, and Dev sign-in works as before.

## When something doesn't work

| What you see | What to check |
|---|---|
| Discord says "Invalid OAuth2 redirect_uri" | The redirect in step 3 must be exactly `http://localhost:5173/auth/discord/callback` and saved. |
| No **Sign in with Discord** button | `DISCORD_CLIENT_ID` or `DISCORD_CLIENT_SECRET` is empty, or `mise run dev` wasn't restarted after editing `.env`. |
| The first lines say `beny_bot=false` | `DISCORD_BOT_TOKEN` is empty, or `mise run dev` wasn't restarted after editing `.env`. |
| `mise run dev` stops at once with "DISCORD_GUILD_ID is required when DISCORD_BOT_TOKEN is set" | Copy the test server's ID into `DISCORD_GUILD_ID` (step 5), or empty `DISCORD_BOT_TOKEN` to keep Beny Bot off. |
| An error mentioning authentication, or `4004` | The bot token is wrong or was reset. Reset it again (step 3) and paste the new one. |
| `/beny` isn't offered | Beny Bot Dev isn't in the server (step 4), `DISCORD_GUILD_ID` is another server's ID, or Discord needs a reload (Command-R). |
| "The application did not respond" | The site isn't running, or the command came from a server other than `DISCORD_GUILD_ID`; Beny Bot ignores other servers. |
| Beny Bot replies "Sign in at http://localhost:5173 first" | That Discord account hasn't signed in on the site, or the DM hasn't approved it yet. Only approved members can use Beny Bot. |
| No waiting-to-join notice | The DM channel isn't set on Settings; Beny Bot Dev can't see `dm-desk` (if you made it private, add Beny Bot Dev to it); the account had signed in before (only a first sign-in sends one); or five notices were already sent in the last hour. The request still appears on `/players`. |
| A token or secret was pasted somewhere it shouldn't be | Reset it on the Developer Portal (step 3), update `.env`, and restart. |
