import { ORPCError } from "@orpc/server";
import { and, desc, eq, sql } from "drizzle-orm";
import {
	INGREDIENTS,
	MENTOR_NOTE,
	POTIONS,
	STARTER_POTION_ID,
	WREN,
} from "#/brewing/catalog";
import {
	crafterModifier,
	deconstructDc,
	degreeOf,
	potionDc,
	primaryOf,
	resolveBrew,
} from "#/brewing/engine";
import { maskTotals } from "#/brewing/knowledge";
import {
	type AnswerKeyEntry,
	type BrewRecord,
	type BrewView,
	type Check,
	type DeconstructView,
	type Inventory,
	TRADITIONS,
	type ViewingAs,
} from "#/brewing/types";
import { db } from "#/db/index";
import {
	brews,
	campaigns,
	codexEntries,
	crafters,
	ingredientInventory,
	potionInventory,
	traditionKnowledge,
} from "#/db/schema";
import type {
	BrewAttempt,
	CrafterEdit,
	Deconstruct,
	InventoryAdjust,
	Reveal,
} from "#/orpc/schema";
import { assertEnoughIngredients } from "./inventory";
import {
	knowledgeFrom,
	toCrafterView,
	toIngredientViews,
	toWorkshopView,
} from "./projection";

function startingInventory(campaignId: string) {
	return INGREDIENTS.map((ingredient) => ({
		campaignId,
		ingredientId: ingredient.id,
		amount: 1,
	}));
}

export async function bootstrap(userId: string) {
	const existing = await db.query.campaigns.findFirst({
		where: eq(campaigns.gmUserId, userId),
	});
	if (existing) return existing;
	return db.transaction(async (tx) => {
		await tx
			.insert(campaigns)
			.values({ gmUserId: userId })
			.onConflictDoNothing({ target: campaigns.gmUserId });
		const campaign = await tx.query.campaigns.findFirst({
			where: eq(campaigns.gmUserId, userId),
		});
		if (!campaign) throw new ORPCError("INTERNAL_SERVER_ERROR");
		await tx
			.insert(crafters)
			.values({ campaignId: campaign.id, ...WREN })
			.onConflictDoNothing();
		await tx
			.insert(codexEntries)
			.values({
				campaignId: campaign.id,
				potionId: STARTER_POTION_ID,
				mentor: true,
				note: MENTOR_NOTE,
			})
			.onConflictDoNothing();
		await tx
			.insert(ingredientInventory)
			.values(startingInventory(campaign.id))
			.onConflictDoNothing();
		return campaign;
	});
}

export async function getWorkshop(userId: string) {
	const campaign = await bootstrap(userId);
	const [crafter, entries, log, stock, potionStock, knowledge] =
		await Promise.all([
			db.query.crafters.findFirst({
				where: eq(crafters.campaignId, campaign.id),
			}),
			db
				.select()
				.from(codexEntries)
				.where(eq(codexEntries.campaignId, campaign.id)),
			db
				.select()
				.from(brews)
				.where(eq(brews.campaignId, campaign.id))
				.orderBy(desc(brews.createdAt), desc(brews.id))
				.limit(50),
			db
				.select()
				.from(ingredientInventory)
				.where(eq(ingredientInventory.campaignId, campaign.id)),
			db
				.select()
				.from(potionInventory)
				.where(eq(potionInventory.campaignId, campaign.id)),
			db
				.select()
				.from(traditionKnowledge)
				.where(eq(traditionKnowledge.campaignId, campaign.id)),
		]);
	if (stock.length < INGREDIENTS.length) {
		// Campaigns created before inventory existed get the fresh-campaign stock.
		await db
			.insert(ingredientInventory)
			.values(startingInventory(campaign.id))
			.onConflictDoNothing();
	}
	const inventory: Inventory = Object.fromEntries(
		INGREDIENTS.map((ingredient) => [
			ingredient.id,
			stock.find((row) => row.ingredientId === ingredient.id)?.amount ?? 1,
		]),
	);
	if (!crafter) throw new ORPCError("INTERNAL_SERVER_ERROR");
	const recovered = await Promise.all(
		entries.map(async (entry) => {
			if (!entry.formed || entry.bestMix || entry.bestOffBy === null)
				return entry;
			const [best] = await db
				.select({ mix: brews.mix })
				.from(brews)
				.where(
					and(
						eq(brews.campaignId, campaign.id),
						sql`${brews.result}->>'kind' = 'formed'`,
						sql`${brews.result}->>'potionId' = ${entry.potionId}`,
						sql`(${brews.result}->>'offBy')::int = ${entry.bestOffBy}`,
					),
				)
				.orderBy(desc(brews.createdAt))
				.limit(1);
			return { ...entry, bestMix: best?.mix ?? null };
		}),
	);
	return toWorkshopView({
		viewingAs: campaign.viewingAs,
		isGm: campaign.gmUserId === userId,
		crafter,
		entries: recovered,
		log: log.map((row) => ({
			id: row.id,
			createdAt: row.createdAt.toISOString(),
			mix: row.mix,
			result: row.result,
		})),
		inventory,
		potionStock: Object.fromEntries(
			potionStock.map((row) => [row.potionId, row.amount]),
		),
		knowledge: knowledgeFrom(knowledge),
	});
}

