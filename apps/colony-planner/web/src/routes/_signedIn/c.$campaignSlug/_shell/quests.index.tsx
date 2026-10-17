import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { CrownIcon, PlusIcon, SwordsIcon } from "lucide-react";
import type { ReactNode } from "react";
import type { Schemas } from "@/api/client";
import { problemMessage } from "@/api/problem";
import { colonyBoardQuery, questQuery, questsQuery } from "@/api/queries";
import { PageHeader } from "@/components/page-header";
import { StatusPanel } from "@/components/status-panel";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
	fallbackCampaignName,
	useCampaign,
	useCampaignId,
	useCampaignName,
} from "@/hooks/use-campaign";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { useMe } from "@/hooks/use-me";
import { useSheetState } from "@/hooks/use-sheet-state";
import { canUseDmTools } from "@/lib/access";
import { dayName, formatNumber } from "@/lib/colony";
import { focusFirstVisible } from "@/lib/focus";
import { groupQuestsByStatus, questKindLabels, questStatusLabels } from "@/lib/ventures";
import { HiddenBadge } from "@/people/people-sections";
import { questHeadingId, questsPageHeadingId } from "@/ventures/ids";
import { QuestSheet } from "@/ventures/quest-sheet";
import { effectsText } from "@/ventures/venture-parts";

type Quest = Schemas["Quest"];
type QuestStatus = Schemas["QuestStatus"];

export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell/quests/")({
	component: QuestsPage,
	loader: ({ context: { campaignId, queryClient } }) => {
		if (campaignId) {
			queryClient.prefetchQuery(questsQuery(campaignId));
			queryClient.prefetchQuery(colonyBoardQuery(campaignId));
		}
	},
});

const shelves: { status: QuestStatus; empty: string }[] = [
	{ empty: "No quests open right now.", status: "ACTIVE" },
	{ empty: "Nothing finished yet.", status: "COMPLETED" },
	{ empty: "No failed quests.", status: "FAILED" },
];

/** /quests (spec §6.2): Active, Done and Failed; players get the quests visible to them. */
function QuestsPage() {
	useDocumentTitle("Quests", useCampaignName() ?? fallbackCampaignName);
	const me = useMe();
	const isDm = canUseDmTools(me);
	const campaignId = useCampaignId();
	const { slug } = useCampaign();
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const quests = useQuery(questsQuery(campaignId));
	const board = useQuery(colonyBoardQuery(campaignId));
	const adding = useSheetState<true>();
	const resources = board.data?.resources ?? [];

	let body: ReactNode;
	if (quests.isPending) {
		body = (
			<div aria-busy="true" className="grid gap-2 md:grid-cols-2">
				<Skeleton className="h-24 rounded-xl" />
				<Skeleton className="h-24 rounded-xl" />
			</div>
		);
	} else if (quests.isError) {
		body = (
			<StatusPanel
				description={problemMessage(quests.error, "The quests couldn't be loaded.")}
				icon={SwordsIcon}
				title="The quests couldn't be loaded"
				tone="ember"
			/>
		);
	} else {
		const groups = groupQuestsByStatus(quests.data.quests);
		body = shelves.map(({ status, empty }) => (
			<section aria-labelledby={`quests-${status}`} className="flex flex-col gap-3" key={status}>
				<h2
					className="flex items-center gap-2 font-display font-semibold text-xl tracking-wide"
					id={`quests-${status}`}
				>
					{questStatusLabels[status]}
					<span className="font-sans font-normal text-base text-muted-foreground">
						{formatNumber(groups[status].length)}
					</span>
				</h2>
				{groups[status].length === 0 ? (
					<p className="rounded-xl border border-border border-dashed px-4 py-4 text-muted-foreground">
						{empty}
					</p>
				) : (
					<ul className="grid gap-2 md:grid-cols-2">
						{groups[status].map((quest) => (
							<QuestCard key={quest.id} quest={quest} resources={resources} />
						))}
					</ul>
				)}
			</section>
		));
	}

	return (
		<div className="flex flex-col gap-8">
			<PageHeader
				actions={
					isDm ? (
						<Button
							onClick={() => {
								adding.show(true);
							}}
						>
							<PlusIcon />
							New quest
						</Button>
					) : null
				}
				description={
					isDm
						? "Every quest and mission. Players see the ones visible to them, never the failure cost or your notes."
						: "What the colony has been asked to do, and what it pays."
				}
				headingId={questsPageHeadingId}
				title="Quests"
			/>
			{body}
			{isDm && adding.value ? (
				<QuestSheet
					key={adding.session}
					onCreated={(created) => {
						queryClient.setQueryData(questQuery(campaignId, created.id).queryKey, created);
						navigate({
							params: { campaignSlug: slug, questId: created.id },
							to: "/c/$campaignSlug/quests/$questId",
						}).then(() => {
							focusFirstVisible(questHeadingId);
						});
					}}
					onOpenChange={adding.onOpenChange}
					open={adding.open}
					quest={null}
				/>
			) : null}
		</div>
	);
}

function QuestCard({
	quest,
	resources,
}: {
	quest: Quest;
	resources: readonly { key: string; label: string }[];
}) {
	const { slug } = useCampaign();
	const reward = effectsText(quest.rewardEffects, resources, "");
	const giver = quest.patron?.name ?? quest.giverNpc?.displayName;
	return (
		<li className="relative flex flex-col gap-1.5 rounded-xl border border-border bg-card px-4 py-3 shadow-card transition-colors focus-within:ring-[3px] focus-within:ring-ring/40 hover:border-gilt/40">
			<h3 className="font-semibold text-base leading-snug [overflow-wrap:anywhere]">
				<Link
					className="outline-none after:absolute after:inset-0 after:rounded-xl"
					params={{ campaignSlug: slug, questId: quest.id }}
					to="/c/$campaignSlug/quests/$questId"
				>
					{quest.title}
				</Link>
			</h3>
			{giver ? (
				<p className="text-muted-foreground text-sm [overflow-wrap:anywhere]">From {giver}</p>
			) : null}
			{reward || quest.rewardFavor > 0 ? (
				<p className="inline-flex flex-wrap items-center gap-x-2 text-sm">
					<span className="text-muted-foreground">Reward</span>
					{reward ? <span>{reward}</span> : null}
					{quest.rewardFavor > 0 ? (
						<span className="inline-flex items-center gap-1">
							<CrownIcon aria-hidden="true" className="size-3.5 text-gilt-ink" />+
							{formatNumber(quest.rewardFavor)} favor
						</span>
					) : null}
				</p>
			) : null}
			<p className="flex flex-wrap gap-1.5">
				<HiddenBadge item={quest} />
				<Badge variant="outline">{questKindLabels[quest.kind]}</Badge>
				{quest.resolvedDay === null ? null : (
					<Badge variant="muted">{dayName(quest.resolvedDay)}</Badge>
				)}
			</p>
		</li>
	);
}
