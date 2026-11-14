import { escapeHtml } from "../html";
import type {
	DeliveryPayload,
	DeliveryUndonePayload,
	NextSession,
	NpcPayload,
	PlacePayload,
	PollClosedPayload,
	PollPostedPayload,
	QuestPayload,
} from "../wire";

const lineBreaks = /\r\n|\r|\n/g;

export function formatNumber(value: number): string {
	return value.toLocaleString("en-US");
}

export function isWebAddress(address: string): boolean {
	try {
		const protocol = new URL(address).protocol;
		return protocol === "https:" || protocol === "http:";
	} catch {
		return false;
	}
}

function isBlank(text: string): boolean {
	return text.trim() === "";
}

function multiline(text: string): string {
	return escapeHtml(text).replace(lineBreaks, "<br>");
}

function prose(heading: string, text: string): string {
	if (isBlank(text)) {
		return "";
	}
	return `<h3>${heading}</h3><p>${multiline(text)}</p>`;
}

/** Each fact is a label and a value already escaped; a blank value drops its fact. */
function facts(entries: [string, string][]): string {
	const shown = entries.filter(([, value]) => !isBlank(value));
	if (shown.length === 0) {
		return "";
	}
	return `<ul>${shown.map(([label, value]) => `<li><strong>${label}:</strong> ${value}</li>`).join("")}</ul>`;
}

function textOrBlank(text: string | null): string {
	return text === null ? "" : escapeHtml(text);
}

function joined(texts: string[]): string {
	return texts
		.filter((text) => !isBlank(text))
		.map(escapeHtml)
		.join(", ");
}

function list(items: string[]): string {
	return `<ul>${items.map((item) => `<li>${item}</li>`).join("")}</ul>`;
}

export function siteLink(siteUrl: string, label: string): string {
	if (!isWebAddress(siteUrl)) {
		return "";
	}
	return `<p><a href="${escapeHtml(siteUrl)}">${escapeHtml(label)}</a></p>`;
}

function itemLine(item: { name: string; quantity: number }): string {
	return `${escapeHtml(item.name)} (${formatNumber(item.quantity)})`;
}

export function itemList(heading: string, items: { name: string; quantity: number }[]): string {
	if (items.length === 0) {
		return "";
	}
	return `<p>${heading}</p>${list(items.map(itemLine))}`;
}

const npcStatusLabels: Record<NpcPayload["status"], string> = {
	ALIVE: "Alive",
	DEAD: "Dead",
	MISSING: "Missing",
	UNKNOWN: "",
};

const npcHealthLabels: Record<NpcPayload["health"], string> = {
	DEAD: "Dead",
	HEALTHY: "Healthy",
	INJURED: "Injured",
	SICK: "Sick",
	UNKNOWN: "",
};

const npcRelationLabels: Record<NpcPayload["relation"], string> = {
	ALLY: "Ally",
	ENEMY: "Enemy",
	NEUTRAL: "Neutral",
	SUSPICIOUS: "Suspicious",
	UNKNOWN: "",
};

export function renderNpc(npc: NpcPayload): string {
	return [
		`<h2>${escapeHtml(npc.displayName)}</h2>`,
		facts([
			["Also known as", joined(npc.aliases)],
			["Labels", joined(npc.labels)],
			["Race", escapeHtml(npc.race)],
			["Class", escapeHtml(npc.characterClass)],
			["Level", npc.level === null ? "" : formatNumber(npc.level)],
			["Age", escapeHtml(npc.age)],
			["Status", npcStatusLabels[npc.status]],
			["Health", npcHealthLabels[npc.health]],
			["Relation", npcRelationLabels[npc.relation]],
			["Colony role", textOrBlank(npc.colonyRole)],
			["Now at", textOrBlank(npc.currentPlace)],
			["From", textOrBlank(npc.originPlace)],
		]),
		prose("Appearance", npc.appearance),
		prose("Personality", npc.personality),
		prose("Backstory", npc.backstory),
		prose("Player notes", npc.playerNotes),
		siteLink(npc.siteUrl, `Open ${npc.displayName} on Colony Planner`),
	].join("");
}

export function renderPlace(place: PlacePayload): string {
	return [
		`<h2>${escapeHtml(place.name)}</h2>`,
		isBlank(place.description) ? "" : `<p>${multiline(place.description)}</p>`,
		prose("Player notes", place.playerNotes),
		siteLink(place.siteUrl, `Open ${place.name} on Colony Planner`),
	].join("");
}

const questKindLabels: Record<QuestPayload["kind"], string> = {
	COLONY: "Colony",
	MAINLAND: "Mainland",
};

const questStatusLabels: Record<QuestPayload["status"], string> = {
	ACTIVE: "Active",
	COMPLETED: "Completed",
	FAILED: "Failed",
};

