import { EraserIcon } from "lucide-react";
import { useEffect, useMemo, useRef } from "react";
import { cn } from "@/lib/utils";
import type { StrokeTerrain, Terrain } from "./stroke-queue";
import { createTerrainTile, terrainStyle, terrainStyles } from "./terrain";

const unchartBrush = "UNCHART";

/** A terrain's pattern as a CSS background, for chips and the hex sheet. */
export function useTerrainSwatches(): ReadonlyMap<Terrain, string> {
	return useMemo(() => {
		const pixelRatio = window.devicePixelRatio || 1;
		return new Map(
			terrainStyles.map((style) => [
				style.terrain,
				`url(${createTerrainTile(style.terrain, pixelRatio).toDataURL()})`,
			]),
		);
	}, []);
}

export function TerrainSwatch({
	terrain,
	swatches,
	className,
}: {
	terrain: Terrain;
	swatches: ReadonlyMap<Terrain, string>;
	className?: string;
}) {
	return (
		<span
			aria-hidden="true"
			className={cn("inline-block shrink-0 rounded-sm border border-black/25", className)}
			style={{
				backgroundColor: terrainStyle(terrain).color,
				backgroundImage: swatches.get(terrain),
				backgroundSize: "12px 12px",
			}}
		/>
	);
}

/**
 * The brush picker (spec §6.4): a horizontally scrolling row of the 8 terrains, each with its
 * pattern, plus Unchart; the selected chip is enlarged.
 */
export function TerrainPalette({
	brush,
	onBrushChange,
}: {
	brush: StrokeTerrain;
	onBrushChange: (brush: StrokeTerrain) => void;
}) {
	const swatches = useTerrainSwatches();
	const chipsRef = useRef<HTMLDivElement>(null);
	// A shortcut can pick a chip that is scrolled out of sight on a phone.
	useEffect(() => {
		chipsRef.current
			?.querySelector(`[data-brush="${brush ?? unchartBrush}"]`)
			?.scrollIntoView({ behavior: "smooth", block: "nearest", inline: "nearest" });
	}, [brush]);
	const chipClass =
		"flex h-11 shrink-0 snap-start items-center gap-2 rounded-full border px-3 font-medium text-sm transition-transform focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/45";
	return (
		<fieldset className="-mx-4 min-w-0 md:mx-0">
			<legend className="sr-only">Brush</legend>
			<div
				className="flex snap-x items-center gap-2 overflow-x-auto px-4 py-2 md:px-0"
				ref={chipsRef}
			>
				{terrainStyles.map((style, index) => {
					const isSelected = brush === style.terrain;
					return (
						<button
							aria-keyshortcuts={String(index + 1)}
							aria-pressed={isSelected}
							className={cn(
								chipClass,
								isSelected
									? "scale-110 border-gilt bg-surface-raised shadow-sm"
									: "border-hairline bg-surface",
							)}
							data-brush={style.terrain}
							key={style.terrain}
							onClick={() => {
								onBrushChange(style.terrain);
							}}
							type="button"
						>
							<TerrainSwatch
								className="size-6 rounded-full"
								swatches={swatches}
								terrain={style.terrain}
							/>
							{style.label}
						</button>
					);
				})}
				<button
					aria-keyshortcuts="E"
					aria-pressed={brush === null}
					className={cn(
						chipClass,
						brush === null
							? "scale-110 border-gilt bg-surface-raised shadow-sm"
							: "border-hairline bg-surface",
					)}
					data-brush={unchartBrush}
					onClick={() => {
						onBrushChange(null);
					}}
					type="button"
				>
					<EraserIcon className="size-5" />
					Unchart
				</button>
			</div>
		</fieldset>
	);
}
