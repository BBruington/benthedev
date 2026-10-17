import { useQuery, useQueryClient } from "@tanstack/react-query";
import { CrownIcon, HandCoinsIcon, SendIcon } from "lucide-react";
import { type FormEvent, useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import {
	colonyBoardQuery,
	membersQuery,
	patronActionsQuery,
	patronQueryKeyPrefix,
	patronsQuery,
	requestQueryKeyPrefix,
	requestsQueryKeyPrefix,
} from "@/api/queries";
import type { ColonyChanges } from "@/colony/use-colony-changes";
import { BottomSheet } from "@/components/bottom-sheet";
import { Field, fieldInputProps } from "@/components/field";
import { NativeSelect, SwitchRow, TextArea } from "@/components/form-controls";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useCampaignId } from "@/hooks/use-campaign";
import { tableName } from "@/lib/access";
import { formatNumber } from "@/lib/colony";
import { forgetFailedSaves } from "@/lib/forms";
import {
	draftToEffects,
	effectsToDraft,
	emptyEffectsDraft,
	parseFavor,
	spendShortfall,
} from "@/lib/ventures";
import { ChangeError, useReviewedVersion } from "@/people/workforce-sheets";
import { costLine, EffectsEditor } from "./venture-parts";

type Patron = Schemas["Patron"];
type PatronAction = Schemas["PatronAction"];

const nameMaxLength = 80;
const proseMaxLength = 4000;
const noteMaxLength = 500;

function usePatronSettled() {
	const queryClient = useQueryClient();
	const campaignId = useCampaignId();
	return () =>
		Promise.all([
			queryClient.invalidateQueries({ queryKey: patronsQuery(campaignId).queryKey }),
			queryClient.invalidateQueries({ queryKey: patronQueryKeyPrefix(campaignId) }),
			queryClient.invalidateQueries({ queryKey: patronActionsQuery(campaignId).queryKey }),
		]);
}

function SheetActions({
	canSave,
	label,
	onCancel,
}: {
	canSave: boolean;
	label: string;
	onCancel: () => void;
}) {
	return (
		<div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
			<Button onClick={onCancel} type="button" variant="ghost">
				Cancel
			</Button>
			<Button disabled={!canSave} type="submit">
				{label}
			</Button>
		</div>
	);
}

function GeneralError({ error, fallback }: { error: unknown; fallback: string }) {
	if (error === null || Object.keys(fieldErrors(error)).length > 0) {
		return null;
	}
	return (
		<p className="text-ember-ink text-sm" role="alert">
			{problemMessage(error, fallback)}
		</p>
	);
}

/**
 * Create or edit a patron (spec §5.7). New patrons start hidden (Open Question 9); afterwards only the
 * switch on their page shows or hides them, and only the archive switch archives them.
 */
