import { describe, expect, test } from "bun:test";
import {
	createFoundryWorld,
	eachWriteHadFreshOptions,
	type FoundryWorld,
} from "../testing/foundry-world";
import type { NpcPayload, OutboxRow, PlacePayload, QuestPayload } from "../wire";
import { readField } from "../wire";
import { createCoreAdapter } from "./core";

const npc: NpcPayload = {
	age: "40s",
	aliases: ["The Smith", " "],
	appearance: "Tall.\nScarred hands.",
	backstory: "",
	characterClass: "Fighter",
	colonyRole: "Blacksmith",
	currentPlace: "The <Forge>",
	displayName: "Brenna <Ironhand>",
	health: "UNKNOWN",
	labels: [],
	level: 3,
	originPlace: null,
	personality: "Gruff & kind",
	playerNotes: "",
	race: "Dwarf",
	relation: "ALLY",
	siteUrl: "https://colony.benthedev.com/c/port-eldari/npcs/1",
	status: "ALIVE",
};

const place: PlacePayload = {
	description: "A ruined tower.",
	name: "Old Tower",
	playerNotes: "Haunted?",
	siteUrl: "https://colony.benthedev.com/c/port-eldari/places/2",
};

const quest: QuestPayload = {
	description: "Clear the road.",
	giver: "Brenna",
	kind: "MAINLAND",
	loot: [{ description: "Sharp", name: "Dagger <+1>", quantity: 2 }],
	patron: "The Guild",
	resolvedDay: null,
	reward: "50 gold",
	rewardFavor: 2,
	siteUrl: "https://colony.benthedev.com/c/port-eldari/quests/3",
	status: "ACTIVE",
	title: "Bandits",
};

function row(kind: string, targetKey: string, payload: unknown, seq = 1): OutboxRow {
	return { kind, payload, seq, targetKey };
}

function adapterFor(world: FoundryWorld) {
	return createCoreAdapter(world.dependencies);
}

async function apply(world: FoundryWorld, outboxRow: OutboxRow) {
	await adapterFor(world).appliers[outboxRow.kind]?.(outboxRow);
}

function ownership(world: FoundryWorld, externalId: string): unknown {
	return readField(world.entryFor(externalId)?.data.ownership, "default");
}

function bridgePageContent(world: FoundryWorld, externalId: string): unknown {
	const page = world
		.entryFor(externalId)
		?.pageList.find((candidate) => candidate.getFlag("colony-bridge", "externalId") === externalId);
	return readField(page?.data.text, "content");
}

