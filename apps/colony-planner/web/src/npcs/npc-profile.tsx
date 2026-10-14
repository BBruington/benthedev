import { Link } from "@tanstack/react-router";
import { EyeOffIcon } from "lucide-react";
import type { ReactNode } from "react";
import type { Schemas } from "@/api/client";
import { Badge } from "@/components/ui/badge";
import { useCampaign } from "@/hooks/use-campaign";
import { npcHealthLabels, npcRelationLabels, npcStatusLabels } from "@/lib/people";
import { SessionLink } from "@/sessions/session-parts";

type Npc = Schemas["Npc"];
type PlaceReference = Schemas["PlaceReference"];
type SessionReference = Schemas["SessionReference"];

/** A titled block of free text; empty text shows nothing, so a sparse record stays short. */
export function ProseBlock({
	title,
	text,
	dmOnly = false,
}: {
	title: string;
	text: string;
	/** Marks text only the DM receives. */
	dmOnly?: boolean;
}) {
	if (text.trim() === "") {
		return null;
	}
	return (
		<section className="grid gap-1.5">
			<h3 className="flex flex-wrap items-center gap-2 font-medium text-gilt-ink text-xs uppercase tracking-[0.18em]">
				{title}
				{dmOnly ? (
					<Badge className="normal-case tracking-normal" variant="verdigris">
						<EyeOffIcon />
						Only you see this
					</Badge>
				) : null}
			</h3>
			<p className="whitespace-pre-line font-flavor text-lg leading-relaxed [overflow-wrap:anywhere]">
				{text}
			</p>
		</section>
	);
}

function PlaceLink({ place }: { place: PlaceReference }) {
	const { slug } = useCampaign();
	if (!place) {
		return null;
	}
	return (
		<Link
			className="text-gilt-ink underline underline-offset-4"
			params={{ campaignSlug: slug, placeId: place.id }}
			to="/c/$campaignSlug/places/$placeId"
		>
			{place.name}
		</Link>
	);
}

/** The short facts about an NPC, as a definition list; unknown facts are left out. */
export function NpcFacts({ npc }: { npc: Npc }) {
	const facts: { term: string; detail: ReactNode }[] = [];
	function add(term: string, detail: ReactNode, isKnown: boolean) {
		if (isKnown) {
			facts.push({ detail, term });
		}
	}
	add("Race", npc.race, npc.race !== "");
	add("Class", npc.characterClass, npc.characterClass !== "");
	add("Level", npc.level, npc.level !== null);
	add("Age", npc.age, npc.age !== "");
	add("Status", npcStatusLabels[npc.status], true);
	add("Health", npcHealthLabels[npc.health], true);
	add("Relation", npcRelationLabels[npc.relation], true);
	add("Where they are", <PlaceLink place={npc.currentPlace} />, npc.currentPlace !== null);
	add("Where they're from", <PlaceLink place={npc.originPlace} />, npc.originPlace !== null);
	// The API sends null while unset, although the generated type doesn't say so.
	const firstSession: SessionReference | null = npc.firstSession;
	if (firstSession) {
		add("First met", <SessionLink session={firstSession} />, true);
	}
	add("Aliases", npc.aliases.join(", "), npc.aliases.length > 0);
	return (
		<dl className="grid grid-cols-2 gap-x-4 gap-y-3 sm:grid-cols-3">
			{facts.map((fact) => (
				<div className="min-w-0" key={fact.term}>
					<dt className="text-muted-foreground text-xs uppercase tracking-[0.12em]">{fact.term}</dt>
					<dd className="font-medium [overflow-wrap:anywhere]">{fact.detail}</dd>
				</div>
			))}
		</dl>
	);
}
