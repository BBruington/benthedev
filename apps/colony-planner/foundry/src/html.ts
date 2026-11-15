const htmlEscapes: Record<string, string> = {
	"'": "&#39;",
	'"': "&quot;",
	"&": "&amp;",
	"<": "&lt;",
	">": "&gt;",
};

const htmlSpecialCharacters = /["&'<>]/g;

/** Escapes text for HTML element content and quoted attribute values. */
export function escapeHtml(text: string): string {
	return text.replace(htmlSpecialCharacters, (character) => htmlEscapes[character] ?? character);
}

/** The part of a parsed node the conversion reads, which the browser's DOM and happy-dom's share. */
export interface ParsedNode {
	readonly childNodes: ArrayLike<ParsedNode>;
	readonly nodeName: string;
	readonly nodeType: number;
	readonly nodeValue: string | null;
	getAttribute?(name: string): string | null;
}

export interface HtmlParser {
	parseFromString(source: string, type: "text/html"): { readonly body: ParsedNode | null };
}

/** Whether a field that becomes player-visible leaves out GM secret blocks, unlabelled enrichers and inline rolls. */
export type SecretBlocks = "keep" | "omit";

export interface PlainText {
	/** The secret blocks' text, unlabelled enrichers and inline rolls, when they were left out; "" otherwise. */
	secrets: string;
	text: string;
}

const elementNode = 1;
const textNode = 3;
const lineEndingElements = new Set([
	"BLOCKQUOTE",
	"DIV",
	"H1",
	"H2",
	"H3",
	"H4",
	"H5",
	"H6",
	"LI",
	"P",
	"SECTION",
	"TR",
]);
const tableCells = new Set(["TD", "TH"]);
const unreadElements = new Set(["SCRIPT", "STYLE", "TEMPLATE"]);
// A target may hold one level of brackets, as pf2e's `@Damage[(2d6+4)[fire]]{Fire}` and an inline roll's
// `[[/r 2d6[fire]]]` do. Either may carry a `{Label}`, which is what Foundry shows players.
const enricherLink =
	/(?:@[A-Za-z]+\[(?:[^[\]]|\[[^\]]*\])*\]|\[\[(?:[^[\]]|\[[^\]]*\])*\]\])\{([^}]*)\}/g;