describe.each([
	["NPC_UPSERT", "NPC_WITHDRAW", "npc:1", npc, "Colony NPCs", "folder:npcs", "Brenna <Ironhand>"],
	[
		"PLACE_UPSERT",
		"PLACE_WITHDRAW",
		"place:2",
		place,
		"Colony Places",
		"folder:places",
		"Old Tower",
	],
	["QUEST_UPSERT", "QUEST_WITHDRAW", "quest:3", quest, "Colony Quests", "folder:quests", "Bandits"],
])("%s and %s", (upsertKind, withdrawKind, targetKey, payload, folderName, folderKey, name) => {
	test("creates the folder and an entry in it, readable by players, flagged with the target key", async () => {
		const world = createFoundryWorld();
		await apply(world, row(upsertKind, targetKey, payload));
		expect(world.writes.map((write) => write.kind)).toEqual(["folder create", "entry create"]);
		expect(world.folders[0]?.data).toEqual({
			flags: { "colony-bridge": { externalId: folderKey } },
			name: folderName,
			type: "JournalEntry",
		});
		const entry = world.entryFor(targetKey);
		expect(entry?.data).toMatchObject({
			folder: world.folders[0]?.id,
			name,
			ownership: { default: 2 },
		});
		expect(entry?.pageList[0]?.data).toMatchObject({
			flags: { "colony-bridge": { externalId: targetKey } },
			name: "Colony Planner",
			text: { format: 1 },
			type: "text",
		});
		expect(eachWriteHadFreshOptions(world)).toBe(true);
	});

	test("finds the folder by flag after the GM renames it, and makes no second one", async () => {
		const world = createFoundryWorld();
		await apply(world, row(upsertKind, targetKey, payload));
		const folder = world.folders[0];
		await folder?.update({ name: "My folder" });
		await apply(world, row(upsertKind, `${targetKey}0`, payload, 2));
		expect(world.folders).toHaveLength(1);
		expect(world.entryFor(`${targetKey}0`)?.data.folder).toBe(folder?.id);
	});

	test("updates the entry's name and its own page in place, leaving the GM's folder, ownership and pages", async () => {
		const world = createFoundryWorld();
		await apply(world, row(upsertKind, targetKey, payload));
		const entry = world.entryFor(targetKey);
		await entry?.update({ folder: "gm-folder", "ownership.default": 3 });
		await entry?.createEmbeddedDocuments("JournalEntryPage", [{ name: "GM notes" }]);
		const renamed = { ...payload, displayName: "Renamed", name: "Renamed", title: "Renamed" };
		world.writes.length = 0;
		await apply(world, row(upsertKind, targetKey, renamed, 2));
		expect(world.entries).toHaveLength(1);
		expect(world.writes.map((write) => write.kind)).toEqual(["page update", "entry update"]);
		expect(entry?.data).toMatchObject({
			folder: "gm-folder",
			name: "Renamed",
			ownership: { default: 3 },
		});
		expect(entry?.pageList.map((page) => page.data.name)).toEqual(["Colony Planner", "GM notes"]);
		expect(bridgePageContent(world, targetKey)).toContain("Renamed");
		expect(eachWriteHadFreshOptions(world)).toBe(true);
	});

	test("a withdraw hides the entry from players, renamed and blanked, without deleting it, and the next upsert shows it again", async () => {
		const world = createFoundryWorld();
		await apply(world, row(upsertKind, targetKey, payload));
		const pageBefore = bridgePageContent(world, targetKey);
		await apply(world, row(withdrawKind, targetKey, {}, 2));
		expect(ownership(world, targetKey)).toBe(0);
		expect(world.entryFor(targetKey)?.getFlag("colony-bridge", "withdrawn")).toBe(true);
		expect(world.entryFor(targetKey)?.data.name).toBe("Hidden on Colony Planner");
		expect(bridgePageContent(world, targetKey)).toBe("");
		await apply(world, row(upsertKind, targetKey, payload, 3));
		expect(ownership(world, targetKey)).toBe(2);
		expect(world.entryFor(targetKey)?.getFlag("colony-bridge", "withdrawn")).toBe(false);
		expect(world.entryFor(targetKey)?.data.name).toBe(name);
		expect(bridgePageContent(world, targetKey)).toBe(pageBefore);
		expect(world.entries).toHaveLength(1);
		expect(eachWriteHadFreshOptions(world)).toBe(true);
	});

	test("a withdraw sets every player's own level to None, and keeps the GMs' levels and the GM's pages", async () => {
		const world = createFoundryWorld();
		await apply(world, row(upsertKind, targetKey, payload));
		const entry = world.entryFor(targetKey);
		await entry?.update({ "ownership.gm-2": 3, "ownership.player-1": 2 });
		await entry?.createEmbeddedDocuments("JournalEntryPage", [
			{ name: "GM notes", text: { content: "Plans" } },
		]);
		await apply(world, row(withdrawKind, targetKey, {}, 2));
		expect(entry?.data.ownership).toEqual({ default: 0, "gm-2": 3, "player-1": 0 });
		expect(entry?.pageList.map((page) => page.data.name)).toEqual(["Colony Planner", "GM notes"]);
		expect(readField(entry?.pageList[1]?.data.text, "content")).toBe("Plans");
	});

	test.each(["entry update", "page update"] as const)(
		"a withdraw whose %s a hook cancels fails, and its replay hides the entry",
		async (cancelled) => {
			const world = createFoundryWorld();
			await apply(world, row(upsertKind, targetKey, payload));
			await world.entryFor(targetKey)?.update({ "ownership.player-1": 2 });
			world.cancelNext.add(cancelled);
			await expect(apply(world, row(withdrawKind, targetKey, {}, 2))).rejects.toThrow(
				"Foundry didn't hide 1 journal entry from players.",
			);
			await apply(world, row(withdrawKind, targetKey, {}, 2));
			expect(world.entryFor(targetKey)?.data.ownership).toEqual({ default: 0, "player-1": 0 });
			expect(world.entryFor(targetKey)?.data.name).toBe("Hidden on Colony Planner");
			expect(bridgePageContent(world, targetKey)).toBe("");
		},
	);

	test("a withdraw of an entry Foundry never had succeeds and writes nothing", async () => {
		const world = createFoundryWorld();
		await apply(world, row(withdrawKind, targetKey, {}));
		expect(world.writes).toEqual([]);
	});

	test("a replayed upsert or withdraw leaves one entry in the same state", async () => {
		const world = createFoundryWorld();
		await apply(world, row(upsertKind, targetKey, payload));
		await apply(world, row(upsertKind, targetKey, payload));
		expect(world.entries).toHaveLength(1);
		expect(world.folders).toHaveLength(1);
		await apply(world, row(withdrawKind, targetKey, {}, 2));
		await apply(world, row(withdrawKind, targetKey, {}, 2));
		expect(ownership(world, targetKey)).toBe(0);
		expect(world.entries).toHaveLength(1);
	});

	test("payloads of the wrong shape fail the row", async () => {
		const world = createFoundryWorld();
		const adapter = adapterFor(world);
		await expect(
			adapter.appliers[upsertKind]?.(row(upsertKind, targetKey, { ...payload, siteUrl: 7 })),
		).rejects.toThrow("payload wasn't understood");
		await expect(
			adapter.appliers[withdrawKind]?.(row(withdrawKind, targetKey, null)),
		).rejects.toThrow(`The ${withdrawKind} row's payload wasn't understood.`);
		expect(world.writes).toEqual([]);
	});
});

