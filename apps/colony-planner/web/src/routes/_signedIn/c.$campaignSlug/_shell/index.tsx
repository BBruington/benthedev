import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
	AnchorIcon,
	ArrowRightIcon,
	CastleIcon,
	CheckIcon,
	type LucideIcon,
	MessagesSquareIcon,
	PenLineIcon,
	SparklesIcon,
	UserCheckIcon,
} from "lucide-react";
import { type ReactNode, useState } from "react";
import type { Schemas } from "@/api/client";
import { problemMessage } from "@/api/problem";
import {
	colonyBoardQuery,
	latestRecapQuery,
	mapsQuery,
	marketQuery,
	membersQuery,
	nextSessionQuery,
	recentTimelineQuery,
	settingsQuery,
} from "@/api/queries";
import { AdjustSheet } from "@/colony/adjust-sheet";
import { AdvanceSheet } from "@/colony/advance-sheet";
import { ColonistsStrip, DmTools, RecentEntries, ResourceTiles } from "@/colony/board-sections";
import { CorrectDaySheet } from "@/colony/correct-day-sheet";
import { ShipmentsSheet } from "@/colony/shipments-sheet";
import { UndoDialog } from "@/colony/undo-dialog";
import { type ColonyChanges, useColonyChanges } from "@/colony/use-colony-changes";
import { DayDial } from "@/components/day-dial";
import { MemberAvatar } from "@/components/member-avatar";
import { PageHeader } from "@/components/page-header";
import { CharacterNameSheet } from "@/components/shell/character-name-sheet";
import { StatusPanel } from "@/components/status-panel";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { fallbackCampaignName, useCampaign, useCampaignId } from "@/hooks/use-campaign";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { useMe } from "@/hooks/use-me";
import { useSheetState } from "@/hooks/use-sheet-state";
import { cachedMe, canUseDmTools, type Me, tableName } from "@/lib/access";
import { dayName, shipmentLine, voyageProgress } from "@/lib/colony";
import { focusFirstVisible } from "@/lib/focus";
import { goodsShipLine, goodsShipProgress } from "@/lib/market";
import { cn } from "@/lib/utils";
import { countOf } from "@/lib/words";
import { artSource } from "@/maps/art";
import { BoardSessions } from "@/sessions/board-sessions";
import { MyRequestsCard, PendingRequestsCard } from "@/ventures/board-ventures";

type ColonyBoard = Schemas["ColonyBoard"];
type MarketOrder = Schemas["MarketOrder"];

export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell/")({
	component: ColonyPage,
	loader: ({ context: { campaignId, queryClient } }) => {
		if (!campaignId) {
			return;
		}
		// Prefetching never throws; the page shows its own loading and error states.
		queryClient.prefetchQuery(colonyBoardQuery(campaignId));
		queryClient.prefetchQuery(recentTimelineQuery(campaignId));
		queryClient.prefetchQuery(mapsQuery(campaignId));
		queryClient.prefetchQuery(marketQuery(campaignId));
		queryClient.prefetchQuery(nextSessionQuery(campaignId));
		queryClient.prefetchQuery(latestRecapQuery(campaignId));
		const me = cachedMe(queryClient, campaignId);
		if (me && canUseDmTools(me)) {
			queryClient.prefetchQuery(membersQuery(campaignId));
			queryClient.prefetchQuery(settingsQuery(campaignId));
		}
	},
});

const boardHeadingId = "colony-board-heading";

/**
 * The colony board (spec §6.2): the clock, the stockpile, the colonists, the latest entries, and the
 * next game night with the last recap.
 */
