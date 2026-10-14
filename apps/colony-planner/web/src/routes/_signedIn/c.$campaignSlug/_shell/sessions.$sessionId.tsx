import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
	ArrowLeftIcon,
	ArrowRightIcon,
	BadgeCheckIcon,
	CalendarDaysIcon,
	CalendarXIcon,
	FlagIcon,
	PenLineIcon,
	RotateCcwIcon,
	SaveIcon,
} from "lucide-react";
import { type FormEvent, useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { fieldErrors, problemMessage, problemStatus } from "@/api/problem";
import {
	recentTimelineQuery,
	sessionQuery,
	sessionSeriesQuery,
	settingsQuery,
	timelineQueryKeyPrefix,
} from "@/api/queries";
import { BottomSheet } from "@/components/bottom-sheet";
import { Field, fieldInputProps } from "@/components/field";
import { TextArea } from "@/components/form-controls";
import { StatusPanel } from "@/components/status-panel";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { fallbackCampaignName, useCampaign, useCampaignName } from "@/hooks/use-campaign";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { useMe } from "@/hooks/use-me";
import { useSheetState } from "@/hooks/use-sheet-state";
import { cachedMe, canUseDmTools } from "@/lib/access";
import { parseWholeNumber } from "@/lib/colony";
import { focusFirstVisible } from "@/lib/focus";
import { forgetFailedSaves } from "@/lib/forms";
import { colonyDaysLabel, durationLabel, localDateLabel } from "@/lib/sessions";
import { useSessionsSettled } from "@/sessions/series-sheet";
import { sessionHeadingId } from "@/sessions/session-ids";
import { SessionWhen, useTableTimeZone } from "@/sessions/session-parts";

type Session = Schemas["Session"];

const statusLabels: Record<Schemas["SessionStatus"], string> = {
	CANCELLED: "Cancelled",
	PLAYED: "Played",
	SCHEDULED: "Scheduled",
};
const statusVariants = { CANCELLED: "ember", PLAYED: "moss", SCHEDULED: "gilt" } as const;
const recapMaxLength = 20_000;
const daysHeadingId = "session-days-heading";

export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell/sessions/$sessionId")({
	component: SessionPage,
	loader: ({ context: { campaignId, queryClient }, params: { sessionId } }) => {
		if (!campaignId) {
			return;
		}
		queryClient.prefetchQuery(sessionQuery(campaignId, sessionId));
		queryClient.prefetchQuery(sessionSeriesQuery(campaignId));
		const me = cachedMe(queryClient, campaignId);
		if (me && canUseDmTools(me)) {
			queryClient.prefetchQuery(settingsQuery(campaignId));
		}
	},
});

function BackToSessions() {
	const { slug } = useCampaign();
	return (
		<Button asChild className="self-start" size="sm" variant="ghost">
			<Link params={{ campaignSlug: slug }} to="/c/$campaignSlug/sessions">
				<ArrowLeftIcon />
				All sessions
			</Link>
		</Button>
	);
}

/** /sessions/{sessionId} (spec §6.2, §5.11): one game night, its colony days and its recap. */
function SessionPage() {
	const { sessionId } = Route.useParams();
	const { campaignId } = useCampaign();
	const me = useMe();
	const isDm = canUseDmTools(me);
	const session = useQuery(sessionQuery(campaignId, sessionId));
	useDocumentTitle(session.data?.title ?? "Session", useCampaignName() ?? fallbackCampaignName);

	if (session.isPending) {
		return (
			<div aria-busy="true" className="flex flex-col gap-6">
				<Skeleton className="h-16 w-2/3" />
				<Skeleton className="h-64 rounded-2xl" />
			</div>
		);
	}
	if (session.isError) {
		const isMissing = problemStatus(session.error) === 404;
		return (
			<div className="flex flex-col gap-4">
				<BackToSessions />
				<StatusPanel
					description={
						isMissing
							? "There's no session at that link."
							: problemMessage(session.error, "This session couldn't be loaded.")
					}
					icon={CalendarDaysIcon}
					title={isMissing ? "No such session" : "This session couldn't be loaded"}
					tone="ember"
				/>
			</div>
		);
	}
	return <SessionDetail isDm={isDm} session={session.data} />;
}

