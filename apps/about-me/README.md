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
