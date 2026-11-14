import { describe, expect, test } from "bun:test";
import type { FoundrySettings, SettingConfig } from "./foundry-types";
import { createBridgeSettings, registerSettings } from "./settings";

function fakeSettings() {
	const values = new Map<string, unknown>();
	const configs = new Map<string, SettingConfig>();
	const writes: [string, unknown][] = [];
	const settings: FoundrySettings = {
		get: (_namespace, key) => values.get(key) ?? configs.get(key)?.default,
		register: (_namespace, key, config) => {
			configs.set(key, config);
		},
		registerMenu: () => undefined,
		set: (_namespace, key, value) => {
			writes.push([key, value]);
			values.set(key, value);
			return Promise.resolve(value);
		},
	};
	registerSettings(settings, () => undefined);
	return { configs, settings, values, writes };
}

describe("createBridgeSettings", () => {
	test("a link names its world, in this browser only, and starts with an empty cursor and queue", async () => {
		const { configs, settings, values, writes } = fakeSettings();
		values.set("outboxCursor", 40);
		values.set("pendingEvents", {
			events: [
				{
					id: "event-a",
					kind: "TIME_ADVANCED",
					occurredAt: "2026-11-12T10:00:00Z",
					payload: { days: 1 },
				},
			],
		});
		const bridgeSettings = createBridgeSettings(settings);
		await bridgeSettings.saveLink({
			campaignName: "Ridgehold",
			token: "new-token",
			worldId: "world-pf2e",
		});
		expect(configs.get("worldId")?.scope).toBe("client");
		expect(bridgeSettings.linkedWorldId()).toBe("world-pf2e");
		expect(bridgeSettings.acknowledgedThrough()).toBe(0);
		expect(bridgeSettings.pendingEvents()).toEqual([]);
		// The old token goes first and the new one last, so no token sits beside another link's world.
		expect(writes.at(0)).toEqual(["token", ""]);
		expect(writes.at(-1)).toEqual(["token", "new-token"]);
	});

	test("a link saved by 0.1.1 names no world", () => {
		const { settings, values } = fakeSettings();
		values.set("token", "old-token");
		expect(createBridgeSettings(settings).linkedWorldId()).toBe("");
	});
});
