import "leaflet/dist/leaflet.css";
import "@/maps/zoom-control.css";
import {
	CRS,
	control,
	map as createLeafletMap,
	type Map as LeafletMap,
	type LeafletMouseEvent,
	latLng,
	latLngBounds,
	point,
} from "leaflet";
import { type RefObject, useEffect, useImperativeHandle, useRef } from "react";
import { subscribeToResolvedTheme } from "@/lib/theme";
import { type ChartedHex, createHexCanvas, type HexCanvas } from "./hex-canvas";
import {
	type HexCoordinate,
	hexesWithinRadius,
	hexLine,
	hexToWorld,
	hexWorldBounds,
	starterRadius,
	type WorldBounds,
	worldToHex,
} from "./hex-math";
import {
	type GestureEffect,
	type GestureEvent,
	idleGesture,
	type PointerKind,
	stepGesture,
} from "./paint-gesture";

export type ExploreMode = "look" | "paint";

/** What the toolbar asks of the map. */
export type ExploreMapView = {
	fit: () => void;
	zoomToHex: (cell: HexCoordinate) => void;
};

const minimumZoom = -4;
const maximumZoom = 3;
/** Labels show from zoom 1 up (labelMinimumHexWidth in hex-canvas.ts), so Find lands past it. */
const findZoom = 2;

function worldBoundsToLatLng(bounds: WorldBounds) {
	return latLngBounds(latLng(-bounds.maxY, bounds.minX), latLng(-bounds.minY, bounds.maxX));
}

/** Shows everything charted, or the starter area while nothing is. */
function fitCharted(map: LeafletMap, charted: readonly ChartedHex[]) {
	const bounds = hexWorldBounds(
		charted.length > 0 ? charted : hexesWithinRadius({ q: 0, r: 0 }, starterRadius),
	);
	if (bounds) {
		map.fitBounds(worldBoundsToLatLng(bounds), { animate: false, padding: point(24, 24) });
	}
}

function pointerKindOf(event: PointerEvent): PointerKind {
	if (event.pointerType === "touch" || event.pointerType === "pen") {
		return event.pointerType;
	}
	return "mouse";
}

/**
 * The Leaflet map behind /explore (spec §6.4, §8.3): CRS.Simple with one hex canvas. Look mode is
 * Leaflet's own pan, pinch and tap; Paint mode turns Leaflet's dragging and touch zoom off and feeds
 * every pointer to the gesture reducer, which paints with one finger and pans and zooms with two.
 */
