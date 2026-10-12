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

	test("collects the keys of several topics, scoped to the campaign", () => {
		const keys = queryKeysForTopics(campaignA, ["members", "resources"]);
		expect(keys).toEqual([
			[
				"get",
				"/api/campaigns/{campaignId}/members",
				{ params: { path: { campaignId: campaignA } } },
			],
			[
				"get",
				"/api/campaigns/{campaignId}/resources",
				{ params: { path: { campaignId: campaignA } } },
			],
		]);
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
