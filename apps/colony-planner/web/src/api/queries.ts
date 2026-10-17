import { api, type Schemas } from "./client";
import type { operations } from "./schema";

// One definition per read, so route guards, screens and the live-update map share the same keys.
// dmOnly marks reads the server refuses while view as player is on; the client skips refetching
// them then, since the answer can only be 403.

// Reads outside every campaign (account and site-owner screens). They have no stream
// (docs/design/multi-campaign.md §5.1), so they refetch whenever the window regains focus and after
// their own mutations. "always" because the client keeps every read fresh until told otherwise.
const outsideCampaign = { refetchOnWindowFocus: "always" } as const;

export const meQuery = api.queryOptions("get", "/api/me", undefined, outsideCampaign);
export const loginOptionsQuery = api.queryOptions("get", "/api/login-options");
/** The caller's campaigns, whatever their status; the SPA resolves a /c/{slug} to its ID here. */
export const myCampaignsQuery = api.queryOptions(
	"get",
	"/api/campaigns",
	undefined,
	outsideCampaign,
);
export const myCampaignRequestQuery = api.queryOptions(
	"get",
	"/api/campaign-requests/mine",
	undefined,
	outsideCampaign,
);

/** /join/{inviteCode}: who runs the campaign, its status and where the caller stands there. */
export function inviteQuery(inviteCode: string) {
	return api.queryOptions(
		"get",
		"/api/invites/{inviteCode}",
		{ params: { path: { inviteCode } } },
		outsideCampaign,
	);
}

// The site owner's /admin: directory data only.
export const adminCampaignRequestsQuery = api.queryOptions(
	"get",
	"/api/admin/campaign-requests",
	undefined,
	outsideCampaign,
);
export const adminCampaignsQuery = api.queryOptions(
	"get",
	"/api/admin/campaigns",
	undefined,
	outsideCampaign,
);

// Reads inside one campaign. Every key holds the campaign ID, so caches of two campaigns never mix and
// a campaign's stream invalidates only its own reads.
function inCampaign(campaignId: string) {
	return { params: { path: { campaignId } } };
}

/** The caller's membership here (pending level): the waiting, no-access and paused screens use it. */
export function campaignMeQuery(campaignId: string) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/me", inCampaign(campaignId));
}

export function membersQuery(campaignId: string) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/members", inCampaign(campaignId), {
		meta: { dmOnly: true },
	});
}

export function settingsQuery(campaignId: string) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/settings", inCampaign(campaignId), {
		meta: { dmOnly: true },
	});
}

/** The DM's invite card; the code appears in no other read. */
export function inviteLinkQuery(campaignId: string) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/invite", inCampaign(campaignId), {
		meta: { dmOnly: true },
	});
}

/** The linked Discord server's text channels, for the Settings channel picker. */
export function discordChannelsQuery(campaignId: string) {
	return api.queryOptions(
		"get",
		"/api/campaigns/{campaignId}/discord/channels",
		inCampaign(campaignId),
		{ meta: { dmOnly: true } },
	);
}

// M1: the colony board, the timeline and the DM's resource settings.
export function colonyBoardQuery(campaignId: string) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/colony", inCampaign(campaignId));
}

export function recentTimelineQuery(campaignId: string) {
	return api.queryOptions(
		"get",
		"/api/campaigns/{campaignId}/timeline/recent",
		inCampaign(campaignId),
	);
}

export function resourcesQuery(campaignId: string) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/resources", inCampaign(campaignId), {
		meta: { dmOnly: true },
	});
}

/** One range of timeline days, both ends inclusive; the timeline page asks for seven at a time. */
export function timelineQuery(campaignId: string, fromDay: number, toDay: number) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/timeline", {
		params: { path: { campaignId }, query: { fromDay, toDay } },
	});
}

/** What advancing `days` days would do; Confirm sends the preview's version as expectedVersion. */
export function advancePreviewQuery(campaignId: string, days: number) {
	return api.queryOptions(
		"get",
		"/api/campaigns/{campaignId}/colony/advance-preview",
		{ params: { path: { campaignId }, query: { days } } },
		{ meta: { dmOnly: true } },
	);
}

/**
 * What an adjustment (or recorded cargo) would do, caps included. It is a POST because it takes the
 * whole adjustment as a body, but it writes nothing, so it is cached and refetched like any read.
 */
export function adjustPreviewQuery(campaignId: string, input: Schemas["AdjustInput"]) {
	return api.queryOptions(
		"post",
		"/api/campaigns/{campaignId}/colony/adjust-preview",
		{ body: input, params: { path: { campaignId } } },
		{ meta: { dmOnly: true } },
	);
}

// M2: people, NPCs and places. Every read is already projected for the viewer by the server.

/** /people: every role's headcounts and named colonists; workforce forms send its version. */
export function workforceQuery(campaignId: string) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/people", inCampaign(campaignId));
}

