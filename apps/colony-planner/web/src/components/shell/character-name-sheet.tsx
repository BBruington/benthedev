import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "@/api/client";
import { campaignMeQuery, myCampaignsQuery } from "@/api/queries";
import { BottomSheet } from "@/components/bottom-sheet";
import { CharacterNameForm } from "@/components/character-name-form";
import { useCampaignId } from "@/hooks/use-campaign";
import type { Me } from "@/lib/access";

/**
 * The caller's own character name in this campaign (PUT .../me/profile replaces the whole
 * profile); each campaign has its own.
 */
export function CharacterNameSheet({
	me,
	open,
	onOpenChange,
}: {
	me: Me;
	open: boolean;
	onOpenChange: (open: boolean) => void;
}) {
	return (
		<BottomSheet
			description="How the table knows you. The DM and the other players see it beside your Discord name."
			onOpenChange={onOpenChange}
			open={open}
			title="Your character"
		>
			<OwnCharacterNameForm
				initialName={me.member.characterName ?? ""}
				onDone={() => {
					onOpenChange(false);
				}}
			/>
		</BottomSheet>
	);
}

function OwnCharacterNameForm({
	initialName,
	onDone,
}: {
	initialName: string;
	onDone: () => void;
}) {
	const queryClient = useQueryClient();
	const campaignId = useCampaignId();
	const mutation = api.useMutation("put", "/api/campaigns/{campaignId}/me/profile", {
		onSuccess: (me) => {
			queryClient.setQueryData(campaignMeQuery(campaignId).queryKey, me);
			queryClient.invalidateQueries({ queryKey: myCampaignsQuery.queryKey });
			if (me.member.characterName) {
				toast.success(`You're playing ${me.member.characterName}.`);
			} else {
				toast.success("Character name cleared.");
			}
			onDone();
		},
	});

	return (
		<CharacterNameForm
			description="Leave it empty if you haven't settled on one."
			error={mutation.error}
			failureMessage="Your character name couldn't be saved."
			id="profile-character-name"
			initialName={initialName}
			isPending={mutation.isPending}
			onCancel={onDone}
			onSubmit={(characterName) => {
				mutation.mutate({ body: { characterName }, params: { path: { campaignId } } });
			}}
			placeholder="For example, Isolde Varr"
		/>
	);
}
