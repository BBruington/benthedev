import { expect, test } from "bun:test";
import { isActiveGm } from "./active-gm";

const gm = { id: "gm-1", isGM: true };
const secondGm = { id: "gm-2", isGM: true };
const player = { id: "player-1", isGM: false };

test("only the active GM's client acts", () => {
	expect(isActiveGm({ user: gm, users: { activeGM: gm } })).toBe(true);
	expect(isActiveGm({ user: secondGm, users: { activeGM: gm } })).toBe(false);
	expect(isActiveGm({ user: player, users: { activeGM: gm } })).toBe(false);
	expect(isActiveGm({ user: gm, users: { activeGM: null } })).toBe(false);
});
