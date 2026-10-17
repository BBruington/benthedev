import { useQuery, useQueryClient } from "@tanstack/react-query";
import { EyeOffIcon, PlusIcon, Trash2Icon } from "lucide-react";
import { type FormEvent, useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import {
	colonyBoardQuery,
	npcsQuery,
	patronsQuery,
	questQueryKeyPrefix,
	questsQueryKeyPrefix,
	recentTimelineQuery,
	timelineQueryKeyPrefix,
} from "@/api/queries";
import { BottomSheet } from "@/components/bottom-sheet";
import { Field, fieldInputProps } from "@/components/field";
import { NativeSelect, SwitchRow, TextArea } from "@/components/form-controls";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useCampaignId } from "@/hooks/use-campaign";
import { forgetFailedSaves } from "@/lib/forms";
import { enumValues } from "@/lib/people";
import {
	draftsToLoot,
	draftToEffects,
	type EffectsDraft,
	effectsToDraft,
	emptyEffectsDraft,
	type LootDraft,
	lootToDrafts,
	newLootDraft,
	parseFavor,
	questKindLabels,
} from "@/lib/ventures";
import { EffectsEditor, FormSection } from "./venture-parts";

type Quest = Schemas["Quest"];
type QuestInput = Schemas["QuestInput"];
type QuestKind = Schemas["QuestKind"];

const titleMaxLength = 120;
const proseMaxLength = 4000;

type Draft = {
	title: string;
	kind: QuestKind;
	description: string;
	giverNpcId: string | null;
	patronId: string | null;
	reward: EffectsDraft;
	rewardFavorText: string;
	loot: LootDraft[];
	failure: EffectsDraft;
	failureFavorText: string;
	dmNotes: string;
};

function toDraft(quest: Quest | null): Draft {
	if (!quest) {
		return {
			description: "",
			dmNotes: "",
			failure: emptyEffectsDraft,
			failureFavorText: "",
			giverNpcId: null,
			kind: "COLONY",
			loot: [],
			patronId: null,
			reward: emptyEffectsDraft,
			rewardFavorText: "",
			title: "",
		};
	}
	return {
		description: quest.description,
		dmNotes: quest.dm?.dmNotes ?? "",
		failure: quest.dm ? effectsToDraft(quest.dm.failureEffects) : emptyEffectsDraft,
		failureFavorText: quest.dm?.failureFavor ? String(quest.dm.failureFavor) : "",
		giverNpcId: quest.giverNpc?.id ?? null,
		kind: quest.kind,
		loot: lootToDrafts(quest.loot),
		patronId: quest.patron?.id ?? null,
		reward: effectsToDraft(quest.rewardEffects),
		rewardFavorText: quest.rewardFavor ? String(quest.rewardFavor) : "",
		title: quest.title,
	};
}

/** The quest a draft describes, or the problems that keep it from saving. */
function buildInput(draft: Draft): { input: QuestInput | null; problems: Record<string, string> } {
	const reward = draftToEffects(draft.reward);
	const failure = draftToEffects(draft.failure);
	const rewardFavor = parseFavor(draft.rewardFavorText, false);
	const failureFavor = parseFavor(draft.failureFavorText, false);
	const loot = draftsToLoot(draft.loot);
	const problems: Record<string, string> = {};
	for (const [key, problem] of Object.entries(reward.problems)) {
		problems[`reward-${key}`] = problem;
	}
	for (const [key, problem] of Object.entries(failure.problems)) {
		problems[`failure-${key}`] = problem;
	}
	for (const [position, problem] of Object.entries(loot.problems)) {
		problems[`loot-${position}-${problem.field}`] = problem.message;
	}
	if (rewardFavor.problem) {
		problems.rewardFavor = rewardFavor.problem;
	} else if ((rewardFavor.value ?? 0) > 0 && draft.patronId === null) {
		problems.rewardFavor = "Favor needs a patron giver. Pick one above.";
	}
	if (failureFavor.problem) {
		problems.failureFavor = failureFavor.problem;
	} else if ((failureFavor.value ?? 0) > 0 && draft.patronId === null) {
		problems.failureFavor = "Losing favor needs a patron giver. Pick one above.";
	}
	if (draft.title.trim() === "" || Object.keys(problems).length > 0) {
		return { input: null, problems };
	}
	return {
		input: {
			description: draft.description,
			dmNotes: draft.dmNotes,
			failureEffects: failure.effects,
			failureFavor: failureFavor.value ?? 0,
			giverNpcId: draft.giverNpcId,
			kind: draft.kind,
			loot: loot.loot,
			patronId: draft.patronId,
			rewardEffects: reward.effects,
			rewardFavor: rewardFavor.value ?? 0,
			title: draft.title,
		},
		problems,
	};
}

