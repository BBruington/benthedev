import { type UseQueryResult, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import {
	CheckIcon,
	EllipsisVerticalIcon,
	HourglassIcon,
	PenLineIcon,
	RotateCcwIcon,
	ShieldCheckIcon,
	UserMinusIcon,
	UsersIcon,
	UserXIcon,
	XIcon,
} from "lucide-react";
import { Fragment, type ReactNode, useRef, useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { problemMessage } from "@/api/problem";
import { membersQuery } from "@/api/queries";
import { BottomSheet } from "@/components/bottom-sheet";
import { CharacterNameForm } from "@/components/character-name-form";
import { MemberAvatar } from "@/components/member-avatar";
import { PageHeader } from "@/components/page-header";
import { Forbidden } from "@/components/shell/forbidden";
import { StatTile } from "@/components/stat-tile";
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
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Skeleton } from "@/components/ui/skeleton";
import { fallbackCampaignName, useCampaignId, useCampaignName } from "@/hooks/use-campaign";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { useMe } from "@/hooks/use-me";
import { cachedMe, canUseDmTools, tableName } from "@/lib/access";
import { focusFirstVisible } from "@/lib/focus";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/time";

type Member = Schemas["Member"];

/** What a removed member's row says: core decides which kind of removal it was. */
const removalLabels: Record<Schemas["RemovalKind"], string> = {
	DENIED: "Denied",
	LEFT: "Left",
	REMOVED: "Removed",
};

/** At most 20 requests wait at once per campaign; past that, the invite link turns people away. */
const pendingCap = 20;

export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell/players")({
	component: PlayersRoute,
	loader: ({ context: { campaignId, queryClient } }) => {
		if (!campaignId) {
			return;
		}
		const me = cachedMe(queryClient, campaignId);
		if (me && canUseDmTools(me)) {
			queryClient.prefetchQuery(membersQuery(campaignId));
		}
	},
});

function PlayersRoute() {
	const me = useMe();
	if (!canUseDmTools(me)) {
		return <Forbidden me={me} />;
	}
	return <PlayersPage />;
}

type Confirmation = { kind: "deny" | "remove" | "readmit"; member: Member };

function PlayersPage() {
	useDocumentTitle("Players", useCampaignName() ?? fallbackCampaignName);
	const members = useQuery(membersQuery(useCampaignId()));
	const actions = useMemberActions();
	const [confirmation, setConfirmation] = useState<Confirmation | null>(null);
	const [renaming, setRenaming] = useState<Member | null>(null);

	return (
		<div className="flex flex-col gap-8 md:gap-10">
			<PageHeader
				description="Who is waiting to join, who plays, and who was turned away or left. Anyone with your invite link can ask; only you let them in."
				eyebrow="Dungeon Master"
				title="Players"
			/>
			<MembersBody
				actions={actions}
				members={members}
				onConfirm={setConfirmation}
				onRename={setRenaming}
			/>
			<ConfirmMemberAction
				confirmation={confirmation}
				onConfirmed={(confirmed) => {
					actions.run(confirmed.kind, confirmed.member);
					setConfirmation(null);
				}}
				onOpenChange={(open) => {
					if (!open) {
						setConfirmation(null);
					}
				}}
			/>
			<CharacterNameEditor
				member={renaming}
				onOpenChange={(open) => {
					if (!open) {
						setRenaming(null);
					}
				}}
			/>
		</div>
	);
}

