import { HammerIcon, HardHatIcon } from "lucide-react";
import { type FormEvent, useState } from "react";
import type { Schemas } from "@/api/client";
import { fieldErrors } from "@/api/problem";
import type { ColonyChanges } from "@/colony/use-colony-changes";
import { BottomSheet } from "@/components/bottom-sheet";
import { Button } from "@/components/ui/button";
import { formatNumber, labelFromResourceKey } from "@/lib/colony";
import {
	type CrewLine,
	costShortfall,
	isLineDone,
	type NumberDraft,
	parseCrews,
	progressText,
} from "@/lib/construction";
import { ChangeError, useReviewedVersion } from "@/people/workforce-sheets";
import { CrewFields } from "./crew-fields";

type BuildingBlueprint = Schemas["BuildingBlueprint"];
type Building = Schemas["Building"];
type ConstructionRole = Schemas["ConstructionRole"];
type ColonyResource = Schemas["ColonyResource"];

const startFormId = "start-building-form";
const crewsFormId = "building-crews-form";

/**
 * Start a building (any active member, construction design §1): the cost against the stockpile,
 * with any shortfall named before submit, and the first crew of each labor line. Start takes the
 * whole cost at once; the server refuses it if the colony moved on or can't pay.
 */
export function StartSheet({
	open,
	onOpenChange,
	blueprint,
	roles,
	resources,
	version,
	changes,
	labelOf,
	onStarted,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	blueprint: BuildingBlueprint;
	roles: readonly ConstructionRole[];
	resources: readonly ColonyResource[];
	version: number;
	changes: ColonyChanges;
	labelOf: (roleKey: string) => string;
	onStarted: () => void;
}) {
	const start = changes.startBuilding;
	const reviewedVersion = useReviewedVersion(version, start);
	const [draft, setDraft] = useState<NumberDraft>({});
	const lines: CrewLine[] = blueprint.labor.map((line) => ({
		crew: 0,
		maxCrew: line.maxCrew,
		roleKey: line.roleKey,
	}));
	const { crews, problems } = parseCrews(draft, lines, roles);
	const shortfall = costShortfall(blueprint.cost, resources);
	const serverErrors = fieldErrors(start.error);
	const canStart = Object.keys(problems).length === 0 && shortfall === null && !start.isPending;
	const costKeys = Object.keys(blueprint.cost);

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!canStart) {
			return;
		}
		start.mutate(
			{
				body: {
					blueprintId: blueprint.id,
					blueprintUpdatedAt: blueprint.updatedAt,
					crews,
					expectedVersion: reviewedVersion,
				},
				...changes.inCampaign,
			},
			{
				onSuccess: () => {
					onOpenChange(false);
					onStarted();
				},
			},
		);
	}

	return (
		<BottomSheet
			description="The whole cost leaves the stockpile now. Each crew member adds a day of work a day."
			footer={
				<>
					<Button
						onClick={() => {
							onOpenChange(false);
						}}
						variant="ghost"
					>
						Not now
					</Button>
					<Button disabled={!canStart} form={startFormId} type="submit">
						<HammerIcon />
						{start.isPending ? "Starting…" : `Start the ${blueprint.name}`}
					</Button>
				</>
			}
			onOpenChange={onOpenChange}
			open={open}
			title={`Start the ${blueprint.name}`}
		>
			<form className="grid gap-5" id={startFormId} noValidate onSubmit={handleSubmit}>
				<section aria-labelledby="start-cost-heading" className="grid gap-2">
					<h3 className="font-medium" id="start-cost-heading">
						Cost
					</h3>
					{costKeys.length === 0 ? (
						<p className="text-muted-foreground">It costs nothing.</p>
					) : (
						<ul className="grid gap-1.5">
							{costKeys.map((key) => {
								const resource = resources.find((candidate) => candidate.key === key);
								return (
									<li
										className="flex flex-wrap items-center justify-between gap-x-3 rounded-lg bg-surface-sunken/60 px-3 py-2 tabular-nums"
										key={key}
									>
										<span className="font-medium">
											{resource?.label ?? labelFromResourceKey(key)}{" "}
											{formatNumber(blueprint.cost[key] ?? 0)}
										</span>
										<span className="text-muted-foreground text-sm">
											{resource ? `the colony has ${formatNumber(resource.amount)}` : "archived"}
										</span>
									</li>
								);
							})}
						</ul>
					)}
					{shortfall ? (
						<p
							className="rounded-lg border border-ember/40 bg-ember/10 px-3 py-2 text-ember-ink text-sm"
							role="alert"
						>
							{shortfall}
						</p>
					) : null}
				</section>
				<section aria-labelledby="start-crews-heading" className="grid gap-2">
					<h3 className="font-medium" id="start-crews-heading">
						First crews
					</h3>
					<CrewFields
						detailOf={(line) => {
							const labor = blueprint.labor.find((candidate) => candidate.roleKey === line.roleKey);
							return labor ? `${formatNumber(labor.effort)} ${labelOf(line.roleKey)}-days` : null;
						}}
						draft={draft}
						idPrefix="start-crew"
						labelOf={labelOf}
						lines={lines}
						onChange={(next) => {
							setDraft(next);
							if (start.isError) {
								start.reset();
							}
						}}
						problems={problems}
						roles={roles}
					/>
					{serverErrors.crews ? (
						<p className="text-ember-ink text-sm">{serverErrors.crews}</p>
					) : null}
					{serverErrors.blueprintId ? (
						<p className="text-ember-ink text-sm">{serverErrors.blueprintId}</p>
					) : null}
				</section>
				<ChangeError error={start.error} fallback="The building couldn't be started." />
			</form>
		</BottomSheet>
	);
}

