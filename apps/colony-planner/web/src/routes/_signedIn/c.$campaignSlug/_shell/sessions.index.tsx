import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { CalendarDaysIcon, PenLineIcon, PlusIcon, Trash2Icon } from "lucide-react";
import { type ReactNode, useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { problemMessage } from "@/api/problem";
import {
	playedSessionsQuery,
	sessionQuery,
	sessionSeriesQuery,
	settingsQuery,
	upcomingSessionsQuery,
} from "@/api/queries";
import { PageHeader } from "@/components/page-header";
import { StatusPanel } from "@/components/status-panel";
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { fallbackCampaignName, useCampaign, useCampaignName } from "@/hooks/use-campaign";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { useMe } from "@/hooks/use-me";
import { useSheetState } from "@/hooks/use-sheet-state";
import { cachedMe, canUseDmTools } from "@/lib/access";
import { focusFirstVisible } from "@/lib/focus";
import {
	cadenceSummary,
	clockLabel,
	colonyDaysLabel,
	durationLabel,
	localDateLabel,
	sessionDateKey,
	timezoneCity,
} from "@/lib/sessions";
import { SeriesSheet, useSessionsSettled } from "@/sessions/series-sheet";
import { sessionHeadingId, sessionsPageHeadingId } from "@/sessions/session-ids";
import { CalendarLinkCard, useTableTimeZone } from "@/sessions/session-parts";
import { type DateActions, dateToggleId, UpcomingDates } from "@/sessions/upcoming-dates";

type Session = Schemas["Session"];
type SessionSeries = Schemas["SessionSeries"];

export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell/sessions/")({
	component: SessionsPage,
	loader: ({ context: { campaignId, queryClient } }) => {
		if (!campaignId) {
			return;
		}
		queryClient.prefetchQuery(upcomingSessionsQuery(campaignId));
		queryClient.prefetchQuery(playedSessionsQuery(campaignId));
		queryClient.prefetchQuery(sessionSeriesQuery(campaignId));
		const me = cachedMe(queryClient, campaignId);
		if (me && canUseDmTools(me)) {
			queryClient.prefetchQuery(settingsQuery(campaignId));
		}
	},
});

/** ", New York time" when the zone is known; players get the table's offset beside each date instead. */
function tableZoneSuffix(tableTimeZone: string | undefined, separator = ", "): string {
	return tableTimeZone ? `${separator}${timezoneCity(tableTimeZone)} time` : "";
}

function seriesDatesLabel(series: SessionSeries): string {
	if (series.everyWeeks === null) {
		return localDateLabel(series.firstDate);
	}
	const from = `From ${localDateLabel(series.firstDate)}`;
	return series.lastDate ? `${from} to ${localDateLabel(series.lastDate)}` : from;
}

/** /sessions (spec §6.2, §5.11): upcoming game nights, the history with recaps, and the ICS link. */
function SessionsPage() {
	useDocumentTitle("Sessions", useCampaignName() ?? fallbackCampaignName);
	const me = useMe();
	const isDm = canUseDmTools(me);
	const { campaignId } = useCampaign();
	const tableTimeZone = useTableTimeZone();
	const series = useQuery(sessionSeriesQuery(campaignId));
	const editing = useSheetState<SessionSeries | "new">();
	const startLocalOf = (seriesId: string | null) =>
		series.data?.find((candidate) => candidate.id === seriesId)?.startLocal;

	return (
		<div className="flex flex-col gap-8">
			<PageHeader
				actions={
					isDm ? (
						<Button
							onClick={() => {
								editing.show("new");
							}}
						>
							<PlusIcon />
							Schedule
						</Button>
					) : null
				}
				description={
					isDm
						? `Game nights run on the table's clock${tableZoneSuffix(tableTimeZone)}. Everyone sees each date in their own time zone.`
						: "Upcoming game nights in your own time, and what happened at past ones."
				}
				headingId={sessionsPageHeadingId}
				title="Sessions"
			/>
			<UpcomingSessions isDm={isDm} startLocalOf={startLocalOf} tableTimeZone={tableTimeZone} />
			<PlayedSessions />
			{isDm ? (
				<SeriesList
					onEdit={(chosen) => {
						editing.show(chosen);
					}}
					series={series}
					tableTimeZone={tableTimeZone}
				/>
			) : null}
			<CalendarLinkCard canRegenerate={isDm} />
			{isDm && editing.value ? (
				<SeriesSheet
					key={editing.session}
					onOpenChange={editing.onOpenChange}
					open={editing.open}
					series={editing.value === "new" ? null : editing.value}
					tableTimeZone={tableTimeZone}
				/>
			) : null}
		</div>
	);
}

