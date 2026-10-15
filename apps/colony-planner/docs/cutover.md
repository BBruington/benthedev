# Cutover runbook: Colony Planner goes live

This is your step-by-step guide for moving the table from party-planner to Colony Planner. Every step
here touches a production account or a secret, so every step is yours: agents never sign in to Railway,
Neon or the Discord Developer Portal, and never see a production value.

**The plan, in order** (SPEC.md §10 "M6", as decided on 2026-10-14):

0. Merge M6 into `main` (Part 0): Railway builds `main`, and only M6 has the `Dockerfile`.
1. Prepare: a Neon database, a Railway service, its settings, and Beny Bot Dev's redirect (Parts 1–4).
2. Go live on Railway's own address (`https://<something>.up.railway.app`), set up your campaign there,
   and run a test session with your private test server (Parts 5–9).
3. Switch day: move your domain from party-planner to the new service, stop the old site and the old
   bot, and move Beny Bot to your real Discord server (Parts 10–12).
4. Afterwards: retire party-planner over 30 days and keep an eye on Neon's usage (Parts 13–14).

The old party-planner site and the old Beny Bot keep serving your table, untouched, until Part 10.
Budget about two hours for Parts 1–9 (most of it entering your colony) and an hour for switch day.

**Two rules for every step:**

- **Never paste a secret into a chat**, an AI tool, an issue or a commit. Secrets go from where they're
  made straight into Railway. The secrets here are the Neon connection string, the Discord client secret
  and the Discord bot token.
- **One bot token, one running Beny Bot.** The live site uses your existing development application,
  **Beny Bot Dev**, for now. If your Mac's `mise run dev` and the Railway service both run with its token,
  both answer every command and both run their planners against different databases: duplicate events,
  reminders and replies. Part 4 says exactly how to keep that from happening.

---

## Part 0. Merge M6 into `main`

Railway builds whatever is on `main`, and the `Dockerfile`, the production checks and Go 1.27.2 arrive
with M6. Built from an older `main`, Railway would guess how to build the app on its own: no website in
the binary and no production mode.

1. On GitHub, merge the M6 pull request into `main`.
2. Open the repository's **Actions** tab and wait until the run on `main` shows all four checks green
   (lint, typecheck, docker, check).

**Success looks like:** `main` on GitHub has a `Dockerfile` at the top, and its latest commit has a green
tick.

## Part 1. The Neon database

You'll create a new Neon project with one database called `colony` on Postgres 17, and copy its
**direct** connection string.

1. Open https://console.neon.tech and sign in. Click **New Project**.
2. Fill in:
   - **Project name:** `colony-planner`.
   - **Postgres version:** **17**.
   - **Region:** the one closest to where you'll run Railway. Pick the same side of the same continent
     for both (for example Neon's **AWS US East 1 (N. Virginia)** with Railway's **US East (Virginia)**).
     Every page view asks the database a few things, so distance shows up as slowness.
3. Click **Create**. Neon makes a default database called `neondb`; this app wants its own.
4. In the left sidebar open **Databases** (or **Branches** → **main** → **Roles & Databases**), click
   **New database**, name it `colony`, keep the owner Neon suggests (`neondb_owner`), and create it.
