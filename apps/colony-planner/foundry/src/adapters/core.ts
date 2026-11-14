import type {
	DocumentClass,
	FlaggedDocument,
	FoundryChatMessage,
	FoundryCollection,
	FoundryFolder,
	FoundryGame,
	FoundryJournalEntry,
	JournalEntryClass,
	OwnedDocument,
} from "../foundry-types";
import { escapeHtml } from "../html";
import { moduleId } from "../settings";
import { secondsPerDay } from "../time-tracker";
import {
	type ColonySummaryPayload,
	type DaysAdvancedPayload,
	isColonySummaryPayload,
	isDaysAdvancedPayload,
	isDeliveryPayload,
	isDeliveryUndonePayload,
	isNpcPayload,
	isPlacePayload,
	isPollClosedPayload,
	isPollPostedPayload,
	isQuestPayload,
	isWithdrawPayload,
	type OutboxRow,
} from "../wire";
import type { BridgeAdapter } from "./adapter";
import {
	formatNumber,
	renderDelivery,
	renderDeliveryUndone,
	renderNextSession,
	renderNpc,
	renderPlace,
	renderPollClosed,
	renderPollPosted,
	renderQuest,
	siteLink,
} from "./render";

/** Every write the bridge makes carries this, so the module's own hooks skip it (design §6). */
export function bridgeWriteOptions(): { colonyBridge: true } {
	return { colonyBridge: true };
}

const colonyExternalId = "colony";
// A DAYS_ADVANCED message carries this flag until Foundry's clock has moved, so a retried row finishes
// a clock move that failed after its message was posted.
const clockOwedFlag = "clockOwed";
// A withdrawn entry carries this flag, so its next upsert shows it to players again.
const withdrawnFlag = "withdrawn";
const noneOwnership = 0;
const observerOwnership = 2;
const htmlPageFormat = 1;
const bridgePageName = "Colony Planner";
const withdrawnName = "Hidden on Colony Planner";
export const chatSpeaker = { alias: "Colony Planner" };

interface ContentFolder {
	externalId: string;
	name: string;
}

const npcFolder: ContentFolder = { externalId: "folder:npcs", name: "Colony NPCs" };
const placeFolder: ContentFolder = { externalId: "folder:places", name: "Colony Places" };
const questFolder: ContentFolder = { externalId: "folder:quests", name: "Colony Quests" };
const contentKeyPrefixes = ["npc:", "place:", "quest:"];
const deliveryKeyPrefix = "delivery:";
const deliveryUndoneKeyPrefix = "delivery-undone:";

export interface CoreAdapterDependencies {
	chatMessage: DocumentClass<FoundryChatMessage>;
	folder: DocumentClass<FoundryFolder>;
	foundryGame: Pick<FoundryGame, "folders" | "journal" | "messages" | "time" | "users">;
	journalEntry: JournalEntryClass;
	moveClockWithColonyDays: () => boolean;
}

/** Journal and chat, which every game system has: what a world gets when no system adapter fits. */
export function createCoreAdapter(dependencies: CoreAdapterDependencies): BridgeAdapter {
	const withdraw = (row: OutboxRow) => applyWithdraw(row, dependencies);
	return {
		appliers: {
			COLONY_SUMMARY: (row) => applyColonySummary(row, dependencies),
			DAYS_ADVANCED: (row) => applyDaysAdvanced(row, dependencies),
			DELIVERY: (row) => applyDelivery(row, dependencies),
			DELIVERY_UNDONE: (row) => applyDeliveryUndone(row, dependencies),
			NPC_UPSERT: (row) => applyNpcUpsert(row, dependencies),
			NPC_WITHDRAW: withdraw,
			PLACE_UPSERT: (row) => applyPlaceUpsert(row, dependencies),
			PLACE_WITHDRAW: withdraw,
			POLL_CLOSED: (row) => applyPollClosed(row, dependencies),
			POLL_POSTED: (row) => applyPollPosted(row, dependencies),
			QUEST_UPSERT: (row) => applyQuestUpsert(row, dependencies),
			QUEST_WITHDRAW: withdraw,
		},
		name: "core",
		withdrawBridgedEntries: () => withdrawBridgedEntries(dependencies),
	};
}

