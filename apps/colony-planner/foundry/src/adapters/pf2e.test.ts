import { describe, expect, test } from "bun:test";
import {
	createFoundryWorld,
	eachWriteHadFreshOptions,
	type FoundryWorld,
} from "../testing/foundry-world";
import { type OutboxRow, readField } from "../wire";
import { selectAdapter } from "./adapter";
import { createPf2eAdapter } from "./pf2e";

const ropeUuid = "Compendium.pf2e.equipment-srd.Item.abcdefghij123456";
const torchUuid = "Compendium.pf2e.equipment-srd.Item.bcdefghij1234567";
const spellUuid = "Compendium.pf2e.spells-srd.Item.cdefghij12345678";
const missingUuid = "Compendium.pf2e.equipment-srd.Item.zzzzzzzzzzzzzzzz";
const brokenPackUuid = "Compendium.pf2e.broken-pack.Item.yyyyyyyyyyyyyyyy";
const worldItemUuid = "Actor.abcdefghij123456.Item.abcdefghij123456";

const delivery = {
	items: [
		{ compendiumUuid: ropeUuid, name: "Rope", quantity: 3 },
		{ compendiumUuid: null, name: "Boots <of Striding>", quantity: 1 },
		{ compendiumUuid: spellUuid, name: "Fireball scroll", quantity: 1 },
		{ compendiumUuid: missingUuid, name: "Lost lantern", quantity: 1 },
		{ compendiumUuid: brokenPackUuid, name: "Typo", quantity: 1 },
		{ compendiumUuid: torchUuid, name: "Torch", quantity: 10 },
	],
	paidDay: 14,
	summary: "20 food, Rope, Boots <of Striding>",
	totalGold: 1200,
};

const deliveryRow = { kind: "DELIVERY", payload: delivery, seq: 4, targetKey: "delivery:9" };

function worldWithCompendium(options: { party?: boolean } = {}): FoundryWorld {
	const world = createFoundryWorld(options);
	world.addCompendiumItem(ropeUuid, "Rope");
	world.addCompendiumItem(torchUuid, "Torch", { quantity: 1 });
	world.addCompendiumItem(spellUuid, "Fireball", { documentName: "Item", quantity: undefined });
	world.addCompendiumItem(worldItemUuid, "GM's secret blade");
	return world;
}

async function applyDelivery(world: FoundryWorld, row: OutboxRow = deliveryRow) {
	await createPf2eAdapter(world.dependencies).appliers.DELIVERY?.(row);
}

function messageFlag(world: FoundryWorld, key: string): unknown {
	return world.messages[0]?.getFlag("colony-bridge", key);
}

