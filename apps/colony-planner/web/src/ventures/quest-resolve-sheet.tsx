import { useQuery } from "@tanstack/react-query";
import { BadgeCheckIcon, CircleXIcon } from "lucide-react";
import type { Schemas } from "@/api/client";
import { problemMessage } from "@/api/problem";
import { completeQuestPreviewQuery, failQuestPreviewQuery } from "@/api/queries";
import { type ColonyChanges, isConflict } from "@/colony/use-colony-changes";
import { BottomSheet } from "@/components/bottom-sheet";
import { Button } from "@/components/ui/button";
import { ChangePreview } from "./venture-parts";

type Quest = Schemas["Quest"];
export type Resolution = "complete" | "fail";

/**
 * Complete or Fail with a preview first (spec §5.6): "Food 40 → 60 · +3 to Unassigned · Lady Vex
 * favor 1 → 3". Confirming is one colony change, undoable while it's the latest; a second click
 * (another tab, another device) gets "Already resolved" and pays nothing.
 */
export function QuestResolveSheet({
	open,
	onOpenChange,
	quest,
	resolution,
	changes,
	onResolved,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	quest: Quest;
	resolution: Resolution;
	changes: ColonyChanges;
	onResolved: () => void;
}) {
	const campaignId = changes.inCampaign.params.path.campaignId;
	const isComplete = resolution === "complete";
	const completePreview = useQuery({
		...completeQuestPreviewQuery(campaignId, quest.id),
		enabled: open && isComplete,
	});
	const failPreview = useQuery({
		...failQuestPreviewQuery(campaignId, quest.id),
		enabled: open && !isComplete,
	});
	const preview = isComplete ? completePreview : failPreview;
	const mutation = isComplete ? changes.completeQuest : changes.failQuest;
	const canConfirm = preview.data !== undefined && !preview.isFetching && !mutation.isPending;

	function confirm() {
		mutation.mutate(
			{ params: { path: { campaignId, questId: quest.id } } },
			{
				onSuccess: () => {
					onOpenChange(false);
					onResolved();
				},
			},
		);
	}

	let confirmLabel = isComplete ? "Complete quest" : "Fail quest";
	if (mutation.isPending) {
		confirmLabel = "Saving…";
	}
	let description = isComplete
		? "Pays the reward now. Gains stop at each resource's maximum; new colonists land in Unassigned."
		: "Takes the failure cost now. Losses stop at each resource's minimum; favor may go below 0.";
	if (quest.dm?.revealed === false) {
		description += " The quest is hidden, so only you see the timeline entry.";
	}

	return (
		<BottomSheet
			description={description}
			footer={
				<>
					<Button
						onClick={() => {
							onOpenChange(false);
						}}
						variant="ghost"
					>
						Cancel
					</Button>
					<Button
						disabled={!canConfirm}
						onClick={confirm}
						variant={isComplete ? "default" : "destructive"}
					>
						{isComplete ? <BadgeCheckIcon /> : <CircleXIcon />}
						{confirmLabel}
					</Button>
				</>
			}
			onOpenChange={(next) => {
				if (!next) {
					mutation.reset();
				}
				onOpenChange(next);
			}}
			open={open}
			title={isComplete ? `Complete ${quest.title}?` : `Fail ${quest.title}?`}
		>
			<div className="grid gap-4">
				<ChangePreview
					emptyText={
						isComplete
							? "The reward changes nothing in the stockpile."
							: "No failure cost: nothing is taken."
					}
					error={preview.error}
					isPending={preview.isPending}
					preview={preview.data}
				/>
				{mutation.error ? (
					<p
						className="rounded-lg border border-ember/40 bg-ember/10 px-3 py-2 text-ember-ink text-sm"
						role="alert"
					>
						{problemMessage(mutation.error, "The quest couldn't be resolved.")}
						{isConflict(mutation.error) ? " The page has been refreshed." : null}
					</p>
				) : null}
			</div>
		</BottomSheet>
	);
}
