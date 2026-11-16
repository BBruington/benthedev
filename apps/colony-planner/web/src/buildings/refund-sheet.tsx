import { BanIcon, BrickWallIcon } from "lucide-react";
import { type FormEvent, useState } from "react";
import type { Schemas } from "@/api/client";
import { fieldErrors } from "@/api/problem";
import type { ColonyChanges } from "@/colony/use-colony-changes";
import { BottomSheet } from "@/components/bottom-sheet";
import { Button } from "@/components/ui/button";
import { type NumberDraft, parseAmounts, refundPrefill } from "@/lib/construction";
import { ChangeError, useReviewedVersion } from "@/people/workforce-sheets";
import { AmountsEditor } from "./amounts-editor";

type Building = Schemas["Building"];
type ColonyResource = Schemas["ColonyResource"];

export type RefundKind = "cancel" | "demolish";

const refundFormId = "building-refund-form";

/**
 * The DM's Cancel (a project, prefilled with the full cost) or Demolish (a standing building,
 * prefilled empty). The app has no salvage rate: whatever the DM types comes back, each gain stopping
 * at its maximum (construction design K4).
 */
export function RefundSheet({
	open,
	onOpenChange,
	kind,
	building,
	resources,
	version,
	changes,
	onDone,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	kind: RefundKind;
	building: Building;
	resources: readonly ColonyResource[];
	version: number;
	changes: ColonyChanges;
	onDone: () => void;
}) {
	const mutation = kind === "cancel" ? changes.cancelBuilding : changes.demolishBuilding;
	const reviewedVersion = useReviewedVersion(version, mutation);
	const [prefill] = useState(() =>
		kind === "cancel" ? refundPrefill(building.cost, resources) : { draft: {}, leftOut: [] },
	);
	const [draft, setDraft] = useState<NumberDraft>(prefill.draft);
	const { amounts, problems } = parseAmounts(draft);
	const serverErrors = fieldErrors(mutation.error);
	const canConfirm = Object.keys(problems).length === 0 && !mutation.isPending;
	const isCancel = kind === "cancel";

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!canConfirm) {
			return;
		}
		mutation.mutate(
			{
				body: { expectedVersion: reviewedVersion, refund: amounts },
				params: { path: { ...changes.inCampaign.params.path, buildingId: building.id } },
			},
			{
				onSuccess: () => {
					onOpenChange(false);
					onDone();
				},
			},
		);
	}

	let confirmLabel = isCancel ? "Cancel the project" : "Demolish it";
	if (mutation.isPending) {
		confirmLabel = isCancel ? "Cancelling…" : "Demolishing…";
	}

	return (
		<BottomSheet
			className="md:max-w-xl"
			description={
				isCancel
					? "Its crews go back to idle. Type what the colony gets back, down to 0; gains stop at each maximum."
					: "It stops standing. Type anything the colony gets back; gains stop at each maximum."
			}
			footer={
				<>
					<Button
						onClick={() => {
							onOpenChange(false);
						}}
						variant="ghost"
					>
						{isCancel ? "Keep building" : "Keep it"}
					</Button>
					<Button disabled={!canConfirm} form={refundFormId} type="submit" variant="destructive">
						{isCancel ? <BanIcon /> : <BrickWallIcon />}
						{confirmLabel}
					</Button>
				</>
			}
			onOpenChange={onOpenChange}
			open={open}
			title={isCancel ? `Cancel the ${building.name}` : `Demolish the ${building.name}`}
		>
			<form className="grid gap-4" id={refundFormId} noValidate onSubmit={handleSubmit}>
				<fieldset className="grid gap-2">
					<legend className="mb-2 font-medium">Refund</legend>
					<AmountsEditor
						draft={draft}
						idPrefix="building-refund"
						onChange={(next) => {
							setDraft(next);
							if (mutation.isError) {
								mutation.reset();
							}
						}}
						problems={problems}
						resources={resources}
					/>
				</fieldset>
				{prefill.leftOut.length > 0 ? (
					<p className="text-muted-foreground text-sm">
						Left out because it was archived after the start: {prefill.leftOut.join(", ")}.
					</p>
				) : null}
				{serverErrors.refund ? (
					<p className="text-ember-ink text-sm">{serverErrors.refund}</p>
				) : null}
				<ChangeError
					error={mutation.error}
					fallback={
						isCancel ? "The project couldn't be cancelled." : "The building couldn't be demolished."
					}
				/>
			</form>
		</BottomSheet>
	);
}
