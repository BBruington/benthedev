import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { ListChecksIcon, PlusIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { problemMessage } from "@/api/problem";
import { pollsQuery, recentTimelineQuery, timelineQueryKeyPrefix } from "@/api/queries";
import { PageHeader } from "@/components/page-header";
import { StatusPanel } from "@/components/status-panel";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { fallbackCampaignName, useCampaignId, useCampaignName } from "@/hooks/use-campaign";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { useMe } from "@/hooks/use-me";
import { useSheetState } from "@/hooks/use-sheet-state";
import { canUseDmTools } from "@/lib/access";
import { isPollOpen } from "@/lib/polls";
import { focusAfterClose } from "@/people/colonist-actions";
import { CreatePollSheet } from "@/polls/create-poll-sheet";
import { PollCard, pollHeadingId } from "@/polls/poll-card";
import { PollConfirmDialog, type PollConfirmKind } from "@/polls/poll-confirm-dialog";
import { usePollClock } from "@/polls/use-poll-clock";
import { usePollVote } from "@/polls/use-poll-vote";

type Poll = Schemas["Poll"];

export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell/polls")({
	component: PollsScreen,
	loader: ({ context: { campaignId, queryClient } }) => {
		if (!campaignId) {
			return;
		}
		queryClient.prefetchQuery(pollsQuery(campaignId));
	},
});

const pollsHeadingId = "polls-heading";

/**
 * /polls: the DM asks the table something and every player answers here. Open polls come first,
 * newest first, then the most recent closed ones with their final counts.
 */
function PollsScreen() {
	useDocumentTitle("Polls", useCampaignName() ?? fallbackCampaignName);
	const me = useMe();
	const canManage = canUseDmTools(me);
	// The DM runs polls and never votes, even while viewing as a player.
	const canVote = !me.isDm;
	const campaignId = useCampaignId();
	const queryClient = useQueryClient();
	const polls = useQuery(pollsQuery(campaignId));
	const createSheet = useSheetState<true>();
	const { vote, isVoting } = usePollVote();
	const [confirmOpen, setConfirmOpen] = useState(false);
	const [confirmTarget, setConfirmTarget] = useState<{ kind: PollConfirmKind; poll: Poll } | null>(
		null,
	);
	const now = usePollClock();

	function refreshPolls() {
		return queryClient.invalidateQueries({ queryKey: pollsQuery(campaignId).queryKey });
	}

	const close = api.useMutation("post", "/api/campaigns/{campaignId}/polls/{pollId}/close", {
		onError: (error) => {
			toast.error(problemMessage(error, "The poll couldn't be closed."));
		},
		onSettled: () =>
			Promise.all([
				refreshPolls(),
				queryClient.invalidateQueries({ queryKey: recentTimelineQuery(campaignId).queryKey }),
				queryClient.invalidateQueries({ queryKey: timelineQueryKeyPrefix(campaignId) }),
			]),
		onSuccess: () => {
			toast.success("Poll closed.");
		},
	});
	const remove = api.useMutation("delete", "/api/campaigns/{campaignId}/polls/{pollId}", {
		onError: (error) => {
			toast.error(problemMessage(error, "The poll couldn't be deleted."));
		},
		onSettled: refreshPolls,
		onSuccess: () => {
			toast.success("Poll deleted.");
		},
	});

	function confirm(kind: PollConfirmKind, poll: Poll) {
		setConfirmTarget({ kind, poll });
		setConfirmOpen(true);
	}

	function handleConfirmed() {
		if (!confirmTarget) {
			return;
		}
		const { kind, poll } = confirmTarget;
		const mutation = kind === "close" ? close : remove;
		mutation.mutate(
			{ params: { path: { campaignId, pollId: poll.id } } },
			{
				onSettled: () => {
					// A closed poll moves down the page and a deleted one is gone, so focus follows or falls back.
					focusAfterClose(...(kind === "close" ? [pollHeadingId(poll.id)] : []), pollsHeadingId);
				},
			},
		);
	}

	let body: React.ReactNode;
	if (polls.isPending) {
		body = (
			<div aria-busy="true" className="flex flex-col gap-6">
				<Skeleton className="h-56 rounded-xl" />
				<Skeleton className="h-40 rounded-xl" />
			</div>
		);
	} else if (polls.isError) {
		body = (
			<StatusPanel
				description={problemMessage(polls.error, "Polls couldn't be loaded.")}
				icon={ListChecksIcon}
				title="Polls couldn't be loaded"
				tone="ember"
			/>
		);
	} else {
		// A poll that passed its closing time while the page was open joins the closed ones, newest close first.
		const open = polls.data.open.filter((poll) => isPollOpen(poll, now));
		const closed = [
			...polls.data.open.filter((poll) => !isPollOpen(poll, now)),
			...polls.data.closed,
		].sort(
			(first, second) =>
				Date.parse(second.closedAt ?? second.closesAt) -
				Date.parse(first.closedAt ?? first.closesAt),
		);
		const renderCard = (poll: Poll) => (
			<PollCard
				canManage={canManage}
				canVote={canVote}
				isVoting={isVoting(poll.id)}
				key={poll.id}
				now={now}
				onClose={() => {
					confirm("close", poll);
				}}
				onDelete={() => {
					confirm("delete", poll);
				}}
				onVote={(optionIds) => {
					// The vote form remounts with the new choice, so focus goes to the poll's heading.
					vote(poll.id, optionIds, () => {
						focusAfterClose(pollHeadingId(poll.id));
					});
				}}
				poll={poll}
			/>
		);
		body = (
			<div className="flex flex-col gap-10">
				<section aria-labelledby="open-polls-heading" className="flex flex-col gap-4">
					<h2 className="font-display font-semibold text-2xl tracking-wide" id="open-polls-heading">
						Open
					</h2>
					{open.length === 0 ? (
						<p className="rounded-xl border border-border border-dashed px-4 py-4 text-muted-foreground">
							{canManage
								? "No open polls. Start one to ask the table something."
								: "No open polls. When the DM asks the table something, it shows up here."}
						</p>
					) : (
						open.map(renderCard)
					)}
				</section>
				{closed.length > 0 ? (
					<section aria-labelledby="closed-polls-heading" className="flex flex-col gap-4">
						<h2
							className="font-display font-semibold text-2xl tracking-wide"
							id="closed-polls-heading"
						>
							Closed
						</h2>
						{closed.map(renderCard)}
					</section>
				) : null}
			</div>
		);
	}

	return (
		<div className="flex flex-col gap-8">
			<PageHeader
				actions={
					canManage ? (
						<Button
							onClick={() => {
								createSheet.show(true);
							}}
						>
							<PlusIcon />
							New poll
						</Button>
					) : null
				}
				description="Questions for the whole table, answered between sessions"
				headingId={pollsHeadingId}
				title="Polls"
			/>
			{body}
			{canManage ? (
				<>
					{createSheet.value ? (
						<CreatePollSheet
							focusTargetId={pollsHeadingId}
							key={createSheet.session}
							newPollVoting={polls.data?.newPollVoting}
							onOpenChange={createSheet.onOpenChange}
							open={createSheet.open}
						/>
					) : null}
					{confirmTarget ? (
						<PollConfirmDialog
							kind={confirmTarget.kind}
							onConfirm={handleConfirmed}
							onOpenChange={setConfirmOpen}
							open={confirmOpen}
							question={confirmTarget.poll.question}
						/>
					) : null}
				</>
			) : null}
		</div>
	);
}