/** Keeps the page's own copy current at once; the topic refreshes everything else that reads sessions. */
function useSessionSaved() {
	const queryClient = useQueryClient();
	const { campaignId } = useCampaign();
	const settle = useSessionsSettled();
	return {
		onSettled: () => {
			settle();
			queryClient.invalidateQueries({ queryKey: timelineQueryKeyPrefix(campaignId) });
			queryClient.invalidateQueries({ queryKey: recentTimelineQuery(campaignId).queryKey });
		},
		save: (saved: Session) => {
			if (saved.id) {
				queryClient.setQueryData(sessionQuery(campaignId, saved.id).queryKey, saved);
			}
		},
	};
}

function SessionDetail({ session, isDm }: { session: Session; isDm: boolean }) {
	const { campaignId } = useCampaign();
	const tableTimeZone = useTableTimeZone();
	const series = useQuery(sessionSeriesQuery(campaignId));
	const startLocal = series.data?.find((item) => item.id === session.seriesId)?.startLocal;
	const renaming = useSheetState<true>();

	return (
		<div className="flex flex-col gap-6">
			<BackToSessions />
			<header className="flex flex-wrap items-start gap-4">
				<div className="min-w-0 flex-1 space-y-2">
					<h1
						className="font-display font-semibold text-3xl tracking-wide outline-none [overflow-wrap:anywhere] md:text-4xl"
						id={sessionHeadingId}
						tabIndex={-1}
					>
						{session.title}
					</h1>
					<p className="flex flex-wrap items-center gap-1.5">
						<Badge variant={statusVariants[session.status]}>{statusLabels[session.status]}</Badge>
						<Badge variant="outline">{localDateLabel(session.localDate)}</Badge>
						<Badge variant="muted">{durationLabel(session.durationMinutes)}</Badge>
					</p>
					<SessionWhen session={session} startLocal={startLocal} tableTimeZone={tableTimeZone} />
				</div>
				{isDm ? (
					<div className="flex w-full flex-wrap gap-2 sm:w-auto">
						<StatusActions session={session} />
						{session.status === "PLAYED" ? (
							<Button
								onClick={() => {
									renaming.show(true);
								}}
								variant="outline"
							>
								<PenLineIcon />
								Rename
							</Button>
						) : null}
					</div>
				) : null}
			</header>
			<ColonyDays isDm={isDm} session={session} />
			<section
				aria-labelledby="session-recap-heading"
				className="flex flex-col gap-3 rounded-2xl border border-border bg-card p-5 shadow-card md:p-7"
			>
				<h2 className="font-display font-semibold text-xl tracking-wide" id="session-recap-heading">
					Recap
				</h2>
				{isDm ? (
					<RecapEditor key={session.id} session={session} />
				) : (
					<RecapText recap={session.recap} />
				)}
			</section>
			{isDm && renaming.value ? (
				<RenameSheet
					key={renaming.session}
					onOpenChange={renaming.onOpenChange}
					open={renaming.open}
					session={session}
				/>
			) : null}
		</div>
	);
}

function RecapText({ recap }: { recap: string }) {
	if (recap.trim() === "") {
		return <p className="text-muted-foreground italic">No recap yet.</p>;
	}
	return (
		<p className="whitespace-pre-line font-flavor text-lg leading-relaxed [overflow-wrap:anywhere]">
			{recap}
		</p>
	);
}

