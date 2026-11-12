import type { FoundrySettings } from "./foundry-types";
import { type InboundEvent, isInboundEvent, readField } from "./wire";

export const moduleId = "colony-bridge";

/**
 * Typed reads and writes of the module's settings. The token and everything tied to it are `client`
 * scope (this browser's local storage), because world settings reach every player's browser (design
 * B4). World settings hold nothing secret.
 */
export interface BridgeSettings {
	acknowledgedThrough(): number;
	bridgeEnabled(): boolean;
	campaignName(): string;
	lastReportedDay(): number | undefined;
	moveClockWithColonyDays(): boolean;
	pendingEvents(): InboundEvent[];
	saveAcknowledgedThrough(seq: number): Promise<unknown>;
	saveLastReportedDay(day: number): Promise<unknown>;
	saveLink(link: { campaignName: string; token: string }): Promise<unknown>;
	savePendingEvents(events: InboundEvent[]): Promise<unknown>;
	token(): string;
}

export function registerSettings(
	settings: FoundrySettings,
	onBridgeSettingChange: () => void,
): void {
	settings.register(moduleId, "enabled", {
		config: true,
		default: true,
		hint: "Turn the Colony Planner bridge on or off for this world.",
		name: "Bridge on",
		onChange: onBridgeSettingChange,
		scope: "world",
		type: Boolean,
	});
	settings.register(moduleId, "moveClock", {
		config: true,
		default: false,
		hint: "When Colony Planner advances the colony's days, move this world's clock by the same number of days.",
		name: "Move Foundry's clock with colony days",
		scope: "world",
		type: Boolean,
	});
	settings.register(moduleId, "timeTracker", {
		config: false,
		default: {},
		scope: "world",
		type: Object,
	});
	settings.register(moduleId, "token", {
		config: false,
		default: "",
		scope: "client",
		type: String,
	});
	settings.register(moduleId, "campaignName", {
		config: false,
		default: "",
		scope: "client",
		type: String,
	});
	settings.register(moduleId, "outboxCursor", {
		config: false,
		default: 0,
		scope: "client",
		type: Number,
	});
	settings.register(moduleId, "pendingEvents", {
		config: false,
		default: { events: [] },
		scope: "client",
		type: Object,
	});
}

export function createBridgeSettings(settings: FoundrySettings): BridgeSettings {
	const read = (key: string) => settings.get(moduleId, key);
	const write = (key: string, value: unknown) => settings.set(moduleId, key, value);
	return {
		acknowledgedThrough: () => readNumber(read("outboxCursor")) ?? 0,
		bridgeEnabled: () => read("enabled") === true,
		campaignName: () => readString(read("campaignName")),
		lastReportedDay: () => readNumber(readField(read("timeTracker"), "day")),
		moveClockWithColonyDays: () => read("moveClock") === true,
		pendingEvents: () => {
			const events = readField(read("pendingEvents"), "events");
			return Array.isArray(events) ? events.filter(isInboundEvent) : [];
		},
		saveAcknowledgedThrough: (seq) => write("outboxCursor", seq),
		saveLastReportedDay: (day) => write("timeTracker", { day }),
		// The cursor and the queue restart with every link: a new link may belong to another campaign,
		// whose seq numbers are its own.
		saveLink: async (link) => {
			await write("outboxCursor", 0);
			await write("pendingEvents", { events: [] });
			await write("campaignName", link.campaignName);
			await write("token", link.token);
		},
		savePendingEvents: (events) => write("pendingEvents", { events }),
		token: () => readString(read("token")),
	};
}

function readString(value: unknown): string {
	return typeof value === "string" ? value : "";
}

function readNumber(value: unknown): number | undefined {
	return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}
