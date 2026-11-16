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

/**
 * The DM's confirm for Add building (construction design K19). Its trigger stays on the blueprint
 * card, so focus goes back to it. Keep `name` set while it closes, as useSheetState does.
 */
export function AddBuildingDialog({
	open,
	name,
	onOpenChange,
	onConfirm,
}: {
	open: boolean;
	name: string;
	onOpenChange: (open: boolean) => void;
	onConfirm: () => void;
}) {
	return (
		<AlertDialog onOpenChange={onOpenChange} open={open}>
			<AlertDialogContent>
				<AlertDialogHeader>
					<AlertDialogTitle>Add {name}?</AlertDialogTitle>
					<AlertDialogDescription>
						It stands finished from today, for free: nothing leaves the stockpile, no crew works on
						it and its completion effects don't apply. Its storage counts at once and its daily
						output starts tomorrow.
					</AlertDialogDescription>
				</AlertDialogHeader>
				<AlertDialogFooter>
					<AlertDialogCancel>Not now</AlertDialogCancel>
					<AlertDialogAction onClick={onConfirm}>Add building</AlertDialogAction>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	);
}
