import { describe, expect, test } from "bun:test";
import { Window } from "happy-dom";
import { cleanLine, clipText, escapeHtml, type HtmlParser, htmlToPlainText } from "./html";

test("escapeHtml escapes every character that could open markup or end an attribute", () => {
	expect(escapeHtml(`<img src=x onerror="alert('hi')"> & more`)).toBe(
		"&lt;img src=x onerror=&quot;alert(&#39;hi&#39;)&quot;&gt; &amp; more",
	);
	expect(escapeHtml("Port Eldari")).toBe("Port Eldari");
});

const parser: HtmlParser = new new Window().DOMParser();

describe("htmlToPlainText", () => {
	test("ends a line at each block element and br, and decodes entities", () => {
		const html =
			"<h2>Brother&nbsp;Aldo</h2><p>Tall &amp; <em>quiet</em>.<br>Smells of incense.</p><ul><li>Robes</li><li>Staff</li></ul><blockquote>Peace.</blockquote>";
		expect(htmlToPlainText(html, "keep", parser).text).toBe(
			"Brother Aldo\nTall & quiet.\nSmells of incense.\nRobes\nStaff\nPeace.",
		);
	});

	test("keeps at most one blank line, one space in a run, and no tabs or other control characters", () => {
		const html = "<p>One\t\ttwo   three\u0007</p><p></p><p></p><div>\n\n\n</div><p>  Four  </p>";
		expect(htmlToPlainText(html, "keep", parser).text).toBe("One two three\nFour");
		expect(htmlToPlainText("<p>A</p>\n\n\n\n<p>B</p>", "keep", parser).text).toBe("A\nB");
		expect(htmlToPlainText("A<br><br><br><br>B<br>", "keep", parser).text).toBe("A\n\nB");
	});

	test("turns enricher links into their labels and leaves other enrichers as written", () => {
		const html =
			"<p>Carries @UUID[Compendium.pf2e.equipment-srd.Item.abcdefghij123456]{a healing potion}, deals @Damage[(2d6+4)[fire]]{Fire} and @Check[fortitude|dc:20]. [[/r 1d6]]</p>";
		expect(htmlToPlainText(html, "keep", parser).text).toBe(
			"Carries a healing potion, deals Fire and @Check[fortitude|dc:20]. [[/r 1d6]]",
		);
	});

	test("leaves out an unlabelled enricher where players read, and hands it back with the secrets", () => {
		const html =
			'<p>Resists @Check[fortitude|dc:25|showDC:gm] and @Damage[(2d6+4)[fire]]{Fire}.</p><section class="secret"><p>A cultist.</p></section>';
		expect(htmlToPlainText(html, "omit", parser)).toEqual({
			secrets: "A cultist.\n@Check[fortitude|dc:25|showDC:gm]",
			text: "Resists and Fire.",
		});
		expect(htmlToPlainText("<p>@Check[will|dc:18]</p>", "omit", parser)).toEqual({
			secrets: "@Check[will|dc:18]",
			text: "",
		});
	});

	test("leaves out an unlabelled inline roll where players read, and keeps a labelled one's label", () => {
		const html =
			"<p>Strikes [[/r 1d20+12]] for [[/gmr 2d6[fire]]] and heals [[/r 1d4]]{a little}.</p>";
		expect(htmlToPlainText(html, "omit", parser)).toEqual({
			secrets: "[[/r 1d20+12]]\n[[/gmr 2d6[fire]]]",
			text: "Strikes for and heals a little.",
		});
		expect(htmlToPlainText(html, "keep", parser).text).toBe(
			"Strikes [[/r 1d20+12]] for [[/gmr 2d6[fire]]] and heals a little.",
		);
	});

	test("leaves out GM secret blocks when asked, and hands their text back", () => {
		const html =
			'<p>A kindly monk.</p><section class="secret" id="secret-1"><p>Secretly a cultist.</p><p>Owes the Duke.</p></section><p>Bald.</p>';
		expect(htmlToPlainText(html, "omit", parser)).toEqual({
			secrets: "Secretly a cultist.\nOwes the Duke.",
			text: "A kindly monk.\nBald.",
		});
		expect(htmlToPlainText(html, "keep", parser)).toEqual({
			secrets: "",
			text: "A kindly monk.\nSecretly a cultist.\nOwes the Duke.\nBald.",
		});
	});

	test("a secret block's own text keeps its inline markup on one line", () => {
		const html =
			'<section class="secret"><p>The <strong>true</strong> name is <a>Bob</a>.</p></section>';
		expect(htmlToPlainText(html, "omit", parser).secrets).toBe("The true name is Bob.");
	});

	test("pf2e's GM and owner-only blocks are secret blocks too", () => {
		const html =
			'<p>Rope.</p><p data-visibility="gm">GM only.</p><div data-visibility="owner">Owner only.</div><p data-visibility="all">Everyone.</p>';
		expect(htmlToPlainText(html, "omit", parser)).toEqual({
			secrets: "GM only.\nOwner only.",
			text: "Rope.\nEveryone.",
		});
	});

	test("table cells are separated, and rows end lines", () => {
		const html =
			"<table><tr><th>Level</th><th>Damage</th></tr><tr><td>1</td><td>2d6</td></tr></table>";
		expect(htmlToPlainText(html, "keep", parser).text).toBe("Level Damage\n1 2d6");
	});

	test("only a section with the secret class is a secret block", () => {
		const html =
			'<section class="journal secret-ish"><p>Shown.</p></section><div class="secret">Also shown.</div>';
		expect(htmlToPlainText(html, "omit", parser)).toEqual({
			secrets: "",
			text: "Shown.\nAlso shown.",
		});
	});

	test("never reads scripts, styles or templates", () => {
		const html =
			"<p>Safe</p><script>alert('x')</script><style>p{color:red}</style><template><p>Hidden</p></template><img src=x onerror=alert(1)><!-- note -->";
		expect(htmlToPlainText(html, "keep", parser).text).toBe("Safe");
	});

	test("plain text with no markup passes through, and empty input is empty", () => {
		expect(htmlToPlainText("Just words", "omit", parser)).toEqual({
			secrets: "",
			text: "Just words",
		});
		expect(htmlToPlainText("", "omit", parser)).toEqual({ secrets: "", text: "" });
	});
});

describe("clipText and cleanLine", () => {
	test("clipText counts code points and ends a clipped text in an ellipsis within the limit", () => {
		expect(clipText("abcdef", 6)).toBe("abcdef");
		expect(clipText("abcdefg", 6)).toBe("abcde…");
		expect(clipText("🐉🐉🐉", 3)).toBe("🐉🐉🐉");
		expect(clipText("🐉🐉🐉🐉", 3)).toBe("🐉🐉…");
		expect(clipText("abc   defg", 6)).toBe("abc…");
	});

	test("cleanLine makes one line of a name", () => {
		expect(cleanLine("  Brother\n\nAldo\tthe\r\nWise  ", 80)).toBe("Brother Aldo the Wise");
		expect(cleanLine("x".repeat(90), 80)).toHaveLength(80);
	});
});
