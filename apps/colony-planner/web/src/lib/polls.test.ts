import { describe, expect, test } from "bun:test";
import type { Schemas } from "@/api/client";
import {
	closeTimeText,
	isPollOpen,
	optionShare,
	optionsProblem,
	sameChoice,
	votersText,
	voteSummary,
} from "./polls";

type Poll = Schemas["Poll"];
type PollOption = Schemas["PollOption"];

function option(overrides: Partial<PollOption> = {}): PollOption {
	return {
		count: 0,
		id: "a",
		label: "Riverside",
		otherVotes: 0,
		position: 0,
		voters: [],
		...overrides,
	};
}

function poll(overrides: Partial<Poll> = {}): Poll {
	return {
		allowMultiple: false,
		closed: false,
		closedAt: null,
		closesAt: "2026-11-05T12:00:00Z",
		createdAt: "2026-11-01T12:00:00Z",
		discordUrl: null,
		id: "poll-1",
		myOptionIds: [],
		options: [option(), option({ id: "b", label: "Hilltop", position: 1 })],
		question: "Where do we build the mill?",
		voterCount: 0,
		voting: "SITE",
		...overrides,
	};
}

describe("isPollOpen", () => {
	const now = new Date("2026-11-04T12:00:00Z");
	test("a future deadline is open", () => {
		expect(isPollOpen(poll(), now)).toBe(true);
	});
	test("a passed deadline is closed before anything records it", () => {
		expect(isPollOpen(poll({ closesAt: "2026-11-04T11:59:00Z" }), now)).toBe(false);
	});
	test("the deadline itself is closed", () => {
		expect(isPollOpen(poll({ closesAt: "2026-11-04T12:00:00Z" }), now)).toBe(false);
	});
	test("a recorded close wins", () => {
		expect(isPollOpen(poll({ closed: true }), now)).toBe(false);
	});
});

describe("closeTimeText", () => {
	const now = new Date("2026-11-04T12:00:00Z");
	test("an open poll says when it closes", () => {
		expect(closeTimeText(poll(), now)).toStartWith("Closes ");
	});
	test("an early close shows when the DM closed it, not the deadline", () => {
		const closedEarly = poll({ closed: true, closedAt: "2026-11-03T09:15:00Z" });
		expect(closeTimeText(closedEarly, now)).toBe(
			closeTimeText(poll({ closed: true, closesAt: "2026-11-03T09:15:00Z" }), now),
		);
	});
	test("a passed deadline nobody recorded yet shows the deadline", () => {
		expect(closeTimeText(poll({ closesAt: "2026-11-04T11:00:00Z" }), now)).toStartWith("Closed ");
	});
});

describe("votersText", () => {
	const ana = { name: "Ana" };
	const bo = { name: "Bo" };
	test("nobody picked it", () => {
		expect(votersText(option())).toBeNull();
	});
	test("players only", () => {
		expect(votersText(option({ voters: [ana, bo] }))).toBe("Ana, Bo");
	});
	test("one other and no players", () => {
		expect(votersText(option({ otherVotes: 1 }))).toBe("1 other");
	});
	test("two others and no players", () => {
		expect(votersText(option({ otherVotes: 2 }))).toBe("2 others");
	});
	test("a player and one other", () => {
		expect(votersText(option({ otherVotes: 1, voters: [ana] }))).toBe("Ana and 1 other");
	});
	test("players and two others", () => {
		expect(votersText(option({ otherVotes: 2, voters: [ana, bo] }))).toBe("Ana, Bo and 2 others");
	});
});

describe("voteSummary", () => {
	test("no votes", () => {
		expect(voteSummary(poll())).toBe("No votes yet");
	});
	test("one leader", () => {
		const leading = poll({
			options: [option({ count: 2 }), option({ count: 1, id: "b", label: "Hilltop", position: 1 })],
			voterCount: 3,
		});
		expect(voteSummary(leading)).toBe("3 votes, Riverside leads");
	});
	test("a tie", () => {
		const tied = poll({
			options: [option({ count: 1 }), option({ count: 1, id: "b", label: "Hilltop", position: 1 })],
			voterCount: 2,
		});
		expect(voteSummary(tied)).toBe("2 votes, tied");
	});
});

describe("optionShare", () => {
	test("no voters is an empty bar", () => {
		expect(optionShare(0, 0)).toBe(0);
	});
	test("a share of voters", () => {
		expect(optionShare(1, 4)).toBe(0.25);
	});
});

describe("sameChoice", () => {
	test("order doesn't matter", () => {
		expect(sameChoice(["a", "b"], ["b", "a"])).toBe(true);
		expect(sameChoice(["a"], ["a", "b"])).toBe(false);
	});
});

describe("optionsProblem", () => {
	test("duplicates ignoring case and spaces", () => {
		expect(optionsProblem(["Mill", " mill "])).toBe("Each option must be different.");
	});
	test("blanks wait for the user to finish", () => {
		expect(optionsProblem(["Mill", ""])).toBeUndefined();
	});
	test("distinct options pass", () => {
		expect(optionsProblem(["Mill", "Forge"])).toBeUndefined();
	});
});
