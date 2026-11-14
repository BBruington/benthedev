import { expect, test } from "bun:test";
import { colonyOrigin } from "./colony-origin";

const installed = (manifest: string) => ({ manifest, version: "1.0.0" });

test("Colony Planner's origin comes from the installed manifest URL", () => {
	expect(colonyOrigin(installed("https://colony.benthedev.com/foundry/module.json"))).toBe(
		"https://colony.benthedev.com",
	);
	expect(colonyOrigin(installed("http://localhost:8080/foundry/module.json"))).toBe(
		"http://localhost:8080",
	);
});

test("no address without a usable manifest, and never plain http to another host", () => {
	expect(colonyOrigin(undefined)).toBeUndefined();
	expect(colonyOrigin(installed(""))).toBeUndefined();
	expect(colonyOrigin(installed("not a url"))).toBeUndefined();
	expect(
		colonyOrigin(installed("http://colony.benthedev.com/foundry/module.json")),
	).toBeUndefined();
	expect(colonyOrigin(installed("javascript:alert(1)"))).toBeUndefined();
});