/** Mark played, End session, Cancel and Restore, by where the session stands (spec §5.11). */
function StatusActions({ session }: { session: Session }) {
	const { campaignId } = useCampaign();
	const saved = useSessionSaved();
	const dateBody = {
		body: { localDate: session.localDate, seriesId: session.seriesId ?? "" },
		params: { path: { campaignId } },
	};
	const options = (failure: string, success: (result: Session) => string) => ({
		onError: (error: unknown) => {
			toast.error(problemMessage(error, failure));
		},
		onSettled: () => {
			saved.onSettled();
			focusFirstVisible(sessionHeadingId);
		},
		onSuccess: (result: Session) => {
			saved.save(result);
			toast.success(success(result));
		},
	});
	const markPlayed = api.useMutation(
		"post",
		"/api/campaigns/{campaignId}/sessions/mark-played",
		options(
			"The session couldn't be marked played.",
			(result) => `${result.title} is marked played.`,
		),
	);
	const end = api.useMutation(
		"post",
		"/api/campaigns/{campaignId}/sessions/{sessionId}/end",
		options("The session couldn't be ended.", (result) => `${result.title} has ended.`),
	);
	const cancel = api.useMutation(
		"post",
		"/api/campaigns/{campaignId}/sessions/cancel",
		options("That date couldn't be cancelled.", (result) => `${result.title} is cancelled.`),
	);
	const restore = api.useMutation(
		"post",
		"/api/campaigns/{campaignId}/sessions/restore",
		options("That date couldn't be restored.", (result) => `${result.title} is back on.`),
	);
	const isPending = markPlayed.isPending || end.isPending || cancel.isPending || restore.isPending;

	if (session.status === "PLAYED") {
		if (session.colonyDayEnd !== null || session.id === null) {
			return null;
		}
		const sessionId = session.id;
		return (
			<Button
				disabled={isPending}
				onClick={() => {
					end.mutate({ params: { path: { campaignId, sessionId } } });
				}}
			>
				<FlagIcon />
				End session
			</Button>
		);
	}
	if (session.seriesId === null) {
		return null;
	}
	if (session.status === "CANCELLED") {
		return (
			<Button
				disabled={isPending}
				onClick={() => {
					restore.mutate(dateBody);
				}}
				variant="outline"
			>
				<RotateCcwIcon />
				Restore
			</Button>
		);
	}
	return (
		<>
			<Button
				disabled={isPending}
				onClick={() => {
					markPlayed.mutate(dateBody);
				}}
			>
				<BadgeCheckIcon />
				Mark played
			</Button>
			<Button
				disabled={isPending}
				onClick={() => {
					cancel.mutate(dateBody);
				}}
				variant="ghost-destructive"
			>
				<CalendarXIcon />
				Cancel this date
			</Button>
		</>
	);
}

/** The colony days the session covered, with the DM's correction (spec §5.11: both stamps are editable). */
function ColonyDays({ session, isDm }: { session: Session; isDm: boolean }) {
	const { slug } = useCampaign();
	const days = colonyDaysLabel(session.colonyDayStart, session.colonyDayEnd);
	if (session.status !== "PLAYED") {
		return null;
	}
	return (
		<section
			aria-labelledby={daysHeadingId}
			className="flex flex-col gap-3 rounded-2xl border border-border bg-card p-5 shadow-card md:p-7"
		>
			<h2
				className="font-display font-semibold text-xl tracking-wide outline-none"
				id={daysHeadingId}
				tabIndex={-1}
			>
				In the colony
			</h2>
			<p className="flex flex-wrap items-center gap-x-3 gap-y-2 text-lg">
				<span>{days ?? "No colony days stamped."}</span>
				{session.colonyDayStart !== null && session.colonyDayEnd === null ? (
					<Badge variant="gilt">Still in play</Badge>
				) : null}
			</p>
			{session.colonyDayStart === null ? null : (
				<Button asChild className="self-start" size="sm" variant="outline">
					<Link
						params={{ campaignSlug: slug }}
						search={{ day: session.colonyDayStart }}
						to="/c/$campaignSlug/timeline"
					>
						See it on the timeline
						<ArrowRightIcon aria-hidden="true" />
					</Link>
				</Button>
			)}
			{isDm && session.id ? (
				// Ending the session or another device's edit moves the stamps; the form starts over from them.
				<DaysEditor
					key={`${session.id}:${session.colonyDayStart}:${session.colonyDayEnd}`}
					session={session}
				/>
			) : null}
		</section>
	);
}

