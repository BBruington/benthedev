import type { Map as LeafletMap } from "leaflet";
import { latLng } from "leaflet";
import {
	frontierCells,
	type HexCoordinate,
	hexesWithinRadius,
	hexFromNumericKey,
	hexSize,
	hexToWorld,
	hexWorldBounds,
	numericHexKey,
	starterRadius,
	unitHexCorners,
	type WorldBounds,
} from "./hex-math";
import { truncateLabel } from "./labels";
import type { Terrain } from "./stroke-queue";
import { createTerrainTile, terrainStyle, terrainStyles } from "./terrain";

export type ChartedHex = { q: number; r: number; terrain: Terrain; label: string };

type LabelledCenter = { x: number; y: number; label: string; terrain: Terrain };
/** A hex whose terrain changed since the cached bitmap was painted; null means uncharted. */
type ChangedHex = { x: number; y: number; terrain: Terrain | null };

/** Where world point (0, 0) lands in a target's CSS pixels, the zoom scale, and the world rectangle to fill. */
type Viewport = { originX: number; originY: number; scale: number; world: WorldBounds };

type Raster = {
	bitmap: HTMLCanvasElement;
	contentVersion: number;
	outlineColor: string;
	pixelRatio: number;
	zoom: number;
	world: WorldBounds;
};

const squareRootOfThree = Math.sqrt(3);
// Below these on-screen hex widths, patterns and outlines are noise and labels can't be read.
const patternMinimumHexWidth = 10;
const outlineMinimumHexWidth = 6;
const labelMinimumHexWidth = 56;
// The whole map is cached as one bitmap while it fits these device pixels (16 MB at most).
const rasterMaximumSide = 4096;
const rasterMaximumArea = 4096 * 1024;
// A pinch or a stroke draws the cached bitmap (scaled, or with the stroke's hexes on top) and
// repaints it once the map has rested this long.
const rasterRestInMilliseconds = 150;
const rasterMaximumChanges = 400;

/**
 * The hex map's one canvas (spec §8.3). It sits over the Leaflet container and redraws from the
 * map's view on every move, in one path per terrain. Hex centres are flat number arrays, so 5,000 hexes
 * cost no objects per frame. Zoomed out, where every hex is in view, the terrain is painted once into
 * a cached bitmap that panning only copies; zoomed in, only the hexes in view are drawn.
 */