export function PatronSheet({
	open,
	onOpenChange,
	patron,
	onCreated,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	patron: Patron | null;
	onCreated?: (created: Schemas["PatronPage"]) => void;
}) {
	const campaignId = useCampaignId();
	const settle = usePatronSettled();
	const [name, setName] = useState(patron?.name ?? "");
	const [description, setDescription] = useState(patron?.description ?? "");
	const [playerNotes, setPlayerNotes] = useState(patron?.playerNotes ?? "");
	const [dmNotes, setDmNotes] = useState(patron?.dm?.dmNotes ?? "");
	const [revealed, setRevealed] = useState(false);
	const create = api.useMutation("post", "/api/campaigns/{campaignId}/patrons", {
		onSettled: settle,
	});
	const update = api.useMutation("put", "/api/campaigns/{campaignId}/patrons/{patronId}", {
		onSettled: settle,
	});
	const error = create.error ?? update.error;
	const serverErrors = fieldErrors(error);
	const isSaving = create.isPending || update.isPending;
	const canSave = name.trim() !== "" && !isSaving;

	function edited() {
		forgetFailedSaves(create, update);
	}

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!canSave) {
			return;
		}
		const body = { description, dmNotes, name, playerNotes, sortOrder: patron?.sortOrder ?? 0 };
		if (patron) {
			update.mutate(
				{ body, params: { path: { campaignId, patronId: patron.id } } },
				{
					onSuccess: (saved) => {
						toast.success(`${saved.patron.name} saved.`);
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
						created.patron.dm?.revealed
							? `${created.patron.name} added.`
							: `${created.patron.name} added, hidden from players.`,
					);
					onOpenChange(false);
					onCreated?.(created);
				},
			},
		);
	}

	return (
		<BottomSheet
			description={
				patron
					? "Players see the name, description, player notes and favor once the patron is visible. Show, hide or archive them on their page."
					: "New patrons start hidden from players. Turn on Visible to players when the party knows them."
			}
			onOpenChange={onOpenChange}
			open={open}
			title={patron ? `Edit ${patron.name}` : "New patron"}
		>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<Field error={serverErrors.name} id="patron-name" label="Name">
					<Input
						{...fieldInputProps("patron-name", serverErrors.name, false)}
						autoComplete="off"
						autoFocus={!patron}
						maxLength={nameMaxLength}
						onChange={(event) => {
							setName(event.target.value);
							edited();
						}}
						value={name}
					/>
				</Field>
				{patron ? null : (
					<SwitchRow
						checked={revealed}
						description={
							revealed ? "Players see them and their favor." : "Hidden: players can't see them."
						}
						id="patron-revealed"
						label="Visible to players"
						onCheckedChange={(checked) => {
							setRevealed(checked);
							edited();
						}}
					/>
				)}
				<Field
					error={serverErrors.description}
					id="patron-description"
					label="Description"
					optional
				>
					<TextArea
						{...fieldInputProps("patron-description", serverErrors.description, false)}
						maxLength={proseMaxLength}
						onChange={(event) => {
							setDescription(event.target.value);
							edited();
						}}
						value={description}
					/>
				</Field>
				<Field
					error={serverErrors.playerNotes}
					id="patron-playerNotes"
					label="Player notes"
					optional
				>
					<TextArea
						{...fieldInputProps("patron-playerNotes", serverErrors.playerNotes, false)}
						maxLength={proseMaxLength}
						onChange={(event) => {
							setPlayerNotes(event.target.value);
							edited();
						}}
						value={playerNotes}
					/>
				</Field>
				<Field
					description="Only you see these."
					error={serverErrors.dmNotes}
					id="patron-dmNotes"
					label="DM notes"
					optional
				>
					<TextArea
						{...fieldInputProps("patron-dmNotes", serverErrors.dmNotes, true)}
						maxLength={proseMaxLength}
						onChange={(event) => {
							setDmNotes(event.target.value);
							edited();
						}}
						value={dmNotes}
					/>
				</Field>
				<GeneralError error={error} fallback="The patron couldn't be saved." />
				<SheetActions
					canSave={canSave}
					label={isSaving ? "Saving…" : "Save patron"}
					onCancel={() => {
						onOpenChange(false);
					}}
				/>
			</form>
		</BottomSheet>
	);
}

/**
 * Define or edit one of a patron's actions (spec §5.7): its favor cost and what it brings. New actions
 * start hidden; the Visible and Archive switches on the patron's page change that.
 */
