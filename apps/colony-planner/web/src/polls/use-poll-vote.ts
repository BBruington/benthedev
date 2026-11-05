import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "@/api/client";
import { problemMessage } from "@/api/problem";
import { pollsQuery } from "@/api/queries";
import { useCampaignId } from "@/hooks/use-campaign";

/**
 * A player's vote on the Polls page and the board. `vote` replaces the whole choice; an empty one
 * withdraws it. `votingPollId` is the poll whose vote is on its way, so only its form waits.
 */
export function usePollVote() {
	const campaignId = useCampaignId();
	const queryClient = useQueryClient();
	const mutation = api.useMutation("put", "/api/campaigns/{campaignId}/polls/{pollId}/vote", {
		onError: (error) => {
			toast.error(problemMessage(error, "Your vote couldn't be saved."));
		},
		onSettled: () => queryClient.invalidateQueries({ queryKey: pollsQuery(campaignId).queryKey }),
	});

	function vote(pollId: string, optionIds: string[], onSuccess?: () => void) {
		mutation.mutate(
			{ body: { optionIds }, params: { path: { campaignId, pollId } } },
			{
				onSuccess: () => {
					toast.success(optionIds.length === 0 ? "Vote withdrawn." : "Vote saved.");
					onSuccess?.();
				},
			},
		);
	}

	return {
		vote,
		votingPollId: mutation.isPending ? mutation.variables?.params.path.pollId : undefined,
	};
}