// Undiscovered potions stay unknown to every route, so stock and deconstruction can't reveal them.
async function assertPotionInCodex(
	executor: Pick<typeof db, "select">,
	campaignId: string,
	potionId: string,
) {
	const [entry] = await executor
		.select({
			mentor: codexEntries.mentor,
			formed: codexEntries.formed,
			revealed: codexEntries.revealed,
		})
		.from(codexEntries)
		.where(
			and(
				eq(codexEntries.campaignId, campaignId),
				eq(codexEntries.potionId, potionId),
			),
		);
	if (!entry || !(entry.mentor || entry.formed || entry.revealed))
		throw new ORPCError("BAD_REQUEST", { message: "Unknown potion" });
}

function rollDie(sides: number): number {
	const range = 0x100000000;
	const limit = range - (range % sides);
	const buffer = new Uint32Array(1);
	do {
		crypto.getRandomValues(buffer);
	} while (buffer[0] >= limit);
	return (buffer[0] % sides) + 1;
}

export async function attemptBrew(
	userId: string,
	input: BrewAttempt,
): Promise<BrewView> {
	if (
		Object.keys(input.mix).some(
			(id) => !INGREDIENTS.some((ingredient) => ingredient.id === id),
		)
	) {
		throw new ORPCError("BAD_REQUEST", { message: "Unknown ingredient" });
	}
	const campaign = await bootstrap(userId);
	return db.transaction(async (tx) => {
		const [crafter] = await tx
			.select()
			.from(crafters)
			.where(eq(crafters.campaignId, campaign.id))
			.for("update");
		if (!crafter) throw new ORPCError("INTERNAL_SERVER_ERROR");
		await tx
			.insert(ingredientInventory)
			.values(startingInventory(campaign.id))
			.onConflictDoNothing();
		const stock = await tx
			.select()
			.from(ingredientInventory)
			.where(eq(ingredientInventory.campaignId, campaign.id))
			.for("update");
		assertEnoughIngredients(
			input.mix,
			Object.fromEntries(stock.map((row) => [row.ingredientId, row.amount])),
		);
		for (const [ingredientId, units] of Object.entries(input.mix)) {
			await tx
				.update(ingredientInventory)
				.set({ amount: sql`${ingredientInventory.amount} - ${units}` })
				.where(
					and(
						eq(ingredientInventory.campaignId, campaign.id),
						eq(ingredientInventory.ingredientId, ingredientId),
					),
				);
		}
		const result = resolveBrew({
			mix: input.mix,
			crafter,
			d20: input.d20 ?? rollDie(20),
			tableRoll: input.d20 !== undefined,
			rollD3: () => rollDie(3),
		});
		const record: BrewRecord = {
			...result,
			potionName:
				result.kind === "inert"
					? null
					: (POTIONS.find((potion) => potion.id === result.potionId)?.name ??
						null),
		};
		await tx
			.insert(brews)
			.values({ campaignId: campaign.id, mix: input.mix, result: record });
		await tx
			.update(crafters)
			.set({ downtimeDays: sql`${crafters.downtimeDays} + 1` })
			.where(eq(crafters.campaignId, campaign.id));
		if (result.kind === "formed") {
			await tx
				.insert(codexEntries)
				.values({
					campaignId: campaign.id,
					potionId: result.potionId,
					formed: true,
					perfected: result.perfect,
					bestOffBy: result.offBy,
					bestMix: input.mix,
				})
				.onConflictDoUpdate({
					target: [codexEntries.campaignId, codexEntries.potionId],
					set: {
						formed: true,
						perfected: sql`${codexEntries.perfected} OR ${result.perfect}`,
						bestOffBy: sql`LEAST(${codexEntries.bestOffBy}, ${result.offBy})`,
						bestMix: sql`CASE WHEN ${codexEntries.bestOffBy} IS NULL OR ${result.offBy} < ${codexEntries.bestOffBy} OR (${codexEntries.bestMix} IS NULL AND ${result.offBy} = ${codexEntries.bestOffBy}) THEN excluded.best_mix ELSE ${codexEntries.bestMix} END`,
					},
				});
			if (result.quantity > 0) {
				await tx
					.insert(potionInventory)
					.values({
						campaignId: campaign.id,
						potionId: result.potionId,
						amount: Math.min(999, result.quantity),
					})
					.onConflictDoUpdate({
						target: [potionInventory.campaignId, potionInventory.potionId],
						set: {
							amount: sql`least(999, ${potionInventory.amount} + ${result.quantity})`,
						},
					});
			}
		}
		const knowledge = await tx
			.select()
			.from(traditionKnowledge)
			.where(eq(traditionKnowledge.campaignId, campaign.id));
		const ingredients = toIngredientViews({
			viewingAs: campaign.viewingAs,
			knowledge: knowledgeFrom(knowledge),
			modifier: crafterModifier(crafter),
		});
		return {
			...record,
			totals: maskTotals(record.totals, input.mix, ingredients),
		};
	});
}