describe("guards", () => {
	test.each([
		["NPC_UPSERT", { ...npc, status: "Alive" }],
		["NPC_UPSERT", { ...npc, relation: "FRIEND" }],
		["NPC_UPSERT", { ...npc, level: 2.5 }],
		["NPC_UPSERT", { ...npc, aliases: null }],
		["NPC_UPSERT", { ...npc, currentPlace: undefined }],
		["QUEST_UPSERT", { ...quest, kind: "SIDE" }],
		["QUEST_UPSERT", { ...quest, rewardFavor: "2" }],
		["QUEST_UPSERT", { ...quest, loot: [{ name: "Dagger", quantity: 1 }] }],
		["PLACE_UPSERT", { ...place, playerNotes: null }],
		["NPC_WITHDRAW", { displayName: "Brenna" }],
		["PLACE_WITHDRAW", []],
	])("%s refuses %j", async (kind, payload) => {
		const world = createFoundryWorld();
		await expect(apply(world, row(kind, "npc:1", payload))).rejects.toThrow(
			"payload wasn't understood",
		);
		expect(world.writes).toEqual([]);
	});
});

describe("partial failures finish on replay", () => {
	test("an entry that failed after its folder was made lands in that folder", async () => {
		const world = createFoundryWorld();
		world.failNext.add("entry create");
		await expect(apply(world, row("NPC_UPSERT", "npc:1", npc))).rejects.toThrow();
		expect(world.folders).toHaveLength(1);
		expect(world.entries).toHaveLength(0);
		await apply(world, row("NPC_UPSERT", "npc:1", npc));
		expect(world.folders).toHaveLength(1);
		expect(world.entryFor("npc:1")?.data.folder).toBe(world.folders[0]?.id);
	});

	test("a withdrawn entry stays hidden until its new page is written", async () => {
		const world = createFoundryWorld();
		await apply(world, row("NPC_UPSERT", "npc:1", npc));
		await apply(world, row("NPC_WITHDRAW", "npc:1", {}, 2));
		world.failNext.add("page update");
		await expect(
			apply(world, row("NPC_UPSERT", "npc:1", { ...npc, displayName: "Known name" }, 3)),
		).rejects.toThrow();
		expect(ownership(world, "npc:1")).toBe(0);
		world.failNext.add("entry update");
		await expect(
			apply(world, row("NPC_UPSERT", "npc:1", { ...npc, displayName: "Known name" }, 3)),
		).rejects.toThrow();
		expect(ownership(world, "npc:1")).toBe(0);
		expect(bridgePageContent(world, "npc:1")).toContain("Known name");
		await apply(world, row("NPC_UPSERT", "npc:1", { ...npc, displayName: "Known name" }, 3));
		expect(ownership(world, "npc:1")).toBe(2);
		expect(world.entryFor("npc:1")?.data.name).toBe("Known name");
	});

	test("an entry whose bridge page the GM deleted gets the page back", async () => {
		const world = createFoundryWorld();
		world.addEntry({ flags: { "colony-bridge": { externalId: "place:2" } }, name: "Old Tower" });
		await apply(world, row("PLACE_UPSERT", "place:2", place));
		expect(world.writes.map((write) => write.kind)).toEqual(["page create", "entry update"]);
		expect(bridgePageContent(world, "place:2")).toContain("A ruined tower.");
	});
});

