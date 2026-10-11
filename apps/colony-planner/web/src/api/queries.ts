import { api } from "./client";

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