export function externalIdFlag(externalId: string) {
	return { [moduleId]: { externalId } };
}

export function hasExternalId(document: FlaggedDocument, externalId: string): boolean {
	return document.getFlag(moduleId, "externalId") === externalId;
}

function gameMasterIds(dependencies: Pick<CoreAdapterDependencies, "foundryGame">): Set<string> {
	return new Set(dependencies.foundryGame.users.filter((user) => user.isGM).map((user) => user.id));
}

function playerGrants(document: OwnedDocument, gameMasters: Set<string>): string[] {
	return Object.keys(document.ownership ?? {}).filter(
		(userId) => userId !== "default" && !gameMasters.has(userId),
	);
}

/**
 * Players allowed to make journal entries can flag one with any key, and an owner can change one, so
 * the bridge uses an entry or folder only while no player could have: the server-set last editor is
 * a GM and no player has a level of their own above None.
 */
function isGameMasterDocument(document: OwnedDocument, gameMasters: Set<string>): boolean {
	const lastModifiedBy = document._stats?.lastModifiedBy;
	if (typeof lastModifiedBy === "string" && !gameMasters.has(lastModifiedBy)) {
		return false;
	}
	return playerGrants(document, gameMasters).every(
		(userId) => (document.ownership?.[userId] ?? noneOwnership) <= noneOwnership,
	);
}

function findGameMasterDocument<Document extends OwnedDocument>(
	documents: FoundryCollection<Document>,
	externalId: string,
	dependencies: Pick<CoreAdapterDependencies, "foundryGame">,
): Document | undefined {
	const gameMasters = gameMasterIds(dependencies);
	return documents.find(
		(document) =>
			hasExternalId(document, externalId) && isGameMasterDocument(document, gameMasters),
	);
}

/** Players can post chat messages carrying any flag, so only a GM's message marks a row done. */
export function findGameMasterMessage(
	externalId: string,
	dependencies: Pick<CoreAdapterDependencies, "foundryGame">,
): FoundryChatMessage | undefined {
	return dependencies.foundryGame.messages.find(
		(message) => message.author?.isGM === true && hasExternalId(message, externalId),
	);
}

function isContentEntry(entry: FlaggedDocument): boolean {
	const externalId = entry.getFlag(moduleId, "externalId");
	return (
		typeof externalId === "string" &&
		contentKeyPrefixes.some((prefix) => externalId.startsWith(prefix))
	);
}

function htmlPage(externalId: string, name: string, content: string) {
	return {
		flags: externalIdFlag(externalId),
		name,
		text: { content, format: htmlPageFormat },
		type: "text",
	};
}

/** Only the bridge's own page is replaced; pages the GM added to the entry stay. */
async function replaceBridgePage(
	entry: FoundryJournalEntry,
	externalId: string,
	pageData: ReturnType<typeof htmlPage>,
): Promise<void> {
	const page = entry.pages.find((journalPage) => hasExternalId(journalPage, externalId));
	if (page) {
		await page.update({ "text.content": pageData.text.content }, bridgeWriteOptions());
		return;
	}
	await entry.createEmbeddedDocuments("JournalEntryPage", [pageData], bridgeWriteOptions());
}

async function applyColonySummary(
	row: OutboxRow,
	dependencies: CoreAdapterDependencies,
): Promise<void> {
	if (!isColonySummaryPayload(row.payload)) {
		throw new Error("The colony summary's payload wasn't understood.");
	}
	const entry = findGameMasterDocument(
		dependencies.foundryGame.journal,
		colonyExternalId,
		dependencies,
	);
	const pageData = htmlPage(colonyExternalId, "Colony", renderColonySummary(row.payload));
	if (!entry) {
		await dependencies.journalEntry.create(
			{
				flags: externalIdFlag(colonyExternalId),
				name: "Colony",
				ownership: { default: observerOwnership },
				pages: [pageData],
			},
			bridgeWriteOptions(),
		);
		return;
	}
	await replaceBridgePage(entry, colonyExternalId, pageData);
}