function SectionHeading({ id, children }: { id: string; children: ReactNode }) {
	return (
		<h2
			className="font-display font-semibold text-xl tracking-wide outline-none"
			id={id}
			tabIndex={-1}
		>
			{children}
		</h2>
	);
}

function ListSkeleton() {
	return (
		<div aria-busy="true" className="grid gap-2">
			<Skeleton className="h-20 rounded-xl" />
			<Skeleton className="h-20 rounded-xl" />
		</div>
	);
}

function UpcomingSessions({
	isDm,
	tableTimeZone,
	startLocalOf,
}: {
	isDm: boolean;
	tableTimeZone: string | undefined;
	startLocalOf: (seriesId: string | null) => string | undefined;
}) {
	const { campaignId } = useCampaign();
	const upcoming = useQuery(upcomingSessionsQuery(campaignId));
	const actions = useDateActions();
	let body: ReactNode;
	if (upcoming.isPending) {
		body = <ListSkeleton />;
	} else if (upcoming.isError) {
		body = (
			<p className="text-muted-foreground">
				{problemMessage(upcoming.error, "The upcoming dates couldn't be loaded.")}
			</p>
		);
	} else if (upcoming.data.length === 0) {
		body = (
			<p className="rounded-xl border border-border border-dashed px-4 py-4 text-muted-foreground">
				{isDm
					? "Nothing scheduled in the next 12 weeks. Schedule a weekly game or a one-off."
					: "Nothing scheduled in the next 12 weeks."}
			</p>
		);
	} else {
		body = (
			<UpcomingDates
				actions={isDm ? actions : null}
				sessions={upcoming.data}
				startLocalOf={startLocalOf}
				tableTimeZone={tableTimeZone}
			/>
		);
	}
	return (
		<section aria-labelledby="sessions-upcoming" className="flex flex-col gap-3">
			<SectionHeading id="sessions-upcoming">Upcoming</SectionHeading>
			{body}
		</section>
	);
}

function useDateActions(): DateActions {
	const { campaignId, slug } = useCampaign();
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const settle = useSessionsSettled();
	const cancel = api.useMutation("post", "/api/campaigns/{campaignId}/sessions/cancel", {
		onError: (error) => {
			toast.error(problemMessage(error, "That date couldn't be cancelled."));
		},
		onSettled: settle,
	});
	const restore = api.useMutation("post", "/api/campaigns/{campaignId}/sessions/restore", {
		onError: (error) => {
			toast.error(problemMessage(error, "That date couldn't be restored."));
		},
		onSettled: settle,
	});
	const markPlayed = api.useMutation("post", "/api/campaigns/{campaignId}/sessions/mark-played", {
		onError: (error) => {
			toast.error(problemMessage(error, "The session couldn't be marked played."));
		},
		onSettled: settle,
	});

	// The date may move between the next-game card and the list, so wait for React to render that.
	function focusToggle(session: Session) {
		requestAnimationFrame(() => {
			focusFirstVisible(dateToggleId(session), "sessions-upcoming");
		});
	}

	function bodyOf(session: Session) {
		return {
			body: { localDate: session.localDate, seriesId: session.seriesId ?? "" },
			params: { path: { campaignId } },
		};
	}

	return {
		cancel: (session: Session) => {
			cancel.mutate(bodyOf(session), {
				onSettled: () => {
					focusToggle(session);
				},
				onSuccess: () => {
					toast.success(`${session.title} on ${localDateLabel(session.localDate)} is cancelled.`);
				},
			});
		},
		isPending: cancel.isPending || restore.isPending || markPlayed.isPending,
		markPlayed: (session: Session) => {
			markPlayed.mutate(bodyOf(session), {
				onSuccess: (played) => {
					toast.success(`${played.title} is marked played.`);
					if (played.id === null) {
						return;
					}
					queryClient.setQueryData(sessionQuery(campaignId, played.id).queryKey, played);
					navigate({
						params: { campaignSlug: slug, sessionId: played.id },
						to: "/c/$campaignSlug/sessions/$sessionId",
					}).then(() => {
						focusFirstVisible(sessionHeadingId);
					});
				},
			});
		},
		restore: (session: Session) => {
			restore.mutate(bodyOf(session), {
				onSettled: () => {
					focusToggle(session);
				},
				onSuccess: () => {
					toast.success(`${session.title} on ${localDateLabel(session.localDate)} is back on.`);
				},
			});
		},
	};
}

