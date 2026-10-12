import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
	ArchiveIcon,
	ArchiveRestoreIcon,
	ArrowLeftIcon,
	CrownIcon,
	HandCoinsIcon,
	PenLineIcon,
	PlusIcon,
	ScaleIcon,
	SendIcon,
} from "lucide-react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { problemMessage, problemStatus } from "@/api/problem";
import {
	colonyBoardQuery,
	membersQuery,
	patronActionsQuery,
	patronQuery,
	patronQueryKeyPrefix,
	patronsQuery,
	recentTimelineQuery,
	timelineQueryKeyPrefix,
} from "@/api/queries";
import { useColonyChanges } from "@/colony/use-colony-changes";
import { SwitchRow } from "@/components/form-controls";
import { StatusPanel } from "@/components/status-panel";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
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
import { cn } from "@/lib/utils";
import { spendShortfall } from "@/lib/ventures";
import { ProseBlock } from "@/npcs/npc-profile";
import { HiddenBadge } from "@/people/people-sections";
import { patronActionsHeadingId, patronHeadingId } from "@/ventures/ids";
import {
	ActionSheet,
	FavorSheet,
	PatronSheet,
	RequestSheet,
	UseActionSheet,
} from "@/ventures/patron-sheets";
import { ArchivedBadge, costLine, FavorCount, RequestCard } from "@/ventures/venture-parts";

type Patron = Schemas["Patron"];
type PatronAction = Schemas["PatronAction"];
type PatronPageData = Schemas["PatronPage"];
type ColonyResource = Schemas["ColonyResource"];

export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell/patrons/$patronId")({
	component: PatronPage,
	loader: ({ context: { campaignId, queryClient }, params: { patronId } }) => {
		if (!campaignId) {
			return;
		}
		queryClient.prefetchQuery(patronQuery(campaignId, patronId));
		queryClient.prefetchQuery(colonyBoardQuery(campaignId));
		const me = cachedMe(queryClient, campaignId);
		if (me && canUseDmTools(me)) {
			queryClient.prefetchQuery(membersQuery(campaignId));
		}
	},
});

function BackToPatrons() {
	const { slug } = useCampaign();
	return (
		<Button asChild className="self-start" size="sm" variant="ghost">
			<Link params={{ campaignSlug: slug }} to="/c/$campaignSlug/patrons">
				<ArrowLeftIcon />
				All patrons
			</Link>
		</Button>
	);
}

/** /patrons/{patronId} (spec §6.2, §5.7): favor, actions and request history. */
function PatronPage() {
	const { patronId } = Route.useParams();
	const campaignId = useCampaignId();
	const me = useMe();
	const isDm = canUseDmTools(me);
	const page = useQuery(patronQuery(campaignId, patronId));
	useDocumentTitle(page.data?.patron.name ?? "Patron", useCampaignName() ?? fallbackCampaignName);

	if (page.isPending) {
		return (
			<div aria-busy="true" className="flex flex-col gap-6">
				<Skeleton className="h-16 w-2/3" />
				<Skeleton className="h-64 rounded-2xl" />
			</div>
		);
	}
	if (page.isError) {
		const isMissing = problemStatus(page.error) === 404;
		return (
			<div className="flex flex-col gap-4">
				<BackToPatrons />
				<StatusPanel
					description={
						isMissing
							? "There's no patron at that link."
							: problemMessage(page.error, "This patron couldn't be loaded.")
					}
					icon={CrownIcon}
					title={isMissing ? "No such patron" : "This patron couldn't be loaded"}
					tone="ember"
				/>
			</div>
		);
	}
	return <PatronDetail isDm={isDm} page={page.data} />;
}

function usePatronSettled() {
	const queryClient = useQueryClient();
	const campaignId = useCampaignId();
	return () =>
		Promise.all([
			queryClient.invalidateQueries({ queryKey: patronsQuery(campaignId).queryKey }),
			queryClient.invalidateQueries({ queryKey: patronQueryKeyPrefix(campaignId) }),
			queryClient.invalidateQueries({ queryKey: patronActionsQuery(campaignId).queryKey }),
			queryClient.invalidateQueries({ queryKey: timelineQueryKeyPrefix(campaignId) }),
			queryClient.invalidateQueries({ queryKey: recentTimelineQuery(campaignId).queryKey }),
		]);
}