function DaysEditor({ session }: { session: Session }) {
	const { campaignId } = useCampaign();
	const saved = useSessionSaved();
	const [start, setStart] = useState(String(session.colonyDayStart ?? ""));
	const [end, setEnd] = useState(session.colonyDayEnd === null ? "" : String(session.colonyDayEnd));
	const mutation = api.useMutation("put", "/api/campaigns/{campaignId}/sessions/{sessionId}/days", {
		onError: (error) => {
			if (Object.keys(fieldErrors(error)).length === 0) {
				toast.error(problemMessage(error, "The days couldn't be saved."));
			}
		},
		onSettled: saved.onSettled,
		onSuccess: (result) => {
			saved.save(result);
			toast.success(
				`${result.title}: ${colonyDaysLabel(result.colonyDayStart, result.colonyDayEnd) ?? "days"} saved.`,
			);
		},
	});
	const errors = fieldErrors(mutation.error);
	const parsedStart = parseWholeNumber(start);
	const parsedEnd = parseWholeNumber(end);
	const isValid = typeof parsedStart === "number" && parsedEnd !== "invalid";
	const isDirty = parsedStart !== session.colonyDayStart || parsedEnd !== session.colonyDayEnd;

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!session.id || typeof parsedStart !== "number" || parsedEnd === "invalid") {
			return;
		}
		mutation.mutate({
			body: { colonyDayEnd: parsedEnd, colonyDayStart: parsedStart },
			params: { path: { campaignId, sessionId: session.id } },
		});
	}

	return (
		<form className="grid gap-4 border-border border-t pt-4" noValidate onSubmit={handleSubmit}>
			<p className="text-muted-foreground text-sm">
				Correct the stamps if the session started or ended on another colony day. Nothing else in
				the colony changes.
			</p>
			<div className="grid grid-cols-2 gap-3 sm:max-w-md">
				<Field error={errors.colonyDayStart} id="session-day-start" label="First day">
					<Input
						{...fieldInputProps("session-day-start", errors.colonyDayStart, false)}
						inputMode="numeric"
						onChange={(event) => {
							setStart(event.target.value);
							forgetFailedSaves(mutation);
						}}
						value={start}
					/>
				</Field>
				<Field error={errors.colonyDayEnd} id="session-day-end" label="Last day" optional>
					<Input
						{...fieldInputProps("session-day-end", errors.colonyDayEnd, false)}
						inputMode="numeric"
						onChange={(event) => {
							setEnd(event.target.value);
							forgetFailedSaves(mutation);
						}}
						placeholder="Not ended"
						value={end}
					/>
				</Field>
			</div>
			<Button
				className="self-start"
				disabled={!(isValid && isDirty) || mutation.isPending}
				type="submit"
				variant="outline"
			>
				<SaveIcon />
				{mutation.isPending ? "Saving…" : "Save days"}
			</Button>
		</form>
	);
}