function MembersBody({
	members,
	actions,
	onConfirm,
	onRename,
}: {
	members: UseQueryResult<Schemas["MemberGroups"], unknown>;
	actions: MemberActions;
	onConfirm: (confirmation: Confirmation) => void;
	onRename: (member: Member) => void;
}) {
	if (members.isPending) {
		return <PlayersSkeleton />;
	}
	if (members.isError) {
		return (
			<StatusPanel
				description={problemMessage(members.error, "The member list couldn't be loaded.")}
				icon={UsersIcon}
				title="Players couldn't be loaded"
				tone="ember"
			/>
		);
	}

	const { pending, active, removed } = members.data;
	const waiting = [...pending].sort(byAskedFirst);
	const playing = [...active].sort(byDmThenName);
	const turnedAway = [...removed].sort(byName);

	return (
		<>
			<div className="grid grid-cols-3 gap-3 md:gap-4">
				<StatTile icon={HourglassIcon} label="Waiting" tone="gilt" value={pending.length} />
				<StatTile icon={UsersIcon} label="Playing" tone="moss" value={active.length} />
				{/* Counts the "Removed, left or denied" section; a label that long truncates at 360 px. */}
				<StatTile icon={UserXIcon} label="Out" tone="ember" value={removed.length} />
			</div>

			<MemberSection
				count={waiting.length}
				description="People who open your invite link and ask appear here straight away, and Beny Bot pings your DM channel."
				emptyMessage="No one is waiting to join."
				headingId={sectionHeadingIds.waiting}
				notice={
					waiting.length >= pendingCap ? (
						<p className="rounded-lg border border-gilt/40 bg-gilt/10 px-4 py-3 text-sm">
							The waiting list is full ({pendingCap}). New people opening your invite link are told
							you aren't taking players until you approve or deny someone.
						</p>
					) : null
				}
				title="Waiting to join"
			>
				{waiting.map((member) => (
					<MemberRow
						actions={
							<>
								<Button
									disabled={actions.isBusy(member)}
									onClick={() => {
										onConfirm({ kind: "deny", member });
									}}
									size="sm"
									variant="ghost-destructive"
								>
									<XIcon />
									Deny
								</Button>
								<Button
									disabled={actions.isBusy(member)}
									onClick={() => {
										actions.run("approve", member);
									}}
									size="sm"
								>
									<CheckIcon />
									Let in
								</Button>
							</>
						}
						detail={askedAt(member)}
						key={member.id}
						member={member}
					/>
				))}
			</MemberSection>

			<MemberSection
				count={playing.length}
				description="Character names show across the site and in Beny Bot's replies."
				emptyMessage="No one has been let in yet."
				headingId={sectionHeadingIds.playing}
				title="Playing"
			>
				{playing.map((member) => (
					<MemberRow
						actions={
							<ActiveMemberActions
								isBusy={actions.isBusy(member)}
								member={member}
								onRemove={() => {
									onConfirm({ kind: "remove", member });
								}}
								onRename={() => {
									onRename(member);
								}}
							/>
						}
						detail={lastSeen(member)}
						key={member.id}
						member={member}
						showCharacterName
					/>
				))}
			</MemberSection>

			<MemberSection
				count={turnedAway.length}
				description="People you removed or denied, and players who left, can't ask again, even with the invite link. Only you can let them back in."
				emptyMessage="No one has been removed, denied or left."
				headingId={sectionHeadingIds.removed}
				title="Removed, left or denied"
			>
				{turnedAway.map((member) => (
					<MemberRow
						actions={
							<Button
								disabled={actions.isBusy(member)}
								onClick={() => {
									onConfirm({ kind: "readmit", member });
								}}
								size="sm"
								variant="outline"
							>
								<RotateCcwIcon />
								Re-admit
							</Button>
						}
						detail={removalLabel(member)}
						key={member.id}
						member={member}
						muted
						showCharacterName
					/>
				))}
			</MemberSection>
		</>
	);
}

function byAskedFirst(first: Member, second: Member): number {
	return (first.dm?.createdAt ?? "").localeCompare(second.dm?.createdAt ?? "");
}

function byName(first: Member, second: Member): number {
	return tableName(first).localeCompare(tableName(second));
}

function byDmThenName(first: Member, second: Member): number {
	const firstIsDm = first.dm?.isDm ?? false;
	const secondIsDm = second.dm?.isDm ?? false;
	if (firstIsDm !== secondIsDm) {
		return firstIsDm ? -1 : 1;
	}
	return byName(first, second);
}

function removalLabel(member: Member): string {
	const removalKind = member.dm?.removalKind;
	if (!removalKind) {
		return removalLabels.REMOVED;
	}
	return removalLabels[removalKind];
}

function askedAt(member: Member): ReactNode {
	const createdAt = member.dm?.createdAt;
	if (!createdAt) {
		return null;
	}
	return (
		<time dateTime={createdAt} title={formatAbsoluteTime(createdAt)}>
			Asked {formatRelativeTime(createdAt)}
		</time>
	);
}

