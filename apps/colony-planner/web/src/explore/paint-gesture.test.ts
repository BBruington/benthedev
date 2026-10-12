import { describe, expect, test } from "bun:test";
import {
	type GestureEffect,
	type GestureEvent,
	type GestureState,
	idleGesture,
	paintSlopInPixels,
	stepGesture,
} from "./paint-gesture";

function run(events: readonly GestureEvent[]): { state: GestureState; effects: GestureEffect[] } {
	let state = idleGesture;
	const effects: GestureEffect[] = [];
	for (const event of events) {
		const step = stepGesture(state, event);
		state = step.state;
		effects.push(...step.effects);
	}
	return { effects, state };
}

const touchDown = (pointerId: number, x: number, y: number): GestureEvent => ({
	point: { x, y },
	pointerId,
	pointerKind: "touch",
	type: "down",
});
const move = (pointerId: number, x: number, y: number): GestureEvent => ({
	point: { x, y },
	pointerId,
	type: "move",
});
const up = (pointerId: number, x: number, y: number): GestureEvent => ({
	point: { x, y },
	pointerId,
	type: "up",
});

describe("one finger paints", () => {
	test("a tap paints the hex under it and ends the stroke", () => {
		expect(run([touchDown(1, 10, 10), up(1, 11, 10)])).toEqual({
			effects: [
				{ from: { x: 10, y: 10 }, to: { x: 10, y: 10 }, type: "paint" },
				{ type: "endStroke" },
			],
			state: idleGesture,
		});
	});

	test("a drag paints from where it started, segment by segment", () => {
		const { effects, state } = run([
			touchDown(1, 0, 0),
			move(1, 3, 0),
			move(1, 20, 0),
			move(1, 40, 0),
			up(1, 40, 0),
		]);
		expect(effects).toEqual([
			{ from: { x: 0, y: 0 }, to: { x: 20, y: 0 }, type: "paint" },
			{ from: { x: 20, y: 0 }, to: { x: 40, y: 0 }, type: "paint" },
			{ from: { x: 40, y: 0 }, to: { x: 40, y: 0 }, type: "paint" },
			{ type: "endStroke" },
		]);
		expect(state).toEqual(idleGesture);
	});

	test("small jitter under the slop doesn't paint yet", () => {
		const { effects } = run([touchDown(1, 0, 0), move(1, paintSlopInPixels - 1, 0)]);
		expect(effects).toEqual([]);
	});

	test("a mouse paints at once, with no slop", () => {
		const { effects, state } = run([
			{ point: { x: 5, y: 5 }, pointerId: 1, pointerKind: "mouse", type: "down" },
		]);
		expect(effects).toEqual([{ from: { x: 5, y: 5 }, to: { x: 5, y: 5 }, type: "paint" }]);
		expect(state.kind).toBe("painting");
	});

	test("a cancelled touch paints nothing", () => {
		expect(run([touchDown(1, 0, 0), { pointerId: 1, type: "cancel" }]).effects).toEqual([]);
	});
});

describe("two fingers always pan and zoom", () => {
	test("a second finger landing before the first moved paints nothing", () => {
		const { effects, state } = run([touchDown(1, 0, 0), touchDown(2, 100, 0)]);
		expect(effects).toEqual([]);
		expect(state.kind).toBe("pinching");
	});

	test("a second finger mid-stroke ends the stroke and starts a pinch", () => {
		const { effects, state } = run([touchDown(1, 0, 0), move(1, 30, 0), touchDown(2, 100, 0)]);
		expect(effects).toEqual([
			{ from: { x: 0, y: 0 }, to: { x: 30, y: 0 }, type: "paint" },
			{ type: "endStroke" },
		]);
		expect(state.kind).toBe("pinching");
	});

	test("moving both fingers together pans, with no net zoom", () => {
		const { effects } = run([
			touchDown(1, 0, 0),
			touchDown(2, 100, 0),
			move(1, 0, 50),
			move(2, 100, 50),
		]);
		const pinches = effects.filter((effect) => effect.type === "pinch");
		expect(pinches.map((effect) => [effect.anchor, effect.midpoint])).toEqual([
			[
				{ x: 50, y: 0 },
				{ x: 50, y: 25 },
			],
			[
				{ x: 50, y: 25 },
				{ x: 50, y: 50 },
			],
		]);
		const netScale = pinches.reduce((product, effect) => product * effect.scale, 1);
		expect(netScale).toBeCloseTo(1);
	});

	test("spreading the fingers zooms in around them", () => {
		const { effects } = run([touchDown(1, 50, 0), touchDown(2, 150, 0), move(2, 250, 0)]);
		expect(effects).toEqual([
			{ anchor: { x: 100, y: 0 }, midpoint: { x: 150, y: 0 }, scale: 2, type: "pinch" },
		]);
	});

	test("after a pinch the remaining finger never paints, until every finger lifts", () => {
		const { effects, state } = run([
			touchDown(1, 0, 0),
			touchDown(2, 100, 0),
			up(2, 100, 0),
			move(1, 60, 60),
			up(1, 60, 60),
		]);
		expect(effects).toEqual([]);
		expect(state).toEqual(idleGesture);
		const next = run([
			touchDown(1, 0, 0),
			touchDown(2, 100, 0),
			up(1, 0, 0),
			up(2, 100, 0),
			touchDown(3, 5, 5),
			up(3, 5, 5),
		]);
		expect(next.effects).toEqual([
			{ from: { x: 5, y: 5 }, to: { x: 5, y: 5 }, type: "paint" },
			{ type: "endStroke" },
		]);
	});

	test("a third finger is ignored during a pinch", () => {
		const { effects, state } = run([
			touchDown(1, 0, 0),
			touchDown(2, 100, 0),
			touchDown(3, 50, 50),
			move(3, 80, 80),
			up(3, 80, 80),
		]);
		expect(effects).toEqual([]);
		expect(state.kind).toBe("pinching");
	});
});
