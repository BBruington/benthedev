import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArchiveIcon, BoxesIcon, PenLineIcon, PlusIcon } from "lucide-react";
import { type FormEvent, useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import { colonyBoardQuery, resourcesQuery } from "@/api/queries";
import { BottomSheet } from "@/components/bottom-sheet";
import {
	ColonyIcon,
	type ColonyIconName,
	colonyIconLabel,
	colonyIconNames,
	colonyIcons,
} from "@/components/colony-icon";
import { Field, fieldInputProps } from "@/components/field";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { useSheetState } from "@/hooks/use-sheet-state";
import { formatNumber, parseWholeNumber } from "@/lib/colony";
import { cn } from "@/lib/utils";

type ResourceSettings = Schemas["ResourceSettings"];
type Editing = { kind: "new" } | { kind: "edit"; resource: ResourceSettings };

export const resourcesHeadingId = "resources-heading";

function boundsText(resource: ResourceSettings): string {
	const max =
		resource.maxAmount === null ? "no maximum" : `max ${formatNumber(resource.maxAmount)}`;
	return `min ${formatNumber(resource.minAmount)} · ${max}`;
}

/**
 * The DM's resource list on /settings (spec §4.2, §6.2): the colony's stock is data, so adding,
 * renaming, choosing an icon, setting bounds and archiving need no code change.
 */
export function ResourceSettingsCard() {
	const resources = useQuery(resourcesQuery);
	const editing = useSheetState<Editing>();
	return (
		<section
			aria-labelledby={resourcesHeadingId}
			className="scroll-mt-28 rounded-2xl border border-border bg-card shadow-card"
			id="resources"
		>
			<div className="flex flex-wrap items-start gap-4 border-border border-b px-5 py-5 md:px-7">
				<span
					aria-hidden="true"
					className="inline-flex size-10 shrink-0 items-center justify-center rounded-lg bg-gilt/12 text-gilt-ink"
				>
					<BoxesIcon className="size-5" />
				</span>
				<div className="min-w-0 flex-1">
					<h2
						className="font-display font-semibold text-xl tracking-wide outline-none"
						id={resourcesHeadingId}
						tabIndex={-1}
					>
						Resources
					</h2>
					<p className="mt-1 max-w-prose text-muted-foreground text-sm">
						What the colony stockpiles. Players see every amount on the board. Archiving hides a
						resource without losing its history; food can't be archived, since upkeep eats it.
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
					Add a resource
				</Button>
			</div>
			<div className="px-5 py-5 md:px-7">
				<ResourceList
					onEdit={(resource) => {
						editing.show({ kind: "edit", resource });
					}}
					resources={resources}
				/>
			</div>
			{editing.value ? (
				<ResourceSheet
					editing={editing.value}
					key={editing.session}
					onOpenChange={editing.onOpenChange}
					open={editing.open}
				/>
			) : null}
		</section>
	);
}

function ResourceList({
	resources,
	onEdit,
}: {
	resources: { data?: Schemas["ResourceList"]; isPending: boolean; error: unknown };
	onEdit: (resource: ResourceSettings) => void;
}) {
	if (resources.isPending) {
		return (
			<div aria-busy="true" className="grid gap-2">
				<Skeleton className="h-16 rounded-xl" />
				<Skeleton className="h-16 rounded-xl" />
			</div>
		);
	}
	if (!resources.data) {
		return (
			<p className="text-ember-ink">
				{problemMessage(resources.error, "The resources couldn't be loaded.")}
			</p>
		);
	}
	const sorted = [...resources.data.resources].sort((first, second) => {
		if (first.archived !== second.archived) {
			return first.archived ? 1 : -1;
		}
		return first.sortOrder - second.sortOrder;
	});
	return (
		<ul className="grid gap-2">
			{sorted.map((resource) => (
				<li
					className={cn(
						"flex flex-wrap items-center gap-x-4 gap-y-2 rounded-xl border border-border px-4 py-3",
						resource.archived ? "bg-transparent" : "bg-surface/60",
					)}
					key={resource.key}
				>
					<span
						aria-hidden="true"
						className={cn(
							"inline-flex size-9 shrink-0 items-center justify-center rounded-lg",
							resource.archived ? "bg-muted text-muted-foreground" : "bg-gilt/12 text-gilt-ink",
						)}
					>
						<ColonyIcon className="size-[1.125rem]" name={resource.icon} />
					</span>
					<div className="min-w-0 flex-1">
						<p className="flex flex-wrap items-center gap-2">
							<span className={cn("font-semibold", resource.archived && "text-muted-foreground")}>
								{resource.label}
							</span>
							{resource.isSystem ? <Badge variant="verdigris">Used by upkeep</Badge> : null}
							{resource.archived ? (
								<Badge variant="muted">
									<ArchiveIcon />
									Archived
								</Badge>
							) : null}
						</p>
						<p className="text-muted-foreground text-sm tabular-nums">
							{formatNumber(resource.amount)} on hand · {boundsText(resource)}
						</p>
					</div>
					<Button
						aria-label={`Edit ${resource.label}`}
						onClick={() => {
							onEdit(resource);
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

function ResourceSheet({
	open,
	onOpenChange,
	editing,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	editing: Editing;
}) {
	const queryClient = useQueryClient();
	const existing = editing.kind === "edit" ? editing.resource : null;
	const [label, setLabel] = useState(existing?.label ?? "");
	const [icon, setIcon] = useState<ColonyIconName>(existing?.icon ?? "Package");
	const [minText, setMinText] = useState(String(existing?.minAmount ?? 0));
	const [maxText, setMaxText] = useState(
		existing?.maxAmount === null || existing === null ? "" : String(existing.maxAmount),
	);
	const [archived, setArchived] = useState(existing?.archived ?? false);

	function settle() {
		queryClient.invalidateQueries({ queryKey: resourcesQuery.queryKey });
		queryClient.invalidateQueries({ queryKey: colonyBoardQuery.queryKey });
	}
	const create = api.useMutation("post", "/api/resources", { onSettled: settle });
	const update = api.useMutation("put", "/api/resources/{resourceKey}", { onSettled: settle });
	const error = create.error ?? update.error;
	const isSaving = create.isPending || update.isPending;

	const minAmount = parseWholeNumber(minText);
	const maxAmount = parseWholeNumber(maxText);
	const serverErrors = fieldErrors(error);
	let minError = serverErrors.minAmount;
	if (typeof minAmount !== "number") {
		minError = "Enter a whole number.";
	}
	let maxError = serverErrors.maxAmount;
	if (maxAmount === "invalid") {
		maxError = "Enter a whole number, or leave it empty for no maximum.";
	} else if (
		typeof maxAmount === "number" &&
		typeof minAmount === "number" &&
		maxAmount < minAmount
	) {
		maxError = "The maximum can't be below the minimum.";
	}
	const canSave =
		label.trim() !== "" && minError === undefined && maxError === undefined && !isSaving;
	const showGeneralError = error !== null && Object.keys(serverErrors).length === 0;

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!canSave || typeof minAmount !== "number" || maxAmount === "invalid") {
			return;
		}
		const common = { icon, label: label.trim(), maxAmount, minAmount };
		if (existing) {
			update.mutate(
				{
					body: { ...common, archived, sortOrder: existing.sortOrder },
					params: { path: { resourceKey: existing.key } },
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
			{ body: common },
			{
				onSuccess: (saved) => {
					toast.success(`${saved.label} added to the stockpile.`);
					onOpenChange(false);
				},
			},
		);
	}

	return (
		<BottomSheet
			description={
				existing
					? `${formatNumber(existing.amount)} on hand. New bounds must still hold that amount.`
					: "It starts at the amount nearest zero that its bounds allow."
			}
			onOpenChange={onOpenChange}
			open={open}
			title={existing ? `Edit ${existing.label}` : "Add a resource"}
		>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<Field error={serverErrors.label} id="resource-label" label="Name">
					<Input
						{...fieldInputProps("resource-label", serverErrors.label, false)}
						autoComplete="off"
						autoFocus
						maxLength={40}
						onChange={(event) => {
							setLabel(event.target.value);
						}}
						placeholder="For example, Timber"
						value={label}
					/>
				</Field>
				<IconPicker onChange={setIcon} value={icon} />
				<div className="grid gap-4 sm:grid-cols-2">
					<Field error={minError} id="resource-min" label="Minimum">
						<Input
							{...fieldInputProps("resource-min", minError, false)}
							autoComplete="off"
							inputMode="numeric"
							onChange={(event) => {
								setMinText(event.target.value);
							}}
							value={minText}
						/>
					</Field>
					<Field
						description="Leave empty for no maximum."
						error={maxError}
						id="resource-max"
						label="Maximum"
						optional
					>
						<Input
							{...fieldInputProps("resource-max", maxError, true)}
							autoComplete="off"
							inputMode="numeric"
							onChange={(event) => {
								setMaxText(event.target.value);
							}}
							placeholder="No maximum"
							value={maxText}
						/>
					</Field>
				</div>
				{existing ? (
					<div className="flex items-center justify-between gap-4 rounded-lg border border-border px-3 py-3">
						<Label className="grid gap-0.5" htmlFor="resource-archived">
							<span>Archived</span>
							<span className="font-normal text-muted-foreground text-sm">
								{existing.isSystem
									? "Upkeep uses this resource, so it stays."
									: "Hidden from the board; its history stays on the timeline."}
							</span>
						</Label>
						<Switch
							checked={archived}
							disabled={existing.isSystem}
							id="resource-archived"
							onCheckedChange={setArchived}
						/>
					</div>
				) : null}
				{showGeneralError ? (
					<p className="text-ember-ink text-sm" role="alert">
						{problemMessage(error, "The resource couldn't be saved.")}
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
						{isSaving ? "Saving…" : "Save resource"}
					</Button>
				</div>
			</form>
		</BottomSheet>
	);
}

/** The curated icons as a radio group: arrow keys move through them, as in any radio group. */
function IconPicker({
	value,
	onChange,
}: {
	value: ColonyIconName;
	onChange: (icon: ColonyIconName) => void;
}) {
	return (
		<fieldset className="grid gap-2">
			<legend className="mb-2 font-medium text-sm">
				Icon <span className="font-normal text-muted-foreground">· {colonyIconLabel(value)}</span>
			</legend>
			<div className="grid grid-cols-[repeat(auto-fill,minmax(2.75rem,1fr))] gap-1.5">
				{colonyIconNames.map((name) => {
					const Icon = colonyIcons[name];
					const isSelected = name === value;
					return (
						<label
							className={cn(
								"relative inline-flex size-11 cursor-pointer items-center justify-center rounded-md border transition-colors has-[:focus-visible]:ring-[3px] has-[:focus-visible]:ring-ring/45",
								isSelected
									? "border-gilt bg-gilt/15 text-gilt-ink"
									: "border-border text-muted-foreground hover:text-foreground",
							)}
							key={name}
							title={colonyIconLabel(name)}
						>
							<input
								checked={isSelected}
								className="sr-only"
								name="resource-icon"
								onChange={() => {
									onChange(name);
								}}
								type="radio"
								value={name}
							/>
							<Icon aria-hidden="true" className="size-5" />
							<span className="sr-only">{colonyIconLabel(name)}</span>
						</label>
					);
				})}
			</div>
		</fieldset>
	);
}
