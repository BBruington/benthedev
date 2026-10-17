import { ArrowRightLeftIcon, UsersIcon } from "lucide-react";
import { type FormEvent, type ReactNode, useLayoutEffect, useState } from "react";
import type { Schemas } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import { type ColonyChanges, isConflict } from "@/colony/use-colony-changes";
import { BottomSheet } from "@/components/bottom-sheet";
import { Field, fieldInputProps } from "@/components/field";
import { NativeSelect, SwitchRow } from "@/components/form-controls";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { parseWholeNumber } from "@/lib/colony";
import { roleCountProblem, transferCountProblem } from "@/lib/people";

type Workforce = Schemas["Workforce"];
type WorkforceRole = Schemas["WorkforceRole"];

/** A role picker; with `showUnnamed`, each option says how many unnamed colonists it has. */
export function RoleSelect({
	id,
	roles,
	value,
	onChange,
	showUnnamed = false,
	error,
}: {
	id: string;
	roles: readonly WorkforceRole[];
	value: string;
	onChange: (roleKey: string) => void;
	showUnnamed?: boolean;
	error?: string;
}) {
	return (
		<NativeSelect
			{...fieldInputProps(id, error, false)}
			onChange={(event) => {
				onChange(event.target.value);
			}}
			value={value}
		>
			{roles.map((role) => (
				<option key={role.key} value={role.key}>
					{showUnnamed ? `${role.label} (${role.unnamedCount} unnamed)` : role.label}
				</option>
			))}
		</NativeSelect>
	);
}

function PublicSwitch({
	id,
	checked,
	onCheckedChange,
}: {
	id: string;
	checked: boolean;
	onCheckedChange: (checked: boolean) => void;
}) {
	return (
		<SwitchRow
			checked={checked}
			description={
				checked
					? "Everyone sees this entry on the timeline."
					: "Only you see this entry; turn on to post it publicly."
			}
			id={id}
			label="Post publicly"
			onCheckedChange={onCheckedChange}
		/>
	);
}

/**
 * The workforce version a sheet's change is based on: the one it opened with, so a change made on
 * another device meanwhile is refused (409) instead of overwritten. While a conflict shows, the sheet
 * has the refreshed numbers, so a retry is based on those. The mutation belongs to the page and
 * outlives the sheet, so it is reset as the sheet opens: an earlier opening's conflict isn't this one's.
 */
export function useReviewedVersion(
	liveVersion: number,
	mutation: { error: unknown; reset: () => void },
): number {
	const { error, reset } = mutation;
	const [reviewedVersion, setReviewedVersion] = useState(liveVersion);
	useLayoutEffect(() => {
		reset();
	}, [reset]);
	if (isConflict(error) && reviewedVersion !== liveVersion) {
		setReviewedVersion(liveVersion);
	}
	return reviewedVersion;
}

/** The general error under a colony change form; a conflict says the numbers were refreshed. */
export function ChangeError({ error, fallback }: { error: unknown; fallback: string }) {
	if (error === null || Object.keys(fieldErrors(error)).length > 0) {
		return null;
	}
	return (
		<p
			className="rounded-lg border border-ember/40 bg-ember/10 px-3 py-2 text-ember-ink text-sm"
			role="alert"
		>
			{problemMessage(error, fallback)}
			{isConflict(error) ? " The numbers have been refreshed." : null}
		</p>
	);
}

export function SheetButtons({
	onCancel,
	canSave,
	children,
}: {
	onCancel: () => void;
	canSave: boolean;
	children: ReactNode;
}) {
	return (
		<div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
			<Button onClick={onCancel} type="button" variant="ghost">
				Cancel
			</Button>
			<Button disabled={!canSave} type="submit">
				{children}
			</Button>
		</div>
	);
}