describe("creates a hook cancels", () => {
	test("an upsert whose entry create is cancelled fails, and its replay makes the entry", async () => {
		const world = createFoundryWorld();
		world.cancelNext.add("entry create");
		await expect(apply(world, row("NPC_UPSERT", "npc:1", npc))).rejects.toThrow(
			"Foundry didn't make the journal entry for npc:1.",
		);
		expect(world.entries).toEqual([]);
		await apply(world, row("NPC_UPSERT", "npc:1", npc));
		expect(world.entryFor("npc:1")?.data.name).toBe("Brenna <Ironhand>");
	});

	test("an upsert whose bridge page create is cancelled fails, and its replay adds the page", async () => {
		const world = createFoundryWorld();
		world.addEntry({ flags: { "colony-bridge": { externalId: "place:2" } }, name: "Old Tower" });
		world.cancelNext.add("page create");
		await expect(apply(world, row("PLACE_UPSERT", "place:2", place))).rejects.toThrow(
			"Foundry didn't add the Colony Planner page.",
		);
		await apply(world, row("PLACE_UPSERT", "place:2", place));
		expect(bridgePageContent(world, "place:2")).toContain("A ruined tower.");
	});

	test("the colony summary whose entry create is cancelled fails", async () => {
		const world = createFoundryWorld();
		world.cancelNext.add("entry create");
		await expect(
			apply(
				world,
				row("COLONY_SUMMARY", "colony", {
					campaignName: "Port Eldari",
					day: 22,
					nextGoodsShipDay: null,
					nextShipmentDay: null,
					resources: [],
					siteUrl: "https://colony.benthedev.com/c/port-eldari",
					workforce: [],
				}),
			),
		).rejects.toThrow("Foundry didn't make the journal entry for colony.");
		expect(world.entries).toEqual([]);
	});
});

describe("withdrawBridgedEntries", () => {
	test("withdraws every NPC, place and quest entry in one write, and leaves the rest", async () => {
		const world = createFoundryWorld();
		await apply(world, row("NPC_UPSERT", "npc:1", npc));
		await apply(world, row("PLACE_UPSERT", "place:2", place));
		await apply(world, row("QUEST_UPSERT", "quest:3", quest));
		world.addEntry({
			flags: { "colony-bridge": { externalId: "colony" } },
			name: "Colony",
			ownership: { default: 2 },
		});
		world.addEntry({ name: "GM's own", ownership: { default: 2 } });
		await world.entryFor("place:2")?.update({ "ownership.player-1": 3 });
		world.writes.length = 0;
		await adapterFor(world).withdrawBridgedEntries();
		expect(world.writes.map((write) => write.kind)).toEqual([
			"entries update",
			"page update",
			"page update",
			"page update",
		]);
		expect(world.entries.map((entry) => readField(entry.data.ownership, "default"))).toEqual([
			0, 0, 0, 2, 2,
		]);
		expect(world.entries.map((entry) => entry.data.name)).toEqual([
			"Hidden on Colony Planner",
			"Hidden on Colony Planner",
			"Hidden on Colony Planner",
			"Colony",
			"GM's own",
		]);
		expect(readField(world.entryFor("place:2")?.data.ownership, "player-1")).toBe(0);
		expect(bridgePageContent(world, "npc:1")).toBe("");
		expect(world.entryFor("quest:3")?.getFlag("colony-bridge", "withdrawn")).toBe(true);
		expect(eachWriteHadFreshOptions(world)).toBe(true);
		world.writes.length = 0;
		await adapterFor(world).withdrawBridgedEntries();
		expect(world.writes.map((write) => write.kind)).toEqual(["entries update"]);
		await apply(world, row("NPC_UPSERT", "npc:1", npc, 4));
		expect(ownership(world, "npc:1")).toBe(2);
		expect(world.entryFor("npc:1")?.data.name).toBe("Brenna <Ironhand>");
	});

	test.each(["entries update", "page update"] as const)(
		"fails when a hook cancels its %s, and its retry hides every entry",
		async (cancelled) => {
			const world = createFoundryWorld();
			await apply(world, row("NPC_UPSERT", "npc:1", npc));
			await apply(world, row("PLACE_UPSERT", "place:2", place));
			world.cancelNext.add(cancelled);
			await expect(adapterFor(world).withdrawBridgedEntries()).rejects.toThrow(
				"Foundry didn't hide",
			);
			await adapterFor(world).withdrawBridgedEntries();
			expect(world.entries.map((entry) => entry.data.name)).toEqual([
				"Hidden on Colony Planner",
				"Hidden on Colony Planner",
			]);
			expect(bridgePageContent(world, "npc:1")).toBe("");
			expect(bridgePageContent(world, "place:2")).toBe("");
		},
	);

	test("writes nothing when the bridge made no entries", async () => {
		const world = createFoundryWorld();
		await adapterFor(world).withdrawBridgedEntries();
		expect(world.writes).toEqual([]);
	});
});