/**
 * Create or fully edit a quest (spec §5.6). Only a new quest's visibility is set here: afterwards the
 * switch on its page is the one way to change it, so a stale edit sheet can't reveal or hide it.
 * Status changes only through Complete, Fail and Reopen.
 */
export function QuestSheet({
	open,
	onOpenChange,
	quest,
	onCreated,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	quest: Quest | null;
	onCreated?: (created: Quest) => void;
}) {
	const queryClient = useQueryClient();
	const campaignId = useCampaignId();
	const board = useQuery(colonyBoardQuery(campaignId));
	const npcs = useQuery(npcsQuery(campaignId));
	const patrons = useQuery(patronsQuery(campaignId));
	const [draft, setDraft] = useState(() => toDraft(quest));
	// Quests start visible (spec §5.6, Open Question 9), and posting is suppressible.
	const [revealed, setRevealed] = useState(true);
	const [postToTimeline, setPostToTimeline] = useState(true);

	function settle() {
		queryClient.invalidateQueries({ queryKey: questsQueryKeyPrefix(campaignId) });
		queryClient.invalidateQueries({ queryKey: questQueryKeyPrefix(campaignId) });
		queryClient.invalidateQueries({ queryKey: timelineQueryKeyPrefix(campaignId) });
		queryClient.invalidateQueries({ queryKey: recentTimelineQuery(campaignId).queryKey });
	}
	const create = api.useMutation("post", "/api/campaigns/{campaignId}/quests", {
		onSettled: settle,
	});
	const update = api.useMutation("put", "/api/campaigns/{campaignId}/quests/{questId}", {
		onSettled: settle,
	});
	const error = create.error ?? update.error;
	const isSaving = create.isPending || update.isPending;
	const serverErrors = fieldErrors(error);
	const { input, problems } = buildInput(draft);
	const canSave = input !== null && !isSaving;
	const showGeneralError = error !== null && Object.keys(serverErrors).length === 0;
	const resources = board.data?.resources ?? [];
	const isTitleBlank = draft.title.trim() === "";

	function set<Key extends keyof Draft>(key: Key, value: Draft[Key]) {
		setDraft((current) => ({ ...current, [key]: value }));
		forgetFailedSaves(create, update);
	}

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!(canSave && input)) {
			return;
		}
		if (quest) {
			update.mutate(
				{ body: input, params: { path: { campaignId, questId: quest.id } } },
				{
					onSuccess: (saved) => {
						toast.success(`${saved.title} saved.`);
						onOpenChange(false);
					},
				},
			);
			return;
		}
		create.mutate(
			{
				body: { ...input, postToTimeline: revealed && postToTimeline, revealed },
				params: { path: { campaignId } },
			},
			{
				onSuccess: (created) => {
					// A posted quest's live "New quest" toast (useTimelineToasts) already announces it.
					if (!created.dm?.revealed) {
						toast.success(`${created.title} added, hidden from players.`);
					} else if (!postToTimeline) {
						toast.success(`${created.title} posted.`);
					}
					onOpenChange(false);
					onCreated?.(created);
				},
			},
		);
	}

	const npcOptions = npcs.data?.npcs ?? [];
	const patronOptions = (patrons.data?.patrons ?? []).filter(
		(patron) => !patron.archived || patron.id === draft.patronId,
	);
	let saveLabel = quest ? "Save quest" : "Add quest";
	if (isSaving) {
		saveLabel = "Saving…";
	}

	return (
		<BottomSheet
			className="md:max-w-2xl"
			description={
				quest
					? "Players see the title, description, reward and a revealed giver, never the failure cost or your notes. Show or hide the quest with the switch on its page."
					: "New quests are visible to players unless you turn the switch off."
			}
			onOpenChange={onOpenChange}
			open={open}
			title={quest ? `Edit ${quest.title}` : "New quest"}
		>
			<form className="grid gap-6" noValidate onSubmit={handleSubmit}>
				<FormSection title="The quest">
					<Field
						description={isTitleBlank ? "Give it a title to save it." : undefined}
						error={serverErrors.title}
						id="quest-title"
						label="Title"
					>
						<Input
							{...fieldInputProps("quest-title", serverErrors.title, isTitleBlank)}
							autoComplete="off"
							autoFocus={!quest}
							maxLength={titleMaxLength}
							onChange={(event) => {
								set("title", event.target.value);
							}}
							value={draft.title}
						/>
					</Field>
					<Field
						description="A label only; it changes nothing."
						error={serverErrors.kind}
						id="quest-kind"
						label="Where"
					>
						<NativeSelect
							{...fieldInputProps("quest-kind", serverErrors.kind, true)}
							onChange={(event) => {
								const kind = enumValues(questKindLabels).find(
									(value) => value === event.target.value,
								);
								if (kind) {
									set("kind", kind);
								}
							}}
							value={draft.kind}
						>
							{enumValues(questKindLabels).map((value) => (
								<option key={value} value={value}>
									{questKindLabels[value]}
								</option>
							))}
						</NativeSelect>
					</Field>
					<Field
						error={serverErrors.description}
						id="quest-description"
						label="Description"
						optional
					>
						<TextArea
							{...fieldInputProps("quest-description", serverErrors.description, false)}
							maxLength={proseMaxLength}
							onChange={(event) => {
								set("description", event.target.value);
							}}
							value={draft.description}
						/>
					</Field>
					{quest ? null : (
						<>
							<SwitchRow
								checked={revealed}
								description={
									revealed
										? "Players see it on the quest board."
										: "Hidden: players can't see it anywhere."
								}
								id="quest-revealed"
								label="Visible to players"
								onCheckedChange={(checked) => {
									setRevealed(checked);
									forgetFailedSaves(create, update);
								}}
							/>
							{revealed ? (
								<div className="flex min-h-11 items-center gap-3 rounded-lg border border-border px-3">
									<input
										checked={postToTimeline}
										className="size-5 accent-[var(--color-gilt)]"
										id="quest-post"
										onChange={(event) => {
											setPostToTimeline(event.target.checked);
										}}
										type="checkbox"
									/>
									<Label className="py-2" htmlFor="quest-post">
										Post "New quest" to the timeline
									</Label>
								</div>
							) : null}
						</>
					)}
				</FormSection>

				<FormSection
					description="Players see a giver only while that NPC or patron is visible to them."
					title="Who gave it"
				>
					<div className="grid gap-4 sm:grid-cols-2">
						<Field error={serverErrors.giverNpcId} id="quest-giverNpcId" label="NPC" optional>
							<NativeSelect
								{...fieldInputProps("quest-giverNpcId", serverErrors.giverNpcId, false)}
								onChange={(event) => {
									set("giverNpcId", event.target.value === "" ? null : event.target.value);
								}}
								value={draft.giverNpcId ?? ""}
							>
								<option value="">None</option>
								{npcOptions.map((npc) => (
									<option key={npc.id} value={npc.id}>
										{npc.dm?.revealed === false ? `${npc.displayName} (hidden)` : npc.displayName}
									</option>
								))}
							</NativeSelect>
						</Field>
						<Field
							description="Needed for favor."
							error={serverErrors.patronId}
							id="quest-patronId"
							label="Patron"
							optional
						>
							<NativeSelect
								{...fieldInputProps("quest-patronId", serverErrors.patronId, true)}
								onChange={(event) => {
									set("patronId", event.target.value === "" ? null : event.target.value);
								}}
								value={draft.patronId ?? ""}
							>
								<option value="">None</option>
								{patronOptions.map((patron) => (
									<option key={patron.id} value={patron.id}>
										{patron.dm?.revealed === false ? `${patron.name} (hidden)` : patron.name}
									</option>
								))}
							</NativeSelect>
						</Field>
					</div>
				</FormSection>

				<FormSection
					description="Paid when you complete the quest. Gains stop at each resource's maximum."
					title="Reward"
				>
					<EffectsEditor
						draft={draft.reward}
						idPrefix="quest-reward"
						onChange={(reward) => {
							set("reward", reward);
						}}
						problems={prefixed(problems, "reward-")}
						resources={resources}
					/>
					{serverErrors.rewardEffects ? (
						<p className="text-ember-ink text-sm">{serverErrors.rewardEffects}</p>
					) : null}
					<Field
						error={problems.rewardFavor ?? serverErrors.rewardFavor}
						id="quest-rewardFavor"
						label="Favor with the patron"
						optional
					>
						<Input
							{...fieldInputProps(
								"quest-rewardFavor",
								problems.rewardFavor ?? serverErrors.rewardFavor,
								false,
							)}
							autoComplete="off"
							inputMode="numeric"
							onChange={(event) => {
								set("rewardFavorText", event.target.value);
							}}
							placeholder="0"
							value={draft.rewardFavorText}
						/>
					</Field>
					<LootEditor
						drafts={draft.loot}
						onChange={(loot) => {
							set("loot", loot);
						}}
						problems={problems}
						serverError={serverErrors.loot}
					/>
				</FormSection>

				<section className="grid gap-4 rounded-xl border border-verdigris/40 bg-verdigris/5 p-4">
					<FormSection
						description="Taken when you fail the quest. Losses stop at each resource's minimum; favor may go below 0."
						title={
							<span className="inline-flex flex-wrap items-center gap-2">
								Failure cost
								<Badge className="normal-case tracking-normal" variant="verdigris">
									<EyeOffIcon />
									Only you see this until it fails
								</Badge>
							</span>
						}
					>
						<EffectsEditor
							draft={draft.failure}
							idPrefix="quest-failure"
							onChange={(failure) => {
								set("failure", failure);
							}}
							problems={prefixed(problems, "failure-")}
							resources={resources}
						/>
						{serverErrors.failureEffects ? (
							<p className="text-ember-ink text-sm">{serverErrors.failureEffects}</p>
						) : null}
						<Field
							description="Lost with the patron giver."
							error={problems.failureFavor ?? serverErrors.failureFavor}
							id="quest-failureFavor"
							label="Favor lost"
							optional
						>
							<Input
								{...fieldInputProps(
									"quest-failureFavor",
									problems.failureFavor ?? serverErrors.failureFavor,
									true,
								)}
								autoComplete="off"
								inputMode="numeric"
								onChange={(event) => {
									set("failureFavorText", event.target.value);
								}}
								placeholder="0"
								value={draft.failureFavorText}
							/>
						</Field>
					</FormSection>
				</section>

				<Field
					description="Only you see these."
					error={serverErrors.dmNotes}
					id="quest-dmNotes"
					label="DM notes"
					optional
				>
					<TextArea
						{...fieldInputProps("quest-dmNotes", serverErrors.dmNotes, true)}
						maxLength={proseMaxLength}
						onChange={(event) => {
							set("dmNotes", event.target.value);
						}}
						value={draft.dmNotes}
					/>
				</Field>

				{showGeneralError ? (
					<p className="text-ember-ink text-sm" role="alert">
						{problemMessage(error, "The quest couldn't be saved.")}
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
						{saveLabel}
					</Button>
				</div>
			</form>
		</BottomSheet>
	);
}

/** The problems that start with `prefix`, keyed without it. */
function prefixed(problems: Record<string, string>, prefix: string): Record<string, string> {
	const result: Record<string, string> = {};
	for (const [key, problem] of Object.entries(problems)) {
		if (key.startsWith(prefix)) {
			result[key.slice(prefix.length)] = problem;
		}
	}
	return result;
}

/** Display-only loot (spec §5.6): a name, how many, and a line about it. */
function LootEditor({
	drafts,
	problems,
	serverError,
	onChange,
}: {
	drafts: LootDraft[];
	problems: Record<string, string>;
	serverError: string | undefined;
	onChange: (drafts: LootDraft[]) => void;
}) {
	function update(position: number, change: Partial<LootDraft>) {
		onChange(drafts.map((draft, index) => (index === position ? { ...draft, ...change } : draft)));
	}
	return (
		<div className="grid gap-2">
			<p className="font-medium text-sm">Loot</p>
			{drafts.length === 0 ? (
				<p className="text-muted-foreground text-sm">No loot. Items here are for show only.</p>
			) : null}
			<ul className="grid gap-2">
				{drafts.map((draft, position) => {
					const nameProblem = problems[`loot-${position}-name`];
					const quantityProblem = problems[`loot-${position}-quantity`];
					const rowId = `quest-loot-${position}`;
					return (
						<li className="grid gap-2 rounded-lg border border-border p-3" key={draft.rowKey}>
							<div className="flex gap-2">
								<Input
									{...fieldInputProps(`${rowId}-name`, nameProblem, false)}
									aria-label={`Item ${position + 1} name`}
									autoComplete="off"
									className="min-w-0 flex-1"
									maxLength={120}
									onChange={(event) => {
										update(position, { name: event.target.value });
									}}
									placeholder="Silver ring"
									value={draft.name}
								/>
								<Input
									{...fieldInputProps(`${rowId}-quantity`, quantityProblem, false)}
									aria-label={`Item ${position + 1} quantity`}
									autoComplete="off"
									className="w-16 text-right"
									inputMode="numeric"
									onChange={(event) => {
										update(position, { quantityText: event.target.value });
									}}
									placeholder="1"
									value={draft.quantityText}
								/>
								<Button
									aria-label={`Remove item ${position + 1}`}
									onClick={() => {
										onChange(drafts.filter((_, index) => index !== position));
									}}
									size="icon"
									type="button"
									variant="ghost"
								>
									<Trash2Icon />
								</Button>
							</div>
							<Input
								aria-label={`Item ${position + 1} description`}
								autoComplete="off"
								maxLength={400}
								onChange={(event) => {
									update(position, { description: event.target.value });
								}}
								placeholder="About it (optional)"
								value={draft.description}
							/>
							{nameProblem ? (
								<p className="text-ember-ink text-sm" id={`${rowId}-name-error`}>
									{nameProblem}
								</p>
							) : null}
							{quantityProblem ? (
								<p className="text-ember-ink text-sm" id={`${rowId}-quantity-error`}>
									{quantityProblem}
								</p>
							) : null}
						</li>
					);
				})}
			</ul>
			{serverError ? <p className="text-ember-ink text-sm">{serverError}</p> : null}
			<Button
				className="justify-self-start"
				onClick={() => {
					onChange([...drafts, newLootDraft()]);
				}}
				type="button"
				variant="outline"
			>
				<PlusIcon />
				Add loot
			</Button>
		</div>
	);
}
