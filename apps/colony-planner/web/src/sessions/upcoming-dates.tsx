import { Link } from "@tanstack/react-router";
import { BadgeCheckIcon, CalendarXIcon, ChevronDownIcon, RotateCcwIcon } from "lucide-react";
import { useEffect, useId, useState } from "react";
import type { Schemas } from "@/api/client";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useCampaign } from "@/hooks/use-campaign";
import {
	countdownLabel,
	durationLabel,
	localDateLabel,
	planUpcoming,
	sessionDateKey,
	zonedTime,
} from "@/lib/sessions";
import { cn } from "@/lib/utils";
import { SessionWhen } from "./session-parts";

type Session = Schemas["Session"];

/**
 * Cancel, restore and mark played act on a date, which may not have a stored row yet. Their buttons
 * are aria-disabled rather than disabled while one runs: disabling the focused button would drop focus.
 */
export type DateActions = {
	isPending: boolean;
	cancel: (session: Session) => void;
	restore: (session: Session) => void;
	markPlayed: (session: Session) => void;
};

const minuteInMilliseconds = 60 * 1000;
const dayInMilliseconds = 24 * 60 * minuteInMilliseconds;

/** The date's cancel or restore button, where focus returns after either. */
export function dateToggleId(session: Session): string {
	return `session-toggle-${sessionDateKey(session)}`;
}

/**
 * The upcoming window as one next-game card, the few dates after it and the rest behind a
 * disclosure, so a weekly series doesn't push the history and the schedule off the screen.
 */
export function UpcomingDates({
	sessions,
	startLocalOf,
	tableTimeZone,
	actions,
}: {
	sessions: Session[];
	startLocalOf: (seriesId: string | null) => string | undefined;
	tableTimeZone: string | undefined;
	/** The DM's date controls; absent for players and view as player. */
	actions: DateActions | null;
}) {
	const [showAll, setShowAll] = useState(false);
	const laterId = useId();
	const now = useMinuteClock();
	const plan = planUpcoming(sessions);
	return (
		<div className="flex flex-col gap-3">
			{plan.next ? (
				<NextGameCard
					actions={actions}
					now={now}
					session={plan.next}
					startLocal={startLocalOf(plan.next.seriesId)}
					tableTimeZone={tableTimeZone}
				/>
			) : (
				<p className="rounded-xl border border-border border-dashed px-4 py-3 text-muted-foreground">
					No game night ahead in the next 12 weeks.
				</p>
			)}
			{plan.comingUp.length > 0 ? (
				<DateList actions={actions} now={now} sessions={plan.comingUp} />
			) : null}
			{plan.later.length > 0 ? (
				<>
					<Button
						aria-controls={laterId}
						aria-expanded={showAll}
						className="self-start"
						onClick={() => {
							setShowAll(!showAll);
						}}
						size="sm"
						variant="outline"
					>
						<ChevronDownIcon className={cn("transition-transform", showAll && "rotate-180")} />
						{showAll ? "Show fewer" : `Show all ${sessions.length} dates`}
					</Button>
					<div className="space-y-4" hidden={!showAll} id={laterId}>
						{plan.later.map((month) => (
							<section
								aria-labelledby={`${laterId}-${month.key}`}
								className="flex flex-col gap-2"
								key={month.key}
							>
								<h3
									className="font-semibold text-muted-foreground text-sm uppercase tracking-wider"
									id={`${laterId}-${month.key}`}
								>
									{month.label}
								</h3>
								<DateList actions={actions} now={now} sessions={month.sessions} />
							</section>
						))}
					</div>
				</>
			) : null}
		</div>
	);
}

/** The countdown and the Mark played check move on while the page stays open; queries never go stale. */
function useMinuteClock(): Date {
	const [now, setNow] = useState(() => new Date());
	useEffect(() => {
		const timer = setInterval(() => {
			setNow(new Date());
		}, minuteInMilliseconds);
		return () => {
			clearInterval(timer);
		};
	}, []);
	return now;
}

function SessionTitle({ session }: { session: Session }) {
	const { slug } = useCampaign();
	if (session.id === null) {
		return session.title;
	}
	return (
		<Link
			className="underline-offset-4 hover:underline"
			params={{ campaignSlug: slug, sessionId: session.id }}
			to="/c/$campaignSlug/sessions/$sessionId"
		>
			{session.title}
		</Link>
	);
}

