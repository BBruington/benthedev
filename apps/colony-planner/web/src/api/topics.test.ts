import { describe, expect, test } from "bun:test";
import openapi from "./openapi.json";
import { isInvalidateMessage, queryKeysForTopics, topicQueryKeys } from "./topics";

const campaignA = "00000000-0000-4000-8000-00000000000a";
const campaignB = "00000000-0000-4000-8000-00000000000b";

describe("topic → query-key map", () => {
	test("maps every topic the server can publish", () => {
		const serverTopics = [...openapi.components.schemas.Topic.enum].sort();
		expect(Object.keys(topicQueryKeys(campaignA)).sort()).toEqual(serverTopics);
	});

	test("every topic invalidates at least one query", () => {
		for (const keys of Object.values(topicQueryKeys(campaignA))) {
			expect(keys.length).toBeGreaterThan(0);
		}
	});

	test("a key two topics share is invalidated once, so its read isn't run twice", () => {
		const keys = queryKeysForTopics(campaignA, ["maps", "places"]);
		if (keys === "everything") {
			throw new Error("maps and places are known topics");
		}
		const hashes = keys.map((key) => JSON.stringify(key));
		expect(new Set(hashes).size).toBe(hashes.length);
		expect(hashes.length).toBeGreaterThan(0);
	});

	test("collects the keys of several topics, scoped to the campaign", () => {
		const keys = queryKeysForTopics(campaignA, ["resources", "people"]);
		expect(keys).toEqual([
			[
				"get",
				"/api/campaigns/{campaignId}/resources",
				{ params: { path: { campaignId: campaignA } } },
			],
			[
				"get",
				"/api/campaigns/{campaignId}/people",
				{ params: { path: { campaignId: campaignA } } },
			],
			["get", "/api/campaigns/{campaignId}/roles", { params: { path: { campaignId: campaignA } } }],
		]);
	});

	test("refreshes quests and requests that show a giver, a patron or a requester's name", () => {
		const paths = (topic: "npcs" | "patrons" | "members") =>
			topicQueryKeys(campaignA)[topic].map((key) => key[1]);
		expect(paths("npcs")).toEqual(
			expect.arrayContaining([
				"/api/campaigns/{campaignId}/quests",
				"/api/campaigns/{campaignId}/quests/{questId}",
			]),
		);
		expect(paths("patrons")).toEqual(
			expect.arrayContaining([
				"/api/campaigns/{campaignId}/quests",
				"/api/campaigns/{campaignId}/quests/{questId}",
				"/api/campaigns/{campaignId}/requests",
				"/api/campaigns/{campaignId}/requests/{requestId}",
			]),
		);
		expect(paths("members")).toEqual(
			expect.arrayContaining([
				"/api/campaigns/{campaignId}/requests",
				"/api/campaigns/{campaignId}/requests/{requestId}",
				"/api/campaigns/{campaignId}/patrons/{patronId}",
			]),
		);
	});

	test("a campaign's keys never name another campaign", () => {
		const encoded = JSON.stringify(topicQueryKeys(campaignA));
		expect(encoded).toContain(campaignA);
		expect(encoded).not.toContain(campaignB);
	});

	test("an unknown topic refetches everything", () => {
		expect(queryKeysForTopics(campaignA, ["me", "colony-of-the-future"])).toBe("everything");
	});

	test("recognises invalidate messages", () => {
		expect(isInvalidateMessage({ topics: ["me"] })).toBe(true);
		expect(isInvalidateMessage({ topics: "me" })).toBe(false);
		expect(isInvalidateMessage({ topics: [1] })).toBe(false);
		expect(isInvalidateMessage(null)).toBe(false);
	});
});