async function findOrCreateFolder(
	folder: ContentFolder,
	dependencies: CoreAdapterDependencies,
): Promise<FoundryFolder> {
	const existing = findGameMasterDocument(
		dependencies.foundryGame.folders,
		folder.externalId,
		dependencies,
	);
	if (existing) {
		return existing;
	}
	const created = await dependencies.folder.create(
		{ flags: externalIdFlag(folder.externalId), name: folder.name, type: "JournalEntry" },
		bridgeWriteOptions(),
	);
	if (!created) {
		throw new Error(`Foundry didn't make the ${folder.name} folder.`);
	}
	return created;
}

/**
 * Creates the target's entry in its folder, readable by players, or updates the one it finds by
 * flag, leaving the folder, ownership and other pages as the GM set them. The page is written before
 * the entry, so a withdrawn entry is shown to players again only once its page is current.
 */
async function upsertEntry(
	row: OutboxRow,
	content: { folder: ContentFolder; html: string; name: string },
	dependencies: CoreAdapterDependencies,
): Promise<void> {
	const pageData = htmlPage(row.targetKey, bridgePageName, content.html);
	const entry = findGameMasterDocument(
		dependencies.foundryGame.journal,
		row.targetKey,
		dependencies,
	);
	if (!entry) {
		const folder = await findOrCreateFolder(content.folder, dependencies);
		await dependencies.journalEntry.create(
			{
				flags: externalIdFlag(row.targetKey),
				folder: folder.id,
				name: content.name,
				ownership: { default: observerOwnership },
				pages: [pageData],
			},
			bridgeWriteOptions(),
		);
		return;
	}
	await replaceBridgePage(entry, row.targetKey, pageData);
	const reopening = entry.getFlag(moduleId, withdrawnFlag) === true;
	await entry.update(
		{
			name: content.name,
			...(reopening && {
				[`flags.${moduleId}.${withdrawnFlag}`]: false,
				"ownership.default": observerOwnership,
			}),
		},
		bridgeWriteOptions(),
	);
}

/**
 * Players may receive documents they can't open, so a withdrawn entry also loses its name and every
 * player's own level, and its bridge page is blanked after (design §13.7).
 */
function withdrawUpdate(entry: FoundryJournalEntry, gameMasters: Set<string>) {
	return {
		[`flags.${moduleId}.${withdrawnFlag}`]: true,
		name: withdrawnName,
		"ownership.default": noneOwnership,
		...Object.fromEntries(
			playerGrants(entry, gameMasters).map((userId) => [`ownership.${userId}`, noneOwnership]),
		),
	};
}

function bridgePageOf(entry: FoundryJournalEntry) {
	const externalId = entry.getFlag(moduleId, "externalId");
	return typeof externalId === "string"
		? entry.pages.find((page) => hasExternalId(page, externalId))
		: undefined;
}

/** One write per page that still shows text, so a repeated withdrawal writes only the entries. */
async function blankBridgePages(entries: FoundryJournalEntry[]): Promise<void> {
	const pages = entries.flatMap((entry) => {
		const page = bridgePageOf(entry);
		return page && page.text?.content !== "" ? [page] : [];
	});
	await Promise.all(pages.map((page) => page.update({ "text.content": "" }, bridgeWriteOptions())));
}

/**
 * Hides the target's entries from players and never deletes them, so the GM keeps the entry and the
 * pages they added. Every entry with the key is hidden, a player's own included: hiding can't leak.
 */