export function ActionSheet({
	open,
	onOpenChange,
	patron,
	action,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	patron: Patron;
	action: PatronAction | null;
}) {
	const campaignId = useCampaignId();
	const settle = usePatronSettled();
	const board = useQuery(colonyBoardQuery(campaignId));
	const [name, setName] = useState(action?.name ?? "");
	const [description, setDescription] = useState(action?.description ?? "");
	const [costText, setCostText] = useState(action ? String(action.favorCost) : "");
	const [effectsDraft, setEffectsDraft] = useState(
		action ? effectsToDraft(action.effects) : emptyEffectsDraft,
	);
	const [revealed, setRevealed] = useState(false);
	const create = api.useMutation("post", "/api/campaigns/{campaignId}/patron-actions", {
		onSettled: settle,
	});
	const update = api.useMutation("put", "/api/campaigns/{campaignId}/patron-actions/{actionId}", {
		onSettled: settle,
	});
	const error = create.error ?? update.error;
	const serverErrors = fieldErrors(error);
	const isSaving = create.isPending || update.isPending;
	const cost = parseFavor(costText, false);
	const { effects, problems } = draftToEffects(effectsDraft);
	const canSave =
		name.trim() !== "" &&
		cost.problem === undefined &&
		Object.keys(problems).length === 0 &&
		!isSaving;

	function edited() {
		forgetFailedSaves(create, update);
	}

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!canSave) {
			return;
		}
		const body = {
			description,
			effects,
			favorCost: cost.value ?? 0,
			name,
			sortOrder: action?.sortOrder ?? 0,
		};
		if (action) {
			update.mutate(
				{ body, params: { path: { actionId: action.id, campaignId } } },
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
			{ body: { ...body, patronId: patron.id, revealed }, params: { path: { campaignId } } },
			{
				onSuccess: (created) => {
					toast.success(
						created.dm?.revealed
							? `${created.name} added.`
							: `${created.name} added, hidden from players.`,
					);
					onOpenChange(false);
				},
			},
		);
	}

	return (
		<BottomSheet
			className="md:max-w-xl"
			description={`Something ${patron.name} can be asked for. A request spends the favor and applies the effects; it's refused if anything would fall short.`}
			onOpenChange={onOpenChange}
			open={open}
			title={action ? `Edit ${action.name}` : "New action"}
		>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<Field error={serverErrors.name} id="action-name" label="Name">
					<Input
						{...fieldInputProps("action-name", serverErrors.name, false)}
						autoComplete="off"
						autoFocus={!action}
						maxLength={nameMaxLength}
						onChange={(event) => {
							setName(event.target.value);
							edited();
						}}
						placeholder="Grain Barge"
						value={name}
					/>
				</Field>
				{action ? null : (
					<SwitchRow
						checked={revealed}
						description={
							revealed ? "Players can request it." : "Hidden: players can't see or request it."
						}
						id="action-revealed"
						label="Visible to players"
						onCheckedChange={(checked) => {
							setRevealed(checked);
							edited();
						}}
					/>
				)}
				<Field
					error={serverErrors.description}
					id="action-description"
					label="Description"
					optional
				>
					<TextArea
						{...fieldInputProps("action-description", serverErrors.description, false)}
						maxLength={proseMaxLength}
						onChange={(event) => {
							setDescription(event.target.value);
							edited();
						}}
						value={description}
					/>
				</Field>
				<Field
					error={cost.problem ?? serverErrors.favorCost}
					id="action-favorCost"
					label="Favor cost"
				>
					<Input
						{...fieldInputProps("action-favorCost", cost.problem ?? serverErrors.favorCost, false)}
						autoComplete="off"
						inputMode="numeric"
						onChange={(event) => {
							setCostText(event.target.value);
							edited();
						}}
						placeholder="0"
						value={costText}
					/>
				</Field>
				<fieldset className="grid gap-2">
					<legend className="mb-1 font-medium text-sm">Effects</legend>
					<EffectsEditor
						draft={effectsDraft}
						idPrefix="action-effects"
						onChange={(next) => {
							setEffectsDraft(next);
							edited();
						}}
						problems={problems}
						resources={board.data?.resources ?? []}
					/>
					{serverErrors.effects ? (
						<p className="text-ember-ink text-sm">{serverErrors.effects}</p>
					) : null}
				</fieldset>
				<GeneralError error={error} fallback="The action couldn't be saved." />
				<SheetActions
					canSave={canSave}
					label={isSaving ? "Saving…" : "Save action"}
					onCancel={() => {
						onOpenChange(false);
					}}
				/>
			</form>
		</BottomSheet>
	);
}

