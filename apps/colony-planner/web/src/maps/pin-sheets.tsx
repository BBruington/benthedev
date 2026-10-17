import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowRightIcon, MapPinOffIcon } from "lucide-react";
import { type FormEvent, useState } from "react";
import type { Schemas } from "@/api/client";
import { placeQuery, placesQuery } from "@/api/queries";
import { BottomSheet } from "@/components/bottom-sheet";
import { Field } from "@/components/field";
import { NativeSelect } from "@/components/form-controls";
import { Button } from "@/components/ui/button";
import { useCampaign } from "@/hooks/use-campaign";

type MapPin = Schemas["MapPin"];
type Place = Schemas["Place"];

/**
 * A tapped pin (spec §6.2): the place's name and opening lines, and the way to its page. While the DM
 * edits pins, it also takes the pin off.
 */
export function PinSheet({
	open,
	onOpenChange,
	pin,
	onRemove,
	isRemoving = false,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	pin: MapPin;
	/** Only while the DM edits pins. */
	onRemove?: () => void;
	isRemoving?: boolean;
}) {
	const { campaignId, slug } = useCampaign();
	const place = useQuery(placeQuery(campaignId, pin.placeId));
	return (
		<BottomSheet onOpenChange={onOpenChange} open={open} title={pin.placeName}>
			<div className="grid gap-5">
				{place.data?.description ? (
					<p className="line-clamp-4 whitespace-pre-line text-muted-foreground">
						{place.data.description}
					</p>
				) : null}
				<div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
					{onRemove ? (
						<Button disabled={isRemoving} onClick={onRemove} variant="ghost-destructive">
							<MapPinOffIcon />
							{isRemoving ? "Removing…" : "Remove pin"}
						</Button>
					) : null}
					<Button asChild>
						<Link
							params={{ campaignSlug: slug, placeId: pin.placeId }}
							to="/c/$campaignSlug/places/$placeId"
						>
							Open {pin.placeName}
							<ArrowRightIcon />
						</Link>
					</Button>
				</div>
			</div>
		</BottomSheet>
	);
}

function placeOptionLabel(place: Place, mapId: string): string {
	let label = place.name;
	if (place.pin && place.pin.mapId === mapId) {
		label += " (moves its pin here)";
	} else if (place.pin) {
		label += ` (moves from ${place.pin.mapName})`;
	}
	if (place.dm?.revealed === false) {
		label += " · hidden";
	}
	return label;
}

/** Unpinned places first, since a tap on the map is usually for somewhere new. */
function pinOrder(places: Place[]): Place[] {
	return [...places.filter((place) => !place.pin), ...places.filter((place) => place.pin)];
}

/** The DM tapped a spot while editing pins: which place goes there (a place has one pin at most). */
export function PlacePinSheet({
	open,
	onOpenChange,
	mapId,
	onPin,
	isSaving,
	error,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	mapId: string;
	onPin: (placeId: string) => void;
	isSaving: boolean;
	error: string | null;
}) {
	const { campaignId, slug } = useCampaign();
	const places = useQuery(placesQuery(campaignId));
	const ordered = pinOrder(places.data?.places ?? []);
	const [chosenId, setChosenId] = useState<string | null>(null);
	const placeId = chosenId ?? ordered.at(0)?.id ?? "";

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (placeId !== "" && !isSaving) {
			onPin(placeId);
		}
	}

	if (places.isSuccess && ordered.length === 0) {
		return (
			<BottomSheet onOpenChange={onOpenChange} open={open} title="Pin a place here">
				<div className="grid gap-4">
					<p className="text-muted-foreground">
						There are no places to pin yet. Add one on Places, then come back and pin it.
					</p>
					<Button asChild variant="outline">
						<Link params={{ campaignSlug: slug }} to="/c/$campaignSlug/places">
							Go to Places
							<ArrowRightIcon />
						</Link>
					</Button>
				</div>
			</BottomSheet>
		);
	}

	return (
		<BottomSheet
			description="Players see the pin while both the map and the place are visible."
			onOpenChange={onOpenChange}
			open={open}
			title="Pin a place here"
		>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<Field error={error ?? undefined} id="pin-place" label="Place">
					<NativeSelect
						aria-describedby={error ? "pin-place-error" : undefined}
						aria-invalid={error ? true : undefined}
						autoFocus
						disabled={!places.isSuccess}
						id="pin-place"
						onChange={(event) => {
							setChosenId(event.target.value);
						}}
						value={placeId}
					>
						{ordered.map((place) => (
							<option key={place.id} value={place.id}>
								{placeOptionLabel(place, mapId)}
							</option>
						))}
					</NativeSelect>
				</Field>
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
					<Button disabled={placeId === "" || isSaving} type="submit">
						{isSaving ? "Pinning…" : "Pin it here"}
					</Button>
				</div>
			</form>
		</BottomSheet>
	);
}
