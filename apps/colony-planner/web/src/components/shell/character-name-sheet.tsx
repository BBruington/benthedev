import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "@/api/client";
import { meQuery } from "@/api/queries";
import { BottomSheet } from "@/components/bottom-sheet";
import { CharacterNameForm } from "@/components/character-name-form";
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
			<OwnCharacterNameForm
				initialName={me.member.characterName ?? ""}
				onDone={() => {
					onOpenChange(false);
				}}
			/>
		</BottomSheet>
	);
}

function OwnCharacterNameForm({
	initialName,
	onDone,
}: {
	initialName: string;
	onDone: () => void;
}) {
	const queryClient = useQueryClient();
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

	return (
		<CharacterNameForm
			description="Leave it empty if you haven't settled on one."
			error={mutation.error}
			failureMessage="Your character name couldn't be saved."
			id="profile-character-name"
			initialName={initialName}
			isPending={mutation.isPending}
			onCancel={onDone}
			onSubmit={(characterName) => {
				mutation.mutate({ body: { characterName } });
			}}
			placeholder="For example, Isolde Varr"
		/>
	);
}
