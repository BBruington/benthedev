import {
	CalendarClockIcon,
	EyeOffIcon,
	FeatherIcon,
	HourglassIcon,
	type LucideIcon,
	PackageOpenIcon,
	ScaleIcon,
	ShipIcon,
	SunriseIcon,
	Undo2Icon,
	UserMinusIcon,
	UserPlusIcon,
	UsersIcon,
} from "lucide-react";
import type { ReactNode } from "react";
import type { Schemas } from "@/api/client";
import { dayName } from "@/lib/colony";
import { cn } from "@/lib/utils";

type Entry = Schemas["TimelineEntry"];
export type TimelineKind = Schemas["TimelineKind"];

type Glyph = { icon: LucideIcon; label: string; tone: string };

/** Each kind's typed glyph (spec §6.6); the Record fails typechecking when a kind is added. */
export const timelineKinds: Record<TimelineKind, Glyph> = {
	CARGO_RECORDED: { icon: PackageOpenIcon, label: "Cargo", tone: "bg-moss/14 text-moss-ink" },
	COLONIST_JOINED: {
		icon: UserPlusIcon,
		label: "Colonist joined",
		tone: "bg-verdigris/14 text-verdigris-ink",
	},
	COLONIST_LEFT: {
		icon: UserMinusIcon,
		label: "Colonist left",
		tone: "bg-ember/12 text-ember-ink",
	},
	CUSTOM: { icon: FeatherIcon, label: "Note", tone: "bg-gilt/12 text-gilt-ink" },
	DAY_CORRECTED: {
		icon: HourglassIcon,
		label: "Day corrected",
		tone: "bg-muted text-muted-foreground",
	},
	DAYS_ADVANCED: { icon: SunriseIcon, label: "Days pass", tone: "bg-gilt/12 text-gilt-ink" },
	SHIPMENT_ARRIVED: {
		icon: ShipIcon,
		label: "Ship arrived",
		tone: "bg-verdigris/14 text-verdigris-ink",
	},
	SHIPMENT_SCHEDULE_CHANGED: {
		icon: CalendarClockIcon,
		label: "Shipments",
		tone: "bg-verdigris/14 text-verdigris-ink",
	},
	STOCK_ADJUSTED: { icon: ScaleIcon, label: "Stock adjusted", tone: "bg-gilt/12 text-gilt-ink" },
	UNDO: { icon: Undo2Icon, label: "Undo", tone: "bg-ember/12 text-ember-ink" },
	WORKFORCE_CHANGED: {
		icon: UsersIcon,
		label: "Workforce",
		tone: "bg-verdigris/14 text-verdigris-ink",
	},
};

/**
 * One timeline entry with its glyph, title and narration. For the DM, a DM-only entry carries an
 * eye-slash and an undone one is struck through (spec §5.8); players never receive either.
 */
export function TimelineEntry({
	entry,
	showDay = false,
	actions,
	className,
}: {
	entry: Entry;
	/** Name the day in the meta line, for lists that aren't grouped by day. */
	showDay?: boolean;
	/** The DM's controls for this entry. */
	actions?: ReactNode;
	className?: string;
}) {
	const glyph = timelineKinds[entry.kind];
	const isDmOnly = entry.dm?.visibility === "DM";
	const isUndone = Boolean(entry.dm?.undoneAt);
	const meta: string[] = [];
	if (showDay) {
		meta.push(dayName(entry.day));
	}
	meta.push(glyph.label);
	if (entry.dm?.editedAt) {
		meta.push("Edited");
	}
	return (
		<article
			className={cn("flex items-start gap-3", isUndone && "opacity-70", className)}
			data-slot="timeline-entry"
		>
			<span
				aria-hidden="true"
				className={cn(
					"mt-0.5 inline-flex size-9 shrink-0 items-center justify-center rounded-full",
					glyph.tone,
				)}
			>
				<glyph.icon className="size-[1.125rem]" />
			</span>
			<div className="min-w-0 flex-1">
				<h3
					className={cn(
						"font-semibold text-base leading-snug [overflow-wrap:anywhere]",
						isUndone && "line-through decoration-ember-ink/70",
					)}
				>
					{entry.title}
				</h3>
				{entry.body ? (
					<p
						className={cn(
							"mt-0.5 whitespace-pre-line font-flavor text-muted-foreground italic [overflow-wrap:anywhere]",
							isUndone && "line-through decoration-ember-ink/50",
						)}
					>
						{entry.body}
					</p>
				) : null}
				<p className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-muted-foreground text-xs uppercase tracking-[0.12em]">
					<span>{meta.join(" · ")}</span>
					{isDmOnly ? (
						<span className="inline-flex items-center gap-1 text-verdigris-ink normal-case tracking-normal">
							<EyeOffIcon aria-hidden="true" className="size-3.5" />
							Hidden from players
						</span>
					) : null}
					{isUndone ? (
						<span className="text-ember-ink normal-case tracking-normal">Undone</span>
					) : null}
				</p>
			</div>
			{actions ? <div className="-mt-1 shrink-0">{actions}</div> : null}
		</article>
	);
}
