import { describe, expect, test } from "bun:test";
import { createSseParser, type ServerSentEvent } from "./sse";

function parse(chunks: string[]): ServerSentEvent[] {
	const events: ServerSentEvent[] = [];
	const parser = createSseParser((event) => events.push(event));
	for (const chunk of chunks) {
		parser.push(chunk);
	}
	return events;
}

describe("createSseParser", () => {
	test("reads the bridge's outbox event", () => {
		expect(parse(['event: outbox\ndata: {"seq": 12}\n\n'])).toEqual([
			{ data: '{"seq": 12}', type: "outbox" },
		]);
	});

	test("ignores ping comments and events without data", () => {
		expect(parse([": ping\n\n", "event: outbox\n\n", ": ping\n\n"])).toEqual([]);
	});

	test("joins chunks split anywhere, CRLF included", () => {
		const stream = 'event: outbox\r\ndata: {"seq": 3}\r\n\r\n';
		const oneCharacterAtATime = [...stream];
		expect(parse(oneCharacterAtATime)).toEqual([{ data: '{"seq": 3}', type: "outbox" }]);
	});

	test("accepts bare CR line endings", () => {
		expect(parse(["event: outbox\rdata: 1\r\r"])).toEqual([{ data: "1", type: "outbox" }]);
	});

	test("joins several data lines, defaults the type, and drops an unfinished event", () => {
		expect(parse(["data: first\ndata:second\n\n", "data: unfinished\n"])).toEqual([
			{ data: "first\nsecond", type: "message" },
		]);
	});

	test("keeps each event's type to itself", () => {
		expect(parse(["event: outbox\ndata: 1\n\ndata: 2\n\n"])).toEqual([
			{ data: "1", type: "outbox" },
			{ data: "2", type: "message" },
		]);
	});
});
