import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import {
	ArrowLeftIcon,
	BadgeCheckIcon,
	CircleXIcon,
	CrownIcon,
	EyeOffIcon,
	PenLineIcon,
	RotateCcwIcon,
	SwordsIcon,
	Trash2Icon,
} from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { problemMessage, problemStatus } from "@/api/problem";
import {
	colonyBoardQuery,
	npcsQuery,
	patronsQuery,
	questQuery,
	questQueryKeyPrefix,
	questsQueryKeyPrefix,
	recentTimelineQuery,
	timelineQueryKeyPrefix,
} from "@/api/queries";
import { useColonyChanges } from "@/colony/use-colony-changes";
import { SwitchRow } from "@/components/form-controls";
import { StatusPanel } from "@/components/status-panel";
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
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
import { cachedMe, canUseDmTools } from "@/lib/access";
import { dayName, formatDelta, formatNumber } from "@/lib/colony";
import { focusFirstVisible } from "@/lib/focus";
import { isEmptyEffects, lootItemKeys, questKindLabels, questStatusLabels } from "@/lib/ventures";
import { ProseBlock } from "@/npcs/npc-profile";
import { focusAfterClose } from "@/people/colonist-actions";
import { HiddenBadge } from "@/people/people-sections";
import { questHeadingId, questsPageHeadingId } from "@/ventures/ids";
import { QuestResolveSheet, type Resolution } from "@/ventures/quest-resolve-sheet";
import { QuestSheet } from "@/ventures/quest-sheet";
import { effectsText, PatronLink } from "@/ventures/venture-parts";

type Quest = Schemas["Quest"];

export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell/quests/$questId")({
	component: QuestPage,
	loader: ({ context: { campaignId, queryClient }, params: { questId } }) => {
		if (!campaignId) {
			return;
		}
		queryClient.prefetchQuery(questQuery(campaignId, questId));
		queryClient.prefetchQuery(colonyBoardQuery(campaignId));
		const me = cachedMe(queryClient, campaignId);
		if (me && canUseDmTools(me)) {
			queryClient.prefetchQuery(npcsQuery(campaignId));
			queryClient.prefetchQuery(patronsQuery(campaignId));
		}
	},
});

function BackToQuests() {
	const { slug } = useCampaign();
	return (
		<Button asChild className="self-start" size="sm" variant="ghost">
			<Link params={{ campaignSlug: slug }} to="/c/$campaignSlug/quests">
				<ArrowLeftIcon />
				All quests
			</Link>
		</Button>
	);
}

/** /quests/{questId} (spec §6.2, §5.6): one quest; the DM edits and resolves it in place. */
function QuestPage() {
	const { questId } = Route.useParams();
	const campaignId = useCampaignId();
	const me = useMe();
	const isDm = canUseDmTools(me);
	const quest = useQuery(questQuery(campaignId, questId));
	useDocumentTitle(quest.data?.title ?? "Quest", useCampaignName() ?? fallbackCampaignName);

	if (quest.isPending) {
		return (
			<div aria-busy="true" className="flex flex-col gap-6">
				<Skeleton className="h-16 w-2/3" />
				<Skeleton className="h-64 rounded-2xl" />
			</div>
		);
	}
	if (quest.isError) {
		const isMissing = problemStatus(quest.error) === 404;
		return (
			<div className="flex flex-col gap-4">
				<BackToQuests />
				<StatusPanel
					description={
						isMissing
							? "There's no quest at that link. It may have been removed."
							: problemMessage(quest.error, "This quest couldn't be loaded.")
					}
					icon={SwordsIcon}
					title={isMissing ? "No such quest" : "This quest couldn't be loaded"}
					tone="ember"
				/>
			</div>
		);
	}
	return <QuestDetail isDm={isDm} quest={quest.data} />;
}

function useQuestSettled() {
	const queryClient = useQueryClient();
	const campaignId = useCampaignId();
	return () =>
		Promise.all([
			queryClient.invalidateQueries({ queryKey: questsQueryKeyPrefix(campaignId) }),
			queryClient.invalidateQueries({ queryKey: questQueryKeyPrefix(campaignId) }),
			queryClient.invalidateQueries({ queryKey: timelineQueryKeyPrefix(campaignId) }),
			queryClient.invalidateQueries({ queryKey: recentTimelineQuery(campaignId).queryKey }),
		]);
}

