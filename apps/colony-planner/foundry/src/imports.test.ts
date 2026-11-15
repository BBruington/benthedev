import { describe, expect, test } from "bun:test";
import type { BridgeCondition } from "./bridge";
import type { Pf2eDepositor } from "./deposits";
import type {
	ContextMenuEntry,
	ContextMenuEntryV13,
	ContextMenuTarget,
	ContextOptionsHook,
	DialogInputOptions,
	FoundrySourceDocument,
} from "./foundry-types";
import {
	contextMenuEntry,
	type ImportContext,
	type RichTextReader,
	readDepositGold,
	registerContextMenus,
	selectImportReaders,
} from "./imports";
import { settle } from "./testing/fakes";
import type { InboundEvent } from "./wire";

// Stands in for htmlToPlainText (html.test.ts covers it): text before "||" is public, after it secret.
const readRichText: RichTextReader = (html, secretBlocks) => {
	const [text = "", secret = ""] = html.split("||");
	return secretBlocks === "omit"
		? { secrets: secret, text }
		: { secrets: "", text: [text, secret].filter((part) => part !== "").join("\n") };
};

const actorId = "abcdefghij123456";

function sourceDocument(overrides: Partial<FoundrySourceDocument> & Record<string, unknown>) {
	return {
		id: actorId,
		name: "Brother Aldo",
		system: {},
		type: "npc",
		uuid: `Actor.${actorId}`,
		...overrides,
	};
}

describe("NPC readers", () => {
	test("PF2e NPC: level, public notes as appearance with secret blocks moved to the private notes", () => {
		const actor = sourceDocument({
			system: {
				details: {
					level: { value: 3 },
					privateNotes: "Owes the Duke.",
					publicNotes: "A tall monk.||Secretly a cultist.",
				},
			},
		});
		expect(selectImportReaders("pf2e", readRichText).npc(actor)).toEqual({
			appearance: "A tall monk.",
			characterClass: "",
			dmNotes: "Owes the Duke.\n\nSecretly a cultist.",
			foundryUuid: `Actor.${actorId}`,
			level: 3,
			name: "Brother Aldo",
			race: "",
		});
	});

	test("PF2e: a level -1 or fractional creature arrives with no level", () => {
		const readers = selectImportReaders("pf2e", readRichText);
		const levelOf = (value: unknown) =>
			readers.npc(sourceDocument({ system: { details: { level: { value } } } })).level;
		expect([
			levelOf(-1),
			levelOf(2.5),
			levelOf("3"),
			levelOf(0),
			levelOf(100),
			levelOf(101),
		]).toEqual([null, null, null, 0, 100, null]);
	});

	test("PF2e character: ancestry, class and biography appearance, and no DM notes", () => {
		const actor = sourceDocument({
			ancestry: { name: "Human" },
			class: { name: "Fighter" },
			name: "Valeros",
			system: {
				details: { biography: { appearance: "Scarred." }, level: { value: 5 } },
			},
			type: "character",
		});
		expect(selectImportReaders("pf2e", readRichText).npc(actor)).toMatchObject({
			appearance: "Scarred.",
			characterClass: "Fighter",
			dmNotes: "",
			level: 5,
			race: "Human",
		});
	});

	test("dnd5e character: level, race item, every class, public and full biography", () => {
		const actor = sourceDocument({
			classes: { fighter: { name: "Fighter" }, wizard: { name: "Wizard" } },
			system: {
				details: {
					biography: { public: "Green cloak.", value: "Exiled prince." },
					level: 7,
					race: { name: "Elf" },
				},
			},
			type: "character",
		});
		expect(selectImportReaders("dnd5e", readRichText).npc(actor)).toMatchObject({
			appearance: "Green cloak.",
			characterClass: "Fighter, Wizard",
			dmNotes: "Exiled prince.",
			level: 7,
			race: "Elf",
		});
	});

	test("dnd5e NPC: a challenge rating isn't a level, and a race may be plain text", () => {
		const actor = sourceDocument({
			system: {
				details: { biography: { public: "", value: "" }, cr: 2, level: 4, race: "Goblin" },
			},
		});
		expect(selectImportReaders("dnd5e", readRichText).npc(actor)).toMatchObject({
			level: null,
			race: "Goblin",
		});
	});

	test("other systems send the name only", () => {
		const actor = sourceDocument({ system: { details: { level: { value: 3 } } } });
		expect(selectImportReaders("swade", readRichText).npc(actor)).toEqual({
			appearance: "",
			characterClass: "",
			dmNotes: "",
			foundryUuid: `Actor.${actorId}`,
			level: null,
			name: "Brother Aldo",
			race: "",
		});
	});

	test("names become one line within their limits, and prose is clipped to 4,000", () => {
		const actor = sourceDocument({
			name: `  Brother\n${"A".repeat(100)}`,
			system: { details: { publicNotes: "x".repeat(5000) } },
		});
		const payload = selectImportReaders("pf2e", readRichText).npc(actor);
		expect(Array.from(payload.name)).toHaveLength(80);
		expect(payload.name).toStartWith("Brother A");
		expect(payload.name).toEndWith("…");
		expect(Array.from(payload.appearance)).toHaveLength(4000);
	});
});

