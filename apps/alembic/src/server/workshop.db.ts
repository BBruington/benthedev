import {
	afterAll,
	beforeAll,
	beforeEach,
	describe,
	expect,
	test,
} from "bun:test";
import { and, eq, sql } from "drizzle-orm";
import { migrate } from "drizzle-orm/node-postgres/migrator";
import {
	INGREDIENTS,
	MENTOR_MIX,
	MENTOR_NOTE,
	POTIONS,
	STARTER_POTION_ID,
	WREN,
} from "#/brewing/catalog";
import { type Mix, TRADITIONS, type Tradition } from "#/brewing/types";
import {
	brews,
	campaigns,
	codexEntries,
	crafters,
	ingredientInventory,
	potionInventory,
	traditionKnowledge,
} from "#/db/schema";

const testUrl = process.env.TEST_DATABASE_URL;
if (!testUrl) throw new Error("TEST_DATABASE_URL is required");
if (testUrl === process.env.DATABASE_URL)
	throw new Error(
		"TEST_DATABASE_URL must not be DATABASE_URL: these tests wipe every table",
	);
process.env.DATABASE_URL = testUrl;
// Imported only after the swap above: #/db/index reads DATABASE_URL when it loads.
const { db } = await import("#/db/index");
const {
	adjustInventory,
	attemptBrew,
	bootstrap,
	deconstructItem,
	getAnswerKey,
	getWorkshop,
	revealPotion,
	setViewing,
} = await import("./workshop");

const GM = "user_gm";
const perfectHealing: Mix = { ...MENTOR_MIX, glowcap: 1 };

async function stockOf(campaignId: string) {
	const rows = await db
		.select()
		.from(ingredientInventory)
		.where(eq(ingredientInventory.campaignId, campaignId));
	return Object.fromEntries(rows.map((row) => [row.ingredientId, row.amount]));
}

async function setStock(campaignId: string, amount: number) {
	await db
		.update(ingredientInventory)
		.set({ amount })
		.where(eq(ingredientInventory.campaignId, campaignId));
}

async function codexEntry(campaignId: string, potionId: string) {
	return db.query.codexEntries.findFirst({
		where: (entry, { and }) =>
			and(eq(entry.campaignId, campaignId), eq(entry.potionId, potionId)),
	});
}

async function knowledgeOf(campaignId: string) {
	return db
		.select()
		.from(traditionKnowledge)
		.where(eq(traditionKnowledge.campaignId, campaignId));
}

async function potionStockOf(campaignId: string, potionId: string) {
	const [row] = await db
		.select()
		.from(potionInventory)
		.where(
			and(
				eq(potionInventory.campaignId, campaignId),
				eq(potionInventory.potionId, potionId),
			),
		);
	return row?.amount ?? 0;
}

async function downtimeOf(campaignId: string) {
	const crafter = await db.query.crafters.findFirst({
		where: eq(crafters.campaignId, campaignId),
	});
	return crafter?.downtimeDays;
}

async function learn(
	campaignId: string,
	itemIds: readonly string[],
	traditions: readonly Tradition[] = TRADITIONS,
) {
	await db.insert(traditionKnowledge).values(
		itemIds.flatMap((itemId) =>
			traditions.map((tradition) => ({
				campaignId,
				itemKind: "ingredient" as const,
				itemId,
				tradition,
			})),
		),
	);
}

beforeAll(async () => {
	await migrate(db, { migrationsFolder: `${import.meta.dir}/../../drizzle` });
});

beforeEach(async () => {
	await db.execute(sql`TRUNCATE ${campaigns} CASCADE`);
});

afterAll(async () => {
	await db.$client.end();
});