function PlayedSessions() {
	const { campaignId, slug } = useCampaign();
	const played = useQuery(playedSessionsQuery(campaignId));
	let body: ReactNode;
	if (played.isPending) {
		body = <ListSkeleton />;
	} else if (played.isError) {
		body = (
			<p className="text-muted-foreground">
				{problemMessage(played.error, "The history couldn't be loaded.")}
			</p>
		);
	} else if (played.data.length === 0) {
		body = (
			<p className="rounded-xl border border-border border-dashed px-4 py-4 text-muted-foreground">
				No sessions played yet.
			</p>
		);
	} else {
		body = (
			<ul className="grid gap-2 md:grid-cols-2">
				{played.data.map((session) => {
					const days = colonyDaysLabel(session.colonyDayStart, session.colonyDayEnd);
					return (
						<li
							className="relative flex flex-col gap-1.5 rounded-xl border border-border bg-card px-4 py-3 shadow-card transition-colors focus-within:ring-[3px] focus-within:ring-ring/40 hover:border-gilt/40"
							key={session.id ?? sessionDateKey(session)}
						>
							<h3 className="font-semibold text-base leading-snug [overflow-wrap:anywhere]">
								{session.id === null ? (
									session.title
								) : (
									<Link
										className="outline-none after:absolute after:inset-0 after:rounded-xl"
										params={{ campaignSlug: slug, sessionId: session.id }}
										to="/c/$campaignSlug/sessions/$sessionId"
									>
										{session.title}
									</Link>
								)}
							</h3>
							<p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-muted-foreground text-sm">
								<span>{localDateLabel(session.localDate)}</span>
								{days ? <Badge variant="muted">{days}</Badge> : null}
								{session.colonyDayStart !== null && session.colonyDayEnd === null ? (
									<Badge variant="gilt">In play</Badge>
								) : null}
							</p>
							{session.recap.trim() === "" ? (
								<p className="text-muted-foreground text-sm italic">No recap yet.</p>
							) : (
								<p className="line-clamp-3 whitespace-pre-line font-flavor text-muted-foreground [overflow-wrap:anywhere]">
									{session.recap}
								</p>
							)}
						</li>
					);
				})}
			</ul>
		);
	}
	return (
		<section aria-labelledby="sessions-history" className="flex flex-col gap-3">
			<SectionHeading id="sessions-history">History</SectionHeading>
			{body}
		</section>
	);
}

