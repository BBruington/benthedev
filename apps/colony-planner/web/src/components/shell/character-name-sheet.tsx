import { useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useState } from "react";
import { toast } from "sonner";
import { api } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import { meQuery } from "@/api/queries";
import { BottomSheet } from "@/components/bottom-sheet";
import { Field, fieldInputProps } from "@/components/field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { Me } from "@/lib/access";

/** The signed-in member's own character name (PUT /api/me/profile replaces the whole profile). */
export function CharacterNameSheet({
	me,
	open,
	onOpenChange,
}: {
	me: Me;
	open: boolean;
	onOpenChange: (open: boolean) => void;
}) {
	return (
		<BottomSheet
			description="How the table knows you. The DM and the other players see it beside your Discord name."
			onOpenChange={onOpenChange}
			open={open}
			title="Your character"
		>
			<CharacterNameForm
				initialName={me.member.characterName ?? ""}
				onDone={() => {
					onOpenChange(false);
				}}
			/>
		</BottomSheet>
	);
}

function CharacterNameForm({ initialName, onDone }: { initialName: string; onDone: () => void }) {
	const queryClient = useQueryClient();
	const [characterName, setCharacterName] = useState(initialName);
	const mutation = api.useMutation("put", "/api/me/profile", {
		onSuccess: (me) => {
			queryClient.setQueryData(meQuery.queryKey, me);
			if (me.member.characterName) {
				toast.success(`You're playing ${me.member.characterName}.`);
			} else {
				toast.success("Character name cleared.");
			}
			onDone();
		},
	});
	const errors = fieldErrors(mutation.error);
	const generalError =
		mutation.error && !errors.characterName
			? problemMessage(mutation.error, "Your character name couldn't be saved.")
			: undefined;

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		const trimmed = characterName.trim();
		mutation.mutate({ body: { characterName: trimmed === "" ? null : trimmed } });
	}

	return (
		<form className="grid gap-5" onSubmit={handleSubmit}>
			<Field
				description="Leave it empty if you haven't settled on one."
				error={errors.characterName}
				id="profile-character-name"
				label="Character name"
			>
				<Input
					{...fieldInputProps("profile-character-name", errors.characterName, true)}
					autoComplete="off"
					autoFocus
					maxLength={60}
					onChange={(event) => {
						setCharacterName(event.target.value);
					}}
					placeholder="For example, Isolde Varr"
					value={characterName}
				/>
			</Field>
			{generalError ? (
				<p className="text-ember-ink text-sm" role="alert">
					{generalError}
				</p>
			) : null}
			<div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
				<Button onClick={onDone} type="button" variant="ghost">
					Cancel
				</Button>
				<Button disabled={mutation.isPending} type="submit">
					{mutation.isPending ? "Saving…" : "Save"}
				</Button>
			</div>
		</form>
	);
}
