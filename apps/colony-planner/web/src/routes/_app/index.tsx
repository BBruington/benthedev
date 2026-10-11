import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
	AnchorIcon,
	ArrowRightIcon,
	CheckIcon,
	type LucideIcon,
	MessagesSquareIcon,
	PenLineIcon,
	ScrollTextIcon,
	SunriseIcon,
	UserCheckIcon,
	UsersIcon,
	WheatIcon,
} from "lucide-react";
import { type ReactNode, useState } from "react";
import { membersQuery, settingsQuery } from "@/api/queries";
import { DayDial } from "@/components/day-dial";
import { MemberAvatar } from "@/components/member-avatar";
import { PageHeader } from "@/components/page-header";
import { CharacterNameSheet } from "@/components/shell/character-name-sheet";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { fallbackCampaignName, useCampaignName } from "@/hooks/use-campaign";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { useMe } from "@/hooks/use-me";
import { cachedMe, canUseDmTools, type Me, tableName } from "@/lib/access";
import { cn } from "@/lib/utils";
import { countOf } from "@/lib/words";

export const Route = createFileRoute("/_app/")({
	component: ColonyPage,
	loader: ({ context }) => {
		// Warm the DM's checklist; prefetching never throws, and the page handles a failure itself.
		const me = cachedMe(context.queryClient);
		if (me && canUseDmTools(me)) {
			context.queryClient.prefetchQuery(membersQuery);
			context.queryClient.prefetchQuery(settingsQuery);
		}
	},
});

/** M0's Colony page: the board itself (day dial, stock, shipments) arrives in M1. */
function ColonyPage() {
	const me = useMe();
	const campaignName = useCampaignName(me);
	useDocumentTitle("Colony", campaignName ?? fallbackCampaignName);
	const greetingName = tableName(me.member);

	return (
		<div className="flex flex-col gap-8 md:gap-10">
			<PageHeader
				description={`Welcome back, ${greetingName}.`}
				eyebrow={campaignName ? "The colony" : undefined}
				title={campaignName ?? "The colony"}
			/>
			<ColonyBoardPreview />
			{canUseDmTools(me) ? <TableChecklist /> : <YourPlace me={me} />}
		</div>
	);
}

const comingToTheBoard: { icon: LucideIcon; text: string }[] = [
	{ icon: SunriseIcon, text: "The day clock, and how many days until the next ship from Eldari" },
	{ icon: WheatIcon, text: "The stockpile, changing live as the DM keeps the books" },
	{ icon: UsersIcon, text: "How many colonists there are, and who is unassigned" },
	{ icon: ScrollTextIcon, text: "The latest entries from the colony's timeline" },
];

function ColonyBoardPreview() {
	return (
		<section
			aria-labelledby="colony-board-heading"
			className="relative isolate overflow-hidden rounded-2xl border border-border bg-card shadow-card"
		>
			<div
				aria-hidden="true"
				className="absolute inset-y-0 right-0 -z-10 hidden w-3/5 bg-[url(/eldari.webp)] bg-cover bg-center opacity-[0.14] [mask-image:linear-gradient(to_left,black,transparent)] lg:block"
			/>
			<div className="grid items-center gap-8 p-6 sm:p-8 lg:grid-cols-[auto_1fr] lg:gap-12 lg:p-10">
				<DayDial caption="The clock starts with the colony board" day={null} />
				<div className="min-w-0">
					<Badge variant="gilt">Coming in M1 · Colony clock and books</Badge>
					<h2
						className="mt-4 text-balance font-display font-semibold text-2xl tracking-wide md:text-3xl"
						id="colony-board-heading"
					>
						The colony board is being charted
					</h2>
					<p className="mt-3 max-w-prose font-flavor text-lg text-muted-foreground italic">
						The ink is still drying on the ledgers. When the board arrives, this is where the colony
						lives:
					</p>
					<ul className="mt-5 grid gap-3">
						{comingToTheBoard.map((entry) => (
							<li className="flex items-start gap-3" key={entry.text}>
								<span
									aria-hidden="true"
									className="mt-0.5 inline-flex size-7 shrink-0 items-center justify-center rounded-md bg-gilt/12 text-gilt-ink"
								>
									<entry.icon className="size-4" />
								</span>
								<span>{entry.text}</span>
							</li>
						))}
					</ul>
				</div>
			</div>
		</section>
	);
}

function TableChecklist() {
	const members = useQuery(membersQuery);
	const settings = useQuery(settingsQuery);
	const waitingCount = members.data?.pending.length ?? 0;
	const playerCount = members.data?.active.filter((member) => !member.dm?.isDm).length ?? 0;
	const channelCount = [
		settings.data?.discordAnnounceChannelId,
		settings.data?.discordReminderChannelId,
		settings.data?.discordDmChannelId,
	].filter(Boolean).length;

	return (
		<section aria-labelledby="checklist-heading" className="flex flex-col gap-4">
			<div>
				<h2 className="font-display font-semibold text-xl tracking-wide" id="checklist-heading">
					Ready the table
				</h2>
				<p className="text-muted-foreground">What can be set up before the colony board arrives.</p>
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
					to="/players"
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
					to="/settings"
				/>
				<ChecklistCard
					action="Set channels"
					hash="discord"
					icon={MessagesSquareIcon}
					isDone={channelCount === 3}
					status={
						<QueryStatus
							query={settings}
						>{`${channelCount} of 3 Discord channels set`}</QueryStatus>
					}
					title="Point Beny Bot at your channels"
					to="/settings"
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
	to: "/players" | "/settings";
	hash?: string;
	isDone: boolean;
	isHighlighted?: boolean;
}) {
	return (
		<Link
			className={cn(
				"group flex flex-col gap-4 rounded-xl border bg-card p-5 shadow-card outline-none transition-[border-color,transform] hover:-translate-y-0.5 hover:border-gilt/45 focus-visible:ring-[3px] focus-visible:ring-ring/45",
				isHighlighted ? "border-gilt/55" : "border-border",
			)}
			hash={hash}
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
