import {
	afterAll,
	beforeAll,
	beforeEach,
	describe,
	expect,
	test,
} from "bun:test";
import { eq, sql } from "drizzle-orm";
import { migrate } from "drizzle-orm/node-postgres/migrator";
import {
	INGREDIENTS,
	MENTOR_MIX,
	MENTOR_NOTE,
	POTIONS,
	STARTER_POTION_ID,
	WREN,
} from "#/brewing/catalog";
import type { Mix } from "#/brewing/types";
import {
	brews,
	campaigns,
	codexEntries,
	crafters,
	ingredientInventory,
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
		expect(log[0]).toMatchObject({ mix: MENTOR_MIX, result });
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
		const heartmoss = { ingredientId: "heartmoss" } as const;
		expect(await adjustInventory(GM, { ...heartmoss, delta: -1 })).toEqual({
			ingredientId: "heartmoss",
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
			(await adjustInventory(GM, { ingredientId: "glowcap", delta: 1 })).amount,
		).toBe(2);
		expect(
			(await adjustInventory(GM, { ingredientId: "heartmoss", delta: -1 }))
				.amount,
		).toBe(0);
	});

	test("rejects unknown ingredients", async () => {
		await expect(
			adjustInventory(GM, { ingredientId: "dragon-tear", delta: 1 }),
		).rejects.toMatchObject({ code: "BAD_REQUEST" });
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
