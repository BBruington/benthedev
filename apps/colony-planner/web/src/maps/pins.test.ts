import { describe, expect, test } from "bun:test";
import { artBounds, clampUnit, isOnArt, latLngToPin, pinToLatLng } from "./pins";

const eldari = { height: 1080, width: 1920 };

describe("pin normalization", () => {
	test("the art spans latitude 0 at the bottom to its height at the top", () => {
		expect(artBounds(eldari)).toEqual([
			[0, 0],
			[1080, 1920],
		]);
	});

	test("y counts down from the top edge", () => {
		expect(pinToLatLng({ x: 0, y: 0 }, eldari)).toEqual([1080, 0]);
		expect(pinToLatLng({ x: 1, y: 1 }, eldari)).toEqual([0, 1920]);
		expect(pinToLatLng({ x: 0.25, y: 0.75 }, eldari)).toEqual([270, 480]);
	});

	test("a point survives the round trip", () => {
		const point = { x: 0.3125, y: 0.6 };
		const [lat, lng] = pinToLatLng(point, eldari);
		const back = latLngToPin({ lat, lng }, eldari);
		expect(back.x).toBeCloseTo(point.x, 10);
		expect(back.y).toBeCloseTo(point.y, 10);
	});

	test("the same fractions keep a pin in place on a larger copy of the art", () => {
		const point = latLngToPin({ lat: 540, lng: 480 }, eldari);
		expect(pinToLatLng(point, { height: 2160, width: 3840 })).toEqual([1080, 960]);
	});

	test("a pin dragged past an edge stays on the art", () => {
		expect(latLngToPin({ lat: -50, lng: 2500 }, eldari)).toEqual({ x: 1, y: 1 });
		expect(latLngToPin({ lat: 2000, lng: -10 }, eldari)).toEqual({ x: 0, y: 0 });
	});

	test("clamping turns nonsense into the nearest edge", () => {
		expect(clampUnit(Number.NaN)).toBe(0);
		expect(clampUnit(-0.2)).toBe(0);
		expect(clampUnit(1.4)).toBe(1);
		expect(clampUnit(0.5)).toBe(0.5);
	});

	test("only taps on the art count", () => {
		expect(isOnArt({ lat: 540, lng: 960 }, eldari)).toBe(true);
		expect(isOnArt({ lat: 0, lng: 1920 }, eldari)).toBe(true);
		expect(isOnArt({ lat: -1, lng: 960 }, eldari)).toBe(false);
		expect(isOnArt({ lat: 540, lng: 1921 }, eldari)).toBe(false);
	});
});
