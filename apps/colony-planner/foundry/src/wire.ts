/** The bridge's wire format, docs/design/foundry-bridge.md §12, §13 and §14, and guards for what the server sends. */

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

export const inboundEventKinds = [
	"STATUS",
	"TIME_ADVANCED",
	"COIN_DEPOSIT",
	"NPC_IMPORT",
	"ITEM_IMPORT",
] as const;

export type InboundEventKind = (typeof inboundEventKinds)[number];

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

export interface CoinDepositPayload {
	gold: number;
	actorId: string;
	actorName: string;
}

/** Field names are the site's NpcInput's, so the NPC sheet fills from them directly. */
export interface NpcImportPayload {
	appearance: string;
	characterClass: string;
	dmNotes: string;
	foundryUuid: string;
	level: number | null;
	name: string;
	race: string;
}

/** Field names are the site's MarketItemInput's. */
export interface ItemImportPayload {
	compendiumUuid: string | null;
	description: string;
	foundryUuid: string;
	name: string;
	priceGold: number | null;
}

export interface InboundPayloads {
	COIN_DEPOSIT: CoinDepositPayload;
	ITEM_IMPORT: ItemImportPayload;
	NPC_IMPORT: NpcImportPayload;
	STATUS: StatusPayload;
	TIME_ADVANCED: TimeAdvancedPayload;
}

export interface InboundEvent {
	id: string;
	kind: InboundEventKind;
	occurredAt: string;
	payload: InboundPayloads[InboundEventKind];
}

/** A COIN_DEPOSIT event with its payload's type known. */
export interface CoinDepositEvent extends InboundEvent {
	kind: "COIN_DEPOSIT";
	payload: CoinDepositPayload;
}

export interface DepositRefundPayload {
	depositId: string;
	gold: number;
	actorId: string;
	actorName: string;
}

export interface DepositUndonePayload {
	depositId: string;
	gold: number;
	actorName: string;
}

export interface PairResponse {
	campaignName: string;
	token: string;
}

export interface NextSession {
	endsAt: string;
	localDate: string;
	startsAt: string;
	timezone: string;
	title: string;
}

