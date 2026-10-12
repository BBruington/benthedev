import { Link } from "@tanstack/react-router";
import { SearchIcon, XIcon } from "lucide-react";
import type { Schemas } from "@/api/client";
import { ColonyIcon } from "@/components/colony-icon";
import { NativeSelect } from "@/components/form-controls";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useCampaign } from "@/hooks/use-campaign";
import {
	type ColonistFilter,
	enumValues,
	type HiddenFilter,
	hasRosterFilters,
	hiddenTrueName,
	noRosterFilters,
	npcRelationLabels,
	npcStatusLabels,
	type RosterFilters,
} from "@/lib/people";
import { HiddenBadge, NpcAvatar } from "@/people/people-sections";

type Npc = Schemas["Npc"];

const relationVariants: Record<Schemas["NpcRelation"], "moss" | "muted" | "gilt" | "ember"> = {
	ALLY: "moss",
	ENEMY: "ember",
	NEUTRAL: "muted",
	SUSPICIOUS: "gilt",
	UNKNOWN: "muted",
};

/** Status and relation as badges; "Alive" and an unknown relation go unsaid. */
export function NpcBadges({ npc }: { npc: Npc }) {
	return (
		<>
			{npc.status === "ALIVE" ? null : (
				<Badge variant={npc.status === "DEAD" ? "ember" : "muted"}>
					{npcStatusLabels[npc.status]}
				</Badge>
			)}
			{npc.relation === "UNKNOWN" ? null : (
				<Badge variant={relationVariants[npc.relation]}>{npcRelationLabels[npc.relation]}</Badge>
			)}
			{npc.colonyRole ? (
				<Badge variant="verdigris">
					<ColonyIcon aria-hidden="true" name={npc.colonyRole.icon} />
					{npc.colonyRole.label}
				</Badge>
			) : null}
		</>
	);
}

function summaryLine(npc: Npc): string {
	const parts = [npc.race, npc.characterClass, npc.level === null ? "" : `Level ${npc.level}`];
	return parts.filter((part) => part.trim() !== "").join(" · ");
}

/** One NPC on the roster, linking to their page. */
export function NpcCard({ npc }: { npc: Npc }) {
	const { slug } = useCampaign();
	const summary = summaryLine(npc);
	const trueName = hiddenTrueName(npc);
	return (
		<li className="relative flex items-start gap-3 rounded-xl border border-border bg-card px-4 py-3 shadow-card transition-colors focus-within:ring-[3px] focus-within:ring-ring/40 hover:border-gilt/40">
			<NpcAvatar displayName={npc.displayName} />
			<div className="min-w-0 flex-1">
				<h2 className="font-semibold text-base leading-snug [overflow-wrap:anywhere]">
					<Link
						className="outline-none after:absolute after:inset-0 after:rounded-xl"
						params={{ campaignSlug: slug, npcId: npc.id }}
						to="/c/$campaignSlug/npcs/$npcId"
					>
						{npc.displayName}
					</Link>
				</h2>
				{trueName ? (
					<p className="text-muted-foreground text-sm [overflow-wrap:anywhere]">
						True name {trueName}
					</p>
				) : null}
				{summary ? <p className="text-muted-foreground text-sm">{summary}</p> : null}
				<p className="mt-1.5 flex flex-wrap gap-1.5">
					<HiddenBadge item={npc} />
					<NpcBadges npc={npc} />
				</p>
			</div>
		</li>
	);
}

const colonistChoices: Record<ColonistFilter, string> = {
	all: "Colonists or not",
	colonists: "Colonists",
	others: "Not colonists",
};

const hiddenChoices: Record<HiddenFilter, string> = {
	all: "Hidden or visible",
	hidden: "Hidden",
	visible: "Visible to players",
};

/** Search and filters over the roster; the Hidden filter is the DM's alone. */
export function RosterFilterBar({
	filters,
	isDm,
	onChange,
}: {
	filters: RosterFilters;
	isDm: boolean;
	onChange: (filters: RosterFilters) => void;
}) {
	function update<Key extends keyof RosterFilters>(key: Key, value: RosterFilters[Key]) {
		onChange({ ...filters, [key]: value });
	}
	return (
		<search className="grid gap-3 rounded-xl border border-border bg-card p-3 shadow-card sm:p-4">
			<div className="relative">
				<Label className="sr-only" htmlFor="npc-search">
					Search NPCs
				</Label>
				<SearchIcon
					aria-hidden="true"
					className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground"
				/>
				<Input
					autoComplete="off"
					className="pl-9"
					id="npc-search"
					maxLength={80}
					onChange={(event) => {
						update("search", event.target.value);
					}}
					placeholder={isDm ? "Search names and aliases" : "Search by name or alias"}
					type="search"
					value={filters.search}
				/>
			</div>
			<div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
				<NativeSelect
					aria-label="Status"
					onChange={(event) => {
						const status = enumValues(npcStatusLabels).find(
							(value) => value === event.target.value,
						);
						update("status", status ?? "");
					}}
					value={filters.status}
				>
					<option value="">Any status</option>
					{enumValues(npcStatusLabels).map((value) => (
						<option key={value} value={value}>
							{npcStatusLabels[value]}
						</option>
					))}
				</NativeSelect>
				<NativeSelect
					aria-label="Relation"
					onChange={(event) => {
						const relation = enumValues(npcRelationLabels).find(
							(value) => value === event.target.value,
						);
						update("relation", relation ?? "");
					}}
					value={filters.relation}
				>
					<option value="">Any relation</option>
					{enumValues(npcRelationLabels).map((value) => (
						<option key={value} value={value}>
							{npcRelationLabels[value]}
						</option>
					))}
				</NativeSelect>
				<NativeSelect
					aria-label="Colonists"
					onChange={(event) => {
						const choice = enumValues(colonistChoices).find(
							(value) => value === event.target.value,
						);
						update("colonists", choice ?? "all");
					}}
					value={filters.colonists}
				>
					{enumValues(colonistChoices).map((value) => (
						<option key={value} value={value}>
							{colonistChoices[value]}
						</option>
					))}
				</NativeSelect>
				{isDm ? (
					<NativeSelect
						aria-label="Visibility"
						onChange={(event) => {
							const choice = enumValues(hiddenChoices).find(
								(value) => value === event.target.value,
							);
							update("hidden", choice ?? "all");
						}}
						value={filters.hidden}
					>
						{enumValues(hiddenChoices).map((value) => (
							<option key={value} value={value}>
								{hiddenChoices[value]}
							</option>
						))}
					</NativeSelect>
				) : null}
			</div>
			{hasRosterFilters(filters) ? (
				<div>
					<Button
						onClick={() => {
							onChange(noRosterFilters);
						}}
						size="sm"
						variant="ghost"
					>
						<XIcon />
						Clear filters
					</Button>
				</div>
			) : null}
		</search>
	);
}
