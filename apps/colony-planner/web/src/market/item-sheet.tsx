import { useQueryClient } from "@tanstack/react-query";
import { ArchiveIcon, ArchiveRestoreIcon } from "lucide-react";
import { type FormEvent, useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import { marketQuery } from "@/api/queries";
import { BottomSheet } from "@/components/bottom-sheet";
import { Field, fieldInputProps } from "@/components/field";
import { TextArea } from "@/components/form-controls";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { parseWholeNumber } from "@/lib/colony";
import { forgetFailedSaves } from "@/lib/forms";
import { catalogHeadingId } from "@/market/catalog-list";
import { focusAfterClose } from "@/people/colonist-actions";
import { SheetButtons } from "@/people/workforce-sheets";

type MarketItem = Schemas["MarketItem"];

const nameMaxLength = 80;
const proseMaxLength = 4000;
const largestOrderLimit = 10000;

/**
 * Create or edit a special item (DM only): magic or one-of-a-kind goods the app records but never
 * grants, since supplies and colonists are standing offers priced from the catalog. Archiving is its
 * own button and empties carts of it.
 */
export function ItemSheet({
	open,
	onOpenChange,
	campaignId,
	item,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	campaignId: string;
	item: MarketItem | null;
}) {
	const queryClient = useQueryClient();
	const [name, setName] = useState(item?.name ?? "");
	const [description, setDescription] = useState(item?.description ?? "");
	const [priceText, setPriceText] = useState(item ? String(item.priceGold) : "");
	const [limitText, setLimitText] = useState(item?.maxPerOrder ? String(item.maxPerOrder) : "");
	const settled = {
		onSettled: () => queryClient.invalidateQueries({ queryKey: marketQuery(campaignId).queryKey }),
	};
	const create = api.useMutation("post", "/api/campaigns/{campaignId}/market/items", settled);
	const update = api.useMutation(
		"put",
		"/api/campaigns/{campaignId}/market/items/{itemId}",
		settled,
	);
	const archive = api.useMutation(
		"put",
		"/api/campaigns/{campaignId}/market/items/{itemId}/archived",
		settled,
	);
	const error = create.error ?? update.error ?? archive.error;
	const serverErrors = fieldErrors(error);
	const isSaving = create.isPending || update.isPending || archive.isPending;

	const price = parseWholeNumber(priceText);
	const priceProblem =
		price === "invalid" || (typeof price === "number" && price < 0)
			? "Enter a whole number of gold, 0 or more."
			: undefined;
	const limit = parseWholeNumber(limitText);
	const limitProblem =
		limit === "invalid" || (typeof limit === "number" && (limit < 1 || limit > largestOrderLimit))
			? "Enter a limit from 1 to 10,000, or leave it blank."
			: undefined;
	const canSave =
		name.trim() !== "" &&
		typeof price === "number" &&
		priceProblem === undefined &&
		limitProblem === undefined &&
		!isSaving;

	function edited() {
		forgetFailedSaves(create, update, archive);
	}

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!canSave) {
			return;
		}
		const body = {
			description,
			maxPerOrder: typeof limit === "number" ? limit : null,
			name,
			priceGold: typeof price === "number" ? price : 0,
		};
		if (item) {
			update.mutate(
				{ body, params: { path: { campaignId, itemId: item.id } } },
				{
					onSuccess: (saved) => {
						toast.success(`${saved.name} saved.`);
						onOpenChange(false);
					},
				},
			);
			return;
		}
		create.mutate(
			{ body, params: { path: { campaignId } } },
			{
				onSuccess: (created) => {
					toast.success(`${created.name} added.`);
					onOpenChange(false);
				},
			},
		);
	}

	function handleArchive(target: MarketItem) {
		archive.mutate(
			{ body: { archived: !target.archived }, params: { path: { campaignId, itemId: target.id } } },
			{
				onSuccess: () => {
					toast.success(
						target.archived ? `${target.name} is back.` : `${target.name} is archived.`,
					);
					onOpenChange(false);
					focusAfterClose(catalogHeadingId);
				},
			},
		);
	}

	const generalError =
		error && Object.keys(serverErrors).length === 0
			? problemMessage(error, "The item couldn't be saved.")
			: null;

	return (
		<BottomSheet
			className="md:max-w-xl"
			description="Magic and one-of-a-kind goods. The app records them in the order; you hand them over at the table."
			onOpenChange={onOpenChange}
			open={open}
			title={item ? `Edit ${item.name}` : "New special item"}
		>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<Field error={serverErrors.name} id="item-name" label="Name">
					<Input
						{...fieldInputProps("item-name", serverErrors.name, false)}
						autoComplete="off"
						autoFocus={!item}
						maxLength={nameMaxLength}
						onChange={(event) => {
							setName(event.target.value);
							edited();
						}}
						placeholder="Boots of Striding"
						value={name}
					/>
				</Field>
				<Field error={serverErrors.description} id="item-description" label="Description" optional>
					<TextArea
						{...fieldInputProps("item-description", serverErrors.description, false)}
						maxLength={proseMaxLength}
						onChange={(event) => {
							setDescription(event.target.value);
							edited();
						}}
						value={description}
					/>
				</Field>
				<div className="grid gap-5 sm:grid-cols-2">
					<Field
						error={priceProblem ?? serverErrors.priceGold}
						id="item-price"
						label="Price in gold"
					>
						<Input
							{...fieldInputProps("item-price", priceProblem ?? serverErrors.priceGold, false)}
							autoComplete="off"
							inputMode="numeric"
							onChange={(event) => {
								setPriceText(event.target.value);
								edited();
							}}
							placeholder="0"
							value={priceText}
						/>
					</Field>
					<Field
						description="For unique items."
						error={limitProblem ?? serverErrors.maxPerOrder}
						id="item-limit"
						label="Most per order"
						optional
					>
						<Input
							{...fieldInputProps("item-limit", limitProblem ?? serverErrors.maxPerOrder, true)}
							autoComplete="off"
							inputMode="numeric"
							onChange={(event) => {
								setLimitText(event.target.value);
								edited();
							}}
							value={limitText}
						/>
					</Field>
				</div>
				{generalError ? (
					<p className="text-ember-ink text-sm" role="alert">
						{generalError}
					</p>
				) : null}
				<div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
					{item ? (
						<Button
							disabled={isSaving}
							onClick={() => {
								handleArchive(item);
							}}
							type="button"
							variant="outline"
						>
							{item.archived ? <ArchiveRestoreIcon /> : <ArchiveIcon />}
							{item.archived ? "Bring back" : "Archive"}
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
						{create.isPending || update.isPending ? "Saving…" : "Save item"}
					</SheetButtons>
				</div>
			</form>
		</BottomSheet>
	);
}