describe("rendering", () => {
	test("an NPC's page escapes every string, keeps line breaks and skips blanks and UNKNOWN", async () => {
		const world = createFoundryWorld();
		await apply(world, row("NPC_UPSERT", "npc:1", npc));
		const html = String(bridgePageContent(world, "npc:1"));
		expect(html).toContain("<h2>Brenna &lt;Ironhand&gt;</h2>");
		expect(html).toContain("<li><strong>Also known as:</strong> The Smith</li>");
		expect(html).toContain("<li><strong>Now at:</strong> The &lt;Forge&gt;</li>");
		expect(html).toContain("<li><strong>Status:</strong> Alive</li>");
		expect(html).toContain("<li><strong>Relation:</strong> Ally</li>");
		expect(html).toContain("<li><strong>Level:</strong> 3</li>");
		expect(html).toContain("<p>Tall.<br>Scarred hands.</p>");
		expect(html).toContain("Gruff &amp; kind");
		expect(html).not.toContain("Health");
		expect(html).not.toContain("Labels");
		expect(html).not.toContain("From:");
		expect(html).not.toContain("Backstory");
		expect(html).not.toContain("Player notes");
		expect(html).toContain(
			'<a href="https://colony.benthedev.com/c/port-eldari/npcs/1">Open Brenna &lt;Ironhand&gt; on Colony Planner</a>',
		);
	});

	test("a quest's page shows its reward, favor and loot, escaped, and links only to a web address", async () => {
		const world = createFoundryWorld();
		await apply(
			world,
			row("QUEST_UPSERT", "quest:3", { ...quest, siteUrl: "javascript:alert(1)" }),
		);
		const html = String(bridgePageContent(world, "quest:3"));
		expect(html).toContain("<li><strong>Kind:</strong> Mainland</li>");
		expect(html).toContain("<li><strong>Given by:</strong> Brenna</li>");
		expect(html).toContain("<li><strong>Favor:</strong> +2 with The Guild</li>");
		expect(html).toContain("<li>Dagger &lt;+1&gt; (2): Sharp</li>");
		expect(html).not.toContain("Resolved");
		expect(html).not.toContain("href");
	});

	test("a quest without a patron shows no favor, and a resolved one shows its day", async () => {
		const world = createFoundryWorld();
		await apply(
			world,
			row("QUEST_UPSERT", "quest:3", {
				...quest,
				giver: null,
				loot: [],
				patron: null,
				resolvedDay: 12,
				reward: "",
				rewardFavor: 0,
				status: "COMPLETED",
			}),
		);
		const html = String(bridgePageContent(world, "quest:3"));
		expect(html).toContain("<li><strong>Resolved:</strong> Day 12</li>");
		expect(html).toContain("<li><strong>Status:</strong> Completed</li>");
		expect(html).not.toContain("Favor");
		expect(html).not.toContain("Given by");
		expect(html).not.toContain("Loot");
		expect(html).not.toContain("Reward");
	});

	test("a place's page escapes its text", async () => {
		const world = createFoundryWorld();
		await apply(
			world,
			row("PLACE_UPSERT", "place:2", { ...place, description: "<img src=x onerror=alert(1)>" }),
		);
		const html = String(bridgePageContent(world, "place:2"));
		expect(html).toContain("&lt;img src=x onerror=alert(1)&gt;");
		expect(html).not.toContain("<img");
		expect(html).toContain("<h3>Player notes</h3><p>Haunted?</p>");
	});
});

