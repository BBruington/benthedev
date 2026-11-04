import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import { api } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import { marketQuery } from "@/api/queries";
import { BottomSheet } from "@/components/bottom-sheet";
import { Field, fieldInputProps } from "@/components/field";
import { Input } from "@/components/ui/input";
import { parseWholeNumber } from "@/lib/colony";
import { countOf } from "@/lib/words";
import { SheetButtons } from "@/people/workforce-sheets";

const shortestVoyage = 1;
const longestVoyage = 3650;

/** The DM's voyage length: how many days after an order is placed its ship docks. */
export function VoyageSheet({
	open,
	onOpenChange,
	campaignId,
	voyageDays,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	campaignId: string;
	voyageDays: number;
}) {
	const queryClient = useQueryClient();
	const [daysText, setDaysText] = useState(String(voyageDays));
	const save = api.useMutation("put", "/api/campaigns/{campaignId}/market/voyage", {
		onSuccess: (page) => {
			queryClient.setQueryData(marketQuery(campaignId).queryKey, page);
		},
	});
	const days = parseWholeNumber(daysText);
	const isInRange = typeof days === "number" && days >= shortestVoyage && days <= longestVoyage;
	const fieldId = "market-voyage-days";
	const error =
		fieldErrors(save.error).days ??
		(daysText.trim() !== "" && !isInRange
			? `Enter a whole number from ${shortestVoyage} to ${longestVoyage}.`
			: undefined);
	const generalError =
		save.error && Object.keys(fieldErrors(save.error)).length === 0
			? problemMessage(save.error, "The voyage couldn't be saved.")
			: null;

	return (
		<BottomSheet
			onOpenChange={(next) => {
				if (!next) {
					save.reset();
				}
				onOpenChange(next);
			}}
			open={open}
			title="Voyage"
		>
			<form
				className="grid gap-5"
				onSubmit={(event) => {
					event.preventDefault();
					if (typeof days !== "number" || !isInRange) {
						return;
					}
					save.mutate(
						{ body: { days }, params: { path: { campaignId } } },
						{
							onSuccess: (page) => {
								toast.success(`Orders now take ${countOf(page.voyageDays, "day")}.`);
								onOpenChange(false);
							},
						},
					);
				}}
			>
				<Field
					description="An order docks this many days after it is placed. Orders already at sea keep their day."
					error={error}
					id={fieldId}
					label="Days at sea"
				>
					<Input
						{...fieldInputProps(fieldId, error, true)}
						inputMode="numeric"
						onChange={(event) => {
							setDaysText(event.target.value);
							save.reset();
						}}
						value={daysText}
					/>
				</Field>
				{generalError ? (
					<p className="text-ember-ink text-sm" role="alert">
						{generalError}
					</p>
				) : null}
				<SheetButtons
					canSave={isInRange && !save.isPending}
					onCancel={() => {
						onOpenChange(false);
					}}
				>
					{save.isPending ? "Saving…" : "Save voyage"}
				</SheetButtons>
			</form>
		</BottomSheet>
	);
}
