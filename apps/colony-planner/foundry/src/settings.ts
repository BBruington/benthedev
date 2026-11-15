import type { FoundrySettings } from "./foundry-types";
import { type InboundEvent, isInboundEvent, readField } from "./wire";

export const moduleId = "colony-bridge";
export const depositsSettingKey = "deposits";

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
	/** The Foundry world the link was paired in; "" for none, and for a link saved by 0.1.1. */
	linkedWorldId(): string;
	/** A random ID naming this link, never derived from its token; "" for none. */
	linkId(): string;
	moveClockWithColonyDays(): boolean;
	pendingEvents(): InboundEvent[];
	saveAcknowledgedThrough(seq: number): Promise<unknown>;
	saveLastReportedDay(day: number): Promise<unknown>;
	saveLink(link: BridgeLink): Promise<unknown>;
	savePendingEvents(events: InboundEvent[]): Promise<unknown>;
	saveWithdrawOwed(owed: boolean): Promise<unknown>;
	token(): string;
	/** True from a new link's save until the bridge's old journal entries are withdrawn (design §13.5). */
	withdrawOwed(): boolean;
}

/**
 * A link as this browser stores it. Client settings are shared by every world this browser opens on
 * the same Foundry server, so the link names its world and other worlds treat it as absent.
 */
export interface BridgeLink {
	campaignName: string;
	token: string;
	worldId: string;
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
	// The escrow (design §14.10): world scope so players can't write it, and it outlives a new link.
	settings.register(moduleId, depositsSettingKey, {
		config: false,
		default: { entries: {} },
		scope: "world",
		type: Object,
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
	settings.register(moduleId, "worldId", {
		config: false,
		default: "",
		scope: "client",
		type: String,
	});
	settings.register(moduleId, "linkId", {
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
	settings.register(moduleId, "withdrawOwed", {
		config: false,
		default: false,
		scope: "client",
		type: Boolean,
	});
	settings.register(moduleId, "pendingEvents", {
		config: false,
		default: { events: [] },
		scope: "client",
		type: Object,
	});
}

/**
 * Gives a stored link saved before 0.3.0, whose ID is "", a random one, so no deposit is ever recorded
 * under an ID that another such link shares.
 */
export async function assignMissingLinkId(settings: FoundrySettings): Promise<void> {
	const token = settings.get(moduleId, "token");
	const linkId = settings.get(moduleId, "linkId");
	if (readString(token) !== "" && readString(linkId) === "") {
		await settings.set(moduleId, "linkId", crypto.randomUUID());
	}
}

export function createBridgeSettings(settings: FoundrySettings): BridgeSettings {
	const read = (key: string) => settings.get(moduleId, key);
	const write = (key: string, value: unknown) => settings.set(moduleId, key, value);
	return {
		acknowledgedThrough: () => readNumber(read("outboxCursor")) ?? 0,
		bridgeEnabled: () => read("enabled") === true,
		campaignName: () => readString(read("campaignName")),
		lastReportedDay: () => readNumber(readField(read("timeTracker"), "day")),
		linkedWorldId: () => readString(read("worldId")),
		linkId: () => readString(read("linkId")),
		moveClockWithColonyDays: () => read("moveClock") === true,
		pendingEvents: () => {
			const events = readField(read("pendingEvents"), "events");
			return Array.isArray(events) ? events.filter(isInboundEvent) : [];
		},
		saveAcknowledgedThrough: (seq) => write("outboxCursor", seq),
		saveLastReportedDay: (day) => write("timeTracker", { day }),
		// The cursor and the queue restart with every link: a new link may belong to another campaign or
		// world, whose seq numbers and events are its own. The old token goes first and the new one last,
		// so no token is ever stored beside another link's world.
		saveLink: async (link) => {
			await write("token", "");
			await write("outboxCursor", 0);
			await write("pendingEvents", { events: [] });
			await write("withdrawOwed", link.token !== "");
			await write("campaignName", link.campaignName);
			await write("worldId", link.worldId);
			await write("linkId", link.token === "" ? "" : crypto.randomUUID());
			await write("token", link.token);
		},
		savePendingEvents: (events) => write("pendingEvents", { events }),
		saveWithdrawOwed: (owed) => write("withdrawOwed", owed),
		token: () => readString(read("token")),
		withdrawOwed: () => read("withdrawOwed") === true,
	};
}

function readString(value: unknown): string {
	return typeof value === "string" ? value : "";
}

function readNumber(value: unknown): number | undefined {
	return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}
