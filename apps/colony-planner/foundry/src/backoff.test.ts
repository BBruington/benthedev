import { expect, test } from "bun:test";
import { backoffDelay } from "./backoff";

test("backoff doubles from 1 second and stops at 60", () => {
	expect([0, 1, 2, 3, 4, 5, 6, 7, 20].map(backoffDelay)).toEqual([
		1000, 2000, 4000, 8000, 16_000, 32_000, 60_000, 60_000, 60_000,
	]);
});
