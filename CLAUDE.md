# benthedev

One repository for every app the author deploys under benthedev.com, each on its own subdomain. Every
app lives in `apps/<app>` and has its own CLAUDE.md or README: read that one before changing the app. This
file covers only what spans apps.

## Layout

| Path | Holds |
|---|---|
| `apps/colony-planner/` | Go, React (Vite) and Postgres, plus Beny Bot. Its own Go module, `github.com/BBruington/colony-planner`, and its own Bun project in `web/` |
| `apps/about-me/` | the small static personal site |
| `.github/workflows/<app>.yml` | one CI workflow per app, run only when that app's files change |
| `mise.toml` | the monorepo task root, and the default bun pin (each app pins its own too) |
| `package.json` | the Bun workspace for web apps that share code; empty until two apps share a package |
| `biome.json` | shared lint and format rules; skips `apps/colony-planner`, whose Biome config is its own root |

## Running tasks

Each app keeps its tasks in its own `mise.toml`. Either `cd apps/<app>` and `mise run <task>`, or run
`mise run //apps/<app>:<task>` from anywhere in the repo (for example `mise run //apps/colony-planner:check`).
`mise tasks ls --all` lists every app's tasks. Each app installs its own dependencies from its own lockfile
(`bun install --frozen-lockfile` in `apps/about-me`, colony's `mise run setup` for its `web/`).
In a bare shell, use `mise exec -- <tool>` for the pinned tools.

## Rules

- Each app owns its database. No shared tables, and no app reads another app's database.
- Apps talk only through small, explicit links (one narrow endpoint, a one-time code), never shared state.
- Each app has its own CLAUDE.md or README, CI workflow and Railway project. Push a change to each app
  separately (or merge it in its own PR): one push runs every matching workflow on its last commit, and
  Railway's "Wait for CI" waits on all of them.
- No committed `go.work` until one module imports another. If one is ever committed it must list every Go
  module, colony included, and Dockerfiles build with `GOWORK=off`.
- Apps stay outside the root Bun workspace, each with its own lockfile: Railway builds each app from its own
  folder, so its Dockerfile can't see a root `bun.lock`. An app joins the workspace only when it shares a
  web package, and its build then moves to the repo root.
- Never Nx.
- Pin exact versions (and Docker base images by tag and digest). No em dashes anywhere: code, docs or UI copy.
- VCS is jj, colocated with git. One conventional commit line, no body.

## Domains and Railway

Each app is its own Railway project. Settings live in each project's dashboard, since new Railway projects
ignore `railway.toml`; each app's runbook lists them.

| Host | App | Railway |
|---|---|---|
| colony.benthedev.com | colony-planner | root directory `apps/colony-planner`, watch path `/apps/colony-planner/**` |
| benthedev.com, www.benthedev.com | about-me | root directory `apps/about-me`, watch path `/apps/about-me/**` |
| potions (later, subdomain not chosen) | not built yet | |

DNS is on Cloudflare with DNSSEC on, so never change the nameservers. An app's domain is the CNAME and
`_railway-verify` TXT record Railway shows, left as DNS only (grey cloud); the apex is a flattened CNAME
beside the mail records.

## DNS records never to touch

- The apex MX, SPF and google-site-verification records: ben@benthedev.com mail on Google Workspace.
- `resend._domainkey` and the `send.` MX and SPF records: Resend email.
- party-planner's eight records (party-planner, api, staging and api-staging, each a CNAME plus a
  `_railway-verify` TXT) until colony's switch day.