describe("bootstrap", () => {
	test("creates one seeded campaign per GM, however often it runs", async () => {
		const [first, second] = await Promise.all([bootstrap(GM), bootstrap(GM)]);
		expect(second.id).toBe(first.id);
		expect(await db.select().from(campaigns)).toHaveLength(1);

		const crafter = await db.query.crafters.findFirst({
			where: eq(crafters.campaignId, first.id),
		});
		expect(crafter).toMatchObject({ ...WREN, downtimeDays: 0 });
		expect(await codexEntry(first.id, STARTER_POTION_ID)).toMatchObject({
			mentor: true,
			note: MENTOR_NOTE,
		});
		expect(await stockOf(first.id)).toEqual(
			Object.fromEntries(INGREDIENTS.map((ingredient) => [ingredient.id, 1])),
		);
	});

	test("the workshop restores missing stock rows and a lost best mix", async () => {
		const campaign = await bootstrap(GM);
		await attemptBrew(GM, { mix: MENTOR_MIX, d20: 6 });
		await db
			.delete(ingredientInventory)
			.where(eq(ingredientInventory.campaignId, campaign.id));
		await db
			.update(codexEntries)
			.set({ bestMix: null })
			.where(eq(codexEntries.campaignId, campaign.id));

		const view = await getWorkshop(GM);
		expect(Object.values(view.inventory).every((amount) => amount === 1)).toBe(
			true,
		);
		expect(Object.keys(await stockOf(campaign.id))).toHaveLength(
			INGREDIENTS.length,
		);
		expect(
			view.codex.find((entry) => entry.potionId === STARTER_POTION_ID)
				?.solvedMix,
		).toEqual(MENTOR_MIX);
	});
});

describe("attemptBrew", () => {
	test("spends the mix, logs the brew, adds a downtime day and records the codex", async () => {
		const campaign = await bootstrap(GM);
		const result = await attemptBrew(GM, { mix: MENTOR_MIX, d20: 6 });
		expect(result).toMatchObject({
			kind: "formed",
			potionId: STARTER_POTION_ID,
			potionName: "Healing Potion (Minor)",
			offBy: 14,
			check: { d20: 6, tableRoll: true, degree: "success" },
		});

		const stock = await stockOf(campaign.id);
		for (const ingredient of INGREDIENTS)
			expect(stock[ingredient.id]).toBe(ingredient.id in MENTOR_MIX ? 0 : 1);
		const log = await db.select().from(brews);
		expect(log).toHaveLength(1);
		const { totals, ...unmasked } = result;
		expect(totals).toEqual([null, null, null, null]);
		expect(log[0]).toMatchObject({ mix: MENTOR_MIX, result: unmasked });
		expect(await potionStockOf(campaign.id, STARTER_POTION_ID)).toBe(
			result.kind === "formed" ? result.quantity : -1,
		);
		const crafter = await db.query.crafters.findFirst({
			where: eq(crafters.campaignId, campaign.id),
		});
		expect(crafter?.downtimeDays).toBe(1);
		expect(await codexEntry(campaign.id, STARTER_POTION_ID)).toMatchObject({
			mentor: true,
			formed: true,
			perfected: false,
			bestOffBy: 14,
			bestMix: MENTOR_MIX,
		});
	});

	test("refuses short stock without spending anything", async () => {
		const campaign = await bootstrap(GM);
		await db
			.update(ingredientInventory)
			.set({ amount: 0 })
			.where(eq(ingredientInventory.ingredientId, "heartmoss"));

		await expect(
			attemptBrew(GM, { mix: MENTOR_MIX, d20: 6 }),
		).rejects.toMatchObject({
			code: "BAD_REQUEST",
			message: "Not enough ingredients: Heartmoss (need 1, have 0)",
		});
		expect((await stockOf(campaign.id))["marsh-toad-gland"]).toBe(1);
		expect(await db.select().from(brews)).toHaveLength(0);
		const crafter = await db.query.crafters.findFirst({
			where: eq(crafters.campaignId, campaign.id),
		});
		expect(crafter?.downtimeDays).toBe(0);
	});

	test("rejects unknown ingredients", async () => {
		await expect(
			attemptBrew(GM, { mix: { "dragon-tear": 1 } }),
		).rejects.toMatchObject({
			code: "BAD_REQUEST",
			message: "Unknown ingredient",
		});
	});

	test("the codex keeps the best mix and never loses a perfect brew", async () => {
		const campaign = await bootstrap(GM);
		await setStock(campaign.id, 10);

		await attemptBrew(GM, { mix: perfectHealing, d20: 6 });
		await attemptBrew(GM, { mix: MENTOR_MIX, d20: 6 });
		expect(await codexEntry(campaign.id, STARTER_POTION_ID)).toMatchObject({
			perfected: true,
			bestOffBy: 0,
			bestMix: perfectHealing,
		});

		await db.execute(sql`TRUNCATE ${campaigns} CASCADE`);
		const fresh = await bootstrap(GM);
		await setStock(fresh.id, 10);
		await attemptBrew(GM, { mix: MENTOR_MIX, d20: 6 });
		await attemptBrew(GM, { mix: perfectHealing, d20: 6 });
		expect(await codexEntry(fresh.id, STARTER_POTION_ID)).toMatchObject({
			perfected: true,
			bestOffBy: 0,
			bestMix: perfectHealing,
		});
	});
});

