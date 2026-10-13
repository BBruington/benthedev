import { HandIcon, HexagonIcon, MaximizeIcon, PaintbrushIcon, SearchIcon } from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { problemMessage } from "@/api/problem";
import { BottomSheet } from "@/components/bottom-sheet";
import { StatusPanel } from "@/components/status-panel";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useSheetState } from "@/hooks/use-sheet-state";
import { cn } from "@/lib/utils";
import { ExploreMap, type ExploreMapView, type ExploreMode } from "./explore-map";
import type { ChartedHex } from "./hex-canvas";
import { type HexCoordinate, hexKey } from "./hex-math";
import { HexSheet } from "./hex-sheet";
import { findHexesByLabel } from "./labels";
import type { StrokeTerrain } from "./stroke-queue";
import { terrainStyle, terrainStyles } from "./terrain";
import { TerrainPalette, TerrainSwatch, useTerrainSwatches } from "./terrain-palette";
import { useChartedHexes } from "./use-charted-hexes";

const findResultLimit = 12;

function isTypingTarget(target: EventTarget | null): boolean {
	return (
		target instanceof HTMLElement &&
		(target.isContentEditable || ["INPUT", "SELECT", "TEXTAREA"].includes(target.tagName))
	);
}

function chartedSummary(count: number): string {
	if (count === 0) {
		return "Nothing charted yet. Switch to Paint and start drawing the new world.";
	}
	if (count === 1) {
		return "1 hex charted.";
	}
	return `${count.toLocaleString()} hexes charted.`;
}

/** /explore (spec §5.10, §6.4): the players' own hex map of the new world. */
export function ExploreScreen({ campaignId }: { campaignId: string }) {
	const { displayed, flush, hexes, paint } = useChartedHexes(campaignId);
	const [mode, setMode] = useState<ExploreMode>("look");
	const [brush, setBrush] = useState<StrokeTerrain>("FOREST");
	const viewRef = useRef<ExploreMapView | null>(null);
	const tapped = useSheetState<HexCoordinate>();
	const finding = useSheetState<true>();
	const brushRef = useRef(brush);
	brushRef.current = brush;

	useEffect(() => {
		function handleKeyDown(event: KeyboardEvent) {
			if (event.altKey || event.ctrlKey || event.metaKey || isTypingTarget(event.target)) {
				return;
			}
			if (document.querySelector("[role=dialog]")) {
				return;
			}
			const key = event.key.toLowerCase();
			const terrainIndex = Number.parseInt(key, 10);
			const terrain = terrainStyles[terrainIndex - 1];
			if (terrain && key.length === 1) {
				setBrush(terrain.terrain);
			} else if (key === "p") {
				setMode((current) => (current === "paint" ? "look" : "paint"));
			} else if (key === "e") {
				setBrush(null);
			} else {
				return;
			}
			event.preventDefault();
		}
		window.addEventListener("keydown", handleKeyDown);
		return () => {
			window.removeEventListener("keydown", handleKeyDown);
		};
	}, []);

	if (hexes.isPending) {
		return <Skeleton aria-busy="true" className="h-[60dvh] rounded-xl" />;
	}
	if (hexes.isError && !hexes.data) {
		return (
			<StatusPanel
				description={problemMessage(hexes.error, "The hex map couldn't be loaded.")}
				icon={HexagonIcon}
				title="The hex map couldn't be loaded"
			/>
		);
	}

	const tappedCell = tapped.value;
	const brushName = brush === null ? "Unchart" : terrainStyle(brush).label;
	return (
		<div className="grid gap-2">
			<div className="flex flex-wrap items-center justify-between gap-2">
				<div className="min-w-0">
					<h1
						className="font-display font-semibold text-2xl text-gilt-ink tracking-wide"
						id="explore-heading"
						tabIndex={-1}
					>
						Explore
					</h1>
					<p className="text-muted-foreground text-sm">{chartedSummary(displayed.hexes.length)}</p>
				</div>
				<div className="flex gap-2">
					<Button
						onClick={() => {
							finding.show(true);
						}}
						variant="outline"
					>
						<SearchIcon />
						Find
					</Button>
					<Button
						onClick={() => {
							viewRef.current?.fit();
						}}
						variant="outline"
					>
						<MaximizeIcon />
						Fit
					</Button>
				</div>
			</div>
			<div className="relative isolate h-[calc(100dvh-var(--shell-header-height,4rem)-14.5rem-env(safe-area-inset-bottom))] min-h-80 overflow-hidden rounded-xl border border-hairline md:h-[calc(100dvh-var(--shell-header-height,4rem)-15rem)]">
				<ExploreMap
					hexes={displayed.hexes}
					mode={mode}
					onPaint={(cells) => {
						paint(cells, brushRef.current);
					}}
					onStrokeEnd={flush}
					onTapHex={(cell) => {
						tapped.show(cell);
					}}
					selected={tapped.open && tappedCell ? tappedCell : undefined}
					viewRef={viewRef}
				/>
				{mode === "paint" ? (
					<p className="pointer-events-none absolute top-3 left-3 z-[500] rounded-full bg-surface/90 px-3 py-1.5 text-sm shadow-sm">
						Painting {brushName} · two fingers move the map
					</p>
				) : null}
				<button
					aria-keyshortcuts="P"
					aria-label={mode === "paint" ? "Painting. Switch to Look" : "Looking. Switch to Paint"}
					aria-pressed={mode === "paint"}
					className={cn(
						"absolute right-3 bottom-3 z-[500] flex size-16 items-center justify-center rounded-full border-2 shadow-lg transition-colors focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/45",
						mode === "paint"
							? "border-gilt bg-primary text-primary-foreground"
							: "border-hairline bg-surface-raised text-foreground",
					)}
					onClick={() => {
						setMode((current) => (current === "paint" ? "look" : "paint"));
					}}
					type="button"
				>
					{mode === "paint" ? (
						<PaintbrushIcon className="size-7" />
					) : (
						<HandIcon className="size-7" />
					)}
				</button>
			</div>
			<TerrainPalette brush={brush} onBrushChange={setBrush} />
			{tappedCell ? (
				<HexSheet
					campaignId={campaignId}
					cell={tappedCell}
					hex={displayed.byKey.get(hexKey(tappedCell.q, tappedCell.r))}
					key={`hex-sheet-${tapped.session}`}
					onOpenChange={tapped.onOpenChange}
					open={tapped.open}
				/>
			) : null}
			<FindSheet
				hexes={displayed.hexes}
				key={`find-sheet-${finding.session}`}
				onOpenChange={finding.onOpenChange}
				onPick={(hex) => {
					finding.onOpenChange(false);
					viewRef.current?.zoomToHex(hex);
				}}
				open={finding.open}
			/>
		</div>
	);
}

