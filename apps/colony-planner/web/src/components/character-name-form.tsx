import { type FormEvent, useState } from "react";
import { fieldErrors, problemMessage } from "@/api/problem";
import { Field, fieldInputProps } from "@/components/field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

/**
 * One character-name field with Cancel and Save. It only collects the name: each caller passes its own
 * mutation (the player's own profile, or the DM naming a member's character) and its own toast. A blank
 * name is sent as null, which clears it; the server trims and checks the rest.
 */
export function CharacterNameForm({
	id,
	initialName,
	description,
	placeholder,
	error,
	failureMessage,
	isPending,
	onSubmit,
	onCancel,
}: {
	id: string;
	initialName: string;
	description: string;
	placeholder?: string;
	/** The mutation's error, whose field message shows under the input. */
	error: unknown;
	/** Shown when the save fails for a reason that isn't about the name itself. */
	failureMessage: string;
	isPending: boolean;
	onSubmit: (characterName: string | null) => void;
	onCancel: () => void;
}) {
	const [characterName, setCharacterName] = useState(initialName);
	const errors = fieldErrors(error);
	const generalError =
		error && !errors.characterName ? problemMessage(error, failureMessage) : undefined;

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		const trimmed = characterName.trim();
		onSubmit(trimmed === "" ? null : trimmed);
	}

	return (
		<form className="grid gap-5" onSubmit={handleSubmit}>
			<Field description={description} error={errors.characterName} id={id} label="Character name">
				<Input
					{...fieldInputProps(id, errors.characterName, true)}
					autoComplete="off"
					autoFocus
					maxLength={60}
					onChange={(event) => {
						setCharacterName(event.target.value);
					}}
					placeholder={placeholder}
					value={characterName}
				/>
			</Field>
			{generalError ? (
				<p className="text-ember-ink text-sm" role="alert">
					{generalError}
				</p>
			) : null}
			<div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
				<Button onClick={onCancel} type="button" variant="ghost">
					Cancel
				</Button>
				<Button disabled={isPending} type="submit">
					{isPending ? "Saving…" : "Save"}
				</Button>
			</div>
		</form>
	);
}
