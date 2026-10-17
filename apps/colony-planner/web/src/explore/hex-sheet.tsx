import { useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useState } from "react";
import { toast } from "sonner";
import { api } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import { hexesQuery } from "@/api/queries";
import { BottomSheet } from "@/components/bottom-sheet";
import { Field, fieldInputProps } from "@/components/field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { ChartedHex } from "./hex-canvas";
import type { HexCoordinate } from "./hex-math";
import { terrainStyle } from "./terrain";
import { TerrainSwatch, useTerrainSwatches } from "./terrain-palette";

const labelMaximumLength = 40;

/** Look mode's tap (spec §6.4): a hex's terrain and its full label, which anyone may edit. */
export function HexSheet({
	campaignId,
	cell,
	hex,
	open,
	onOpenChange,
}: {
	campaignId: string;
	cell: HexCoordinate;
	/** Undefined while the hex is uncharted. */
	hex: ChartedHex | undefined;
	open: boolean;
	onOpenChange: (open: boolean) => void;
}) {
	const queryClient = useQueryClient();
	const swatches = useTerrainSwatches();
	const [label, setLabel] = useState(hex?.label ?? "");
	const save = api.useMutation("put", "/api/campaigns/{campaignId}/hexes/{q}/{r}/label", {
		onSettled: () => queryClient.invalidateQueries({ queryKey: hexesQuery(campaignId).queryKey }),
		onSuccess: () => {
			toast.success(label.trim() === "" ? "Label cleared." : "Label saved.");
			onOpenChange(false);
		},
	});
	const errors = fieldErrors(save.error);
	let formError: string | undefined;
	if (save.isError && !errors.label) {
		formError = problemMessage(save.error, "The label couldn't be saved.");
	}

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		save.mutate({
			body: { label },
			params: { path: { campaignId, q: cell.q, r: cell.r } },
		});
	}

	const coordinates = `Hex ${cell.q}, ${cell.r}`;
	if (!hex) {
		return (
			<BottomSheet
				description={coordinates}
				onOpenChange={onOpenChange}
				open={open}
				title="Uncharted"
			>
				<p className="text-base text-muted-foreground">
					Nobody has charted this hex yet. Switch to Paint and pick a terrain to chart it, then tap
					it here to give it a label.
				</p>
			</BottomSheet>
		);
	}

	const style = terrainStyle(hex.terrain);
	return (
		<BottomSheet
			description={coordinates}
			onOpenChange={onOpenChange}
			open={open}
			title={hex.label === "" ? style.label : hex.label}
		>
			<div className="grid gap-5">
				<p className="flex items-center gap-3 text-base">
					<TerrainSwatch className="size-8" swatches={swatches} terrain={hex.terrain} />
					<span>
						<span className="text-muted-foreground">Terrain: </span>
						{style.label}
					</span>
				</p>
				<form className="grid gap-3" noValidate onSubmit={handleSubmit}>
					<Field
						description="Up to 40 characters. Leave it blank to clear the label. Painting keeps it."
						error={errors.label}
						id="hex-label"
						label="Label"
					>
						<Input
							{...fieldInputProps("hex-label", errors.label, true)}
							autoComplete="off"
							maxLength={labelMaximumLength}
							onChange={(event) => {
								setLabel(event.target.value);
								if (save.isError) {
									save.reset();
								}
							}}
							value={label}
						/>
					</Field>
					{formError ? (
						<p className="text-ember-ink text-sm" role="alert">
							{formError}
						</p>
					) : null}
					<div className="flex justify-end">
						<Button disabled={save.isPending} type="submit">
							{save.isPending ? "Saving…" : "Save label"}
						</Button>
					</div>
				</form>
			</div>
		</BottomSheet>
	);
}
