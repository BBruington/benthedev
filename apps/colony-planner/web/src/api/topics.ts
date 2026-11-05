import { hashKey, type QueryKey } from "@tanstack/react-query";
import type { Schemas } from "./client";
import {
	adjustPreviewQueryKeyPrefix,
	advancePreviewQueryKeyPrefix,
	approveRequestPreviewQueryKeyPrefix,
	calendarLinkQuery,
	campaignMeQuery,
	colonyBoardQuery,
	discordChannelsQuery,
	hexesQuery,
	inviteLinkQuery,
	latestRecapQuery,
	mapQueryKeyPrefix,
	mapsQuery,
	marketQuery,
	membersQuery,
	meQuery,
	myCampaignsQuery,
	nextSessionQuery,
	npcQueryKeyPrefix,
	npcsQueryKeyPrefix,
	patronActionsQuery,
	patronQueryKeyPrefix,
	patronsQuery,
	payMarketOrderPreviewQueryKeyPrefix,
	placeQueryKeyPrefix,
	placesQueryKeyPrefix,
	playedSessionsQuery,
	pollsQuery,
	questPreviewQueryKeyPrefixes,
	questQueryKeyPrefix,
	questsQueryKeyPrefix,
	recentTimelineQuery,
	requestQueryKeyPrefix,
	requestsQueryKeyPrefix,
	resourcesQuery,
	rolesQuery,
	sessionBandsQueryKeyPrefix,
	sessionQueryKeyPrefix,
	sessionSeriesQuery,
	settingsQuery,
	timelineQueryKeyPrefix,
	upcomingSessionsQuery,
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
			...questPreviewQueryKeyPrefixes(campaignId),
			approveRequestPreviewQueryKeyPrefix(campaignId),
			payMarketOrderPreviewQueryKeyPrefix(campaignId),
			// The market page carries gold on hand and whether the ship has docked, both colony state.
			marketQuery(campaignId).queryKey,
		],
		hexes: [hexesQuery(campaignId).queryKey],
		// A map's page carries its pins, and a place's page names the map it is pinned on.
		maps: [
			mapsQuery(campaignId).queryKey,
			mapQueryKeyPrefix(campaignId),
			placesQueryKeyPrefix(campaignId),
			placeQueryKeyPrefix(campaignId),
		],
		market: [marketQuery(campaignId).queryKey, payMarketOrderPreviewQueryKeyPrefix(campaignId)],
		// The caller's own membership or session changed: their status here, the picker's card, and
		// view as player (a session toggle that the account read carries too).
		me: [campaignMeQuery(campaignId).queryKey, myCampaignsQuery.queryKey, meQuery.queryKey],
		// Requests, a patron's request history among them, and the market's cart and orders name their
		// member by character name.
		members: [
			membersQuery(campaignId).queryKey,
			requestsQueryKeyPrefix(campaignId),
			requestQueryKeyPrefix(campaignId),
			patronQueryKeyPrefix(campaignId),
			marketQuery(campaignId).queryKey,
		],
		// Quests show their giver NPC by display name, and only while that NPC is visible.
		npcs: [
			npcsQueryKeyPrefix(campaignId),
			npcQueryKeyPrefix(campaignId),
			questsQueryKeyPrefix(campaignId),
			questQueryKeyPrefix(campaignId),
		],
		// Favor and the actions feed the quest and grant previews as well as the patron screens. Quests
		// show their patron giver, and requests whose patron or action is hidden leave players' lists.
		patrons: [
			patronsQuery(campaignId).queryKey,
			patronQueryKeyPrefix(campaignId),
			patronActionsQuery(campaignId).queryKey,
			...questPreviewQueryKeyPrefixes(campaignId),
			approveRequestPreviewQueryKeyPrefix(campaignId),
			questsQueryKeyPrefix(campaignId),
			questQueryKeyPrefix(campaignId),
			requestsQueryKeyPrefix(campaignId),
			requestQueryKeyPrefix(campaignId),
		],
		// Role settings live beside /people: both show headcounts.
		people: [workforceQuery(campaignId).queryKey, rolesQuery(campaignId).queryKey],
		// Pins carry their place's name and leave players' maps when the place is hidden.
		places: [
			placesQueryKeyPrefix(campaignId),
			placeQueryKeyPrefix(campaignId),
			mapQueryKeyPrefix(campaignId),
		],
		polls: [pollsQuery(campaignId).queryKey],
		quests: [
			questsQueryKeyPrefix(campaignId),
			questQueryKeyPrefix(campaignId),
			...questPreviewQueryKeyPrefixes(campaignId),
		],
		// A patron's page carries its request history.
		requests: [
			requestsQueryKeyPrefix(campaignId),
			requestQueryKeyPrefix(campaignId),
			patronQueryKeyPrefix(campaignId),
			approveRequestPreviewQueryKeyPrefix(campaignId),
		],
		resources: [resourcesQuery(campaignId).queryKey],
		// The board's next session and last recap, the timeline's bands, and NPC pages that name a
		// session (by its title and date) all read sessions.
		sessions: [
			sessionSeriesQuery(campaignId).queryKey,
			upcomingSessionsQuery(campaignId).queryKey,
			playedSessionsQuery(campaignId).queryKey,
			nextSessionQuery(campaignId).queryKey,
			latestRecapQuery(campaignId).queryKey,
			sessionBandsQueryKeyPrefix(campaignId),
			sessionQueryKeyPrefix(campaignId),
			calendarLinkQuery(campaignId).queryKey,
			npcsQueryKeyPrefix(campaignId),
			npcQueryKeyPrefix(campaignId),
		],
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
	// Topics share keys (the server sends maps and places together), and a key invalidated twice while
	// its read is in flight would be read twice more.
	const keys = new Map<string, QueryKey>();
	for (const topic of topics) {
		if (!isKnownTopic(topic, known)) {
			return "everything";
		}
		for (const queryKey of known[topic]) {
			keys.set(hashKey(queryKey), queryKey);
		}
	}
	return [...keys.values()];
}

export function isInvalidateMessage(value: unknown): value is { topics: string[] } {
	if (typeof value !== "object" || value === null || !("topics" in value)) {
		return false;
	}
	const { topics } = value;
	return Array.isArray(topics) && topics.every((topic) => typeof topic === "string");
}
