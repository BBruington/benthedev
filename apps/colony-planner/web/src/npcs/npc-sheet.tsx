import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, type ReactNode, useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import {
	npcQueryKeyPrefix,
	npcsQueryKeyPrefix,
	placeQueryKeyPrefix,
	placesQuery,
	workforceQuery,
} from "@/api/queries";
import { BottomSheet } from "@/components/bottom-sheet";
import { Field, fieldInputProps } from "@/components/field";
import { NativeSelect, SwitchRow, TextArea } from "@/components/form-controls";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useCampaignId } from "@/hooks/use-campaign";
import { parseWholeNumber } from "@/lib/colony";
import { forgetFailedSaves } from "@/lib/forms";
import {
	enumValues,
	npcHealthLabels,
	npcRelationLabels,
	npcStatusLabels,
	parseList,
} from "@/lib/people";

type Npc = Schemas["Npc"];
type NpcInput = Schemas["NpcInput"];

const nameMaxLength = 80;
const shortMaxLength = 80;
const proseMaxLength = 4000;

type Draft = Omit<NpcInput, "aliases" | "labels" | "level"> & {
	aliasesText: string;
	labelsText: string;
	levelText: string;
};

type TextKey =
	| "name"
	| "knownName"
	| "race"
	| "characterClass"
	| "age"
	| "appearance"
	| "personality"
	| "backstory"
	| "playerNotes"
	| "dmNotes";

function toDraft(npc: Npc | null): Draft {
	if (!npc) {
		return {
			age: "",
			aliasesText: "",
			appearance: "",
			backstory: "",
			characterClass: "",
			currentPlaceId: null,
			dmNotes: "",
			health: "HEALTHY",
			knownName: "",
			labelsText: "",
			levelText: "",
			name: "",
			originPlaceId: null,
			personality: "",
			playerNotes: "",
			race: "",
			relation: "UNKNOWN",
			status: "ALIVE",
		};
	}
	return {
		age: npc.age,
		aliasesText: npc.aliases.join(", "),
		appearance: npc.appearance,
		backstory: npc.backstory,
		characterClass: npc.characterClass,
		currentPlaceId: npc.currentPlace?.id ?? null,
		dmNotes: npc.dm?.dmNotes ?? "",
		health: npc.health,
		knownName: npc.dm?.knownName ?? "",
		labelsText: npc.labels.join(", "),
		levelText: npc.level === null ? "" : String(npc.level),
		name: npc.dm?.name ?? npc.displayName,
		originPlaceId: npc.originPlace?.id ?? null,
		personality: npc.personality,
		playerNotes: npc.playerNotes,
		race: npc.race,
		relation: npc.relation,
		status: npc.status,
	};
}

function FormSection({ title, children }: { title: string; children: ReactNode }) {
	return (
		<fieldset className="grid gap-4">
			<legend className="mb-1 font-medium text-gilt-ink text-xs uppercase tracking-[0.18em]">
				{title}
			</legend>
			{children}
		</fieldset>
	);
}

/**
 * Create or fully edit an NPC (spec §4.3, §5.9): the DM's whole record, sent as one PUT. Players
 * never see the true name, the known name or the DM notes; the name players see is the known name
 * when there is one. Only a new NPC's visibility is set here: an edit sheet left open must not undo
 * a hide made since, so afterwards the switch on their page is the one way to change it.
 */
