import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArchiveIcon, CrownIcon, UsersIcon, XIcon } from "lucide-react";
import type { ReactNode } from "react";
import type { Schemas } from "@/api/client";
import { problemMessage } from "@/api/problem";
import { resourcesQuery } from "@/api/queries";
import { ResourceChangeList } from "@/colony/resource-change-list";
import { ColonyIcon } from "@/components/colony-icon";
import { fieldInputProps } from "@/components/field";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { useCampaign, useCampaignId } from "@/hooks/use-campaign";
import { describeEffects, formatNumber, labelFromResourceKey } from "@/lib/colony";
import { cn } from "@/lib/utils";
import {
	type EffectsDraft,
	favorChangeLine,
	inactiveEffectKeys,
	requestStatusLabels,
	withoutEffectResource,
} from "@/lib/ventures";

type Effects = Schemas["Effects"];
type ColonyResource = Schemas["ColonyResource"];
type Request = Schemas["Request"];
type RequestStatus = Schemas["RequestStatus"];

export function FormSection({
	title,
	description,
	children,
	className,
}: {
	title: ReactNode;
	description?: ReactNode;
	children: ReactNode;
	className?: string;
}) {
	return (
		<fieldset className={cn("grid gap-4", className)}>
			<legend className="mb-1 font-medium text-gilt-ink text-xs uppercase tracking-[0.18em]">
				{title}
			</legend>
			{description ? <p className="-mt-2 text-muted-foreground text-sm">{description}</p> : null}
			{children}
		</fieldset>
	);
}

/**
 * The effects of a reward, a failure cost, an action or a grant (spec §5.4): a signed change per
 * resource and new colonists, who land in Unassigned. A change saved for a resource archived since
 * gets its own row to clear, since the server refuses it and the board no longer lists it.
 */
export function EffectsEditor({
	idPrefix,
	resources,
	draft,
	problems,
	onChange,
}: {
	idPrefix: string;
	resources: readonly ColonyResource[];
	draft: EffectsDraft;
	problems: Record<string, string>;
	onChange: (draft: EffectsDraft) => void;
}) {
	// The board always lists food, which can't be archived, so an empty list means it hasn't loaded.
	const inactiveKeys = resources.length === 0 ? [] : inactiveEffectKeys(draft, resources);
	const allResources = useQuery({
		...resourcesQuery(useCampaignId()),
		enabled: inactiveKeys.length > 0,
	});
	return (
		<div className="grid gap-2 sm:grid-cols-2">
			{inactiveKeys.map((key) => {
				const label =
					allResources.data?.resources.find((resource) => resource.key === key)?.label ??
					labelFromResourceKey(key);
				return (
					<div
						className="flex items-center gap-2 rounded-lg border border-ember/40 border-dashed bg-ember/5 py-1 pr-1 pl-3"
						key={key}
					>
						<ArchiveIcon aria-hidden="true" className="size-4 shrink-0 text-ember-ink" />
						<span className="min-w-0 flex-1 truncate">
							{label}{" "}
							<span className="text-muted-foreground text-sm">
								{draft.resources[key]} · archived, clear it to save
							</span>
						</span>
						<Button
							aria-label={`Clear ${label}`}
							onClick={() => {
								onChange(withoutEffectResource(draft, key));
							}}
							size="sm"
							type="button"
							variant="ghost"
						>
							<XIcon />
							Clear
						</Button>
					</div>
				);
			})}
			{resources.map((resource) => {
				const id = `${idPrefix}-${resource.key}`;
				const problem = problems[resource.key];
				return (
					<div className="grid gap-1" key={resource.key}>
						<div className="flex items-center gap-2 rounded-lg border border-border bg-surface/60 py-1 pr-1 pl-3">
							<ColonyIcon className="size-4 shrink-0 text-gilt-ink" name={resource.icon} />
							<Label className="min-w-0 flex-1 truncate" htmlFor={id}>
								{resource.label}
							</Label>
							<Input
								{...fieldInputProps(id, problem, false)}
								autoComplete="off"
								className="w-24 text-right"
								inputMode="text"
								onChange={(event) => {
									onChange({
										...draft,
										resources: { ...draft.resources, [resource.key]: event.target.value },
									});
								}}
								placeholder="+0"
								value={draft.resources[resource.key] ?? ""}
							/>
						</div>
						{problem ? (
							<p className="text-ember-ink text-sm" id={`${id}-error`}>
								{problem}
							</p>
						) : null}
					</div>
				);
			})}
			<div className="grid gap-1">
				<div className="flex items-center gap-2 rounded-lg border border-border bg-surface/60 py-1 pr-1 pl-3">
					<UsersIcon aria-hidden="true" className="size-4 shrink-0 text-gilt-ink" />
					<Label className="min-w-0 flex-1 truncate" htmlFor={`${idPrefix}-colonists`}>
						New colonists
					</Label>
					<Input
						{...fieldInputProps(`${idPrefix}-colonists`, problems.colonists, false)}
						autoComplete="off"
						className="w-24 text-right"
						inputMode="numeric"
						onChange={(event) => {
							onChange({ ...draft, colonists: event.target.value });
						}}
						placeholder="0"
						value={draft.colonists}
					/>
				</div>
				{problems.colonists ? (
					<p className="text-ember-ink text-sm" id={`${idPrefix}-colonists-error`}>
						{problems.colonists}
					</p>
				) : null}
			</div>
		</div>
	);
}

