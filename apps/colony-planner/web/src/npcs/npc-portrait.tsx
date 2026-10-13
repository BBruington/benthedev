import { useQueryClient } from "@tanstack/react-query";
import { ImageOffIcon } from "lucide-react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { problemMessage } from "@/api/problem";
import { npcQueryKeyPrefix, npcsQueryKeyPrefix } from "@/api/queries";
import { Button } from "@/components/ui/button";
import { useCampaignId } from "@/hooks/use-campaign";
import { focusFirstVisible } from "@/lib/focus";
import { UploadImageButton } from "@/maps/upload-image-button";
import { NpcAvatar } from "@/people/people-sections";

const portraitUploadId = "npc-portrait-upload";

/**
 * Sets or removes an NPC's portrait (DM only). It saves the moment the image uploads, apart from the
 * sheet's Save, because it has its own endpoint. Players see it only while the NPC is visible.
 */
export function NpcPortraitField({ npc }: { npc: Schemas["Npc"] }) {
	const campaignId = useCampaignId();
	const queryClient = useQueryClient();
	const mutation = api.useMutation("put", "/api/campaigns/{campaignId}/npcs/{npcId}/portrait", {
		onError: (error) => {
			toast.error(problemMessage(error, "The portrait couldn't be saved."));
		},
		// Returned, so the avatar keeps the new portrait until the refetch lands.
		onSettled: () =>
			Promise.all([
				queryClient.invalidateQueries({ queryKey: npcsQueryKeyPrefix(campaignId) }),
				queryClient.invalidateQueries({ queryKey: npcQueryKeyPrefix(campaignId) }),
			]),
		onSuccess: (saved) => {
			toast.success(
				saved.portraitImageId
					? `${saved.displayName}'s portrait is set.`
					: `${saved.displayName}'s portrait was removed.`,
			);
		},
	});
	const shownPortraitId = mutation.isPending
		? (mutation.variables?.body?.imageId ?? null)
		: npc.portraitImageId;

	function savePortrait(imageId: string | null) {
		mutation.mutate(
			{ body: { imageId }, params: { path: { campaignId, npcId: npc.id } } },
			{
				onSuccess: () => {
					if (imageId === null) {
						// Upload stays disabled until the save's render lands, so focus waits a frame for it.
						requestAnimationFrame(() => {
							focusFirstVisible(portraitUploadId);
						});
					}
				},
			},
		);
	}

	return (
		<fieldset className="grid gap-3">
			<legend className="mb-1 font-medium text-gilt-ink text-xs uppercase tracking-[0.18em]">
				Portrait
			</legend>
			<div className="flex flex-wrap items-start gap-4">
				<NpcAvatar displayName={npc.displayName} portraitImageId={shownPortraitId} size="xl" />
				<div className="flex flex-wrap gap-2">
					<UploadImageButton
						busy={mutation.isPending}
						id={portraitUploadId}
						label={shownPortraitId ? "Replace portrait" : "Upload portrait"}
						onUploaded={(image) => {
							savePortrait(image.id);
						}}
					/>
					{shownPortraitId ? (
						<Button
							disabled={mutation.isPending}
							onClick={() => {
								savePortrait(null);
							}}
							type="button"
							variant="ghost-destructive"
						>
							<ImageOffIcon />
							Remove portrait
						</Button>
					) : null}
				</div>
			</div>
			<p className="text-muted-foreground text-sm">
				Saved as soon as it uploads. Without a portrait, the avatar shows initials.
			</p>
		</fieldset>
	);
}