describe("adjustInventory", () => {
	test("stock stays between 0 and 999", async () => {
		const campaign = await bootstrap(GM);
		const heartmoss = { kind: "ingredient", itemId: "heartmoss" } as const;
		expect(await adjustInventory(GM, { ...heartmoss, delta: -1 })).toEqual({
			kind: "ingredient",
			itemId: "heartmoss",
			amount: 0,
		});
		expect(
			(await adjustInventory(GM, { ...heartmoss, delta: -1 })).amount,
		).toBe(0);

		await setStock(campaign.id, 999);
		expect((await adjustInventory(GM, { ...heartmoss, delta: 1 })).amount).toBe(
			999,
		);
	});

	test("a missing stock row starts from the fresh-campaign amount", async () => {
		const campaign = await bootstrap(GM);
		await db
			.delete(ingredientInventory)
			.where(eq(ingredientInventory.campaignId, campaign.id));
		expect(
			(
				await adjustInventory(GM, {
					kind: "ingredient",
					itemId: "glowcap",
					delta: 1,
				})
			).amount,
		).toBe(2);
		expect(
			(
				await adjustInventory(GM, {
					kind: "ingredient",
					itemId: "heartmoss",
					delta: -1,
				})
			).amount,
		).toBe(0);
	});

	test("potion stock starts at 0 and stays between 0 and 999", async () => {
		const campaign = await bootstrap(GM);
		const potion = { kind: "potion", itemId: STARTER_POTION_ID } as const;
		expect(await adjustInventory(GM, { ...potion, delta: -1 })).toEqual({
			...potion,
			amount: 0,
		});
		expect((await adjustInventory(GM, { ...potion, delta: 1 })).amount).toBe(1);
		await db
			.update(potionInventory)
			.set({ amount: 999 })
			.where(eq(potionInventory.campaignId, campaign.id));
		expect((await adjustInventory(GM, { ...potion, delta: 1 })).amount).toBe(
			999,
		);
	});

	test("rejects unknown items", async () => {
		await expect(
			adjustInventory(GM, {
				kind: "ingredient",
				itemId: "dragon-tear",
				delta: 1,
			}),
		).rejects.toMatchObject({ code: "BAD_REQUEST" });
		await expect(
			adjustInventory(GM, { kind: "potion", itemId: "dragon-tear", delta: 1 }),
		).rejects.toMatchObject({ code: "BAD_REQUEST", message: "Unknown potion" });
	});
});

