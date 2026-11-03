# Alembic agent rules

Alembic is one app in the benthedev monorepo; the repo-wide rules are in `../../CLAUDE.md`. Run everything from `apps/alembic`.

## Validate
- Run `mise run check` (Biome, typecheck, `bun test`, then the build) after every change and before reporting done. Fix what it flags; never suppress it.
- Run `mise run test:db` (needs a throwaway Postgres at `TEST_DATABASE_URL`) only when a change touches `src/server/`, `src/db/` or `drizzle/`. CI always runs it.
- Bun is pinned in `mise.toml`, and `bunfig.toml` runs Vite, tsc and drizzle-kit on Bun, so no Node install is needed.
- This app's Biome config is its own root, outside the shared `../../biome.json`.

## shadcn components
- Add components with `bunx shadcn@4.23.0 add <name>`.
- Generated files import `cn` from the `cn` npm package. Change that import to `import { cn } from "#/lib/utils";`. Biome rejects imports from `cn`.

## Browser testing
- Never sign in as Benjamin. Run `mise run dev:login` (with `mise run dev` running) to sign in as the dev Clerk user `alembic-tour`, then load `.auth/alembic-tour.json` as the browser's storage state. Rerun it when the session has expired.

## Database
- Schema lives in `src/db/schema.ts`. After changing it, run `bun run db:generate` and commit the SQL and metadata under `drizzle/`.
- Apply schema changes with `bun run db:migrate`. Never use `drizzle-kit push`, and never edit a migration that has already been applied.
