import { ArchiveIcon, ArchiveRestoreIcon, HammerIcon, PenLineIcon } from "lucide-react";
import type { Schemas } from "@/api/client";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { amountsText, laborText } from "@/lib/construction";
import { cn } from "@/lib/utils";
import { HiddenBadge } from "@/people/people-sections";
import { ArchivedBadge, effectsText } from "@/ventures/venture-parts";

type BuildingBlueprint = Schemas["BuildingBlueprint"];
type Labelled = { key: string; label: string };

/** The DM's controls on a blueprint card; each switch shows the value it is saving. */
export type BlueprintControls = {
	onEdit: (blueprint: BuildingBlueprint) => void;
	onReveal: (blueprint: BuildingBlueprint, revealed: boolean) => void;
	onArchive: (blueprint: BuildingBlueprint, archived: boolean) => void;
	/** The visibility being saved for a blueprint, while it saves. */
	pendingReveal: { blueprintId: string; revealed: boolean } | null;
	pendingArchiveId: string | null;
};

/** A blueprint: what it costs, the work it needs and what finishing it brings, with Start. */
export function BlueprintCard({
	blueprint,
	resources,
	labelOf,
	onStart,
	dmControls,
}: {
	blueprint: BuildingBlueprint;
	resources: readonly Labelled[];
	labelOf: (roleKey: string) => string;
	onStart: (blueprint: BuildingBlueprint) => void;
	/** Present only for the DM (not in view as player). */
	dmControls: BlueprintControls | null;
}) {
	const cost = amountsText(blueprint.cost, resources) || "Nothing";
	const completion = effectsText(blueprint.completionEffects, resources, "");
	return (
		<li
			className={cn(
				"flex flex-col gap-3 rounded-xl border border-border bg-card p-4 shadow-card sm:p-5",
				blueprint.archived && "opacity-75",
			)}
		>
			<div className="space-y-1">
				<h3 className="font-semibold text-lg leading-snug [overflow-wrap:anywhere]">
					{blueprint.name}
				</h3>
				{blueprint.description ? (
					<p className="whitespace-pre-line text-muted-foreground [overflow-wrap:anywhere]">
						{blueprint.description}
					</p>
				) : null}
				{dmControls ? (
					<p className="flex flex-wrap gap-1.5">
						<HiddenBadge item={blueprint} />
						<ArchivedBadge archived={blueprint.archived} />
					</p>
				) : null}
			</div>
			<dl className="grid gap-1.5 text-sm tabular-nums">
				<div className="flex flex-wrap gap-x-2">
					<dt className="font-medium">Cost</dt>
					<dd>{cost}</dd>
				</div>
				<div className="flex flex-wrap gap-x-2">
					<dt className="font-medium">Labor</dt>
					<dd>
						{blueprint.labor.map((line) => laborText(line, labelOf(line.roleKey))).join(" · ")}
					</dd>
				</div>
				{completion ? (
					<div className="flex flex-wrap gap-x-2">
						<dt className="font-medium">When finished</dt>
						<dd>{completion}</dd>
					</div>
				) : null}
			</dl>
			{dmControls ? (
				<DmBlueprintControls blueprint={blueprint} controls={dmControls} onStart={onStart} />
			) : (
				<Button
					onClick={() => {
						onStart(blueprint);
					}}
				>
					<HammerIcon />
					Start
				</Button>
			)}
		</li>
	);
}

function DmBlueprintControls({
	blueprint,
	controls,
	onStart,
}: {
	blueprint: BuildingBlueprint;
	controls: BlueprintControls;
	onStart: (blueprint: BuildingBlueprint) => void;
}) {
	const revealed = blueprint.dm?.revealed ?? true;
	const pending = controls.pendingReveal;
	const isRevealing = pending?.blueprintId === blueprint.id;
	const shownRevealed = isRevealing ? pending.revealed : revealed;
	const isArchiving = controls.pendingArchiveId === blueprint.id;
	const switchId = `blueprint-visible-${blueprint.id}`;
	return (
		<div className="grid gap-2">
			<div className="flex min-h-11 items-center justify-between gap-3 rounded-lg border border-border px-3">
				<Label htmlFor={switchId}>Visible to players</Label>
				<Switch
					checked={shownRevealed}
					disabled={isRevealing}
					id={switchId}
					onCheckedChange={(checked) => {
						controls.onReveal(blueprint, checked);
					}}
				/>
			</div>
			<div className="flex flex-wrap gap-2">
				<Button
					disabled={blueprint.archived}
					onClick={() => {
						onStart(blueprint);
					}}
				>
					<HammerIcon />
					Start
				</Button>
				<Button
					onClick={() => {
						controls.onEdit(blueprint);
					}}
					variant="outline"
				>
					<PenLineIcon />
					Edit
				</Button>
				<Button
					disabled={isArchiving}
					onClick={() => {
						controls.onArchive(blueprint, !blueprint.archived);
					}}
					variant="ghost"
				>
					{blueprint.archived ? <ArchiveRestoreIcon /> : <ArchiveIcon />}
					{blueprint.archived ? "Unarchive" : "Archive"}
				</Button>
			</div>
		</div>
	);
}
