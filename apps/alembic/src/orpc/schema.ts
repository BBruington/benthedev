import { z } from "zod";
import { CRAFTING_RANKS, TOOLS } from "#/brewing/types";

export const crafterEditSchema = z
	.object({
		level: z.number().int().min(1).max(20),
		intMod: z.number().int().min(-1).max(7),
		craftingRank: z.enum(CRAFTING_RANKS),
		alchemicalCrafting: z.boolean(),
		tools: z.enum(TOOLS),
	})
	.refine((v) => !v.alchemicalCrafting || v.craftingRank !== "untrained", {
		message: "Alchemical Crafting requires trained Crafting",
		path: ["alchemicalCrafting"],
	});
export type CrafterEdit = z.infer<typeof crafterEditSchema>;

export const brewAttemptSchema = z.object({
	mix: z.record(z.string(), z.number().int().min(1).max(4)).refine((m) => {
		const units = Object.values(m).reduce((a, b) => a + b, 0);
		return units >= 1 && units <= 4;
	}, "Pick 1-4 units"),
	d20: z.number().int().min(1).max(20).optional(),
});
export type BrewAttempt = z.infer<typeof brewAttemptSchema>;

export const viewingSetSchema = z.object({ as: z.enum(["gm", "player"]) });
export const revealSchema = z.object({
	potionId: z.string(),
	reveal: z.enum(["none", "potion", "formula"]),
});
export type Reveal = z.infer<typeof revealSchema>;

export const inventoryAdjustSchema = z.object({
	ingredientId: z.string(),
	delta: z.union([z.literal(1), z.literal(-1)]),
});
export type InventoryAdjust = z.infer<typeof inventoryAdjustSchema>;
