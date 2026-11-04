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
const fewestShips = 1;
const mostShips = 100;

/** The DM's shipping: how many days an order is at sea, and how many orders may be out at once. */
export function ShippingSheet({
	open,
	onOpenChange,
	campaignId,
	voyageDays,
	shipLimit,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	campaignId: string;
	voyageDays: number;
	shipLimit: number;
}) {
	const queryClient = useQueryClient();
	const [daysText, setDaysText] = useState(String(voyageDays));
	const [shipsText, setShipsText] = useState(String(shipLimit));
	const save = api.useMutation("put", "/api/campaigns/{campaignId}/market/shipping", {
		onSuccess: (page) => {
			queryClient.setQueryData(marketQuery(campaignId).queryKey, page);
		},
	});
	const days = parseWholeNumber(daysText);
	const ships = parseWholeNumber(shipsText);
	const isDaysInRange = typeof days === "number" && days >= shortestVoyage && days <= longestVoyage;
	const isShipsInRange = typeof ships === "number" && ships >= fewestShips && ships <= mostShips;
	const daysFieldId = "market-voyage-days";
	const shipsFieldId = "market-ship-limit";
	const serverErrors = fieldErrors(save.error);
	const daysError =
		serverErrors.voyageDays ??
		(daysText.trim() !== "" && !isDaysInRange
			? `Enter a whole number from ${shortestVoyage} to ${longestVoyage}.`
			: undefined);
	const shipsError =
		serverErrors.shipLimit ??
		(shipsText.trim() !== "" && !isShipsInRange
			? `Enter a whole number from ${fewestShips} to ${mostShips}.`
			: undefined);
	const generalError =
		save.error && Object.keys(serverErrors).length === 0
			? problemMessage(save.error, "The shipping settings couldn't be saved.")
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
			title="Shipping"
		>
			<form
				className="grid gap-5"
				onSubmit={(event) => {
					event.preventDefault();
					if (
						typeof days !== "number" ||
						typeof ships !== "number" ||
						!isDaysInRange ||
						!isShipsInRange
					) {
						return;
					}
					save.mutate(
						{ body: { shipLimit: ships, voyageDays: days }, params: { path: { campaignId } } },
						{
							onSuccess: (page) => {
								toast.success(
									`Orders now take ${countOf(page.voyageDays, "day")}, with up to ${countOf(page.shipLimit, "ship")} out.`,
								);
								onOpenChange(false);
							},
						},
					);
				}}
			>
				<Field
					description="An order docks this many days after it is placed. Orders already at sea keep their day."
					error={daysError}
					id={daysFieldId}
					label="Days at sea"
				>
					<Input
						{...fieldInputProps(daysFieldId, daysError, true)}
						inputMode="numeric"
						onChange={(event) => {
							setDaysText(event.target.value);
							save.reset();
						}}
						value={daysText}
					/>
				</Field>
				<Field
					description="How many orders may be out at once. Lowering it leaves ships already out sailing."
					error={shipsError}
					id={shipsFieldId}
					label="Ships"
				>
					<Input
						{...fieldInputProps(shipsFieldId, shipsError, true)}
						inputMode="numeric"
						onChange={(event) => {
							setShipsText(event.target.value);
							save.reset();
						}}
						value={shipsText}
					/>
				</Field>
				{generalError ? (
					<p className="text-ember-ink text-sm" role="alert">
						{generalError}
					</p>
				) : null}
				<SheetButtons
					canSave={isDaysInRange && isShipsInRange && !save.isPending}
					onCancel={() => {
						onOpenChange(false);
					}}
				>
					{save.isPending ? "Saving…" : "Save shipping"}
				</SheetButtons>
			</form>
		</BottomSheet>
	);
}
