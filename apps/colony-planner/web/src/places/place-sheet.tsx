import { useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import { npcQueryKeyPrefix, placeQueryKeyPrefix, placesQueryKeyPrefix } from "@/api/queries";
import { BottomSheet } from "@/components/bottom-sheet";
import { Field, fieldInputProps } from "@/components/field";
import { SwitchRow, TextArea } from "@/components/form-controls";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useCampaignId } from "@/hooks/use-campaign";
import { forgetFailedSaves } from "@/lib/forms";

type Place = Schemas["Place"];
type PlaceInput = Schemas["PlaceInput"];

const nameMaxLength = 80;
const proseMaxLength = 4000;

function toDraft(place: Place | null): PlaceInput {
	return {
		description: place?.description ?? "",
		dmNotes: place?.dm?.dmNotes ?? "",
		name: place?.name ?? "",
		playerNotes: place?.playerNotes ?? "",
	};
}

/**
 * Create or fully edit a place (spec §4.4); its DM notes never reach players. Only a new place's
 * visibility is set here: an edit sheet left open must not undo a hide made since, so afterwards
 * the switch on its page is the one way to change it.
 */
export function PlaceSheet({
	open,
	onOpenChange,
	place,
	onCreated,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	/** The place to edit, or null to create one. */
	place: Place | null;
	onCreated?: (created: Place) => void;
}) {
	const queryClient = useQueryClient();
	const campaignId = useCampaignId();
	const [draft, setDraft] = useState(() => toDraft(place));
	// New places start visible, as locations always were (spec §5.9, Open Question 9).
	const [revealed, setRevealed] = useState(true);

	function settle() {
		queryClient.invalidateQueries({ queryKey: placesQueryKeyPrefix(campaignId) });
		queryClient.invalidateQueries({ queryKey: placeQueryKeyPrefix(campaignId) });
		queryClient.invalidateQueries({ queryKey: npcQueryKeyPrefix(campaignId) });
	}
	const create = api.useMutation("post", "/api/campaigns/{campaignId}/places", {
		onSettled: settle,
	});
	const update = api.useMutation("put", "/api/campaigns/{campaignId}/places/{placeId}", {
		onSettled: settle,
	});
	const error = create.error ?? update.error;
	const isSaving = create.isPending || update.isPending;
	const serverErrors = fieldErrors(error);
	const canSave = draft.name.trim() !== "" && !isSaving;
	const showGeneralError = error !== null && Object.keys(serverErrors).length === 0;

	function set<Key extends keyof PlaceInput>(key: Key, value: PlaceInput[Key]) {
		setDraft((current) => ({ ...current, [key]: value }));
		forgetFailedSaves(create, update);
	}

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!canSave) {
			return;
		}
		const body = { ...draft, name: draft.name.trim() };
		if (place) {
			update.mutate(
				{ body, params: { path: { campaignId, placeId: place.id } } },
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
			{ body: { ...body, revealed }, params: { path: { campaignId } } },
			{
				onSuccess: (created) => {
					toast.success(
						created.dm?.revealed
							? `${created.name} added.`
							: `${created.name} added, hidden from players.`,
					);
					onOpenChange(false);
					onCreated?.(created);
				},
			},
		);
	}

	let saveLabel = place ? "Save place" : "Add place";
	if (isSaving) {
		saveLabel = "Saving…";
	}

	return (
		<BottomSheet
			className="md:max-w-2xl"
			description={
				place
					? "Players see everything here except your notes, while the place is visible. Show or hide it with the switch on its page."
					: "New places start visible to players. Turn it off for somewhere the party hasn't found."
			}
			onOpenChange={onOpenChange}
			open={open}
			title={place ? `Edit ${place.name}` : "Add a place"}
		>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<Field error={serverErrors.name} id="place-name" label="Name">
					<Input
						{...fieldInputProps("place-name", serverErrors.name, false)}
						autoComplete="off"
						autoFocus={!place}
						maxLength={nameMaxLength}
						onChange={(event) => {
							set("name", event.target.value);
						}}
						value={draft.name}
					/>
				</Field>
				{place ? null : (
					<SwitchRow
						checked={revealed}
						description={
							revealed ? "Players can find it." : "Hidden: players can't find it anywhere."
						}
						id="place-revealed"
						label="Visible to players"
						onCheckedChange={(checked) => {
							setRevealed(checked);
							forgetFailedSaves(create, update);
						}}
					/>
				)}
				<Field error={serverErrors.description} id="place-description" label="Description" optional>
					<TextArea
						{...fieldInputProps("place-description", serverErrors.description, false)}
						maxLength={proseMaxLength}
						onChange={(event) => {
							set("description", event.target.value);
						}}
						value={draft.description}
					/>
				</Field>
				<Field
					description="Players see these while the place is visible."
					error={serverErrors.playerNotes}
					id="place-playerNotes"
					label="Player notes"
					optional
				>
					<TextArea
						{...fieldInputProps("place-playerNotes", serverErrors.playerNotes, true)}
						maxLength={proseMaxLength}
						onChange={(event) => {
							set("playerNotes", event.target.value);
						}}
						value={draft.playerNotes}
					/>
				</Field>
				<Field
					description="Only you see these."
					error={serverErrors.dmNotes}
					id="place-dmNotes"
					label="DM notes"
					optional
				>
					<TextArea
						{...fieldInputProps("place-dmNotes", serverErrors.dmNotes, true)}
						maxLength={proseMaxLength}
						onChange={(event) => {
							set("dmNotes", event.target.value);
						}}
						value={draft.dmNotes}
					/>
				</Field>
				{showGeneralError ? (
					<p className="text-ember-ink text-sm" role="alert">
						{problemMessage(error, "The place couldn't be saved.")}
					</p>
				) : null}
				<div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
					<Button
						onClick={() => {
							onOpenChange(false);
						}}
						type="button"
						variant="ghost"
					>
						Cancel
					</Button>
					<Button disabled={!canSave} type="submit">
						{saveLabel}
					</Button>
				</div>
			</form>
		</BottomSheet>
	);
}
