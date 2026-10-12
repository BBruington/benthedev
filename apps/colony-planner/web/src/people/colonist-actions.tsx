import { ArrowRightLeftIcon, UserPlusIcon } from "lucide-react";
import { type FormEvent, useState } from "react";
import { toast } from "sonner";
import type { Schemas } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import type { ColonyChanges } from "@/colony/use-colony-changes";
import { BottomSheet } from "@/components/bottom-sheet";
import { Field, fieldInputProps } from "@/components/field";
import { NativeSelect, SwitchRow } from "@/components/form-controls";
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { useCampaignId } from "@/hooks/use-campaign";
import { focusFirstVisible } from "@/lib/focus";
import { cn } from "@/lib/utils";
import { ChangeError, RoleSelect, SheetButtons, useReviewedVersion } from "./workforce-sheets";

type Workforce = Schemas["Workforce"];
type JoinMode = Schemas["JoinMode"];
type LeaveMode = Schemas["LeaveMode"];

/**
 * An NPC a colony action is about: who, whether they're hidden from players, and their role key
 * when they're already a colonist.
 */
export type ColonistTarget = {
	id: string;
	displayName: string;
	isHidden: boolean;
	roleKey: string | null;
};

/**
 * Moves focus once a sheet has closed: Radix stops trapping focus as soon as the sheet is told to
 * close, but only after React has rendered that, so the move waits one task.
 */
export function focusAfterClose(...elementIds: string[]) {
	setTimeout(() => {
		focusFirstVisible(...elementIds);
	}, 0);
}

function ModeChoice<Mode extends string>({
	name,
	value,
	onChange,
	choices,
}: {
	name: string;
	value: Mode;
	onChange: (mode: Mode) => void;
	choices: { mode: Mode; label: string; description: string; disabled?: boolean }[];
}) {
	return (
		<fieldset className="grid gap-2">
			<legend className="mb-2 font-medium text-sm">How</legend>
			{choices.map((choice) => (
				<label
					className={cn(
						"flex min-h-11 cursor-pointer items-start gap-3 rounded-lg border px-3 py-3 has-[:focus-visible]:ring-[3px] has-[:focus-visible]:ring-ring/45",
						choice.mode === value ? "border-gilt bg-gilt/10" : "border-border",
						choice.disabled && "cursor-not-allowed opacity-60",
					)}
					key={choice.mode}
				>
					<input
						checked={choice.mode === value}
						className="mt-1 size-4 accent-gilt"
						disabled={choice.disabled}
						name={name}
						onChange={() => {
							onChange(choice.mode);
						}}
						type="radio"
						value={choice.mode}
					/>
					<span className="grid gap-0.5">
						<span className="font-medium">{choice.label}</span>
						<span className="text-muted-foreground text-sm">{choice.description}</span>
					</span>
				</label>
			))}
		</fieldset>
	);
}

/**
 * Join colony (spec §5.5). With no `npc`, the DM first picks one of `candidates` (NPCs who aren't
 * colonists yet). A new colonist adds one to the total; promoting takes the place of an unnamed one.
 */
