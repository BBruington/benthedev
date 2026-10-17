import { describe, expect, test } from "bun:test";
import { forgetFailedSaves } from "./forms";

function fakeSave(isError: boolean) {
	const save = {
		isError,
		reset: () => {
			save.resets += 1;
		},
		resets: 0,
	};
	return save;
}

describe("editing after a failed save", () => {
	test("forgets only the saves that failed", () => {
		const failed = fakeSave(true);
		const untouched = fakeSave(false);
		forgetFailedSaves(failed, untouched);
		expect(failed.resets).toBe(1);
		expect(untouched.resets).toBe(0);
	});
});
