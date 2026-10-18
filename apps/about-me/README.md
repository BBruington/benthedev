# about-me

Ben's personal site at https://benthedev.com and https://www.benthedev.com: one static page built with
Vite, React and Tailwind, prerendered into `dist/index.html` at build time and served by Caddy.

## Commands

Run from this folder, or from anywhere in the repo as `mise run //apps/about-me:<task>`.

| Task | What it does |
| --- | --- |
| `mise run dev` | Vite dev server (client-rendered; the prerender runs only in `build`) |
| `mise run build` | Static site in `dist/` |
| `mise run lint` | `biome ci .` |
| `mise run typecheck` | `tsc --noEmit` |
| `mise run check` | lint and typecheck, then build |

Run `bun install --frozen-lockfile` once first. Fonts (IBM Plex Sans, IBM Plex Mono, Newsreader) are
self-hosted from Fontsource, so the page loads nothing from another origin.

## Deploy (Railway)

Its own Railway project, configured in the dashboard:

- Root directory `apps/about-me`, builder Dockerfile, watch path `/apps/about-me/**`.
- Custom domains `benthedev.com` and `www.benthedev.com`.
- Caddy listens on `$PORT` (8080 when unset) and sets the cache and security headers in `Caddyfile`.
  Its CSP allows only this origin, so a new external script, font or image needs a CSP change too.
- Wait for CI on, so a red about-me workflow blocks the deploy.

## Go live

Every step touches Railway, Cloudflare, Vercel or GitHub, so the author does them by hand, after the
first deploy is up.

1. Railway: the service's **Settings** → **Networking** → **Custom Domain**, add `benthedev.com` and
   `www.benthedev.com`. Railway shows a CNAME and a `_railway-verify` TXT record for each.
2. Cloudflare (`benthedev.com` → **DNS** → **Records**): delete Vercel's apex and `www` records, then add
   Railway's four records exactly as shown, each **DNS only** (grey cloud). The apex CNAME sits beside
   the mail records and Cloudflare flattens it. Change nothing else: the MX, SPF,
   google-site-verification, `resend._domainkey` and `send.` records and the nameservers stay.
3. Once Railway shows both domains with a green tick, open https://benthedev.com and
   https://www.benthedev.com, then send a mail to ben@benthedev.com from another account and check it
   arrives.
4. Remove both domains from the old Vercel project and disconnect its Git repository. On GitHub, change
   `BBruington/about-me`'s homepage (today `https://about-me-beryl-pi.vercel.app`) to
   https://benthedev.com, then archive that repository.