const pollPosted = {
	allowMultiple: false,
	closesAt: "2026-11-20T03:00:00Z",
	options: ["North <road>", "South"],
	pollsUrl: "https://colony.benthedev.com/c/port-eldari/polls",
	question: "Which way?",
	voting: "SITE",
};

const pollClosed = {
	closedAt: "2026-11-20T03:00:00Z",
	options: [
		{ count: 3, label: "North <road>", otherVotes: 1 },
		{ count: 1, label: "South", otherVotes: 0 },
	],
	question: "Which way?",
};

const delivery = {
	items: [
		{ compendiumUuid: null, name: "Boots <of Striding>", quantity: 1 },
		{
			compendiumUuid: "Compendium.pf2e.equipment-srd.Item.abcdefghij123456",
			name: "Rope",
			quantity: 3,
		},
	],
	paidDay: 14,
	summary: "20 food, 3 Farmers, Boots <of Striding>",
	totalGold: 1200,
};

const deliveryUndone = {
	items: [{ name: "Rope", quantity: 3 }],
	summary: "20 food, Rope",
};

function chatData(world: FoundryWorld, index = 0) {
	return world.messages[index]?.data;
}

async function worldWithDelivery(options?: Parameters<typeof createFoundryWorld>[0]) {
	const world = createFoundryWorld(options);
	await apply(world, row("DELIVERY", "delivery:9", delivery));
	return world;
}