function QuestDetail({ quest, isDm }: { quest: Quest; isDm: boolean }) {
	const campaignId = useCampaignId();
	const board = useQuery(colonyBoardQuery(campaignId));
	const resources = board.data?.resources ?? [];
	const editing = useSheetState<true>();
	const resolving = useSheetState<Resolution>();
	const changes = useColonyChanges();
	const [isReopening, setReopening] = useState(false);
	const statusVariant = { ACTIVE: "gilt", COMPLETED: "moss", FAILED: "ember" } as const;
	const lootKeys = lootItemKeys(quest.loot);

	return (
		<div className="flex flex-col gap-6">
			<BackToQuests />
			<header className="flex flex-wrap items-start gap-4">
				<div className="min-w-0 flex-1 space-y-2">
					<h1
						className="font-display font-semibold text-3xl tracking-wide outline-none [overflow-wrap:anywhere] md:text-4xl"
						id={questHeadingId}
						tabIndex={-1}
					>
						{quest.title}
					</h1>
					<p className="flex flex-wrap gap-1.5">
						<Badge variant={statusVariant[quest.status]}>{questStatusLabels[quest.status]}</Badge>
						<HiddenBadge item={quest} />
						<Badge variant="outline">{questKindLabels[quest.kind]}</Badge>
						{quest.resolvedDay === null ? null : (
							<Badge variant="muted">{dayName(quest.resolvedDay)}</Badge>
						)}
					</p>
				</div>
				{isDm ? (
					<div className="flex w-full flex-wrap gap-2 sm:w-auto">
						{quest.status === "ACTIVE" ? (
							<>
								<Button
									onClick={() => {
										resolving.show("complete");
									}}
								>
									<BadgeCheckIcon />
									Complete
								</Button>
								<Button
									onClick={() => {
										resolving.show("fail");
									}}
									variant="outline"
								>
									<CircleXIcon />
									Fail
								</Button>
							</>
						) : null}
						{quest.status === "FAILED" ? (
							<Button
								onClick={() => {
									setReopening(true);
								}}
								variant="outline"
							>
								<RotateCcwIcon />
								Reopen
							</Button>
						) : null}
						<Button
							onClick={() => {
								editing.show(true);
							}}
							variant="outline"
						>
							<PenLineIcon />
							Edit
						</Button>
						<DeleteQuestButton quest={quest} />
					</div>
				) : null}
			</header>
			{isDm && quest.dm ? <RevealSwitch quest={quest} revealed={quest.dm.revealed} /> : null}
			<section className="grid gap-6 rounded-2xl border border-border bg-card p-5 shadow-card md:p-7">
				<ProseBlock text={quest.description} title="The quest" />
				<Givers quest={quest} />
				<section className="grid gap-2">
					<h3 className="font-medium text-gilt-ink text-xs uppercase tracking-[0.18em]">Reward</h3>
					<RewardLines
						effects={effectsText(quest.rewardEffects, resources, "")}
						favor={quest.rewardFavor}
						patronName={quest.patron?.name ?? null}
					/>
					{quest.loot.length > 0 ? (
						<ul className="grid gap-1">
							{quest.loot.map((item, position) => (
								<li key={lootKeys[position]}>
									<span className="font-medium">
										{item.quantity > 1 ? `${formatNumber(item.quantity)} × ` : ""}
										{item.name}
									</span>
									{item.description ? (
										<span className="text-muted-foreground"> · {item.description}</span>
									) : null}
								</li>
							))}
						</ul>
					) : null}
				</section>
				{quest.dm ? <FailureCost quest={quest} resources={resources} /> : null}
				{quest.dm ? <ProseBlock dmOnly text={quest.dm.dmNotes} title="DM notes" /> : null}
			</section>
			{isDm && editing.value ? (
				<QuestSheet
					key={editing.session}
					onOpenChange={editing.onOpenChange}
					open={editing.open}
					quest={quest}
				/>
			) : null}
			{isDm && resolving.value ? (
				<QuestResolveSheet
					changes={changes}
					key={resolving.session}
					onOpenChange={resolving.onOpenChange}
					onResolved={() => {
						focusAfterClose(questHeadingId);
					}}
					open={resolving.open}
					quest={quest}
					resolution={resolving.value}
				/>
			) : null}
			{isDm ? (
				<AlertDialog onOpenChange={setReopening} open={isReopening}>
					<AlertDialogContent>
						<AlertDialogHeader>
							<AlertDialogTitle>Reopen {quest.title}?</AlertDialogTitle>
							<AlertDialogDescription>
								It goes back to Active. Nothing the failure took is given back; use Undo or an
								adjustment for that.
							</AlertDialogDescription>
						</AlertDialogHeader>
						<AlertDialogFooter>
							<AlertDialogCancel>Keep it failed</AlertDialogCancel>
							<AlertDialogAction
								onClick={() => {
									changes.reopenQuest.mutate(
										{ params: { path: { campaignId, questId: quest.id } } },
										{
											onError: (error) => {
												toast.error(problemMessage(error, "The quest couldn't be reopened."));
											},
											onSuccess: () => {
												focusFirstVisible(questHeadingId);
											},
										},
									);
								}}
							>
								Reopen
							</AlertDialogAction>
						</AlertDialogFooter>
					</AlertDialogContent>
				</AlertDialog>
			) : null}
		</div>
	);
}

