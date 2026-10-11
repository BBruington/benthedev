import {
	ChevronLeftIcon,
	ChevronRightIcon,
	EllipsisVerticalIcon,
	PackageOpenIcon,
	PenLineIcon,
	PlusIcon,
	Trash2Icon,
} from "lucide-react";
import { type FormEvent, useState } from "react";
import type { Schemas } from "@/api/client";
import { TimelineEntry, type TimelineKind, timelineKinds } from "@/components/timeline-entry";
import { Button } from "@/components/ui/button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { type DayPage, dayName, daysOfPage, pageLabel, parseWholeNumber } from "@/lib/colony";
import { cn } from "@/lib/utils";

type Entry = Schemas["TimelineEntry"];

export const timelinePageHeadingId = "timeline-page-heading";

/** Earlier, the page's label, Later, Today and Jump to day. */
export function TimelineToolbar({
	page,
	earlier,
	later,
	isOnToday,
	onGo,
	onToday,
}: {
	page: DayPage;
	earlier: DayPage | null;
	later: DayPage | null;
	isOnToday: boolean;
	onGo: (day: number) => void;
	onToday: () => void;
}) {
	const [jumpText, setJumpText] = useState("");
	const jumpDay = parseWholeNumber(jumpText);
	const isJumpValid = typeof jumpDay === "number" && jumpDay >= 0;

	function handleJump(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (isJumpValid) {
			onGo(jumpDay);
			setJumpText("");
		}
	}

	return (
		<div className="flex flex-col gap-3 rounded-xl border border-border bg-card p-3 shadow-card sm:flex-row sm:items-center sm:justify-between sm:p-4">
			<div className="flex items-center gap-1">
				<Button
					aria-label="Earlier days"
					disabled={earlier === null}
					onClick={() => {
						if (earlier) {
							onGo(earlier.fromDay);
						}
					}}
					size="icon"
					variant="ghost"
				>
					<ChevronLeftIcon />
				</Button>
				<h2
					aria-live="polite"
					className="min-w-0 flex-1 text-center font-display font-semibold text-lg tabular-nums tracking-wide outline-none sm:min-w-[12rem] sm:text-xl"
					id={timelinePageHeadingId}
					tabIndex={-1}
				>
					{pageLabel(page)}
				</h2>
				<Button
					aria-label="Later days"
					disabled={later === null}
					onClick={() => {
						if (later) {
							onGo(later.fromDay);
						}
					}}
					size="icon"
					variant="ghost"
				>
					<ChevronRightIcon />
				</Button>
			</div>
			<div className="flex items-center gap-2">
				<Button disabled={isOnToday} onClick={onToday} size="sm" variant="outline">
					Today
				</Button>
				<form className="flex min-w-0 flex-1 items-center gap-2" onSubmit={handleJump}>
					<Label className="sr-only" htmlFor="timeline-jump">
						Jump to day
					</Label>
					<Input
						autoComplete="off"
						className="h-11 min-w-0 sm:w-36"
						id="timeline-jump"
						inputMode="numeric"
						onChange={(event) => {
							setJumpText(event.target.value);
						}}
						placeholder="Jump to day"
						value={jumpText}
					/>
					<Button disabled={!isJumpValid} size="sm" type="submit" variant="secondary">
						Go
					</Button>
				</form>
			</div>
		</div>
	);
}

const allKinds = Object.keys(timelineKinds).filter((kind): kind is TimelineKind =>
	Object.hasOwn(timelineKinds, kind),
);

/** The DM's filter chips by kind and the Hide DM-only switch (spec §5.8). */
export function TimelineFilters({
	shownKinds,
	onToggleKind,
	hideDmOnly,
	onHideDmOnlyChange,
}: {
	/** Empty means every kind. */
	shownKinds: ReadonlySet<TimelineKind>;
	onToggleKind: (kind: TimelineKind) => void;
	hideDmOnly: boolean;
	onHideDmOnlyChange: (hide: boolean) => void;
}) {
	return (
		<div className="flex flex-col gap-3">
			<fieldset className="flex flex-wrap gap-2">
				<legend className="sr-only">Show only these kinds</legend>
				{allKinds.map((kind) => {
					const glyph = timelineKinds[kind];
					const isOn = shownKinds.has(kind);
					return (
						<button
							aria-pressed={isOn}
							className={cn(
								"inline-flex min-h-11 cursor-pointer items-center gap-1.5 rounded-full border px-3 text-sm outline-none transition-colors focus-visible:ring-[3px] focus-visible:ring-ring/45",
								isOn
									? "border-gilt bg-gilt/15 font-medium text-gilt-ink"
									: "border-border bg-surface/60 text-muted-foreground hover:text-foreground",
							)}
							key={kind}
							onClick={() => {
								onToggleKind(kind);
							}}
							type="button"
						>
							<glyph.icon aria-hidden="true" className="size-4" />
							{glyph.label}
						</button>
					);
				})}
			</fieldset>
			<label
				className="inline-flex min-h-11 w-fit cursor-pointer items-center gap-2.5 text-sm"
				htmlFor="timeline-hide-dm-only"
			>
				<Switch
					checked={hideDmOnly}
					id="timeline-hide-dm-only"
					onCheckedChange={onHideDmOnlyChange}
				/>
				Hide DM-only
			</label>
		</div>
	);
}