/**
 * Adjust favor (spec §5.4): by an amount or to a value, with a reason. DM adjustments may take favor
 * below 0. The entry is DM-only unless logged publicly, which a hidden patron can't be.
 */
export function FavorSheet({
	open,
	onOpenChange,
	patron,
	changes,
	boardVersion,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	patron: Patron;
	changes: ColonyChanges;
	boardVersion: number;
}) {
	const mutation = changes.adjustFavor;
	const openedVersion = useReviewedVersion(boardVersion, mutation);
	const [mode, setMode] = useState<"change" | "set">("change");
	const [amountText, setAmountText] = useState("");
	const [reason, setReason] = useState("");
	const [isPublic, setPublic] = useState(false);
	const isHidden = patron.dm?.revealed === false;
	const amount = parseFavor(amountText, true);
	const serverErrors = fieldErrors(mutation.error);
	const amountProblem =
		amount.problem ?? serverErrors.delta ?? serverErrors.setTo ?? serverErrors.favor;
	const hasAmount = amountText.trim() !== "" && amount.problem === undefined;
	const canSave = hasAmount && !mutation.isPending;
	let after: number | null = null;
	if (hasAmount) {
		after = mode === "set" ? (amount.value ?? 0) : patron.favor + (amount.value ?? 0);
	}

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!canSave) {
			return;
		}
		const value = amount.value ?? 0;
		mutation.mutate(
			{
				body: {
					delta: mode === "change" ? value : null,
					expectedVersion: openedVersion,
					public: isPublic && !isHidden,
					reason: reason.trim(),
					setTo: mode === "set" ? value : null,
				},
				params: { path: { campaignId: campaignIdOf(changes), patronId: patron.id } },
			},
			{
				onSuccess: () => {
					onOpenChange(false);
				},
			},
		);
	}

	return (
		<BottomSheet
			description={`${patron.name} has ${formatNumber(patron.favor)} favor. Your adjustments have no floor; only spending stops at 0.`}
			onOpenChange={onOpenChange}
			open={open}
			title="Adjust favor"
		>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<fieldset className="grid gap-2">
					<legend className="mb-1 font-medium text-sm">How</legend>
					<NativeSelect
						aria-label="How to change favor"
						onChange={(event) => {
							setMode(event.target.value === "set" ? "set" : "change");
							mutation.reset();
						}}
						value={mode}
					>
						<option value="change">Change by</option>
						<option value="set">Set to</option>
					</NativeSelect>
				</fieldset>
				<Field
					description={
						after === null
							? undefined
							: `Favor ${formatNumber(patron.favor)} → ${formatNumber(after)}`
					}
					error={amountProblem}
					id="favor-amount"
					label={mode === "set" ? "New favor" : "Change"}
				>
					<Input
						{...fieldInputProps("favor-amount", amountProblem, after !== null)}
						autoComplete="off"
						autoFocus
						onChange={(event) => {
							setAmountText(event.target.value);
							mutation.reset();
						}}
						placeholder={mode === "set" ? String(patron.favor) : "+2 or -1"}
						value={amountText}
					/>
				</Field>
				<Field
					description="Shown on the timeline entry."
					error={serverErrors.reason}
					id="favor-reason"
					label="Reason"
					optional
				>
					<Input
						{...fieldInputProps("favor-reason", serverErrors.reason, true)}
						autoComplete="off"
						maxLength={200}
						onChange={(event) => {
							setReason(event.target.value);
						}}
						value={reason}
					/>
				</Field>
				<SwitchRow
					checked={isPublic && !isHidden}
					description={
						isHidden
							? "Only you see this entry while the patron is hidden."
							: "Everyone sees this entry when it's on; only you otherwise."
					}
					disabled={isHidden}
					id="favor-public"
					label="Log on the public timeline"
					onCheckedChange={setPublic}
				/>
				<ChangeError error={mutation.error} fallback="Favor couldn't be adjusted." />
				<SheetActions
					canSave={canSave}
					label={mutation.isPending ? "Saving…" : "Adjust favor"}
					onCancel={() => {
						onOpenChange(false);
					}}
				/>
			</form>
		</BottomSheet>
	);
}

