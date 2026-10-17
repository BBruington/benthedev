import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { CalendarPlusIcon, CopyIcon, RefreshCwIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { problemMessage } from "@/api/problem";
import { calendarLinkQuery, colonyBoardQuery } from "@/api/queries";
import { SettingsCard } from "@/components/settings/settings-card";
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
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useCampaign } from "@/hooks/use-campaign";
import { copyToClipboard } from "@/lib/clipboard";
import {
	clockLabel,
	localDateLabel,
	tableOffsetMinutes,
	timezoneCity,
	utcOffsetLabel,
	zonedTime,
} from "@/lib/sessions";
import { cn } from "@/lib/utils";

type Session = Schemas["Session"];
type SessionReference = Schemas["SessionReference"];

/**
 * The campaign's time zone, from the board every member reads; undefined only while it loads, when
 * the table's time is labeled by its UTC offset instead.
 */
export function useTableTimeZone(): string | undefined {
	const { campaignId } = useCampaign();
	return useQuery(colonyBoardQuery(campaignId)).data?.timezone;
}

/** "8:00 PM EDT (New York)" or "8:00 PM UTC−4": when the game starts on the table's own clock. */
function tableTimeLabel(
	session: Pick<Session, "localDate" | "startsAt" | "status">,
	startLocal: string | undefined,
	tableTimeZone: string | undefined,
): { label: string; offsetMinutes: number } | null {
	if (tableTimeZone) {
		const table = zonedTime(session.startsAt, tableTimeZone);
		return {
			label: `${table.time} ${table.zone} (${timezoneCity(tableTimeZone)})`,
			offsetMinutes: table.offsetMinutes,
		};
	}
	if (!startLocal) {
		return null;
	}
	const offset = tableOffsetMinutes(session, startLocal);
	if (offset === null) {
		return null;
	}
	return { label: `${clockLabel(startLocal)} ${utcOffsetLabel(offset)}`, offsetMinutes: offset };
}

/**
 * When a session starts: on this viewer's clock first, labeled with their zone, then on the table's
 * clock when the two differ, so a time is never shown without saying whose it is.
 */
export function SessionWhen({
	session,
	startLocal,
	tableTimeZone,
	className,
}: {
	session: Pick<Session, "localDate" | "startsAt" | "status">;
	/** The series' HH:MM, which labels the table's time when its zone name isn't readable. */
	startLocal?: string;
	tableTimeZone: string | undefined;
	className?: string;
}) {
	const viewer = zonedTime(session.startsAt);
	const table = tableTimeLabel(session, startLocal, tableTimeZone);
	const sameClock = table !== null && table.offsetMinutes === viewer.offsetMinutes;
	return (
		<span className={cn("grid gap-0.5", className)}>
			<span>
				<time dateTime={session.startsAt}>
					{viewer.date} · {viewer.time}
				</time>{" "}
				<span className="text-muted-foreground text-sm">
					{viewer.zone}
					{sameClock ? ", the table's time too" : ", your time"}
				</span>
			</span>
			{table && !sameClock ? (
				<span className="text-muted-foreground text-sm">Table time {table.label}</span>
			) : null}
		</span>
	);
}

/** A played session as a link with its title and the table's date (Open Question 10). */
export function SessionLink({ session }: { session: SessionReference }) {
	const { slug } = useCampaign();
	return (
		<Link
			className="text-gilt-ink underline underline-offset-4"
			params={{ campaignSlug: slug, sessionId: session.id }}
			to="/c/$campaignSlug/sessions/$sessionId"
		>
			{session.title}
			<span className="text-muted-foreground"> · {localDateLabel(session.localDate)}</span>
		</Link>
	);
}

/** The ICS subscription link (spec §5.11): every member may copy it; only the DM regenerates it. */
export function CalendarLinkCard({ canRegenerate }: { canRegenerate: boolean }) {
	const { campaignId } = useCampaign();
	const queryClient = useQueryClient();
	const link = useQuery(calendarLinkQuery(campaignId));
	const [isConfirmOpen, setConfirmOpen] = useState(false);
	const regenerate = api.useMutation("post", "/api/campaigns/{campaignId}/calendar/regenerate", {
		onError: (error) => {
			toast.error(problemMessage(error, "The calendar link couldn't be replaced."));
		},
		onSuccess: (fresh) => {
			queryClient.setQueryData(calendarLinkQuery(campaignId).queryKey, fresh);
			toast.success("New calendar link ready. Calendars using the old one stop updating.");
		},
	});

	let body = <Skeleton className="h-11 rounded-md" />;
	if (link.isError) {
		body = (
			<p className="text-muted-foreground text-sm">
				{problemMessage(link.error, "The calendar link couldn't be loaded.")}
			</p>
		);
	} else if (link.data) {
		const calendarUrl = new URL(link.data.path, window.location.origin).toString();
		body = (
			<div className="grid gap-3">
				<label className="sr-only" htmlFor="calendar-link">
					Calendar link
				</label>
				<Input
					className="font-mono text-sm"
					id="calendar-link"
					onFocus={(event) => {
						event.target.select();
					}}
					readOnly
					value={calendarUrl}
				/>
				<div className="flex flex-wrap gap-2">
					<Button
						onClick={() => {
							copyToClipboard(calendarUrl).then(
								() => {
									toast.success("Calendar link copied. Add it to your calendar as a subscription.");
								},
								() => {
									toast.error("Copying didn't work. Select the text and copy it yourself.");
								},
							);
						}}
						type="button"
					>
						<CopyIcon />
						Copy
					</Button>
					{canRegenerate ? (
						<Button
							disabled={regenerate.isPending}
							onClick={() => {
								setConfirmOpen(true);
							}}
							type="button"
							variant="outline"
						>
							<RefreshCwIcon />
							{regenerate.isPending ? "Replacing…" : "New link…"}
						</Button>
					) : null}
				</div>
			</div>
		);
	}

	return (
		<SettingsCard
			description="Subscribe to it in Apple Calendar or another calendar app, and game nights appear there and stay up to date. It's for this table only: anyone with the link sees the schedule."
			icon={CalendarPlusIcon}
			id="calendar"
			title="Calendar link"
		>
			{body}
			{canRegenerate ? (
				<AlertDialog onOpenChange={setConfirmOpen} open={isConfirmOpen}>
					<AlertDialogContent>
						<AlertDialogHeader>
							<AlertDialogTitle>Replace the calendar link?</AlertDialogTitle>
							<AlertDialogDescription>
								Calendars subscribed to the old link stop updating at once. Everyone at the table
								needs to subscribe to the new one.
							</AlertDialogDescription>
						</AlertDialogHeader>
						<AlertDialogFooter>
							<AlertDialogCancel>Keep this link</AlertDialogCancel>
							<AlertDialogAction
								onClick={() => {
									regenerate.mutate({ params: { path: { campaignId } } });
								}}
								variant="destructive"
							>
								Replace link
							</AlertDialogAction>
						</AlertDialogFooter>
					</AlertDialogContent>
				</AlertDialog>
			) : null}
		</SettingsCard>
	);
}
