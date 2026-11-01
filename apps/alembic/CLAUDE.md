# Alembic agent rules

Alembic is one app in the benthedev monorepo; the repo-wide rules are in `../../CLAUDE.md`. Run everything from `apps/alembic`.

## Validate
- Run `mise run check` (Biome, typecheck, `bun test`, then the build) after every change and before reporting done. Fix what it flags; never suppress it.
- Bun is pinned in `mise.toml`, and `bunfig.toml` runs Vite, tsc and drizzle-kit on Bun, so no Node install is needed.
- This app's Biome config is its own root, outside the shared `../../biome.json`.

## shadcn components
- Add components with `bunx shadcn@4.23.0 add <name>`.
- Generated files import `cn` from the `cn` npm package. Change that import to `import { cn } from "#/lib/utils";`. Biome rejects imports from `cn`.

## Database
- Schema lives in `src/db/schema.ts`. After changing it, run `bun run db:generate` and commit the SQL and metadata under `drizzle/`.
- Apply schema changes with `bun run db:migrate`. Never use `drizzle-kit push`, and never edit a migration that has already been applied.