describe("deconstructItem", () => {
	test("a fresh workshop hides every tradition", async () => {
		const view = await getWorkshop(GM);
		for (const ingredient of view.ingredients)
			expect(ingredient.values).toEqual([null, null, null, null]);
		for (const entry of view.codex)
			expect(entry.traditions).toEqual([null, null, null, null]);
	});

	test("a success spends one unit and learns one tradition without downtime", async () => {
		const campaign = await bootstrap(GM);
		const result = await deconstructItem(GM, {
			kind: "ingredient",
			itemId: "heartmoss",
			d20: 6,
		});
		expect(result).toMatchObject({
			kind: "ingredient",
			itemId: "heartmoss",
			name: "Heartmoss",
			check: {
				d20: 6,
				tableRoll: true,
				modifier: 14,
				total: 20,
				dc: 13,
				degree: "success",
			},
			remaining: 0,
		});
		const heartmoss = INGREDIENTS.find(
			(ingredient) => ingredient.id === "heartmoss",
		);
		const learned = result.learned;
		if (!heartmoss || !learned) throw new Error("expected a learned tradition");
		expect(learned.value).toBe(
			heartmoss.values[TRADITIONS.indexOf(learned.tradition)],
		);
		expect((await stockOf(campaign.id)).heartmoss).toBe(0);
		expect(await knowledgeOf(campaign.id)).toEqual([
			{
				campaignId: campaign.id,
				itemKind: "ingredient",
				itemId: "heartmoss",
				tradition: learned.tradition,
			},
		]);
		expect(await downtimeOf(campaign.id)).toBe(0);

		const view = await getWorkshop(GM);
		const values = view.ingredients.find(
			(ingredient) => ingredient.id === "heartmoss",
		)?.values;
		expect(values?.filter((value) => value !== null)).toEqual([learned.value]);
	});

	test("a natural 1 fails, spends the unit and learns nothing", async () => {
		const campaign = await bootstrap(GM);
		const result = await deconstructItem(GM, {
			kind: "ingredient",
			itemId: "heartmoss",
			d20: 1,
		});
		expect(result.check.degree).toBe("failure");
		expect(result.learned).toBeNull();
		expect((await stockOf(campaign.id)).heartmoss).toBe(0);
		expect(await knowledgeOf(campaign.id)).toHaveLength(0);
	});

	test("refuses an empty stock without spending anything", async () => {
		const campaign = await bootstrap(GM);
		await setStock(campaign.id, 0);
		await expect(
			deconstructItem(GM, { kind: "ingredient", itemId: "heartmoss", d20: 6 }),
		).rejects.toMatchObject({
			code: "BAD_REQUEST",
			message: "No Heartmoss left to deconstruct",
		});
		await expect(
			deconstructItem(GM, { kind: "potion", itemId: STARTER_POTION_ID }),
		).rejects.toMatchObject({
			code: "BAD_REQUEST",
			message: "No Healing Potion (Minor) left to deconstruct",
		});
		expect(await knowledgeOf(campaign.id)).toHaveLength(0);
	});

	test("refuses a fully known item without spending it", async () => {
		const campaign = await bootstrap(GM);
		await learn(campaign.id, ["heartmoss"]);
		await expect(
			deconstructItem(GM, { kind: "ingredient", itemId: "heartmoss", d20: 6 }),
		).rejects.toMatchObject({
			code: "BAD_REQUEST",
			message: "Every tradition of Heartmoss is already known",
		});
		expect((await stockOf(campaign.id)).heartmoss).toBe(1);
	});

	test("rejects unknown items", async () => {
		await expect(
			deconstructItem(GM, { kind: "potion", itemId: "heartmoss" }),
		).rejects.toMatchObject({ code: "BAD_REQUEST", message: "Unknown potion" });
	});

	test("a formed brew stocks potions that can be deconstructed", async () => {
		const campaign = await bootstrap(GM);
		const brew = await attemptBrew(GM, { mix: MENTOR_MIX, d20: 6 });
		if (brew.kind !== "formed") throw new Error("expected a formed brew");
		expect(brew.quantity).toBeGreaterThan(0);
		expect(await potionStockOf(campaign.id, STARTER_POTION_ID)).toBe(
			brew.quantity,
		);
		expect(
			(await getWorkshop(GM)).codex.find(
				(entry) => entry.potionId === STARTER_POTION_ID,
			)?.stock,
		).toBe(brew.quantity);

		const result = await deconstructItem(GM, {
			kind: "potion",
			itemId: STARTER_POTION_ID,
			d20: 6,
		});
		expect(result.check).toMatchObject({ dc: 14, degree: "success" });
		expect(result.learned).not.toBeNull();
		expect(result.remaining).toBe(brew.quantity - 1);
		expect(await potionStockOf(campaign.id, STARTER_POTION_ID)).toBe(
			brew.quantity - 1,
		);
	});

	test("logged totals unmask once every mixed ingredient is fully known", async () => {
		const campaign = await bootstrap(GM);
		const brew = await attemptBrew(GM, { mix: MENTOR_MIX, d20: 6 });
		expect(brew.totals).toEqual([null, null, null, null]);
		const [stored] = await db.select().from(brews);
		await learn(campaign.id, Object.keys(MENTOR_MIX));
		expect((await getWorkshop(GM)).log[0].result.totals).toEqual(
			stored.result.totals,
		);
	});

	test("totals unmask per tradition, and the GM sees them all", async () => {
		const campaign = await bootstrap(GM);
		await setStock(campaign.id, 10);
		await learn(campaign.id, Object.keys(MENTOR_MIX), ["primal"]);
		const partial = await attemptBrew(GM, { mix: MENTOR_MIX, d20: 6 });
		expect(partial.totals[1]).not.toBeNull();
		expect([partial.totals[0], partial.totals[2], partial.totals[3]]).toEqual([
			null,
			null,
			null,
		]);

		await setViewing(GM, "gm");
		const gm = await attemptBrew(GM, { mix: MENTOR_MIX, d20: 6 });
		expect(gm.totals.every((total) => total !== null)).toBe(true);
	});

	test("a formed brew caps potion stock at 999", async () => {
		const campaign = await bootstrap(GM);
		await db.insert(potionInventory).values({
			campaignId: campaign.id,
			potionId: STARTER_POTION_ID,
			amount: 999,
		});
		const brew = await attemptBrew(GM, { mix: MENTOR_MIX, d20: 6 });
		expect(brew.kind === "formed" && brew.quantity).toBeGreaterThan(0);
		expect(await potionStockOf(campaign.id, STARTER_POTION_ID)).toBe(999);
	});

	test("undiscovered potions can't be stocked or deconstructed", async () => {
		const campaign = await bootstrap(GM);
		await db.insert(potionInventory).values({
			campaignId: campaign.id,
			potionId: "potion-of-flying",
			amount: 1,
		});
		const flying = { kind: "potion", itemId: "potion-of-flying" } as const;
		await expect(
			adjustInventory(GM, { ...flying, delta: 1 }),
		).rejects.toMatchObject({ code: "BAD_REQUEST", message: "Unknown potion" });
		await expect(deconstructItem(GM, flying)).rejects.toMatchObject({
			code: "BAD_REQUEST",
			message: "Unknown potion",
		});
		expect(await potionStockOf(campaign.id, "potion-of-flying")).toBe(1);
		expect(await knowledgeOf(campaign.id)).toHaveLength(0);
	});

	test("concurrent deconstructs can't spend the same last unit twice", async () => {
		const campaign = await bootstrap(GM);
		const heartmoss = {
			kind: "ingredient",
			itemId: "heartmoss",
			d20: 6,
		} as const;
		const results = await Promise.allSettled([
			deconstructItem(GM, heartmoss),
			deconstructItem(GM, heartmoss),
		]);
		expect(results.filter((r) => r.status === "fulfilled")).toHaveLength(1);
		expect((await stockOf(campaign.id)).heartmoss).toBe(0);
		expect(await knowledgeOf(campaign.id)).toHaveLength(1);
	});
});