function campaignIdOf(changes: ColonyChanges): string {
	return changes.inCampaign.params.path.campaignId;
}

/**
 * Use action now (spec §5.7): the DM makes and grants a request in one step, crediting a player if
 * they choose one. STRICT applies as for any request, so a shortfall is refused and named.
 */
export function UseActionSheet({
	open,
	onOpenChange,
	patron,
	action,
	changes,
	boardVersion,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	patron: Patron;
	action: PatronAction;
	changes: ColonyChanges;
	boardVersion: number;
}) {
	const campaignId = campaignIdOf(changes);
	const board = useQuery(colonyBoardQuery(campaignId));
	const members = useQuery(membersQuery(campaignId));
	const mutation = changes.useActionNow;
	const openedVersion = useReviewedVersion(boardVersion, mutation);
	const [onBehalfOf, setOnBehalfOf] = useState<string | null>(null);
	const [note, setNote] = useState("");
	const resources = board.data?.resources ?? [];
	const players = (members.data?.active ?? []).filter((member) => !member.dm?.isDm);
	const shortfall = spendShortfall(patron, action.favorCost, action.effects, resources);
	const {
		onBehalfOf: onBehalfOfError,
		note: noteError,
		...otherFieldErrors
	} = fieldErrors(mutation.error);
	// actionId ("That action is archived.") has no input here, so it shows above the buttons.
	const otherFieldError = Object.values(otherFieldErrors)[0];
	const isChosenPlayerGone =
		members.data !== undefined && !players.some((member) => member.id === onBehalfOf);
	const goneChoiceId = isChosenPlayerGone ? onBehalfOf : null;

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (mutation.isPending) {
			return;
		}
		mutation.mutate(
			{
				body: {
					actionId: action.id,
					expectedVersion: openedVersion,
					note: note.trim(),
					onBehalfOf,
				},
				params: { path: { campaignId } },
			},
			{
				onSuccess: () => {
					onOpenChange(false);
				},
			},
		);
	}

	return (
		<BottomSheet
			description={`Spends ${patron.name}'s favor now, as if a player had asked and you granted it.`}
			onOpenChange={(next) => {
				if (!next) {
					mutation.reset();
				}
				onOpenChange(next);
			}}
			open={open}
			title={`Use ${action.name}`}
		>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<p className="inline-flex items-center gap-2 rounded-lg bg-surface-sunken/60 px-3 py-2 tabular-nums">
					<CrownIcon aria-hidden="true" className="size-4 text-gilt-ink" />
					{costLine(action.favorCost, action.effects, resources)}
				</p>
				{shortfall ? (
					<p className="rounded-lg border border-ember/40 bg-ember/10 px-3 py-2 text-ember-ink text-sm">
						{shortfall}
					</p>
				) : null}
				<Field
					description="The timeline credits their character; leave it on None to credit no one."
					error={onBehalfOfError}
					id="use-onBehalfOf"
					label="On behalf of"
					optional
				>
					<NativeSelect
						{...fieldInputProps("use-onBehalfOf", onBehalfOfError, true)}
						onChange={(event) => {
							setOnBehalfOf(event.target.value === "" ? null : event.target.value);
							forgetFailedSaves(mutation);
						}}
						value={onBehalfOf ?? ""}
					>
						<option value="">None</option>
						{/* Kept so the select shows the stale choice, and picking None is a change. */}
						{goneChoiceId ? <option value={goneChoiceId}>No longer an active member</option> : null}
						{players.map((member) => (
							<option key={member.id} value={member.id}>
								{tableName(member)}
							</option>
						))}
					</NativeSelect>
				</Field>
				<Field
					description="Players who can see this request read it."
					error={noteError}
					id="use-note"
					label="Note"
					optional
				>
					<TextArea
						{...fieldInputProps("use-note", noteError, true)}
						maxLength={noteMaxLength}
						onChange={(event) => {
							setNote(event.target.value);
							forgetFailedSaves(mutation);
						}}
						value={note}
					/>
				</Field>
				{otherFieldError ? (
					<p
						className="rounded-lg border border-ember/40 bg-ember/10 px-3 py-2 text-ember-ink text-sm"
						role="alert"
					>
						{otherFieldError}
					</p>
				) : null}
				<ChangeError error={mutation.error} fallback="The action couldn't be used." />
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
					<Button disabled={mutation.isPending || shortfall !== null} type="submit">
						<HandCoinsIcon />
						{mutation.isPending ? "Saving…" : "Use it now"}
					</Button>
				</div>
			</form>
		</BottomSheet>
	);
}

