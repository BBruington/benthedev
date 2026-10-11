import { api, type Schemas } from "./client";

// One definition per read, so route guards, screens and the live-update map share the same keys.
// dmOnly marks reads the server refuses while view as player is on; the client skips refetching
// them then, since the answer can only be 403.
export const meQuery = api.queryOptions("get", "/api/me");
export const loginOptionsQuery = api.queryOptions("get", "/api/login-options");
export const membersQuery = api.queryOptions("get", "/api/members", undefined, {
	meta: { dmOnly: true },
});
export const settingsQuery = api.queryOptions("get", "/api/settings", undefined, {
	meta: { dmOnly: true },
});

// M1: the colony board, the timeline and the DM's resource settings.
export const colonyBoardQuery = api.queryOptions("get", "/api/colony");
export const recentTimelineQuery = api.queryOptions("get", "/api/timeline/recent");
export const resourcesQuery = api.queryOptions("get", "/api/resources", undefined, {
	meta: { dmOnly: true },
});

/** One range of timeline days, both ends inclusive; the timeline page asks for seven at a time. */
export function timelineQuery(fromDay: number, toDay: number) {
	return api.queryOptions("get", "/api/timeline", { params: { query: { fromDay, toDay } } });
}

/** What advancing `days` days would do; Confirm sends the preview's version as expectedVersion. */
export function advancePreviewQuery(days: number) {
	return api.queryOptions(
		"get",
		"/api/colony/advance-preview",
		{ params: { query: { days } } },
		{ meta: { dmOnly: true } },
	);
}

/**
 * What an adjustment (or recorded cargo) would do, caps included. It is a POST because it takes the
 * whole adjustment as a body, but it writes nothing, so it is cached and refetched like any read.
 */
export function adjustPreviewQuery(input: Schemas["AdjustInput"]) {
	return api.queryOptions(
		"post",
		"/api/colony/adjust-preview",
		{ body: input },
		{ meta: { dmOnly: true } },
	);
}

// Key prefixes that match every parameterized read of one path, for live invalidation.
export const timelineQueryKeyPrefix = ["get", "/api/timeline"] as const;
export const advancePreviewQueryKeyPrefix = ["get", "/api/colony/advance-preview"] as const;
export const adjustPreviewQueryKeyPrefix = ["post", "/api/colony/adjust-preview"] as const;
