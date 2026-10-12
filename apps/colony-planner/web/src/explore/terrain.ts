import type { Terrain } from "./stroke-queue";

export type TerrainStyle = {
	terrain: Terrain;
	label: string;
	/** Ported from the old map page (spec §5.10). */
	color: string;
	/** Pattern marks and labels: dark on light terrains, light on dark ones. */
	ink: string;
	/** A label's text, and the halo behind it in the opposite shade. */
	text: string;
	halo: string;
};

const darkInk = "rgb(20 20 20 / 0.55)";
const lightInk = "rgb(255 255 255 / 0.6)";
const darkText = "#141414";
const lightText = "#ffffff";

export type TerrainLook = Omit<TerrainStyle, "terrain">;

const terrainLooks: Record<Terrain, TerrainLook> = {
	DESERT: { color: "#eab308", halo: "#fdf3c7", ink: darkInk, label: "Desert", text: darkText },
	FOREST: { color: "#15803d", halo: "#0b3d1d", ink: lightInk, label: "Forest", text: lightText },
	MOUNTAIN: {
		color: "#78716c",
		halo: "#2e2b29",
		ink: lightInk,
		label: "Mountain",
		text: lightText,
	},
	PLAINS: { color: "#a3e635", halo: "#f4fde4", ink: darkInk, label: "Plains", text: darkText },
	SWAMP: { color: "#064e3b", halo: "#021f17", ink: lightInk, label: "Swamp", text: lightText },
	TUNDRA: { color: "#e2e8f0", halo: "#ffffff", ink: darkInk, label: "Tundra", text: darkText },
	VOLCANIC: {
		color: "#c2410c",
		halo: "#4a1904",
		ink: lightInk,
		label: "Volcanic",
		text: lightText,
	},
	WATER: { color: "#3b82f6", halo: "#10306b", ink: lightInk, label: "Water", text: lightText },
};

/** In shortcut order: keys 1 to 8 pick these (spec §6.4). */
const terrainOrder: readonly Terrain[] = [
	"FOREST",
	"PLAINS",
	"WATER",
	"MOUNTAIN",
	"DESERT",
	"SWAMP",
	"TUNDRA",
	"VOLCANIC",
];

export const terrainStyles: readonly TerrainStyle[] = terrainOrder.map((terrain) => ({
	terrain,
	...terrainLooks[terrain],
}));

export function terrainStyle(terrain: Terrain): TerrainLook {
	return terrainLooks[terrain];
}

/** A pattern tile's side in CSS pixels. */
export const patternTileSize = 12;

/**
 * Draws one terrain's tile, its colour plus marks, so colour-blind players can tell terrains apart.
 * The context is already scaled so one unit is one CSS pixel.
 */
export function drawTerrainTile(context: CanvasRenderingContext2D, terrain: Terrain) {
	const style = terrainStyle(terrain);
	const size = patternTileSize;
	context.fillStyle = style.color;
	context.fillRect(0, 0, size, size);
	context.strokeStyle = style.ink;
	context.fillStyle = style.ink;
	context.lineWidth = 1.2;
	context.lineCap = "round";
	context.beginPath();
	switch (terrain) {
		case "FOREST":
			context.moveTo(6, 2.5);
			context.lineTo(9, 8);
			context.lineTo(3, 8);
			context.closePath();
			context.fill();
			return;
		case "PLAINS":
			context.moveTo(1.5, 3.5);
			context.lineTo(5.5, 3.5);
			context.moveTo(7.5, 9.5);
			context.lineTo(11.5, 9.5);
			break;
		case "WATER":
			context.moveTo(1, 6);
			context.quadraticCurveTo(3.5, 3, 6, 6);
			context.quadraticCurveTo(8.5, 9, 11, 6);
			break;
		case "MOUNTAIN":
			context.moveTo(2, 9);
			context.lineTo(6, 3.5);
			context.lineTo(10, 9);
			break;
		case "DESERT":
			context.arc(3, 3, 1, 0, Math.PI * 2);
			context.moveTo(10, 9);
			context.arc(9, 9, 1, 0, Math.PI * 2);
			context.fill();
			return;
		case "SWAMP":
			context.moveTo(2, 9);
			context.lineTo(10, 9);
			context.moveTo(4, 9);
			context.lineTo(4, 5);
			context.moveTo(6, 9);
			context.lineTo(6, 3.5);
			context.moveTo(8, 9);
			context.lineTo(8, 5);
			break;
		case "TUNDRA":
			context.moveTo(6, 2.5);
			context.lineTo(6, 9.5);
			context.moveTo(2.5, 6);
			context.lineTo(9.5, 6);
			break;
		case "VOLCANIC":
			context.moveTo(0, 12);
			context.lineTo(12, 0);
			context.moveTo(-3, 3);
			context.lineTo(3, -3);
			context.moveTo(9, 15);
			context.lineTo(15, 9);
			break;
		default:
			return;
	}
	context.stroke();
}

/** A tile drawn at the screen's pixel density, for createPattern and the palette's chips. */
export function createTerrainTile(terrain: Terrain, pixelRatio: number): HTMLCanvasElement {
	const tile = document.createElement("canvas");
	tile.width = Math.round(patternTileSize * pixelRatio);
	tile.height = tile.width;
	const context = tile.getContext("2d");
	if (context) {
		context.scale(tile.width / patternTileSize, tile.height / patternTileSize);
		drawTerrainTile(context, terrain);
	}
	return tile;
}
