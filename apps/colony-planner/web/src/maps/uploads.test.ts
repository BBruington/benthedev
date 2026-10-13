import { describe, expect, test } from "bun:test";
import {
	fileSize,
	imageFileProblem,
	maxImageBytes,
	megabytes,
	uploadFailureMessage,
} from "./uploads";

describe("choosing an image to upload", () => {
	test("accepts PNG, JPEG, WebP and GIF up to 5 MB", () => {
		for (const type of ["image/png", "image/jpeg", "image/webp", "image/gif"]) {
			expect(imageFileProblem({ name: "eldari", size: maxImageBytes, type })).toBeNull();
		}
	});

	test("names the file and the types the site takes", () => {
		expect(imageFileProblem({ name: "crest.svg", size: 2048, type: "image/svg+xml" })).toBe(
			"crest.svg isn't an image the site can use. Choose a PNG, JPEG, WebP or GIF image.",
		);
		expect(imageFileProblem({ name: "notes", size: 2048, type: "" })).toContain("PNG, JPEG");
	});

	test("says how big a file is when it's over the limit", () => {
		expect(imageFileProblem({ name: "huge.png", size: 6.3 * 1024 * 1024, type: "image/png" })).toBe(
			"huge.png is 6.3 MB. Images can be at most 5 MB, so shrink or crop it first.",
		);
	});

	test("refuses an empty file", () => {
		expect(imageFileProblem({ name: "blank.png", size: 0, type: "image/png" })).toBe(
			"blank.png is empty. Choose another image.",
		);
	});

	test("small files read in KB, larger ones in MB", () => {
		expect(fileSize(300)).toBe("1 KB");
		expect(fileSize(21_504)).toBe("21 KB");
		expect(fileSize(721_815)).toBe("705 KB");
		expect(fileSize(2.5 * 1024 * 1024)).toBe("2.5 MB");
	});

	test("megabytes rounds to one decimal and drops a trailing .0", () => {
		expect(megabytes(5 * 1024 * 1024)).toBe("5");
		expect(megabytes(1.25 * 1024 * 1024)).toBe("1.3");
	});
});

describe("an upload the server refused", () => {
	test("shows the server's message for the file", () => {
		const problem = {
			detail: "validation failed",
			errors: [{ location: "body.image", message: "upload a PNG, JPEG, WebP or GIF image." }],
			status: 422,
		};
		expect(uploadFailureMessage(problem)).toBe("Upload a PNG, JPEG, WebP or GIF image.");
	});

	test("explains a body the server or a proxy found too large", () => {
		expect(uploadFailureMessage({ detail: "Something went wrong. Try again.", status: 413 })).toBe(
			"Images can be at most 5 MB, so shrink or crop it first.",
		);
	});

	test("passes on the campaign's image budget", () => {
		const detail =
			"This campaign's images use 48.2 MB of 50 MB. Replace or remove a map or portrait first.";
		expect(uploadFailureMessage({ detail, status: 409 })).toBe(detail);
	});

	test("falls back to a plain sentence", () => {
		expect(uploadFailureMessage(new Error("boom"))).toBe(
			"The image couldn't be uploaded. Try again.",
		);
	});
});