async function applyWithdraw(row: OutboxRow, dependencies: CoreAdapterDependencies): Promise<void> {
	if (!isWithdrawPayload(row.payload)) {
		throw new Error(`The ${row.kind} row's payload wasn't understood.`);
	}
	const entries = dependencies.foundryGame.journal.filter((journalEntry) =>
		hasExternalId(journalEntry, row.targetKey),
	);
	const gameMasters = gameMasterIds(dependencies);
	await Promise.all(
		entries.map((entry) => entry.update(withdrawUpdate(entry, gameMasters), bridgeWriteOptions())),
	);
	await blankBridgePages(entries);
}

async function withdrawBridgedEntries(dependencies: CoreAdapterDependencies): Promise<void> {
	const entries = dependencies.foundryGame.journal.filter(isContentEntry);
	if (entries.length === 0) {
		return;
	}
	const gameMasters = gameMasterIds(dependencies);
	await dependencies.journalEntry.updateDocuments(
		entries.map((entry) => ({ _id: entry.id, ...withdrawUpdate(entry, gameMasters) })),
		bridgeWriteOptions(),
	);
	await blankBridgePages(entries);
}

async function applyNpcUpsert(
	row: OutboxRow,
	dependencies: CoreAdapterDependencies,
): Promise<void> {
	if (!isNpcPayload(row.payload)) {
		throw new Error("The NPC's payload wasn't understood.");
	}
	await upsertEntry(
		row,
		{ folder: npcFolder, html: renderNpc(row.payload), name: row.payload.displayName },
		dependencies,
	);
}

async function applyPlaceUpsert(
	row: OutboxRow,
	dependencies: CoreAdapterDependencies,
): Promise<void> {
	if (!isPlacePayload(row.payload)) {
		throw new Error("The place's payload wasn't understood.");
	}
	await upsertEntry(
		row,
		{ folder: placeFolder, html: renderPlace(row.payload), name: row.payload.name },
		dependencies,
	);
}

async function applyQuestUpsert(
	row: OutboxRow,
	dependencies: CoreAdapterDependencies,
): Promise<void> {
	if (!isQuestPayload(row.payload)) {
		throw new Error("The quest's payload wasn't understood.");
	}
	await upsertEntry(
		row,
		{ folder: questFolder, html: renderQuest(row.payload), name: row.payload.title },
		dependencies,
	);
}

export function findPostedMessage(
	row: OutboxRow,
	dependencies: Pick<CoreAdapterDependencies, "foundryGame">,
): FoundryChatMessage | undefined {
	return findGameMasterMessage(row.targetKey, dependencies);
}

/** One-shot chat kinds post once: a message flagged with the row's target key means it's done. */
async function postOnce(
	row: OutboxRow,
	message: { content: string; whisper?: string[] },
	dependencies: CoreAdapterDependencies,
): Promise<void> {
	if (findPostedMessage(row, dependencies)) {
		return;
	}
	await dependencies.chatMessage.create(
		{ ...message, flags: externalIdFlag(row.targetKey), speaker: chatSpeaker },
		bridgeWriteOptions(),
	);
}

async function applyPollPosted(
	row: OutboxRow,
	dependencies: CoreAdapterDependencies,
): Promise<void> {
	if (!isPollPostedPayload(row.payload)) {
		throw new Error("The poll's payload wasn't understood.");
	}
	await postOnce(row, { content: renderPollPosted(row.payload) }, dependencies);
}

async function applyPollClosed(
	row: OutboxRow,
	dependencies: CoreAdapterDependencies,
): Promise<void> {
	if (!isPollClosedPayload(row.payload)) {
		throw new Error("The closed poll's payload wasn't understood.");
	}
	await postOnce(row, { content: renderPollClosed(row.payload) }, dependencies);
}

async function applyDelivery(row: OutboxRow, dependencies: CoreAdapterDependencies): Promise<void> {
	if (!isDeliveryPayload(row.payload)) {
		throw new Error("The delivery's payload wasn't understood.");
	}
	await postOnce(row, { content: renderDelivery(row.payload) }, dependencies);
}