function PatronDetail({ page, isDm }: { page: PatronPageData; isDm: boolean }) {
	const { patron } = page;
	const campaignId = useCampaignId();
	const board = useQuery(colonyBoardQuery(campaignId));
	const changes = useColonyChanges();
	const editing = useSheetState<true>();
	const adjusting = useSheetState<true>();
	const actionSheet = useSheetState<PatronAction | null>();
	const using = useSheetState<PatronAction>();
	const requesting = useSheetState<PatronAction>();
	const resources = board.data?.resources ?? [];
	const actions = isDm ? page.actions : page.actions.filter((action) => !action.archived);
	// The DM viewing as a player sees the Request button but can't send one as themselves.
	const canRequest = !useMe().isDm;

	return (
		<div className="flex flex-col gap-6">
			<BackToPatrons />
			<header className="flex flex-wrap items-start gap-4">
				<div className="min-w-0 flex-1 space-y-2">
					<h1
						className="font-display font-semibold text-3xl tracking-wide outline-none [overflow-wrap:anywhere] md:text-4xl"
						id={patronHeadingId}
						tabIndex={-1}
					>
						{patron.name}
					</h1>
					<FavorCount className="text-xl" favor={patron.favor} />
					<p className="flex flex-wrap gap-1.5">
						<HiddenBadge item={patron} />
						<ArchivedBadge archived={patron.archived} />
					</p>
				</div>
				{isDm ? (
					<div className="flex w-full flex-wrap gap-2 sm:w-auto">
						<Button
							disabled={!board.data}
							onClick={() => {
								adjusting.show(true);
							}}
						>
							<ScaleIcon />
							Adjust favor
						</Button>
						<Button
							onClick={() => {
								editing.show(true);
							}}
							variant="outline"
						>
							<PenLineIcon />
							Edit
						</Button>
					</div>
				) : null}
			</header>
			{isDm && patron.dm ? <PatronSwitches patron={patron} revealed={patron.dm.revealed} /> : null}
			{patron.description || patron.playerNotes || patron.dm?.dmNotes ? (
				<section className="grid gap-6 rounded-2xl border border-border bg-card p-5 shadow-card md:p-7">
					<ProseBlock text={patron.description} title="About" />
					<ProseBlock text={patron.playerNotes} title="Player notes" />
					{patron.dm ? <ProseBlock dmOnly text={patron.dm.dmNotes} title="DM notes" /> : null}
				</section>
			) : null}

			<section aria-labelledby={patronActionsHeadingId} className="flex flex-col gap-3">
				<div className="flex flex-wrap items-center justify-between gap-3">
					<h2
						className="font-display font-semibold text-2xl tracking-wide outline-none"
						id={patronActionsHeadingId}
						tabIndex={-1}
					>
						What they offer
					</h2>
					{isDm ? (
						<Button
							onClick={() => {
								actionSheet.show(null);
							}}
							variant="outline"
						>
							<PlusIcon />
							New action
						</Button>
					) : null}
				</div>
				{actions.length === 0 ? (
					<p className="rounded-xl border border-border border-dashed px-4 py-4 text-muted-foreground">
						{isDm
							? "No actions yet. Define what they can be asked for."
							: "Nothing they offer is known yet."}
					</p>
				) : (
					<ul className="grid gap-3 md:grid-cols-2">
						{actions.map((action) => (
							<ActionCard
								action={action}
								canRequest={canRequest}
								isDm={isDm}
								key={action.id}
								onEdit={() => {
									actionSheet.show(action);
								}}
								onRequest={() => {
									requesting.show(action);
								}}
								onUse={() => {
									using.show(action);
								}}
								patron={patron}
								resources={resources}
							/>
						))}
					</ul>
				)}
			</section>

			<section aria-labelledby="patron-requests-heading" className="flex flex-col gap-3">
				<h2
					className="font-display font-semibold text-2xl tracking-wide"
					id="patron-requests-heading"
				>
					Requests
				</h2>
				{page.requests.length === 0 ? (
					<p className="text-muted-foreground">Nobody has asked {patron.name} for anything yet.</p>
				) : (
					<ul className="grid gap-2">
						{page.requests.map((request) => (
							<RequestCard key={request.id} request={request} resources={resources} />
						))}
					</ul>
				)}
			</section>

			{isDm && editing.value ? (
				<PatronSheet
					key={editing.session}
					onOpenChange={editing.onOpenChange}
					open={editing.open}
					patron={patron}
				/>
			) : null}
			{isDm && adjusting.value && board.data ? (
				<FavorSheet
					boardVersion={board.data.version}
					changes={changes}
					key={adjusting.session}
					onOpenChange={adjusting.onOpenChange}
					open={adjusting.open}
					patron={patron}
				/>
			) : null}
			{isDm && actionSheet.session > 0 ? (
				<ActionSheet
					action={actionSheet.value}
					key={actionSheet.session}
					onOpenChange={actionSheet.onOpenChange}
					open={actionSheet.open}
					patron={patron}
				/>
			) : null}
			{isDm && using.value && board.data ? (
				<UseActionSheet
					action={using.value}
					boardVersion={board.data.version}
					changes={changes}
					key={using.session}
					onOpenChange={using.onOpenChange}
					open={using.open}
					patron={patron}
				/>
			) : null}
			{!isDm && requesting.value ? (
				<RequestSheet
					action={requesting.value}
					key={requesting.session}
					onOpenChange={requesting.onOpenChange}
					open={requesting.open}
					patron={patron}
				/>
			) : null}
		</div>
	);
}

