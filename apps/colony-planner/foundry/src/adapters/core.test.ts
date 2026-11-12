import { describe, expect, test } from "bun:test";
import type { FlaggedDocument, FoundryJournalEntry, FoundryJournalPage } from "../foundry-types";
import { readField } from "../wire";
import { createCoreAdapter, renderColonySummary } from "./core";

type Write = { data: object; options: object | undefined; target: string };

function flagged(externalId: string | undefined): FlaggedDocument {
	return {
		getFlag: (scope, key) =>
			scope === "colony-bridge" && key === "externalId" ? externalId : undefined,
	};
}

function fakeWorld(options: { moveClock?: boolean } = {}) {
	const writes: Write[] = [];
	const journalEntries: FoundryJournalEntry[] = [];
	const messages: FlaggedDocument[] = [];
	const advances: { options: object | undefined; seconds: number }[] = [];

	function fakePage(externalId: string | undefined, name: string): FoundryJournalPage {
		return {
			...flagged(externalId),
			update: (data, writeOptions) => {
				writes.push({ data, options: writeOptions, target: `page:${name}` });
				return Promise.resolve();
			},
		};
	}

	function fakeEntry(
		externalId: string | undefined,
		pages: FoundryJournalPage[],
	): FoundryJournalEntry {
		return {
			...flagged(externalId),
			createEmbeddedDocuments: (_name, data, writeOptions) => {
				writes.push({ data: data[0] ?? {}, options: writeOptions, target: "new page" });
				return Promise.resolve([]);
			},
			pages: { find: (predicate) => pages.find(predicate) },
			update: () => Promise.resolve(),
		};
	}

	const adapter = createCoreAdapter({
		chatMessage: {
			create: (data, writeOptions) => {
				writes.push({ data, options: writeOptions, target: "chat" });
				const message = flagged(
					String(readField(readField(readField(data, "flags"), "colony-bridge"), "externalId")),
				);
				messages.push(message);
				return Promise.resolve(message);
			},
		},
		foundryGame: {
			journal: { find: (predicate) => journalEntries.find(predicate) },
			messages: { find: (predicate) => messages.find(predicate) },
			time: {
				advance: (seconds, advanceOptions) => {
					advances.push({ options: advanceOptions, seconds });
					return Promise.resolve();
				},
				worldTime: 0,
			},
		},
		journalEntry: {
			create: (data, writeOptions) => {
				writes.push({ data, options: writeOptions, target: "new entry" });
				return Promise.resolve(undefined);
			},
		},
		moveClockWithColonyDays: () => options.moveClock === true,
	});
	return { adapter, advances, fakeEntry, fakePage, journalEntries, writes };
}

const summary = {
	campaignName: "Port <Eldari>",
	day: 22,
	nextGoodsShipDay: null,
	nextShipmentDay: 30,
	resources: [{ amount: 1250, name: "Food & water" }],
	siteUrl: "https://colony.benthedev.com/c/port-eldari",
	workforce: [{ count: 4, role: "Farmers" }],
};

const summaryRow = { kind: "COLONY_SUMMARY", payload: summary, seq: 1, targetKey: "colony" };
const advanceRow = {
	kind: "DAYS_ADVANCED",
	payload: { days: 3, fromDay: 22, toDay: 25 },
	seq: 2,
	targetKey: "advance:2",
};

describe("COLONY_SUMMARY", () => {
	test("creates the Colony journal entry, flagged and readable by players, as a bridge write", async () => {
		const world = fakeWorld();
		await world.adapter.appliers.COLONY_SUMMARY?.(summaryRow);
		expect(world.writes).toHaveLength(1);
		const [write] = world.writes;
		expect(write?.target).toBe("new entry");
		expect(write?.options).toEqual({ colonyBridge: true });
		expect(write?.data).toMatchObject({
			flags: { "colony-bridge": { externalId: "colony" } },
			name: "Colony",
			ownership: { default: 2 },
			pages: [
				{ flags: { "colony-bridge": { externalId: "colony" } }, name: "Colony", type: "text" },
			],
		});
	});

	test("replaces the bridge's page in the entry it finds by flag, leaving the GM's pages", async () => {
		const world = fakeWorld();
		world.journalEntries.push(
			world.fakeEntry(undefined, []),
			world.fakeEntry("colony", [
				world.fakePage(undefined, "GM notes"),
				world.fakePage("colony", "Colony"),
			]),
		);
		await world.adapter.appliers.COLONY_SUMMARY?.(summaryRow);
		expect(world.writes).toEqual([
			{
				data: { "text.content": renderColonySummary(summary) },
				options: { colonyBridge: true },
				target: "page:Colony",
			},
		]);
	});

	test("adds the page again when the GM deleted it", async () => {
		const world = fakeWorld();
		world.journalEntries.push(world.fakeEntry("colony", []));
		await world.adapter.appliers.COLONY_SUMMARY?.(summaryRow);
		expect(world.writes.map((write) => write.target)).toEqual(["new page"]);
	});

	test("a payload of the wrong shape fails the row", async () => {
		const world = fakeWorld();
		await expect(
			world.adapter.appliers.COLONY_SUMMARY?.({ ...summaryRow, payload: { day: "22" } }),
		).rejects.toThrow("The colony summary's payload wasn't understood.");
	});
});