function SeriesList({
	series,
	tableTimeZone,
	onEdit,
}: {
	series: { data?: SessionSeries[]; isPending: boolean; isError: boolean; error: unknown };
	tableTimeZone: string | undefined;
	onEdit: (series: SessionSeries) => void;
}) {
	let body: ReactNode;
	if (series.isPending) {
		body = <ListSkeleton />;
	} else if (series.isError || !series.data) {
		body = (
			<StatusPanel
				description={problemMessage(series.error, "The schedule couldn't be loaded.")}
				icon={CalendarDaysIcon}
				title="The schedule couldn't be loaded"
				tone="ember"
			/>
		);
	} else if (series.data.length === 0) {
		body = (
			<p className="rounded-xl border border-border border-dashed px-4 py-4 text-muted-foreground">
				No game nights scheduled. Schedule adds a weekly or every-other-week game, or a one-off.
			</p>
		);
	} else {
		body = (
			<ul className="grid gap-2 md:grid-cols-2">
				{series.data.map((item) => (
					<li
						className="flex flex-col gap-2 rounded-xl border border-border bg-card px-4 py-3 shadow-card"
						key={item.id}
					>
						<h3 className="font-semibold [overflow-wrap:anywhere]">{item.title}</h3>
						<p className="text-sm">
							{cadenceSummary(item)} at {clockLabel(item.startLocal)}
							{tableZoneSuffix(tableTimeZone, " ")}
							{" · "}
							{durationLabel(item.durationMinutes)}
						</p>
						<p className="text-muted-foreground text-sm">{seriesDatesLabel(item)}</p>
						<p className="flex flex-wrap gap-1.5">
							<Badge variant={item.announce ? "outline" : "muted"}>
								{item.announce ? "Discord event" : "No Discord event"}
							</Badge>
							<Badge variant={item.remind ? "outline" : "muted"}>
								{item.remind ? "Reminder" : "No reminder"}
							</Badge>
						</p>
						<div className="flex flex-wrap gap-2">
							<Button
								aria-label={`Edit ${item.title}`}
								onClick={() => {
									onEdit(item);
								}}
								size="sm"
								variant="outline"
							>
								<PenLineIcon />
								Edit
							</Button>
							<DeleteSeriesButton series={item} />
						</div>
					</li>
				))}
			</ul>
		);
	}
	return (
		<section aria-labelledby="sessions-schedule" className="flex flex-col gap-3">
			<SectionHeading id="sessions-schedule">Series and one-offs</SectionHeading>
			{body}
		</section>
	);
}

function DeleteSeriesButton({ series }: { series: SessionSeries }) {
	const { campaignId } = useCampaign();
	const settle = useSessionsSettled();
	const [open, setOpen] = useState(false);
	const [confirmed, setConfirmed] = useState(false);
	const mutation = api.useMutation(
		"delete",
		"/api/campaigns/{campaignId}/session-series/{seriesId}",
		{
			onError: (error) => {
				toast.error(problemMessage(error, `${series.title} couldn't be deleted.`));
			},
			onSettled: () => {
				settle();
				focusFirstVisible("sessions-schedule");
			},
			onSuccess: () => {
				toast.success(`${series.title} was taken off the schedule.`);
			},
		},
	);
	return (
		<AlertDialog onOpenChange={setOpen} open={open}>
			<Button
				aria-label={`Delete ${series.title}`}
				onClick={() => {
					setConfirmed(false);
					setOpen(true);
				}}
				size="sm"
				variant="ghost-destructive"
			>
				<Trash2Icon />
				Delete
			</Button>
			<AlertDialogContent
				onCloseAutoFocus={(event) => {
					if (confirmed) {
						event.preventDefault();
					}
				}}
			>
				<AlertDialogHeader>
					<AlertDialogTitle>Delete {series.title}?</AlertDialogTitle>
					<AlertDialogDescription>
						Its upcoming dates leave the schedule and everyone's calendar. Sessions already played
						keep their recaps and colony days.
					</AlertDialogDescription>
				</AlertDialogHeader>
				<AlertDialogFooter>
					<AlertDialogCancel>Keep it</AlertDialogCancel>
					<AlertDialogAction
						onClick={() => {
							setConfirmed(true);
							mutation.mutate({ params: { path: { campaignId, seriesId: series.id } } });
						}}
						variant="destructive"
					>
						Delete
					</AlertDialogAction>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	);
}
