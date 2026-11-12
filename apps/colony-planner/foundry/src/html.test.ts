import { expect, test } from "bun:test";
import { escapeHtml } from "./html";

test("escapeHtml escapes every character that could open markup or end an attribute", () => {
	expect(escapeHtml(`<img src=x onerror="alert('hi')"> & more`)).toBe(
		"&lt;img src=x onerror=&quot;alert(&#39;hi&#39;)&quot;&gt; &amp; more",
	);
	expect(escapeHtml("Port Eldari")).toBe("Port Eldari");
});