describe("PF2e DELIVERY", () => {
	test("adds each resolved physical item to the party with the line's quantity, in one write", async () => {
		const world = worldWithCompendium();
		await applyDelivery(world);
		const itemWrites = world.writes.filter((write) => write.kind === "items create");
		expect(itemWrites).toHaveLength(1);
		expect(itemWrites[0]?.data).toEqual([
			{
				flags: { "colony-bridge": { externalId: "delivery:9:0" }, core: { sheetClass: "" } },
				name: "Rope",
				system: { price: { value: { gp: 5 } }, quantity: 3 },
				type: "equipment",
			},
			{
				flags: { "colony-bridge": { externalId: "delivery:9:5" }, core: { sheetClass: "" } },
				name: "Torch",
				system: { price: { value: { gp: 5 } }, quantity: 10 },
				type: "equipment",
			},
		]);
		expect(world.partyItems.map((item) => readField(item.data.system, "quantity"))).toEqual([
			3, 10,
		]);
		expect(world.writes.map((write) => write.kind)).toEqual(["items create", "chat create"]);
		expect(world.messages[0]?.data.flags).toEqual({
			"colony-bridge": { externalId: "delivery:9" },
		});
		expect(eachWriteHadFreshOptions(world)).toBe(true);
	});

	test("the message lists what was added and what to hand out: no UUID, unresolved, not physical", async () => {
		const world = worldWithCompendium();
		await applyDelivery(world);
		expect(world.messages).toHaveLength(1);
		expect(world.messages[0]?.data.speaker).toEqual({ alias: "Colony Planner" });
		expect(world.messages[0]?.data.content).toBe(
			[
				"<p>Goods unloaded on Day 14, paid 1,200 gold: 20 food, Rope, Boots &lt;of Striding&gt;</p>",
				"<p>Added to The &lt;Wardens&gt;'s inventory:</p><ul><li>Rope (3)</li><li>Torch (10)</li></ul>",
				"<p>Hand these out yourself:</p><ul><li>Boots &lt;of Striding&gt; (1)</li><li>Fireball scroll (1)</li><li>Lost lantern (1)</li><li>Typo (1)</li></ul>",
			].join(""),
		);
	});

	test("a UUID outside a compendium is handed out without being loaded, even when Foundry has it", async () => {
		const world = worldWithCompendium();
		const loaded: string[] = [];
		const fromUuid = world.dependencies.fromUuid;
		world.dependencies.fromUuid = (uuid) => {
			loaded.push(uuid);
			return fromUuid(uuid);
		};
		await applyDelivery(world, {
			...deliveryRow,
			payload: {
				...delivery,
				items: [
					{ compendiumUuid: worldItemUuid, name: "Blade", quantity: 1 },
					{ compendiumUuid: `${ropeUuid}x`, name: "Rope", quantity: 1 },
				],
			},
		});
		expect(loaded).toEqual([]);
		expect(world.partyItems).toEqual([]);
		expect(String(world.messages[0]?.data.content)).toContain(
			"<p>Hand these out yourself:</p><ul><li>Blade (1)</li><li>Rope (1)</li></ul>",
		);
	});

	test("a UUID that resolves to something other than an Item is handed out", async () => {
		const world = createFoundryWorld();
		world.addCompendiumItem(ropeUuid, "Rope journal", { documentName: "JournalEntry" });
		await applyDelivery(world, {
			...deliveryRow,
			payload: { ...delivery, items: [{ compendiumUuid: ropeUuid, name: "Rope", quantity: 1 }] },
		});
		expect(world.partyItems).toEqual([]);
		expect(String(world.messages[0]?.data.content)).toContain(
			"<p>Hand these out yourself:</p><ul><li>Rope (1)</li></ul>",
		);
		expect(messageFlag(world, "itemsOwed")).toBeUndefined();
	});

	test("without a party actor every line is handed out and nothing is added", async () => {
		const world = worldWithCompendium({ party: false });
		await applyDelivery(world);
		expect(world.writes.map((write) => write.kind)).toEqual(["chat create"]);
		const content = String(world.messages[0]?.data.content);
		expect(content).not.toContain("Added to");
		expect(content).toContain("<li>Rope (3)</li>");
		expect(content).toContain("<li>Torch (10)</li>");
		expect(messageFlag(world, "itemsOwed")).toBeUndefined();
	});

	test("a repeat of a finished delivery adds nothing and posts nothing", async () => {
		const world = worldWithCompendium();
		await applyDelivery(world);
		const writes = world.writes.length;
		await applyDelivery(world);
		expect(world.writes).toHaveLength(writes);
		expect(world.partyItems).toHaveLength(2);
	});

	test("when the items fail to save, the message calls none of them added and the row fails, for good", async () => {
		const world = worldWithCompendium();
		world.failNext.add("items create");
		await expect(applyDelivery(world)).rejects.toThrow("Foundry refused the items create.");
		expect(world.messages).toHaveLength(1);
		const content = String(world.messages[0]?.data.content);
		expect(content).not.toContain("Added to");
		expect(content).toContain("<li>Rope (3)</li>");
		expect(content).toContain("<li>Torch (10)</li>");
		await applyDelivery(world);
		expect(world.partyItems).toEqual([]);
		expect(world.messages).toHaveLength(1);
	});

	test("a replay after the message failed to post adds nothing twice, then names the items added", async () => {
		const world = worldWithCompendium();
		world.failNext.add("chat create");
		await expect(applyDelivery(world)).rejects.toThrow("Foundry refused the chat create.");
		expect(world.partyItems).toHaveLength(2);
		expect(world.messages).toEqual([]);
		await applyDelivery(world);
		await applyDelivery(world);
		expect(world.partyItems).toHaveLength(2);
		expect(world.writes.filter((write) => write.kind === "items create")).toHaveLength(1);
		expect(world.messages).toHaveLength(1);
		expect(String(world.messages[0]?.data.content)).toContain(
			"<p>Added to The &lt;Wardens&gt;'s inventory:</p><ul><li>Rope (3)</li><li>Torch (10)</li></ul>",
		);
	});

	test("a replay after only some items reached the party adds the missing ones", async () => {
		const world = worldWithCompendium();
		await world.dependencies.foundryGame.actors.party?.createEmbeddedDocuments("Item", [
			{ flags: { "colony-bridge": { externalId: "delivery:9:0" } }, name: "Rope" },
		]);
		world.writes.length = 0;
		await applyDelivery(world);
		const itemWrite = world.writes.find((write) => write.kind === "items create");
		expect(
			Array.isArray(itemWrite?.data) && itemWrite.data.map((item) => readField(item, "name")),
		).toEqual(["Torch"]);
		expect(String(world.messages[0]?.data.content)).toContain(
			"<ul><li>Rope (3)</li><li>Torch (10)</li></ul>",
		);
	});

	test("a player's chat message with the delivery's key and a crafted list of lines adds nothing extra", async () => {
		const world = worldWithCompendium();
		world.addMessage({
			flags: {
				"colony-bridge": {
					externalId: "delivery:9",
					itemsOwed: true,
					owedLines: [0, 0, 0, 0, 0, 0],
				},
			},
		});
		await applyDelivery(world);
		expect(world.partyItems.map((item) => item.data.name)).toEqual(["Rope", "Torch"]);
		expect(world.messages).toHaveLength(2);
		expect(world.messages[1]?.author?.isGM).toBe(true);
	});

	test("a party item flagged for a line without a compendium UUID doesn't make the message call it added", async () => {
		const world = worldWithCompendium();
		await world.dependencies.foundryGame.actors.party?.createEmbeddedDocuments("Item", [
			{ flags: { "colony-bridge": { externalId: "delivery:9:1" } }, name: "Old boots" },
		]);
		await applyDelivery(world);
		expect(String(world.messages[0]?.data.content)).toContain(
			"<p>Hand these out yourself:</p><ul><li>Boots &lt;of Striding&gt; (1)</li>",
		);
	});

	test("DELIVERY_UNDONE after its applied DELIVERY is the core adapter's whisper to the GMs, and leaves the party's items", async () => {
		const world = worldWithCompendium();
		await applyDelivery(world);
		await createPf2eAdapter(world.dependencies).appliers.DELIVERY_UNDONE?.({
			kind: "DELIVERY_UNDONE",
			payload: { items: [{ name: "Rope", quantity: 3 }], summary: "Rope" },
			seq: 5,
			targetKey: "delivery-undone:9",
		});
		expect(world.partyItems).toHaveLength(2);
		expect(world.messages[1]?.data.whisper).toEqual(["gm-1", "gm-2"]);
	});

	test("DELIVERY_UNDONE without its DELIVERY in this world posts nothing and succeeds", async () => {
		const world = worldWithCompendium();
		await createPf2eAdapter(world.dependencies).appliers.DELIVERY_UNDONE?.({
			kind: "DELIVERY_UNDONE",
			payload: { items: [{ name: "Rope", quantity: 3 }], summary: "Rope" },
			seq: 5,
			targetKey: "delivery-undone:9",
		});
		expect(world.messages).toEqual([]);
		expect(world.writes).toEqual([]);
	});

	test("a payload of the wrong shape fails the row", async () => {
		const world = worldWithCompendium();
		await expect(
			applyDelivery(world, { ...deliveryRow, payload: { ...delivery, paidDay: "14" } }),
		).rejects.toThrow("The delivery's payload wasn't understood.");
		expect(world.writes).toEqual([]);
	});
});

describe("selectAdapter", () => {
	test("a PF2e world gets the PF2e adapter, with every core kind; any other world the core one", () => {
		const world = createFoundryWorld();
		const pf2e = selectAdapter("pf2e", world.dependencies);
		const core = selectAdapter("dnd5e", world.dependencies);
		expect(pf2e.name).toBe("pf2e");
		expect(core.name).toBe("core");
		expect(Object.keys(pf2e.appliers).sort()).toEqual(Object.keys(core.appliers).sort());
		expect(pf2e.appliers.DELIVERY).not.toBe(core.appliers.DELIVERY);
	});
});
