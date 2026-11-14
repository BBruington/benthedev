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
