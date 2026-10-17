import { type FormEvent, useState } from "react";
import type { Schemas } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import { BottomSheet } from "@/components/bottom-sheet";
import { Field, fieldInputProps } from "@/components/field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { dayName, parseWholeNumber } from "@/lib/colony";
import { cn } from "@/lib/utils";

type TimelineEntry = Schemas["TimelineEntry"];
type TimelineEntryInput = Schemas["TimelineEntryInput"];

const titleLimit = 120;
const bodyLimit = 2000;

/** What the sheet edits: a new CUSTOM entry on a day, or an existing entry. */
export type EntryEditing = { kind: "new"; day: number } | { kind: "edit"; entry: TimelineEntry };

/**
 * Add a CUSTOM entry on any day, or edit an entry's text and visibility (spec §5.8). Only a CUSTOM
 * entry can move to another day; editing an auto-logged entry never reverses what it did.
 */
export function TimelineEntrySheet({
	open,
	onOpenChange,
	editing,
	error,
	isSaving,
	onSave,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	editing: EntryEditing;
	/** The save's error, whose field messages show under each input. */
	error: unknown;
	isSaving: boolean;
	onSave: (input: TimelineEntryInput) => void;
}) {
	const existing = editing.kind === "edit" ? editing.entry : null;
	const [dayText, setDayText] = useState(
		String(existing?.day ?? (editing.kind === "new" ? editing.day : 0)),
	);
	const [title, setTitle] = useState(existing?.title ?? "");
	const [body, setBody] = useState(existing?.body ?? "");
	const [isPublic, setPublic] = useState((existing?.dm?.visibility ?? "PUBLIC") === "PUBLIC");
	const canMoveDay = existing === null || existing.kind === "CUSTOM";

	const day = parseWholeNumber(dayText);
	const serverErrors = fieldErrors(error);
	let dayError = serverErrors.day;
	if (dayText.trim() !== "" && (typeof day !== "number" || day < 0)) {
		dayError = "Enter a day, 0 or more.";
	}
	const canSave = typeof day === "number" && day >= 0 && title.trim() !== "" && !isSaving;
	const showGeneralError =
		error !== null && error !== undefined && Object.keys(serverErrors).length === 0;

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!canSave || typeof day !== "number") {
			return;
		}
		onSave({ body, day, title: title.trim(), visibility: isPublic ? "PUBLIC" : "DM" });
	}

	return (
		<BottomSheet
			description={
				existing
					? "Changes the words only; what the entry did to the colony stays."
					: "A note in the colony's ledger, on any day, past or future."
			}
			onOpenChange={onOpenChange}
			open={open}
			title={existing ? "Edit entry" : "Add an entry"}
		>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<Field
					description={
						canMoveDay ? undefined : `Logged entries stay on ${dayName(existing?.day ?? 0)}.`
					}
					error={dayError}
					id="entry-day"
					label="Day"
				>
					<Input
						{...fieldInputProps("entry-day", dayError, !canMoveDay)}
						autoComplete="off"
						disabled={!canMoveDay}
						inputMode="numeric"
						onChange={(event) => {
							setDayText(event.target.value);
						}}
						value={dayText}
					/>
				</Field>
				<Field error={serverErrors.title} id="entry-title" label="Title">
					<Input
						{...fieldInputProps("entry-title", serverErrors.title, false)}
						autoComplete="off"
						autoFocus
						maxLength={titleLimit}
						onChange={(event) => {
							setTitle(event.target.value);
						}}
						placeholder="For example, The well is dug"
						value={title}
					/>
				</Field>
				<Field
					description={`${body.length.toLocaleString("en-US")} of ${bodyLimit.toLocaleString("en-US")} characters`}
					error={serverErrors.body}
					id="entry-body"
					label="Narration"
					optional
				>
					<textarea
						{...fieldInputProps("entry-body", serverErrors.body, true)}
						className={cn(
							"min-h-28 w-full min-w-0 rounded-md border border-input bg-surface-raised/70 px-3 py-2 font-flavor text-base text-foreground italic outline-none transition-[color,border-color,box-shadow] placeholder:text-muted-foreground/70 dark:bg-surface-sunken/60",
							"focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/35",
							"aria-invalid:border-ember-ink aria-invalid:ring-[3px] aria-invalid:ring-destructive/20",
						)}
						maxLength={bodyLimit}
						onChange={(event) => {
							setBody(event.target.value);
						}}
						value={body}
					/>
				</Field>
				<div className="flex items-center justify-between gap-4 rounded-lg border border-border px-3 py-3">
					<Label className="grid gap-0.5" htmlFor="entry-public">
						<span>Players can see this</span>
						<span className="font-normal text-muted-foreground text-sm">
							{isPublic ? "On the public timeline." : "Hidden from players."}
						</span>
					</Label>
					<Switch checked={isPublic} id="entry-public" onCheckedChange={setPublic} />
				</div>
				{showGeneralError ? (
					<p className="text-ember-ink text-sm" role="alert">
						{problemMessage(error, "The entry couldn't be saved.")}
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
						{isSaving ? "Saving…" : "Save entry"}
					</Button>
				</div>
			</form>
		</BottomSheet>
	);
}