/**
 * A player's request (spec §5.7): an action and an optional note, sent to the DM. Offered only when
 * affordable now; the server checks again and names any shortfall.
 */
export function RequestSheet({
	open,
	onOpenChange,
	patron,
	action,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	patron: Patron;
	action: PatronAction;
}) {
	const campaignId = useCampaignId();
	const queryClient = useQueryClient();
	const board = useQuery(colonyBoardQuery(campaignId));
	const [note, setNote] = useState("");
	const mutation = api.useMutation("post", "/api/campaigns/{campaignId}/requests", {
		onSettled: () =>
			Promise.all([
				queryClient.invalidateQueries({ queryKey: requestsQueryKeyPrefix(campaignId) }),
				queryClient.invalidateQueries({ queryKey: requestQueryKeyPrefix(campaignId) }),
				queryClient.invalidateQueries({ queryKey: patronQueryKeyPrefix(campaignId) }),
			]),
	});
	const serverErrors = fieldErrors(mutation.error);
	const resources = board.data?.resources ?? [];

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (mutation.isPending) {
			return;
		}
		mutation.mutate(
			{ body: { actionId: action.id, note: note.trim() }, params: { path: { campaignId } } },
			{
				onSuccess: () => {
					toast.success(`Asked ${patron.name} for ${action.name}. The DM decides.`);
					onOpenChange(false);
				},
			},
		);
	}

	return (
		<BottomSheet
			description={`The DM grants or declines it. Nothing is spent until it's granted.`}
			onOpenChange={onOpenChange}
			open={open}
			title={`Ask for ${action.name}`}
		>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<p className="inline-flex items-center gap-2 rounded-lg bg-surface-sunken/60 px-3 py-2 tabular-nums">
					<CrownIcon aria-hidden="true" className="size-4 text-gilt-ink" />
					{costLine(action.favorCost, action.effects, resources)}
				</p>
				<Field
					description="Tell the DM why, or what you have in mind."
					error={serverErrors.note}
					id="request-note"
					label="Note"
					optional
				>
					<TextArea
						{...fieldInputProps("request-note", serverErrors.note, true)}
						autoFocus
						maxLength={noteMaxLength}
						onChange={(event) => {
							setNote(event.target.value);
							mutation.reset();
						}}
						value={note}
					/>
				</Field>
				{mutation.error && !serverErrors.note ? (
					<p
						className="rounded-lg border border-ember/40 bg-ember/10 px-3 py-2 text-ember-ink text-sm"
						role="alert"
					>
						{serverErrors.actionId ??
							problemMessage(mutation.error, "The request couldn't be sent.")}
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
					<Button disabled={mutation.isPending} type="submit">
						<SendIcon />
						{mutation.isPending ? "Sending…" : "Send request"}
					</Button>
				</div>
			</form>
		</BottomSheet>
	);
}