function ColonyPage() {
	const me = useMe();
	const campaignId = useCampaignId();
	const board = useQuery(colonyBoardQuery(campaignId));
	// Open orders come soonest first, so the board follows the next ship to dock.
	const order = useQuery(marketQuery(campaignId)).data?.orders[0] ?? null;
	const recent = useQuery(recentTimelineQuery(campaignId));
	// The viewer's own map list: players never get a hidden Mainland, so its art can't show here (C6).
	const mainland = useQuery(mapsQuery(campaignId)).data?.maps.find(
		(worldMap) => worldMap.isMainland,
	);
	useDocumentTitle("Colony", board.data?.colonyName ?? fallbackCampaignName);

	if (board.isPending) {
		return <BoardSkeleton />;
	}
	if (board.isError) {
		return (
			<StatusPanel
				description={problemMessage(board.error, "The colony board couldn't be loaded.")}
				icon={CastleIcon}
				title="The board couldn't be loaded"
				tone="ember"
			/>
		);
	}
	const data = board.data;
	return (
		<div className="flex flex-col gap-6 md:gap-8">
			<PageHeader
				description={`Welcome back, ${tableName(me.member)}.`}
				eyebrow={data.campaignName}
				title={data.colonyName}
			/>
			{canUseDmTools(me) && recent.data?.entries.length === 0 ? <FirstSteps /> : null}
			<ClockCard
				backdropSource={mainland ? artSource(campaignId, mainland) : null}
				board={data}
				me={me}
				order={order}
			/>
			<section aria-labelledby="stockpile-heading" className="flex flex-col gap-3">
				<h2 className="sr-only" id="stockpile-heading">
					Stockpile
				</h2>
				<ResourceTiles resources={data.resources} />
			</section>
			<ColonistsStrip board={data} />
			{canUseDmTools(me) ? (
				<PendingRequestsCard resources={data.resources} />
			) : (
				<MyRequestsCard resources={data.resources} />
			)}
			<RecentEntries
				entries={recent.data?.entries}
				isError={recent.isError}
				isPending={recent.isPending}
			/>
			<BoardSessions isDm={canUseDmTools(me)} />
			{canUseDmTools(me) ? <TableChecklist /> : <YourPlace me={me} />}
		</div>
	);
}

function BoardSkeleton() {
	return (
		<div aria-busy="true" className="flex flex-col gap-6">
			<Skeleton className="h-16 w-2/3" />
			<Skeleton className="h-72 rounded-2xl" />
			<div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
				<Skeleton className="h-28 rounded-xl" />
				<Skeleton className="h-28 rounded-xl" />
				<Skeleton className="h-28 rounded-xl" />
				<Skeleton className="h-28 rounded-xl" />
			</div>
		</div>
	);
}

/**
 * "Day 44 · next ship Day 47, in 3 days" (spec §6.2). The party's soonest goods ship takes the line
 * while an order is out, since that's the ship the party waits on.
 */
function clockLine(board: ColonyBoard, order: MarketOrder | null): string {
	if (order) {
		return goodsShipLine(order, board.day);
	}
	const shipments = shipmentLine(board.shipments);
	return `${dayName(board.day)} · ${shipments.charAt(0).toLowerCase()}${shipments.slice(1)}`;
}

/** The day dial with the shipment countdown, and the DM's tools beside it, over the Mainland's art. */
function ClockCard({
	backdropSource,
	board,
	order,
	me,
}: {
	backdropSource: string | null;
	board: ColonyBoard;
	order: MarketOrder | null;
	me: Me;
}) {
	const showDmTools = canUseDmTools(me);
	return (
		<section
			aria-labelledby={boardHeadingId}
			className="relative isolate overflow-hidden rounded-2xl border border-border bg-card shadow-card"
		>
			{backdropSource ? (
				<img
					alt=""
					className="pointer-events-none absolute inset-y-0 right-0 -z-10 hidden h-full w-3/5 object-cover object-center opacity-[0.12] [mask-image:linear-gradient(to_left,black,transparent)] lg:block"
					src={backdropSource}
				/>
			) : null}
			<div
				className={cn(
					"grid items-center gap-6 p-5 sm:p-8",
					showDmTools && "lg:grid-cols-[auto_1fr] lg:gap-10",
				)}
			>
				<div className="flex flex-col items-center gap-2">
					<DayDial
						day={board.day}
						shipmentProgress={
							order ? goodsShipProgress(order, board.day) : voyageProgress(board.shipments)
						}
					/>
					<h2
						className="text-balance text-center font-display font-semibold text-gilt-ink text-lg tracking-wide outline-none"
						id={boardHeadingId}
						tabIndex={-1}
					>
						{clockLine(board, order)}
					</h2>
				</div>
				{showDmTools ? <BoardTools board={board} /> : null}
			</div>
		</section>
	);
}