export function ExploreMap({
	hexes,
	mode,
	selected,
	onTapHex,
	onPaint,
	onStrokeEnd,
	viewRef,
}: {
	hexes: readonly ChartedHex[];
	mode: ExploreMode;
	selected: HexCoordinate | undefined;
	onTapHex: (cell: HexCoordinate) => void;
	onPaint: (cells: HexCoordinate[]) => void;
	onStrokeEnd: () => void;
	viewRef: RefObject<ExploreMapView | null>;
}) {
	const containerRef = useRef<HTMLDivElement>(null);
	const mapRef = useRef<LeafletMap | null>(null);
	const canvasRef = useRef<HexCanvas | null>(null);
	const hexesRef = useRef(hexes);
	hexesRef.current = hexes;
	const modeRef = useRef(mode);
	modeRef.current = mode;
	const callbacksRef = useRef({ onPaint, onStrokeEnd, onTapHex });
	callbacksRef.current = { onPaint, onStrokeEnd, onTapHex };
	// What the canvas last drew, so a render that didn't change the hexes doesn't rebuild the frontier.
	const drawnHexesRef = useRef<readonly ChartedHex[] | null>(null);

	useImperativeHandle(viewRef, () => ({
		fit: () => {
			if (mapRef.current) {
				fitCharted(mapRef.current, hexesRef.current);
			}
		},
		zoomToHex: (cell) => {
			const center = hexToWorld(cell);
			mapRef.current?.setView(latLng(-center.y, center.x), findZoom, { animate: false });
		},
	}));

	useEffect(() => {
		const container = containerRef.current;
		if (!container) {
			return;
		}
		const map = createLeafletMap(container, {
			attributionControl: false,
			crs: CRS.Simple,
			// A Look-mode tap opens the hex sheet, so a double tap must not also zoom.
			doubleClickZoom: false,
			fadeAnimation: false,
			markerZoomAnimation: false,
			maxZoom: maximumZoom,
			minZoom: minimumZoom,
			wheelPxPerZoomLevel: 90,
			zoomAnimation: false,
			zoomControl: false,
			zoomDelta: 0.5,
			// Fractional zoom keeps the custom pinch smooth; the canvas redraws at any scale.
			zoomSnap: 0,
		});
		control.zoom({ position: "topright" }).addTo(map);
		map.setView(latLng(0, 0), 0);
		mapRef.current = map;
		canvasRef.current = createHexCanvas(map);
		canvasRef.current.setHexes(hexesRef.current);
		drawnHexesRef.current = hexesRef.current;
		fitCharted(map, hexesRef.current);
		map.on("click", (event: LeafletMouseEvent) => {
			if (modeRef.current !== "look") {
				return;
			}
			callbacksRef.current.onTapHex(worldToHex({ x: event.latlng.lng, y: -event.latlng.lat }));
		});
		// The frontier and selection take their colours from the theme when drawn, so a switch redraws.
		const stopWatchingTheme = subscribeToResolvedTheme(() => canvasRef.current?.redraw());
		// Leaflet watches only the window, and the top bar's View as player banner resizes the map too;
		// invalidateSize fires Leaflet's resize, which the canvas redraws on.
		const sizeObserver = new ResizeObserver(() => map.invalidateSize());
		sizeObserver.observe(container);
		return () => {
			sizeObserver.disconnect();
			stopWatchingTheme();
			canvasRef.current?.destroy();
			canvasRef.current = null;
			map.remove();
			mapRef.current = null;
		};
	}, []);

	useEffect(() => {
		if (drawnHexesRef.current === hexes) {
			return;
		}
		drawnHexesRef.current = hexes;
		canvasRef.current?.setHexes(hexes);
	});

	useEffect(() => {
		canvasRef.current?.setSelected(selected);
	}, [selected]);

	useEffect(() => {
		const map = mapRef.current;
		const container = containerRef.current;
		if (!(map && container) || mode !== "paint") {
			return;
		}
		map.dragging.disable();
		map.touchZoom.disable();
		map.boxZoom.disable();
		// Leaflet leaves pan-x pan-y once dragging is off, which would hand one-finger moves to the page.
		container.style.touchAction = "none";
		let gesture = idleGesture;

		function hexAt(screen: { x: number; y: number }): HexCoordinate {
			const position = map?.containerPointToLatLng(point(screen.x, screen.y));
			return worldToHex({ x: position?.lng ?? 0, y: -(position?.lat ?? 0) });
		}

		function apply(effect: GestureEffect) {
			if (!map) {
				return;
			}
			if (effect.type === "paint") {
				callbacksRef.current.onPaint(hexLine(hexAt(effect.from), hexAt(effect.to)));
				return;
			}
			if (effect.type === "endStroke") {
				callbacksRef.current.onStrokeEnd();
				return;
			}
			const anchor = map.containerPointToLatLng(point(effect.anchor.x, effect.anchor.y));
			const zoom = Math.min(
				maximumZoom,
				Math.max(minimumZoom, map.getZoom() + Math.log2(effect.scale)),
			);
			const halfSize = map.getSize().divideBy(2);
			const centerPixel = map
				.project(anchor, zoom)
				.subtract(point(effect.midpoint.x, effect.midpoint.y))
				.add(halfSize);
			map.setView(map.unproject(centerPixel, zoom), zoom, { animate: false });
		}

		function handle(event: PointerEvent) {
			if (event.target instanceof Element && event.target.closest(".leaflet-control-container")) {
				return;
			}
			const screen = map?.mouseEventToContainerPoint(event);
			if (!screen) {
				return;
			}
			let gestureEvent: GestureEvent;
			if (event.type === "pointerdown") {
				if (event.pointerType === "mouse" && event.button !== 0) {
					return;
				}
				event.preventDefault();
				container?.setPointerCapture(event.pointerId);
				gestureEvent = {
					point: screen,
					pointerId: event.pointerId,
					pointerKind: pointerKindOf(event),
					type: "down",
				};
			} else if (event.type === "pointermove") {
				gestureEvent = { point: screen, pointerId: event.pointerId, type: "move" };
			} else if (event.type === "pointerup") {
				gestureEvent = { point: screen, pointerId: event.pointerId, type: "up" };
			} else {
				gestureEvent = { pointerId: event.pointerId, type: "cancel" };
			}
			const step = stepGesture(gesture, gestureEvent);
			gesture = step.state;
			for (const effect of step.effects) {
				apply(effect);
			}
		}

		const pointerEvents = ["pointerdown", "pointermove", "pointerup", "pointercancel"] as const;
		for (const type of pointerEvents) {
			container.addEventListener(type, handle);
		}
		return () => {
			for (const type of pointerEvents) {
				container.removeEventListener(type, handle);
			}
			if (gesture.kind === "painting" || gesture.kind === "pressed") {
				callbacksRef.current.onStrokeEnd();
			}
			container.style.touchAction = "";
			map.dragging.enable();
			map.touchZoom.enable();
			map.boxZoom.enable();
		};
	}, [mode]);

	return (
		<div
			aria-label="Hex map of the new world"
			className="explore-map absolute inset-0"
			data-mode={mode}
			ref={containerRef}
			role="application"
			// Inline, because Leaflet's stylesheet paints its container grey and loads after ours.
			style={{ background: "var(--surface-sunken)" }}
		/>
	);
}