function lastSeen(member: Member): ReactNode {
	const lastSeenAt = member.dm?.lastSeenAt;
	if (!lastSeenAt) {
		return null;
	}
	return (
		<time dateTime={lastSeenAt} title={formatAbsoluteTime(lastSeenAt)}>
			Seen {formatRelativeTime(lastSeenAt)}
		</time>
	);
}

function MemberSection({
	headingId,
	title,
	description,
	count,
	emptyMessage,
	notice,
	children,
}: {
	headingId: string;
	title: string;
	description: string;
	count: number;
	emptyMessage: string;
	/** Shown above the list, outside it. */
	notice?: ReactNode;
	children: ReactNode;
}) {
	return (
		<section aria-labelledby={headingId} className="flex flex-col gap-3">
			<div>
				<h2
					className="flex items-center gap-2.5 font-display font-semibold text-xl tracking-wide outline-none"
					id={headingId}
					tabIndex={-1}
				>
					{title}
					<Badge variant="muted">{count}</Badge>
				</h2>
				<p className="text-muted-foreground text-sm">{description}</p>
			</div>
			{notice}
			{count === 0 ? (
				<p className="rounded-xl border border-border border-dashed px-4 py-6 text-center text-muted-foreground">
					{emptyMessage}
				</p>
			) : (
				<ul className="flex flex-col gap-2.5">{children}</ul>
			)}
		</section>
	);
}

function MemberRow({
	member,
	detail,
	actions,
	showCharacterName = false,
	muted = false,
}: {
	member: Member;
	detail?: ReactNode;
	actions: ReactNode;
	showCharacterName?: boolean;
	muted?: boolean;
}) {
	const isDm = member.dm?.isDm ?? false;
	const characterName = showCharacterName ? member.characterName : null;
	const discordUsername = member.dm?.discordUsername;
	const subtitle: { key: string; content: ReactNode }[] = [];
	if (characterName) {
		subtitle.push({ content: member.displayName, key: "display-name" });
	}
	if (discordUsername) {
		subtitle.push({ content: `@${discordUsername}`, key: "discord-username" });
	}
	if (detail) {
		subtitle.push({ content: detail, key: "detail" });
	}
	return (
		<li className="flex flex-wrap items-center gap-x-4 gap-y-3 rounded-xl border border-border bg-card px-4 py-3.5 shadow-card sm:flex-nowrap">
			<div className="flex min-w-0 flex-1 items-center gap-3.5">
				<MemberAvatar
					avatarUrl={member.avatarUrl}
					className={muted ? "opacity-60 grayscale" : undefined}
					name={tableName(member)}
				/>
				<div className="min-w-0">
					<p className="flex items-center gap-2">
						<span className="truncate font-semibold text-base">
							{characterName || member.displayName}
						</span>
						{isDm ? (
							<Badge variant="gilt">
								<ShieldCheckIcon />
								DM
							</Badge>
						) : null}
					</p>
					<p className="truncate text-muted-foreground text-sm">
						{subtitle.map((part, position) => (
							<Fragment key={part.key}>
								{position > 0 ? " · " : null}
								{part.content}
							</Fragment>
						))}
					</p>
				</div>
			</div>
			<div className="flex w-full shrink-0 justify-end gap-2 sm:w-auto">{actions}</div>
		</li>
	);
}

function ActiveMemberActions({
	member,
	isBusy,
	onRename,
	onRemove,
}: {
	member: Member;
	isBusy: boolean;
	onRename: () => void;
	onRemove: () => void;
}) {
	const isDm = member.dm?.isDm ?? false;
	return (
		<>
			<Button disabled={isBusy} onClick={onRename} size="sm" variant="outline">
				<PenLineIcon />
				{member.characterName ? "Rename" : "Name character"}
			</Button>
			{isDm ? null : (
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button
							aria-label={`More for ${tableName(member)}`}
							disabled={isBusy}
							size="icon-sm"
							variant="ghost"
						>
							<EllipsisVerticalIcon />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="end">
						<DropdownMenuItem onSelect={onRemove} variant="destructive">
							<UserMinusIcon />
							Remove from the campaign…
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
			)}
		</>
	);
}