/** The DM's tools and the sheets they open; only mounted for the DM outside View as player. */
function BoardTools({ board }: { board: ColonyBoard }) {
	const changes = useColonyChanges();
	const advance = useSheetState<{ days: number | null }>();
	const adjust = useSheetState<true>();
	const shipments = useSheetState<true>();
	const correctDay = useSheetState<true>();
	const [isUndoOpen, setUndoOpen] = useState(false);
	const lastChange = board.dm?.lastChange ?? null;

	return (
		<>
			<DmTools
				board={board}
				isUndoing={changes.undo.isPending}
				onAdjust={() => {
					adjust.show(true);
				}}
				onAdvance={(days) => {
					advance.show({ days });
				}}
				onCorrectDay={() => {
					correctDay.show(true);
				}}
				onShipments={() => {
					shipments.show(true);
				}}
				onUndo={() => {
					setUndoOpen(true);
				}}
			/>
			{advance.value ? (
				<AdvanceSheet
					changes={changes}
					initialDays={advance.value.days}
					key={advance.session}
					onOpenChange={advance.onOpenChange}
					open={advance.open}
				/>
			) : null}
			{adjust.value ? (
				<AdjustSheet
					cargo={null}
					changes={changes}
					key={adjust.session}
					onOpenChange={adjust.onOpenChange}
					open={adjust.open}
					resources={board.resources}
					version={board.version}
				/>
			) : null}
			{shipments.value ? (
				<ShipmentsSheet
					board={board}
					changes={changes}
					key={shipments.session}
					onOpenChange={shipments.onOpenChange}
					open={shipments.open}
				/>
			) : null}
			{correctDay.value ? (
				<CorrectDaySheet
					board={board}
					changes={changes}
					key={correctDay.session}
					onOpenChange={correctDay.onOpenChange}
					open={correctDay.open}
				/>
			) : null}
			<UndoDialog
				lastChange={lastChange}
				onConfirm={(confirmed) => {
					undoFromBoard(changes, confirmed.groupId);
					setUndoOpen(false);
				}}
				onOpenChange={setUndoOpen}
				open={isUndoOpen}
			/>
		</>
	);
}

/** Undo removes its own button, so focus settles on the day heading (or a fresh Undo after a 409). */
function undoFromBoard(changes: ColonyChanges, groupId: string) {
	changes.undo.mutate(
		{ body: { groupId }, ...changes.inCampaign },
		{
			onError: () => {
				focusFirstVisible("undo-last-change", boardHeadingId);
			},
			onSuccess: () => {
				focusFirstVisible(boardHeadingId);
			},
		},
	);
}

/** A new campaign's DM, before anything has happened (docs/design/multi-campaign.md §5.2). */
function FirstSteps() {
	const { slug } = useCampaign();
	return (
		<section
			aria-labelledby="first-steps-heading"
			className="flex flex-col gap-4 rounded-2xl border border-gilt/45 bg-gilt/8 p-5 sm:flex-row sm:items-center sm:p-6"
		>
			<span
				aria-hidden="true"
				className="inline-flex size-11 shrink-0 items-center justify-center rounded-full border border-gilt/40 bg-card text-gilt-ink"
			>
				<SparklesIcon className="size-5" />
			</span>
			<div className="min-w-0 flex-1">
				<h2 className="font-display font-semibold text-lg tracking-wide" id="first-steps-heading">
					A fresh colony
				</h2>
				<p className="text-muted-foreground">
					Start in Settings: name your colony, pick your time zone, share your invite link.
				</p>
			</div>
			<Button asChild className="shrink-0">
				<Link params={{ campaignSlug: slug }} to="/c/$campaignSlug/settings">
					Open Settings
					<ArrowRightIcon aria-hidden="true" />
				</Link>
			</Button>
		</section>
	);
}

function TableChecklist() {
	const campaignId = useCampaignId();
	const members = useQuery(membersQuery(campaignId));
	const settings = useQuery(settingsQuery(campaignId));
	const waitingCount = members.data?.pending.length ?? 0;
	const playerCount = members.data?.active.filter((member) => !member.dm?.isDm).length ?? 0;
	const channelCount = [
		settings.data?.discordAnnounceChannelId,
		settings.data?.discordReminderChannelId,
		settings.data?.discordDmChannelId,
	].filter(Boolean).length;

	const isAllDone =
		members.isSuccess && waitingCount === 0 && playerCount > 0 && channelCount === 3;
	if (isAllDone) {
		return null;
	}
	return (
		<section aria-labelledby="checklist-heading" className="flex flex-col gap-4">
			<div>
				<h2 className="font-display font-semibold text-xl tracking-wide" id="checklist-heading">
					Ready the table
				</h2>
				<p className="text-muted-foreground">
					What's left to set up. This goes away once it's done.
				</p>
			</div>
			<div className="grid gap-4 lg:grid-cols-3">
				<ChecklistCard
					action={waitingCount > 0 ? "Review requests" : "Open Players"}
					icon={UserCheckIcon}
					isDone={members.isSuccess && waitingCount === 0 && playerCount > 0}
					isHighlighted={waitingCount > 0}
					status={
						<QueryStatus query={members}>
							{waitingCount > 0 ? (
								<span className="font-medium text-gilt-ink">
									{waitingCount} waiting to join · {playerCount} playing
								</span>
							) : (
								`${countOf(playerCount, "player")} let in`
							)}
						</QueryStatus>
					}
					title="Let the players in"
					to="/c/$campaignSlug/players"
				/>
				<ChecklistCard
					action="Open Settings"
					icon={AnchorIcon}
					isDone={settings.isSuccess}
					status={
						<QueryStatus query={settings}>
							{settings.data ? `${settings.data.campaignName} · ${settings.data.timezone}` : null}
						</QueryStatus>
					}
					title="Name the campaign and its time zone"
					to="/c/$campaignSlug/settings"
				/>
				<ChecklistCard
					action={settings.data?.discordServer ? "Set channels" : "Link a server"}
					hash="discord"
					icon={MessagesSquareIcon}
					isDone={channelCount === 3}
					status={
						<QueryStatus query={settings}>
							{settings.data?.discordServer
								? `${channelCount} of 3 Discord channels set`
								: "No Discord server linked yet"}
						</QueryStatus>
					}
					title="Point Beny Bot at your channels"
					to="/c/$campaignSlug/settings"
				/>
			</div>
		</section>
	);
}