export function NpcSheet({
	open,
	onOpenChange,
	npc,
	onCreated,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	/** The NPC to edit, or null to create one. */
	npc: Npc | null;
	onCreated?: (created: Npc) => void;
}) {
	const queryClient = useQueryClient();
	const campaignId = useCampaignId();
	const places = useQuery(placesQuery(campaignId));
	const [draft, setDraft] = useState(() => toDraft(npc));
	// New NPCs start hidden (spec §5.9, Open Question 9).
	const [revealed, setRevealed] = useState(false);

	function settle() {
		queryClient.invalidateQueries({ queryKey: npcsQueryKeyPrefix(campaignId) });
		queryClient.invalidateQueries({ queryKey: npcQueryKeyPrefix(campaignId) });
		queryClient.invalidateQueries({ queryKey: placeQueryKeyPrefix(campaignId) });
		queryClient.invalidateQueries({ queryKey: workforceQuery(campaignId).queryKey });
	}
	const create = api.useMutation("post", "/api/campaigns/{campaignId}/npcs", {
		onSettled: settle,
	});
	const update = api.useMutation("put", "/api/campaigns/{campaignId}/npcs/{npcId}", {
		onSettled: settle,
	});
	const error = create.error ?? update.error;
	const isSaving = create.isPending || update.isPending;
	const serverErrors = fieldErrors(error);

	const level = parseWholeNumber(draft.levelText);
	let levelError = serverErrors.level;
	if (level === "invalid" || (typeof level === "number" && level < 0)) {
		levelError = "Enter a whole number, or leave it empty.";
	}
	const canSave = draft.name.trim() !== "" && levelError === undefined && !isSaving;
	const showGeneralError = error !== null && Object.keys(serverErrors).length === 0;

	function set<Key extends keyof Draft>(key: Key, value: Draft[Key]) {
		setDraft((current) => ({ ...current, [key]: value }));
		forgetFailedSaves(create, update);
	}

	function textProps(key: TextKey, maxLength: number) {
		return {
			...fieldInputProps(`npc-${key}`, serverErrors[key], false),
			maxLength,
			onChange: (event: { target: { value: string } }) => {
				set(key, event.target.value);
			},
			value: draft[key],
		};
	}

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!canSave || level === "invalid") {
			return;
		}
		const { aliasesText, labelsText, levelText: _levelText, ...rest } = draft;
		const body: NpcInput = {
			...rest,
			aliases: parseList(aliasesText),
			labels: parseList(labelsText),
			level,
		};
		if (npc) {
			update.mutate(
				{ body, params: { path: { campaignId, npcId: npc.id } } },
				{
					onSuccess: (saved) => {
						toast.success(`${saved.displayName} saved.`);
						onOpenChange(false);
					},
				},
			);
			return;
		}
		create.mutate(
			{ body: { ...body, revealed }, params: { path: { campaignId } } },
			{
				onSuccess: (created) => {
					toast.success(
						created.dm?.revealed
							? `${created.displayName} added.`
							: `${created.displayName} added, hidden from players.`,
					);
					onOpenChange(false);
					onCreated?.(created);
				},
			},
		);
	}

	const placeOptions = places.data?.places ?? [];
	let saveLabel = npc ? "Save NPC" : "Add NPC";
	if (isSaving) {
		saveLabel = "Saving…";
	}

	function placeSelect(key: "currentPlaceId" | "originPlaceId", label: string) {
		const id = `npc-${key}`;
		return (
			<Field error={serverErrors[key]} id={id} label={label} optional>
				<NativeSelect
					{...fieldInputProps(id, serverErrors[key], false)}
					onChange={(event) => {
						set(key, event.target.value === "" ? null : event.target.value);
					}}
					value={draft[key] ?? ""}
				>
					<option value="">None</option>
					{placeOptions.map((place) => (
						<option key={place.id} value={place.id}>
							{place.dm?.revealed === false ? `${place.name} (hidden)` : place.name}
						</option>
					))}
				</NativeSelect>
			</Field>
		);
	}

	return (
		<BottomSheet
			className="md:max-w-2xl"
			description={
				npc
					? "Everything about them. Players see the known name (or the true name when it's empty), never the true name behind it or your notes. Show or hide them with the switch on their page."
					: "New NPCs start hidden from players. Turn on Visible to players when the party meets them."
			}
			onOpenChange={onOpenChange}
			open={open}
			title={npc ? `Edit ${npc.displayName}` : "Add an NPC"}
		>
			<form className="grid gap-6" noValidate onSubmit={handleSubmit}>
				<FormSection title="Names">
					<Field
						description="Only you see it."
						error={serverErrors.name}
						id="npc-name"
						label="True name"
					>
						<Input
							{...textProps("name", nameMaxLength)}
							{...fieldInputProps("npc-name", serverErrors.name, true)}
							autoComplete="off"
							autoFocus={!npc}
						/>
					</Field>
					<Field
						description="What the party calls them. Leave empty and players see the true name."
						error={serverErrors.knownName}
						id="npc-knownName"
						label="Known name"
						optional
					>
						<Input
							{...textProps("knownName", nameMaxLength)}
							{...fieldInputProps("npc-knownName", serverErrors.knownName, true)}
							autoComplete="off"
						/>
					</Field>
					<Field
						description="Separate them with commas. Players see aliases."
						error={serverErrors.aliases}
						id="npc-aliases"
						label="Aliases"
						optional
					>
						<Input
							{...fieldInputProps("npc-aliases", serverErrors.aliases, true)}
							autoComplete="off"
							onChange={(event) => {
								set("aliasesText", event.target.value);
							}}
							value={draft.aliasesText}
						/>
					</Field>
					{npc ? null : (
						<SwitchRow
							checked={revealed}
							description={
								revealed
									? "Players see them under the known name."
									: "Hidden: players can't find them anywhere."
							}
							id="npc-revealed"
							label="Visible to players"
							onCheckedChange={(checked) => {
								setRevealed(checked);
								forgetFailedSaves(create, update);
							}}
						/>
					)}
				</FormSection>
				<FormSection title="Who they are">
					<div className="grid gap-4 sm:grid-cols-2">
						<Field error={serverErrors.race} id="npc-race" label="Race" optional>
							<Input {...textProps("race", shortMaxLength)} autoComplete="off" />
						</Field>
						<Field
							error={serverErrors.characterClass}
							id="npc-characterClass"
							label="Class"
							optional
						>
							<Input {...textProps("characterClass", shortMaxLength)} autoComplete="off" />
						</Field>
						<Field error={levelError} id="npc-level" label="Level" optional>
							<Input
								{...fieldInputProps("npc-level", levelError, false)}
								autoComplete="off"
								inputMode="numeric"
								onChange={(event) => {
									set("levelText", event.target.value);
								}}
								value={draft.levelText}
							/>
						</Field>
						<Field error={serverErrors.age} id="npc-age" label="Age" optional>
							<Input {...textProps("age", shortMaxLength)} autoComplete="off" />
						</Field>
					</div>
					<Field
						description="Separate them with commas, for example Mainland, Merchant."
						error={serverErrors.labels}
						id="npc-labels"
						label="Labels"
						optional
					>
						<Input
							{...fieldInputProps("npc-labels", serverErrors.labels, true)}
							autoComplete="off"
							onChange={(event) => {
								set("labelsText", event.target.value);
							}}
							value={draft.labelsText}
						/>
					</Field>
					<div className="grid gap-4 sm:grid-cols-3">
						<Field error={serverErrors.status} id="npc-status" label="Status">
							<NativeSelect
								{...fieldInputProps("npc-status", serverErrors.status, false)}
								onChange={(event) => {
									const status = enumValues(npcStatusLabels).find(
										(value) => value === event.target.value,
									);
									if (status) {
										set("status", status);
									}
								}}
								value={draft.status}
							>
								{enumValues(npcStatusLabels).map((value) => (
									<option key={value} value={value}>
										{npcStatusLabels[value]}
									</option>
								))}
							</NativeSelect>
						</Field>
						<Field error={serverErrors.health} id="npc-health" label="Health">
							<NativeSelect
								{...fieldInputProps("npc-health", serverErrors.health, false)}
								onChange={(event) => {
									const health = enumValues(npcHealthLabels).find(
										(value) => value === event.target.value,
									);
									if (health) {
										set("health", health);
									}
								}}
								value={draft.health}
							>
								{enumValues(npcHealthLabels).map((value) => (
									<option key={value} value={value}>
										{npcHealthLabels[value]}
									</option>
								))}
							</NativeSelect>
						</Field>
						<Field error={serverErrors.relation} id="npc-relation" label="Relation">
							<NativeSelect
								{...fieldInputProps("npc-relation", serverErrors.relation, false)}
								onChange={(event) => {
									const relation = enumValues(npcRelationLabels).find(
										(value) => value === event.target.value,
									);
									if (relation) {
										set("relation", relation);
									}
								}}
								value={draft.relation}
							>
								{enumValues(npcRelationLabels).map((value) => (
									<option key={value} value={value}>
										{npcRelationLabels[value]}
									</option>
								))}
							</NativeSelect>
						</Field>
					</div>
					<div className="grid gap-4 sm:grid-cols-2">
						{placeSelect("currentPlaceId", "Where they are")}
						{placeSelect("originPlaceId", "Where they're from")}
					</div>
				</FormSection>
				<FormSection title="Story">
					<Field error={serverErrors.appearance} id="npc-appearance" label="Appearance" optional>
						<TextArea {...textProps("appearance", proseMaxLength)} />
					</Field>
					<Field error={serverErrors.personality} id="npc-personality" label="Personality" optional>
						<TextArea {...textProps("personality", proseMaxLength)} />
					</Field>
					<Field error={serverErrors.backstory} id="npc-backstory" label="Backstory" optional>
						<TextArea {...textProps("backstory", proseMaxLength)} />
					</Field>
					<Field
						description="Players see these once the NPC is visible."
						error={serverErrors.playerNotes}
						id="npc-playerNotes"
						label="Player notes"
						optional
					>
						<TextArea
							{...textProps("playerNotes", proseMaxLength)}
							{...fieldInputProps("npc-playerNotes", serverErrors.playerNotes, true)}
						/>
					</Field>
					<Field
						description="Only you see these."
						error={serverErrors.dmNotes}
						id="npc-dmNotes"
						label="DM notes"
						optional
					>
						<TextArea
							{...textProps("dmNotes", proseMaxLength)}
							{...fieldInputProps("npc-dmNotes", serverErrors.dmNotes, true)}
						/>
					</Field>
				</FormSection>
				{showGeneralError ? (
					<p className="text-ember-ink text-sm" role="alert">
						{problemMessage(error, "The NPC couldn't be saved.")}
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