function NextGameCard({
	session,
	startLocal,
	tableTimeZone,
	actions,
	now,
}: {
	session: Session;
	startLocal: string | undefined;
	tableTimeZone: string | undefined;
	actions: DateActions | null;
	now: Date;
}) {
	return (
		<div className="flex flex-col gap-3 rounded-xl border border-gilt/60 bg-card px-4 py-4 shadow-card sm:flex-row sm:items-end sm:justify-between sm:px-5">
			<div className="min-w-0 space-y-1.5">
				<p className="flex flex-wrap items-center gap-2">
					<Badge variant="gilt">Next game</Badge>
					<span className="font-semibold text-gilt-ink text-sm">
						{countdownLabel(session.startsAt, now)}
					</span>
				</p>
				<h3 className="font-display font-semibold text-xl tracking-wide [overflow-wrap:anywhere]">
					<SessionTitle session={session} />
				</h3>
				<SessionWhen session={session} startLocal={startLocal} tableTimeZone={tableTimeZone} />
				<p className="text-muted-foreground text-sm">{durationLabel(session.durationMinutes)}</p>
			</div>
			{actions && session.seriesId ? (
				<div className="flex flex-wrap gap-2">
					<Button
						aria-disabled={actions.isPending}
						onClick={() => {
							if (!actions.isPending) {
								actions.markPlayed(session);
							}
						}}
						size="sm"
					>
						<BadgeCheckIcon />
						Mark played
					</Button>
					<DateToggle actions={actions} session={session} />
				</div>
			) : null}
		</div>
	);
}

function DateList({
	sessions,
	actions,
	now,
}: {
	sessions: Session[];
	actions: DateActions | null;
	now: Date;
}) {
	return (
		<ul className="divide-y divide-border rounded-xl border border-border bg-card shadow-card">
			{sessions.map((session) => (
				<DateRow actions={actions} key={sessionDateKey(session)} now={now} session={session} />
			))}
		</ul>
	);
}

function DateRow({
	session,
	actions,
	now,
}: {
	session: Session;
	actions: DateActions | null;
	now: Date;
}) {
	const viewer = zonedTime(session.startsAt);
	const isCancelled = session.status === "CANCELLED";
	const isSoon = Date.parse(session.startsAt) - now.getTime() < dayInMilliseconds;
	const strike = isCancelled && "line-through decoration-ember-ink/60";
	return (
		<li className="flex items-center gap-3 px-3 py-2">
			<span
				className={cn(
					"min-w-24 shrink-0 whitespace-nowrap rounded-md border border-border bg-muted px-1.5 py-1 text-center font-semibold text-sm tabular-nums",
					isCancelled && "text-muted-foreground",
					strike,
				)}
			>
				{viewer.date}
			</span>
			<p
				className={cn(
					"flex min-w-0 flex-1 flex-wrap items-center gap-x-2 gap-y-0.5",
					isCancelled && "text-muted-foreground",
				)}
			>
				<span className={cn("font-semibold [overflow-wrap:anywhere]", strike)}>
					<SessionTitle session={session} />
				</span>
				<time className={cn("text-muted-foreground text-sm", strike)} dateTime={session.startsAt}>
					{viewer.time} {viewer.zone}
				</time>
				{isCancelled ? <Badge variant="ember">Cancelled</Badge> : null}
				{session.status === "PLAYED" ? <Badge variant="moss">Played</Badge> : null}
			</p>
			{actions && session.seriesId && session.status !== "PLAYED" ? (
				<div className="flex shrink-0 gap-1">
					{session.status === "SCHEDULED" && isSoon ? (
						<Button
							aria-disabled={actions.isPending}
							aria-label={`Mark played: ${session.title} on ${localDateLabel(session.localDate)}`}
							className="pointer-coarse:min-w-11"
							onClick={() => {
								if (!actions.isPending) {
									actions.markPlayed(session);
								}
							}}
							size="sm"
							variant="ghost"
						>
							<BadgeCheckIcon />
							<span className="max-sm:sr-only">Mark played</span>
						</Button>
					) : null}
					<DateToggle actions={actions} compact session={session} />
				</div>
			) : null}
		</li>
	);
}

/** Cancel or restore; compact rows keep the 44 px touch target but show only the icon on phones. */
function DateToggle({
	session,
	actions,
	compact = false,
}: {
	session: Session;
	actions: DateActions;
	compact?: boolean;
}) {
	const isCancelled = session.status === "CANCELLED";
	const label = isCancelled ? "Restore" : "Cancel";
	return (
		<Button
			aria-disabled={actions.isPending}
			aria-label={`${label} ${session.title} on ${localDateLabel(session.localDate)}`}
			className={cn(compact && "pointer-coarse:min-w-11")}
			id={dateToggleId(session)}
			onClick={() => {
				if (actions.isPending) {
					return;
				}
				if (isCancelled) {
					actions.restore(session);
				} else {
					actions.cancel(session);
				}
			}}
			size="sm"
			variant={isCancelled ? "outline" : "ghost-destructive"}
		>
			{isCancelled ? <RotateCcwIcon /> : <CalendarXIcon />}
			<span className={cn(compact && "max-sm:sr-only")}>{label}</span>
		</Button>
	);
}