const packItemUuid = "Compendium.pf2e.equipment-srd.Item.abcdefghij123456";

function itemDocument(system: unknown, overrides: Partial<FoundrySourceDocument> = {}) {
	return sourceDocument({
		name: "Rope",
		system,
		type: "equipment",
		uuid: `Item.${actorId}`,
		...overrides,
	});
}

describe("item readers", () => {
	const pf2eGold = (value: unknown, per?: number) =>
		selectImportReaders("pf2e", readRichText).item(itemDocument({ price: { per, value } }))
			.priceGold;
	const dnd5eGold = (value: unknown, denomination: string) =>
		selectImportReaders("dnd5e", readRichText).item(
			itemDocument({ price: { denomination, value } }),
		).priceGold;

	test("PF2e prices count pp 10 gp, 10 sp 1 gp and 100 cp 1 gp, and only whole gold is sent", () => {
		expect(pf2eGold({ gp: 5 })).toBe(5);
		expect(pf2eGold({ cp: 0, gp: 2, pp: 1, sp: 10 })).toBe(13);
		expect(pf2eGold({ cp: 100 })).toBe(1);
		expect(pf2eGold({ sp: 5 })).toBeNull();
		expect(pf2eGold({ gp: 1.5 })).toBeNull();
		expect(pf2eGold({ gp: "5" })).toBeNull();
		expect(pf2eGold({})).toBe(0);
	});

	test("PF2e: a bundle's price or a Starfinder amount arrives blank", () => {
		expect(pf2eGold({ gp: 5 }, 10)).toBeNull();
		expect(pf2eGold({ gp: 5 }, 1)).toBe(5);
		expect(pf2eGold({ credits: 5 })).toBeNull();
		expect(pf2eGold({ gp: 5, upb: 2 })).toBeNull();
	});

	test("dnd5e prices are one value in one denomination, and electrum arrives blank", () => {
		expect(dnd5eGold(5, "gp")).toBe(5);
		expect(dnd5eGold(2, "pp")).toBe(20);
		expect(dnd5eGold(1.5, "pp")).toBe(15);
		expect(dnd5eGold(10, "sp")).toBe(1);
		expect(dnd5eGold(300, "cp")).toBe(3);
		expect(dnd5eGold(5, "sp")).toBeNull();
		expect(dnd5eGold(0.5, "gp")).toBeNull();
		expect(dnd5eGold(2, "ep")).toBeNull();
		expect(dnd5eGold(2, "dragons")).toBeNull();
		expect(dnd5eGold(0.001, "cp")).toBeNull();
	});

	test("the description leaves secret blocks out, and PF2e's GM description is never read", () => {
		const item = itemDocument({
			description: { gm: "Cursed.", value: "Fifty feet of hemp.||Actually silk." },
			price: { value: { gp: 1 } },
		});
		expect(selectImportReaders("pf2e", readRichText).item(item)).toEqual({
			compendiumUuid: null,
			description: "Fifty feet of hemp.",
			foundryUuid: `Item.${actorId}`,
			name: "Rope",
			priceGold: 1,
		});
	});

	test("the compendium UUID is the pack's own, or a compendium source, in §13.6's form only", () => {
		const readers = selectImportReaders("pf2e", readRichText);
		const fromPack = itemDocument({}, { pack: "pf2e.equipment-srd", uuid: packItemUuid });
		const dropped = itemDocument({}, { _stats: { compendiumSource: packItemUuid } });
		const worldSource = itemDocument({}, { _stats: { compendiumSource: `Item.${actorId}` } });
		const worldPack = itemDocument({}, { pack: "world.loot", uuid: `Item.${actorId}` });
		expect(readers.item(fromPack).compendiumUuid).toBe(packItemUuid);
		expect(readers.item(dropped).compendiumUuid).toBe(packItemUuid);
		expect(readers.item(worldSource).compendiumUuid).toBeNull();
		expect(readers.item(worldPack).compendiumUuid).toBeNull();
	});

	test("other systems send the name, the UUIDs and no description or price", () => {
		const item = itemDocument(
			{ description: { value: "Words" }, price: { value: { gp: 5 } } },
			{ _stats: { compendiumSource: packItemUuid } },
		);
		expect(selectImportReaders("swade", readRichText).item(item)).toEqual({
			compendiumUuid: packItemUuid,
			description: "",
			foundryUuid: `Item.${actorId}`,
			name: "Rope",
			priceGold: null,
		});
	});
});

