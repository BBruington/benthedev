import { Trash2Icon, TriangleAlertIcon } from "lucide-react";
import { useState } from "react";
import { DeleteCampaignDialog } from "@/components/delete-campaign-dialog";
import { Button } from "@/components/ui/button";
import { SettingsCard } from "./settings-card";

/** The DM deletes their own campaign, after typing its address (C3). */
export function DangerZone({
	campaignName,
	slug,
	isDeleting,
	error,
	onDelete,
	onOpen,
}: {
	campaignName: string;
	slug: string;
	isDeleting: boolean;
	error: unknown;
	onDelete: (confirmSlug: string) => void;
	onOpen: () => void;
}) {
	const [isOpen, setOpen] = useState(false);
	return (
		<SettingsCard
			description="Deleting removes the campaign for everyone in it. Nobody's account is touched."
			icon={TriangleAlertIcon}
			id="danger-zone"
			title="Danger zone"
			tone="ember"
		>
			<div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
				<div className="min-w-0">
					<p className="font-medium">Delete this campaign</p>
					<p className="text-muted-foreground text-sm">
						The colony, its timeline, settings and memberships go for good.
					</p>
				</div>
				<Button
					className="shrink-0"
					onClick={() => {
						onOpen();
						setOpen(true);
					}}
					type="button"
					variant="destructive"
				>
					<Trash2Icon />
					Delete this campaign…
				</Button>
			</div>
			<DeleteCampaignDialog
				campaignName={campaignName}
				error={error}
				isPending={isDeleting}
				onConfirm={onDelete}
				onOpenChange={setOpen}
				open={isOpen}
				slug={slug}
			/>
		</SettingsCard>
	);
}
