import { useQueryClient } from "@tanstack/react-query";
import { ArchiveIcon } from "lucide-react";
import { type FormEvent, useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import { marketQuery } from "@/api/queries";
import { BottomSheet } from "@/components/bottom-sheet";
import { Field, fieldInputProps } from "@/components/field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { parseWholeNumber } from "@/lib/colony";
import { forgetFailedSaves } from "@/lib/forms";
import { catalogHeadingId, type OfferTarget } from "@/market/catalog-list";
import { focusAfterClose } from "@/people/colonist-actions";
import { SheetButtons } from "@/people/workforce-sheets";

const largestAmount = 1_000_000;

/**
 * The DM's price for one resource or role (DM only). Saving puts it on sale, or back on sale; Stop
 * selling archives the offer and empties carts of it. Orders keep the price they were placed at.
 */
export function OfferSheet({
	open,
	onOpenChange,
	campaignId,
	target,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	campaignId: string;
	target: OfferTarget;
}) {
	const queryClient = useQueryClient();
	const offer = target.offer;
	const isForSale = offer !== null && !offer.archived;
	const [priceText, setPriceText] = useState(offer ? String(offer.priceGold) : "");
	const [amountText, setAmountText] = useState(String(offer?.amount ?? 1));
	const showPage = {
		onSuccess: (page: Schemas["MarketPage"]) => {
			queryClient.setQueryData(marketQuery(campaignId).queryKey, page);
		},
	};
	const supply = api.useMutation(
		"put",
		"/api/campaigns/{campaignId}/market/supplies/{resourceKey}",
		showPage,
	);
	const colonist = api.useMutation(
		"put",
		"/api/campaigns/{campaignId}/market/colonists/{roleKey}",
		showPage,
	);
	const archive = api.useMutation(
		"put",
		"/api/campaigns/{campaignId}/market/items/{itemId}/archived",
		{
			onSettled: () =>
				queryClient.invalidateQueries({ queryKey: marketQuery(campaignId).queryKey }),
		},
	);
	const error = supply.error ?? colonist.error ?? archive.error;
	const serverErrors = fieldErrors(error);
	const isSaving = supply.isPending || colonist.isPending || archive.isPending;

	const price = parseWholeNumber(priceText);
	const priceProblem =
		price === "invalid" || (typeof price === "number" && price < 0)
			? "Enter a whole number of gold, 0 or more."
			: undefined;
	const amount = parseWholeNumber(amountText);
	const amountProblem =
		target.kind === "SUPPLY" &&
		(amount === "invalid" || (typeof amount === "number" && (amount < 1 || amount > largestAmount)))
			? "Enter an amount from 1 to 1,000,000."
			: undefined;
	const canSave =
		typeof price === "number" &&
		priceProblem === undefined &&
		(target.kind === "COLONIST" || (typeof amount === "number" && amountProblem === undefined)) &&
		!isSaving;

	function edited() {
		forgetFailedSaves(supply, colonist, archive);
	}

	function saved() {
		toast.success(`${target.label} is for sale.`);
		onOpenChange(false);
	}

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!canSave || typeof price !== "number") {
			return;
		}
		if (target.kind === "SUPPLY") {
			if (typeof amount !== "number") {
				return;
			}
			supply.mutate(
				{
					body: { amount, priceGold: price },
					params: { path: { campaignId, resourceKey: target.key } },
				},
				{ onSuccess: saved },
			);
			return;
		}
		colonist.mutate(
			{ body: { priceGold: price }, params: { path: { campaignId, roleKey: target.key } } },
			{ onSuccess: saved },
		);
	}

	function handleStopSelling(itemId: string) {
		archive.mutate(
			{ body: { archived: true }, params: { path: { campaignId, itemId } } },
			{
				onSuccess: () => {
					toast.success(`${target.label} is no longer for sale.`);
					onOpenChange(false);
					focusAfterClose(catalogHeadingId);
				},
			},
		);
	}

	const generalError =
		error && Object.keys(serverErrors).length === 0
			? problemMessage(error, "The price couldn't be saved.")
			: null;
	const unitText = target.kind === "SUPPLY" ? `${target.label} per purchase` : null;

	return (
		<BottomSheet
			description={
				target.kind === "SUPPLY"
					? "Players buy it in whole purchases. An order keeps the price it was placed at."
					: "Each one bought joins this role when the ship is paid for. An order keeps the price it was placed at."
			}
			onOpenChange={onOpenChange}
			open={open}
			title={`${target.label} price`}
		>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<div className="grid gap-5 sm:grid-cols-2">
					<Field
						error={priceProblem ?? serverErrors.priceGold}
						id="offer-price"
						label="Price in gold"
					>
						<Input
							{...fieldInputProps("offer-price", priceProblem ?? serverErrors.priceGold, false)}
							autoComplete="off"
							autoFocus
							inputMode="numeric"
							onChange={(event) => {
								setPriceText(event.target.value);
								edited();
							}}
							placeholder="0"
							value={priceText}
						/>
					</Field>
					{unitText ? (
						<Field error={amountProblem ?? serverErrors.amount} id="offer-amount" label={unitText}>
							<Input
								{...fieldInputProps("offer-amount", amountProblem ?? serverErrors.amount, false)}
								autoComplete="off"
								inputMode="numeric"
								onChange={(event) => {
									setAmountText(event.target.value);
									edited();
								}}
								value={amountText}
							/>
						</Field>
					) : null}
				</div>
				{generalError ? (
					<p className="text-ember-ink text-sm" role="alert">
						{generalError}
					</p>
				) : null}
				<div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
					{offer && isForSale ? (
						<Button
							disabled={isSaving}
							onClick={() => {
								handleStopSelling(offer.id);
							}}
							type="button"
							variant="outline"
						>
							<ArchiveIcon />
							Stop selling
						</Button>
					) : (
						<span />
					)}
					<SheetButtons
						canSave={canSave}
						onCancel={() => {
							onOpenChange(false);
						}}
					>
						{supply.isPending || colonist.isPending ? "Saving…" : "Save price"}
					</SheetButtons>
				</div>
			</form>
		</BottomSheet>
	);
}
