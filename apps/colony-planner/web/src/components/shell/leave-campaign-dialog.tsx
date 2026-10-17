import { type QueryClient, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { toast } from "sonner";
import { api } from "@/api/client";
import { problemMessage } from "@/api/problem";
import { campaignMeQuery, myCampaignsQuery } from "@/api/queries";
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
import { forgetLastCampaign } from "@/lib/last-campaign";
import { campaignPickerHeadingId } from "./account-ids";

// openapi-react-query keys every mutation by its method and path.
const leaveMutationKey = ["post", "/api/campaigns/{campaignId}/leave"];

/**
 * True while this tab is leaving the campaign. The live "me" topic can arrive before the dialog has
 * taken the leaver to the picker, and the campaign's shell must not send them to the waiting page in
 * the meantime. The leave stays pending until the dialog's navigation has finished.
 */
export function isLeavingCampaign(queryClient: QueryClient, campaignId: string): boolean {
	const leaving = queryClient.isMutating({
		mutationKey: leaveMutationKey,
		predicate: (mutation) => JSON.stringify(mutation.state.variables ?? null).includes(campaignId),
	});
	return leaving > 0;
}

/**
 * A player leaves on their own (docs/design/multi-campaign.md C5): only themselves, from waiting or
 * playing, never the DM. Leaving is final until the DM re-admits them, so it asks first. The picker
 * opens afterwards, with focus on its heading, since the campaign's screens are closed to them now.
 */
export function LeaveCampaignDialog({
	campaignId,
	campaignName,
	isWaiting,
	open,
	onOpenChange,
}: {
	campaignId: string;
	campaignName: string;
	isWaiting: boolean;
	open: boolean;
	onOpenChange: (open: boolean) => void;
}) {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const mutation = api.useMutation("post", "/api/campaigns/{campaignId}/leave", {
		onError: (error) => {
			toast.error(problemMessage(error, "Leaving didn't work. Try again."));
		},
		onSuccess: async (campaignMe) => {
			forgetLastCampaign();
			onOpenChange(false);
			await navigate({ to: "/" });
			queryClient.setQueryData(campaignMeQuery(campaignId).queryKey, campaignMe);
			queryClient.invalidateQueries({ queryKey: myCampaignsQuery.queryKey });
			toast.success(
				isWaiting ? `You stopped waiting for ${campaignName}.` : `You left ${campaignName}.`,
			);
			document.getElementById(campaignPickerHeadingId)?.focus();
		},
	});

	const title = isWaiting ? `Stop waiting for ${campaignName}?` : `Leave ${campaignName}?`;
	const confirmLabel = isWaiting ? "Stop waiting" : "Leave the campaign";

	return (
		<AlertDialog onOpenChange={onOpenChange} open={open}>
			<AlertDialogContent
				onCloseAutoFocus={(event) => {
					// A successful leave moves focus to the picker itself.
					if (mutation.isSuccess) {
						event.preventDefault();
					}
				}}
			>
				<AlertDialogHeader>
					<AlertDialogTitle>{title}</AlertDialogTitle>
					<AlertDialogDescription>
						Only the DM can let you back in. Opening the invite link again won't, and won't let them
						know. Your account and your other campaigns stay as they are.
					</AlertDialogDescription>
				</AlertDialogHeader>
				<AlertDialogFooter>
					<AlertDialogCancel>Stay</AlertDialogCancel>
					<AlertDialogAction
						disabled={mutation.isPending}
						onClick={(event) => {
							// The dialog stays open until the server answers.
							event.preventDefault();
							mutation.mutate({ params: { path: { campaignId } } });
						}}
						variant="destructive"
					>
						{mutation.isPending ? "Leaving…" : confirmLabel}
					</AlertDialogAction>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	);
}
