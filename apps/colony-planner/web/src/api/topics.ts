import type { QueryKey } from "@tanstack/react-query";
import type { Schemas } from "./client";
import { membersQuery, meQuery, settingsQuery } from "./queries";

export type Topic = Schemas["Topic"];

/**
 * Which cached reads each live-update topic makes stale. The Record type makes a topic added on the
 * server fail typechecking here until it is mapped.
 */
export const topicQueryKeys: Record<Topic, readonly QueryKey[]> = {
	me: [meQuery.queryKey],
	members: [membersQuery.queryKey],
	settings: [settingsQuery.queryKey],
};

function isKnownTopic(topic: string): topic is Topic {
	return Object.hasOwn(topicQueryKeys, topic);
}

/**
 * The key prefixes to invalidate for a message's topics. A topic this build doesn't know (a newer
 * server) can only be handled safely by refetching everything.
 */
export function queryKeysForTopics(topics: readonly string[]): readonly QueryKey[] | "everything" {
	const keys: QueryKey[] = [];
	for (const topic of topics) {
		if (!isKnownTopic(topic)) {
			return "everything";
		}
		keys.push(...topicQueryKeys[topic]);
	}
	return keys;
}

export function isInvalidateMessage(value: unknown): value is { topics: string[] } {
	if (typeof value !== "object" || value === null || !("topics" in value)) {
		return false;
	}
	const { topics } = value;
	return Array.isArray(topics) && topics.every((topic) => typeof topic === "string");
}