export function JoinColonySheet({
	open,
	onOpenChange,
	npc,
	candidates,
	workforce,
	changes,
	focusTargetIds,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	npc: ColonistTarget | null;
	candidates: readonly ColonistTarget[];
	workforce: Workforce;
	changes: ColonyChanges;
	/** Where focus lands once they've joined, since the button that opened this may be gone. */
	focusTargetIds: string[];
}) {
	const campaignId = useCampaignId();
	const roles = workforce.roles;
	const [npcId, setNpcId] = useState(npc?.id ?? "");
	const [roleKey, setRoleKey] = useState(
		roles.find((role) => role.isSystem)?.key ?? roles.at(0)?.key ?? "",
	);
	const [mode, setMode] = useState<JoinMode>("NEW_COLONIST");
	const mutation = changes.joinColony;
	const openedVersion = useReviewedVersion(workforce.version, mutation);
	const role = roles.find((candidate) => candidate.key === roleKey);
	const canPromote = (role?.unnamedCount ?? 0) > 0;
	const effectiveMode: JoinMode = canPromote ? mode : "NEW_COLONIST";
	const serverErrors = fieldErrors(mutation.error);
	const canSave = npcId !== "" && roleKey !== "" && !mutation.isPending;
	const chosen = npc ?? candidates.find((candidate) => candidate.id === npcId);
	let submitLabel = "Join";
	if (mutation.isPending) {
		submitLabel = "Joining…";
	} else if (chosen && role) {
		submitLabel = `Join as ${role.label}`;
	}

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!canSave) {
			return;
		}
		mutation.mutate(
			{
				body: { expectedVersion: openedVersion, mode: effectiveMode, roleKey },
				params: { path: { campaignId, npcId } },
			},
			{
				onSuccess: () => {
					onOpenChange(false);
					focusAfterClose(...focusTargetIds);
				},
			},
		);
	}

	return (
		<BottomSheet
			description="Named colonists count toward their role's headcount."
			onOpenChange={onOpenChange}
			open={open}
			title={npc ? `${npc.displayName} joins the colony` : "Join the colony"}
		>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				{npc ? null : (
					<Field
						description={
							candidates.length === 0
								? "No NPC is free to join: there are none yet, or every one is a colonist."
								: undefined
						}
						id="join-npc"
						label="NPC"
					>
						<NativeSelect
							{...fieldInputProps("join-npc", undefined, candidates.length === 0)}
							disabled={candidates.length === 0}
							onChange={(event) => {
								setNpcId(event.target.value);
								mutation.reset();
							}}
							value={npcId}
						>
							<option disabled value="">
								Choose an NPC
							</option>
							{candidates.map((candidate) => (
								<option key={candidate.id} value={candidate.id}>
									{candidate.displayName}
								</option>
							))}
						</NativeSelect>
					</Field>
				)}
				<Field error={serverErrors.roleKey} id="join-role" label="Role">
					<RoleSelect
						error={serverErrors.roleKey}
						id="join-role"
						onChange={(nextKey) => {
							setRoleKey(nextKey);
							mutation.reset();
						}}
						roles={roles}
						showUnnamed
						value={roleKey}
					/>
				</Field>
				<ModeChoice<JoinMode>
					choices={[
						{
							description: "The colony's total goes up by one.",
							label: "A new colonist",
							mode: "NEW_COLONIST",
						},
						{
							description: canPromote
								? `One of ${role?.label ?? "the role"}'s unnamed colonists gets a name; the total stays the same.`
								: `${role?.label ?? "This role"} has no unnamed colonists to promote.`,
							disabled: !canPromote,
							label: "Promote an unnamed colonist",
							mode: "PROMOTE_UNNAMED",
						},
					]}
					name="join-mode"
					onChange={setMode}
					value={effectiveMode}
				/>
				<ChangeError error={mutation.error} fallback="They couldn't join the colony." />
				<SheetButtons
					canSave={canSave}
					onCancel={() => {
						onOpenChange(false);
					}}
				>
					<UserPlusIcon />
					{submitLabel}
				</SheetButtons>
			</form>
		</BottomSheet>
	);
}