export async function updateCrafter(userId: string, input: CrafterEdit) {
	const campaign = await bootstrap(userId);
	const [crafter] = await db
		.update(crafters)
		.set(input)
		.where(eq(crafters.campaignId, campaign.id))
		.returning();
	if (!crafter) throw new ORPCError("INTERNAL_SERVER_ERROR");
	return toCrafterView(crafter);
}

export async function adjustInventory(userId: string, input: InventoryAdjust) {
	if (input.kind === "ingredient") {
		if (!INGREDIENTS.some((ingredient) => ingredient.id === input.itemId))
			throw new ORPCError("BAD_REQUEST", { message: "Unknown ingredient" });
		const campaign = await bootstrap(userId);
		const [saved] = await db
			.insert(ingredientInventory)
			.values({
				campaignId: campaign.id,
				ingredientId: input.itemId,
				amount: Math.max(0, 1 + input.delta),
			})
			.onConflictDoUpdate({
				target: [
					ingredientInventory.campaignId,
					ingredientInventory.ingredientId,
				],
				set: {
					amount: sql`least(999, greatest(0, ${ingredientInventory.amount} + ${input.delta}))`,
				},
			})
			.returning({ amount: ingredientInventory.amount });
		return { kind: input.kind, itemId: input.itemId, amount: saved.amount };
	}
	if (!POTIONS.some((potion) => potion.id === input.itemId))
		throw new ORPCError("BAD_REQUEST", { message: "Unknown potion" });
	const campaign = await bootstrap(userId);
	await assertPotionInCodex(db, campaign.id, input.itemId);
	const [saved] = await db
		.insert(potionInventory)
		.values({
			campaignId: campaign.id,
			potionId: input.itemId,
			amount: Math.max(0, input.delta),
		})
		.onConflictDoUpdate({
			target: [potionInventory.campaignId, potionInventory.potionId],
			set: {
				amount: sql`least(999, greatest(0, ${potionInventory.amount} + ${input.delta}))`,
			},
		})
		.returning({ amount: potionInventory.amount });
	return { kind: input.kind, itemId: input.itemId, amount: saved.amount };
}