function favorText(quest: QuestPayload): string {
	if (quest.patron === null || quest.rewardFavor === 0) {
		return "";
	}
	const sign = quest.rewardFavor > 0 ? "+" : "";
	return `${sign}${formatNumber(quest.rewardFavor)} with ${escapeHtml(quest.patron)}`;
}

function lootLine(loot: QuestPayload["loot"][number]): string {
	const line = itemLine(loot);
	return isBlank(loot.description) ? line : `${line}: ${multiline(loot.description)}`;
}

export function renderQuest(quest: QuestPayload): string {
	return [
		`<h2>${escapeHtml(quest.title)}</h2>`,
		facts([
			["Kind", questKindLabels[quest.kind]],
			["Status", questStatusLabels[quest.status]],
			["Resolved", quest.resolvedDay === null ? "" : `Day ${formatNumber(quest.resolvedDay)}`],
			["Given by", textOrBlank(quest.giver)],
			["Patron", textOrBlank(quest.patron)],
			["Reward", multiline(quest.reward)],
			["Favor", favorText(quest)],
		]),
		isBlank(quest.description) ? "" : `<p>${multiline(quest.description)}</p>`,
		quest.loot.length === 0 ? "" : `<h3>Loot</h3>${list(quest.loot.map(lootLine))}`,
		siteLink(quest.siteUrl, `Open ${quest.title} on Colony Planner`),
	].join("");
}

function formatInTimeZone(
	instant: string,
	timeZone: string,
	format: Intl.DateTimeFormatOptions,
): string | undefined {
	try {
		return new Date(instant).toLocaleString("en-US", { ...format, timeZone });
	} catch {
		// An IANA name this browser's Intl doesn't know.
		return undefined;
	}
}

const sessionDateFormat: Intl.DateTimeFormatOptions = {
	day: "numeric",
	month: "long",
	weekday: "long",
	year: "numeric",
};
const sessionTimeFormat: Intl.DateTimeFormatOptions = { hour: "numeric", minute: "2-digit" };

/** The game night in the campaign's own time zone, named so players elsewhere can convert it. */
export function renderNextSession(session: NextSession): string {
	const date = formatInTimeZone(session.startsAt, session.timezone, sessionDateFormat);
	const starts = formatInTimeZone(session.startsAt, session.timezone, sessionTimeFormat);
	const ends = formatInTimeZone(session.endsAt, session.timezone, sessionTimeFormat);
	const when =
		date && starts && ends
			? `${date}, ${starts} to ${ends} (${escapeHtml(session.timezone)})`
			: escapeHtml(session.localDate);
	const title = isBlank(session.title) ? "" : `${escapeHtml(session.title)}, `;
	return `<p>Next game night: ${title}${when}</p>`;
}

/** In the GM's own time zone, named, since the payload carries none. */
function formatInstant(instant: string): string {
	return new Date(instant).toLocaleString("en-US", {
		day: "numeric",
		hour: "numeric",
		minute: "2-digit",
		month: "short",
		timeZoneName: "short",
		year: "numeric",
	});
}

export function renderPollPosted(poll: PollPostedPayload): string {
	const pick = poll.allowMultiple ? "Pick any" : "Pick one";
	const vote =
		poll.voting === "DISCORD"
			? "<p>Vote in Discord.</p>"
			: siteLink(poll.pollsUrl, "Vote on Colony Planner") || "<p>Vote on Colony Planner.</p>";
	return [
		`<p><strong>New poll:</strong> ${escapeHtml(poll.question)}</p>`,
		list(poll.options.map(escapeHtml)),
		`<p>${pick}. Closes ${formatInstant(poll.closesAt)}.</p>`,
		vote,
	].join("");
}

function voteCount(count: number): string {
	return `${formatNumber(count)} ${count === 1 ? "vote" : "votes"}`;
}

export function renderPollClosed(poll: PollClosedPayload): string {
	return [
		`<p><strong>Poll closed:</strong> ${escapeHtml(poll.question)}</p>`,
		list(poll.options.map((option) => `${escapeHtml(option.label)}: ${voteCount(option.count)}`)),
	].join("");
}

export function renderDeliveryLine(delivery: DeliveryPayload): string {
	return `<p>Goods unloaded on Day ${formatNumber(delivery.paidDay)}, paid ${formatNumber(delivery.totalGold)} gold: ${escapeHtml(delivery.summary)}</p>`;
}

export function renderDelivery(delivery: DeliveryPayload): string {
	return renderDeliveryLine(delivery) + itemList("Hand these out yourself:", delivery.items);
}

export function renderDeliveryUndone(undone: DeliveryUndonePayload): string {
	return [
		`<p><strong>A payment was undone on Colony Planner:</strong> ${escapeHtml(undone.summary)}</p>`,
		itemList("If any of these reached the party, remove them by hand:", undone.items),
	].join("");
}
