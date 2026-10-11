import { describe, expect, test } from "bun:test";
import { QueryClient, QueryObserver } from "@tanstack/react-query";
import { switchProjection } from "./projection";

const meKey = ["get", "/api/me"];
const recentKey = ["get", "/api/timeline/recent"];
const offScreenPageKey = [
	"get",
	"/api/timeline",
	{ params: { query: { fromDay: 15, toDay: 21 } } },
];
const membersKey = ["get", "/api/members"];

// A cache as the DM left it: /api/me and the recent timeline on screen, and an older timeline page
// and a DM-only read cached.
async function dmCache(fetched: string[]) {
	const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
	const reader = (name: string) => () => {
		fetched.push(name);
		return Promise.resolve(`${name} data`);
	};
	const observers = [
		new QueryObserver(queryClient, { queryFn: reader("me"), queryKey: meKey }),
		new QueryObserver(queryClient, { queryFn: reader("recent"), queryKey: recentKey }),
	];
	for (const observer of observers) {
		observer.subscribe(() => undefined);
	}
	await queryClient.prefetchQuery({ queryFn: reader("page"), queryKey: offScreenPageKey });
	await queryClient.prefetchQuery({
		meta: { dmOnly: true },
		queryFn: reader("members"),
		queryKey: membersKey,
	});
	await Promise.all(observers.map((observer) => observer.refetch()));
	fetched.length = 0;
	return queryClient;
}

describe("switchProjection", () => {
	test("entering the player view rereads what is on screen before the view flips", async () => {
		const fetched: string[] = [];
		const queryClient = await dmCache(fetched);
		await switchProjection(queryClient, true);
		expect(fetched).toEqual(["recent", "me"]);
		// A page cached in the DM view is dropped, so it can't open on the DM's entries.
		expect(queryClient.getQueryData<string>(offScreenPageKey)).toBeUndefined();
		// DM-only reads would answer 403 now; they wait for the DM view.
		expect(queryClient.getQueryData<string>(membersKey)).toBe("members data");
	});

	test("leaving the player view flips the view first, then rereads", async () => {
		const fetched: string[] = [];
		const queryClient = await dmCache(fetched);
		await switchProjection(queryClient, false);
		expect(fetched).toEqual(["me", "recent"]);
		expect(queryClient.getQueryData<string>(offScreenPageKey)).toBeUndefined();
		expect(queryClient.getQueryData<string>(membersKey)).toBeUndefined();
	});
});
