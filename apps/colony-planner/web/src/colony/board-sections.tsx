import { Link } from "@tanstack/react-router";
import {
	ArrowRightIcon,
	CalendarClockIcon,
	HourglassIcon,
	PlusIcon,
	ScaleIcon,
	ScrollTextIcon,
	Undo2Icon,
	UsersIcon,
} from "lucide-react";
import type { ReactNode } from "react";
import type { Schemas } from "@/api/client";
import { ColonyIcon, colonyIcons } from "@/components/colony-icon";
import { StatTile } from "@/components/stat-tile";
import { TimelineEntry } from "@/components/timeline-entry";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useCampaign } from "@/hooks/use-campaign";
import { colonistsLine, formatNumber } from "@/lib/colony";
import { formatRelativeTime } from "@/lib/time";
import { cn } from "@/lib/utils";

type ColonyBoard = Schemas["ColonyBoard"];
type ColonyResource = Schemas["ColonyResource"];
type TimelineEntryData = Schemas["TimelineEntry"];

type Tone = "gilt" | "verdigris" | "moss" | "ember" | "ink";

// The seeded resources keep the palette's meanings (spec §6.6: moss is food); a resource the DM adds
// takes gilt until it earns a colour of its own.
const resourceTones: Record<string, Tone> = {
	food: "moss",
	gold: "gilt",
	materials: "ink",
	morale: "verdigris",
};

function resourceCaption(resource: ColonyResource): string | undefined {
	if (resource.maxAmount !== null) {
		return `out of ${formatNumber(resource.maxAmount)}`;
	}
	if (resource.minAmount !== 0) {
		return `at least ${formatNumber(resource.minAmount)}`;
	}
	return undefined;
}

/** The stockpile: one tile per resource, counting to new values with a delta chip (spec §6.3). */
export function ResourceTiles({ resources }: { resources: readonly ColonyResource[] }) {
	if (resources.length === 0) {
		return (
			<p className="rounded-xl border border-border border-dashed px-4 py-6 text-center text-muted-foreground">
				The colony keeps no stock yet.
			</p>
		);
	}
	return (
		<div className="grid grid-cols-2 gap-3 md:gap-4 lg:grid-cols-4">
			{resources.map((resource) => (
				<StatTile
					caption={resourceCaption(resource)}
					icon={colonyIcons[resource.icon]}
					key={resource.key}
					label={resource.label}
					tone={resourceTones[resource.key] ?? "gilt"}
					value={resource.amount}
				/>
			))}
		</div>
	);
}

/** "62 colonists · 5 unassigned", with each role's headcount; Unassigned stands out when > 0. */
export function ColonistsStrip({ board }: { board: ColonyBoard }) {
	const roles = board.roles.filter((role) => role.total > 0 || role.isSystem);
	return (
		<section
			aria-labelledby="colonists-heading"
			className="flex flex-col gap-3 rounded-xl border border-border bg-card p-4 shadow-card sm:p-5"
		>
			<div className="flex items-center gap-3">
				<span
					aria-hidden="true"
					className="inline-flex size-9 shrink-0 items-center justify-center rounded-lg bg-verdigris/14 text-verdigris-ink"
				>
					<UsersIcon className="size-[1.125rem]" />
				</span>
				<h2 className="font-semibold text-lg leading-snug" id="colonists-heading">
					{colonistsLine(board.colonists, board.unassigned)}
				</h2>
			</div>
			{roles.length > 0 ? (
				<ul aria-label="Colonists by role" className="flex flex-wrap gap-2">
					{roles.map((role) => {
						const isUnassignedWaiting = role.isSystem && role.total > 0;
						return (
							<li
								className={cn(
									"inline-flex min-h-9 items-center gap-1.5 rounded-full border px-3 py-1 text-sm",
									isUnassignedWaiting
										? "border-gilt/55 bg-gilt/12 font-medium text-gilt-ink"
										: "border-border bg-surface-sunken/50",
								)}
								key={role.key}
							>
								<ColonyIcon className="size-4 shrink-0" name={role.icon} />
								{role.label}
								<span className="font-semibold tabular-nums">{formatNumber(role.total)}</span>
							</li>
						);
					})}
				</ul>
			) : null}
		</section>
	);
}

