/**
 * Copies text, failing like a refused copy where the browser has no clipboard: it offers one only on
 * secure origins, so not on the plain-http LAN address `mise run dev:phone` serves.
 */
export function copyToClipboard(text: string): Promise<void> {
	if (!("clipboard" in navigator)) {
		return Promise.reject(new Error("This page can't use the clipboard."));
	}
	return navigator.clipboard.writeText(text);
}