function RewardLines({
	effects,
	favor,
	patronName,
}: {
	effects: string;
	favor: number;
	patronName: string | null;
}) {
	if (effects === "" && favor === 0) {
		return <p className="text-muted-foreground">No reward in the stockpile.</p>;
	}
	return (
		<p className="flex flex-wrap items-center gap-x-3 gap-y-1 text-lg">
			{effects ? <span>{effects}</span> : null}
			{favor === 0 ? null : (
				<span className="inline-flex items-center gap-1.5">
					<CrownIcon aria-hidden="true" className="size-4 text-gilt-ink" />
					{formatDelta(favor)} favor{patronName ? ` with ${patronName}` : ""}
				</span>
			)}
		</p>
	);
}

/** Who gave it; players receive a giver only while it is visible to them. */
function Givers({ quest }: { quest: Quest }) {
	const { slug } = useCampaign();
	if (!(quest.giverNpc || quest.patron)) {
		return null;
	}
	return (
		<section className="grid gap-1.5">
			<h3 className="font-medium text-gilt-ink text-xs uppercase tracking-[0.18em]">Given by</h3>
			<p className="flex flex-wrap items-center gap-x-4 gap-y-1 text-lg">
				{quest.giverNpc ? (
					<Link
						className="text-gilt-ink underline underline-offset-4"
						params={{ campaignSlug: slug, npcId: quest.giverNpc.id }}
						to="/c/$campaignSlug/npcs/$npcId"
					>
						{quest.giverNpc.displayName}
					</Link>
				) : null}
				{quest.patron ? <PatronLink patron={quest.patron} /> : null}
			</p>
		</section>
	);
}

/** The DM-only failure cost (Open Question 14): players never receive it. */
function FailureCost({
	quest,
	resources,
}: {
	quest: Quest;
	resources: readonly { key: string; label: string }[];
}) {
	if (!quest.dm) {
		return null;
	}
	const { failureEffects, failureFavor } = quest.dm;
	const isEmpty = isEmptyEffects(failureEffects) && failureFavor === 0;
	return (
		<section className="grid gap-1.5 rounded-xl border border-verdigris/40 bg-verdigris/5 p-4">
			<h3 className="flex flex-wrap items-center gap-2 font-medium text-gilt-ink text-xs uppercase tracking-[0.18em]">
				Failure cost
				<Badge className="normal-case tracking-normal" variant="verdigris">
					<EyeOffIcon />
					Only you see this until it fails
				</Badge>
			</h3>
			{isEmpty ? (
				<p className="text-muted-foreground">None: failing takes nothing.</p>
			) : (
				<RewardLines
					effects={effectsText(failureEffects, resources, "")}
					favor={-failureFavor}
					patronName={quest.patron?.name ?? null}
				/>
			)}
		</section>
	);
}

