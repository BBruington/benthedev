import { useState } from "react";
import type { Schemas } from "@/api/client";
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

type LastChange = NonNullable<Schemas["LastChange"]>;

/**
 * Confirms Undo last change (spec §5.1), naming what it rewinds. Undo removes its own button, so the
 * caller moves focus once it settles and the dialog doesn't hand focus back to a vanished trigger.
 */
export function UndoDialog({
	lastChange,
	open,
	onOpenChange,
	onConfirm,
}: {
	lastChange: LastChange | null;
	open: boolean;
	onOpenChange: (open: boolean) => void;
	onConfirm: (lastChange: LastChange) => void;
}) {
	// Keep the label on screen while the dialog animates closed.
	const [shown, setShown] = useState(lastChange);
	if (lastChange && lastChange !== shown) {
		setShown(lastChange);
	}
	const [confirmed, setConfirmed] = useState(false);
	return (
		<AlertDialog
			onOpenChange={(nextOpen) => {
				if (nextOpen) {
					setConfirmed(false);
				}
				onOpenChange(nextOpen);
			}}
			open={open}
		>
			<AlertDialogContent
				onCloseAutoFocus={(event) => {
					if (confirmed) {
						event.preventDefault();
					}
				}}
			>
				<AlertDialogHeader>
					<AlertDialogTitle>Undo “{shown?.label ?? "the last change"}”?</AlertDialogTitle>
					<AlertDialogDescription>
						The colony goes back to how it was before, and that change's timeline entries are kept
						but marked undone. There is only one level of undo.
					</AlertDialogDescription>
				</AlertDialogHeader>
				<AlertDialogFooter>
					<AlertDialogCancel>Keep it</AlertDialogCancel>
					<AlertDialogAction
						disabled={!shown}
						onClick={() => {
							if (shown) {
								setConfirmed(true);
								onConfirm(shown);
							}
						}}
						variant="destructive"
					>
						Undo
					</AlertDialogAction>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	);
}
