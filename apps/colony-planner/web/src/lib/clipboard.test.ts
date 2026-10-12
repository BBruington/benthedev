import { expect, test } from "bun:test";
import { copyToClipboard } from "./clipboard";

test("refuses, rather than throwing, where the browser has no clipboard", async () => {
	expect("clipboard" in navigator).toBe(false);
	await expect(copyToClipboard("text")).rejects.toThrow("can't use the clipboard");
});