/** Change role (spec §5.5): the total stays the same. */
export function ChangeRoleSheet({
	open,
	onOpenChange,
	npc,
	workforce,
	changes,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	npc: ColonistTarget;
	workforce: Workforce;
	changes: ColonyChanges;
}) {
	const campaignId = useCampaignId();
	const roles = workforce.roles.filter((role) => role.key !== npc.roleKey);
	const [roleKey, setRoleKey] = useState(roles.at(0)?.key ?? "");
	const [isPublic, setPublic] = useState(false);
	const mutation = changes.changeColonyRole;
	const openedVersion = useReviewedVersion(workforce.version, mutation);
	const serverErrors = fieldErrors(mutation.error);
	const canSave = roleKey !== "" && !mutation.isPending;
	// A public entry about a hidden NPC is still written DM-only (spec §5.5).
	let publicDescription = "Only you see this entry; turn on to post it publicly.";
	if (npc.isHidden) {
		publicDescription = `${npc.displayName} is hidden from players, so only you see this entry either way.`;
	} else if (isPublic) {
		publicDescription = "Everyone sees this entry on the timeline.";
	}

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!canSave) {
			return;
		}
		mutation.mutate(
			{
				body: { expectedVersion: openedVersion, public: isPublic, roleKey },
				params: { path: { campaignId, npcId: npc.id } },
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
			description="The colony's total stays the same."
			onOpenChange={onOpenChange}
			open={open}
			title={`Change ${npc.displayName}'s role`}
		>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<Field error={serverErrors.roleKey} id="change-role" label="New role">
					<RoleSelect
						error={serverErrors.roleKey}
						id="change-role"
						onChange={(nextKey) => {
							setRoleKey(nextKey);
							mutation.reset();
						}}
						roles={roles}
						value={roleKey}
					/>
				</Field>
				<SwitchRow
					checked={isPublic}
					description={publicDescription}
					id="change-role-public"
					label="Post publicly"
					onCheckedChange={setPublic}
				/>
				<ChangeError error={mutation.error} fallback="The role couldn't be changed." />
				<SheetButtons
					canSave={canSave}
					onCancel={() => {
						onOpenChange(false);
					}}
				>
					<ArrowRightLeftIcon />
					{mutation.isPending ? "Saving…" : "Change role"}
				</SheetButtons>
			</form>
		</BottomSheet>
	);
}

/**
 * Leave colony (spec §5.5): removing them takes one off the total; returning them to the pool keeps
 * the total and adds one unnamed colonist to their role. The confirm button is gone afterwards, so
 * focus moves to `focusTargetIds`.
 */
export function LeaveColonyDialog({
	open,
	onOpenChange,
	npc,
	roleLabel,
	workforce,
	changes,
	focusTargetIds,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	npc: ColonistTarget | null;
	roleLabel: string;
	workforce: Workforce;
	changes: ColonyChanges;
	focusTargetIds: string[];
}) {
	const campaignId = useCampaignId();
	const [confirmed, setConfirmed] = useState(false);
	// Stays mounted between openings and opens without a Radix trigger, so each opening is noticed
	// here: it takes the version the DM confirms against and forgets the last confirmation.
	const [openedVersion, setOpenedVersion] = useState<number | null>(null);
	if (open && openedVersion === null) {
		setOpenedVersion(workforce.version);
		setConfirmed(false);
	} else if (!open && openedVersion !== null) {
		setOpenedVersion(null);
	}
	const mutation = changes.leaveColony;

	function leave(mode: LeaveMode) {
		if (!npc) {
			return;
		}
		setConfirmed(true);
		mutation.mutate(
			{
				body: { expectedVersion: openedVersion ?? workforce.version, mode },
				params: { path: { campaignId, npcId: npc.id } },
			},
			{
				onError: (error) => {
					toast.error(problemMessage(error, "They couldn't leave the colony."));
					focusFirstVisible(...focusTargetIds);
				},
				onSuccess: () => {
					focusFirstVisible(...focusTargetIds);
				},
			},
		);
	}

	return (
		<AlertDialog onOpenChange={onOpenChange} open={open}>
			<AlertDialogContent
				className="sm:max-w-xl"
				onCloseAutoFocus={(event) => {
					if (confirmed) {
						event.preventDefault();
					}
				}}
			>
				<AlertDialogHeader>
					<AlertDialogTitle>Should {npc?.displayName ?? "they"} leave the colony?</AlertDialogTitle>
					<AlertDialogDescription>
						Remove them and the colony has one colonist fewer. Return them to the pool and{" "}
						{roleLabel} keeps the headcount as one more unnamed colonist.
					</AlertDialogDescription>
				</AlertDialogHeader>
				<AlertDialogFooter>
					<AlertDialogCancel>Keep them</AlertDialogCancel>
					<AlertDialogAction
						onClick={() => {
							leave("RETURN_TO_POOL");
						}}
						variant="outline"
					>
						Return to the pool
					</AlertDialogAction>
					<AlertDialogAction
						onClick={() => {
							leave("REMOVE");
						}}
						variant="destructive"
					>
						Remove from the colony
					</AlertDialogAction>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	);
}