/**
 * Set crews on a building under construction (any active member). It sends the whole crew map from
 * the version the sheet opened with, so two phones moving crews at once can't overwrite each other.
 */
export function CrewsSheet({
	open,
	onOpenChange,
	building,
	roles,
	version,
	changes,
	labelOf,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	building: Building;
	roles: readonly ConstructionRole[];
	version: number;
	changes: ColonyChanges;
	labelOf: (roleKey: string) => string;
}) {
	const setCrews = changes.setBuildingCrews;
	const reviewedVersion = useReviewedVersion(version, setCrews);
	const openLines = building.labor.filter((line) => !isLineDone(line));
	const [draft, setDraft] = useState<NumberDraft>(() =>
		Object.fromEntries(openLines.map((line) => [line.roleKey, String(line.crew)])),
	);
	const { crews, problems } = parseCrews(draft, openLines, roles);
	const serverErrors = fieldErrors(setCrews.error);
	const canSave = Object.keys(problems).length === 0 && !setCrews.isPending;

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (!canSave) {
			return;
		}
		const allCrews = Object.fromEntries(
			building.labor.map((line) => [line.roleKey, crews[line.roleKey] ?? 0]),
		);
		setCrews.mutate(
			{
				body: { crews: allCrews, expectedVersion: reviewedVersion },
				params: { path: { ...changes.inCampaign.params.path, buildingId: building.id } },
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
			description="Each crew member adds a day of work a day. A finished line's crew goes back to idle."
			footer={
				<>
					<Button
						onClick={() => {
							onOpenChange(false);
						}}
						variant="ghost"
					>
						Cancel
					</Button>
					<Button disabled={!canSave} form={crewsFormId} type="submit">
						<HardHatIcon />
						{setCrews.isPending ? "Saving…" : "Save crews"}
					</Button>
				</>
			}
			onOpenChange={onOpenChange}
			open={open}
			title={`Crews for the ${building.name}`}
		>
			<form className="grid gap-4" id={crewsFormId} noValidate onSubmit={handleSubmit}>
				<CrewFields
					detailOf={(line) => {
						const labor = building.labor.find((candidate) => candidate.roleKey === line.roleKey);
						return labor ? progressText(labor, labelOf(line.roleKey)) : null;
					}}
					draft={draft}
					idPrefix="building-crew"
					labelOf={labelOf}
					lines={openLines}
					onChange={(next) => {
						setDraft(next);
						if (setCrews.isError) {
							setCrews.reset();
						}
					}}
					problems={problems}
					roles={roles}
				/>
				{serverErrors.crews ? <p className="text-ember-ink text-sm">{serverErrors.crews}</p> : null}
				<ChangeError error={setCrews.error} fallback="The crews couldn't be changed." />
			</form>
		</BottomSheet>
	);
}