describe("chat kinds", () => {
	test("POLL_POSTED posts the question, options, how to pick, the close time and the vote link", async () => {
		const world = createFoundryWorld();
		await apply(world, row("POLL_POSTED", "poll:5:posted", pollPosted));
		const message = chatData(world);
		expect(message?.speaker).toEqual({ alias: "Colony Planner" });
		expect(message?.flags).toEqual({ "colony-bridge": { externalId: "poll:5:posted" } });
		expect(message?.whisper).toBeUndefined();
		const content = String(message?.content);
		expect(content).toContain("<strong>New poll:</strong> Which way?");
		expect(content).toContain("<li>North &lt;road&gt;</li><li>South</li>");
		expect(content).toContain("Pick one. Closes Nov");
		expect(content).toContain("2026");
		expect(content).toContain(
			'<a href="https://colony.benthedev.com/c/port-eldari/polls">Vote on Colony Planner</a>',
		);
		expect(eachWriteHadFreshOptions(world)).toBe(true);
	});

	test("POLL_POSTED for a Discord poll says to vote in Discord, and a multiple choice one says pick any", async () => {
		const world = createFoundryWorld();
		await apply(
			world,
			row("POLL_POSTED", "poll:5:posted", {
				...pollPosted,
				allowMultiple: true,
				voting: "DISCORD",
			}),
		);
		const content = String(chatData(world)?.content);
		expect(content).toContain("Pick any.");
		expect(content).toContain("<p>Vote in Discord.</p>");
		expect(content).not.toContain("href");
	});

	test("POLL_CLOSED posts each option's count in order, naming no winner", async () => {
		const world = createFoundryWorld();
		await apply(world, row("POLL_CLOSED", "poll:5:closed", pollClosed));
		expect(chatData(world)?.content).toBe(
			"<p><strong>Poll closed:</strong> Which way?</p><ul><li>North &lt;road&gt;: 3 votes</li><li>South: 1 vote</li></ul>",
		);
	});

	test("DELIVERY posts the goods and lists every item to hand out", async () => {
		const world = createFoundryWorld();
		await apply(world, row("DELIVERY", "delivery:9", delivery));
		expect(chatData(world)?.content).toBe(
			"<p>Goods unloaded on Day 14, paid 1,200 gold: 20 food, 3 Farmers, Boots &lt;of Striding&gt;</p><p>Hand these out yourself:</p><ul><li>Boots &lt;of Striding&gt; (1)</li><li>Rope (3)</li></ul>",
		);
		expect(world.partyItems).toEqual([]);
	});

	test("DELIVERY_UNDONE after its DELIVERY is whispered to every GM and only the GMs", async () => {
		const world = await worldWithDelivery();
		await apply(world, row("DELIVERY_UNDONE", "delivery-undone:9", deliveryUndone, 2));
		const message = chatData(world, 1);
		expect(message?.flags).toEqual({ "colony-bridge": { externalId: "delivery-undone:9" } });
		expect(message?.whisper).toEqual(["gm-1", "gm-2"]);
		expect(message?.content).toBe(
			"<p><strong>A payment was undone on Colony Planner:</strong> 20 food, Rope</p><p>If any of these reached the party, remove them by hand:</p><ul><li>Rope (3)</li></ul>",
		);
	});

	test("DELIVERY_UNDONE posts nothing and succeeds when this world never posted its delivery", async () => {
		const world = await worldWithDelivery();
		await apply(world, row("DELIVERY_UNDONE", "delivery-undone:8", deliveryUndone, 2));
		expect(world.messages).toHaveLength(1);
	});

	test("DELIVERY_UNDONE with a target key of the wrong shape fails the row", async () => {
		const world = await worldWithDelivery();
		await expect(
			apply(world, row("DELIVERY_UNDONE", "delivery:9", deliveryUndone, 2)),
		).rejects.toThrow("The undone delivery's target key wasn't understood.");
		expect(world.messages).toHaveLength(1);
	});

	test("DELIVERY_UNDONE refuses to post when Foundry lists no GM, since no recipients means everyone", async () => {
		const world = createFoundryWorld({ users: [{ id: "player-1", isGM: false }] });
		world.addMessage(
			{ flags: { "colony-bridge": { externalId: "delivery:9" } } },
			{ id: "gm-gone", isGM: true },
		);
		await expect(
			apply(world, row("DELIVERY_UNDONE", "delivery-undone:9", deliveryUndone, 2)),
		).rejects.toThrow("Foundry has no GM to whisper the undone delivery to.");
		expect(world.messages).toHaveLength(1);
	});

	test.each([
		["POLL_POSTED", "poll:5:posted", pollPosted, 0],
		["POLL_CLOSED", "poll:5:closed", pollClosed, 0],
		["DELIVERY", "delivery:9", delivery, 0],
		["DELIVERY_UNDONE", "delivery-undone:9", deliveryUndone, 1],
	])(
		"a repeat of %s posts nothing, and a failed post posts once on retry",
		async (kind, key, payload, earlierMessages) => {
			const world = earlierMessages > 0 ? await worldWithDelivery() : createFoundryWorld();
			world.failNext.add("chat create");
			await expect(apply(world, row(kind, key, payload))).rejects.toThrow();
			expect(world.messages).toHaveLength(earlierMessages);
			await apply(world, row(kind, key, payload));
			await apply(world, row(kind, key, payload));
			expect(world.messages).toHaveLength(earlierMessages + 1);
		},
	);

	test.each([
		["POLL_POSTED", { ...pollPosted, closesAt: "soon" }],
		["POLL_POSTED", { ...pollPosted, voting: "EMAIL" }],
		["POLL_CLOSED", { ...pollClosed, options: [{ count: "3", label: "North" }] }],
		["DELIVERY", { ...delivery, items: [{ compendiumUuid: 5, name: "Rope", quantity: 1 }] }],
		["DELIVERY_UNDONE", { summary: "x" }],
	])("%s with a payload of the wrong shape fails the row", async (kind, payload) => {
		const world = createFoundryWorld();
		await expect(apply(world, row(kind, "key", payload))).rejects.toThrow(
			"payload wasn't understood",
		);
		expect(world.messages).toEqual([]);
	});

	test.each([
		["POLL_POSTED", "poll:5:posted", pollPosted],
		["POLL_CLOSED", "poll:5:closed", pollClosed],
		["DELIVERY", "delivery:9", delivery],
		["DELIVERY_UNDONE", "delivery-undone:9", deliveryUndone],
	])("%s whose post a hook cancels fails, and its retry posts once", async (kind, key, payload) => {
		const world = kind === "DELIVERY_UNDONE" ? await worldWithDelivery() : createFoundryWorld();
		const before = world.messages.length;
		world.cancelNext.add("chat create");
		await expect(apply(world, row(kind, key, payload, 2))).rejects.toThrow(
			`Foundry didn't post the ${kind} message in chat.`,
		);
		expect(world.messages).toHaveLength(before);
		await apply(world, row(kind, key, payload, 2));
		expect(world.messages).toHaveLength(before + 1);
	});
});

