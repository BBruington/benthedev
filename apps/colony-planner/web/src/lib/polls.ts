import type { Schemas } from "@/api/client";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/time";
import { countOf } from "@/lib/words";

type Poll = Schemas["Poll"];
type PollOption = Schemas["PollOption"];

// Discord's poll limits, which every poll takes so a poll could always be posted there.
export const pollFewestOptions = 2;
export const pollMostOptions = 10;

/** Discord's own duration presets, in hours. */
export const pollDurationChoices = [
	{ hours: 1, label: "1 hour" },
	{ hours: 4, label: "4 hours" },
	{ hours: 8, label: "8 hours" },
	{ hours: 24, label: "1 day" },
	{ hours: 72, label: "3 days" },
	{ hours: 168, label: "1 week" },
	{ hours: 336, label: "2 weeks" },
] as const;

export const defaultPollDurationHours = 24;

/** The server derives closed from closes_at too, but a page left open past the deadline still says open. */
export function isPollOpen(poll: Poll, now: Date = new Date()): boolean {
	return !poll.closed && Date.parse(poll.closesAt) > now.getTime();
}

export function pickText(poll: Poll): string {
	return poll.allowMultiple ? "Pick any" : "Pick one";
}

/** "Closes Nov 4, 2026, 7:30 PM (in 2 days)" or "Closed Nov 4, 2026, 7:30 PM". */
export function closeTimeText(poll: Poll, now: Date = new Date()): string {
	if (!isPollOpen(poll, now)) {
		return `Closed ${formatAbsoluteTime(poll.closedAt ?? poll.closesAt)}`;
	}
	return `Closes ${formatAbsoluteTime(poll.closesAt)} (${formatRelativeTime(poll.closesAt, now)})`;
}

/**
 * An option's voters: the players by name, then Discord voters who aren't players as a count
 * ("Ana, Bo and 2 others", "1 other"). Null when nobody picked it.
 */
export function votersText(option: PollOption): string | null {
	const names = option.voters.map((voter) => voter.name);
	if (option.otherVotes === 0) {
		return names.length === 0 ? null : names.join(", ");
	}
	const others = countOf(option.otherVotes, "other");
	return names.length === 0 ? others : `${names.join(", ")} and ${others}`;
}

/** "No votes yet", "3 votes, Riverside leads", "4 votes, tied". */
export function voteSummary(poll: Poll): string {
	if (poll.voterCount === 0) {
		return "No votes yet";
	}
	const highest = Math.max(...poll.options.map((option) => option.count));
	const leaders = poll.options.filter((option) => option.count === highest);
	const votes = countOf(poll.voterCount, "vote");
	if (leaders.length === 1 && leaders[0]) {
		return `${votes}, ${leaders[0].label} leads`;
	}
	return `${votes}, tied`;
}

/** A bar's share of the poll: of the people who voted, so several-answer bars can each be full. */
export function optionShare(count: number, voterCount: number): number {
	if (voterCount === 0) {
		return 0;
	}
	return Math.min(1, count / voterCount);
}

export function sameChoice(first: readonly string[], second: readonly string[]): boolean {
	return first.length === second.length && first.every((optionId) => second.includes(optionId));
}

/** The draft's problem with its options before the server sees them, matching core's messages. */
export function optionsProblem(options: readonly string[]): string | undefined {
	const cleaned = options.map((option) => option.trim().toLowerCase());
	if (cleaned.some((option) => option === "")) {
		return undefined;
	}
	if (new Set(cleaned).size !== cleaned.length) {
		return "Each option must be different.";
	}
	return undefined;
}
