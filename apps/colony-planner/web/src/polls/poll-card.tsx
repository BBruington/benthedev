import { ClockIcon, LockIcon, Trash2Icon } from "lucide-react";
import type { Schemas } from "@/api/client";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { closeTimeText, isPollOpen, pickText } from "@/lib/polls";
import { cn } from "@/lib/utils";
import { countOf } from "@/lib/words";
import { DiscordVoteLink, PollResults, PollVoteForm } from "./poll-options";

type Poll = Schemas["Poll"];

export function pollHeadingId(pollId: string): string {
	return `poll-${pollId}-heading`;
}

/**
 * One poll on the Polls page. A player votes in place while a site poll is open, or follows the link
 * to a Discord poll; the DM and everyone after it closes see the counts and who picked what. Close and
 * Delete are the DM's.
 */
export function PollCard({
	poll,
	now,
	canVote,
	canManage,
	isVoting,
	onVote,
	onClose,
	onDelete,
}: {
	poll: Poll;
	now: Date;
	canVote: boolean;
	canManage: boolean;
	isVoting: boolean;
	onVote: (optionIds: string[]) => void;
	onClose: () => void;
	onDelete: () => void;
}) {
	const open = isPollOpen(poll, now);
	const votesInDiscord = poll.voting === "DISCORD";
	const headingId = pollHeadingId(poll.id);
	return (
		<article
			aria-labelledby={headingId}
			className={cn(
				"flex flex-col gap-4 rounded-xl border bg-card p-4 shadow-card sm:p-5",
				open ? "border-gilt/40" : "border-border",
			)}
		>
			<header className="grid gap-2">
				<h3
					className="text-balance font-display font-semibold text-xl tracking-wide [overflow-wrap:anywhere]"
					id={headingId}
					tabIndex={-1}
				>
					{poll.question}
				</h3>
				<p className="flex flex-wrap items-center gap-x-3 gap-y-1.5 text-muted-foreground text-sm">
					{open ? null : <Badge variant="muted">Closed</Badge>}
					<Badge variant="outline">{pickText(poll)}</Badge>
					<span className="tabular-nums">{countOf(poll.voterCount, "voter")}</span>
					<span className="flex items-center gap-1.5">
						<ClockIcon aria-hidden="true" className="size-4 shrink-0" />
						{closeTimeText(poll, now)}
					</span>
				</p>
			</header>
			{canVote && open && !votesInDiscord ? (
				<PollVoteForm
					isSaving={isVoting}
					key={poll.myOptionIds.join(",")}
					labelledBy={headingId}
					onVote={onVote}
					poll={poll}
				/>
			) : (
				<PollResults poll={poll} />
			)}
			{votesInDiscord && open ? <DiscordVoteLink poll={poll} /> : null}
			{canManage ? (
				<div className="flex flex-wrap justify-end gap-2 border-border border-t pt-3">
					<Button onClick={onDelete} size="sm" variant="ghost-destructive">
						<Trash2Icon />
						Delete
					</Button>
					{open ? (
						<Button onClick={onClose} size="sm" variant="outline">
							<LockIcon />
							Close poll
						</Button>
					) : null}
				</div>
			) : null}
		</article>
	);
}
