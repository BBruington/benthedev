import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import {
	ArrowLeftIcon,
	ArrowRightLeftIcon,
	DramaIcon,
	LogOutIcon,
	PenLineIcon,
	Trash2Icon,
	UserPlusIcon,
} from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { problemMessage, problemStatus } from "@/api/problem";
import {
	npcQuery,
	npcQueryKeyPrefix,
	npcsQueryKeyPrefix,
	placeQueryKeyPrefix,
	playedSessionsQuery,
	workforceQuery,
} from "@/api/queries";
import { useColonyChanges } from "@/colony/use-colony-changes";
import { ColonyIcon } from "@/components/colony-icon";
import { Field, fieldInputProps } from "@/components/field";
import { NativeSelect, SwitchRow } from "@/components/form-controls";
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
import { focusFirstVisible } from "@/lib/focus";
import { hiddenTrueName } from "@/lib/people";
import { localDateLabel } from "@/lib/sessions";
import { npcHeadingId, npcsPageHeadingId } from "@/npcs/npc-ids";
import { NpcFacts, ProseBlock } from "@/npcs/npc-profile";
import { NpcBadges } from "@/npcs/npc-roster";
import { NpcSheet } from "@/npcs/npc-sheet";
import {
	ChangeRoleSheet,
	type ColonistTarget,
	JoinColonySheet,
	LeaveColonyDialog,
} from "@/people/colonist-actions";
import { HiddenBadge, NpcAvatar } from "@/people/people-sections";

type Npc = Schemas["Npc"];
type SessionReference = Schemas["SessionReference"];

export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell/npcs/$npcId")({
	component: NpcPage,
	loader: ({ context: { campaignId, queryClient }, params: { npcId } }) => {
		if (!campaignId) {
			return;
		}
		queryClient.prefetchQuery(npcQuery(campaignId, npcId));
		const me = cachedMe(queryClient, campaignId);
		if (me && canUseDmTools(me)) {
			queryClient.prefetchQuery(workforceQuery(campaignId));
			queryClient.prefetchQuery(playedSessionsQuery(campaignId));
		}
	},
});

const colonyHeadingId = "npc-colony-heading";
function BackToRoster() {
	const { slug } = useCampaign();
	return (
		<Button asChild className="self-start" size="sm" variant="ghost">
			<Link params={{ campaignSlug: slug }} to="/c/$campaignSlug/npcs">
				<ArrowLeftIcon />
				All NPCs
			</Link>
		</Button>
	);
}

/** /npcs/{npcId} (spec §6.2, §5.9): one NPC; the DM edits in place, players see the redacted view. */
function NpcPage() {
	const { npcId } = Route.useParams();
	const campaignId = useCampaignId();
	const me = useMe();
	const isDm = canUseDmTools(me);
	const npc = useQuery(npcQuery(campaignId, npcId));
	useDocumentTitle(npc.data?.displayName ?? "NPC", useCampaignName() ?? fallbackCampaignName);

	if (npc.isPending) {
		return (
			<div aria-busy="true" className="flex flex-col gap-6">
				<Skeleton className="h-24 w-2/3" />
				<Skeleton className="h-64 rounded-2xl" />
			</div>
		);
	}
	if (npc.isError) {
		const isMissing = problemStatus(npc.error) === 404;
		return (
			<div className="flex flex-col gap-4">
				<BackToRoster />
				<StatusPanel
					description={
						isMissing
							? "There's nobody by that link. They may have been removed."
							: problemMessage(npc.error, "This NPC couldn't be loaded.")
					}
					icon={DramaIcon}
					title={isMissing ? "No such NPC" : "This NPC couldn't be loaded"}
					tone="ember"
				/>
			</div>
		);
	}
	return <NpcDetail isDm={isDm} npc={npc.data} />;
}

function NpcDetail({ npc, isDm }: { npc: Npc; isDm: boolean }) {
	const editing = useSheetState<true>();
	const trueName = hiddenTrueName(npc);
	return (
		<div className="flex flex-col gap-6">
			<BackToRoster />
			<header className="flex flex-wrap items-start gap-4">
				<NpcAvatar displayName={npc.displayName} portraitImageId={npc.portraitImageId} size="xl" />
				<div className="min-w-0 flex-1 space-y-2">
					<h1
						className="font-display font-semibold text-3xl tracking-wide outline-none [overflow-wrap:anywhere] md:text-4xl"
						id={npcHeadingId}
						tabIndex={-1}
					>
						{npc.displayName}
					</h1>
					{trueName ? (
						<p className="text-muted-foreground">
							True name <span className="font-semibold text-foreground">{trueName}</span>
							<span className="sr-only">, only you see it</span>
						</p>
					) : null}
					<p className="flex flex-wrap gap-1.5">
						<HiddenBadge item={npc} />
						<NpcBadges npc={npc} />
						{npc.labels.map((label) => (
							<Badge key={label} variant="outline">
								{label}
							</Badge>
						))}
					</p>
				</div>
				{isDm ? (
					<div className="flex w-full flex-wrap gap-2 sm:w-auto">
						<Button
							onClick={() => {
								editing.show(true);
							}}
							variant="outline"
						>
							<PenLineIcon />
							Edit
						</Button>
						<DeleteNpcButton npc={npc} />
					</div>
				) : null}
			</header>
			{isDm && npc.dm ? <RevealSwitch npc={npc} revealed={npc.dm.revealed} /> : null}
			{isDm ? <FirstSessionPicker npc={npc} /> : null}
			<section className="grid gap-6 rounded-2xl border border-border bg-card p-5 shadow-card md:p-7">
				<NpcFacts npc={npc} />
				<ProseBlock text={npc.appearance} title="Appearance" />
				<ProseBlock text={npc.personality} title="Personality" />
				<ProseBlock text={npc.backstory} title="Backstory" />
				<ProseBlock text={npc.playerNotes} title="Player notes" />
				{npc.dm ? <ProseBlock dmOnly text={npc.dm.dmNotes} title="DM notes" /> : null}
			</section>
			<ColonySection isDm={isDm} npc={npc} />
			{isDm && editing.value ? (
				<NpcSheet
					key={editing.session}
					npc={npc}
					onOpenChange={editing.onOpenChange}
					open={editing.open}
				/>
			) : null}
		</div>
	);
}

