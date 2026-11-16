import { useQueryClient } from "@tanstack/react-query";
import { PlusIcon, Trash2Icon } from "lucide-react";
import { type FormEvent, useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import { constructionQuery } from "@/api/queries";
import { BottomSheet } from "@/components/bottom-sheet";
import { Field, fieldInputProps } from "@/components/field";
import { NativeSelect, SwitchRow, TextArea } from "@/components/form-controls";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useCampaignId } from "@/hooks/use-campaign";
import {
	amountsToDraft,
	type LaborDraft,
	laborToDrafts,
	type NumberDraft,
	newLaborDraft,
	parseAmounts,
	parseLabor,
} from "@/lib/construction";
import { focusFirstVisible } from "@/lib/focus";
import { forgetFailedSaves } from "@/lib/forms";
import {
	draftToEffects,
	type EffectsDraft,
	effectsToDraft,
	emptyEffectsDraft,
} from "@/lib/ventures";
import { EffectsEditor, FormSection } from "@/ventures/venture-parts";
import { AmountsEditor } from "./amounts-editor";

type BuildingBlueprint = Schemas["BuildingBlueprint"];
type BlueprintInput = Schemas["BlueprintInput"];
type ColonyResource = Schemas["ColonyResource"];
type Labelled = { key: string; label: string };

const nameMaxLength = 80;
const proseMaxLength = 4000;
const blueprintFormId = "blueprint-form";

type Draft = {
	name: string;
	description: string;
	dmNotes: string;
	cost: NumberDraft;
	labor: LaborDraft[];
	completion: EffectsDraft;
};

function toDraft(blueprint: BuildingBlueprint | null, constructionRoleKey: string | null): Draft {
	if (!blueprint) {
		return {
			completion: emptyEffectsDraft,
			cost: {},
			description: "",
			dmNotes: "",
			labor: constructionRoleKey ? [newLaborDraft(constructionRoleKey)] : [],
			name: "",
		};
	}
	return {
		completion: effectsToDraft(blueprint.completionEffects),
		cost: amountsToDraft(blueprint.cost),
		description: blueprint.description,
		dmNotes: blueprint.dm?.dmNotes ?? "",
		labor: laborToDrafts(blueprint.labor),
		name: blueprint.name,
	};
}

