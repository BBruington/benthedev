import type { QueryKey } from "@tanstack/react-query";
import type { Schemas } from "./client";
import {
	adjustPreviewQueryKeyPrefix,
	advancePreviewQueryKeyPrefix,
	campaignMeQuery,
	colonyBoardQuery,
	discordChannelsQuery,
	inviteLinkQuery,
	membersQuery,
	meQuery,
	myCampaignsQuery,
	npcQueryKeyPrefix,
	npcsQueryKeyPrefix,
	placeQueryKeyPrefix,
	placesQueryKeyPrefix,
	recentTimelineQuery,
	resourcesQuery,
	rolesQuery,
	settingsQuery,
	timelineQueryKeyPrefix,
	workforceQuery,
} from "./queries";

export type Topic = Schemas["Topic"];

/**
 * Which cached reads each live-update topic makes stale, for one campaign: a campaign's stream speaks
 * only for that campaign, so every key holds its ID. The Record type makes a topic added on the server
 * fail typechecking here until it is mapped.
 */
export function topicQueryKeys(campaignId: string): Record<Topic, readonly QueryKey[]> {
	return {
		// A preview is only valid for the version it saw, so any colony change makes it stale.
		colony: [
			colonyBoardQuery(campaignId).queryKey,
			advancePreviewQueryKeyPrefix(campaignId),
			adjustPreviewQueryKeyPrefix(campaignId),
		],
		// The caller's own membership or session changed: their status here, the picker's card, and
		// view as player (a session toggle that the account read carries too).
		me: [campaignMeQuery(campaignId).queryKey, myCampaignsQuery.queryKey, meQuery.queryKey],
		members: [membersQuery(campaignId).queryKey],
		npcs: [npcsQueryKeyPrefix(campaignId), npcQueryKeyPrefix(campaignId)],
		// Role settings live beside /people: both show headcounts.
		people: [workforceQuery(campaignId).queryKey, rolesQuery(campaignId).queryKey],
		places: [placesQueryKeyPrefix(campaignId), placeQueryKeyPrefix(campaignId)],
		resources: [resourcesQuery(campaignId).queryKey],
		// The Discord card and its channel list live under settings; so does the campaign name.
		settings: [
			settingsQuery(campaignId).queryKey,
			inviteLinkQuery(campaignId).queryKey,
			discordChannelsQuery(campaignId).queryKey,
			campaignMeQuery(campaignId).queryKey,
			myCampaignsQuery.queryKey,
		],
		timeline: [timelineQueryKeyPrefix(campaignId), recentTimelineQuery(campaignId).queryKey],
	};
}

function isKnownTopic(topic: string, known: Record<Topic, readonly QueryKey[]>): topic is Topic {
	return Object.hasOwn(known, topic);
}

/**
 * The key prefixes to invalidate for a message's topics in one campaign. A topic this build doesn't
 * know (a newer server) can only be handled safely by refetching everything.
 */
export function queryKeysForTopics(
	campaignId: string,
	topics: readonly string[],
): readonly QueryKey[] | "everything" {
	const known = topicQueryKeys(campaignId);
	const keys: QueryKey[] = [];
	for (const topic of topics) {
		if (!isKnownTopic(topic, known)) {
			return "everything";
		}
		keys.push(...known[topic]);
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