/** The patron's Visible and Archived switches: the only ways to change either (spec §5.7, §5.9). */
function PatronSwitches({ patron, revealed }: { patron: Patron; revealed: boolean }) {
	const campaignId = useCampaignId();
	const settle = usePatronSettled();
	const path = { params: { path: { campaignId, patronId: patron.id } } };
	const reveal = api.useMutation("put", "/api/campaigns/{campaignId}/patrons/{patronId}/revealed", {
		onError: (error) => {
			toast.error(problemMessage(error, "Their visibility couldn't be changed."));
		},
		onSettled: settle,
		onSuccess: (saved) => {
			toast.success(
				saved.patron.dm?.revealed
					? `Players can now see ${saved.patron.name}.`
					: `${saved.patron.name} is hidden from players.`,
			);
		},
	});
	const archive = api.useMutation(
		"put",
		"/api/campaigns/{campaignId}/patrons/{patronId}/archived",
		{
			onError: (error) => {
				toast.error(problemMessage(error, "They couldn't be archived."));
			},
			onSettled: settle,
			onSuccess: (saved) => {
				toast.success(
					saved.patron.archived
						? `${saved.patron.name} is archived.`
						: `${saved.patron.name} is back.`,
				);
			},
		},
	);
	const shownRevealed = reveal.isPending
		? (reveal.variables?.body?.revealed ?? revealed)
		: revealed;
	const shownArchived = archive.isPending
		? (archive.variables?.body?.archived ?? patron.archived)
		: patron.archived;
	return (
		<div className="grid gap-2 sm:grid-cols-2">
			<SwitchRow
				checked={shownRevealed}
				description={
					shownRevealed
						? "Players see them, their favor and their visible actions."
						: "Hidden: players can't see them, their actions or entries about them."
				}
				disabled={reveal.isPending}
				id="patron-visible"
				label="Visible to players"
				onCheckedChange={(checked) => {
					reveal.mutate({ body: { revealed: checked }, ...path });
				}}
			/>
			<SwitchRow
				checked={shownArchived}
				description={
					shownArchived
						? "Archived: gone from pickers and players' lists; history stays."
						: "Active: in pickers and, while visible, players' lists."
				}
				disabled={archive.isPending}
				id="patron-archived"
				label="Archived"
				onCheckedChange={(checked) => {
					archive.mutate({ body: { archived: checked }, ...path });
				}}
			/>
		</div>
	);
}

