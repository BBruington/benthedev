import { describe, expect, test } from "bun:test";
import { createBridge } from "./bridge";
import type { Fetch } from "./bridge-client";
import type { FoundryUser } from "./foundry-types";
import type { BridgeSettings } from "./settings";
import { createFakeSchedule, jsonResponse, settle } from "./testing/fakes";
import { secondsPerDay } from "./time-tracker";
import type { InboundEvent } from "./wire";

const thisGm: FoundryUser = { id: "gm-this", isGM: true };
const otherGm: FoundryUser = { id: "gm-other", isGM: true };
const origin = "https://colony.benthedev.com";

function memorySettings(initial: { enabled?: boolean; lastReportedDay?: number; token?: string }) {
	const values = {
		campaignName: "Port Eldari",
		cursor: 0,
		enabled: initial.enabled ?? true,
		events: [] as InboundEvent[],
		lastReportedDay: initial.lastReportedDay,
		token: initial.token ?? "secret-token",
	};
	const settings: BridgeSettings = {
		acknowledgedThrough: () => values.cursor,
		bridgeEnabled: () => values.enabled,
		campaignName: () => values.campaignName,
		lastReportedDay: () => values.lastReportedDay,
		moveClockWithColonyDays: () => false,
		pendingEvents: () => values.events,
		saveAcknowledgedThrough: (seq) => {
			values.cursor = seq;
			return Promise.resolve();
		},
		saveLastReportedDay: (day) => {
			values.lastReportedDay = day;
			return Promise.resolve();
		},
		saveLink: (link) => {
			values.campaignName = link.campaignName;
			values.token = link.token;
			values.cursor = 0;
			values.events = [];
			return Promise.resolve();
		},
		savePendingEvents: (events) => {
			values.events = events;
			return Promise.resolve();
		},
		token: () => values.token,
	};
	return { settings, values };
}

function fakeColonyPlanner(
	options: { holdRequestsWithToken?: string; refuseToken?: boolean } = {},
) {
	const requests: { authorization: string | null; method: string; path: string }[] = [];
	const heldAnswers: ((response: Response) => void)[] = [];
	const sentEvents: InboundEvent[] = [];
	const openStreams: AbortSignal[] = [];
	const fetchFunction: Fetch = (url, init) => {
		const path = url.slice(origin.length);
		requests.push({
			authorization: new Headers(init.headers).get("Authorization"),
			method: init.method ?? "GET",
			path,
		});
		if (options.refuseToken) {
			return Promise.resolve(jsonResponse(401, { title: "Unauthorized" }));
		}
		const heldToken = `Bearer ${options.holdRequestsWithToken}`;
		if (path !== "/api/bridge/stream" && requests.at(-1)?.authorization === heldToken) {
			return new Promise<Response>((resolve) => heldAnswers.push(resolve));
		}
		if (path === "/api/bridge/stream") {
			const signal = init.signal ?? new AbortController().signal;
			openStreams.push(signal);
			const body = new ReadableStream<Uint8Array>({
				start(controller) {
					signal.addEventListener("abort", () => controller.error(signal.reason));
				},
			});
			return Promise.resolve(new Response(body, { status: 200 }));
		}
		if (path.startsWith("/api/bridge/outbox")) {
			return Promise.resolve(jsonResponse(200, { more: false, rows: [] }));
		}
		if (path === "/api/bridge/events") {
			const body: { events: InboundEvent[] } = JSON.parse(String(init.body));
			sentEvents.push(...body.events);
			return Promise.resolve(jsonResponse(200, { accepted: body.events.length, duplicates: 0 }));
		}
		return Promise.resolve(new Response(null, { status: 204 }));
	};
	const liveStreams = () => openStreams.filter((signal) => !signal.aborted).length;
	/** Answers every held request 401, as if the link was revoked while they were in flight. */
	const refuseHeldRequests = () => {
		for (const answer of heldAnswers.splice(0)) {
			answer(jsonResponse(401, { title: "Unauthorized" }));
		}
	};
	return {
		fetchFunction,
		heldCount: () => heldAnswers.length,
		liveStreams,
		refuseHeldRequests,
		requests,
		sentEvents,
	};
}