/** Find (spec §6.4): label search; picking a result zooms the map to that hex. */
function FindSheet({
	hexes,
	open,
	onOpenChange,
	onPick,
}: {
	hexes: readonly ChartedHex[];
	open: boolean;
	onOpenChange: (open: boolean) => void;
	onPick: (hex: ChartedHex) => void;
}) {
	const [search, setSearch] = useState("");
	const swatches = useTerrainSwatches();
	const matches = findHexesByLabel(hexes, search, findResultLimit);
	let results: ReactNode = null;
	if (search.trim() !== "" && matches.length === 0) {
		results = (
			<p className="text-base text-muted-foreground">No labelled hex matches “{search.trim()}”.</p>
		);
	} else if (matches.length > 0) {
		results = (
			<ul className="grid gap-1">
				{matches.map((hex) => (
					<li key={hexKey(hex.q, hex.r)}>
						<button
							className="flex min-h-11 w-full items-center gap-3 rounded-md px-2 py-2 text-left hover:bg-accent focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/45"
							onClick={() => {
								onPick(hex);
							}}
							type="button"
						>
							<TerrainSwatch className="size-6" swatches={swatches} terrain={hex.terrain} />
							<span className="min-w-0 flex-1 truncate">{hex.label}</span>
							<span className="text-muted-foreground text-sm">
								{terrainStyle(hex.terrain).label}
							</span>
						</button>
					</li>
				))}
			</ul>
		);
	}
	return (
		<BottomSheet
			description="Search the labels the party has written on the map."
			onOpenChange={onOpenChange}
			open={open}
			title="Find a hex"
		>
			<div className="grid gap-4">
				<Input
					aria-label="Label"
					autoComplete="off"
					autoFocus
					onChange={(event) => {
						setSearch(event.target.value);
					}}
					placeholder="Search labels"
					type="search"
					value={search}
				/>
				{results}
			</div>
		</BottomSheet>
	);
}
