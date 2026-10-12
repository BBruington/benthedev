import { keepPreviousData, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { DramaIcon, PlusIcon } from "lucide-react";
import { type ReactNode, useState } from "react";
import { problemMessage } from "@/api/problem";
import { npcQuery, npcsQuery } from "@/api/queries";
import { PageHeader } from "@/components/page-header";
import { StatusPanel } from "@/components/status-panel";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
	fallbackCampaignName,
	useCampaign,
	useCampaignId,
	useCampaignName,
} from "@/hooks/use-campaign";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { useMe } from "@/hooks/use-me";
import { useSheetState } from "@/hooks/use-sheet-state";
import { canUseDmTools } from "@/lib/access";
import { focusFirstVisible } from "@/lib/focus";
import { hasRosterFilters, keepsNpc, noRosterFilters, rosterQuery } from "@/lib/people";
import { countOf } from "@/lib/words";
import { npcHeadingId, npcsPageHeadingId } from "@/npcs/npc-ids";
import { NpcCard, RosterFilterBar } from "@/npcs/npc-roster";
import { NpcSheet } from "@/npcs/npc-sheet";

export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell/npcs/")({
	component: NpcRosterPage,
	loader: ({ context: { campaignId, queryClient } }) => {
		if (campaignId) {
			queryClient.prefetchQuery(npcsQuery(campaignId));
		}
	},
});

const searchDelayInMilliseconds = 250;

/** /npcs (spec §6.2): the roster, searched and filtered; players get revealed NPCs only. */
function NpcRosterPage() {
	useDocumentTitle("NPCs", useCampaignName() ?? fallbackCampaignName);
	const me = useMe();
	const isDm = canUseDmTools(me);
	const campaignId = useCampaignId();
	const { slug } = useCampaign();
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const [filters, setFilters] = useState(noRosterFilters);
	const settledFilters = useDebouncedValue(filters, searchDelayInMilliseconds);
	const npcs = useQuery({
		...npcsQuery(campaignId, rosterQuery(settledFilters, isDm)),
		placeholderData: keepPreviousData,
	});
	const adding = useSheetState<true>();

	const shown = (npcs.data?.npcs ?? []).filter((npc) => keepsNpc(settledFilters, npc));
	let body: ReactNode;
	if (npcs.isPending) {
		body = (
			<div aria-busy="true" className="grid gap-2 md:grid-cols-2">
				<Skeleton className="h-24 rounded-xl" />
				<Skeleton className="h-24 rounded-xl" />
				<Skeleton className="h-24 rounded-xl" />
			</div>
		);
	} else if (npcs.isError) {
		body = (
			<StatusPanel
				description={problemMessage(npcs.error, "The NPCs couldn't be loaded.")}
				icon={DramaIcon}
				title="The NPCs couldn't be loaded"
				tone="ember"
			/>
		);
	} else if (shown.length === 0) {
		let emptyText = "The party hasn't met anyone yet.";
		if (hasRosterFilters(settledFilters)) {
			emptyText = "Nobody matches. Try other filters.";
		} else if (isDm) {
			emptyText = "No NPCs yet. Add the first one.";
		}
		body = (
			<p className="rounded-xl border border-border border-dashed px-4 py-6 text-center text-muted-foreground">
				{emptyText}
			</p>
		);
	} else {
		body = (
			<ul aria-label="NPCs" className="grid gap-2 md:grid-cols-2">
				{shown.map((npc) => (
					<NpcCard key={npc.id} npc={npc} />
				))}
			</ul>
		);
	}

	return (
		<div className="flex flex-col gap-6">
			<PageHeader
				actions={
					isDm ? (
						<Button
							onClick={() => {
								adding.show(true);
							}}
						>
							<PlusIcon />
							Add an NPC
						</Button>
					) : null
				}
				description={
					isDm
						? "Everyone in your campaign. Players see only those visible to them, under the name they know."
						: "The people the party knows, by the names they go by."
				}
				headingId={npcsPageHeadingId}
				title="NPCs"
			/>
			<RosterFilterBar filters={filters} isDm={isDm} onChange={setFilters} />
			<p aria-live="polite" className="sr-only">
				{npcs.data ? `${countOf(shown.length, "NPC")} shown` : ""}
			</p>
			{body}
			{isDm && adding.value ? (
				<NpcSheet
					key={adding.session}
					npc={null}
					onCreated={(created) => {
						// Seeded, so the new page's heading is there to take focus once the navigation settles.
						queryClient.setQueryData(npcQuery(campaignId, created.id).queryKey, created);
						navigate({
							params: { campaignSlug: slug, npcId: created.id },
							to: "/c/$campaignSlug/npcs/$npcId",
						}).then(() => {
							focusFirstVisible(npcHeadingId);
						});
					}}
					onOpenChange={adding.onOpenChange}
					open={adding.open}
				/>
			) : null}
		</div>
	);
}
