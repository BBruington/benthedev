import { CopyIcon, LinkIcon, RefreshCwIcon } from "lucide-react";
import { useState } from "react";
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
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { SettingsCard } from "./settings-card";

/**
 * The campaign's invite link (docs/design/multi-campaign.md §5.3): anyone who opens it can ask to
 * join, and the DM lets them in. Resetting it locks out everyone holding the old one.
 */
export function InviteLinkCard({
	inviteUrl,
	loadError,
	isResetting,
	onCopy,
	onReset,
}: {
	inviteUrl: string | undefined;
	loadError: string | undefined;
	isResetting: boolean;
	onCopy: (inviteUrl: string) => void;
	onReset: () => void;
}) {
	const [isConfirmOpen, setConfirmOpen] = useState(false);
	let body = <Skeleton className="h-11 rounded-md" />;
	if (loadError) {
		body = <p className="text-muted-foreground text-sm">{loadError}</p>;
	} else if (inviteUrl) {
		body = (
			<div className="grid gap-3">
				<label className="sr-only" htmlFor="settings-invite-link">
					Invite link
				</label>
				<Input
					className="font-mono text-sm"
					id="settings-invite-link"
					onFocus={(event) => {
						event.target.select();
					}}
					readOnly
					value={inviteUrl}
				/>
				<div className="flex flex-wrap gap-2">
					<Button
						onClick={() => {
							onCopy(inviteUrl);
						}}
						type="button"
					>
						<CopyIcon />
						Copy
					</Button>
					<Button
						disabled={isResetting}
						onClick={() => {
							setConfirmOpen(true);
						}}
						type="button"
						variant="outline"
					>
						<RefreshCwIcon />
						{isResetting ? "Resetting…" : "Reset link…"}
					</Button>
				</div>
			</div>
		);
	}
	return (
		<SettingsCard
			description="Share it with your players. Anyone who opens it can ask to join; only you let them in."
			icon={LinkIcon}
			id="invite"
			title="Invite link"
		>
			{body}
			<AlertDialog onOpenChange={setConfirmOpen} open={isConfirmOpen}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>Reset the invite link?</AlertDialogTitle>
						<AlertDialogDescription>
							The old link stops working at once. People already waiting stay waiting.
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>Keep this link</AlertDialogCancel>
						<AlertDialogAction onClick={onReset} variant="destructive">
							Reset link
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</SettingsCard>
	);
}
