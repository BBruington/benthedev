import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import { api } from "@/api/client";
import { problemMessage } from "@/api/problem";
import { pollsQuery } from "@/api/queries";
import { useCampaignId } from "@/hooks/use-campaign";

/**
 * A player's vote on the Polls page and the board. `vote` replaces the whole choice; an empty one
 * withdraws it. `isVoting` tells whether a poll's own vote is on its way, so its form waits until that
 * request settles while every other poll's form stays usable.
 */
export function usePollVote() {
	const campaignId = useCampaignId();
	const queryClient = useQueryClient();
	const [pendingPollIds, setPendingPollIds] = useState<ReadonlySet<string>>(() => new Set());
	// mutate's own callbacks fire only for the latest call, so every per-vote step hangs off the hook's.
	const mutation = api.useMutation("put", "/api/campaigns/{campaignId}/polls/{pollId}/vote", {
		onError: (error) => {
			toast.error(problemMessage(error, "Your vote couldn't be saved."));
		},
		onSettled: (_data, _error, variables) => {
			setPendingPollIds((current) => {
				const next = new Set(current);
				next.delete(variables.params.path.pollId);
				return next;
			});
			return queryClient.invalidateQueries({ queryKey: pollsQuery(campaignId).queryKey });
		},
		onSuccess: (_data, variables) => {
			toast.success(variables.body.optionIds.length === 0 ? "Vote withdrawn." : "Vote saved.");
		},
	});

	function vote(pollId: string, optionIds: string[], onSuccess?: () => void) {
		setPendingPollIds((current) => new Set(current).add(pollId));
		mutation.mutateAsync({ body: { optionIds }, params: { path: { campaignId, pollId } } }).then(
			() => onSuccess?.(),
			() => {
				// onError has already told the player.
			},
		);
	}

	return {
		isVoting: (pollId: string) => pendingPollIds.has(pollId),
		vote,
	};
}