/** The DM's role list on Settings, archived roles included. */
export function rolesQuery(campaignId: string) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/roles", inCampaign(campaignId), {
		meta: { dmOnly: true },
	});
}

export type NpcListFilter = NonNullable<operations["listNpcs"]["parameters"]["query"]>;

/** The NPC roster, searched and filtered; an empty filter lists everyone the viewer may see. */
export function npcsQuery(campaignId: string, filter: NpcListFilter = {}) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/npcs", {
		params: { path: { campaignId }, query: filter },
	});
}

export function npcQuery(campaignId: string, npcId: string) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/npcs/{npcId}", {
		params: { path: { campaignId, npcId } },
	});
}

/** Places, optionally narrowed to names containing `search`. */
export function placesQuery(campaignId: string, search = "") {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/places", {
		params: { path: { campaignId }, query: { search } },
	});
}

export function placeQuery(campaignId: string, placeId: string) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/places/{placeId}", {
		params: { path: { campaignId, placeId } },
	});
}

// M3: quests, patrons, their actions and requests. Every read is already projected for the viewer by
// the server: hidden quests, patrons and actions never reach players, and a decision note reaches only
// the requester and the DM.

export function questsQuery(campaignId: string) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/quests", inCampaign(campaignId));
}

export function questQuery(campaignId: string, questId: string) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/quests/{questId}", {
		params: { path: { campaignId, questId } },
	});
}

/** What Complete would pay: "Food 40 → 60 · +3 to Unassigned · Lady Vex favor 1 → 3". */
export function completeQuestPreviewQuery(campaignId: string, questId: string) {
	return api.queryOptions(
		"get",
		"/api/campaigns/{campaignId}/quests/{questId}/complete-preview",
		{ params: { path: { campaignId, questId } } },
		{ meta: { dmOnly: true } },
	);
}

/** What Fail would cost, from the quest's DM-only failure cost. */
export function failQuestPreviewQuery(campaignId: string, questId: string) {
	return api.queryOptions(
		"get",
		"/api/campaigns/{campaignId}/quests/{questId}/fail-preview",
		{ params: { path: { campaignId, questId } } },
		{ meta: { dmOnly: true } },
	);
}

export function patronsQuery(campaignId: string) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/patrons", inCampaign(campaignId));
}

/** A patron's page: the patron, its actions and its request history. */
export function patronQuery(campaignId: string, patronId: string) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/patrons/{patronId}", {
		params: { path: { campaignId, patronId } },
	});
}

/** Every action the viewer may see, for pickers; the DM's list keeps archived ones, flagged. */
export function patronActionsQuery(campaignId: string) {
	return api.queryOptions(
		"get",
		"/api/campaigns/{campaignId}/patron-actions",
		inCampaign(campaignId),
	);
}

export type RequestListFilter = NonNullable<operations["listRequests"]["parameters"]["query"]>;

/** The DM's queue and history or the party's history; `{ mine: true }` is "my requests". */
export function requestsQuery(campaignId: string, filter: RequestListFilter = {}) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/requests", {
		params: { path: { campaignId }, query: filter },
	});
}

export function requestQuery(campaignId: string, requestId: string) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/requests/{requestId}", {
		params: { path: { campaignId, requestId } },
	});
}

/**
 * The approval sheet's before → after and validity line for a grant, with the DM's tweak if any. A
 * POST because it takes the tweak as a body, but it writes nothing.
 */
export function approveRequestPreviewQuery(
	campaignId: string,
	requestId: string,
	input: Schemas["ApproveRequestInput"],
) {
	return api.queryOptions(
		"post",
		"/api/campaigns/{campaignId}/requests/{requestId}/approve-preview",
		{ body: input, params: { path: { campaignId, requestId } } },
		{ meta: { dmOnly: true } },
	);
}

// Key prefixes that match every parameterized read of one path in one campaign, for live
// invalidation. TanStack Query matches the trailing object partially, so the query parameters and
// bodies of each read don't matter.
// M4: world maps, place pins and the hex map. Players get revealed maps only, and a map's pins name
// revealed places only; the hex map has no secrets.

export function mapsQuery(campaignId: string) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/maps", inCampaign(campaignId));
}

export function mapQuery(campaignId: string, mapId: string) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/maps/{mapId}", {
		params: { path: { campaignId, mapId } },
	});
}

/** Every charted hex; a hexes invalidation refetches the whole map (spec §5.10). */
export function hexesQuery(campaignId: string) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/hexes", inCampaign(campaignId));
}

/** Every series and one-off session (spec §4.5), for /sessions and the DM's series editor. */
export function sessionSeriesQuery(campaignId: string) {
	return api.queryOptions(
		"get",
		"/api/campaigns/{campaignId}/session-series",
		inCampaign(campaignId),
	);
}

/** The next 12 weeks of dates, oldest first; cancelled ones are included and marked. */
export function upcomingSessionsQuery(campaignId: string) {
	return api.queryOptions(
		"get",
		"/api/campaigns/{campaignId}/sessions/upcoming",
		inCampaign(campaignId),
	);
}