function QueryStatus({
	query,
	children,
}: {
	query: { isPending: boolean; isError: boolean };
	children: ReactNode;
}) {
	if (query.isPending) {
		return <Skeleton className="h-4 w-40" />;
	}
	if (query.isError) {
		return <span className="text-muted-foreground">Not available yet</span>;
	}
	return <>{children}</>;
}

function ChecklistCard({
	icon: Icon,
	title,
	status,
	action,
	to,
	hash,
	isDone,
	isHighlighted = false,
}: {
	icon: LucideIcon;
	title: string;
	status: ReactNode;
	action: string;
	to: "/c/$campaignSlug/players" | "/c/$campaignSlug/settings";
	hash?: string;
	isDone: boolean;
	isHighlighted?: boolean;
}) {
	const { slug } = useCampaign();
	return (
		<Link
			className={cn(
				"group flex flex-col gap-4 rounded-xl border bg-card p-5 shadow-card outline-none transition-[border-color,transform] hover:-translate-y-0.5 hover:border-gilt/45 focus-visible:ring-[3px] focus-visible:ring-ring/45",
				isHighlighted ? "border-gilt/55" : "border-border",
			)}
			hash={hash}
			params={{ campaignSlug: slug }}
			to={to}
		>
			<div className="flex items-center justify-between">
				<span
					aria-hidden="true"
					className="inline-flex size-10 items-center justify-center rounded-lg bg-gilt/12 text-gilt-ink"
				>
					<Icon className="size-5" />
				</span>
				{isDone ? (
					<span className="inline-flex items-center gap-1 text-moss-ink text-sm">
						<CheckIcon aria-hidden="true" className="size-4" />
						Done
					</span>
				) : null}
			</div>
			<div className="min-w-0 flex-1">
				<h3 className="font-semibold text-lg leading-snug">{title}</h3>
				<div className="mt-1 truncate text-muted-foreground text-sm">{status}</div>
			</div>
			<span className="inline-flex items-center gap-1.5 font-medium text-gilt-ink text-sm">
				{action}
				<ArrowRightIcon
					aria-hidden="true"
					className="size-4 transition-transform group-hover:translate-x-0.5"
				/>
			</span>
		</Link>
	);
}

function YourPlace({ me }: { me: Me }) {
	const [isSheetOpen, setSheetOpen] = useState(false);
	const characterName = me.member.characterName;
	return (
		<section
			aria-labelledby="your-place-heading"
			className="flex flex-col gap-5 rounded-2xl border border-border bg-card p-6 shadow-card sm:flex-row sm:items-center"
		>
			<MemberAvatar avatarUrl={me.member.avatarUrl} name={tableName(me.member)} size="lg" />
			<div className="min-w-0 flex-1">
				<p className="font-medium text-muted-foreground text-xs uppercase tracking-[0.18em]">
					Your place at the table
				</p>
				<h2
					className="mt-1 truncate font-display font-semibold text-2xl text-gilt-ink tracking-wide"
					id="your-place-heading"
				>
					{characterName || me.member.displayName}
				</h2>
				<p className="text-muted-foreground">
					{characterName
						? `Played by ${me.member.displayName}`
						: "Add your character's name so the table knows who you're playing."}
				</p>
			</div>
			<Button
				onClick={() => {
					setSheetOpen(true);
				}}
				variant="outline"
			>
				<PenLineIcon />
				{characterName ? "Change name" : "Add character name"}
			</Button>
			<CharacterNameSheet me={me} onOpenChange={setSheetOpen} open={isSheetOpen} />
		</section>
	);
}
