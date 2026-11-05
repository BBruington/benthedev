import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowRightIcon, ListChecksIcon } from "lucide-react";
import type { Schemas } from "@/api/client";
import { pollsQuery } from "@/api/queries";
import { Button } from "@/components/ui/button";
import { useCampaign } from "@/hooks/use-campaign";
import { closeTimeText, isPollOpen, pickText, voteSummary } from "@/lib/polls";
import { focusAfterClose } from "@/people/colonist-actions";
import { DiscordVoteLink, PollVoteForm } from "./poll-options";
import { usePollClock } from "./use-poll-clock";
import { usePollVote } from "./use-poll-vote";

type Poll = Schemas["Poll"];

const boardPollsHeadingId = "board-polls-heading";

/**
 * The board's open polls: a player who hasn't answered votes right here (or follows the link to a
 * posted Discord poll), and everyone else gets one line per poll that leads to the Polls page.
 * Nothing shows while no poll is open.
 */
export function BoardPolls({ isDm }: { isDm: boolean }) {
	const { campaignId, slug } = useCampaign();
	const polls = useQuery(pollsQuery(campaignId));
	const { vote, isVoting } = usePollVote();
	const now = usePollClock();
	const open = (polls.data?.open ?? []).filter((poll) => isPollOpen(poll, now));
	if (open.length === 0) {
		return null;
	}
	const toVote = isDm
		? []
		: open.filter(
				(poll) =>
					poll.myOptionIds.length === 0 && (poll.voting === "SITE" || Boolean(poll.discordUrl)),
			);
	const answered = open.filter((poll) => !toVote.includes(poll));

	return (
		<section
			aria-labelledby={boardPollsHeadingId}
			className="flex flex-col gap-4 rounded-xl border border-border bg-card p-4 shadow-card sm:p-5"
		>
			<div className="flex items-center justify-between gap-3">
				<h2
					className="flex items-center gap-2 font-display font-semibold text-xl tracking-wide"
					id={boardPollsHeadingId}
					tabIndex={-1}
				>
					<ListChecksIcon aria-hidden="true" className="size-5 text-gilt-ink" />
					Polls
				</h2>
				<Button asChild size="sm" variant="ghost">
					<Link params={{ campaignSlug: slug }} to="/c/$campaignSlug/polls">
						All polls
						<ArrowRightIcon aria-hidden="true" />
					</Link>
				</Button>
			</div>
			{toVote.map((poll) => (
				<BoardVote
					isSaving={isVoting(poll.id)}
					key={poll.id}
					now={now}
					onVote={(optionIds) => {
						// A vote turns this card into a results line, so focus goes to the section's heading.
						vote(poll.id, optionIds, () => {
							focusAfterClose(boardPollsHeadingId);
						});
					}}
					poll={poll}
				/>
			))}
			{answered.length > 0 ? (
				<ul className="grid gap-2">
					{answered.map((poll) => (
						<li key={poll.id}>
							<Link
								className="flex min-h-11 flex-col justify-center gap-0.5 rounded-lg px-3 py-2 transition-colors hover:bg-muted/60 focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/45"
								params={{ campaignSlug: slug }}
								to="/c/$campaignSlug/polls"
							>
								<span className="font-medium [overflow-wrap:anywhere]">{poll.question}</span>
								<span className="text-muted-foreground text-sm">{voteSummary(poll)}</span>
							</Link>
						</li>
					))}
				</ul>
			) : null}
		</section>
	);
}

function BoardVote({
	poll,
	now,
	isSaving,
	onVote,
}: {
	poll: Poll;
	now: Date;
	isSaving: boolean;
	onVote: (optionIds: string[]) => void;
}) {
	const headingId = `board-poll-${poll.id}-question`;
	return (
		<div className="grid gap-3 border-border border-t pt-4">
			<div className="grid gap-1">
				<h3 className="font-semibold [overflow-wrap:anywhere]" id={headingId}>
					{poll.question}
				</h3>
				<p className="text-muted-foreground text-sm">
					{pickText(poll)} · {closeTimeText(poll, now)}
				</p>
			</div>
			{poll.voting === "DISCORD" ? (
				<DiscordVoteLink poll={poll} />
			) : (
				<PollVoteForm
					isSaving={isSaving}
					labelledBy={headingId}
					onVote={onVote}
					poll={poll}
					showVoters={false}
				/>
			)}
		</div>
	);
}