/** "Food +20 · 3 colonists", or `empty` when the effects change nothing. */
export function effectsText(
	effects: Effects,
	resources: readonly { key: string; label: string }[],
	empty = "Nothing",
): string {
	return describeEffects(effects, resources) || empty;
}

/** "3 favor → Food +7": what an action costs and what it brings. */
export function costLine(
	favorCost: number,
	effects: Effects,
	resources: readonly { key: string; label: string }[],
): string {
	return `${formatNumber(favorCost)} favor → ${effectsText(effects, resources)}`;
}

const requestStatusVariants: Record<RequestStatus, "gilt" | "moss" | "ember" | "muted"> = {
	APPROVED: "moss",
	DECLINED: "ember",
	PENDING: "gilt",
	WITHDRAWN: "muted",
};

export function RequestStatusBadge({ status }: { status: RequestStatus }) {
	return <Badge variant={requestStatusVariants[status]}>{requestStatusLabels[status]}</Badge>;
}

export function ArchivedBadge({ archived }: { archived: boolean }) {
	if (!archived) {
		return null;
	}
	return (
		<Badge variant="muted">
			<ArchiveIcon />
			Archived
		</Badge>
	);
}

export function PatronLink({ patron }: { patron: { id: string; name: string } }) {
	const { slug } = useCampaign();
	return (
		<Link
			className="inline-flex items-center gap-1 text-gilt-ink underline underline-offset-4"
			params={{ campaignSlug: slug, patronId: patron.id }}
			to="/c/$campaignSlug/patrons/$patronId"
		>
			<CrownIcon aria-hidden="true" className="size-3.5" />
			{patron.name}
		</Link>
	);
}

/**
 * One request in a list: who asked whom for what, its status, and the decision note when the server
 * sent one (only the requester and the DM receive it).
 */