/** The blueprint a draft describes, or the problems that keep it from saving. */
function buildInput(
	draft: Draft,
	sortOrder: number,
): { input: BlueprintInput | null; problems: Record<string, string> } {
	const cost = parseAmounts(draft.cost);
	const labor = parseLabor(draft.labor);
	const completion = draftToEffects(draft.completion);
	const problems: Record<string, string> = {};
	for (const [key, problem] of Object.entries(cost.problems)) {
		problems[`cost-${key}`] = problem;
	}
	for (const [key, problem] of Object.entries(labor.problems)) {
		problems[`labor-${key}`] = problem;
	}
	for (const [key, problem] of Object.entries(completion.problems)) {
		problems[`completion-${key}`] = problem;
	}
	if (draft.name.trim() === "" || Object.keys(problems).length > 0) {
		return { input: null, problems };
	}
	return {
		input: {
			completionEffects: completion.effects,
			cost: cost.amounts,
			description: draft.description,
			dmNotes: draft.dmNotes,
			labor: labor.labor,
			name: draft.name,
			sortOrder,
		},
		problems,
	};
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

/**
 * Create or fully edit a blueprint (the DM). A new blueprint starts hidden unless the switch says
 * otherwise; afterwards its card's switch is the one way to change that, and Archive the one way to
 * withdraw it. Edits never touch buildings already started (construction design K18).
 */
export function BlueprintSheet({
	open,
	onOpenChange,
	blueprint,
	resources,
	roles,
	constructionRoleKey,
	labelOf,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	blueprint: BuildingBlueprint | null;
	resources: readonly ColonyResource[];
	/** The unarchived roles a labor line may name. */
	roles: readonly Labelled[];
	constructionRoleKey: string | null;
	labelOf: (roleKey: string) => string;
}) {
	const queryClient = useQueryClient();
	const campaignId = useCampaignId();
	const [draft, setDraft] = useState(() => toDraft(blueprint, constructionRoleKey));
	const [revealed, setRevealed] = useState(false);

	function settle() {
		return queryClient.invalidateQueries({ queryKey: constructionQuery(campaignId).queryKey });
	}
	const create = api.useMutation("post", "/api/campaigns/{campaignId}/construction/blueprints", {
		onSettled: settle,
	});
	const update = api.useMutation(
		"put",
		"/api/campaigns/{campaignId}/construction/blueprints/{blueprintId}",
		{ onSettled: settle },
	);
	const error = create.error ?? update.error;
	const isSaving = create.isPending || update.isPending;
	const serverErrors = fieldErrors(error);
	const { input, problems } = buildInput(draft, blueprint?.sortOrder ?? 0);
	const canSave = input !== null && !isSaving;
	const showGeneralError = error !== null && Object.keys(serverErrors).length === 0;
	const isNameBlank = draft.name.trim() === "";

	function set<Key extends keyof Draft>(key: Key, value: Draft[Key]) {
		setDraft((current) => ({ ...current, [key]: value }));
		forgetFailedSaves(create, update);
	}

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!(canSave && input)) {
			return;
		}
		if (blueprint) {
			update.mutate(
				{ body: input, params: { path: { blueprintId: blueprint.id, campaignId } } },
				{
					onSuccess: (saved) => {
						toast.success(`${saved.name} saved.`);
						onOpenChange(false);
					},
				},
			);
			return;
		}
		create.mutate(
			{ body: { ...input, revealed }, params: { path: { campaignId } } },
			{
				onSuccess: (created) => {
					toast.success(
						created.dm?.revealed
							? `${created.name} added. Players can build it.`
							: `${created.name} added, hidden from players.`,
					);
					onOpenChange(false);
				},
			},
		);
	}

	let saveLabel = blueprint ? "Save blueprint" : "Add blueprint";
	if (isSaving) {
		saveLabel = "Saving…";
	}

	return (
		<BottomSheet
			className="md:max-w-2xl"
			description={
				blueprint
					? "Buildings already started keep the cost, labor and completion effects they started with. Show or hide it with the switch on its card."
					: "New blueprints are hidden from players unless you turn the switch on."
			}
			footer={
				<>
					<Button
						onClick={() => {
							onOpenChange(false);
						}}
						variant="ghost"
					>
						Cancel
					</Button>
					<Button disabled={!canSave} form={blueprintFormId} type="submit">
						{saveLabel}
					</Button>
				</>
			}
			onOpenChange={onOpenChange}
			open={open}
			title={blueprint ? `Edit ${blueprint.name}` : "New blueprint"}
		>
			<form className="grid gap-6" id={blueprintFormId} noValidate onSubmit={handleSubmit}>
				<FormSection title="The building">
					<Field
						description={isNameBlank ? "Give it a name to save it." : undefined}
						error={serverErrors.name}
						id="blueprint-name"
						label="Name"
					>
						<Input
							{...fieldInputProps("blueprint-name", serverErrors.name, isNameBlank)}
							autoComplete="off"
							autoFocus={!blueprint}
							maxLength={nameMaxLength}
							onChange={(event) => {
								set("name", event.target.value);
							}}
							value={draft.name}
						/>
					</Field>
					<Field
						error={serverErrors.description}
						id="blueprint-description"
						label="Description"
						optional
					>
						<TextArea
							{...fieldInputProps("blueprint-description", serverErrors.description, false)}
							maxLength={proseMaxLength}
							onChange={(event) => {
								set("description", event.target.value);
							}}
							value={draft.description}
						/>
					</Field>
					{blueprint ? null : (
						<SwitchRow
							checked={revealed}
							description={
								revealed
									? "Players see it and can start it."
									: "Hidden: players can't see it or anything built from it."
							}
							id="blueprint-revealed"
							label="Visible to players"
							onCheckedChange={(checked) => {
								setRevealed(checked);
								forgetFailedSaves(create, update);
							}}
						/>
					)}
				</FormSection>

				<FormSection
					description="Taken in full when a building starts. Leave a resource blank for none."
					title="Cost"
				>
					<AmountsEditor
						draft={draft.cost}
						idPrefix="blueprint-cost"
						onChange={(cost) => {
							set("cost", cost);
						}}
						problems={prefixed(problems, "cost-")}
						resources={resources}
					/>
					{serverErrors.cost ? <p className="text-ember-ink text-sm">{serverErrors.cost}</p> : null}
				</FormSection>

				<FormSection
					description="Each colonist on a crew adds one role-day a day. A crew cap keeps a big crew from raising it in a day."
					title="Labor"
				>
					<LaborEditor
						drafts={draft.labor}
						labelOf={labelOf}
						onChange={(labor) => {
							set("labor", labor);
						}}
						problems={prefixed(problems, "labor-")}
						roles={roles}
					/>
					{serverErrors.labor ? (
						<p className="text-ember-ink text-sm">{serverErrors.labor}</p>
					) : null}
				</FormSection>

				<FormSection
					description="Applied once, on the day a building of it finishes. Gains stop at each maximum."
					title="When it's finished"
				>
					<EffectsEditor
						draft={draft.completion}
						idPrefix="blueprint-completion"
						onChange={(completion) => {
							set("completion", completion);
						}}
						problems={prefixed(problems, "completion-")}
						resources={resources}
					/>
					{serverErrors.completionEffects ? (
						<p className="text-ember-ink text-sm">{serverErrors.completionEffects}</p>
					) : null}
				</FormSection>

				<Field
					description="Only you see these."
					error={serverErrors.dmNotes}
					id="blueprint-dmNotes"
					label="DM notes"
					optional
				>
					<TextArea
						{...fieldInputProps("blueprint-dmNotes", serverErrors.dmNotes, true)}
						maxLength={proseMaxLength}
						onChange={(event) => {
							set("dmNotes", event.target.value);
						}}
						value={draft.dmNotes}
					/>
				</Field>

				{showGeneralError ? (
					<p className="text-ember-ink text-sm" role="alert">
						{problemMessage(error, "The blueprint couldn't be saved.")}
					</p>
				) : null}
			</form>
		</BottomSheet>
	);
}