function useNpcSettled() {
	const queryClient = useQueryClient();
	const campaignId = useCampaignId();
	return () =>
		Promise.all([
			queryClient.invalidateQueries({ queryKey: npcsQueryKeyPrefix(campaignId) }),
			queryClient.invalidateQueries({ queryKey: npcQueryKeyPrefix(campaignId) }),
			queryClient.invalidateQueries({ queryKey: placeQueryKeyPrefix(campaignId) }),
			queryClient.invalidateQueries({ queryKey: workforceQuery(campaignId).queryKey }),
		]);
}

function RevealSwitch({ npc, revealed }: { npc: Npc; revealed: boolean }) {
	const campaignId = useCampaignId();
	const settle = useNpcSettled();
	const mutation = api.useMutation("put", "/api/campaigns/{campaignId}/npcs/{npcId}/revealed", {
		onError: (error) => {
			toast.error(problemMessage(error, "Their visibility couldn't be changed."));
		},
		// Returned, so the mutation stays pending until the refetch lands and the switch never
		// snaps back to the old value in between.
		onSettled: settle,
		onSuccess: (saved) => {
			toast.success(
				saved.dm?.revealed
					? `Players can now see ${saved.displayName}.`
					: `${saved.displayName} is hidden from players.`,
			);
		},
	});
	const shownRevealed = mutation.isPending
		? (mutation.variables?.body?.revealed ?? revealed)
		: revealed;
	return (
		<SwitchRow
			checked={shownRevealed}
			description={
				shownRevealed
					? `Players see them as ${npc.displayName}, without the true name or your notes.`
					: "Hidden: players can't find them in lists, search, places or the colony roster."
			}
			disabled={mutation.isPending}
			id="npc-visible"
			label="Visible to players"
			onCheckedChange={(checked) => {
				mutation.mutate({
					body: { revealed: checked },
					params: { path: { campaignId, npcId: npc.id } },
				});
			}}
		/>
	);
}

/**
 * Where the party first met them (Open Question 10): a played session, saved at once through its own
 * endpoint like visibility, so an open edit sheet can't overwrite it.
 */
function FirstSessionPicker({ npc }: { npc: Npc }) {
	const campaignId = useCampaignId();
	const settle = useNpcSettled();
	const played = useQuery(playedSessionsQuery(campaignId));
	const current: SessionReference | null = npc.firstSession;
	const mutation = api.useMutation(
		"put",
		"/api/campaigns/{campaignId}/npcs/{npcId}/first-session",
		{
			onError: (error) => {
				toast.error(problemMessage(error, "The first session couldn't be saved."));
			},
			onSettled: settle,
			onSuccess: (saved) => {
				const savedSession: SessionReference | null = saved.firstSession;
				toast.success(
					savedSession
						? `${saved.displayName} was first met at ${savedSession.title}.`
						: `${saved.displayName}'s first session was cleared.`,
				);
			},
		},
	);
	const shownId = mutation.isPending
		? (mutation.variables?.body?.sessionId ?? "")
		: (current?.id ?? "");
	const choices = played.data ?? [];
	const isListed = choices.some((session) => session.id === current?.id);
	return (
		<Field
			description={
				played.data && choices.length === 0
					? "No sessions played yet. Mark one played on Sessions first."
					: "Players see it as a link to that session's page."
			}
			id="npc-first-session"
			label="First met"
		>
			<NativeSelect
				{...fieldInputProps("npc-first-session", undefined, true)}
				disabled={mutation.isPending || !played.data}
				onChange={(event) => {
					mutation.mutate({
						body: { sessionId: event.target.value === "" ? null : event.target.value },
						params: { path: { campaignId, npcId: npc.id } },
					});
				}}
				value={shownId}
			>
				<option value="">Not set</option>
				{current && !isListed ? (
					<option value={current.id}>
						{current.title} · {localDateLabel(current.localDate)}
					</option>
				) : null}
				{choices.map((session) =>
					session.id ? (
						<option key={session.id} value={session.id}>
							{session.title} · {localDateLabel(session.localDate)}
						</option>
					) : null,
				)}
			</NativeSelect>
		</Field>
	);
}

