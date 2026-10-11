import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "@/api/client";
import { problemMessage } from "@/api/problem";
import { meQuery } from "@/api/queries";

/** The DM's View as player switch (spec §2.1). Every other read changes projection with it. */
export function useViewAsPlayer() {
	const queryClient = useQueryClient();
	const mutation = api.useMutation("post", "/api/me/view-as-player", {
		onError: (error) => {
			toast.error(problemMessage(error, "View as player couldn't be changed."));
		},
		onSuccess: async (_data, variables) => {
			const viewingAsPlayer = variables.body.enabled;
			// The switch answers with no body: it ran under the old projection, so /api/me is read again.
			await queryClient.invalidateQueries({ queryKey: meQuery.queryKey });
			// Every other read changes projection. DM-only reads would only answer 403 while viewing
			// as a player, so they wait until the DM view returns.
			queryClient.invalidateQueries({
				predicate: (query) =>
					query.queryKey[1] !== meQuery.queryKey[1] &&
					!(viewingAsPlayer && query.meta?.dmOnly === true),
			});
			if (viewingAsPlayer) {
				toast("Viewing as a player", {
					description: "You see what the table sees. DM tools are hidden until you switch back.",
				});
			} else {
				toast("Back to the DM view");
			}
		},
	});
	return {
		isSwitching: mutation.isPending,
		setViewAsPlayer: (enabled: boolean) => {
			mutation.mutate({ body: { enabled } });
		},
	};
}
