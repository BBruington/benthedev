import { useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { imageUrl } from "@/api/images";
import { fieldErrors, problemMessage } from "@/api/problem";
import { mapQueryKeyPrefix, mapsQuery } from "@/api/queries";
import { BottomSheet } from "@/components/bottom-sheet";
import { Field, fieldInputProps } from "@/components/field";
import { SwitchRow } from "@/components/form-controls";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useCampaignId } from "@/hooks/use-campaign";
import { forgetFailedSaves } from "@/lib/forms";
import { UploadImageButton } from "./upload-image-button";
import { fileSize } from "./uploads";

type WorldMap = Schemas["WorldMap"];

const nameMaxLength = 80;

export function useMapsSettled() {
	const queryClient = useQueryClient();
	const campaignId = useCampaignId();
	return () =>
		Promise.all([
			queryClient.invalidateQueries({ queryKey: mapsQuery(campaignId).queryKey }),
			queryClient.invalidateQueries({ queryKey: mapQueryKeyPrefix(campaignId) }),
		]);
}

/**
 * Adds a map by uploading its art (DM only). There is no starting map: each DM uploads their own
 * Mainland (decided 2026-10-12, C6). New maps start visible, like places (spec §5.9).
 */
export function NewMapSheet({
	open,
	onOpenChange,
	hasMainland,
	onCreated,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	/** Whether another map is already the Mainland; the first map becomes it unless the DM says no. */
	hasMainland: boolean;
	onCreated: (created: WorldMap) => void;
}) {
	const campaignId = useCampaignId();
	const settle = useMapsSettled();
	const [name, setName] = useState("");
	const [art, setArt] = useState<Schemas["UploadedImage"] | null>(null);
	const [revealed, setRevealed] = useState(true);
	const [mainland, setMainland] = useState(!hasMainland);
	const create = api.useMutation("post", "/api/campaigns/{campaignId}/maps", {
		onSettled: settle,
	});
	const serverErrors = fieldErrors(create.error);
	const showGeneralError = create.error !== null && Object.keys(serverErrors).length === 0;
	const canSave = name.trim() !== "" && art !== null && !create.isPending;

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!(canSave && art)) {
			return;
		}
		create.mutate(
			{
				body: {
					externalUrl: "",
					height: art.height,
					imageId: art.id,
					mainland,
					name: name.trim(),
					revealed,
					width: art.width,
				},
				params: { path: { campaignId } },
			},
			{
				onSuccess: (created) => {
					toast.success(
						created.dm?.revealed === false
							? `${created.name} added, hidden from players.`
							: `${created.name} added.`,
					);
					onOpenChange(false);
					onCreated(created);
				},
			},
		);
	}

	return (
		<BottomSheet
			className="md:max-w-xl"
			description="Upload the map's art: a PNG, JPEG, WebP or GIF image of at most 5 MB. Then place pins for your places on it."
			onOpenChange={onOpenChange}
			open={open}
			title="Add a map"
		>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<Field error={serverErrors.name} id="map-name" label="Name">
					<Input
						{...fieldInputProps("map-name", serverErrors.name, false)}
						autoComplete="off"
						autoFocus
						maxLength={nameMaxLength}
						onChange={(event) => {
							setName(event.target.value);
							forgetFailedSaves(create);
						}}
						placeholder="The Mainland"
						value={name}
					/>
				</Field>
				<div className="grid gap-2">
					<p className="font-medium text-sm">Art</p>
					{art ? (
						<figure className="grid gap-1.5">
							<img
								alt=""
								className="aspect-video w-full rounded-lg border border-border bg-surface-sunken object-contain"
								src={imageUrl(campaignId, art.id)}
							/>
							<figcaption className="text-muted-foreground text-sm">
								{art.width.toLocaleString()} × {art.height.toLocaleString()} px ·{" "}
								{fileSize(art.byteSize)}
							</figcaption>
						</figure>
					) : null}
					<UploadImageButton
						label={art ? "Choose different art" : "Upload art"}
						onUploaded={(uploaded) => {
							setArt(uploaded);
							forgetFailedSaves(create);
						}}
					/>
					{serverErrors.imageId ? (
						<p className="text-ember-ink text-sm">{serverErrors.imageId}</p>
					) : null}
				</div>
				<SwitchRow
					checked={mainland}
					description={
						mainland
							? "The Mainland, where the ships come from. Only one map can be it."
							: "Another map of the world."
					}
					id="map-mainland"
					label="Mainland"
					onCheckedChange={(checked) => {
						setMainland(checked);
						forgetFailedSaves(create);
					}}
				/>
				<SwitchRow
					checked={revealed}
					description={revealed ? "Players can open it." : "Hidden: players can't find it."}
					id="map-revealed"
					label="Visible to players"
					onCheckedChange={(checked) => {
						setRevealed(checked);
						forgetFailedSaves(create);
					}}
				/>
				{showGeneralError ? (
					<p className="text-ember-ink text-sm" role="alert">
						{problemMessage(create.error, "The map couldn't be added.")}
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
						{create.isPending ? "Adding…" : "Add map"}
					</Button>
				</div>
			</form>
		</BottomSheet>
	);
}

/** Renames a map (DM only). Its art, pins and visibility each change elsewhere. */
export function RenameMapSheet({
	open,
	onOpenChange,
	map,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	map: WorldMap;
}) {
	const campaignId = useCampaignId();
	const settle = useMapsSettled();
	const [name, setName] = useState(map.name);
	const update = api.useMutation("put", "/api/campaigns/{campaignId}/maps/{mapId}", {
		onSettled: settle,
	});
	const serverErrors = fieldErrors(update.error);
	const showGeneralError = update.error !== null && Object.keys(serverErrors).length === 0;
	const canSave = name.trim() !== "" && !update.isPending;

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!canSave) {
			return;
		}
		update.mutate(
			{ body: { name: name.trim() }, params: { path: { campaignId, mapId: map.id } } },
			{
				onSuccess: (saved) => {
					toast.success(`${saved.name} saved.`);
					onOpenChange(false);
				},
			},
		);
	}

	return (
		<BottomSheet onOpenChange={onOpenChange} open={open} title={`Rename ${map.name}`}>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<Field error={serverErrors.name} id="map-rename" label="Name">
					<Input
						{...fieldInputProps("map-rename", serverErrors.name, false)}
						autoComplete="off"
						autoFocus
						maxLength={nameMaxLength}
						onChange={(event) => {
							setName(event.target.value);
							forgetFailedSaves(update);
						}}
						value={name}
					/>
				</Field>
				{showGeneralError ? (
					<p className="text-ember-ink text-sm" role="alert">
						{problemMessage(update.error, "The map couldn't be renamed.")}
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
						{update.isPending ? "Saving…" : "Save name"}
					</Button>
				</div>
			</form>
		</BottomSheet>
	);
}