describe("contextMenuEntry", () => {
	const ran: string[] = [];
	const action = {
		icon: "<i></i>",
		label: "Send to Colony Planner",
		run: (target: ContextMenuTarget) => {
			ran.push(target.dataset.entryId ?? "");
			return Promise.resolve();
		},
		visible: () => true,
	};

	test("v13 entries are {name, icon, condition, callback}", () => {
		const entry = contextMenuEntry(13, action);
		expect(Object.keys(entry).sort()).toEqual(["callback", "condition", "icon", "name"]);
		expect(entry).toMatchObject({ icon: "<i></i>", name: "Send to Colony Planner" });
	});

	test("v14 entries are {label, icon, visible, onClick}, with the target second", async () => {
		const entry = contextMenuEntry(14, action);
		expect(Object.keys(entry).sort()).toEqual(["icon", "label", "onClick", "visible"]);
		if ("onClick" in entry) {
			await entry.onClick(new Event("click"), { dataset: { entryId: "abc" } });
		}
		expect(ran).toEqual(["abc"]);
	});
});

const partyId = "partyactor123456";
const characterId = "characterkyra123";

function fakeDepositor(
	id: string,
	type: Pf2eDepositor["type"],
	name: string,
): Pf2eDepositor & { uuid: string } {
	return {
		id,
		inventory: {
			addCurrency: () => Promise.resolve(),
			currency: { copperValue: 10_000 },
			removeCurrency: () => Promise.resolve(true),
		},
		name,
		type,
		uuid: `Actor.${id}`,
	};
}

const worldActors = new Map<string, unknown>([
	[actorId, sourceDocument({ system: { details: { level: { value: 3 } } } })],
	[partyId, fakeDepositor(partyId, "party", "The Wayfarers")],
	[characterId, fakeDepositor(characterId, "character", "Kyra")],
	[
		"embeddedactor123",
		sourceDocument({
			uuid: "Scene.abcdefghij123456.Token.abcdefghij123456.Actor.embeddedactor123",
		}),
	],
]);

const actorDirectory = { collection: { get: (id: string) => worldActors.get(id) } };
const actorPack = {
	collection: { getDocument: (id: string) => Promise.resolve(worldActors.get(id)) },
	pack: true,
};
const itemPack = {
	collection: {
		getDocument: () =>
			Promise.resolve(
				itemDocument(
					{ price: { value: { gp: 2 } } },
					{ pack: "pf2e.equipment-srd", uuid: packItemUuid },
				),
			),
	},
	pack: true,
};