export type EntryActions = {
	onAdd: (day: number) => void;
	onEdit: (entry: Entry) => void;
	onDelete: (entry: Entry) => void;
	onRecordCargo: (entry: Entry) => void;
};

export function dayHeadingId(day: number): string {
	return `timeline-day-${day}`;
}

/** Every day of the page in order, today ringed, with its entries (spec §5.8). */
export function TimelineDays({
	page,
	today,
	entries,
	actions,
}: {
	page: DayPage;
	today: number;
	entries: readonly Entry[];
	/** The DM's controls; absent for players. */
	actions: EntryActions | null;
}) {
	const byDay = new Map<number, Entry[]>();
	for (const entry of entries) {
		const list = byDay.get(entry.day) ?? [];
		list.push(entry);
		byDay.set(entry.day, list);
	}
	return (
		<ol className="flex flex-col gap-3">
			{daysOfPage(page).map((day) => {
				const dayEntries = byDay.get(day) ?? [];
				const isToday = day === today;
				return (
					<li
						aria-current={isToday ? "date" : undefined}
						className={cn(
							"rounded-xl border bg-card shadow-card",
							isToday ? "border-gilt ring-2 ring-gilt/45" : "border-border",
							dayEntries.length === 0 && !isToday && "bg-card/50 shadow-none",
						)}
						key={day}
					>
						<div className="flex items-center gap-3 px-4 py-2">
							<h3
								className={cn(
									"flex flex-1 items-center gap-2 font-display font-semibold tracking-wide outline-none",
									isToday ? "text-gilt-ink" : "text-foreground",
									dayEntries.length === 0 && !isToday && "text-muted-foreground",
								)}
								id={dayHeadingId(day)}
								tabIndex={-1}
							>
								{dayName(day)}
								{day === 0 ? (
									<span className="font-sans font-normal text-muted-foreground text-sm">Day 0</span>
								) : null}
								{isToday ? (
									<span className="rounded-full bg-gilt px-2 py-0.5 font-sans font-semibold text-on-gilt text-xs uppercase tracking-[0.12em]">
										Today
									</span>
								) : null}
							</h3>
							{actions ? (
								<Button
									aria-label={`Add an entry on ${dayName(day)}`}
									onClick={() => {
										actions.onAdd(day);
									}}
									size="icon-sm"
									variant="ghost"
								>
									<PlusIcon />
								</Button>
							) : null}
						</div>
						{dayEntries.length > 0 ? (
							<ul className="grid gap-4 border-border border-t px-4 py-4">
								{dayEntries.map((entry) => (
									<li key={entry.id}>
										<TimelineEntry
											actions={actions ? <EntryMenu actions={actions} entry={entry} /> : null}
											entry={entry}
										/>
									</li>
								))}
							</ul>
						) : null}
					</li>
				);
			})}
		</ol>
	);
}

function EntryMenu({ entry, actions }: { entry: Entry; actions: EntryActions }) {
	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button aria-label={`More for ${entry.title}`} size="icon-sm" variant="ghost">
					<EllipsisVerticalIcon />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end">
				<DropdownMenuItem
					onSelect={() => {
						actions.onEdit(entry);
					}}
				>
					<PenLineIcon />
					Edit…
				</DropdownMenuItem>
				{entry.dm?.canRecordCargo ? (
					<DropdownMenuItem
						onSelect={() => {
							actions.onRecordCargo(entry);
						}}
					>
						<PackageOpenIcon />
						Record cargo…
					</DropdownMenuItem>
				) : null}
				{entry.dm?.canDelete ? (
					<DropdownMenuItem
						onSelect={() => {
							actions.onDelete(entry);
						}}
						variant="destructive"
					>
						<Trash2Icon />
						Delete…
					</DropdownMenuItem>
				) : null}
			</DropdownMenuContent>
		</DropdownMenu>
	);
}

/** Applies the DM's filters; with no kind chosen, every kind shows. */
export function filterEntries(
	entries: readonly Entry[],
	shownKinds: ReadonlySet<TimelineKind>,
	hideDmOnly: boolean,
): Entry[] {
	return entries.filter((entry) => {
		if (shownKinds.size > 0 && !shownKinds.has(entry.kind)) {
			return false;
		}
		if (hideDmOnly && entry.dm?.visibility === "DM") {
			return false;
		}
		return true;
	});
}