5. Click **Connect** (top of the project dashboard). In the dialog:
   - **Branch:** `main`; **Database:** `colony`; **Role:** `neondb_owner`.
   - Switch **Connection pooling** **off**. The app must use the direct connection: it locks the database
     while it upgrades itself at start-up, and a pooled connection can't hold that lock.
   - Copy the connection string (the copy button). Don't paste it anywhere but Railway (Part 3).
   - In Railway's value field (Part 3), before saving, change `sslmode=require` to
     **`sslmode=verify-full`**, and keep `&channel_binding=require` (add it if it's missing). With
     `require`, the server would accept any certificate, so someone on the network between Railway and
     Neon could pose as the database and read its password. The live server refuses to start without
     `verify-full`.

   **Success looks like:** the string starts `postgresql://neondb_owner:`, its host starts `ep-` and does
   **not** contain `-pooler`, the database name before the `?` is `colony`, not `neondb`, and once edited
   it ends `/colony?sslmode=verify-full&channel_binding=require`.
6. Leave the compute settings at Neon's defaults. On the free plan the compute **scales to zero** after
   5 minutes with nothing to do; that is what keeps the site free between sessions.

**If it fails:** Neon refuses a database named `colony` only if one exists already; use that one. If the
string contains `-pooler`, the pooling switch is still on.

## Part 2. The Railway service

1. Open https://railway.com and sign in. You're on the **Hobby** plan (one small always-on server).
2. Click **New Project** → **Deploy from GitHub repo**. If Railway asks, give its GitHub app access to the
   private repository `BBruington/colony-planner` (only that one is enough).
3. Pick `BBruington/colony-planner`. Railway finds the `Dockerfile` at the top of the repository and
   starts a first build at once. **That first deploy fails** (it has no settings yet). That's expected;
   carry on.
4. Click the new service (the box on the project's canvas) and open **Settings**. Railway no longer
   reads `railway.toml` for a new service, so set these by hand (the file keeps the same values on record),
   from top to bottom:
   - **Source:** the branch is `main`, and **Wait for CI** is **on**, so a push to `main` deploys only
     after the GitHub checks pass. (The switch appears once GitHub has run the checks on `main` at least
     once.)
   - **Networking → Public Networking:** click **Generate Domain**. If Railway asks for a port, type
     `8080`. Copy the address it shows, such as `https://colony-planner-production.up.railway.app`. This
     is your **Railway address** until switch day.
   - **Build → Builder:** **Dockerfile**, with the Dockerfile path `Dockerfile`.
   - **Deploy → Regions:** the region next to your Neon database (Part 1).
   - **Deploy → Replicas:** **1**. Never raise it: Beny Bot, its timers, the sign-in cache and the live
     updates all live inside the one server, and a second copy would run a second Beny Bot.
   - **Deploy → Healthcheck Path:** `/healthz`, with a timeout of `120` seconds. Railway checks it only
     while a new version starts, and keeps the old version serving if the new one never answers.
   - **Deploy → Restart Policy:** **On Failure**, at most `10` retries.
   - **Serverless** (also called App Sleeping): **off**. A sleeping server can't send reminders or create
     Discord events on time.

## Part 3. The environment variables

Railway keeps the server's settings as variables. Open the service → **Variables** and add each one with
**New Variable** (name on the left, value on the right). Type the names exactly.

| Name | Value | Where it comes from |
|---|---|---|
| `DATABASE_URL` | the direct connection string, ending `sslmode=verify-full&channel_binding=require` | Neon, Part 1 step 5. **Secret.** |
| `PUBLIC_URL` | your Railway address, such as `https://colony-planner-production.up.railway.app` | Part 2 step 4. `https://`, no slash at the end. On switch day it becomes your domain. |
| `PORT` | `8080` | Type it. The port the server listens on, and the one you gave **Generate Domain**. |
| `DISCORD_CLIENT_ID` | Beny Bot Dev's Client ID | Developer Portal → Beny Bot Dev → **OAuth2** → **Client ID**. Not a secret. |
| `DISCORD_CLIENT_SECRET` | Beny Bot Dev's client secret | **Secret.** See below. |
| `DISCORD_BOT_TOKEN` | Beny Bot Dev's bot token | **Secret.** See below. |
| `SITE_OWNER_DISCORD_ID` | your own Discord user ID (17 to 20 digits) | Discord → right-click your name → **Copy User ID**. The same as in `.env`. Not a secret. |

There is nothing for you to make up: the app has no signing key or cookie secret of its own (each
sign-in is a random token the server creates and stores only as a fingerprint). If you ever need a
random secret for something else, `openssl rand -base64 32` in Terminal makes one; paste it straight
into Railway.

**The two Discord secrets.** They are already in your Mac's `.env`, because Beny Bot Dev is the
application you test with. Either copy each one from `.env` (open it in your editor, copy the part after
the `=`, paste it into Railway), or make a new one in the Developer Portal: **OAuth2** → **Reset Secret**
for the client secret, **Bot** → **Reset Token** for the token. A reset ends the old value everywhere, so
if you reset one, paste the new value into `.env` as well.

Leave out `TEST_DATABASE_URL` and `POSTGRES_PASSWORD`: they're for your Mac only. `APP_ENV` is already
`production` inside the image, so it needs no variable.

When all seven are in, Railway shows your changes waiting at the top of the canvas. Click **Deploy**
(Railway may call it **Apply changes**). Don't do that until Part 4 is done if `mise run dev` is running
on your Mac with the bot token.

## Part 4. Beny Bot Dev: the live redirect and the same-token rule

### The redirect

1. Open https://discord.com/developers/applications → **Beny Bot Dev** → **OAuth2**.
2. Under **Redirects**, keep `http://localhost:5173/auth/discord/callback` and click **Add Redirect**:

   ```
   https://<your Railway address without https://>/auth/discord/callback
   ```

   For example `https://colony-planner-production.up.railway.app/auth/discord/callback`. Click **Save
   Changes**. It must match `PUBLIC_URL` character for character, plus `/auth/discord/callback`.
3. On **Bot**, **Public Bot** can stay off while Beny Bot only joins servers you own. Before another DM
   invites Beny Bot to their own server, switch it **on**, or Discord refuses their invite.
4. Optional: players will see the bot's name in your real server. On **Bot**, you can change its
   **Username** from "Beny Bot Dev" to "Beny Bot".

### The same-token rule

While Railway has `DISCORD_BOT_TOKEN`, your Mac's `mise run dev` must run **without** it. Discord sign-in
on your Mac keeps working (that uses the client ID and secret, which both places can share).

**Turn the bot off on your Mac** (do this now, before the first live deploy):

1. Stop `mise run dev` if it's running (Control-C in its tab).
2. Open `.env` in your editor. Find the line that starts `DISCORD_BOT_TOKEN=`. Type `# ` at the very start
   of that line, so the app ignores it but you keep the token. Under it, add a new line:

   ```
   DISCORD_BOT_TOKEN=
   ```

   Save.
3. Run `mise run dev`. **Success looks like:** the `api |` line "Colony Planner is listening" ends with
   `beny_bot=false`.

Typing `DISCORD_BOT_TOKEN= mise run dev` does **not** work: mise reads `.env` and its value wins over
anything typed in front of the command. Edit `.env` as above.

**When you want to test Beny Bot on your Mac** (away from game days, because your real table has no Beny
Bot while you do this: no reminders, no new events, and commands answered from your Mac's sample data):

1. Railway → the service → **Variables** → the **⋮** next to `DISCORD_BOT_TOKEN` → **Remove**, then
   **Deploy**. Wait until the new deployment says **Active** and its log line "Colony Planner is listening"
   shows `"beny_bot":false`.
2. In `.env`, delete the empty `DISCORD_BOT_TOKEN=` line and the `# ` in front of the real one. Save and
   restart `mise run dev`: its line shows `beny_bot=true`.

**Switching back** (the same day): in `.env`, put the `# ` back and the empty line under it, and restart
`mise run dev` (`beny_bot=false`). Then in Railway → **Variables** → **New Variable**
`DISCORD_BOT_TOKEN`, paste the token (from the commented line in `.env`), **Deploy**, and check the log
shows `"beny_bot":true`. Discord can take a minute to offer the slash commands again.

Never run the bot in both places, even for a minute. If it happens, every command gets two answers;
stop `mise run dev` and the duplicates stop.

## Part 5. The first live deploy, and reading its logs

1. With Part 4 done, click **Deploy** in Railway (if you haven't yet). Open the service → **Deployments**
   → the newest one. It moves through **Building**, **Deploying** and **Active**. The build takes a few
   minutes.
2. Click **View logs**. **Build Logs** show the image being built; **Deploy Logs** show the server.
   **Success looks like**, in Deploy Logs:
   - lines with `"msg":"migration applied"`, one per migration file (only on the first deploy; later
     deploys apply only new ones);
   - then `"msg":"Colony Planner is listening"` with `"dev_sign_in":false`, `"discord_sign_in":true`
     and `"beny_bot":true`.
3. Open your Railway address. The sign-in page shows "Sign in with Discord" over the Eldari art, and no
   Dev sign-in panel.

**If it fails**, the last lines of Deploy Logs say why. Some messages also carry a hint for your Mac,
such as "locally, `mise run …`": ignore it here, and never run a `mise` command to fix the live site.

| The log says | Fix |
|---|---|
| `APP_ENV must be one of` | Someone added an `APP_ENV` variable with another value: remove it (the image sets `production`). |
| `DATABASE_URL is not set` or `PUBLIC_URL is not set` | That variable is missing or its name is misspelled in Railway (Part 3). |
| `PUBLIC_URL must be an https address` | `PUBLIC_URL` must start with `https://`. |
| `SITE_OWNER_DISCORD_ID is not set` or `must be a Discord user ID` | Add it: 17 to 20 digits, nothing else. |
| `DATABASE_URL must check the database's certificate` | Change `sslmode=require` to `sslmode=verify-full` in `DATABASE_URL` (Part 1 step 5). |
| `connect to the database` or `DATABASE_URL:` | The connection string is wrong or incomplete. Copy it from Neon again (Part 1 step 5). If it worked before, check Neon's status page. |
| `records migrations this build doesn't have` (or another line about a migration), then the server stops | Copy the line (it contains no secret) and ask for help. |
| the deployment says **Failed**, "Healthcheck failed", and no "listening" line | The server didn't start; scroll up for its first error line. If "listening" is there, the port is wrong: `PORT` must be `8080`, the same number as the domain's port (Settings → Networking). |
| the deploy is **Active**, but the page says "This site answers only at its own address." | `PUBLIC_URL` isn't exactly the address in Settings → Networking: `https://`, the same name, no slash at the end. Fix it and **Deploy**. |
| Build Logs end in an error | Ask for help with the last 20 lines of Build Logs (they contain no secret). |

After the first deploy, a failed one leaves the previous version serving. The very first has nothing to
fall back on, so the site stays down until a deploy succeeds.

## Part 6. First sign-in, and your own campaign

1. On your Railway address, click **Sign in with Discord**, and **Authorize**. You land on **All
   campaigns**, which says there's nothing yet.
   - "Invalid OAuth2 redirect_uri" from Discord: the redirect in Part 4 doesn't match `PUBLIC_URL`
     exactly.
   - You land back on the sign-in page with "Discord didn't finish signing you in": check that
     `DISCORD_CLIENT_SECRET` in Railway is the current secret.
2. Open the menu behind your picture (top right). **Site admin** is there: `SITE_OWNER_DISCORD_ID` worked.
3. **Run your own campaign** → fill in the campaign's name and its address. **The address is part of every
   link and can never change after this**, so choose it carefully. Send it. As the site owner your own
   request is approved at once: the page says your campaign is ready, and you're its DM.
4. Open it. **Site admin** lists it as active, with you as its DM.

## Part 7. Enter your colony's real state

Production starts empty: there is no import and no sample data (`colony seed` refuses to run there). Enter
the colony through the normal screens. Each change lands on the timeline as a **DM-only** entry, so
players won't see the setup. If you make a mistake, **Undo** on the board takes back the latest change.

1. **Settings:**
   - **Campaign:** the campaign name and the **time zone** your table plays in.
   - **Roles:** add or rename roles to match your colony.
   - **Event location:** what the Discord event shows ("Online" until you change it).
2. **Colony board:**
   - **Correct day** → the colony's current day.
   - **Adjust stock** → for each of gold, food, materials and morale, pick **Set to** (instead of
     **Change by**) and type your number.
   - **Shipments** → the interval and the next arrival day.
3. **People:** set each role's headcount with **Set count**, and the rest in Unassigned.
4. **NPCs:** add each NPC. They start hidden; reveal the ones the party has met. For a named colonist,
   choose **promote an existing** member of their role if you already counted them in step 3, or **as a
   new colonist** if not.
5. **Places**, **Quests** and **Patrons** (with each patron's actions and the party's favor). Quests start
   visible, patrons and actions hidden.
6. **Maps:** **Upload** your Mainland. Eldari is in this repository at
   `~/myCodeSpace/colony-planner/web/public/eldari.webp` (in the file picker press Command-Shift-G and
   paste that path). Name it "Eldari", make it the **Mainland**, and pin your places on it.
7. **Sessions:** **Schedule** your series (weekly or every other week, the day and the start time in the
   table's time zone). Leave its **Discord event** and **Reminder** switches as you want them for the real
   game; until switch day they post only into your private test server.

**Success looks like:** the board matches your notes, and **View as player** (profile menu) shows players
what they should see and nothing hidden.

## Part 8. Link your private test server, for now

Your real Discord server gets Beny Bot on switch day (Part 12), once the old bot is stopped, so the table
never sees two bots posting. Until then, link the private test server.

1. In Discord, invite Beny Bot Dev to the test server if it isn't there (it is if you followed
   `docs/discord-dev-setup.md`).
2. On the live site: **Settings** → **Discord server** → **Get a link code**. In the test server type
   `/link` and paste the code. The Settings card shows the server as linked.
3. Pick the **Announce**, **Reminder** and **DM** channels on Settings, and save.

**Success looks like:** within a few seconds your series' next date has an event under **Events** in the
test server. It's fine that your Mac's Sample Colony is linked to the same test server: only the place
with the token acts on it.

**If it fails:** `/link` says the code is unknown → codes last 15 minutes and are lost when the server
restarts; get a new one. No events → check the series' **Discord event** switch and the channels.

## Part 9. A test session on the Railway address

Run this with a friend, or with a second Discord account of your own in a private window.

1. They open your campaign's invite link (**Settings** → **Invite link** → **Copy**), sign in with
   Discord and ask to join. You approve them on **Players**, and their page opens without a refresh.
2. **Advance** a day on the board: their window shows the change within a couple of seconds.
3. They ask a patron for an action; you **Grant** it from the request prompt in the test server's DM
   channel.
4. **Schedule** a one-off session 23 hours from now with both switches on: exactly one reminder appears in
   the test server, and Railway → **Deployments** → ⋮ → **Restart** doesn't send a second one.
5. In Discord, **Start** that session's event, **Advance** a day, then **End** it: the session shows its
   colony days and the timeline shows its band.
6. `/colony` and `/nextsession` in the test server match the website.
7. **Settings** → **Calendar link** → **Copy**, and in Google Calendar (on the web) **Other calendars** →
   **+** → **From URL** → paste. The sessions appear (Google can take a few hours to fetch it). You'll
   re-add it with the new address on switch day.
8. On a phone, open the Railway address: Safari's **Share** → **Add to Home Screen** (or Chrome's **Install
   app**) works over https.

Anything wrong here is cheap to fix: the real table is still on party-planner.

## Part 10. Switch day: move your domain

The old site stops answering at your domain in step 2; the new one answers a few minutes later.

1. Decide the address, for example `https://<your domain>` (the one party-planner uses today, so bookmarks
   keep working).
2. **Free the domain from party-planner.** In Railway, open the old party-planner project → its web
   service → **Settings** → **Networking** → the custom domain → the trash icon. (If party-planner's domain
   is set up somewhere other than Railway, remove it there.)
3. **Give it to the new service.** In the new project → the service → **Settings** → **Networking** →
   **Custom Domain** → type your domain → port `8080` → **Add**. Railway shows the DNS record(s) to create:
   usually a **CNAME** whose value ends in `.up.railway.app`, sometimes also a **TXT** record to prove the
   domain is yours. Keep that panel open.
4. **At your registrar** (where you bought the domain), open its DNS settings. Edit the record that points
   your domain at the old party-planner service so its value is the new one Railway shows, and add the TXT
   record if there is one. Save.
5. **Railway → Variables → `PUBLIC_URL`** → change it to `https://<your domain>` (no slash at the end) →
   **Deploy**. The live site answers only at `PUBLIC_URL`'s address, so until this deploy is **Active** your
   domain shows "This site answers only at its own address.", and afterwards the Railway address does.
6. Wait. Railway's domain panel turns green and issues the https certificate once the record has spread,
   usually within minutes, sometimes an hour.
7. **Discord Developer Portal → Beny Bot Dev → OAuth2 → Redirects → Add Redirect**
   `https://<your domain>/auth/discord/callback` → **Save Changes**. Remove the Railway-address redirect
   once everything works.

**Success looks like:** `https://<your domain>` shows the new sign-in page; **Sign in with Discord** lands
you on your campaign as DM; and `https://<your domain>/auth/dev` answers "No such route." (there is no Dev
sign-in in production). Everyone, you included, signs in again at the new address (a sign-in belongs to
one address), and calendar subscriptions and home-screen icons are re-added from the new address.

**If it fails:** a certificate warning or the old site → the DNS record hasn't spread yet, or still points
at the old target; check it at the registrar. "This site answers only at its own address." at your
domain → `PUBLIC_URL` isn't the domain yet, or that deploy isn't **Active**. "Invalid OAuth2
redirect_uri" → step 7 doesn't match `PUBLIC_URL`.

## Part 11. Stop party-planner and the old bot

1. In Railway, open the old party-planner project. For each running service (the web app, and the Go API,
   which runs the old Beny Bot): **Deployments** → the active deployment's **⋮** → **Remove**. This stops it
   but keeps the service, its settings and its history; **Redeploy** brings it back if you ever need to
   roll back.
2. In your real Discord server, the old Beny Bot shows as offline within a minute or two.
3. Delete the old recurring event: **Events** at the top of the channel list → the old event → **⋯** →
   **Delete Event** (or **Cancel Event**).
4. Remove the old Beny Bot from the server: **Server Settings** → **Integrations** → **Beny Bot** →
   **Remove** (or right-click it in the member list → **Kick**). That also removes its old commands.

**Success looks like:** typing `/` in the real server no longer offers `/schedule`, `/registercampaign` or
`/available`, and they stay gone after the new service restarts.

## Part 12. Beny Bot in your real server

1. On the live site, **Settings** → **Discord server** → **Unlink…** the test server and confirm. Its
   upcoming events there are deleted.
2. Invite Beny Bot Dev to your real server with the invite link on the same card (you need **Manage
   Server** there). Check the four permissions it lists (View Channels, Send Messages, Embed Links, Manage
   Events) and **Authorize**.
3. **Get a link code**, then `/link` with it in the real server. Pick the **Announce**, **Reminder** and
   **DM** channels on Settings and save. The DM channel should be one only you can read: requests and new
   players are posted there.
4. Share your campaign's invite link with your players. Each signs in with Discord and asks to join, and
   you approve them on **Players**.

**Success looks like:** the next session has exactly one Discord event in the real server, it gets exactly
one reminder, and `/colony` answers there.

## Part 13. Retire party-planner (30 days)

Keep the stopped old services and the old Neon database for **30 days**, so you can roll back. Put a
reminder in your calendar for 30 days after switch day.

**Now (switch day)**, end the old secrets that leaked into the old repository's history. Some live in
other companies' accounts (Clerk, Resend, Google), so deleting the old Railway and Neon projects later
doesn't end them:

1. **Old Neon credentials:** in Neon, open the **old** party-planner project → **Roles** (or **Branches** →
   **Roles & Databases**) → its role → **Reset password**. Don't copy the new password anywhere: nothing
   needs it.
2. **Old bot token and client secret:** in the Developer Portal, open the **old** Beny Bot application →
   **Bot** → **Reset Token**, then **OAuth2** → **Reset Secret**. Don't copy either new value. The old ones
   now work nowhere.
3. **Clerk:** in the Clerk dashboard, open the party-planner application → **API Keys** → roll (or delete)
   the secret key, and under **Webhooks** delete the endpoint that pointed at the old site.
4. **Resend:** in the Resend dashboard → **API Keys** → delete the key party-planner used.
5. **Google:** in Google Cloud Console → **APIs & Services** → **Credentials** → the OAuth client
   party-planner used → reset its secret, or delete the client if nothing else uses it.

**To roll back after this** (the old services are still there): get a fresh connection string from the
old Neon project, reset the old Beny Bot's token again, and paste both into the old services' variables
(along with new Clerk, Resend and Google values if the old site still needs them); then **Redeploy** them
and move the domain back. The old Beny Bot and Beny Bot Dev must never be in your real server at the same
time, so remove Beny Bot Dev from it first.

**On day 30:**

6. In Railway, delete the old party-planner project (its **Settings** → **Danger** → **Delete project**).
   That also deletes its variables, including the RSA keypair the old site used for its cookies.
7. In Neon, delete the old project (its **Settings** → **Delete project**).
8. In Clerk, delete the party-planner application (its **Settings** → **Delete application**). Clerk
   keeps every old user's profile until you do.
9. In Resend, remove your domain (**Domains**) if nothing else sends mail from it.
10. Delete the RSA key files (`*.pem`) and `.env` files from your local party-planner folder, or the whole
    folder if you no longer want it. Never push its jj keep-refs anywhere.
11. Optionally delete the old Beny Bot application in the Developer Portal (**General Information** →
    **Delete App**).

## Part 14. Keeping an eye on Neon

The free plan includes **100 CU-hours** of compute a month and **0.5 GB** of storage per project. The
site is built so the database sleeps whenever nobody is doing anything.

- **Usage:** Neon console → your `colony-planner` project → **Usage** (or **Billing** → **Usage**) shows
  this month's CU-hours and storage. Check it a few days after switch day, then monthly.
- **Sleeping overnight:** leave a player's tab open overnight (a phone that locks, or a laptop that
  sleeps). In the morning, **Monitoring** (or the compute on **Branches** → **main**) shows the compute
  **Idle**/**Suspended** for most of the night. Then try the costly case once: a desktop tab left
  **visible** overnight on a screen that stays on. Note the CU-hours **Usage** adds that night (expect
  about 1); that measurement, not a guess, decides whether the membership cache is needed.
- **What wakes it:** someone using the site, Beny Bot answering a command, and Beny Bot's own timers
  (events and reminders). Open tabs that are hidden, sleeping phones and the live-update heartbeats don't.
  A tab left **visible on a screen that stays on** reconnects every 10 minutes and wakes the database for
  about 5 minutes each time: roughly 1 CU-hour a night for each such tab.
- **If CU-hours head past 100** (Usage shows more than about 3 a day on quiet days), ask for the
  membership cache (SPEC.md §10 "As built in M6"), which removes that reconnect's database read, or
  move to Neon's paid plan.
- **Storage** grows mostly with uploaded images (each campaign may upload up to 50 MB).

Railway: the project's **Usage** page shows the month's cost; the Hobby plan's monthly fee includes $5 of
usage, which one small server normally fits.

---

## Appendix: deleting someone's account

The privacy page (`/privacy`) promises that the site owner deletes an account on request. There's no
button for it yet, so you do it in Neon's **SQL Editor** (left sidebar; choose the `colony` database).

1. If the person runs a campaign, they delete it first (its **Settings** → **Delete this campaign**), or
   you suspend and delete it on **Site admin**.
2. Find them, and check exactly one row comes back:

   ```sql
   SELECT id, display_name, discord_username FROM account WHERE discord_username = 'their-discord-username';
   ```

3. Paste this, with their username, and **Run**. It deletes their sign-ins and campaign requests, takes them
   out of every campaign (as if they'd left, with no character name), and replaces their Discord details
   on the account with "Former member". What they wrote into campaigns stays part of those campaigns'
   stories, and so do traces of who they were: their display name in timeline entries already written
   with it (a patron action called in while they had no character name), their Discord ID on a server
   link they made with `/link`, and their name in Beny Bot's past messages in Discord.

   ```sql
   WITH target AS (
     SELECT id FROM account WHERE discord_username = 'their-discord-username'
   ), ended_sign_ins AS (
     DELETE FROM web_session WHERE account_id IN (SELECT id FROM target)
   ), withdrawn_requests AS (
     DELETE FROM campaign_request WHERE account_id IN (SELECT id FROM target)
   ), left_campaigns AS (
     UPDATE campaign_member
     SET status = 'REMOVED', left_on_own = true, character_name = NULL
     WHERE account_id IN (SELECT id FROM target) AND role = 'PLAYER'
   )
   UPDATE account
   SET discord_user_id = 'deleted:' || id, discord_username = 'deleted',
       display_name = 'Former member', avatar_url = NULL, last_seen_at = NULL
   WHERE id IN (SELECT id FROM target)
   RETURNING id;
   ```

   It answers with one ID. If it answers with none, the username didn't match.
4. Railway → the service → **Deployments** → **⋮** → **Restart**, so the server forgets their sign-in (it
   keeps sign-ins in memory for up to a day).

If they sign in with Discord again later, they get a brand-new account.