function menusFor(options: {
	condition?: BridgeCondition;
	dialogAnswer?: unknown;
	generation?: number;
	systemId?: string;
}) {
	const hooks = new Map<string, ContextOptionsHook>();
	const notices: string[] = [];
	const queued: InboundEvent[] = [];
	const deposits: [string, number, string][] = [];
	const dialogs: DialogInputOptions[] = [];
	const status = { condition: options.condition ?? "running" };
	const context: ImportContext = {
		bridge: {
			linkId: () => "link-1",
			queueEvent: (event) => {
				queued.push(event);
				return true;
			},
			status: () => ({
				campaignName: "Port Eldari",
				condition: status.condition,
				connection: "live",
				lastProblem: undefined,
				linked: true,
			}),
		},
		createId: () => "event-1",
		deposits: {
			deposit: (party, gold, link) => {
				deposits.push([party.id, gold, link.id()]);
				return Promise.resolve();
			},
		},
		inputDialog: (dialog) => {
			dialogs.push(dialog);
			return Promise.resolve(options.dialogAnswer ?? null);
		},
		isCompendium: (application) =>
			typeof application === "object" && application !== null && "pack" in application,
		notifications: {
			error: (message) => notices.push(`error: ${message}`),
			info: (message) => notices.push(`info: ${message}`),
			warn: (message) => notices.push(`warn: ${message}`),
		},
		now: () => new Date("2026-11-14T10:00:00.000Z"),
		readers: selectImportReaders(options.systemId ?? "pf2e", readRichText),
		systemId: options.systemId ?? "pf2e",
	};
	registerContextMenus(
		{
			on: (name, callback) => {
				hooks.set(name, callback);
				return hooks.size;
			},
		},
		options.generation ?? 13,
		() => context,
		(error) => notices.push(`report: ${String(error)}`),
	);
	function entries(hook: string, application: unknown): ContextMenuEntryV13[] {
		const menuItems: ContextMenuEntry[] = [];
		hooks.get(hook)?.(application, menuItems);
		return menuItems.filter((entry): entry is ContextMenuEntryV13 => "callback" in entry);
	}
	const target = (entryId: string): ContextMenuTarget => ({ dataset: { entryId } });
	const visibleLabels = (hook: string, application: unknown, entryId: string) =>
		entries(hook, application)
			.filter((entry) => entry.condition(target(entryId)))
			.map((entry) => entry.name);
	return { deposits, dialogs, entries, hooks, notices, queued, status, target, visibleLabels };
}

