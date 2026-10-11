import { CalendarClockIcon, CalendarX2Icon } from "lucide-react";
import { type FormEvent, useState } from "react";
import type { Schemas } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import { BottomSheet } from "@/components/bottom-sheet";
import { Field, fieldInputProps } from "@/components/field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { formatNumber, parseWholeNumber, shipmentLine } from "@/lib/colony";
import { type ColonyChanges, isConflict } from "./use-colony-changes";

type ColonyBoard = Schemas["ColonyBoard"];

/**
 * The shipment cadence (spec §5.3): an interval and the next arrival day, or no schedule at all. An
 * empty next-day field sends null, which keeps the current arrival or counts one interval from today.
 */
export function ShipmentsSheet({
	open,
	onOpenChange,
	board,
	changes,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	board: ColonyBoard;
	changes: ColonyChanges;
}) {
	const [intervalText, setIntervalText] = useState(
		board.shipments ? String(board.shipments.intervalDays) : "",
	);
	const [nextDayText, setNextDayText] = useState("");
	const save = changes.setShipments;
	const clear = changes.clearShipments;

	const interval = parseWholeNumber(intervalText);
	const nextDay = parseWholeNumber(nextDayText);
	const problems: { intervalDays?: string; nextDay?: string } = {};
	if (intervalText.trim() !== "" && (typeof interval !== "number" || interval < 1)) {
		problems.intervalDays = "Enter a whole number of days, 1 or more.";
	}
	if (nextDay === "invalid") {
		problems.nextDay = "Enter a day number, or leave it empty.";
	}
	const serverErrors = fieldErrors(save.error);
	const intervalError = problems.intervalDays ?? serverErrors.intervalDays;
	const nextDayError = problems.nextDay ?? serverErrors.nextDay;
	const canSave =
		typeof interval === "number" && interval >= 1 && nextDay !== "invalid" && !save.isPending;

	let nextDayHint = "Leave empty to keep the next arrival as it is.";
	if (!board.shipments) {
		const fallbackDay = typeof interval === "number" && interval >= 1 ? board.day + interval : null;
		nextDayHint =
			fallbackDay === null
				? "Leave empty for one interval from today."
				: `Leave empty for Day ${formatNumber(fallbackDay)}, one interval from today.`;
	}

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!canSave || typeof interval !== "number") {
			return;
		}
		save.mutate(
			{
				body: {
					expectedVersion: board.version,
					intervalDays: interval,
					nextDay: typeof nextDay === "number" ? nextDay : null,
				},
			},
			{
				onSuccess: () => {
					onOpenChange(false);
				},
			},
		);
	}

	function handleClear() {
		clear.mutate(
			{ body: { expectedVersion: board.version } },
			{
				onSuccess: () => {
					onOpenChange(false);
				},
			},
		);
	}

	const failure = save.error ?? clear.error;
	const showGeneralError = failure !== null && Object.keys(fieldErrors(failure)).length === 0;

	return (
		<BottomSheet
			description={`${shipmentLine(board.shipments)}. Ships deliver nothing on their own: record each cargo from its arrival on the timeline.`}
			onOpenChange={onOpenChange}
			open={open}
			title="Shipments from Eldari"
		>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<Field
					description="A ship arrives every this many days."
					error={intervalError}
					id="shipments-interval"
					label="Days between ships"
				>
					<Input
						{...fieldInputProps("shipments-interval", intervalError, true)}
						autoComplete="off"
						autoFocus
						inputMode="numeric"
						onChange={(event) => {
							setIntervalText(event.target.value);
							save.reset();
						}}
						placeholder="For example, 7"
						value={intervalText}
					/>
				</Field>
				<Field
					description={nextDayHint}
					error={nextDayError}
					id="shipments-next-day"
					label="Next arrival day"
					optional
				>
					<Input
						{...fieldInputProps("shipments-next-day", nextDayError, true)}
						autoComplete="off"
						inputMode="numeric"
						onChange={(event) => {
							setNextDayText(event.target.value);
							save.reset();
						}}
						placeholder={board.shipments ? `Day ${formatNumber(board.shipments.nextDay)}` : ""}
						value={nextDayText}
					/>
				</Field>

				{showGeneralError ? (
					<p
						className="rounded-lg border border-ember/40 bg-ember/10 px-3 py-2 text-ember-ink text-sm"
						role="alert"
					>
						{problemMessage(failure, "The schedule couldn't be changed.")}
						{isConflict(failure) ? " The board has been refreshed." : null}
					</p>
				) : null}

				<div className="flex flex-col-reverse gap-2 sm:flex-row sm:items-center">
					{board.shipments ? (
						<Button
							className="sm:mr-auto"
							disabled={clear.isPending}
							onClick={handleClear}
							type="button"
							variant="ghost-destructive"
						>
							<CalendarX2Icon />
							{clear.isPending ? "Clearing…" : "Clear schedule"}
						</Button>
					) : null}
					<Button
						className="sm:ml-auto"
						onClick={() => {
							onOpenChange(false);
						}}
						type="button"
						variant="ghost"
					>
						Cancel
					</Button>
					<Button disabled={!canSave} type="submit">
						<CalendarClockIcon />
						{save.isPending ? "Saving…" : "Save schedule"}
					</Button>
				</div>
			</form>
		</BottomSheet>
	);
}
