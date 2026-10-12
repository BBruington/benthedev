// Paint mode's gesture split (spec §6.4): one finger paints, two fingers always pan and pinch-zoom.
// Leaflet's dragging and touch zoom are off in Paint mode and this reducer owns every pointer, so the
// split doesn't depend on how Leaflet's touch handling behaves with dragging off (spec §8.3).

export type ScreenPoint = { x: number; y: number };
export type PointerKind = "mouse" | "pen" | "touch";

export type GestureEvent =
	| { type: "down"; pointerId: number; point: ScreenPoint; pointerKind: PointerKind }
	| { type: "move"; pointerId: number; point: ScreenPoint }
	| { type: "up"; pointerId: number; point: ScreenPoint }
	| { type: "cancel"; pointerId: number };

type TrackedPointer = { pointerId: number; point: ScreenPoint };

export type GestureState =
	| { kind: "idle" }
	/** One finger is down but hasn't moved far: it may still become the first finger of a pinch. */
	| { kind: "pressed"; pointer: TrackedPointer; start: ScreenPoint }
	| { kind: "painting"; pointer: TrackedPointer }
	| { kind: "pinching"; first: TrackedPointer; second: TrackedPointer }
	/** A pinch lost a finger; the rest are ignored until they lift, so a pinch never ends in paint. */
	| { kind: "lifting"; pointerIds: readonly number[] };

export type GestureEffect =
	/** Paint every hex along the screen segment from one point to the other. */
	| { type: "paint"; from: ScreenPoint; to: ScreenPoint }
	| { type: "endStroke" }
	/**
	 * Keep the map point that sat under `anchor` under `midpoint`, scaling the view by `scale`.
	 * Both points are between the two fingers, before and after the move.
	 */
	| { type: "pinch"; anchor: ScreenPoint; midpoint: ScreenPoint; scale: number };

export type GestureStep = { state: GestureState; effects: GestureEffect[] };

/** How far a lone finger moves before it is a stroke rather than, perhaps, a pinch landing. */
export const paintSlopInPixels = 8;

export const idleGesture: GestureState = { kind: "idle" };

export function stepGesture(state: GestureState, event: GestureEvent): GestureStep {
	switch (state.kind) {
		case "idle":
			return stepIdle(event);
		case "pressed":
			return stepPressed(state, event);
		case "painting":
			return stepPainting(state, event);
		case "pinching":
			return stepPinching(state, event);
		case "lifting":
			return stepLifting(state, event);
		default:
			return { effects: [], state };
	}
}

function stepIdle(event: GestureEvent): GestureStep {
	if (event.type !== "down") {
		return { effects: [], state: idleGesture };
	}
	const pointer = { point: event.point, pointerId: event.pointerId };
	// A mouse or pen has one contact, so it paints at once.
	if (event.pointerKind !== "touch") {
		return {
			effects: [{ from: event.point, to: event.point, type: "paint" }],
			state: { kind: "painting", pointer },
		};
	}
	return { effects: [], state: { kind: "pressed", pointer, start: event.point } };
}

function stepPressed(
	state: Extract<GestureState, { kind: "pressed" }>,
	event: GestureEvent,
): GestureStep {
	if (event.type === "down") {
		return {
			effects: [],
			state: {
				first: state.pointer,
				kind: "pinching",
				second: { point: event.point, pointerId: event.pointerId },
			},
		};
	}
	if (event.pointerId !== state.pointer.pointerId) {
		return { effects: [], state };
	}
	if (event.type === "cancel") {
		return { effects: [], state: idleGesture };
	}
	if (event.type === "up") {
		return {
			effects: [{ from: state.start, to: state.start, type: "paint" }, { type: "endStroke" }],
			state: idleGesture,
		};
	}
	if (distance(state.start, event.point) < paintSlopInPixels) {
		return { effects: [], state };
	}
	return {
		effects: [{ from: state.start, to: event.point, type: "paint" }],
		state: { kind: "painting", pointer: { point: event.point, pointerId: event.pointerId } },
	};
}

function stepPainting(
	state: Extract<GestureState, { kind: "painting" }>,
	event: GestureEvent,
): GestureStep {
	if (event.type === "down") {
		return {
			effects: [{ type: "endStroke" }],
			state: {
				first: state.pointer,
				kind: "pinching",
				second: { point: event.point, pointerId: event.pointerId },
			},
		};
	}
	if (event.pointerId !== state.pointer.pointerId) {
		return { effects: [], state };
	}
	if (event.type === "cancel") {
		return { effects: [{ type: "endStroke" }], state: idleGesture };
	}
	if (event.type === "up") {
		return {
			effects: [
				{ from: state.pointer.point, to: event.point, type: "paint" },
				{ type: "endStroke" },
			],
			state: idleGesture,
		};
	}
	return {
		effects: [{ from: state.pointer.point, to: event.point, type: "paint" }],
		state: { kind: "painting", pointer: { point: event.point, pointerId: event.pointerId } },
	};
}

function stepPinching(
	state: Extract<GestureState, { kind: "pinching" }>,
	event: GestureEvent,
): GestureStep {
	const isFirst = event.pointerId === state.first.pointerId;
	const isSecond = event.pointerId === state.second.pointerId;
	if (event.type === "down" || !(isFirst || isSecond)) {
		return { effects: [], state };
	}
	if (event.type === "up" || event.type === "cancel") {
		const remaining = isFirst ? state.second.pointerId : state.first.pointerId;
		return { effects: [], state: { kind: "lifting", pointerIds: [remaining] } };
	}
	const moved = { point: event.point, pointerId: event.pointerId };
	const first = isFirst ? moved : state.first;
	const second = isSecond ? moved : state.second;
	const previousDistance = distance(state.first.point, state.second.point);
	const nextDistance = distance(first.point, second.point);
	const scale = previousDistance > 0 && nextDistance > 0 ? nextDistance / previousDistance : 1;
	return {
		effects: [
			{
				anchor: midpointOf(state.first.point, state.second.point),
				midpoint: midpointOf(first.point, second.point),
				scale,
				type: "pinch",
			},
		],
		state: { first, kind: "pinching", second },
	};
}

function stepLifting(
	state: Extract<GestureState, { kind: "lifting" }>,
	event: GestureEvent,
): GestureStep {
	if (event.type === "down") {
		return {
			effects: [],
			state: { kind: "lifting", pointerIds: [...state.pointerIds, event.pointerId] },
		};
	}
	if (event.type === "move") {
		return { effects: [], state };
	}
	const pointerIds = state.pointerIds.filter((pointerId) => pointerId !== event.pointerId);
	if (pointerIds.length === 0) {
		return { effects: [], state: idleGesture };
	}
	return { effects: [], state: { kind: "lifting", pointerIds } };
}

function distance(first: ScreenPoint, second: ScreenPoint): number {
	return Math.hypot(first.x - second.x, first.y - second.y);
}

function midpointOf(first: ScreenPoint, second: ScreenPoint): ScreenPoint {
	return { x: (first.x + second.x) / 2, y: (first.y + second.y) / 2 };
}
