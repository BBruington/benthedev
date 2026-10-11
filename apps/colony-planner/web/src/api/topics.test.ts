import { describe, expect, test } from "bun:test";
import openapi from "./openapi.json";
import { isInvalidateMessage, queryKeysForTopics, topicQueryKeys } from "./topics";

describe("topic → query-key map", () => {
	test("maps every topic the server can publish", () => {
		const serverTopics = [...openapi.components.schemas.Topic.enum].sort();
		expect(Object.keys(topicQueryKeys).sort()).toEqual(serverTopics);
	});

	test("every topic invalidates at least one query", () => {
		for (const keys of Object.values(topicQueryKeys)) {
			expect(keys.length).toBeGreaterThan(0);
		}
	});

	test("collects the keys of several topics", () => {
		expect(queryKeysForTopics(["me", "settings"])).toEqual([
			["get", "/api/me"],
			["get", "/api/settings"],
		]);
	});

	test("an unknown topic refetches everything", () => {
		expect(queryKeysForTopics(["me", "colony-of-the-future"])).toBe("everything");
	});

	test("recognises invalidate messages", () => {
		expect(isInvalidateMessage({ topics: ["me"] })).toBe(true);
		expect(isInvalidateMessage({ topics: "me" })).toBe(false);
		expect(isInvalidateMessage({ topics: [1] })).toBe(false);
		expect(isInvalidateMessage(null)).toBe(false);
	});
});
