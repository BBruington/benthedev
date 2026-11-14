/** The bridge's wire format, docs/design/foundry-bridge.md §12, and guards for what the server sends. */

export interface OutboxRow {
	kind: string;
	payload: unknown;
	seq: number;
	targetKey: string;
}

export interface OutboxPage {
	more: boolean;
	rows: OutboxRow[];
}

export interface FailedRow {
	reason: string;
	seq: number;
}

export interface Acknowledgement {
	failed: FailedRow[];
	through: number;
}

export type InboundEventKind = "STATUS" | "TIME_ADVANCED";

export interface StatusPayload {
	foundryVersion: string;
	moduleVersion: string;
	systemId: string;
	systemVersion: string;
	worldTitle: string;
}

export interface TimeAdvancedPayload {
	days: number;
}

export interface InboundEvent {
	id: string;
	kind: InboundEventKind;
	occurredAt: string;
	payload: StatusPayload | TimeAdvancedPayload;
}

export interface PairResponse {
	campaignName: string;
	token: string;
}

export interface ColonySummaryPayload {
	campaignName: string;
	day: number;
	nextGoodsShipDay: number | null;
	nextShipmentDay: number | null;
	resources: { amount: number; name: string }[];
	siteUrl: string;
	workforce: { count: number; role: string }[];
}

export interface DaysAdvancedPayload {
	days: number;
	fromDay: number;
	toDay: number;
}

export const maximumFailureReasonLength = 300;
export const maximumStatusFieldLength = 100;
export const maximumEventsPerBatch = 100;
export const maximumReportedDays = 3650;

export function readField(value: unknown, field: string): unknown {
	if (typeof value !== "object" || value === null) {
		return undefined;
	}
	return (value as Record<string, unknown>)[field];
}

function isFiniteNumber(value: unknown): value is number {
	return typeof value === "number" && Number.isFinite(value);
}

function isDayOrNull(value: unknown): value is number | null {
	return value === null || isFiniteNumber(value);
}

export function isOutboxPage(value: unknown): value is OutboxPage {
	const rows = readField(value, "rows");
	return (
		typeof readField(value, "more") === "boolean" && Array.isArray(rows) && rows.every(isOutboxRow)
	);
}

function isOutboxRow(value: unknown): value is OutboxRow {
	return (
		typeof readField(value, "kind") === "string" &&
		Number.isSafeInteger(readField(value, "seq")) &&
		typeof readField(value, "targetKey") === "string"
	);
}

export function isPairResponse(value: unknown): value is PairResponse {
	return (
		typeof readField(value, "token") === "string" &&
		typeof readField(value, "campaignName") === "string"
	);
}

export function isInboundEvent(value: unknown): value is InboundEvent {
	const kind = readField(value, "kind");
	return (
		typeof readField(value, "id") === "string" &&
		(kind === "STATUS" || kind === "TIME_ADVANCED") &&
		typeof readField(value, "occurredAt") === "string" &&
		typeof readField(value, "payload") === "object"
	);
}

export function isColonySummaryPayload(value: unknown): value is ColonySummaryPayload {
	const resources = readField(value, "resources");
	const workforce = readField(value, "workforce");
	return (
		typeof readField(value, "campaignName") === "string" &&
		isFiniteNumber(readField(value, "day")) &&
		Array.isArray(resources) &&
		resources.every(
			(resource) =>
				typeof readField(resource, "name") === "string" &&
				isFiniteNumber(readField(resource, "amount")),
		) &&
		Array.isArray(workforce) &&
		workforce.every(
			(group) =>
				typeof readField(group, "role") === "string" && isFiniteNumber(readField(group, "count")),
		) &&
		isDayOrNull(readField(value, "nextShipmentDay")) &&
		isDayOrNull(readField(value, "nextGoodsShipDay")) &&
		typeof readField(value, "siteUrl") === "string"
	);
}

export function isDaysAdvancedPayload(value: unknown): value is DaysAdvancedPayload {
	return (
		isFiniteNumber(readField(value, "fromDay")) &&
		isFiniteNumber(readField(value, "toDay")) &&
		Number.isSafeInteger(readField(value, "days"))
	);
}
