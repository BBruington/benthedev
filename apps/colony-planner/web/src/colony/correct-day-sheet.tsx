import { HourglassIcon } from "lucide-react";
import { type FormEvent, useState } from "react";
import type { Schemas } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import { BottomSheet } from "@/components/bottom-sheet";
import { Field, fieldInputProps } from "@/components/field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { dayName, parseWholeNumber } from "@/lib/colony";
import { type ColonyChanges, isConflict } from "./use-colony-changes";

type ColonyBoard = Schemas["ColonyBoard"];

/**
 * Correct day (spec §5.3): sets the day outright, with no upkeep and no arrivals. A passed shipment
 * date rolls forward; the entry it writes is for the DM only.
 */
export function CorrectDaySheet({
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
	const [dayText, setDayText] = useState(String(board.day));
	const mutation = changes.correctDay;
	const day = parseWholeNumber(dayText);
	let problem: string | undefined;
	if (dayText.trim() !== "" && (typeof day !== "number" || day < 0)) {
		problem = "Enter a day, 0 or more.";
	}
	const serverErrors = fieldErrors(mutation.error);
	const dayError = problem ?? serverErrors.day;
	const canSave = typeof day === "number" && day >= 0 && day !== board.day && !mutation.isPending;

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!canSave || typeof day !== "number") {
			return;
		}
		mutation.mutate(
			{ body: { day, expectedVersion: board.version }, ...changes.inCampaign },
			{
				onSuccess: () => {
					onOpenChange(false);
				},
			},
		);
	}

	const showGeneralError =
		mutation.error !== null && Object.keys(fieldErrors(mutation.error)).length === 0;

	return (
		<BottomSheet
			description="Fix the day without anything happening: no upkeep, no arrivals. Only you see the timeline entry."
			onOpenChange={onOpenChange}
			open={open}
			title="Correct the day"
		>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<Field
					description={`Today is ${dayName(board.day)}. A shipment date that falls behind the new day moves to the next one after it.`}
					error={dayError}
					id="correct-day"
					label="The day it really is"
				>
					<Input
						{...fieldInputProps("correct-day", dayError, true)}
						autoComplete="off"
						autoFocus
						inputMode="numeric"
						onChange={(event) => {
							setDayText(event.target.value);
							mutation.reset();
						}}
						value={dayText}
					/>
				</Field>
				{showGeneralError ? (
					<p
						className="rounded-lg border border-ember/40 bg-ember/10 px-3 py-2 text-ember-ink text-sm"
						role="alert"
					>
						{problemMessage(mutation.error, "The day couldn't be corrected.")}
						{isConflict(mutation.error) ? " The board has been refreshed." : null}
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
						<HourglassIcon />
						{mutation.isPending ? "Saving…" : "Correct the day"}
					</Button>
				</div>
			</form>
		</BottomSheet>
	);
}