describe("documents players planted", () => {
	const plantedFlags = (externalId: string) => ({ "colony-bridge": { externalId } });

	test.each([
		["a level of their own", { ownership: { default: 0, "player-1": 3 } }],
		["their last edit", { _stats: { lastModifiedBy: "player-1" }, ownership: { default: 2 } }],
	])(
		"an upsert ignores an entry with its key that a player could change (%s) and makes its own",
		async (_case, planted) => {
			const world = createFoundryWorld();
			const plant = world.addEntry({ ...planted, flags: plantedFlags("npc:1"), name: "Planted" });
			await apply(world, row("NPC_UPSERT", "npc:1", npc));
			expect(world.entries).toHaveLength(2);
			expect(plant.data.name).toBe("Planted");
			expect(world.writes.map((write) => write.kind)).toEqual(["folder create", "entry create"]);
			expect(world.entries[1]?.data.name).toBe("Brenna <Ironhand>");
		},
	);

	test("an entry whose players' own levels are all None is the bridge's", async () => {
		const world = createFoundryWorld();
		world.addEntry({
			_stats: { lastModifiedBy: "gm-2" },
			flags: plantedFlags("npc:1"),
			name: "Old",
			ownership: { default: 0, "gm-1": 3, "player-1": 0 },
		});
		await apply(world, row("NPC_UPSERT", "npc:1", npc));
		expect(world.entries).toHaveLength(1);
		expect(world.entries[0]?.data.name).toBe("Brenna <Ironhand>");
	});

	test("an upsert ignores a folder a player made with the folder's key", async () => {
		const world = createFoundryWorld();
		const plant = world.addFolder({
			_stats: { lastModifiedBy: "player-1" },
			flags: plantedFlags("folder:npcs"),
			name: "Mine",
			type: "JournalEntry",
		});
		await apply(world, row("NPC_UPSERT", "npc:1", npc));
		expect(world.folders).toHaveLength(2);
		expect(world.entryFor("npc:1")?.data.folder).not.toBe(plant.id);
		expect(world.entryFor("npc:1")?.data.folder).toBe(world.folders[1]?.id);
	});

	test("a withdraw hides a player's entry with its key too, since hiding can't leak", async () => {
		const world = createFoundryWorld();
		const plant = world.addEntry({
			flags: plantedFlags("npc:1"),
			name: "Planted",
			ownership: { default: 2, "player-1": 3 },
		});
		await apply(world, row("NPC_WITHDRAW", "npc:1", {}));
		expect(plant.data.ownership).toEqual({ default: 0, "player-1": 0 });
	});

	test.each([
		["POLL_POSTED", "poll:5:posted", pollPosted],
		["POLL_CLOSED", "poll:5:closed", pollClosed],
		["DELIVERY", "delivery:9", delivery],
		["DELIVERY_UNDONE", "delivery-undone:9", deliveryUndone],
	])(
		"a player's chat message flagged with the key doesn't stop %s from posting",
		async (kind, key, payload) => {
			const world = kind === "DELIVERY_UNDONE" ? await worldWithDelivery() : createFoundryWorld();
			const before = world.messages.length;
			world.addMessage({ flags: plantedFlags(key) });
			await apply(world, row(kind, key, payload, 2));
			expect(world.messages).toHaveLength(before + 2);
			expect(world.messages.at(-1)?.author?.isGM).toBe(true);
		},
	);

	test("DELIVERY_UNDONE whispers nothing when only a player's message names the delivery", async () => {
		const world = createFoundryWorld();
		world.addMessage({ flags: plantedFlags("delivery:9") });
		await apply(world, row("DELIVERY_UNDONE", "delivery-undone:9", deliveryUndone, 2));
		expect(world.messages).toHaveLength(1);
		expect(world.writes).toEqual([]);
	});
});