export async function deconstructItem(
	userId: string,
	input: Deconstruct,
): Promise<DeconstructView> {
	const item =
		input.kind === "ingredient"
			? INGREDIENTS.find((ingredient) => ingredient.id === input.itemId)
			: POTIONS.find((potion) => potion.id === input.itemId);
	if (!item)
		throw new ORPCError("BAD_REQUEST", {
			message:
				input.kind === "ingredient" ? "Unknown ingredient" : "Unknown potion",
		});
	const values = "values" in item ? item.values : item.target;
	const campaign = await bootstrap(userId);
	return db.transaction(async (tx) => {
		const [crafter] = await tx
			.select()
			.from(crafters)
			.where(eq(crafters.campaignId, campaign.id))
			.for("update");
		if (!crafter) throw new ORPCError("INTERNAL_SERVER_ERROR");
		let stock: number;
		if (input.kind === "ingredient") {
			await tx
				.insert(ingredientInventory)
				.values(startingInventory(campaign.id))
				.onConflictDoNothing();
			const [row] = await tx
				.select({ amount: ingredientInventory.amount })
				.from(ingredientInventory)
				.where(
					and(
						eq(ingredientInventory.campaignId, campaign.id),
						eq(ingredientInventory.ingredientId, item.id),
					),
				)
				.for("update");
			stock = row?.amount ?? 0;
		} else {
			await assertPotionInCodex(tx, campaign.id, item.id);
			const [row] = await tx
				.select({ amount: potionInventory.amount })
				.from(potionInventory)
				.where(
					and(
						eq(potionInventory.campaignId, campaign.id),
						eq(potionInventory.potionId, item.id),
					),
				)
				.for("update");
			stock = row?.amount ?? 0;
		}
		if (stock < 1)
			throw new ORPCError("BAD_REQUEST", {
				message: `No ${item.name} left to deconstruct`,
			});
		const known = await tx
			.select({ tradition: traditionKnowledge.tradition })
			.from(traditionKnowledge)
			.where(
				and(
					eq(traditionKnowledge.campaignId, campaign.id),
					eq(traditionKnowledge.itemKind, input.kind),
					eq(traditionKnowledge.itemId, item.id),
				),
			);
		const unlearned = TRADITIONS.filter(
			(tradition) => !known.some((row) => row.tradition === tradition),
		);
		if (unlearned.length === 0)
			throw new ORPCError("BAD_REQUEST", {
				message: `Every tradition of ${item.name} is already known`,
			});
		if (input.kind === "ingredient") {
			await tx
				.update(ingredientInventory)
				.set({ amount: sql`${ingredientInventory.amount} - 1` })
				.where(
					and(
						eq(ingredientInventory.campaignId, campaign.id),
						eq(ingredientInventory.ingredientId, item.id),
					),
				);
		} else {
			await tx
				.update(potionInventory)
				.set({ amount: sql`${potionInventory.amount} - 1` })
				.where(
					and(
						eq(potionInventory.campaignId, campaign.id),
						eq(potionInventory.potionId, item.id),
					),
				);
		}
		const dc = deconstructDc(item);
		const modifier = crafterModifier(crafter);
		const d20 = input.d20 ?? rollDie(20);
		const check: Check = {
			d20,
			tableRoll: input.d20 !== undefined,
			modifier,
			total: d20 + modifier,
			dc,
			degree: degreeOf(d20, d20 + modifier, dc),
		};
		let learned: DeconstructView["learned"] = null;
		if (check.degree === "success" || check.degree === "critical-success") {
			const tradition = unlearned[rollDie(unlearned.length) - 1];
			await tx
				.insert(traditionKnowledge)
				.values({
					campaignId: campaign.id,
					itemKind: input.kind,
					itemId: item.id,
					tradition,
				})
				.onConflictDoNothing();
			learned = { tradition, value: values[TRADITIONS.indexOf(tradition)] };
		}
		return {
			kind: input.kind,
			itemId: item.id,
			name: item.name,
			check,
			learned,
			remaining: stock - 1,
		};
	});
}

export async function setViewing(userId: string, viewingAs: ViewingAs) {
	const campaign = await bootstrap(userId);
	if (viewingAs === "gm" && campaign.gmUserId !== userId)
		throw new ORPCError("FORBIDDEN");
	await db
		.update(campaigns)
		.set({ viewingAs })
		.where(eq(campaigns.id, campaign.id));
	return { viewingAs };
}

async function requireGm(userId: string) {
	const campaign = await bootstrap(userId);
	if (campaign.gmUserId !== userId || campaign.viewingAs !== "gm")
		throw new ORPCError("FORBIDDEN");
	return campaign;
}

export async function getAnswerKey(userId: string): Promise<AnswerKeyEntry[]> {
	const campaign = await requireGm(userId);
	const entries = await db
		.select()
		.from(codexEntries)
		.where(eq(codexEntries.campaignId, campaign.id));
	return POTIONS.map((potion) => {
		const entry = entries.find((row) => row.potionId === potion.id);
		return {
			potion,
			primary: primaryOf(potion),
			dc: potionDc(potion),
			known: Boolean(entry && (entry.mentor || entry.formed || entry.revealed)),
			revealed: entry?.revealed ?? false,
			formulaRevealed: entry?.formulaRevealed ?? false,
			perfected: entry?.perfected ?? false,
		};
	});
}

export async function revealPotion(
	userId: string,
	potionId: string,
	reveal: Reveal["reveal"],
): Promise<{ ok: true }> {
	if (!POTIONS.some((potion) => potion.id === potionId))
		throw new ORPCError("BAD_REQUEST", { message: "Unknown potion" });
	const campaign = await requireGm(userId);
	const revealed = reveal !== "none";
	const formulaRevealed = reveal === "formula";
	await db
		.insert(codexEntries)
		.values({ campaignId: campaign.id, potionId, revealed, formulaRevealed })
		.onConflictDoUpdate({
			target: [codexEntries.campaignId, codexEntries.potionId],
			set: { revealed, formulaRevealed },
			setWhere: and(
				eq(codexEntries.campaignId, campaign.id),
				eq(codexEntries.potionId, potionId),
			),
		});
	return { ok: true };
}