/** The history, newest first. */
export function playedSessionsQuery(campaignId: string) {
	return api.queryOptions(
		"get",
		"/api/campaigns/{campaignId}/sessions/played",
		inCampaign(campaignId),
	);
}

/** The board's next session; its `session` is null when nothing is planned. */
export function nextSessionQuery(campaignId: string) {
	return api.queryOptions(
		"get",
		"/api/campaigns/{campaignId}/sessions/next",
		inCampaign(campaignId),
	);
}

/** The board's last recap snippet; its `session` is null when no session has a recap. */
export function latestRecapQuery(campaignId: string) {
	return api.queryOptions(
		"get",
		"/api/campaigns/{campaignId}/sessions/latest-recap",
		inCampaign(campaignId),
	);
}

/** The played sessions whose colony days meet a timeline range, both ends inclusive. */
export function sessionBandsQuery(campaignId: string, fromDay: number, toDay: number) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/sessions/bands", {
		params: { path: { campaignId }, query: { fromDay, toDay } },
	});
}

/** One stored session; an upcoming date without a row has no ID and no page yet. */
export function sessionQuery(campaignId: string, sessionId: string) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/sessions/{sessionId}", {
		params: { path: { campaignId, sessionId } },
	});
}

/** The ICS subscription path; every member may read it, and only the DM regenerates it. */
export function calendarLinkQuery(campaignId: string) {
	return api.queryOptions("get", "/api/campaigns/{campaignId}/calendar", inCampaign(campaignId));
}

export function timelineQueryKeyPrefix(campaignId: string) {
	return ["get", "/api/campaigns/{campaignId}/timeline", inCampaign(campaignId)] as const;
}

export function advancePreviewQueryKeyPrefix(campaignId: string) {
	return [
		"get",
		"/api/campaigns/{campaignId}/colony/advance-preview",
		inCampaign(campaignId),
	] as const;
}

export function adjustPreviewQueryKeyPrefix(campaignId: string) {
	return [
		"post",
		"/api/campaigns/{campaignId}/colony/adjust-preview",
		inCampaign(campaignId),
	] as const;
}

export function npcsQueryKeyPrefix(campaignId: string) {
	return ["get", "/api/campaigns/{campaignId}/npcs", inCampaign(campaignId)] as const;
}

export function npcQueryKeyPrefix(campaignId: string) {
	return ["get", "/api/campaigns/{campaignId}/npcs/{npcId}", inCampaign(campaignId)] as const;
}

export function placesQueryKeyPrefix(campaignId: string) {
	return ["get", "/api/campaigns/{campaignId}/places", inCampaign(campaignId)] as const;
}

export function placeQueryKeyPrefix(campaignId: string) {
	return ["get", "/api/campaigns/{campaignId}/places/{placeId}", inCampaign(campaignId)] as const;
}

export function questsQueryKeyPrefix(campaignId: string) {
	return ["get", "/api/campaigns/{campaignId}/quests", inCampaign(campaignId)] as const;
}

export function questQueryKeyPrefix(campaignId: string) {
	return ["get", "/api/campaigns/{campaignId}/quests/{questId}", inCampaign(campaignId)] as const;
}

export function questPreviewQueryKeyPrefixes(campaignId: string) {
	return [
		[
			"get",
			"/api/campaigns/{campaignId}/quests/{questId}/complete-preview",
			inCampaign(campaignId),
		],
		["get", "/api/campaigns/{campaignId}/quests/{questId}/fail-preview", inCampaign(campaignId)],
	] as const;
}

export function patronQueryKeyPrefix(campaignId: string) {
	return ["get", "/api/campaigns/{campaignId}/patrons/{patronId}", inCampaign(campaignId)] as const;
}

export function requestsQueryKeyPrefix(campaignId: string) {
	return ["get", "/api/campaigns/{campaignId}/requests", inCampaign(campaignId)] as const;
}

export function requestQueryKeyPrefix(campaignId: string) {
	return [
		"get",
		"/api/campaigns/{campaignId}/requests/{requestId}",
		inCampaign(campaignId),
	] as const;
}

export function approveRequestPreviewQueryKeyPrefix(campaignId: string) {
	return [
		"post",
		"/api/campaigns/{campaignId}/requests/{requestId}/approve-preview",
		inCampaign(campaignId),
	] as const;
}

export function mapQueryKeyPrefix(campaignId: string) {
	return ["get", "/api/campaigns/{campaignId}/maps/{mapId}", inCampaign(campaignId)] as const;
}

export function sessionBandsQueryKeyPrefix(campaignId: string) {
	return ["get", "/api/campaigns/{campaignId}/sessions/bands", inCampaign(campaignId)] as const;
}

export function sessionQueryKeyPrefix(campaignId: string) {
	return [
		"get",
		"/api/campaigns/{campaignId}/sessions/{sessionId}",
		inCampaign(campaignId),
	] as const;
}