const laborAddId = "blueprint-labor-add";

/** One line per role: its work in role-days and an optional crew cap. */
function LaborEditor({
	drafts,
	roles,
	labelOf,
	problems,
	onChange,
}: {
	drafts: LaborDraft[];
	roles: readonly Labelled[];
	labelOf: (roleKey: string) => string;
	problems: Record<string, string>;
	onChange: (drafts: LaborDraft[]) => void;
}) {
	const usedKeys = new Set(drafts.map((draft) => draft.roleKey));
	const nextRole = roles.find((role) => !usedKeys.has(role.key));

	function update(position: number, change: Partial<LaborDraft>) {
		onChange(drafts.map((draft, index) => (index === position ? { ...draft, ...change } : draft)));
	}

	return (
		<div className="grid gap-2">
			{problems.labor ? <p className="text-muted-foreground text-sm">{problems.labor}</p> : null}
			<ul className="grid gap-2">
				{drafts.map((draft, position) => {
					const rowId = `blueprint-labor-${draft.rowKey}`;
					const problem = problems[draft.rowKey];
					const isListed = roles.some((role) => role.key === draft.roleKey);
					// A line whose work isn't typed yet reads as a hint, not an error.
					const isHint = draft.effortText.trim() === "";
					let problemProps = {};
					if (problem) {
						problemProps = isHint
							? { "aria-describedby": `${rowId}-error` }
							: { "aria-describedby": `${rowId}-error`, "aria-invalid": true };
					}
					return (
						<li className="grid gap-2 rounded-lg border border-border p-3" key={draft.rowKey}>
							<div className="flex gap-2">
								<NativeSelect
									{...problemProps}
									aria-label={`Line ${position + 1} role`}
									className="min-w-0 flex-1"
									id={`${rowId}-role`}
									onChange={(event) => {
										update(position, { roleKey: event.target.value });
									}}
									value={draft.roleKey}
								>
									{roles.map((role) => (
										<option
											disabled={role.key !== draft.roleKey && usedKeys.has(role.key)}
											key={role.key}
											value={role.key}
										>
											{role.label}
										</option>
									))}
									{isListed ? null : (
										<option value={draft.roleKey}>{labelOf(draft.roleKey)} (archived)</option>
									)}
								</NativeSelect>
								<Button
									aria-label={`Remove line ${position + 1}`}
									onClick={() => {
										const remaining = drafts.filter((_, index) => index !== position);
										onChange(remaining);
										const neighbour = remaining[Math.min(position, remaining.length - 1)];
										requestAnimationFrame(() => {
											focusFirstVisible(
												...(neighbour ? [`blueprint-labor-${neighbour.rowKey}-role`] : []),
												laborAddId,
											);
										});
									}}
									size="icon"
									type="button"
									variant="ghost"
								>
									<Trash2Icon />
								</Button>
							</div>
							<div className="grid grid-cols-2 gap-2">
								<Field id={`${rowId}-effort`} label="Role-days">
									<Input
										{...problemProps}
										autoComplete="off"
										id={`${rowId}-effort`}
										inputMode="numeric"
										onChange={(event) => {
											update(position, { effortText: event.target.value });
										}}
										placeholder="60"
										value={draft.effortText}
									/>
								</Field>
								<Field id={`${rowId}-maxCrew`} label="Crew cap" optional>
									<Input
										{...problemProps}
										autoComplete="off"
										id={`${rowId}-maxCrew`}
										inputMode="numeric"
										onChange={(event) => {
											update(position, { maxCrewText: event.target.value });
										}}
										placeholder="None"
										value={draft.maxCrewText}
									/>
								</Field>
							</div>
							{problem ? (
								<p
									className={isHint ? "text-muted-foreground text-sm" : "text-ember-ink text-sm"}
									id={`${rowId}-error`}
								>
									{problem}
								</p>
							) : null}
						</li>
					);
				})}
			</ul>
			{nextRole ? (
				<Button
					className="justify-self-start"
					id={laborAddId}
					onClick={() => {
						const added = newLaborDraft(nextRole.key);
						onChange([...drafts, added]);
						requestAnimationFrame(() => {
							focusFirstVisible(`blueprint-labor-${added.rowKey}-effort`);
						});
					}}
					type="button"
					variant="outline"
				>
					<PlusIcon />
					Add a role's work
				</Button>
			) : null}
		</div>
	);
}
