import { BanIcon, BrickWallIcon, HardHatIcon, HouseIcon } from "lucide-react";
import type { Schemas } from "@/api/client";
import { Button } from "@/components/ui/button";
import { dayName, formatNumber } from "@/lib/colony";
import {
	amountsText,
	isLineDone,
	progressPercent,
	progressText,
	projectedFinishText,
} from "@/lib/construction";
import { HiddenBadge } from "@/people/people-sections";
import { effectsText } from "@/ventures/venture-parts";

type Building = Schemas["Building"];
type Labelled = { key: string; label: string };

function LaborProgress({
	building,
	labelOf,
}: {
	building: Building;
	labelOf: (roleKey: string) => string;
}) {
	return (
		<ul className="grid gap-3">
			{building.labor.map((line) => {
				const label = labelOf(line.roleKey);
				const percent = progressPercent(line);
				let crewText = `Crew ${formatNumber(line.crew)}`;
				if (isLineDone(line)) {
					crewText = "Done";
				} else if (line.maxCrew !== null) {
					crewText = `Crew ${formatNumber(line.crew)} of ${formatNumber(line.maxCrew)}`;
				}
				return (
					<li className="grid gap-1.5" key={line.roleKey}>
						<p className="flex flex-wrap items-baseline justify-between gap-x-3 tabular-nums">
							<span className="font-medium">{progressText(line, label)}</span>
							<span className="text-muted-foreground text-sm">{crewText}</span>
						</p>
						<div aria-hidden="true" className="h-2 overflow-hidden rounded-full bg-surface-sunken">
							<div
								className={isLineDone(line) ? "h-full bg-moss" : "h-full bg-gilt"}
								style={{ width: `${percent}%` }}
							/>
						</div>
					</li>
				);
			})}
		</ul>
	);
}

/** A building under construction: each line's progress and crew, its finish day, and its actions. */
export function ProjectCard({
	building,
	blueprint,
	labelOf,
	isDm,
	onSetCrews,
	onCancel,
}: {
	building: Building;
	blueprint: { dm?: { revealed: boolean } } | undefined;
	labelOf: (roleKey: string) => string;
	isDm: boolean;
	onSetCrews: (building: Building) => void;
	onCancel: (building: Building) => void;
}) {
	const isStalled = building.projectedFinishDay === null;
	return (
		<li className="flex flex-col gap-4 rounded-xl border border-border bg-card p-4 shadow-card sm:p-5">
			<div className="space-y-1">
				<h3 className="font-semibold text-lg leading-snug [overflow-wrap:anywhere]">
					{building.name}
				</h3>
				<p className="flex flex-wrap items-center gap-2 text-muted-foreground text-sm">
					Started on {dayName(building.startedDay)}
					{blueprint ? <HiddenBadge item={blueprint} /> : null}
				</p>
			</div>
			<LaborProgress building={building} labelOf={labelOf} />
			<p className={isStalled ? "font-semibold text-ember-ink" : "font-semibold text-gilt-ink"}>
				{projectedFinishText(building, labelOf)}
			</p>
			<div className="flex flex-wrap gap-2">
				<Button
					className="flex-1 sm:flex-none"
					onClick={() => {
						onSetCrews(building);
					}}
				>
					<HardHatIcon />
					Set crews
				</Button>
				{isDm ? (
					<Button
						onClick={() => {
							onCancel(building);
						}}
						variant="ghost"
					>
						<BanIcon />
						Cancel
					</Button>
				) : null}
			</div>
		</li>
	);
}

/** A finished building: a record of what stands, with Demolish for the DM. */
export function StandingCard({
	building,
	blueprint,
	resources,
	isDm,
	onDemolish,
}: {
	building: Building;
	blueprint: { dm?: { revealed: boolean } } | undefined;
	resources: readonly Labelled[];
	isDm: boolean;
	onDemolish: (building: Building) => void;
}) {
	const completion = effectsText(building.completionEffects, resources, "");
	return (
		<li className="flex flex-col gap-3 rounded-xl border border-moss/35 bg-card p-4 shadow-card sm:p-5">
			<div className="space-y-1">
				<h3 className="flex items-center gap-2 font-semibold text-lg leading-snug [overflow-wrap:anywhere]">
					<HouseIcon aria-hidden="true" className="size-5 shrink-0 text-moss-ink" />
					{building.name}
				</h3>
				<p className="flex flex-wrap items-center gap-2 text-muted-foreground text-sm">
					Finished on {dayName(building.finishedDay ?? building.startedDay)}
					{blueprint ? <HiddenBadge item={blueprint} /> : null}
				</p>
				{completion ? <p className="text-sm tabular-nums">When finished: {completion}</p> : null}
			</div>
			{isDm ? (
				<div className="flex flex-wrap gap-2">
					<Button
						onClick={() => {
							onDemolish(building);
						}}
						variant="ghost"
					>
						<BrickWallIcon />
						Demolish
					</Button>
				</div>
			) : null}
		</li>
	);
}

/** Cancelled and demolished buildings, for the record. */
export function PastBuildings({
	buildings,
	resources,
}: {
	buildings: readonly Building[];
	resources: readonly Labelled[];
}) {
	return (
		<ul className="flex flex-col divide-y divide-border">
			{buildings.map((building) => {
				const ended = dayName(building.endedDay ?? building.startedDay);
				const status =
					building.status === "CANCELLED" ? `cancelled on ${ended}` : `demolished on ${ended}`;
				const refund = building.refund ? amountsText(building.refund, resources) : "";
				return (
					<li className="py-2 first:pt-0 last:pb-0" key={building.id}>
						<p className="font-semibold [overflow-wrap:anywhere]">
							{building.name} · {status}
						</p>
						<p className="text-muted-foreground text-sm tabular-nums">
							Started on {dayName(building.startedDay)}
							{building.finishedDay === null
								? ""
								: `, finished on ${dayName(building.finishedDay)}`}
							{refund ? ` · gave back ${refund}` : " · gave back nothing"}
						</p>
					</li>
				);
			})}
		</ul>
	);
}