/** The newest five timeline entries, and the way to the rest. */
export function RecentEntries({
	entries,
	isPending,
	isError,
}: {
	entries: readonly TimelineEntryData[] | undefined;
	isPending: boolean;
	isError: boolean;
}) {
	const { slug } = useCampaign();
	let body: ReactNode;
	if (isPending) {
		body = (
			<div aria-busy="true" className="grid gap-3">
				<Skeleton className="h-14 rounded-lg" />
				<Skeleton className="h-14 rounded-lg" />
			</div>
		);
	} else if (isError || !entries) {
		body = <p className="text-muted-foreground">The timeline couldn't be loaded.</p>;
	} else if (entries.length === 0) {
		body = (
			<p className="font-flavor text-muted-foreground italic">
				Nothing written yet. The ledger opens with the first day.
			</p>
		);
	} else {
		body = (
			<ul className="grid gap-4">
				{entries.slice(0, 5).map((entry) => (
					<li key={entry.id}>
						<TimelineEntry entry={entry} showDay />
					</li>
				))}
			</ul>
		);
	}
	return (
		<section
			aria-labelledby="recent-entries-heading"
			className="flex flex-col gap-4 rounded-xl border border-border bg-card p-4 shadow-card sm:p-5"
		>
			<div className="flex items-center justify-between gap-3">
				<h2
					className="flex items-center gap-2 font-display font-semibold text-xl tracking-wide"
					id="recent-entries-heading"
				>
					<ScrollTextIcon aria-hidden="true" className="size-5 text-gilt-ink" />
					Latest from the timeline
				</h2>
				<Button asChild size="sm" variant="ghost">
					<Link params={{ campaignSlug: slug }} to="/c/$campaignSlug/timeline">
						Timeline
						<ArrowRightIcon aria-hidden="true" />
					</Link>
				</Button>
			</div>
			{body}
		</section>
	);
}

export const advanceChipDays = [1, 3, 7] as const;

/** The DM's tools on the board (spec §6.2); players and View as player never see them. */
export function DmTools({
	board,
	onAdvance,
	onAdjust,
	onShipments,
	onCorrectDay,
	onUndo,
	isUndoing,
}: {
	board: ColonyBoard;
	onAdvance: (days: number | null) => void;
	onAdjust: () => void;
	onShipments: () => void;
	onCorrectDay: () => void;
	onUndo: () => void;
	isUndoing: boolean;
}) {
	const lastChange = board.dm?.lastChange ?? null;
	return (
		<div className="grid gap-4">
			<fieldset className="grid gap-2">
				<legend className="mb-2 font-medium text-muted-foreground text-xs uppercase tracking-[0.18em]">
					Advance the clock
				</legend>
				<div className="grid grid-cols-4 gap-2">
					{advanceChipDays.map((days) => (
						<Button
							className="px-2 tabular-nums"
							id={`advance-chip-${days}`}
							key={days}
							onClick={() => {
								onAdvance(days);
							}}
							variant="outline"
						>
							+{days}
							<span className="sr-only">{days === 1 ? " day" : " days"}</span>
						</Button>
					))}
					<Button
						className="px-2"
						id="advance-chip-custom"
						onClick={() => {
							onAdvance(null);
						}}
						variant="outline"
					>
						<PlusIcon className="hidden sm:inline" />
						Custom
						<span className="sr-only"> number of days</span>
					</Button>
				</div>
			</fieldset>
			<div className="flex flex-wrap gap-2">
				<Button onClick={onAdjust} size="sm" variant="secondary">
					<ScaleIcon />
					Adjust stock
				</Button>
				<Button onClick={onShipments} size="sm" variant="outline">
					<CalendarClockIcon />
					Shipments
				</Button>
				<Button onClick={onCorrectDay} size="sm" variant="outline">
					<HourglassIcon />
					Correct day
				</Button>
			</div>
			{lastChange ? (
				<div className="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-lg border border-border bg-surface-sunken/50 px-3 py-2">
					<p className="min-w-0 flex-1 text-sm">
						<span className="text-muted-foreground">Last change: </span>
						<span className="font-medium">{lastChange.label}</span>{" "}
						<span className="text-muted-foreground">
							· {formatRelativeTime(lastChange.changedAt)}
						</span>
					</p>
					<Button
						disabled={isUndoing}
						id="undo-last-change"
						onClick={onUndo}
						size="sm"
						variant="ghost-destructive"
					>
						<Undo2Icon />
						Undo
					</Button>
				</div>
			) : (
				<p className="flex items-center gap-2 text-muted-foreground text-sm">
					<Badge variant="muted">Undo</Badge>
					Nothing to undo since the last change.
				</p>
			)}
		</div>
	);
}
