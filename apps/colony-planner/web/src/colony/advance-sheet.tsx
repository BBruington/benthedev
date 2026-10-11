import { useQuery } from "@tanstack/react-query";
import { ShipIcon, SunriseIcon, UsersIcon } from "lucide-react";
import { type FormEvent, useState } from "react";
import type { Schemas } from "@/api/client";
import { problemMessage } from "@/api/problem";
import { advancePreviewQuery } from "@/api/queries";
import { BottomSheet } from "@/components/bottom-sheet";
import { Field, fieldInputProps } from "@/components/field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { formatNumber, needsRetype, parseWholeNumber, retypeAboveDays } from "@/lib/colony";
import { ResourceChangeList } from "./resource-change-list";
import { type ColonyChanges, isConflict } from "./use-colony-changes";

type AdvancePreview = Schemas["AdvancePreview"];

const previewDelayInMilliseconds = 300;

function validDays(text: string): number | null {
	const parsed = parseWholeNumber(text);
	if (typeof parsed !== "number" || parsed < 1) {
		return null;
	}
	return parsed;
}

/**
 * Advance N days (spec §5.2): the DM always sees the preview before Confirm, and above a year has to
 * type the number again. Confirm sends the version the preview saw, so a stale preview gets a 409.
 */
export function AdvanceSheet({
	open,
	onOpenChange,
	initialDays,
	changes,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	/** A quick chip's day count, or null for Custom. */
	initialDays: number | null;
	changes: ColonyChanges;
}) {
	const [daysText, setDaysText] = useState(initialDays === null ? "" : String(initialDays));
	const [retypeText, setRetypeText] = useState("");
	const days = validDays(daysText);
	const previewDays = useDebouncedValue(
		days,
		initialDays === days ? 0 : previewDelayInMilliseconds,
	);
	const preview = useQuery({
		...advancePreviewQuery(changes.inCampaign.params.path.campaignId, previewDays ?? 1),
		enabled: open && previewDays !== null,
	});
	const mutation = changes.advance;

	const asksRetype = days !== null && needsRetype(days);
	const retyped = validDays(retypeText);
	const previewMatches = preview.data !== undefined && days !== null && preview.data.days === days;
	const canConfirm =
		previewMatches &&
		!mutation.isPending &&
		(!asksRetype || retyped === days) &&
		!preview.isFetching;

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!(canConfirm && preview.data && days !== null)) {
			return;
		}
		mutation.mutate(
			{ body: { days, expectedVersion: preview.data.version }, ...changes.inCampaign },
			{
				onSuccess: () => {
					onOpenChange(false);
				},
			},
		);
	}

	let daysError: string | undefined;
	if (daysText.trim() !== "" && days === null) {
		daysError = "Enter a whole number of days, 1 or more.";
	}

	return (
		<BottomSheet
			description="Upkeep is taken for every day and ships arrive on schedule. Nothing else happens."
			onOpenChange={onOpenChange}
			open={open}
			title="Advance the clock"
		>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<Field error={daysError} id="advance-days" label="Days to advance">
					<Input
						{...fieldInputProps("advance-days", daysError, false)}
						autoComplete="off"
						autoFocus
						inputMode="numeric"
						onChange={(event) => {
							setDaysText(event.target.value);
							mutation.reset();
						}}
						placeholder="For example, 10"
						value={daysText}
					/>
				</Field>

				<AdvancePreviewPanel
					days={days}
					error={preview.error}
					isLoading={preview.isPending && previewDays !== null}
					preview={previewMatches ? preview.data : undefined}
				/>

				{asksRetype ? (
					<Field
						description={`More than ${retypeAboveDays} days at once: type ${formatNumber(days)} again to be sure.`}
						id="advance-retype"
						label="Type the number again"
					>
						<Input
							{...fieldInputProps("advance-retype", undefined, true)}
							autoComplete="off"
							inputMode="numeric"
							onChange={(event) => {
								setRetypeText(event.target.value);
							}}
							value={retypeText}
						/>
					</Field>
				) : null}

				{mutation.error ? (
					<p
						className="rounded-lg border border-ember/40 bg-ember/10 px-3 py-2 text-ember-ink text-sm"
						role="alert"
					>
						{problemMessage(mutation.error, "The clock couldn't be advanced.")}
						{isConflict(mutation.error) ? " The preview above is fresh." : null}
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
					<Button disabled={!canConfirm} type="submit">
						<SunriseIcon />
						{confirmLabel(days, mutation.isPending)}
					</Button>
				</div>
			</form>
		</BottomSheet>
	);
}

function confirmLabel(days: number | null, isPending: boolean): string {
	if (isPending) {
		return "Advancing…";
	}
	if (days === null) {
		return "Confirm";
	}
	return days === 1 ? "Advance 1 day" : `Advance ${formatNumber(days)} days`;
}

function AdvancePreviewPanel({
	days,
	preview,
	isLoading,
	error,
}: {
	days: number | null;
	preview: AdvancePreview | undefined;
	isLoading: boolean;
	error: unknown;
}) {
	if (days === null) {
		return null;
	}
	if (error) {
		return (
			<p className="rounded-lg border border-ember/40 bg-ember/10 px-3 py-2 text-ember-ink text-sm">
				{problemMessage(error, "The preview couldn't be worked out.")}
			</p>
		);
	}
	if (isLoading || !preview) {
		return (
			<div aria-busy="true" className="grid gap-2">
				<Skeleton className="h-6 w-3/4" />
				<Skeleton className="h-10" />
				<Skeleton className="h-10" />
			</div>
		);
	}
	return (
		<section aria-label="Preview" aria-live="polite" className="grid gap-3">
			<p className="font-display font-semibold text-gilt-ink text-lg tracking-wide">
				Day {formatNumber(preview.fromDay)} → {formatNumber(preview.toDay)}
			</p>
			<ResourceChangeList changes={preview.resources} />
			<p className="flex items-center gap-2 text-muted-foreground text-sm">
				<UsersIcon aria-hidden="true" className="size-4" />
				Upkeep for {formatNumber(preview.colonists)}{" "}
				{preview.colonists === 1 ? "colonist" : "colonists"}
			</p>
			<ArrivalsLine preview={preview} />
			<p className="font-flavor text-muted-foreground italic">{preview.summary}</p>
		</section>
	);
}

function ArrivalsLine({ preview }: { preview: AdvancePreview }) {
	const count = preview.arrivalCount;
	const runs = preview.arrivals.map((run) => {
		if (run.count === 1) {
			return `Day ${formatNumber(run.first)}`;
		}
		return `Days ${formatNumber(run.first)} – ${formatNumber(run.last)}, every ${formatNumber(run.every)}`;
	});
	let text = "No shipment arrivals";
	if (count === 1) {
		text = "1 shipment arrival";
	} else if (count > 1) {
		text = `${formatNumber(count)} shipment arrivals`;
	}
	return (
		<p className="flex items-start gap-2 rounded-lg border border-verdigris/35 bg-verdigris/10 px-3 py-2 text-sm">
			<ShipIcon aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-verdigris-ink" />
			<span>
				<span className="font-semibold">{text}</span>
				{runs.length > 0 ? ` (${runs.join("; ")})` : null}
				{count > 0 ? ". Cargo is recorded from each arrival on the timeline." : null}
			</span>
		</p>
	);
}