describe("registerContextMenus", () => {
	test("registers the Actor and Item hooks, and every entry hides while the bridge isn't running here", () => {
		const menus = menusFor({ condition: "another-gm-is-active" });
		expect([...menus.hooks.keys()]).toEqual(["getActorContextOptions", "getItemContextOptions"]);
		expect(menus.visibleLabels("getActorContextOptions", actorDirectory, actorId)).toEqual([]);
		expect(menus.visibleLabels("getActorContextOptions", actorDirectory, partyId)).toEqual([]);
		expect(menus.visibleLabels("getItemContextOptions", itemPack, "x")).toEqual([]);
	});

	test("an NPC gets Send, a PF2e party only Deposit, a PF2e character both, and the party's placeholder rows nothing", () => {
		const menus = menusFor({});
		expect(menus.visibleLabels("getActorContextOptions", actorDirectory, actorId)).toEqual([
			"Send to Colony Planner",
		]);
		expect(menus.visibleLabels("getActorContextOptions", actorDirectory, partyId)).toEqual([
			"Deposit to colony…",
		]);
		expect(menus.visibleLabels("getActorContextOptions", actorDirectory, characterId)).toEqual([
			"Send to Colony Planner",
			"Deposit to colony…",
		]);
		expect(menus.visibleLabels("getActorContextOptions", actorDirectory, "otherParties")).toEqual(
			[],
		);
		expect(menus.visibleLabels("getActorContextOptions", actorDirectory, "")).toEqual([]);
	});

	test("a pack shows Send on every entry and never Deposit; only PF2e worlds deposit", () => {
		for (const depositorId of [partyId, characterId]) {
			expect(menusFor({}).visibleLabels("getActorContextOptions", actorPack, depositorId)).toEqual([
				"Send to Colony Planner",
			]);
			expect(
				menusFor({ systemId: "dnd5e" }).visibleLabels(
					"getActorContextOptions",
					actorDirectory,
					depositorId,
				),
			).toEqual(["Send to Colony Planner"]);
		}
	});

	test("Send queues an NPC_IMPORT and says so", async () => {
		const menus = menusFor({});
		const [send] = menus.entries("getActorContextOptions", actorDirectory);
		await send?.callback(menus.target(actorId));
		expect(menus.queued).toEqual([
			{
				id: "event-1",
				kind: "NPC_IMPORT",
				occurredAt: "2026-11-14T10:00:00.000Z",
				payload: {
					appearance: "",
					characterClass: "",
					dmNotes: "",
					foundryUuid: `Actor.${actorId}`,
					level: 3,
					name: "Brother Aldo",
					race: "",
				},
			},
		]);
		expect(menus.notices).toEqual(["info: Queued Brother Aldo for Colony Planner's inbox."]);
	});

	test("Send refuses a party from a pack, and an actor that isn't a world or pack actor", async () => {
		const menus = menusFor({});
		const [packSend] = menus.entries("getActorContextOptions", actorPack);
		await packSend?.callback(menus.target(partyId));
		const [directorySend] = menus.entries("getActorContextOptions", actorDirectory);
		await directorySend?.callback(menus.target("embeddedactor123"));
		expect(menus.queued).toEqual([]);
		expect(menus.notices).toEqual([
			"warn: A party can't become an NPC. Send its members to Colony Planner one at a time.",
			"warn: Colony Planner takes an actor from the Actors tab or a compendium, with a name.",
		]);
	});

	test("Send loads an item from its pack and queues an ITEM_IMPORT with its compendium UUID", async () => {
		const menus = menusFor({ generation: 13 });
		const [send] = menus.entries("getItemContextOptions", itemPack);
		await send?.callback(menus.target("abcdefghij123456"));
		expect(menus.queued.map((event) => [event.kind, event.payload])).toEqual([
			[
				"ITEM_IMPORT",
				{
					compendiumUuid: packItemUuid,
					description: "",
					foundryUuid: packItemUuid,
					name: "Rope",
					priceGold: 2,
				},
			],
		]);
	});

	test("a click after the bridge stopped here reads nothing and warns", async () => {
		const menus = menusFor({});
		const [send] = menus.entries("getActorContextOptions", actorDirectory);
		menus.status.condition = "another-gm-is-active";
		await send?.callback(menus.target(actorId));
		expect(menus.queued).toEqual([]);
		expect(menus.notices[0]).toStartWith("warn: Colony Bridge isn't running in this browser");
	});

	test("Deposit asks for the gold and hands a whole number to the escrow", async () => {
		const menus = menusFor({ dialogAnswer: { gold: 40 } });
		const deposit = menus.entries("getActorContextOptions", actorDirectory)[1];
		await deposit?.callback(menus.target(partyId));
		expect(menus.dialogs[0]?.content).toContain("Gold pieces to deposit");
		expect(menus.dialogs[0]?.content).toContain("Paid with any coins worth exactly this much.");
		expect(menus.deposits).toEqual([[partyId, 40, "link-1"]]);
	});

	test("Deposit from a PF2e character pays from that character's own coins", async () => {
		const menus = menusFor({ dialogAnswer: { gold: "12" } });
		const deposit = menus.entries("getActorContextOptions", actorDirectory)[1];
		await deposit?.callback(menus.target(characterId));
		expect(menus.deposits).toEqual([[characterId, 12, "link-1"]]);
	});

	test("Deposit does nothing when dismissed, and warns on anything but a whole number from 1 to 1,000,000", async () => {
		const dismissed = menusFor({ dialogAnswer: null });
		await dismissed
			.entries("getActorContextOptions", actorDirectory)[1]
			?.callback(dismissed.target(partyId));
		const fractional = menusFor({ dialogAnswer: { gold: 1.5 } });
		await fractional
			.entries("getActorContextOptions", actorDirectory)[1]
			?.callback(fractional.target(partyId));
		await settle();
		expect(dismissed.notices).toEqual([]);
		expect(dismissed.deposits).toEqual([]);
		expect(fractional.notices).toEqual([
			"warn: Type a whole number of gold pieces from 1 to 1,000,000.",
		]);
		expect(fractional.deposits).toEqual([]);
	});
});

test("readDepositGold takes a whole number from 1 to 1,000,000, as a number or its text", () => {
	expect([40, "40", " 7 ", 1_000_000].map(readDepositGold)).toEqual([40, 40, 7, 1_000_000]);
	expect([0, -1, 1.5, "1.5", "1e3", 1_000_001, "", null, Number.NaN].map(readDepositGold)).toEqual(
		Array(9).fill(undefined),
	);
});