async function applyDeliveryUndone(
	row: OutboxRow,
	dependencies: CoreAdapterDependencies,
): Promise<void> {
	if (!isDeliveryUndonePayload(row.payload)) {
		throw new Error("The undone delivery's payload wasn't understood.");
	}
	if (!row.targetKey.startsWith(deliveryUndoneKeyPrefix)) {
		throw new Error("The undone delivery's target key wasn't understood.");
	}
	const deliveryKey = deliveryKeyPrefix + row.targetKey.slice(deliveryUndoneKeyPrefix.length);
	// Every undone Pay sends this row; a world that never posted the delivery has nothing to clean up.
	if (!findGameMasterMessage(deliveryKey, dependencies)) {
		return;
	}
	const gameMasterIds = dependencies.foundryGame.users
		.filter((user) => user.isGM)
		.map((user) => user.id);
	// Foundry posts a message with no whisper recipients to everyone.
	if (gameMasterIds.length === 0) {
		throw new Error("Foundry has no GM to whisper the undone delivery to.");
	}
	await postOnce(
		row,
		{ content: renderDeliveryUndone(row.payload), whisper: gameMasterIds },
		dependencies,
	);
}

async function applyDaysAdvanced(
	row: OutboxRow,
	dependencies: CoreAdapterDependencies,
): Promise<void> {
	if (!isDaysAdvancedPayload(row.payload)) {
		throw new Error("The colony day change's payload wasn't understood.");
	}
	const message =
		findPostedMessage(row, dependencies) ??
		(await dependencies.chatMessage.create(
			{
				content: renderDaysAdvanced(row.payload),
				flags: {
					[moduleId]: {
						externalId: row.targetKey,
						...(dependencies.moveClockWithColonyDays() && { [clockOwedFlag]: true }),
					},
				},
				speaker: chatSpeaker,
			},
			bridgeWriteOptions(),
		));
	if (!message) {
		throw new Error("Foundry didn't post the colony's day change in chat.");
	}
	if (message.getFlag(moduleId, clockOwedFlag) !== true) {
		return;
	}
	await dependencies.foundryGame.time.advance(
		row.payload.days * secondsPerDay,
		bridgeWriteOptions(),
	);
	await message.update({ [`flags.${moduleId}.${clockOwedFlag}`]: false }, bridgeWriteOptions());
}

function dayLine(label: string, day: number | null): string {
	if (day === null) {
		return "";
	}
	return `<p>${label}: Day ${formatNumber(day)}</p>`;
}

function listOrNote(items: string[], emptyNote: string): string {
	if (items.length === 0) {
		return `<p>${emptyNote}</p>`;
	}
	return `<ul>${items.map((item) => `<li>${item}</li>`).join("")}</ul>`;
}

/** The journal page's HTML. Every string from the payload is escaped. */
export function renderColonySummary(summary: ColonySummaryPayload): string {
	const resources = summary.resources.map(
		(resource) => `${escapeHtml(resource.name)}: ${formatNumber(resource.amount)}`,
	);
	const workforce = summary.workforce.map(
		(group) => `${escapeHtml(group.role)}: ${formatNumber(group.count)}`,
	);
	return [
		`<h2>${escapeHtml(summary.campaignName)}</h2>`,
		`<p><strong>Day ${formatNumber(summary.day)}</strong></p>`,
		dayLine("Next shipment", summary.nextShipmentDay),
		dayLine("Next goods ship docks", summary.nextGoodsShipDay),
		summary.nextSession ? renderNextSession(summary.nextSession) : "",
		"<h3>Stockpile</h3>",
		listOrNote(resources, "Nothing stockpiled."),
		"<h3>Workforce</h3>",
		listOrNote(workforce, "No workforce."),
		siteLink(summary.siteUrl, `Open ${summary.campaignName} on Colony Planner`),
	].join("");
}

export function renderDaysAdvanced(change: DaysAdvancedPayload): string {
	const dayWord = change.days === 1 ? "day" : "days";
	return `<p>The colony advanced ${formatNumber(change.days)} ${dayWord}, from Day ${formatNumber(change.fromDay)} to Day ${formatNumber(change.toDay)}.</p>`;
}