function DeleteNpcButton({ npc }: { npc: Npc }) {
	const campaignId = useCampaignId();
	const { slug } = useCampaign();
	const navigate = useNavigate();
	const settle = useNpcSettled();
	const [open, setOpen] = useState(false);
	const [confirmed, setConfirmed] = useState(false);
	const mutation = api.useMutation("delete", "/api/campaigns/{campaignId}/npcs/{npcId}", {
		onError: (error) => {
			toast.error(problemMessage(error, `${npc.displayName} couldn't be deleted.`));
			document.getElementById(npcHeadingId)?.focus();
		},
		onSettled: settle,
		onSuccess: () => {
			toast.success(`${npc.displayName} was deleted.`);
			navigate({ params: { campaignSlug: slug }, to: "/c/$campaignSlug/npcs" }).then(() => {
				focusFirstVisible(npcsPageHeadingId);
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
					<AlertDialogTitle>Delete {npc.displayName}?</AlertDialogTitle>
					<AlertDialogDescription>
						Their record is gone for good; timeline entries that mention them stay. A colonist has
						to leave the colony first.
					</AlertDialogDescription>
				</AlertDialogHeader>
				<AlertDialogFooter>
					<AlertDialogCancel>Keep them</AlertDialogCancel>
					<AlertDialogAction
						onClick={() => {
							setConfirmed(true);
							mutation.mutate({ params: { path: { campaignId, npcId: npc.id } } });
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

/** Their place in the colony (spec §5.5); the DM's controls join, move and remove them. */
function ColonySection({ npc, isDm }: { npc: Npc; isDm: boolean }) {
	const campaignId = useCampaignId();
	const workforce = useQuery({ ...workforceQuery(campaignId), enabled: isDm });
	const changes = useColonyChanges();
	const join = useSheetState<true>();
	const changeRole = useSheetState<true>();
	const [isLeaving, setLeaving] = useState(false);
	const role = npc.colonyRole;
	const target: ColonistTarget = {
		displayName: npc.displayName,
		id: npc.id,
		isHidden: npc.dm?.revealed === false,
		roleKey: role?.key ?? null,
	};
	if (!(role || isDm)) {
		return null;
	}
	const focusTargets = [colonyHeadingId, npcHeadingId];
	return (
		<section
			aria-labelledby={colonyHeadingId}
			className="flex flex-col gap-3 rounded-2xl border border-border bg-card p-5 shadow-card md:p-7"
		>
			<h2
				className="font-display font-semibold text-xl tracking-wide outline-none"
				id={colonyHeadingId}
				tabIndex={-1}
			>
				The colony
			</h2>
			{role ? (
				<p className="inline-flex items-center gap-2 text-lg">
					<ColonyIcon aria-hidden="true" className="size-5 text-verdigris-ink" name={role.icon} />
					Works as {role.label}
				</p>
			) : (
				<p className="text-muted-foreground">Not a colonist.</p>
			)}
			{isDm && role && npc.status === "DEAD" ? (
				<p className="rounded-lg border border-gilt/40 bg-gilt/10 px-3 py-2 text-gilt-ink text-sm">
					Marked dead, but still counted as a colonist. Remove them from the colony if they're gone.
				</p>
			) : null}
			{isDm ? (
				<div className="flex flex-wrap gap-2">
					{role ? (
						<>
							<Button
								disabled={!workforce.data}
								onClick={() => {
									changeRole.show(true);
								}}
								variant="outline"
							>
								<ArrowRightLeftIcon />
								Change role
							</Button>
							<Button
								disabled={!workforce.data}
								onClick={() => {
									setLeaving(true);
								}}
								variant="ghost-destructive"
							>
								<LogOutIcon />
								Leave colony
							</Button>
						</>
					) : (
						<Button
							disabled={!workforce.data}
							onClick={() => {
								join.show(true);
							}}
							variant="outline"
						>
							<UserPlusIcon />
							Join colony
						</Button>
					)}
				</div>
			) : null}
			{isDm && workforce.data && join.value ? (
				<JoinColonySheet
					candidates={[]}
					changes={changes}
					focusTargetIds={focusTargets}
					key={join.session}
					npc={target}
					onOpenChange={join.onOpenChange}
					open={join.open}
					workforce={workforce.data}
				/>
			) : null}
			{isDm && workforce.data && changeRole.value ? (
				<ChangeRoleSheet
					changes={changes}
					key={changeRole.session}
					npc={target}
					onOpenChange={changeRole.onOpenChange}
					open={changeRole.open}
					workforce={workforce.data}
				/>
			) : null}
			{isDm && workforce.data ? (
				<LeaveColonyDialog
					changes={changes}
					focusTargetIds={focusTargets}
					npc={target}
					onOpenChange={setLeaving}
					open={isLeaving}
					roleLabel={role?.label ?? "their role"}
					workforce={workforce.data}
				/>
			) : null}
		</section>
	);
}