describe("GM answer key and reveals", () => {
	const truth = POTIONS.find((potion) => potion.id === "truth-potion");
	if (!truth) throw new Error("Missing Truth Potion");

	test("are refused while viewing as a player", async () => {
		await bootstrap(GM);
		await expect(getAnswerKey(GM)).rejects.toMatchObject({ code: "FORBIDDEN" });
		await expect(revealPotion(GM, truth.id, "potion")).rejects.toMatchObject({
			code: "FORBIDDEN",
		});
	});

	test("reveal levels set and clear the codex flags", async () => {
		const campaign = await bootstrap(GM);
		await setViewing(GM, "gm");

		await revealPotion(GM, truth.id, "formula");
		expect(await codexEntry(campaign.id, truth.id)).toMatchObject({
			revealed: true,
			formulaRevealed: true,
		});
		const key = await getAnswerKey(GM);
		expect(key.find((entry) => entry.potion.id === truth.id)).toMatchObject({
			known: true,
			revealed: true,
			formulaRevealed: true,
		});
		const view = await getWorkshop(GM);
		expect(
			view.codex.find((entry) => entry.potionId === truth.id)?.solvedMix,
		).toEqual(truth.perfectRecipe);

		await revealPotion(GM, truth.id, "potion");
		expect(await codexEntry(campaign.id, truth.id)).toMatchObject({
			revealed: true,
			formulaRevealed: false,
		});

		await revealPotion(GM, truth.id, "none");
		expect(await codexEntry(campaign.id, truth.id)).toMatchObject({
			revealed: false,
			formulaRevealed: false,
		});
		expect(
			(await getWorkshop(GM)).codex.some(
				(entry) => entry.potionId === truth.id,
			),
		).toBe(false);
	});

	test("rejects unknown potions", async () => {
		await bootstrap(GM);
		await setViewing(GM, "gm");
		await expect(
			revealPotion(GM, "elixir-of-life", "potion"),
		).rejects.toMatchObject({
			code: "BAD_REQUEST",
		});
	});
});
