export interface ServerSentEvent {
	data: string;
	type: string;
}

const lineBreak = /\r\n|\r|\n/;

/**
 * An incremental parser for the event stream format (the WHATWG HTML spec's "event stream
 * interpretation"): feed it text chunks as they arrive, split anywhere, and it calls `onEvent` for each
 * complete event. Comments (the server's `: ping`) and `id` and `retry` fields are ignored, since the
 * bridge reconnects on its own schedule and resumes from its outbox cursor, not a stream ID.
 */
export function createSseParser(onEvent: (event: ServerSentEvent) => void): {
	push(chunk: string): void;
} {
	let buffer = "";
	let skipLeadingLineFeed = false;
	let eventType = "";
	let dataLines: string[] = [];

	function dispatch() {
		if (dataLines.length > 0) {
			onEvent({ data: dataLines.join("\n"), type: eventType || "message" });
		}
		eventType = "";
		dataLines = [];
	}

	function processLine(line: string) {
		if (line === "") {
			dispatch();
			return;
		}
		if (line.startsWith(":")) {
			return;
		}
		const colon = line.indexOf(":");
		const field = colon === -1 ? line : line.slice(0, colon);
		let value = colon === -1 ? "" : line.slice(colon + 1);
		if (value.startsWith(" ")) {
			value = value.slice(1);
		}
		if (field === "event") {
			eventType = value;
		} else if (field === "data") {
			dataLines.push(value);
		}
	}

	return {
		push(chunk) {
			if (chunk === "") {
				return;
			}
			// A "\r" ends its line at once; when it was half of a "\r\n" split across chunks, the "\n"
			// opening the next chunk belongs to that same line break.
			const text = skipLeadingLineFeed && chunk.startsWith("\n") ? chunk.slice(1) : chunk;
			skipLeadingLineFeed = chunk.endsWith("\r");
			const lines = (buffer + text).split(lineBreak);
			buffer = lines.pop() ?? "";
			for (const line of lines) {
				processLine(line);
			}
		},
	};
}
