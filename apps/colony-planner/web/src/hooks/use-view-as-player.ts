import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "@/api/client";
import { problemMessage } from "@/api/problem";
import { switchProjection } from "@/lib/projection";

/** The DM's View as player switch (spec §2.1). Every other read changes projection with it. */
export function useViewAsPlayer() {
	const queryClient = useQueryClient();
	const mutation = api.useMutation("post", "/api/me/view-as-player", {
		onError: (error) => {
			toast.error(problemMessage(error, "View as player couldn't be changed."));
		},
		onSuccess: async (_data, variables) => {
			const viewingAsPlayer = variables.body.enabled;
			await switchProjection(queryClient, viewingAsPlayer);
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
