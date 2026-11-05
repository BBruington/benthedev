import { useQueryClient } from "@tanstack/react-query";
import { PlusIcon, XIcon } from "lucide-react";
import { type FormEvent, useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import { pollsQuery } from "@/api/queries";
import { BottomSheet } from "@/components/bottom-sheet";
import { Field, fieldInputProps } from "@/components/field";
import { NativeSelect, SwitchRow } from "@/components/form-controls";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useCampaignId } from "@/hooks/use-campaign";
import { forgetFailedSaves } from "@/lib/forms";
import {
	defaultPollDurationHours,
	optionsProblem,
	pollDurationChoices,
	pollFewestOptions,
	pollMostOptions,
	pollOptionMaxLength,
	pollQuestionMaxLength,
} from "@/lib/polls";
import { focusAfterClose } from "@/people/colonist-actions";
import { SheetButtons } from "@/people/workforce-sheets";

type OptionDraft = { key: number; label: string };

/** The DM's New poll sheet. Mount it with the sheet's session as its key so each opening starts blank. */
export function CreatePollSheet({
	open,
	onOpenChange,
	focusTargetId,
	newPollVoting,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	/** Where focus lands after the poll is made, since the new card isn't rendered yet. */
	focusTargetId: string;
	/** Where players will vote on this poll; unknown until the poll list loads. */
	newPollVoting: Schemas["PollList"]["newPollVoting"] | undefined;
}) {
	const campaignId = useCampaignId();
	const queryClient = useQueryClient();
	const [question, setQuestion] = useState("");
	const [options, setOptions] = useState<OptionDraft[]>([
		{ key: 0, label: "" },
		{ key: 1, label: "" },
	]);
	const [nextKey, setNextKey] = useState(2);
	const [allowMultiple, setAllowMultiple] = useState(false);
	const [durationHours, setDurationHours] = useState<number>(defaultPollDurationHours);
	const create = api.useMutation("post", "/api/campaigns/{campaignId}/polls", {
		onSettled: () => queryClient.invalidateQueries({ queryKey: pollsQuery(campaignId).queryKey }),
	});

	const error = create.error;
	const serverErrors = fieldErrors(error);
	const generalError =
		error && Object.keys(serverErrors).length === 0
			? problemMessage(error, "The poll couldn't be started.")
			: null;
	const labels = options.map((option) => option.label);
	const duplicateProblem = optionsProblem(labels);
	const optionsError = duplicateProblem ?? serverErrors.options;
	const canSave =
		question.trim() !== "" &&
		labels.every((label) => label.trim() !== "") &&
		!duplicateProblem &&
		!create.isPending;

	function edited() {
		forgetFailedSaves(create);
	}

	function addOption() {
		const key = nextKey;
		setOptions((current) => [...current, { key, label: "" }]);
		setNextKey(key + 1);
		edited();
		focusAfterClose(`poll-option-${key}`);
	}

	function removeOption(index: number) {
		const neighbor = options[index + 1] ?? options[index - 1];
		setOptions((current) => current.filter((_, currentIndex) => currentIndex !== index));
		edited();
		if (neighbor) {
			focusAfterClose(`poll-option-${neighbor.key}`);
		}
	}

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!canSave) {
			return;
		}
		create.mutate(
			{
				body: {
					allowMultiple,
					durationHours,
					options: labels.map((label) => label.trim()),
					question: question.trim(),
				},
				params: { path: { campaignId } },
			},
			{
				onSuccess: () => {
					toast.success("Poll started.");
					onOpenChange(false);
					focusAfterClose(focusTargetId);
				},
			},
		);
	}

	return (
		<BottomSheet
			className="md:max-w-xl"
			description="Everyone at the table can vote until you close it or its time runs out."
			onOpenChange={onOpenChange}
			open={open}
			title="New poll"
		>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<Field error={serverErrors.question} id="poll-question" label="Question">
					<Input
						{...fieldInputProps("poll-question", serverErrors.question, false)}
						autoComplete="off"
						autoFocus
						maxLength={pollQuestionMaxLength}
						onChange={(event) => {
							setQuestion(event.target.value);
							edited();
						}}
						placeholder="Where should the new mill go?"
						value={question}
					/>
				</Field>
				<fieldset
					aria-describedby={optionsError ? "poll-options-error" : undefined}
					className="grid gap-2"
				>
					<legend className="mb-2 font-medium text-sm">Options</legend>
					{options.map((option, index) => (
						<div className="flex items-center gap-2" key={option.key}>
							<Input
								aria-invalid={optionsError ? true : undefined}
								aria-label={`Option ${index + 1}`}
								autoComplete="off"
								id={`poll-option-${option.key}`}
								maxLength={pollOptionMaxLength}
								onChange={(event) => {
									const label = event.target.value;
									setOptions((current) =>
										current.map((draft) =>
											draft.key === option.key ? { ...draft, label } : draft,
										),
									);
									edited();
								}}
								value={option.label}
							/>
							{options.length > pollFewestOptions ? (
								<Button
									aria-label={`Remove option ${index + 1}`}
									onClick={() => {
										removeOption(index);
									}}
									size="icon"
									type="button"
									variant="ghost"
								>
									<XIcon />
								</Button>
							) : null}
						</div>
					))}
					{optionsError ? (
						<p className="text-ember-ink text-sm" id="poll-options-error">
							{optionsError}
						</p>
					) : null}
					{options.length < pollMostOptions ? (
						<Button
							className="justify-self-start"
							onClick={addOption}
							type="button"
							variant="outline"
						>
							<PlusIcon />
							Add option
						</Button>
					) : (
						<p className="text-muted-foreground text-sm">
							A poll takes at most {pollMostOptions} options.
						</p>
					)}
				</fieldset>
				<SwitchRow
					checked={allowMultiple}
					description="Players tick every option they like instead of one."
					id="poll-allow-multiple"
					label="Allow several answers"
					onCheckedChange={(checked) => {
						setAllowMultiple(checked);
						edited();
					}}
				/>
				<Field error={serverErrors.durationHours} id="poll-duration" label="Closes after">
					<NativeSelect
						{...fieldInputProps("poll-duration", serverErrors.durationHours, false)}
						onChange={(event) => {
							setDurationHours(Number(event.target.value));
							edited();
						}}
						value={String(durationHours)}
					>
						{pollDurationChoices.map((choice) => (
							<option key={choice.hours} value={String(choice.hours)}>
								{choice.label}
							</option>
						))}
					</NativeSelect>
				</Field>
				{newPollVoting ? (
					<p className="text-muted-foreground text-sm">
						{newPollVoting === "DISCORD"
							? "Beny Bot posts this as a Discord poll. Players vote there."
							: "Players vote here on the site."}
					</p>
				) : null}
				<div className="grid gap-3">
					{generalError ? (
						<p className="text-ember-ink text-sm" role="alert">
							{generalError}
						</p>
					) : null}
					<SheetButtons
						canSave={canSave}
						onCancel={() => {
							onOpenChange(false);
						}}
					>
						{create.isPending ? "Starting…" : "Start poll"}
					</SheetButtons>
				</div>
			</form>
		</BottomSheet>
	);
}