describe("renderColonySummary", () => {
	test("escapes every string from the payload", () => {
		const html = renderColonySummary({
			...summary,
			resources: [{ amount: 1, name: "<script>alert(1)</script>" }],
			siteUrl: 'https://colony.benthedev.com/"onmouseover="alert(1)',
			workforce: [{ count: 2, role: "<b>Guards</b>" }],
		});
		expect(html).not.toContain("<script>");
		expect(html).not.toContain("<b>");
		expect(html).toContain("&lt;script&gt;alert(1)&lt;/script&gt;: 1");
		expect(html).toContain("&lt;b&gt;Guards&lt;/b&gt;: 2");
		expect(html).toContain("<h2>Port &lt;Eldari&gt;</h2>");
		expect(html).toContain('href="https://colony.benthedev.com/&quot;onmouseover=&quot;alert(1)"');
	});

	test("shows the day, the next shipment and empty lists, and links only to a web address", () => {
		const html = renderColonySummary({
			...summary,
			resources: [],
			siteUrl: "javascript:alert(1)",
			workforce: [],
		});
		expect(html).toContain("<strong>Day 22</strong>");
		expect(html).toContain("Next shipment: Day 30");
		expect(html).not.toContain("Next goods ship");
		expect(html).toContain("Nothing stockpiled.");
		expect(html).toContain("No workforce.");
		expect(html).not.toContain("href");
		expect(renderColonySummary(summary)).toContain("Food &amp; water: 1,250");
	});
});

describe("DAYS_ADVANCED", () => {
	test("posts a chat message and leaves the clock alone while the world setting is off", async () => {
		const world = fakeWorld();
		await world.adapter.appliers.DAYS_ADVANCED?.(advanceRow);
		expect(world.writes).toEqual([
			{
				data: {
					content: "<p>The colony advanced 3 days, from Day 22 to Day 25.</p>",
					flags: { "colony-bridge": { externalId: "advance:2" } },
					speaker: { alias: "Colony Planner" },
				},
				options: { colonyBridge: true },
				target: "chat",
			},
		]);
		expect(world.advances).toEqual([]);
	});

	test("moves Foundry's clock by whole days when the setting is on", async () => {
		const world = fakeWorld({ moveClock: true });
		await world.adapter.appliers.DAYS_ADVANCED?.(advanceRow);
		expect(world.advances).toEqual([{ options: { colonyBridge: true }, seconds: 3 * 86_400 }]);
	});

	test("a repeat of the same row does nothing", async () => {
		const world = fakeWorld({ moveClock: true });
		await world.adapter.appliers.DAYS_ADVANCED?.(advanceRow);
		await world.adapter.appliers.DAYS_ADVANCED?.(advanceRow);
		expect(world.writes).toHaveLength(1);
		expect(world.advances).toHaveLength(1);
	});

	test("posts the message on its own after the summary's page update, which Foundry stamps a parent onto", async () => {
		const world = fakeWorld();
		const page = world.fakePage("colony", "Colony");
		world.journalEntries.push(
			world.fakeEntry("colony", [
				{
					...page,
					update: (data, writeOptions) => {
						// Foundry's Document#update writes the parent into the options object it was given.
						Object.assign(writeOptions ?? {}, { parent: "the Colony journal" });
						return page.update(data, writeOptions);
					},
				},
			]),
		);
		await world.adapter.appliers.COLONY_SUMMARY?.(summaryRow);
		await world.adapter.appliers.DAYS_ADVANCED?.(advanceRow);
		const chatWrite = world.writes.find((write) => write.target === "chat");
		expect(chatWrite?.options).toEqual({ colonyBridge: true });
	});
});
