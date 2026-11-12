import type {
	DocumentClass,
	FlaggedDocument,
	FoundryGame,
	FoundryJournalEntry,
} from "../foundry-types";
import { escapeHtml } from "../html";
import { moduleId } from "../settings";
import { secondsPerDay } from "../time-tracker";
import {
	type ColonySummaryPayload,
	type DaysAdvancedPayload,
	isColonySummaryPayload,
	isDaysAdvancedPayload,
	type OutboxRow,
} from "../wire";
import type { BridgeAdapter } from "./adapter";

/** Every write the bridge makes carries this, so the module's own hooks skip it (design §6). */
export function bridgeWriteOptions(): { colonyBridge: true } {
	return { colonyBridge: true };
}

const colonyExternalId = "colony";
const observerOwnership = 2;
const htmlPageFormat = 1;

export interface CoreAdapterDependencies {
	chatMessage: DocumentClass<FlaggedDocument>;
	foundryGame: Pick<FoundryGame, "journal" | "messages" | "time">;
	journalEntry: DocumentClass<FoundryJournalEntry>;
	moveClockWithColonyDays: () => boolean;
}

/** Journal and chat, which every game system has: what a world gets when no system adapter fits. */
export function createCoreAdapter(dependencies: CoreAdapterDependencies): BridgeAdapter {
	return {
		appliers: {
			COLONY_SUMMARY: (row) => applyColonySummary(row, dependencies),
			DAYS_ADVANCED: (row) => applyDaysAdvanced(row, dependencies),
		},
		name: "core",
	};
}

function externalIdFlag(externalId: string) {
	return { [moduleId]: { externalId } };
}

function hasExternalId(document: FlaggedDocument, externalId: string): boolean {
	return document.getFlag(moduleId, "externalId") === externalId;
}

async function applyColonySummary(
	row: OutboxRow,
	dependencies: CoreAdapterDependencies,
): Promise<void> {
	if (!isColonySummaryPayload(row.payload)) {
		throw new Error("The colony summary's payload wasn't understood.");
	}
	const content = renderColonySummary(row.payload);
	const entry = dependencies.foundryGame.journal.find((journalEntry) =>
		hasExternalId(journalEntry, colonyExternalId),
	);
	const pageData = {
		flags: externalIdFlag(colonyExternalId),
		name: "Colony",
		text: { content, format: htmlPageFormat },
		type: "text",
	};
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
	// Only the bridge's own page is replaced; pages the GM added to the entry stay.
	const page = entry.pages.find((journalPage) => hasExternalId(journalPage, colonyExternalId));
	if (page) {
		await page.update({ "text.content": content }, bridgeWriteOptions());
		return;
	}
	await entry.createEmbeddedDocuments("JournalEntryPage", [pageData], bridgeWriteOptions());
}

async function applyDaysAdvanced(
	row: OutboxRow,
	dependencies: CoreAdapterDependencies,
): Promise<void> {
	if (!isDaysAdvancedPayload(row.payload)) {
		throw new Error("The colony day change's payload wasn't understood.");
	}
	const alreadyPosted = dependencies.foundryGame.messages.find((message) =>
		hasExternalId(message, row.targetKey),
	);
	if (alreadyPosted) {
		return;
	}
	await dependencies.chatMessage.create(
		{
			content: renderDaysAdvanced(row.payload),
			flags: externalIdFlag(row.targetKey),
			speaker: { alias: "Colony Planner" },
		},
		bridgeWriteOptions(),
	);
	if (dependencies.moveClockWithColonyDays()) {
		await dependencies.foundryGame.time.advance(
			row.payload.days * secondsPerDay,
			bridgeWriteOptions(),
		);
	}
}

function formatNumber(value: number): string {
	return value.toLocaleString("en-US");
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

function isWebAddress(address: string): boolean {
	try {
		const protocol = new URL(address).protocol;
		return protocol === "https:" || protocol === "http:";
	} catch {
		return false;
	}
}

/** The journal page's HTML. Every string from the payload is escaped. */
export function renderColonySummary(summary: ColonySummaryPayload): string {
	const resources = summary.resources.map(
		(resource) => `${escapeHtml(resource.name)}: ${formatNumber(resource.amount)}`,
	);
	const workforce = summary.workforce.map(
		(group) => `${escapeHtml(group.role)}: ${formatNumber(group.count)}`,
	);
	const siteLink = isWebAddress(summary.siteUrl)
		? `<p><a href="${escapeHtml(summary.siteUrl)}">Open ${escapeHtml(summary.campaignName)} on Colony Planner</a></p>`
		: "";
	return [
		`<h2>${escapeHtml(summary.campaignName)}</h2>`,
		`<p><strong>Day ${formatNumber(summary.day)}</strong></p>`,
		dayLine("Next shipment", summary.nextShipmentDay),
		dayLine("Next goods ship docks", summary.nextGoodsShipDay),
		"<h3>Stockpile</h3>",
		listOrNote(resources, "Nothing stockpiled."),
		"<h3>Workforce</h3>",
		listOrNote(workforce, "No workforce."),
		siteLink,
	].join("");
}

export function renderDaysAdvanced(change: DaysAdvancedPayload): string {
	const dayWord = change.days === 1 ? "day" : "days";
	return `<p>The colony advanced ${formatNumber(change.days)} ${dayWord}, from Day ${formatNumber(change.fromDay)} to Day ${formatNumber(change.toDay)}.</p>`;
}