function PlayersSkeleton() {
	return (
		<div aria-busy="true" className="flex flex-col gap-8">
			<div className="grid grid-cols-3 gap-3 md:gap-4">
				<Skeleton className="h-28 rounded-xl" />
				<Skeleton className="h-28 rounded-xl" />
				<Skeleton className="h-28 rounded-xl" />
			</div>
			<div className="flex flex-col gap-2.5">
				<Skeleton className="h-6 w-48" />
				<Skeleton className="h-[4.5rem] rounded-xl" />
				<Skeleton className="h-[4.5rem] rounded-xl" />
			</div>
		</div>
	);
}

type MemberActionKind = "approve" | "deny" | "remove" | "readmit";

/** Section headings take focus when an action moves a row (and its buttons) to another section. */
const sectionHeadingIds = {
	playing: "members-playing",
	removed: "members-removed-or-denied",
	waiting: "members-waiting-to-join",
} as const;

/** Where focus goes once an action succeeds (the row's new section) or fails (the row's own section). */
const focusTargets: Record<MemberActionKind, { succeeded: string; failed: string }> = {
	approve: { failed: sectionHeadingIds.waiting, succeeded: sectionHeadingIds.playing },
	deny: { failed: sectionHeadingIds.waiting, succeeded: sectionHeadingIds.removed },
	readmit: { failed: sectionHeadingIds.removed, succeeded: sectionHeadingIds.playing },
	remove: { failed: sectionHeadingIds.playing, succeeded: sectionHeadingIds.removed },
};

type MemberActions = {
	run: (kind: MemberActionKind, member: Member) => void;
	isBusy: (member: Member) => boolean;
};

const successMessages: Record<MemberActionKind, (name: string) => string> = {
	approve: (name) => `${name} is in.`,
	deny: (name) => `${name} was turned away.`,
	readmit: (name) => `${name} is back in.`,
	remove: (name) => `${name} was removed from this campaign.`,
};

const failureMessages: Record<MemberActionKind, string> = {
	approve: "They couldn't be let in.",
	deny: "They couldn't be turned away.",
	readmit: "They couldn't be re-admitted.",
	remove: "They couldn't be removed.",
};

/** The four decisions on /players; each refreshes the lists and says what happened. */
function useMemberActions(): MemberActions {
	const queryClient = useQueryClient();
	const campaignId = useCampaignId();
	const [busyMemberIds, setBusyMemberIds] = useState<ReadonlySet<string>>(new Set());

	function settle(kind: MemberActionKind, member: Member) {
		return {
			onError: (error: unknown) => {
				toast.error(problemMessage(error, failureMessages[kind]));
				focusFirstVisible(focusTargets[kind].failed);
			},
			onSettled: () => {
				setBusyMemberIds((current) => {
					const next = new Set(current);
					next.delete(member.id);
					return next;
				});
				queryClient.invalidateQueries({ queryKey: membersQuery(campaignId).queryKey });
			},
			onSuccess: () => {
				toast.success(successMessages[kind](tableName(member)));
				focusFirstVisible(focusTargets[kind].succeeded);
			},
		};
	}

	const approve = api.useMutation("post", "/api/campaigns/{campaignId}/members/{memberId}/approve");
	const deny = api.useMutation("post", "/api/campaigns/{campaignId}/members/{memberId}/deny");
	const remove = api.useMutation("post", "/api/campaigns/{campaignId}/members/{memberId}/remove");
	const readmit = api.useMutation("post", "/api/campaigns/{campaignId}/members/{memberId}/readmit");
	const mutations = { approve, deny, readmit, remove };

	return {
		isBusy: (member) => busyMemberIds.has(member.id),
		run: (kind, member) => {
			setBusyMemberIds((current) => new Set(current).add(member.id));
			mutations[kind].mutate(
				{ params: { path: { campaignId, memberId: member.id } } },
				settle(kind, member),
			);
		},
	};
}

const confirmationCopy: Record<
	Confirmation["kind"],
	{ title: (name: string) => string; description: string; action: string; destructive: boolean }