export function createHexCanvas(map: LeafletMap) {
	const container = map.getContainer();
	const canvas = document.createElement("canvas");
	canvas.setAttribute("aria-hidden", "true");
	Object.assign(canvas.style, {
		inset: "0",
		pointerEvents: "none",
		position: "absolute",
		zIndex: "250",
	});
	container.append(canvas);
	const context = canvas.getContext("2d");

	let centersByTerrain = new Map<Terrain, Float64Array>();
	let labelled: LabelledCenter[] = [];
	let outlineCenters = new Float64Array();
	let contentBounds: WorldBounds | undefined;
	let contentVersion = 0;
	let terrainsByKey = new Map<number, Terrain>();
	let changesSinceRaster = new Map<number, ChangedHex>();
	let selected: HexCoordinate | undefined;
	let patterns = new Map<Terrain, CanvasPattern>();
	let patternPixelRatio = 0;
	let raster: Raster | undefined;
	let rasterTimer: ReturnType<typeof setTimeout> | undefined;
	let truncatedLabels = new Map<string, string>();
	let truncatedForWidth = 0;
	let frameRequest = 0;

	function scheduleDraw() {
		if (frameRequest === 0) {
			frameRequest = requestAnimationFrame(draw);
		}
	}

	function setHexes(hexes: readonly ChartedHex[]) {
		const grouped = new Map<Terrain, number[]>();
		const nextLabelled: LabelledCenter[] = [];
		const nextTerrains = new Map<number, Terrain>();
		for (const hex of hexes) {
			const center = hexToWorld(hex);
			const key = numericHexKey(hex.q, hex.r);
			nextTerrains.set(key, hex.terrain);
			if (terrainsByKey.get(key) !== hex.terrain) {
				changesSinceRaster.set(key, { terrain: hex.terrain, x: center.x, y: center.y });
			}
			let centers = grouped.get(hex.terrain);
			if (!centers) {
				centers = [];
				grouped.set(hex.terrain, centers);
			}
			centers.push(center.x, center.y);
			if (hex.label !== "") {
				nextLabelled.push({ label: hex.label, terrain: hex.terrain, x: center.x, y: center.y });
			}
		}
		for (const [key] of terrainsByKey) {
			if (!nextTerrains.has(key)) {
				const center = hexToWorld(hexFromNumericKey(key));
				changesSinceRaster.set(key, { terrain: null, x: center.x, y: center.y });
			}
		}
		terrainsByKey = nextTerrains;
		centersByTerrain = new Map(
			Array.from(grouped, ([terrain, centers]) => [terrain, Float64Array.from(centers)]),
		);
		labelled = nextLabelled;
		const outlines =
			hexes.length === 0 ? hexesWithinRadius({ q: 0, r: 0 }, starterRadius) : frontierCells(hexes);
		outlineCenters = Float64Array.from(
			outlines.flatMap((cell) => {
				const center = hexToWorld(cell);
				return [center.x, center.y];
			}),
		);
		// The frontier rings the charted hexes, so its bounds hold everything drawn.
		contentBounds = hexWorldBounds(outlines);
		contentVersion += 1;
		scheduleDraw();
	}

	function setSelected(cell: HexCoordinate | undefined) {
		selected = cell;
		scheduleDraw();
	}

	function ensurePatterns(target: CanvasRenderingContext2D, pixelRatio: number) {
		if (patternPixelRatio === pixelRatio) {
			return;
		}
		patternPixelRatio = pixelRatio;
		patterns = new Map();
		for (const style of terrainStyles) {
			const pattern = target.createPattern(createTerrainTile(style.terrain, pixelRatio), "repeat");
			if (pattern) {
				patterns.set(style.terrain, pattern);
			}
		}
	}

	function cornersAt(scale: number) {
		const radius = hexSize * scale;
		return unitHexCorners.map((corner) => ({ x: corner.x * radius, y: corner.y * radius }));
	}

	function traceHex(
		path: Path2D,
		corners: readonly { x: number; y: number }[],
		screenX: number,
		screenY: number,
	) {
		const [first, ...rest] = corners;
		if (!first) {
			return;
		}
		path.moveTo(screenX + first.x, screenY + first.y);
		for (const corner of rest) {
			path.lineTo(screenX + corner.x, screenY + corner.y);
		}
		path.closePath();
	}

	function traceInView(centers: Float64Array, viewport: Viewport): Path2D | undefined {
		const corners = cornersAt(viewport.scale);
		const { world } = viewport;
		let path: Path2D | undefined;
		for (let index = 0; index < centers.length; index += 2) {
			const worldX = centers[index] ?? 0;
			const worldY = centers[index + 1] ?? 0;
			if (
				worldX < world.minX ||
				worldX > world.maxX ||
				worldY < world.minY ||
				worldY > world.maxY
			) {
				continue;
			}
			path ??= new Path2D();
			traceHex(
				path,
				corners,
				viewport.originX + worldX * viewport.scale,
				viewport.originY + worldY * viewport.scale,
			);
		}
		return path;
	}

	/** The frontier's dashed outlines and every terrain's fill, pattern and edges. */
	function paintTerrain(
		target: CanvasRenderingContext2D,
		viewport: Viewport,
		pixelRatio: number,
		outlineColor: string,
	) {
		const hexWidth = squareRootOfThree * hexSize * viewport.scale;
		const outlinePath = traceInView(outlineCenters, viewport);
		if (outlinePath) {
			target.setLineDash([4, 4]);
			target.lineWidth = 1;
			target.strokeStyle = outlineColor;
			target.globalAlpha = 0.7;
			target.stroke(outlinePath);
			target.globalAlpha = 1;
			target.setLineDash([]);
		}
		const usePatterns = hexWidth >= patternMinimumHexWidth;
		if (usePatterns) {
			ensurePatterns(target, pixelRatio);
		}
		// Anchored to the world, so a pattern travels with the map instead of swimming under it.
		const patternTransform = new DOMMatrix([
			1 / pixelRatio,
			0,
			0,
			1 / pixelRatio,
			viewport.originX,
			viewport.originY,
		]);
		for (const [terrain, centers] of centersByTerrain) {
			const path = traceInView(centers, viewport);
			if (!path) {
				continue;
			}
			const pattern = usePatterns ? patterns.get(terrain) : undefined;
			if (pattern) {
				pattern.setTransform(patternTransform);
				target.fillStyle = pattern;
			} else {
				target.fillStyle = terrainStyle(terrain).color;
			}
			target.fill(path);
			if (hexWidth >= outlineMinimumHexWidth) {
				target.lineWidth = 1;
				target.strokeStyle = "rgb(0 0 0 / 0.28)";
				target.stroke(path);
			}
		}
	}

	function paintRaster(
		bounds: WorldBounds,
		zoom: number,
		pixelRatio: number,
		outlineColor: string,
	): Raster | undefined {
		const scale = map.getZoomScale(zoom, 0);
		const bitmap = document.createElement("canvas");
		bitmap.width = Math.ceil((bounds.maxX - bounds.minX) * scale * pixelRatio);
		bitmap.height = Math.ceil((bounds.maxY - bounds.minY) * scale * pixelRatio);
		const target = bitmap.getContext("2d");
		if (!target) {
			return undefined;
		}
		target.setTransform(pixelRatio, 0, 0, pixelRatio, 0, 0);
		paintTerrain(
			target,
			{ originX: -bounds.minX * scale, originY: -bounds.minY * scale, scale, world: bounds },
			pixelRatio,
			outlineColor,
		);
		changesSinceRaster = new Map();
		return { bitmap, contentVersion, outlineColor, pixelRatio, world: bounds, zoom };
	}

	function fitsRaster(bounds: WorldBounds, scale: number, pixelRatio: number): boolean {
		const width = (bounds.maxX - bounds.minX) * scale * pixelRatio;
		const height = (bounds.maxY - bounds.minY) * scale * pixelRatio;
		return (
			width <= rasterMaximumSide &&
			height <= rasterMaximumSide &&
			width * height <= rasterMaximumArea
		);
	}

	/** Copies the cached bitmap, repainting it when stale; false when the map is too big for one. */
	function drawFromRaster(
		target: CanvasRenderingContext2D,
		zoom: number,
		origin: { x: number; y: number },
		pixelRatio: number,
		outlineColor: string,
	): boolean {
		const bounds = contentBounds;
		const scale = map.getZoomScale(zoom, 0);
		if (!(bounds && fitsRaster(bounds, scale, pixelRatio))) {
			raster = undefined;
			changesSinceRaster = new Map();
			return false;
		}
		const isUsable =
			raster?.pixelRatio === pixelRatio &&
			raster.outlineColor === outlineColor &&
			changesSinceRaster.size <= rasterMaximumChanges;
		const isStale = raster?.contentVersion !== contentVersion;
		const zoomDrift = raster ? Math.abs(raster.zoom - zoom) : Number.POSITIVE_INFINITY;
		if (!isUsable || zoomDrift > 1) {
			raster = paintRaster(bounds, zoom, pixelRatio, outlineColor);
		} else if (zoomDrift > 0 || isStale) {
			clearTimeout(rasterTimer);
			rasterTimer = setTimeout(() => {
				raster = undefined;
				scheduleDraw();
			}, rasterRestInMilliseconds);
		}
		if (!raster) {
			return false;
		}
		const stretch = scale / map.getZoomScale(raster.zoom, 0);
		target.drawImage(
			raster.bitmap,
			origin.x + raster.world.minX * scale,
			origin.y + raster.world.minY * scale,
			(raster.bitmap.width / raster.pixelRatio) * stretch,
			(raster.bitmap.height / raster.pixelRatio) * stretch,
		);
		if (changesSinceRaster.size > 0) {
			drawChanges(
				target,
				{ originX: origin.x, originY: origin.y, scale, world: bounds },
				pixelRatio,
				outlineColor,
			);
		}
		return true;
	}

	/** A stroke's hexes over the stale bitmap: erased ones cut out to a dashed outline, painted ones filled. */
	function drawChanges(
		target: CanvasRenderingContext2D,
		viewport: Viewport,
		pixelRatio: number,
		outlineColor: string,
	) {
		const corners = cornersAt(viewport.scale);
		const erased = new Path2D();
		const painted = new Map<Terrain, Path2D>();
		for (const change of changesSinceRaster.values()) {
			const screenX = viewport.originX + change.x * viewport.scale;
			const screenY = viewport.originY + change.y * viewport.scale;
			if (change.terrain === null) {
				traceHex(erased, corners, screenX, screenY);
				continue;
			}
			let path = painted.get(change.terrain);
			if (!path) {
				path = new Path2D();
				painted.set(change.terrain, path);
			}
			traceHex(path, corners, screenX, screenY);
		}
		target.globalCompositeOperation = "destination-out";
		target.fill(erased);
		target.globalCompositeOperation = "source-over";
		target.setLineDash([4, 4]);
		target.lineWidth = 1;
		target.strokeStyle = outlineColor;
		target.stroke(erased);
		target.setLineDash([]);
		const hexWidth = squareRootOfThree * hexSize * viewport.scale;
		const usePatterns = hexWidth >= patternMinimumHexWidth;
		if (usePatterns) {
			ensurePatterns(target, pixelRatio);
		}
		for (const [terrain, path] of painted) {
			const pattern = usePatterns ? patterns.get(terrain) : undefined;
			if (pattern) {
				pattern.setTransform(
					new DOMMatrix([1 / pixelRatio, 0, 0, 1 / pixelRatio, viewport.originX, viewport.originY]),
				);
				target.fillStyle = pattern;
			} else {
				target.fillStyle = terrainStyle(terrain).color;
			}
			target.fill(path);
			if (hexWidth >= outlineMinimumHexWidth) {
				target.strokeStyle = "rgb(0 0 0 / 0.28)";
				target.stroke(path);
			}
		}
	}

	function draw() {
		frameRequest = 0;
		if (!context) {
			return;
		}
		const size = map.getSize();
		const pixelRatio = window.devicePixelRatio || 1;
		const width = Math.round(size.x * pixelRatio);
		const height = Math.round(size.y * pixelRatio);
		if (canvas.width !== width || canvas.height !== height) {
			canvas.width = width;
			canvas.height = height;
			canvas.style.width = `${size.x}px`;
			canvas.style.height = `${size.y}px`;
		}
		context.setTransform(pixelRatio, 0, 0, pixelRatio, 0, 0);
		context.clearRect(0, 0, size.x, size.y);

		const zoom = map.getZoom();
		const scale = map.getZoomScale(zoom, 0);
		const origin = map.latLngToContainerPoint(latLng(0, 0));
		const radius = hexSize * scale;
		const hexWidth = squareRootOfThree * radius;
		const viewport: Viewport = {
			originX: origin.x,
			originY: origin.y,
			scale,
			world: {
				maxX: (size.x + radius - origin.x) / scale,
				maxY: (size.y + radius - origin.y) / scale,
				minX: (-radius - origin.x) / scale,
				minY: (-radius - origin.y) / scale,
			},
		};
		const themeStyles = getComputedStyle(container);
		const outlineColor = themeStyles.getPropertyValue("--ink-muted").trim() || "#6b5a44";
		const selectedColor = themeStyles.getPropertyValue("--gilt").trim() || "#7a5a1c";

		if (!drawFromRaster(context, zoom, origin, pixelRatio, outlineColor)) {
			paintTerrain(context, viewport, pixelRatio, outlineColor);
		}

		if (selected) {
			const center = hexToWorld(selected);
			const path = new Path2D();
			traceHex(path, cornersAt(scale), origin.x + center.x * scale, origin.y + center.y * scale);
			context.lineWidth = 3;
			context.strokeStyle = selectedColor;
			context.stroke(path);
		}

		if (hexWidth >= labelMinimumHexWidth && labelled.length > 0) {
			drawLabels(context, hexWidth, viewport);
		}
	}

	function drawLabels(target: CanvasRenderingContext2D, hexWidth: number, viewport: Viewport) {
		const fontSize = Math.round(Math.min(16, Math.max(11, hexWidth * 0.17)));
		const maximumWidth = hexWidth * 0.84;
		if (truncatedForWidth !== maximumWidth) {
			truncatedForWidth = maximumWidth;
			truncatedLabels = new Map();
		}
		target.font = `600 ${fontSize}px "Alegreya Sans", system-ui, sans-serif`;
		target.textAlign = "center";
		target.textBaseline = "middle";
		target.lineJoin = "round";
		target.lineWidth = 3;
		const { world } = viewport;
		for (const hex of labelled) {
			if (hex.x < world.minX || hex.x > world.maxX || hex.y < world.minY || hex.y > world.maxY) {
				continue;
			}
			let text = truncatedLabels.get(hex.label);
			if (text === undefined) {
				text = truncateLabel(
					hex.label,
					maximumWidth,
					(candidate) => target.measureText(candidate).width,
				);
				truncatedLabels.set(hex.label, text);
			}
			const style = terrainStyle(hex.terrain);
			const x = viewport.originX + hex.x * viewport.scale;
			const y = viewport.originY + hex.y * viewport.scale;
			target.strokeStyle = style.halo;
			target.strokeText(text, x, y);
			target.fillStyle = style.text;
			target.fillText(text, x, y);
		}
	}

	map.on("move zoom resize viewreset", scheduleDraw);
	scheduleDraw();

	return {
		destroy() {
			map.off("move zoom resize viewreset", scheduleDraw);
			cancelAnimationFrame(frameRequest);
			clearTimeout(rasterTimer);
			canvas.remove();
		},
		redraw: scheduleDraw,
		setHexes,
		setSelected,
	};
}

export type HexCanvas = ReturnType<typeof createHexCanvas>;