// One with no label shows its target, which may name a hidden DC (`@Check[fortitude|dc:25|showDC:gm]`) or
// a GM's roll (`[[/gmr 1d20+12]]`).
const unlabelledEnricher =
	/(?:@[A-Za-z]+\[(?:[^[\]]|\[[^\]]*\])*\]|\[\[(?:[^[\]]|\[[^\]]*\])*\]\])(?!\{)/g;
// Every control character but the line break, and the no-break space `&nbsp;` decodes to.
const controlCharacters = /[^\P{Cc}\n]|\u00a0/gu;
const whitespace = /\s+/;
const htmlWhitespace = /[\t\n\f\r ]+/g;
const lineBreaks = /\r\n|\r/g;
const spaceRuns = / {2,}/g;
const spacesAroundLineBreaks = / *\n */g;
const blankLineRuns = /\n{3,}/g;
const lineBreakRuns = /\n+/g;
const ellipsis = "…";

// Foundry's `section.secret`, and pf2e's GM and owner-only blocks, which only enrichHTML hides.
function isSecretBlock(node: ParsedNode): boolean {
	const classes = node.getAttribute?.("class") ?? "";
	const visibility = node.getAttribute?.("data-visibility");
	return (
		(node.nodeName === "SECTION" && classes.split(whitespace).includes("secret")) ||
		visibility === "gm" ||
		visibility === "owner"
	);
}

/** Plain text as it is written, line by line. */
interface TextBuilder {
	atLineStart: boolean;
	parts: string[];
}

function newBuilder(): TextBuilder {
	return { atLineStart: true, parts: [] };
}

/** Source whitespace is one space, as a browser shows it, and none at the start of a line. */
function pushText(builder: TextBuilder, source: string): void {
	const spaced = source.replace(htmlWhitespace, " ");
	const text = builder.atLineStart ? spaced.trimStart() : spaced;
	if (text === "") {
		return;
	}
	builder.parts.push(text);
	builder.atLineStart = false;
}

/** A block's edge ends the line it is on; nested edges together end one line. */
function endLine(builder: TextBuilder): void {
	if (!builder.atLineStart) {
		builder.parts.push("\n");
		builder.atLineStart = true;
	}
}

function lineBreak(builder: TextBuilder): void {
	builder.parts.push("\n");
	builder.atLineStart = true;
}

/** Walks the tree in order; a secret block left out goes to `secrets` instead of `text`. */
function collectText(
	node: ParsedNode,
	secretBlocks: SecretBlocks,
	text: TextBuilder,
	secrets: TextBuilder,
): void {
	if (node.nodeType === textNode) {
		pushText(text, node.nodeValue ?? "");
		return;
	}
	if (node.nodeType !== elementNode || unreadElements.has(node.nodeName)) {
		return;
	}
	if (node.nodeName === "BR") {
		lineBreak(text);
		return;
	}
	const secretBlock = secretBlocks === "omit" && isSecretBlock(node);
	const target = secretBlock ? secrets : text;
	const endsLine = secretBlock || lineEndingElements.has(node.nodeName);
	if (endsLine) {
		endLine(target);
	}
	if (tableCells.has(node.nodeName)) {
		pushText(target, " ");
	}
	for (const child of Array.from(node.childNodes)) {
		collectText(child, secretBlock ? "keep" : secretBlocks, target, secrets);
	}
	if (endsLine) {
		endLine(target);
	}
}

/**
 * Spaces for tabs and other control characters but line breaks, one space for a run, at most one
 * blank line in a row, enricher links as their labels, trimmed: what the site's cleanMultilineText
 * accepts.
 */
export function normalizeProse(text: string): string {
	return text
		.replace(lineBreaks, "\n")
		.replace(enricherLink, "$1")
		.replace(controlCharacters, " ")
		.replace(spaceRuns, " ")
		.replace(spacesAroundLineBreaks, "\n")
		.replace(blankLineRuns, "\n\n")
		.trim();
}

/** One line of plain text, for a name: what the site's cleanText accepts. */
export function cleanLine(text: string, limit: number): string {
	return clipText(normalizeProse(text).replace(lineBreakRuns, " "), limit);
}

/** At most `limit` code points, as the site counts them, ending in "…" when clipped. */
export function clipText(text: string, limit: number): string {
	const codePoints = Array.from(text);
	if (codePoints.length <= limit) {
		return text;
	}
	return `${codePoints
		.slice(0, limit - 1)
		.join("")
		.trimEnd()}${ellipsis}`;
}

/**
 * Foundry's rich text as plain text (design §14.11). The parser's `text/html` mode runs no scripts and
 * loads no images. v13 has no helper that strips markup: TextEditor.decodeHTML decodes entities only.
 */
export function htmlToPlainText(
	html: string,
	secretBlocks: SecretBlocks,
	parser: HtmlParser,
): PlainText {
	const body = parser.parseFromString(html, "text/html").body;
	const text = newBuilder();
	const secrets = newBuilder();
	if (body) {
		collectText(body, secretBlocks, text, secrets);
	}
	const shown = text.parts.join("");
	if (secretBlocks === "keep") {
		return { secrets: normalizeProse(secrets.parts.join("")), text: normalizeProse(shown) };
	}
	for (const enricher of shown.match(unlabelledEnricher) ?? []) {
		endLine(secrets);
		pushText(secrets, enricher);
	}
	return {
		secrets: normalizeProse(secrets.parts.join("")),
		text: normalizeProse(shown.replace(unlabelledEnricher, "")),
	};
}
