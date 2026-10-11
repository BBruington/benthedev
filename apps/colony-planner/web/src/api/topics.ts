import type { QueryKey } from "@tanstack/react-query";
import type { Schemas } from "./client";
import {
	adjustPreviewQueryKeyPrefix,
	advancePreviewQueryKeyPrefix,
	colonyBoardQuery,
	membersQuery,
	meQuery,
	recentTimelineQuery,
	resourcesQuery,
	settingsQuery,
	timelineQueryKeyPrefix,
} from "./queries";

export type Topic = Schemas["Topic"];

/**
 * Which cached reads each live-update topic makes stale. The Record type makes a topic added on the
 * server fail typechecking here until it is mapped.
 */
export const topicQueryKeys: Record<Topic, readonly QueryKey[]> = {
	// A preview is only valid for the version it saw, so any colony change makes it stale.
	colony: [colonyBoardQuery.queryKey, advancePreviewQueryKeyPrefix, adjustPreviewQueryKeyPrefix],
	me: [meQuery.queryKey],
	members: [membersQuery.queryKey],
	resources: [resourcesQuery.queryKey],
	settings: [settingsQuery.queryKey],
	timeline: [timelineQueryKeyPrefix, recentTimelineQuery.queryKey],
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
