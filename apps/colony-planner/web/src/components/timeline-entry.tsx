import { Link } from "@tanstack/react-router";
import {
	BadgeCheckIcon,
	CalendarClockIcon,
	CircleXIcon,
	CoinsIcon,
	CrownIcon,
	DicesIcon,
	EyeOffIcon,
	FeatherIcon,
	HandCoinsIcon,
	HourglassIcon,
	type LucideIcon,
	PackageOpenIcon,
	RotateCcwIcon,
	ScaleIcon,
	ScrollTextIcon,
	ShipIcon,
	StoreIcon,
	SunriseIcon,
	Undo2Icon,
	UserMinusIcon,
	UserPlusIcon,
	UsersIcon,
	XIcon,
} from "lucide-react";
import type { ReactNode } from "react";
import type { Schemas } from "@/api/client";
import { useCampaign } from "@/hooks/use-campaign";
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
	FAVOR_ADJUSTED: { icon: CrownIcon, label: "Favor", tone: "bg-gilt/12 text-gilt-ink" },
	MARKET_ORDER_CANCELLED: {
		icon: XIcon,
		label: "Order cancelled",
		tone: "bg-muted text-muted-foreground",
	},
	MARKET_ORDER_DOCKED: {
		icon: ShipIcon,
		label: "Goods ship docked",
		tone: "bg-verdigris/14 text-verdigris-ink",
	},
	MARKET_ORDER_PAID: { icon: CoinsIcon, label: "Goods unloaded", tone: "bg-moss/14 text-moss-ink" },
	MARKET_ORDER_PLACED: { icon: StoreIcon, label: "Order placed", tone: "bg-gilt/12 text-gilt-ink" },
	PATRON_ACTION: {
		icon: HandCoinsIcon,
		label: "Patron's favor",
		tone: "bg-verdigris/14 text-verdigris-ink",
	},
	QUEST_COMPLETED: {
		icon: BadgeCheckIcon,
		label: "Quest done",
		tone: "bg-moss/14 text-moss-ink",
	},
	QUEST_FAILED: { icon: CircleXIcon, label: "Quest failed", tone: "bg-ember/12 text-ember-ink" },
	QUEST_POSTED: { icon: ScrollTextIcon, label: "New quest", tone: "bg-gilt/12 text-gilt-ink" },
	QUEST_REOPENED: {
		icon: RotateCcwIcon,
		label: "Quest reopened",
		tone: "bg-muted text-muted-foreground",
	},
	SESSION_PLAYED: {
		icon: DicesIcon,
		label: "Session played",
		tone: "bg-gilt/12 text-gilt-ink",
	},
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

const entryLinkClasses =
	"underline decoration-gilt-ink/50 underline-offset-4 hover:decoration-gilt-ink";

/**
 * The entry's title, linking to the NPC, quest, patron or session it is about. The server sends those IDs to a
 * player only while the row is visible to them, so a link never points at something hidden.
 */
function EntryTitle({ entry, slug }: { entry: Entry; slug: string }) {
	if (entry.npcId) {
		return (
			<Link
				className={entryLinkClasses}
				params={{ campaignSlug: slug, npcId: entry.npcId }}
				to="/c/$campaignSlug/npcs/$npcId"
			>
				{entry.title}
			</Link>
		);
	}
	if (entry.questId) {
		return (
			<Link
				className={entryLinkClasses}
				params={{ campaignSlug: slug, questId: entry.questId }}
				to="/c/$campaignSlug/quests/$questId"
			>
				{entry.title}
			</Link>
		);
	}
	if (entry.patronId) {
		return (
			<Link
				className={entryLinkClasses}
				params={{ campaignSlug: slug, patronId: entry.patronId }}
				to="/c/$campaignSlug/patrons/$patronId"
			>
				{entry.title}
			</Link>
		);
	}
	if (entry.sessionId) {
		return (
			<Link
				className={entryLinkClasses}
				params={{ campaignSlug: slug, sessionId: entry.sessionId }}
				to="/c/$campaignSlug/sessions/$sessionId"
			>
				{entry.title}
			</Link>
		);
	}
	return entry.title;
}

/**
 * One timeline entry with its glyph, title and narration. For the DM, a DM-only entry carries an
 * eye-slash and an undone one is struck through (spec §5.8); players never receive either. An
 * entry about an NPC, a quest, a patron or a session links its title to that page.
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
	const { slug } = useCampaign();
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
					<EntryTitle entry={entry} slug={slug} />
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
				{entry.dm?.note ? (
					<p className="mt-0.5 inline-flex items-start gap-1 whitespace-pre-line text-sm text-verdigris-ink [overflow-wrap:anywhere]">
						<EyeOffIcon aria-label="Only you see this" className="mt-0.5 size-3.5 shrink-0" />
						{entry.dm.note}
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
