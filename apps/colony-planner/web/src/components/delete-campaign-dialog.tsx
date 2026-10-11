import { type FormEvent, useState } from "react";
import { fieldErrors, problemMessage } from "@/api/problem";
import { Field, fieldInputProps } from "@/components/field";
import {
	AlertDialog,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

/**
 * Deleting a campaign, for the site owner (a suspended one) and for its DM (C3): it lists what goes
 * and what stays, and asks for the campaign's address typed out before the button works
 * (docs/design/multi-campaign.md §5.5). The server checks the typed address too.
 */
export function DeleteCampaignDialog({
	campaignName,
	slug,
	open,
	onOpenChange,
	isPending,
	error,
	onConfirm,
	onCloseAutoFocus,
}: {
	campaignName: string;
	slug: string;
	open: boolean;
	onOpenChange: (open: boolean) => void;
	isPending: boolean;
	error: unknown;
	onConfirm: (confirmSlug: string) => void;
	onCloseAutoFocus?: (event: Event) => void;
}) {
	const [typed, setTyped] = useState("");
	const serverError = fieldErrors(error).confirmSlug;
	const generalError =
		error && serverError === undefined
			? problemMessage(error, "The campaign couldn't be deleted.")
			: undefined;
	const matches = typed.trim() === slug;

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (matches && !isPending) {
			onConfirm(typed.trim());
		}
	}

	return (
		<AlertDialog
			onOpenChange={(nextOpen) => {
				if (!nextOpen) {
					setTyped("");
				}
				onOpenChange(nextOpen);
			}}
			open={open}
		>
			<AlertDialogContent
				className="max-h-[calc(100dvh-2rem)] overflow-y-auto"
				onCloseAutoFocus={onCloseAutoFocus}
			>
				<AlertDialogHeader>
					<AlertDialogTitle>Delete {campaignName}?</AlertDialogTitle>
					<AlertDialogDescription asChild>
						<div className="grid gap-3 text-left">
							<p>
								<span className="font-medium text-foreground">Goes for good:</span> the colony, its
								timeline, settings, resources and roles, every membership, the invite link and the
								Discord link.
							</p>
							<p>
								<span className="font-medium text-foreground">Stays:</span> everyone's account and
								their other campaigns.
							</p>
							<p>This can't be undone.</p>
						</div>
					</AlertDialogDescription>
				</AlertDialogHeader>
				<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
					<Field
						error={serverError}
						id="delete-campaign-slug"
						label={
							<>
								Type <span className="font-mono text-ember-ink">{slug}</span> to confirm
							</>
						}
					>
						<Input
							{...fieldInputProps("delete-campaign-slug", serverError, false)}
							autoCapitalize="none"
							autoComplete="off"
							onChange={(event) => {
								setTyped(event.target.value);
							}}
							spellCheck={false}
							value={typed}
						/>
					</Field>
					{generalError ? (
						<p className="text-ember-ink text-sm" role="alert">
							{generalError}
						</p>
					) : null}
					<AlertDialogFooter>
						<AlertDialogCancel type="button">Keep it</AlertDialogCancel>
						<Button disabled={!matches || isPending} type="submit" variant="destructive">
							{isPending ? "Deleting…" : "Delete the campaign"}
						</Button>
					</AlertDialogFooter>
				</form>
			</AlertDialogContent>
		</AlertDialog>
	);
}
