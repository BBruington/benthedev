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

const confirmCopy = {
	close: {
		action: "Close poll",
		cancel: "Keep it open",
		description: "Voting ends now, and the final counts go on the timeline.",
		title: "Close this poll?",
	},
	delete: {
		action: "Delete poll",
		cancel: "Keep it",
		description:
			"The poll, its options and every vote are removed. A timeline entry from its closing stays.",
		title: "Delete this poll?",
	},
};

export type PollConfirmKind = keyof typeof confirmCopy;

/**
 * The DM's Close and Delete confirm. The poll's button moves or goes, so the caller places focus.
 * Keep `kind` and `question` set while it closes, as useSheetState does, so its words stay put.
 */
export function PollConfirmDialog({
	open,
	kind,
	question,
	onOpenChange,
	onConfirm,
}: {
	open: boolean;
	kind: PollConfirmKind;
	question: string;
	onOpenChange: (open: boolean) => void;
	onConfirm: () => void;
}) {
	const [confirmed, setConfirmed] = useState(false);
	const copy = confirmCopy[kind];
	return (
		<AlertDialog onOpenChange={onOpenChange} open={open}>
			<AlertDialogContent
				onCloseAutoFocus={(event) => {
					if (confirmed) {
						event.preventDefault();
						setConfirmed(false);
					}
				}}
			>
				<AlertDialogHeader>
					<AlertDialogTitle>{copy.title}</AlertDialogTitle>
					<AlertDialogDescription>
						<span className="font-medium text-foreground">{question}</span> {copy.description}
					</AlertDialogDescription>
				</AlertDialogHeader>
				<AlertDialogFooter>
					<AlertDialogCancel>{copy.cancel}</AlertDialogCancel>
					<AlertDialogAction
						onClick={() => {
							setConfirmed(true);
							onConfirm();
						}}
						variant={kind === "delete" ? "destructive" : "default"}
					>
						{copy.action}
					</AlertDialogAction>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	);
}
