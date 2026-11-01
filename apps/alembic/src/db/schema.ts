import { sql } from "drizzle-orm";
import {
	boolean,
	check,
	integer,
	jsonb,
	pgTable,
	primaryKey,
	text,
	timestamp,
	uuid,
} from "drizzle-orm/pg-core";
import type {
	BrewView,
	CraftingRank,
	Mix,
	Tools,
	ViewingAs,
} from "#/brewing/types";

export const campaigns = pgTable("campaigns", {
	id: uuid().defaultRandom().primaryKey(),
	gmUserId: text("gm_user_id").notNull().unique(),
	viewingAs: text("viewing_as").$type<ViewingAs>().notNull().default("player"),
});

export const crafters = pgTable("crafters", {
	campaignId: uuid("campaign_id")
		.primaryKey()
		.references(() => campaigns.id, { onDelete: "cascade" }),
	name: text().notNull(),
	className: text("class_name").notNull(),
	level: integer().notNull(),
	intMod: integer("int_mod").notNull(),
	craftingRank: text("crafting_rank").$type<CraftingRank>().notNull(),
	alchemicalCrafting: boolean("alchemical_crafting").notNull(),
	tools: text().$type<Tools>().notNull(),
	downtimeDays: integer("downtime_days").notNull().default(0),
});

export const codexEntries = pgTable(
	"codex_entries",
	{
		campaignId: uuid("campaign_id")
			.notNull()
			.references(() => campaigns.id, { onDelete: "cascade" }),
		potionId: text("potion_id").notNull(),
		mentor: boolean().notNull().default(false),
		formed: boolean().notNull().default(false),
		revealed: boolean().notNull().default(false),
		formulaRevealed: boolean("formula_revealed").notNull().default(false),
		perfected: boolean().notNull().default(false),
		bestOffBy: integer("best_off_by"),
		bestMix: jsonb("best_mix").$type<Mix>(),
		note: text(),
	},
	(table) => [primaryKey({ columns: [table.campaignId, table.potionId] })],
);

export const brews = pgTable("brews", {
	id: uuid().defaultRandom().primaryKey(),
	campaignId: uuid("campaign_id")
		.notNull()
		.references(() => campaigns.id, { onDelete: "cascade" }),
	mix: jsonb().$type<Mix>().notNull(),
	result: jsonb().$type<BrewView>().notNull(),
	createdAt: timestamp("created_at", { withTimezone: true })
		.notNull()
		.defaultNow(),
});

export const ingredientInventory = pgTable(
	"ingredient_inventory",
	{
		campaignId: uuid("campaign_id")
			.notNull()
			.references(() => campaigns.id, { onDelete: "cascade" }),
		ingredientId: text("ingredient_id").notNull(),
		amount: integer().notNull().default(1),
	},
	(table) => [
		primaryKey({ columns: [table.campaignId, table.ingredientId] }),
		check(
			"ingredient_inventory_amount_non_negative",
			sql`${table.amount} >= 0`,
		),
	],
);