export interface ColonySummaryPayload {
	campaignName: string;
	day: number;
	nextGoodsShipDay: number | null;
	/** Absent from rows a Phase 1 server queued before the update. */
	nextSession?: NextSession | null;
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

export const npcStatuses = ["ALIVE", "DEAD", "MISSING", "UNKNOWN"] as const;
export const npcHealths = ["HEALTHY", "INJURED", "SICK", "UNKNOWN", "DEAD"] as const;
export const npcRelations = ["ALLY", "NEUTRAL", "SUSPICIOUS", "ENEMY", "UNKNOWN"] as const;
export const questKinds = ["MAINLAND", "COLONY"] as const;
export const questStatuses = ["ACTIVE", "COMPLETED", "FAILED"] as const;
export const pollVotings = ["SITE", "DISCORD"] as const;

export interface NpcPayload {
	age: string;
	aliases: string[];
	appearance: string;
	backstory: string;
	characterClass: string;
	colonyRole: string | null;
	currentPlace: string | null;
	displayName: string;
	health: (typeof npcHealths)[number];
	labels: string[];
	level: number | null;
	originPlace: string | null;
	personality: string;
	playerNotes: string;
	race: string;
	relation: (typeof npcRelations)[number];
	siteUrl: string;
	status: (typeof npcStatuses)[number];
}

export interface PlacePayload {
	description: string;
	name: string;
	playerNotes: string;
	siteUrl: string;
}

export interface QuestLoot {
	description: string;
	name: string;
	quantity: number;
}

export interface QuestPayload {
	description: string;
	giver: string | null;
	kind: (typeof questKinds)[number];
	loot: QuestLoot[];
	patron: string | null;
	resolvedDay: number | null;
	reward: string;
	rewardFavor: number;
	siteUrl: string;
	status: (typeof questStatuses)[number];
	title: string;
}

export interface DeliveryItem {
	compendiumUuid: string | null;
	name: string;
	quantity: number;
}

export interface DeliveryPayload {
	items: DeliveryItem[];
	paidDay: number;
	summary: string;
	totalGold: number;
}

export interface DeliveryUndonePayload {
	items: { name: string; quantity: number }[];
	summary: string;
}

export interface PollPostedPayload {
	allowMultiple: boolean;
	closesAt: string;
	options: string[];
	pollsUrl: string;
	question: string;
	voting: (typeof pollVotings)[number];
}

export interface PollClosedPayload {
	closedAt: string;
	options: { count: number; label: string; otherVotes: number }[];
	question: string;
}

export const maximumFailureReasonLength = 300;
export const maximumStatusFieldLength = 100;
export const maximumEventsPerBatch = 100;
export const maximumReportedDays = 3650;

// Design §14.2's limits, which Colony Planner checks again.
export const maximumDepositGold = 1_000_000;
export const maximumActorNameLength = 100;
export const maximumFoundryUuidLength = 200;
export const maximumNpcNameLength = 80;
export const maximumNpcLevel = 100;
export const maximumNpcShortTextLength = 80;
export const maximumProseLength = 4000;
export const maximumItemNameLength = 80;
/** The server's game.MaxStored, the most any colony amount or price may be. */
export const maximumPriceGold = 2_147_483_647;
export const foundryActorIdPattern = /^[A-Za-z0-9]{16}$/;
export const importedActorUuidPattern =
	/^(Compendium\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.)?Actor\.[A-Za-z0-9]{16}$/;
export const importedItemUuidPattern =
	/^(Compendium\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.)?Item\.[A-Za-z0-9]{16}$/;
/** Design §13.6's pattern, the one form the catalog's Foundry item UUID takes. */
export const compendiumItemUuidPattern =
	/^Compendium\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.Item\.[A-Za-z0-9]{16}$/;

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

function isString(value: unknown): value is string {
	return typeof value === "string";
}

function isStringOrNull(value: unknown): value is string | null {
	return value === null || isString(value);
}

function isInteger(value: unknown): value is number {
	return Number.isSafeInteger(value);
}

function isIntegerOrNull(value: unknown): value is number | null {
	return value === null || isInteger(value);
}

function isListOf<Item>(value: unknown, isItem: (item: unknown) => item is Item): value is Item[] {
	return Array.isArray(value) && value.every(isItem);
}

function isOneOf<Value extends string>(values: readonly Value[], value: unknown): value is Value {
	return values.some((allowed) => allowed === value);
}

function isInstant(value: unknown): value is string {
	return isString(value) && !Number.isNaN(Date.parse(value));
}

function hasStringFields(value: unknown, fields: string[]): boolean {
	return fields.every((field) => isString(readField(value, field)));
}

const localDatePattern = /^\d{4}-\d{2}-\d{2}$/;

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

/** Checks an event a reload reads back from storage; an unknown kind would be refused, so it is dropped. */
export function isInboundEvent(value: unknown): value is InboundEvent {
	return (
		typeof readField(value, "id") === "string" &&
		isOneOf(inboundEventKinds, readField(value, "kind")) &&
		typeof readField(value, "occurredAt") === "string" &&
		typeof readField(value, "payload") === "object"
	);
}

export function isCoinDepositPayload(value: unknown): value is CoinDepositPayload {
	const gold = readField(value, "gold");
	const actorId = readField(value, "actorId");
	return (
		isInteger(gold) &&
		gold >= 1 &&
		gold <= maximumDepositGold &&
		isString(actorId) &&
		foundryActorIdPattern.test(actorId) &&
		isString(readField(value, "actorName"))
	);
}

export function isCoinDepositEvent(value: unknown): value is CoinDepositEvent {
	return (
		isInboundEvent(value) && value.kind === "COIN_DEPOSIT" && isCoinDepositPayload(value.payload)
	);
}

function isDepositAmount(value: unknown): value is number {
	return isInteger(value) && value >= 1 && value <= maximumDepositGold;
}

export function isDepositRefundPayload(value: unknown): value is DepositRefundPayload {
	return (
		hasStringFields(value, ["depositId", "actorId", "actorName"]) &&
		isDepositAmount(readField(value, "gold"))
	);
}

export function isDepositUndonePayload(value: unknown): value is DepositUndonePayload {
	return (
		hasStringFields(value, ["depositId", "actorName"]) && isDepositAmount(readField(value, "gold"))
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
		typeof readField(value, "siteUrl") === "string" &&
		isNextSessionOrNone(readField(value, "nextSession"))
	);
}

function isNextSessionOrNone(value: unknown): value is NextSession | null | undefined {
	if (value === undefined || value === null) {
		return true;
	}
	const localDate = readField(value, "localDate");
	return (
		hasStringFields(value, ["title", "timezone"]) &&
		isInstant(readField(value, "startsAt")) &&
		isInstant(readField(value, "endsAt")) &&
		isString(localDate) &&
		localDatePattern.test(localDate)
	);
}

export function isDaysAdvancedPayload(value: unknown): value is DaysAdvancedPayload {
	return (
		isFiniteNumber(readField(value, "fromDay")) &&
		isFiniteNumber(readField(value, "toDay")) &&
		Number.isSafeInteger(readField(value, "days"))
	);
}

/** A withdraw's payload is `{}`: it says nothing about the row it withdraws. */
export function isWithdrawPayload(value: unknown): value is Record<string, never> {
	return (
		typeof value === "object" &&
		value !== null &&
		!Array.isArray(value) &&
		Object.keys(value).length === 0
	);
}

export function isNpcPayload(value: unknown): value is NpcPayload {
	return (
		hasStringFields(value, [
			"displayName",
			"race",
			"characterClass",
			"age",
			"appearance",
			"personality",
			"backstory",
			"playerNotes",
			"siteUrl",
		]) &&
		isListOf(readField(value, "aliases"), isString) &&
		isListOf(readField(value, "labels"), isString) &&
		isIntegerOrNull(readField(value, "level")) &&
		isOneOf(npcStatuses, readField(value, "status")) &&
		isOneOf(npcHealths, readField(value, "health")) &&
		isOneOf(npcRelations, readField(value, "relation")) &&
		isStringOrNull(readField(value, "currentPlace")) &&
		isStringOrNull(readField(value, "originPlace")) &&
		isStringOrNull(readField(value, "colonyRole"))
	);
}

export function isPlacePayload(value: unknown): value is PlacePayload {
	return hasStringFields(value, ["name", "description", "playerNotes", "siteUrl"]);
}

function isQuestLoot(value: unknown): value is QuestLoot {
	return hasStringFields(value, ["name", "description"]) && isInteger(readField(value, "quantity"));
}

export function isQuestPayload(value: unknown): value is QuestPayload {
	return (
		hasStringFields(value, ["title", "description", "reward", "siteUrl"]) &&
		isOneOf(questKinds, readField(value, "kind")) &&
		isOneOf(questStatuses, readField(value, "status")) &&
		isStringOrNull(readField(value, "giver")) &&
		isStringOrNull(readField(value, "patron")) &&
		isInteger(readField(value, "rewardFavor")) &&
		isListOf(readField(value, "loot"), isQuestLoot) &&
		isIntegerOrNull(readField(value, "resolvedDay"))
	);
}

function isDeliveryItem(value: unknown): value is DeliveryItem {
	return (
		isString(readField(value, "name")) &&
		isInteger(readField(value, "quantity")) &&
		isStringOrNull(readField(value, "compendiumUuid"))
	);
}

export function isDeliveryPayload(value: unknown): value is DeliveryPayload {
	return (
		isInteger(readField(value, "paidDay")) &&
		isInteger(readField(value, "totalGold")) &&
		isString(readField(value, "summary")) &&
		isListOf(readField(value, "items"), isDeliveryItem)
	);
}

function isUndoneItem(value: unknown): value is DeliveryUndonePayload["items"][number] {
	return isString(readField(value, "name")) && isInteger(readField(value, "quantity"));
}

export function isDeliveryUndonePayload(value: unknown): value is DeliveryUndonePayload {
	return isString(readField(value, "summary")) && isListOf(readField(value, "items"), isUndoneItem);
}

export function isPollPostedPayload(value: unknown): value is PollPostedPayload {
	return (
		hasStringFields(value, ["question", "pollsUrl"]) &&
		isListOf(readField(value, "options"), isString) &&
		typeof readField(value, "allowMultiple") === "boolean" &&
		isOneOf(pollVotings, readField(value, "voting")) &&
		isInstant(readField(value, "closesAt"))
	);
}

function isPollResult(value: unknown): value is PollClosedPayload["options"][number] {
	return (
		isString(readField(value, "label")) &&
		isInteger(readField(value, "count")) &&
		isInteger(readField(value, "otherVotes"))
	);
}

export function isPollClosedPayload(value: unknown): value is PollClosedPayload {
	return (
		isString(readField(value, "question")) &&
		isInstant(readField(value, "closedAt")) &&
		isListOf(readField(value, "options"), isPollResult)
	);
}