/** Transfer (spec §5.5): moves unnamed colonists between roles; the total stays the same. */
export function TransferSheet({
	open,
	onOpenChange,
	workforce,
	changes,
	initialFromKey,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	workforce: Workforce;
	changes: ColonyChanges;
	initialFromKey: string | undefined;
}) {
	const roles = workforce.roles;
	const defaultFrom =
		initialFromKey ?? roles.find((role) => role.isSystem)?.key ?? roles.at(0)?.key ?? "";
	const [fromKey, setFromKey] = useState(defaultFrom);
	const [toKey, setToKey] = useState(
		roles.find((role) => role.key !== defaultFrom)?.key ?? defaultFrom,
	);
	const [countText, setCountText] = useState("1");
	const [isPublic, setPublic] = useState(false);
	const mutation = changes.transferColonists;
	const openedVersion = useReviewedVersion(workforce.version, mutation);
	const from = roles.find((role) => role.key === fromKey);
	const serverErrors = fieldErrors(mutation.error);
	const countError = transferCountProblem(countText, from) ?? serverErrors.count;
	const toError =
		fromKey === toKey ? "Pick a different role to move them to." : serverErrors.toRoleKey;
	const count = parseWholeNumber(countText);
	const canSave =
		typeof count === "number" &&
		countError === undefined &&
		toError === undefined &&
		!mutation.isPending;

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!canSave || typeof count !== "number") {
			return;
		}
		mutation.mutate(
			{
				body: {
					count,
					expectedVersion: openedVersion,
					fromRoleKey: fromKey,
					public: isPublic,
					toRoleKey: toKey,
				},
				...changes.inCampaign,
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
			description="Move unnamed colonists from one role to another. The colony's total stays the same."
			onOpenChange={onOpenChange}
			open={open}
			title="Transfer colonists"
		>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<Field error={serverErrors.fromRoleKey} id="transfer-from" label="From">
					<RoleSelect
						error={serverErrors.fromRoleKey}
						id="transfer-from"
						onChange={(roleKey) => {
							setFromKey(roleKey);
							mutation.reset();
						}}
						roles={roles}
						showUnnamed
						value={fromKey}
					/>
				</Field>
				<Field error={toError} id="transfer-to" label="To">
					<RoleSelect
						error={toError}
						id="transfer-to"
						onChange={(roleKey) => {
							setToKey(roleKey);
							mutation.reset();
						}}
						roles={roles}
						value={toKey}
					/>
				</Field>
				<Field error={countError} id="transfer-count" label="How many">
					<Input
						{...fieldInputProps("transfer-count", countError, false)}
						autoComplete="off"
						inputMode="numeric"
						onChange={(event) => {
							setCountText(event.target.value);
							mutation.reset();
						}}
						value={countText}
					/>
				</Field>
				<PublicSwitch checked={isPublic} id="transfer-public" onCheckedChange={setPublic} />
				<ChangeError error={mutation.error} fallback="The colonists couldn't be moved." />
				<SheetButtons
					canSave={canSave}
					onCancel={() => {
						onOpenChange(false);
					}}
				>
					<ArrowRightLeftIcon />
					{mutation.isPending ? "Moving…" : "Transfer"}
				</SheetButtons>
			</form>
		</BottomSheet>
	);
}

/**
 * Set count (spec §5.5): a correction to a role's unnamed count, and how deaths and departures are
 * recorded. Named colonists are untouched.
 */
export function SetCountSheet({
	open,
	onOpenChange,
	workforce,
	changes,
	initialRoleKey,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	workforce: Workforce;
	changes: ColonyChanges;
	initialRoleKey: string | undefined;
}) {
	const roles = workforce.roles;
	const [roleKey, setRoleKey] = useState(
		initialRoleKey ?? roles.find((role) => role.isSystem)?.key ?? roles.at(0)?.key ?? "",
	);
	const role = roles.find((candidate) => candidate.key === roleKey);
	const [countText, setCountText] = useState(String(role?.unnamedCount ?? 0));
	const [isPublic, setPublic] = useState(false);
	const mutation = changes.setRoleCount;
	const openedVersion = useReviewedVersion(workforce.version, mutation);
	const serverErrors = fieldErrors(mutation.error);
	const countError = roleCountProblem(countText) ?? serverErrors.count;
	const count = parseWholeNumber(countText);
	const canSave =
		typeof count === "number" &&
		countError === undefined &&
		count !== role?.unnamedCount &&
		!mutation.isPending;

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!canSave || typeof count !== "number") {
			return;
		}
		mutation.mutate(
			{
				body: { count, expectedVersion: openedVersion, public: isPublic, roleKey },
				...changes.inCampaign,
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
			description="Correct a role's unnamed colonists, for example after deaths or departures. Named colonists aren't changed."
			onOpenChange={onOpenChange}
			open={open}
			title="Set a role's count"
		>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<Field error={serverErrors.roleKey} id="set-count-role" label="Role">
					<RoleSelect
						error={serverErrors.roleKey}
						id="set-count-role"
						onChange={(nextKey) => {
							setRoleKey(nextKey);
							const next = roles.find((candidate) => candidate.key === nextKey);
							setCountText(String(next?.unnamedCount ?? 0));
							mutation.reset();
						}}
						roles={roles}
						value={roleKey}
					/>
				</Field>
				<Field
					description={role ? `${role.unnamedCount} now.` : undefined}
					error={countError}
					id="set-count-count"
					label="Unnamed colonists"
				>
					<Input
						{...fieldInputProps("set-count-count", countError, role !== undefined)}
						autoComplete="off"
						inputMode="numeric"
						onChange={(event) => {
							setCountText(event.target.value);
							mutation.reset();
						}}
						value={countText}
					/>
				</Field>
				<PublicSwitch checked={isPublic} id="set-count-public" onCheckedChange={setPublic} />
				<ChangeError error={mutation.error} fallback="The count couldn't be set." />
				<SheetButtons
					canSave={canSave}
					onCancel={() => {
						onOpenChange(false);
					}}
				>
					<UsersIcon />
					{mutation.isPending ? "Saving…" : "Set count"}
				</SheetButtons>
			</form>
		</BottomSheet>
	);
}
