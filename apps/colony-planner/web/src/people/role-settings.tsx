import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArchiveIcon, PenLineIcon, PlusIcon, UsersIcon } from "lucide-react";
import { type FormEvent, useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import { colonyBoardQuery, rolesQuery, workforceQuery } from "@/api/queries";
import { IconPicker } from "@/colony/resource-settings";
import { BottomSheet } from "@/components/bottom-sheet";
import { ColonyIcon, type ColonyIconName } from "@/components/colony-icon";
import { Field, fieldInputProps } from "@/components/field";
import { SwitchRow } from "@/components/form-controls";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useCampaignId } from "@/hooks/use-campaign";
import { useSheetState } from "@/hooks/use-sheet-state";
import { formatNumber, parseWholeNumber } from "@/lib/colony";
import { forgetFailedSaves } from "@/lib/forms";
import { canArchiveRole } from "@/lib/people";
import { cn } from "@/lib/utils";
import { countOf } from "@/lib/words";

type RoleSettings = Schemas["RoleSettings"];
type Editing = { kind: "new" } | { kind: "edit"; role: RoleSettings };

const rolesHeadingId = "roles-heading";

function headcountText(role: RoleSettings): string {
	const named = role.namedCount > 0 ? ` (${formatNumber(role.namedCount)} named)` : "";
	return `${countOf(role.headcount, "colonist")}${named} · position ${role.sortOrder}`;
}

/**
 * The DM's colonist roles on /settings (spec §4.2, §6.2), in the style of the resources card. Roles
 * are data: adding, renaming, choosing an icon and reordering need no code change. A role can be
 * archived only once nobody works it, and Unassigned is where new colonists land, so it stays.
 */
export function RoleSettingsCard() {
	const roles = useQuery(rolesQuery(useCampaignId()));
	const editing = useSheetState<Editing>();
	return (
		<section
			aria-labelledby={rolesHeadingId}
			className="scroll-mt-28 rounded-2xl border border-border bg-card shadow-card"
			id="roles"
		>
			<div className="flex flex-wrap items-start gap-4 border-border border-b px-5 py-5 md:px-7">
				<span
					aria-hidden="true"
					className="inline-flex size-10 shrink-0 items-center justify-center rounded-lg bg-verdigris/14 text-verdigris-ink"
				>
					<UsersIcon className="size-5" />
				</span>
				<div className="min-w-0 flex-1">
					<h2
						className="font-display font-semibold text-xl tracking-wide outline-none"
						id={rolesHeadingId}
						tabIndex={-1}
					>
						Colonist roles
					</h2>
					<p className="mt-1 max-w-prose text-muted-foreground text-sm">
						The jobs colonists do. Players see every role's headcount on the board and on People. A
						role can be archived once nobody works it; Unassigned stays, since new colonists land
						there.
					</p>
				</div>
				<Button
					onClick={() => {
						editing.show({ kind: "new" });
					}}
					size="sm"
					variant="outline"
				>
					<PlusIcon />
					Add a role
				</Button>
			</div>
			<div className="px-5 py-5 md:px-7">
				<RoleList
					onEdit={(role) => {
						editing.show({ kind: "edit", role });
					}}
					roles={roles}
				/>
			</div>
			{editing.value ? (
				<RoleSheet
					editing={editing.value}
					key={editing.session}
					onOpenChange={editing.onOpenChange}
					open={editing.open}
				/>
			) : null}
		</section>
	);
}

function RoleList({
	roles,
	onEdit,
}: {
	roles: { data?: Schemas["RoleList"]; isPending: boolean; error: unknown };
	onEdit: (role: RoleSettings) => void;
}) {
	if (roles.isPending) {
		return (
			<div aria-busy="true" className="grid gap-2">
				<Skeleton className="h-16 rounded-xl" />
				<Skeleton className="h-16 rounded-xl" />
			</div>
		);
	}
	if (!roles.data) {
		return (
			<p className="text-ember-ink">
				{problemMessage(roles.error, "The roles couldn't be loaded.")}
			</p>
		);
	}
	const sorted = [...roles.data.roles].sort((first, second) => {
		if (first.archived !== second.archived) {
			return first.archived ? 1 : -1;
		}
		return first.sortOrder - second.sortOrder;
	});
	return (
		<ul className="grid gap-2">
			{sorted.map((role) => (
				<li
					className={cn(
						"flex flex-wrap items-center gap-x-4 gap-y-2 rounded-xl border border-border px-4 py-3",
						role.archived ? "bg-transparent" : "bg-surface/60",
					)}
					key={role.key}
				>
					<span
						aria-hidden="true"
						className={cn(
							"inline-flex size-9 shrink-0 items-center justify-center rounded-lg",
							role.archived
								? "bg-muted text-muted-foreground"
								: "bg-verdigris/14 text-verdigris-ink",
						)}
					>
						<ColonyIcon className="size-[1.125rem]" name={role.icon} />
					</span>
					<div className="min-w-0 flex-1">
						<p className="flex flex-wrap items-center gap-2">
							<span className={cn("font-semibold", role.archived && "text-muted-foreground")}>
								{role.label}
							</span>
							{role.isSystem ? <Badge variant="verdigris">New colonists land here</Badge> : null}
							{role.archived ? (
								<Badge variant="muted">
									<ArchiveIcon />
									Archived
								</Badge>
							) : null}
						</p>
						<p className="text-muted-foreground text-sm tabular-nums">{headcountText(role)}</p>
					</div>
					<Button
						aria-label={`Edit ${role.label}`}
						onClick={() => {
							onEdit(role);
						}}
						size="sm"
						variant="ghost"
					>
						<PenLineIcon />
						Edit
					</Button>
				</li>
			))}
		</ul>
	);
}