export function RequestCard({
	request,
	resources,
	actions,
}: {
	request: Request;
	resources: readonly { key: string; label: string }[];
	actions?: ReactNode;
}) {
	const requester = request.onBehalfOf ?? request.requestedBy;
	return (
		<li className="flex flex-col gap-2 rounded-xl border border-border bg-card px-4 py-3 shadow-card sm:flex-row sm:items-start">
			<div className="min-w-0 flex-1 space-y-1">
				<p className="flex flex-wrap items-center gap-2">
					<span className="font-semibold [overflow-wrap:anywhere]">{request.title}</span>
					<RequestStatusBadge status={request.status} />
					{request.mine ? <Badge variant="outline">Yours</Badge> : null}
				</p>
				<p className="text-muted-foreground text-sm [overflow-wrap:anywhere]">
					{requester ? `${requester.name} · ` : ""}
					{request.patronId ? (
						<PatronLink patron={{ id: request.patronId, name: request.patronName }} />
					) : (
						request.patronName
					)}
					{` · Day ${formatNumber(request.requestedDay)}`}
				</p>
				<p className="text-sm tabular-nums">
					{costLine(request.favorCost, request.effects, resources)}
				</p>
				{request.appliedEffects ? (
					<p className="text-muted-foreground text-sm tabular-nums">
						Granted as {effectsText(request.appliedEffects, resources)}
					</p>
				) : null}
				{request.note ? (
					<p className="font-flavor text-muted-foreground italic [overflow-wrap:anywhere]">
						“{request.note}”
					</p>
				) : null}
				{request.decisionNote ? (
					<p className="rounded-lg bg-surface-sunken/60 px-3 py-2 text-sm [overflow-wrap:anywhere]">
						<span className="font-medium">The DM's note: </span>
						{request.decisionNote}
					</p>
				) : null}
			</div>
			{actions ? <div className="flex shrink-0 flex-wrap gap-2">{actions}</div> : null}
		</li>
	);
}

type PreviewShape = {
	resources: Schemas["ResourceChangePreview"][];
	colonistsBefore: number;
	colonistsAfter: number;
	favor: Schemas["FavorChangePreview"];
	summary: string;
};

/** Before → after for a quest's resolution or a grant, favor included (spec §5.6, §6.5). */
export function ChangePreview({
	preview,
	isPending,
	error,
	emptyText,
}: {
	preview: PreviewShape | undefined;
	isPending: boolean;
	error: unknown;
	emptyText: string;
}) {
	if (error) {
		return (
			<p className="rounded-lg border border-ember/40 bg-ember/10 px-3 py-2 text-ember-ink text-sm">
				{problemMessage(error, "The preview couldn't be worked out.")}
			</p>
		);
	}
	if (isPending || !preview) {
		return (
			<div aria-busy="true" className="grid gap-2">
				<Skeleton className="h-10" />
				<Skeleton className="h-5 w-2/3" />
			</div>
		);
	}
	const changesNothing =
		preview.resources.length === 0 &&
		preview.colonistsAfter === preview.colonistsBefore &&
		preview.favor === null;
	return (
		<section aria-label="Preview" aria-live="polite" className="grid gap-2">
			{changesNothing ? <p className="text-muted-foreground">{emptyText}</p> : null}
			<ResourceChangeList changes={preview.resources} />
			{preview.colonistsAfter === preview.colonistsBefore ? null : (
				<p className="text-sm tabular-nums">
					Colonists {formatNumber(preview.colonistsBefore)} → {formatNumber(preview.colonistsAfter)}
				</p>
			)}
			{preview.favor ? (
				<p className="inline-flex items-center gap-2 text-sm tabular-nums">
					<CrownIcon aria-hidden="true" className="size-4 text-gilt-ink" />
					{favorChangeLine(preview.favor)}
				</p>
			) : null}
			{preview.summary ? (
				<p className="font-flavor text-muted-foreground italic">{preview.summary}</p>
			) : null}
		</section>
	);
}

export function FavorCount({ favor, className }: { favor: number; className?: string }) {
	return (
		<span
			className={cn(
				"inline-flex items-center gap-1.5 font-semibold tabular-nums",
				favor < 0 ? "text-ember-ink" : "text-gilt-ink",
				className,
			)}
		>
			<CrownIcon aria-hidden="true" className="size-4" />
			{formatNumber(favor)} favor
		</span>
	);
}