function ActionCard({
	action,
	patron,
	resources,
	isDm,
	canRequest,
	onEdit,
	onUse,
	onRequest,
}: {
	action: PatronAction;
	patron: Patron;
	resources: readonly ColonyResource[];
	isDm: boolean;
	canRequest: boolean;
	onEdit: () => void;
	onUse: () => void;
	onRequest: () => void;
}) {
	const shortfall = spendShortfall(patron, action.favorCost, action.effects, resources);
	const isUnavailable = action.archived || patron.archived;
	return (
		<li
			className={cn(
				"flex flex-col gap-3 rounded-xl border border-border bg-card p-4 shadow-card",
				action.archived && "opacity-75",
			)}
		>
			<div className="space-y-1">
				<h3 className="font-semibold text-lg leading-snug [overflow-wrap:anywhere]">
					{action.name}
				</h3>
				<p className="inline-flex items-center gap-2 tabular-nums">
					<CrownIcon aria-hidden="true" className="size-4 text-gilt-ink" />
					{costLine(action.favorCost, action.effects, resources)}
				</p>
				{action.description ? (
					<p className="text-muted-foreground [overflow-wrap:anywhere]">{action.description}</p>
				) : null}
				<p className="flex flex-wrap gap-1.5">
					<HiddenBadge item={action} />
					<ArchivedBadge archived={action.archived} />
				</p>
			</div>
			{isDm ? (
				<DmActionControls
					action={action}
					isUnavailable={isUnavailable}
					onEdit={onEdit}
					onUse={onUse}
				/>
			) : (
				<div className="grid gap-1.5">
					<Button disabled={shortfall !== null || !canRequest} onClick={onRequest}>
						<SendIcon />
						Request
					</Button>
					{shortfall ? <p className="text-ember-ink text-sm">{shortfall}</p> : null}
					{canRequest ? null : (
						<p className="text-muted-foreground text-sm">Players send requests from here.</p>
					)}
				</div>
			)}
		</li>
	);
}

function DmActionControls({
	action,
	isUnavailable,
	onEdit,
	onUse,
}: {
	action: PatronAction;
	isUnavailable: boolean;
	onEdit: () => void;
	onUse: () => void;
}) {
	const campaignId = useCampaignId();
	const settle = usePatronSettled();
	const path = { params: { path: { actionId: action.id, campaignId } } };
	const reveal = api.useMutation(
		"put",
		"/api/campaigns/{campaignId}/patron-actions/{actionId}/revealed",
		{
			onError: (error) => {
				toast.error(problemMessage(error, "Its visibility couldn't be changed."));
			},
			onSettled: settle,
		},
	);
	const archive = api.useMutation(
		"put",
		"/api/campaigns/{campaignId}/patron-actions/{actionId}/archived",
		{
			onError: (error) => {
				toast.error(problemMessage(error, "It couldn't be archived."));
			},
			onSettled: settle,
			onSuccess: (saved) => {
				toast.success(saved.archived ? `${saved.name} is archived.` : `${saved.name} is back.`);
				document.getElementById(patronActionsHeadingId)?.focus();
			},
		},
	);
	const revealed = action.dm?.revealed ?? true;
	const shownRevealed = reveal.isPending
		? (reveal.variables?.body?.revealed ?? revealed)
		: revealed;
	const switchId = `action-visible-${action.id}`;
	return (
		<div className="grid gap-2">
			<div className="flex min-h-11 items-center justify-between gap-3 rounded-lg border border-border px-3">
				<Label htmlFor={switchId}>Visible to players</Label>
				<Switch
					checked={shownRevealed}
					disabled={reveal.isPending}
					id={switchId}
					onCheckedChange={(checked) => {
						reveal.mutate({ body: { revealed: checked }, ...path });
					}}
				/>
			</div>
			<div className="flex flex-wrap gap-2">
				<Button disabled={isUnavailable} onClick={onUse}>
					<HandCoinsIcon />
					Use action now
				</Button>
				<Button onClick={onEdit} variant="outline">
					<PenLineIcon />
					Edit
				</Button>
				<Button
					disabled={archive.isPending}
					onClick={() => {
						archive.mutate({ body: { archived: !action.archived }, ...path });
					}}
					variant="ghost"
				>
					{action.archived ? <ArchiveRestoreIcon /> : <ArchiveIcon />}
					{action.archived ? "Unarchive" : "Archive"}
				</Button>
			</div>
		</div>
	);
}
