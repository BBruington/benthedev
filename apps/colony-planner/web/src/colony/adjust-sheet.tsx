import { useQuery } from "@tanstack/react-query";
import { PackageOpenIcon, ScaleIcon, UsersIcon } from "lucide-react";
import { type FormEvent, useState } from "react";
import type { Schemas } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import { adjustPreviewQuery } from "@/api/queries";
import { BottomSheet } from "@/components/bottom-sheet";
import { ColonyIcon } from "@/components/colony-icon";
import { Field, fieldInputProps } from "@/components/field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { dayName, formatNumber, parseWholeNumber } from "@/lib/colony";
import { cn } from "@/lib/utils";
import { ResourceChangeList } from "./resource-change-list";
import { type ColonyChanges, isConflict } from "./use-colony-changes";

type ColonyResource = Schemas["ColonyResource"];
type AdjustInput = Schemas["AdjustInput"];

/** The arrival a Record cargo adjustment belongs to. */
export type CargoArrival = { arrivalEntryId: string; day: number; title: string };

type Mode = "change" | "set";
type ResourceDraft = { mode: Mode; text: string };

const previewDelayInMilliseconds = 350;
const reasonLimit = 200;

/** The input the drafts describe, or the first problem with them; null when nothing would change. */
function buildInput(
	resources: readonly ColonyResource[],
	drafts: Record<string, ResourceDraft>,
	colonistsText: string,
	common: Pick<AdjustInput, "reason" | "public" | "source" | "arrivalEntryId" | "expectedVersion">,
): { input: AdjustInput | null; problems: Record<string, string> } {
	const deltas: Record<string, number> = {};
	const setTo: Record<string, number> = {};
	const problems: Record<string, string> = {};
	for (const resource of resources) {
		const draft = drafts[resource.key];
		if (!draft) {
			continue;
		}
		const parsed = parseWholeNumber(draft.text);
		if (parsed === "invalid") {
			problems[resource.key] = "Enter a whole number.";
		} else if (parsed !== null && draft.mode === "set") {
			setTo[resource.key] = parsed;
		} else if (parsed !== null && parsed !== 0) {
			deltas[resource.key] = parsed;
		}
	}
	const colonists = parseWholeNumber(colonistsText);
	if (colonists === "invalid" || (typeof colonists === "number" && colonists < 0)) {
		problems.colonists = "Enter 0 or more new colonists.";
	}
	const colonistCount = typeof colonists === "number" ? colonists : 0;
	const hasChange =
		Object.keys(deltas).length > 0 || Object.keys(setTo).length > 0 || colonistCount > 0;
	if (Object.keys(problems).length > 0 || !hasChange) {
		return { input: null, problems };
	}
	return { input: { ...common, colonists: colonistCount, deltas, setTo }, problems };
}

/**
 * Adjust stock, or Record cargo from an arrival (spec §5.3–5.4): deltas or set-to values, a reason and
 * the public switch. The preview shows what the bounds will do ("capped") before the DM confirms.
 */