function archiveDescription(role: RoleSettings): string {
	if (role.isSystem) {
		return "New colonists land here, so it stays.";
	}
	if (!(role.archived || canArchiveRole(role))) {
		return `${countOf(role.headcount, "colonist")} in this role. Move them out first.`;
	}
	return "Hidden from the board and People; its history stays on the timeline.";
}

function RoleSheet({
	open,
	onOpenChange,
	editing,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	editing: Editing;
}) {
	const queryClient = useQueryClient();
	const existing = editing.kind === "edit" ? editing.role : null;
	const [label, setLabel] = useState(existing?.label ?? "");
	const [icon, setIcon] = useState<ColonyIconName>(existing?.icon ?? "Users");
	const [sortOrderText, setSortOrderText] = useState(String(existing?.sortOrder ?? ""));
	const [archived, setArchived] = useState(existing?.archived ?? false);

	const campaignId = useCampaignId();
	function settle() {
		queryClient.invalidateQueries({ queryKey: rolesQuery(campaignId).queryKey });
		queryClient.invalidateQueries({ queryKey: workforceQuery(campaignId).queryKey });
		queryClient.invalidateQueries({ queryKey: colonyBoardQuery(campaignId).queryKey });
	}
	const create = api.useMutation("post", "/api/campaigns/{campaignId}/roles", {
		onSettled: settle,
	});
	const update = api.useMutation("put", "/api/campaigns/{campaignId}/roles/{roleKey}", {
		onSettled: settle,
	});
	const error = create.error ?? update.error;
	const isSaving = create.isPending || update.isPending;
	const serverErrors = fieldErrors(error);

	const sortOrder = parseWholeNumber(sortOrderText);
	let sortOrderError = serverErrors.sortOrder;
	if (existing && typeof sortOrder !== "number") {
		sortOrderError = "Enter a whole number.";
	}
	const canSave = label.trim() !== "" && sortOrderError === undefined && !isSaving;
	const showGeneralError = error !== null && Object.keys(serverErrors).length === 0;

	function edited() {
		forgetFailedSaves(create, update);
	}

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!canSave) {
			return;
		}
		if (existing) {
			update.mutate(
				{
					body: {
						archived,
						icon,
						label: label.trim(),
						sortOrder: typeof sortOrder === "number" ? sortOrder : existing.sortOrder,
					},
					params: { path: { campaignId, roleKey: existing.key } },
				},
				{
					onSuccess: (saved) => {
						toast.success(`${saved.label} saved.`);
						onOpenChange(false);
					},
				},
			);
			return;
		}
		create.mutate(
			{ body: { icon, label: label.trim() }, params: { path: { campaignId } } },
			{
				onSuccess: (saved) => {
					toast.success(`${saved.label} added. Transfer colonists into it from People.`);
					onOpenChange(false);
				},
			},
		);
	}

	return (
		<BottomSheet
			description={
				existing ? headcountText(existing) : "It starts with nobody in it, at the end of the list."
			}
			onOpenChange={onOpenChange}
			open={open}
			title={existing ? `Edit ${existing.label}` : "Add a role"}
		>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<Field
					description={existing?.isSystem ? "Unassigned keeps its name." : undefined}
					error={serverErrors.label}
					id="role-label"
					label="Name"
				>
					<Input
						{...fieldInputProps("role-label", serverErrors.label, existing?.isSystem === true)}
						autoComplete="off"
						autoFocus={!existing?.isSystem}
						maxLength={40}
						onChange={(event) => {
							setLabel(event.target.value);
							edited();
						}}
						placeholder="For example, Fisher"
						readOnly={existing?.isSystem === true}
						value={label}
					/>
				</Field>
				<IconPicker
					groupName="role-icon"
					onChange={(nextIcon) => {
						setIcon(nextIcon);
						edited();
					}}
					value={icon}
				/>
				{existing ? (
					<>
						<Field
							description="Lower numbers come first on the board and on People."
							error={sortOrderError}
							id="role-sort-order"
							label="Position"
						>
							<Input
								{...fieldInputProps("role-sort-order", sortOrderError, true)}
								autoComplete="off"
								inputMode="numeric"
								onChange={(event) => {
									setSortOrderText(event.target.value);
									edited();
								}}
								value={sortOrderText}
							/>
						</Field>
						<SwitchRow
							checked={archived}
							description={archiveDescription(existing)}
							disabled={!(existing.archived || canArchiveRole(existing))}
							id="role-archived"
							label="Archived"
							onCheckedChange={(checked) => {
								setArchived(checked);
								edited();
							}}
						/>
					</>
				) : null}
				{showGeneralError ? (
					<p className="text-ember-ink text-sm" role="alert">
						{problemMessage(error, "The role couldn't be saved.")}
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
						{isSaving ? "Saving…" : "Save role"}
					</Button>
				</div>
			</form>
		</BottomSheet>
	);
}