function RevealSwitch({ quest, revealed }: { quest: Quest; revealed: boolean }) {
	const campaignId = useCampaignId();
	const settle = useQuestSettled();
	const [postToTimeline, setPostToTimeline] = useState(true);
	const mutation = api.useMutation("put", "/api/campaigns/{campaignId}/quests/{questId}/revealed", {
		onError: (error) => {
			toast.error(problemMessage(error, "Its visibility couldn't be changed."));
		},
		onSettled: settle,
		onSuccess: (saved, variables) => {
			// A posted quest's live "New quest" toast (useTimelineToasts) already announces it.
			if (!saved.dm?.revealed) {
				toast.success(`${saved.title} is hidden from players.`);
			} else if (!variables.body?.postToTimeline) {
				toast.success(`Players can now see ${saved.title}.`);
			}
		},
	});
	const shownRevealed = mutation.isPending
		? (mutation.variables?.body?.revealed ?? revealed)
		: revealed;
	return (
		<div className="grid gap-2">
			<SwitchRow
				checked={shownRevealed}
				description={
					shownRevealed
						? "Players see it, without the failure cost or your notes."
						: "Hidden: players can't see it anywhere, and its entries leave their timeline."
				}
				disabled={mutation.isPending}
				id="quest-visible"
				label="Visible to players"
				onCheckedChange={(checked) => {
					mutation.mutate({
						body: { postToTimeline: checked && postToTimeline, revealed: checked },
						params: { path: { campaignId, questId: quest.id } },
					});
				}}
			/>
			{shownRevealed ? null : (
				<div className="flex min-h-11 items-center gap-3 rounded-lg border border-border px-3">
					<input
						checked={postToTimeline}
						className="size-5 accent-[var(--color-gilt)]"
						id="quest-reveal-post"
						onChange={(event) => {
							setPostToTimeline(event.target.checked);
						}}
						type="checkbox"
					/>
					<Label className="py-2" htmlFor="quest-reveal-post">
						Post "New quest" to the timeline when it's shown
					</Label>
				</div>
			)}
		</div>
	);
}

function DeleteQuestButton({ quest }: { quest: Quest }) {
	const campaignId = useCampaignId();
	const { slug } = useCampaign();
	const navigate = useNavigate();
	const settle = useQuestSettled();
	const [open, setOpen] = useState(false);
	const [confirmed, setConfirmed] = useState(false);
	const mutation = api.useMutation("delete", "/api/campaigns/{campaignId}/quests/{questId}", {
		onError: (error) => {
			toast.error(problemMessage(error, `${quest.title} couldn't be deleted.`));
			document.getElementById(questHeadingId)?.focus();
		},
		onSettled: settle,
		onSuccess: () => {
			toast.success(`${quest.title} was deleted.`);
			navigate({ params: { campaignSlug: slug }, to: "/c/$campaignSlug/quests" }).then(() => {
				focusFirstVisible(questsPageHeadingId);
			});
		},
	});
	return (
		<AlertDialog onOpenChange={setOpen} open={open}>
			<Button
				onClick={() => {
					setConfirmed(false);
					setOpen(true);
				}}
				variant="ghost-destructive"
			>
				<Trash2Icon />
				Delete
			</Button>
			<AlertDialogContent
				onCloseAutoFocus={(event) => {
					if (confirmed) {
						event.preventDefault();
					}
				}}
			>
				<AlertDialogHeader>
					<AlertDialogTitle>Delete {quest.title}?</AlertDialogTitle>
					<AlertDialogDescription>
						The quest is gone for good. Its timeline entries keep their text; nothing it paid or
						cost is reversed.
					</AlertDialogDescription>
				</AlertDialogHeader>
				<AlertDialogFooter>
					<AlertDialogCancel>Keep it</AlertDialogCancel>
					<AlertDialogAction
						onClick={() => {
							setConfirmed(true);
							mutation.mutate({ params: { path: { campaignId, questId: quest.id } } });
						}}
						variant="destructive"
					>
						Delete
					</AlertDialogAction>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	);
}