export function AdjustSheet({
	open,
	onOpenChange,
	resources,
	version,
	cargo,
	changes,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	resources: readonly ColonyResource[];
	version: number;
	/** Set for Record cargo: the adjustment is the cargo of this arrival, logged publicly by default. */
	cargo: CargoArrival | null;
	changes: ColonyChanges;
}) {
	const [drafts, setDrafts] = useState<Record<string, ResourceDraft>>({});
	const [colonistsText, setColonistsText] = useState("");
	const [reason, setReason] = useState("");
	const [isPublic, setPublic] = useState(cargo !== null);
	const mutation = changes.adjust;

	const { input, problems } = buildInput(resources, drafts, colonistsText, {
		arrivalEntryId: cargo?.arrivalEntryId ?? null,
		expectedVersion: version,
		public: isPublic,
		reason: reason.trim(),
		source: cargo ? "SHIPMENT" : "ADJUSTMENT",
	});
	const previewInput = useDebouncedValue(input, previewDelayInMilliseconds);
	const inputKey = JSON.stringify(input);
	const previewIsCurrent = JSON.stringify(previewInput) === inputKey;
	const preview = useQuery({
		...adjustPreviewQuery(changes.inCampaign.params.path.campaignId, previewInput ?? emptyInput),
		enabled: open && previewInput !== null,
	});
	const serverErrors = fieldErrors(mutation.error ?? preview.error);
	const canConfirm =
		input !== null &&
		previewIsCurrent &&
		preview.data !== undefined &&
		!preview.isFetching &&
		!mutation.isPending;

	function updateDraft(key: string, change: Partial<ResourceDraft>) {
		setDrafts((current) => ({
			...current,
			[key]: { mode: current[key]?.mode ?? "change", text: current[key]?.text ?? "", ...change },
		}));
		mutation.reset();
	}

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!(canConfirm && input && preview.data)) {
			return;
		}
		mutation.mutate(
			{ body: { ...input, expectedVersion: preview.data.version }, ...changes.inCampaign },
			{
				onSuccess: () => {
					onOpenChange(false);
				},
			},
		);
	}

	const title = cargo ? "Record cargo" : "Adjust stock";
	const description = cargo
		? `What came ashore with ${cargo.title} (${dayName(cargo.day)}). New colonists land in Unassigned.`
		: "Change the stockpile by an amount, or set it outright. Bounds still apply.";

	return (
		<BottomSheet description={description} onOpenChange={onOpenChange} open={open} title={title}>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<fieldset className="grid gap-3">
					<legend className="mb-2 font-medium text-sm">Resources</legend>
					{resources.map((resource, position) => (
						<ResourceRow
							autoFocus={position === 0}
							draft={drafts[resource.key] ?? { mode: "change", text: "" }}
							error={problems[resource.key]}
							key={resource.key}
							onChange={(change) => {
								updateDraft(resource.key, change);
							}}
							resource={resource}
						/>
					))}
				</fieldset>

				<Field
					description="They join as Unassigned colonists."
					error={problems.colonists ?? serverErrors.colonists}
					id="adjust-colonists"
					label={
						<>
							<UsersIcon aria-hidden="true" className="size-4 text-muted-foreground" />
							New colonists
						</>
					}
					optional
				>
					<Input
						{...fieldInputProps(
							"adjust-colonists",
							problems.colonists ?? serverErrors.colonists,
							true,
						)}
						autoComplete="off"
						inputMode="numeric"
						onChange={(event) => {
							setColonistsText(event.target.value);
							mutation.reset();
						}}
						placeholder="0"
						value={colonistsText}
					/>
				</Field>

				<Field
					description="Shown on the timeline entry."
					error={serverErrors.reason}
					id="adjust-reason"
					label="Reason"
					optional
				>
					<Input
						{...fieldInputProps("adjust-reason", serverErrors.reason, true)}
						autoComplete="off"
						maxLength={reasonLimit}
						onChange={(event) => {
							setReason(event.target.value);
						}}
						placeholder={cargo ? "For example, grain and settlers" : "For example, spoiled stores"}
						value={reason}
					/>
				</Field>

				<div className="flex items-center justify-between gap-4 rounded-lg border border-border px-3 py-3">
					<Label className="grid gap-0.5" htmlFor="adjust-public">
						<span>Log on the public timeline</span>
						<span className="font-normal text-muted-foreground text-sm">
							{isPublic ? "Everyone sees this entry." : "Only you see this entry."}
						</span>
					</Label>
					<Switch checked={isPublic} id="adjust-public" onCheckedChange={setPublic} />
				</div>

				<AdjustPreviewPanel
					error={previewIsCurrent ? preview.error : null}
					hasInput={input !== null}
					isLoading={!previewIsCurrent || preview.isPending}
					preview={previewIsCurrent ? preview.data : undefined}
				/>

				{mutation.error ? (
					<p
						className="rounded-lg border border-ember/40 bg-ember/10 px-3 py-2 text-ember-ink text-sm"
						role="alert"
					>
						{problemMessage(mutation.error, "The stockpile couldn't be changed.")}
						{isConflict(mutation.error)
							? " The preview is fresh; check it and confirm again."
							: null}
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
					<Button disabled={!canConfirm} type="submit">
						{cargo ? <PackageOpenIcon /> : <ScaleIcon />}
						{mutation.isPending ? "Saving…" : title}
					</Button>
				</div>
			</form>
		</BottomSheet>
	);
}

const emptyInput: AdjustInput = {
	arrivalEntryId: null,
	colonists: 0,
	deltas: {},
	expectedVersion: 0,
	public: false,
	reason: "",
	setTo: {},
	source: "ADJUSTMENT",
};

function boundsText(resource: ColonyResource): string {
	if (resource.maxAmount === null) {
		return resource.minAmount === 0 ? "" : `min ${formatNumber(resource.minAmount)}`;
	}
	return `${formatNumber(resource.minAmount)}–${formatNumber(resource.maxAmount)}`;
}

function ResourceRow({
	resource,
	draft,
	error,
	autoFocus,
	onChange,
}: {
	resource: ColonyResource;
	draft: ResourceDraft;
	error: string | undefined;
	autoFocus: boolean;
	onChange: (change: Partial<ResourceDraft>) => void;
}) {
	const id = `adjust-${resource.key}`;
	const bounds = boundsText(resource);
	return (
		<div className="grid gap-2 rounded-lg border border-border bg-surface/60 p-3">
			<div className="flex items-center gap-2">
				<ColonyIcon className="size-4 shrink-0 text-gilt-ink" name={resource.icon} />
				<Label className="min-w-0 flex-1 truncate" htmlFor={id}>
					{resource.label}
				</Label>
				<span className="text-muted-foreground text-sm tabular-nums">
					{formatNumber(resource.amount)}
					{bounds ? ` · ${bounds}` : null}
				</span>
			</div>
			<div className="flex gap-2">
				<fieldset className="inline-flex shrink-0 rounded-md border border-input p-0.5">
					<legend className="sr-only">How to change {resource.label}</legend>
					<ModeButton
						isSelected={draft.mode === "change"}
						label="Change by"
						onSelect={() => {
							onChange({ mode: "change" });
						}}
					/>
					<ModeButton
						isSelected={draft.mode === "set"}
						label="Set to"
						onSelect={() => {
							onChange({ mode: "set" });
						}}
					/>
				</fieldset>
				<Input
					{...fieldInputProps(id, error, false)}
					autoComplete="off"
					autoFocus={autoFocus}
					inputMode="numeric"
					onChange={(event) => {
						onChange({ text: event.target.value });
					}}
					placeholder={draft.mode === "set" ? formatNumber(resource.amount) : "+0"}
					value={draft.text}
				/>
			</div>
			{error ? (
				<p className="text-ember-ink text-sm" id={`${id}-error`}>
					{error}
				</p>
			) : null}
		</div>
	);
}

function ModeButton({
	label,
	isSelected,
	onSelect,
}: {
	label: string;
	isSelected: boolean;
	onSelect: () => void;
}) {
	return (
		<button
			aria-pressed={isSelected}
			className={cn(
				"inline-flex h-10 min-w-[4.5rem] cursor-pointer items-center justify-center rounded px-2.5 font-medium text-sm outline-none transition-colors focus-visible:ring-[3px] focus-visible:ring-ring/45",
				isSelected
					? "bg-primary text-primary-foreground"
					: "text-muted-foreground hover:text-foreground",
			)}
			onClick={onSelect}
			type="button"
		>
			{label}
		</button>
	);
}

function AdjustPreviewPanel({
	hasInput,
	preview,
	isLoading,
	error,
}: {
	hasInput: boolean;
	preview: Schemas["AdjustPreview"] | undefined;
	isLoading: boolean;
	error: unknown;
}) {
	if (!hasInput) {
		return (
			<p className="rounded-lg border border-border border-dashed px-3 py-3 text-muted-foreground text-sm">
				Enter an amount and the preview appears here.
			</p>
		);
	}
	if (error) {
		return (
			<p className="rounded-lg border border-ember/40 bg-ember/10 px-3 py-2 text-ember-ink text-sm">
				{problemMessage(error, "The preview couldn't be worked out.")}
			</p>
		);
	}
	if (isLoading || !preview) {
		return (
			<div aria-busy="true" className="grid gap-2">
				<Skeleton className="h-10" />
				<Skeleton className="h-5 w-2/3" />
			</div>
		);
	}
	return (
		<section aria-label="Preview" aria-live="polite" className="grid gap-2">
			<ResourceChangeList changes={preview.resources} />
			{preview.colonistsAfter === preview.colonistsBefore ? null : (
				<p className="text-sm">
					Colonists {formatNumber(preview.colonistsBefore)} → {formatNumber(preview.colonistsAfter)}
				</p>
			)}
			<p className="font-flavor text-muted-foreground italic">{preview.summary}</p>
		</section>
	);
}