function bridgeFor(options: {
	activeGm: FoundryUser | null;
	colonyPlanner: ReturnType<typeof fakeColonyPlanner>;
	settings: BridgeSettings;
}) {
	const errors: string[] = [];
	const foundryGame = {
		modules: { get: () => ({ manifest: `${origin}/foundry/module.json`, version: "1.4.0" }) },
		system: { id: "dnd5e", version: "5.2.5" },
		time: { advance: () => Promise.resolve(), worldTime: 40 * secondsPerDay },
		user: thisGm,
		users: { activeGM: options.activeGm },
		version: "13.351",
		world: { title: "Practice world" },
	};
	const bridge = createBridge({
		adapter: { appliers: {}, name: "core" },
		colonyOrigin: origin,
		fetchFunction: options.colonyPlanner.fetchFunction,
		foundryGame,
		notifications: {
			error: (message) => errors.push(message),
			info: () => undefined,
			warn: (message) => errors.push(message),
		},
		schedule: createFakeSchedule().schedule,
		settings: options.settings,
	});
	return { bridge, errors, foundryGame };
}

describe("createBridge", () => {
	test("a GM who isn't the active GM does nothing", async () => {
		const colonyPlanner = fakeColonyPlanner();
		const { bridge } = bridgeFor({
			activeGm: otherGm,
			colonyPlanner,
			settings: memorySettings({}).settings,
		});
		bridge.evaluate();
		await settle();
		expect(colonyPlanner.requests).toEqual([]);
		expect(bridge.status().condition).toBe("another-gm-is-active");
	});

	test("the active GM connects with the token, sends STATUS and reads the outbox from its cursor", async () => {
		const colonyPlanner = fakeColonyPlanner();
		const { settings, values } = memorySettings({});
		values.cursor = 17;
		const { bridge } = bridgeFor({ activeGm: thisGm, colonyPlanner, settings });
		bridge.evaluate();
		await settle();
		expect(colonyPlanner.requests.map((request) => `${request.method} ${request.path}`)).toEqual([
			"GET /api/bridge/stream",
			"POST /api/bridge/events",
			"GET /api/bridge/outbox?after=17",
		]);
		expect(
			colonyPlanner.requests.every((request) => request.authorization === "Bearer secret-token"),
		).toBe(true);
		expect(colonyPlanner.sentEvents.map((event) => [event.kind, event.payload])).toEqual([
			[
				"STATUS",
				{
					foundryVersion: "13.351",
					moduleVersion: "1.4.0",
					systemId: "dnd5e",
					systemVersion: "5.2.5",
					worldTitle: "Practice world",
				},
			],
		]);
		expect(bridge.status()).toMatchObject({ condition: "running", connection: "live" });
	});

	test("hands off: it starts when this GM becomes the active GM and stops when another does", async () => {
		const colonyPlanner = fakeColonyPlanner();
		const { bridge, foundryGame } = bridgeFor({
			activeGm: otherGm,
			colonyPlanner,
			settings: memorySettings({}).settings,
		});
		bridge.evaluate();
		foundryGame.users.activeGM = thisGm;
		bridge.evaluate();
		await settle();
		expect(colonyPlanner.liveStreams()).toBe(1);
		foundryGame.users.activeGM = otherGm;
		bridge.evaluate();
		await settle();
		expect(colonyPlanner.liveStreams()).toBe(0);
		expect(bridge.status().connection).toBeUndefined();
	});

	test("does nothing while the bridge is off or no link is stored", async () => {
		const colonyPlanner = fakeColonyPlanner();
		const off = bridgeFor({
			activeGm: thisGm,
			colonyPlanner,
			settings: memorySettings({ enabled: false }).settings,
		});
		off.bridge.evaluate();
		const unlinked = bridgeFor({
			activeGm: thisGm,
			colonyPlanner,
			settings: memorySettings({ token: "" }).settings,
		});
		unlinked.bridge.evaluate();
		await settle();
		expect(colonyPlanner.requests).toEqual([]);
		expect(off.bridge.status().condition).toBe("turned-off");
		expect(unlinked.bridge.status().condition).toBe("not-linked");
	});

	test("a refused token stops the bridge, forgets the token and tells the GM to pair again", async () => {
		const colonyPlanner = fakeColonyPlanner({ refuseToken: true });
		const { settings, values } = memorySettings({});
		const { bridge, errors } = bridgeFor({ activeGm: thisGm, colonyPlanner, settings });
		bridge.evaluate();
		await settle();
		expect(colonyPlanner.requests).toHaveLength(1);
		expect(values.token).toBe("");
		expect(bridge.status().condition).toBe("needs-new-code");
		expect(errors).toEqual([
			"Colony Bridge: Colony Planner ended this world's link. Make a new code on Colony Planner's Settings and connect again.",
		]);
	});

	test("reports Foundry's clock moves but not the bridge's own", async () => {
		const colonyPlanner = fakeColonyPlanner();
		const { bridge } = bridgeFor({
			activeGm: thisGm,
			colonyPlanner,
			settings: memorySettings({ lastReportedDay: 40 }).settings,
		});
		bridge.evaluate();
		await settle();
		bridge.observeWorldTime(43 * secondsPerDay, { colonyBridge: true });
		bridge.observeWorldTime(45 * secondsPerDay + 100, {});
		await settle();
		const timeEvents = colonyPlanner.sentEvents.filter((event) => event.kind === "TIME_ADVANCED");
		expect(timeEvents.map((event) => event.payload)).toEqual([{ days: 2 }]);
	});

	test("a new link from pairing replaces the old one and connects", async () => {
		const colonyPlanner = fakeColonyPlanner();
		const { settings, values } = memorySettings({ token: "" });
		const { bridge } = bridgeFor({ activeGm: thisGm, colonyPlanner, settings });
		bridge.evaluate();
		await bridge.saveLink({ campaignName: "Ridgehold", token: "new-token" });
		await settle();
		expect(values.token).toBe("new-token");
		expect(colonyPlanner.requests[0]?.authorization).toBe("Bearer new-token");
		expect(bridge.status()).toMatchObject({ campaignName: "Ridgehold", condition: "running" });
		await bridge.forgetLink();
		expect(values.token).toBe("");
		expect(colonyPlanner.liveStreams()).toBe(0);
	});

	test("pairing again while the old link's requests are in flight keeps the new link whole", async () => {
		const colonyPlanner = fakeColonyPlanner({ holdRequestsWithToken: "old-token" });
		const { settings, values } = memorySettings({ token: "old-token" });
		values.cursor = 50;
		const { bridge, errors } = bridgeFor({ activeGm: thisGm, colonyPlanner, settings });
		bridge.evaluate();
		await settle();
		expect(colonyPlanner.heldCount()).toBe(2);
		await bridge.saveLink({ campaignName: "Ridgehold", token: "new-token" });
		await settle();
		colonyPlanner.refuseHeldRequests();
		await settle();
		expect(values.token).toBe("new-token");
		expect(values.cursor).toBe(0);
		expect(errors).toEqual([]);
		expect(bridge.status()).toMatchObject({ condition: "running", connection: "live" });
		expect(colonyPlanner.liveStreams()).toBe(1);
	});

	test("while a code is traded the bridge is stopped, and it starts again when pairing fails", async () => {
		const colonyPlanner = fakeColonyPlanner();
		const { bridge, foundryGame } = bridgeFor({
			activeGm: thisGm,
			colonyPlanner,
			settings: memorySettings({}).settings,
		});
		bridge.evaluate();
		await settle();
		const endPairing = bridge.beginPairing();
		await settle();
		expect(colonyPlanner.liveStreams()).toBe(0);
		foundryGame.users.activeGM = thisGm;
		bridge.evaluate();
		await settle();
		expect(colonyPlanner.liveStreams()).toBe(0);
		endPairing();
		await settle();
		expect(colonyPlanner.liveStreams()).toBe(1);
	});
});
