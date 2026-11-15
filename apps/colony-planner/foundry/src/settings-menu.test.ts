import { expect, test } from "bun:test";
import type { Bridge, BridgeCondition } from "./bridge";
import { forgettableDeposits } from "./settings-menu";

function bridgeIn(condition: BridgeCondition): Pick<Bridge, "status"> {
	return {
		status: () => ({
			campaignName: "Port Eldari",
			condition,
			connection: undefined,
			lastProblem: undefined,
			linked: condition !== "not-linked",
		}),
	};
}

test("Forget is offered only where the bridge runs, holding the link's token", () => {
	const deposits = { forget: () => Promise.resolve(), interrupted: () => [] };
	const offered = (condition: BridgeCondition | undefined) =>
		forgettableDeposits({
			bridge: () => (condition ? bridgeIn(condition) : undefined),
			deposits: () => deposits,
		});
	expect(offered("running")).toBe(deposits);
	for (const condition of [
		"another-gm-is-active",
		"not-linked",
		"needs-new-code",
		"turned-off",
		"no-colony-address",
		undefined,
	] as const) {
		expect(offered(condition)).toBeUndefined();
	}
});
