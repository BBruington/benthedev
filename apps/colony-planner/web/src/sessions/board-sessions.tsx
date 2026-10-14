import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowRightIcon, CalendarDaysIcon, FeatherIcon } from "lucide-react";
import type { ReactNode } from "react";
import type { Schemas } from "@/api/client";
import { latestRecapQuery, nextSessionQuery, sessionSeriesQuery } from "@/api/queries";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useCampaign } from "@/hooks/use-campaign";
import { colonyDaysLabel, localDateLabel } from "@/lib/sessions";
import { SessionWhen, useTableTimeZone } from "./session-parts";

type Session = Schemas["Session"];

function Half({
	headingId,
	icon: Icon,
	title,
	children,
}: {
	headingId: string;
	icon: typeof CalendarDaysIcon;
	title: string;
	children: ReactNode;
}) {
	return (
		<section aria-labelledby={headingId} className="flex min-w-0 flex-col gap-3">
			<h2
				className="flex items-center gap-2 font-display font-semibold text-xl tracking-wide"
				id={headingId}
			>
				<Icon aria-hidden="true" className="size-5 text-gilt-ink" />
				{title}
			</h2>
			{children}
		</section>
	);
}

/**
 * The board's next game night and last recap snippet (spec §6.2). Both reads answer
 * `{session: null}` when there's nothing to show, although the generated type says Session.
 */
export function BoardSessions({ isDm }: { isDm: boolean }) {
	const { campaignId, slug } = useCampaign();
	const next = useQuery(nextSessionQuery(campaignId));
	const recap = useQuery(latestRecapQuery(campaignId));
	const series = useQuery(sessionSeriesQuery(campaignId));
	const tableTimeZone = useTableTimeZone();
	const nextSession: Session | null = next.data?.session ?? null;
	const recapSession: Session | null = recap.data?.session ?? null;

	let nextBody: ReactNode = <Skeleton className="h-16 rounded-lg" />;
	if (next.isError) {
		nextBody = <p className="text-muted-foreground">The schedule couldn't be loaded.</p>;
	} else if (next.isSuccess && nextSession === null) {
		nextBody = (
			<p className="text-muted-foreground">
				{isDm
					? "Nothing scheduled. Plan the next game night on Sessions."
					: "Nothing scheduled yet."}
			</p>
		);
	} else if (nextSession) {
		const startLocal = series.data?.find((item) => item.id === nextSession.seriesId)?.startLocal;
		nextBody = (
			<div className="grid gap-1">
				<p className="font-semibold [overflow-wrap:anywhere]">{nextSession.title}</p>
				<SessionWhen session={nextSession} startLocal={startLocal} tableTimeZone={tableTimeZone} />
			</div>
		);
	}

	let recapBody: ReactNode = <Skeleton className="h-16 rounded-lg" />;
	if (recap.isError) {
		recapBody = <p className="text-muted-foreground">The last recap couldn't be loaded.</p>;
	} else if (recap.isSuccess && recapSession === null) {
		recapBody = <p className="text-muted-foreground">No recaps written yet.</p>;
	} else if (recapSession) {
		const days = colonyDaysLabel(recapSession.colonyDayStart, recapSession.colonyDayEnd);
		recapBody = (
			<div className="grid gap-2">
				<p className="flex flex-wrap items-center gap-x-2 gap-y-1">
					{recapSession.id ? (
						<Link
							className="font-semibold text-gilt-ink underline-offset-4 [overflow-wrap:anywhere] hover:underline"
							params={{ campaignSlug: slug, sessionId: recapSession.id }}
							to="/c/$campaignSlug/sessions/$sessionId"
						>
							{recapSession.title}
						</Link>
					) : (
						<span className="font-semibold">{recapSession.title}</span>
					)}
					<span className="text-muted-foreground text-sm">
						{localDateLabel(recapSession.localDate)}
					</span>
					{days ? <Badge variant="muted">{days}</Badge> : null}
				</p>
				<p className="line-clamp-3 whitespace-pre-line font-flavor text-muted-foreground italic [overflow-wrap:anywhere]">
					{recapSession.recap}
				</p>
			</div>
		);
	}

	return (
		<div className="flex flex-col gap-4 rounded-xl border border-border bg-card p-4 shadow-card sm:p-5">
			<div className="grid gap-6 lg:grid-cols-2">
				<Half headingId="board-next-session" icon={CalendarDaysIcon} title="Next game night">
					{nextBody}
				</Half>
				<Half headingId="board-last-recap" icon={FeatherIcon} title="Last time">
					{recapBody}
				</Half>
			</div>
			<Button asChild className="self-start" size="sm" variant="ghost">
				<Link params={{ campaignSlug: slug }} to="/c/$campaignSlug/sessions">
					Sessions
					<ArrowRightIcon aria-hidden="true" />
				</Link>
			</Button>
		</div>
	);
}