function RecapEditor({ session }: { session: Session }) {
	const { campaignId } = useCampaign();
	const saved = useSessionSaved();
	const [recap, setRecap] = useState(session.recap);
	const [baseline, setBaseline] = useState(session.recap);
	// Another device may save the recap; an untouched editor follows it, an edited one keeps the typing.
	if (session.recap !== baseline) {
		setBaseline(session.recap);
		if (recap === baseline) {
			setRecap(session.recap);
		}
	}
	const mutation = api.useMutation(
		"put",
		"/api/campaigns/{campaignId}/sessions/{sessionId}/recap",
		{
			onError: (error) => {
				if (Object.keys(fieldErrors(error)).length === 0) {
					toast.error(problemMessage(error, "The recap couldn't be saved."));
				}
			},
			onSettled: saved.onSettled,
			onSuccess: (result) => {
				saved.save(result);
				toast.success("Recap saved. Everyone can read it now.");
			},
		},
	);
	const errors = fieldErrors(mutation.error);
	const isDirty = recap !== session.recap;

	if (!session.id) {
		return <RecapText recap={session.recap} />;
	}
	const sessionId = session.id;
	return (
		<form
			className="grid gap-3"
			noValidate
			onSubmit={(event) => {
				event.preventDefault();
				mutation.mutate({ body: { recap }, params: { path: { campaignId, sessionId } } });
			}}
		>
			<Field
				description="Everyone at the table reads it here."
				error={errors.recap}
				id="session-recap"
				label="What happened"
			>
				<TextArea
					{...fieldInputProps("session-recap", errors.recap, true)}
					className="min-h-48 font-flavor text-lg"
					maxLength={recapMaxLength}
					onChange={(event) => {
						setRecap(event.target.value);
						forgetFailedSaves(mutation);
					}}
					value={recap}
				/>
			</Field>
			<div className="flex flex-wrap gap-2">
				<Button disabled={!isDirty || mutation.isPending} type="submit">
					<SaveIcon />
					{mutation.isPending ? "Saving…" : "Save recap"}
				</Button>
				{isDirty ? (
					<Button
						onClick={() => {
							setRecap(session.recap);
							mutation.reset();
						}}
						type="button"
						variant="ghost"
					>
						<RotateCcwIcon />
						Discard
					</Button>
				) : null}
			</div>
		</form>
	);
}

function RenameSheet({
	session,
	open,
	onOpenChange,
}: {
	session: Session;
	open: boolean;
	onOpenChange: (open: boolean) => void;
}) {
	const { campaignId } = useCampaign();
	const saved = useSessionSaved();
	const [title, setTitle] = useState(session.title);
	const mutation = api.useMutation(
		"put",
		"/api/campaigns/{campaignId}/sessions/{sessionId}/title",
		{
			onSettled: saved.onSettled,
		},
	);
	const errors = fieldErrors(mutation.error);
	const showGeneralError = mutation.error !== null && Object.keys(errors).length === 0;
	const canSave = title.trim() !== "" && !mutation.isPending && session.id !== null;

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!(canSave && session.id)) {
			return;
		}
		mutation.mutate(
			{ body: { title: title.trim() }, params: { path: { campaignId, sessionId: session.id } } },
			{
				onSuccess: (result) => {
					saved.save(result);
					toast.success(`Renamed to ${result.title}.`);
					onOpenChange(false);
				},
			},
		);
	}

	return (
		<BottomSheet
			description="Only this session changes; its series keeps its name."
			onOpenChange={onOpenChange}
			open={open}
			title="Rename this session"
		>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<Field error={errors.title} id="session-title" label="Title">
					<Input
						{...fieldInputProps("session-title", errors.title, false)}
						autoComplete="off"
						autoFocus
						maxLength={80}
						onChange={(event) => {
							setTitle(event.target.value);
							forgetFailedSaves(mutation);
						}}
						value={title}
					/>
				</Field>
				{showGeneralError ? (
					<p className="text-ember-ink text-sm" role="alert">
						{problemMessage(mutation.error, "The session couldn't be renamed.")}
					</p>
				) : null}
				<div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
					<Button
						onClick={() => {
							onOpenChange(false);
						}}
						type="button"
						variant="ghost"
					>
						Cancel
					</Button>
					<Button disabled={!canSave} type="submit">
						{mutation.isPending ? "Saving…" : "Rename"}
					</Button>
				</div>
			</form>
		</BottomSheet>
	);
}