> = {
	deny: {
		action: "Deny",
		description:
			"They won't be able to ask again with the invite link, and opening it won't ping you. You can re-admit them later from Removed, left or denied.",
		destructive: true,
		title: (name) => `Turn ${name} away?`,
	},
	readmit: {
		action: "Re-admit",
		description: "They'll see the colony again, as a player.",
		destructive: false,
		title: (name) => `Let ${name} back in?`,
	},
	remove: {
		action: "Remove",
		description:
			"They lose access to this campaign at once and can't ask again with the invite link. Their other campaigns aren't affected. You can re-admit them later.",
		destructive: true,
		title: (name) => `Remove ${name} from the campaign?`,
	},
};

function ConfirmMemberAction({
	confirmation,
	onOpenChange,
	onConfirmed,
}: {
	confirmation: Confirmation | null;
	onOpenChange: (open: boolean) => void;
	onConfirmed: (confirmation: Confirmation) => void;
}) {
	// Keep the last confirmation's text on screen while the dialog animates closed.
	const [shown, setShown] = useState(confirmation);
	if (confirmation && confirmation !== shown) {
		setShown(confirmation);
	}
	const copy = shown ? confirmationCopy[shown.kind] : undefined;
	// After a confirmed action the trigger is disabled and about to move, so Radix can't return focus to
	// it; the action moves focus to a section heading when it settles instead (useMemberActions).
	const confirmed = useRef(false);
	return (
		<AlertDialog onOpenChange={onOpenChange} open={confirmation !== null}>
			<AlertDialogContent
				onCloseAutoFocus={(event) => {
					if (confirmed.current) {
						event.preventDefault();
						confirmed.current = false;
					}
				}}
			>
				{shown && copy ? (
					<>
						<AlertDialogHeader>
							<AlertDialogTitle>{copy.title(tableName(shown.member))}</AlertDialogTitle>
							<AlertDialogDescription>{copy.description}</AlertDialogDescription>
						</AlertDialogHeader>
						<AlertDialogFooter>
							<AlertDialogCancel>Cancel</AlertDialogCancel>
							<AlertDialogAction
								onClick={() => {
									confirmed.current = true;
									onConfirmed(shown);
								}}
								variant={copy.destructive ? "destructive" : "default"}
							>
								{copy.action}
							</AlertDialogAction>
						</AlertDialogFooter>
					</>
				) : null}
			</AlertDialogContent>
		</AlertDialog>
	);
}

function CharacterNameEditor({
	member,
	onOpenChange,
}: {
	member: Member | null;
	onOpenChange: (open: boolean) => void;
}) {
	const [shown, setShown] = useState(member);
	if (member && member !== shown) {
		setShown(member);
	}
	return (
		<BottomSheet
			description={shown ? `The character ${shown.displayName} plays.` : undefined}
			onOpenChange={onOpenChange}
			open={member !== null}
			title="Character name"
		>
			{shown ? (
				<MemberCharacterNameForm
					key={shown.id}
					member={shown}
					onDone={() => {
						onOpenChange(false);
					}}
				/>
			) : null}
		</BottomSheet>
	);
}

function MemberCharacterNameForm({ member, onDone }: { member: Member; onDone: () => void }) {
	const queryClient = useQueryClient();
	const campaignId = useCampaignId();
	const mutation = api.useMutation(
		"put",
		"/api/campaigns/{campaignId}/members/{memberId}/character-name",
		{
			onSettled: () => {
				queryClient.invalidateQueries({ queryKey: membersQuery(campaignId).queryKey });
			},
			onSuccess: (updated) => {
				if (updated.characterName) {
					toast.success(`${updated.displayName} plays ${updated.characterName}.`);
				} else {
					toast.success(`${updated.displayName}'s character name was cleared.`);
				}
				onDone();
			},
		},
	);

	return (
		<CharacterNameForm
			description="Leave it empty to clear it."
			error={mutation.error}
			failureMessage="The character name couldn't be saved."
			id="member-character-name"
			initialName={member.characterName ?? ""}
			isPending={mutation.isPending}
			onCancel={onDone}
			onSubmit={(characterName) => {
				mutation.mutate({
					body: { characterName },
					params: { path: { campaignId, memberId: member.id } },
				});
			}}
		/>
	);
}
