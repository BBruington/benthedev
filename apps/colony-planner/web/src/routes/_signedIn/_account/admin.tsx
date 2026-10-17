import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
	CheckIcon,
	CopyIcon,
	EllipsisVerticalIcon,
	PauseCircleIcon,
	PlayCircleIcon,
	ShieldAlertIcon,
	Trash2Icon,
	XIcon,
} from "lucide-react";
import { type FormEvent, useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import {
	adminCampaignRequestsQuery,
	adminCampaignsQuery,
	meQuery,
	myCampaignRequestQuery,
	myCampaignsQuery,
} from "@/api/queries";
import { DeleteCampaignDialog } from "@/components/delete-campaign-dialog";
import { Field, fieldInputProps } from "@/components/field";
import { MemberAvatar } from "@/components/member-avatar";
import { PageHeader } from "@/components/page-header";
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
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { useAccount } from "@/hooks/use-me";
import { slugProblem } from "@/lib/campaigns";
import { copyToClipboard } from "@/lib/clipboard";
import { focusFirstVisible } from "@/lib/focus";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/time";
import { countOf } from "@/lib/words";

type AdminCampaignRequest = Schemas["AdminCampaignRequest"];
type AdminCampaign = Schemas["AdminCampaign"];

const requestsHeadingId = "admin-requests-heading";
const campaignsHeadingId = "admin-campaigns-heading";

// The site owner's page (docs/design/multi-campaign.md §4.3, §5.2, §5.5): open campaign requests and
// the campaign directory. Directory data only: no colony, timeline or settings of any campaign.
export const Route = createFileRoute("/_signedIn/_account/admin")({
	component: AdminRoute,
	loader: ({ context }) => {
		const account = context.queryClient.getQueryData<Schemas["AccountMe"]>(meQuery.queryKey);
		if (account?.isSiteOwner) {
			context.queryClient.prefetchQuery(adminCampaignRequestsQuery);
			context.queryClient.prefetchQuery(adminCampaignsQuery);
		}
	},
});

function AdminRoute() {
	const account = useAccount();
	useDocumentTitle("Site admin", "Colony Planner");
	if (!account.isSiteOwner) {
		return (
			<StatusPanel
				actions={
					<Button asChild variant="outline">
						<Link to="/">Your campaigns</Link>
					</Button>
				}
				description="This page is for the person who runs this site."
				eyebrow={<Badge variant="ember">403 · Site owner only</Badge>}
				icon={ShieldAlertIcon}
				title="Only the site owner can open this page"
				tone="ember"
			/>
		);
	}
	return (
		<div className="flex flex-col gap-10">
			<PageHeader
				description="Approve new campaigns, and pause or delete existing ones. You manage campaigns here; what happens inside them stays with their tables."
				eyebrow="Site owner"
				title="Site admin"
			/>
			<RequestsSection />
			<CampaignsSection />
		</div>
	);
}

function SectionHeading({ id, title, count }: { id: string; title: string; count?: number }) {
	return (
		<h2
			className="flex items-center gap-3 font-display font-semibold text-2xl tracking-wide outline-none"
			id={id}
			tabIndex={-1}
		>
			{title}
			{count === undefined ? null : <Badge variant="muted">{count}</Badge>}
		</h2>
	);
}

function RequestsSection() {
	const requests = useQuery(adminCampaignRequestsQuery);
	const [approving, setApproving] = useState<AdminCampaignRequest | null>(null);
	const [denying, setDenying] = useState<AdminCampaignRequest | null>(null);
	const decide = useDecisions();

	let body = (
		<div aria-busy="true" className="grid gap-3">
			<Skeleton className="h-32 rounded-2xl" />
		</div>
	);
	if (requests.isError) {
		body = (
			<p className="text-muted-foreground">
				{problemMessage(requests.error, "The requests couldn't be loaded.")}
			</p>
		);
	} else if (requests.data?.requests.length === 0) {
		body = (
			<p className="rounded-2xl border border-border border-dashed px-5 py-6 text-center text-muted-foreground">
				No one is asking to run a campaign right now.
			</p>
		);
	} else if (requests.data) {
		body = (
			<ul className="grid gap-4">
				{requests.data.requests.map((entry) => (
					<li key={entry.request.id}>
						<RequestCard entry={entry} onApprove={setApproving} onDeny={setDenying} />
					</li>
				))}
			</ul>
		);
	}

	return (
		<section aria-labelledby={requestsHeadingId} className="flex flex-col gap-4">
			<SectionHeading
				count={requests.data?.requests.length}
				id={requestsHeadingId}
				title="Requests"
			/>
			{body}
			<ApproveDialog
				approve={decide.approve}
				entry={approving}
				onOpenChange={(open) => {
					if (!open) {
						setApproving(null);
					}
				}}
			/>
			<AlertDialog
				onOpenChange={(open) => {
					if (!open) {
						setDenying(null);
					}
				}}
				open={denying !== null}
			>
				<AlertDialogContent
					onCloseAutoFocus={(event) => {
						if (decide.deny.isSuccess) {
							event.preventDefault();
							focusFirstVisible(requestsHeadingId);
						}
					}}
				>
					<AlertDialogHeader>
						<AlertDialogTitle>Say no to {denying?.request.campaignName}?</AlertDialogTitle>
						<AlertDialogDescription>
							{denying?.displayName} sees "The site owner said no this time" and may ask again. If
							you want to tell them why, tell them on Discord.
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>Not yet</AlertDialogCancel>
						<AlertDialogAction
							disabled={decide.deny.isPending}
							onClick={(event) => {
								event.preventDefault();
								if (denying) {
									decide.deny.mutate(
										{ params: { path: { campaignRequestId: denying.request.id } } },
										{
											onSuccess: () => {
												setDenying(null);
											},
										},
									);
								}
							}}
							variant="destructive"
						>
							{decide.deny.isPending ? "Saying no…" : "Deny"}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</section>
	);
}

/** Approve and deny refresh the owner's lists, and the owner's own picker, which may gain a campaign. */
function useDecisions() {
	const queryClient = useQueryClient();
	function refresh() {
		queryClient.invalidateQueries({ queryKey: adminCampaignRequestsQuery.queryKey });
		queryClient.invalidateQueries({ queryKey: adminCampaignsQuery.queryKey });
		queryClient.invalidateQueries({ queryKey: myCampaignRequestQuery.queryKey });
		queryClient.invalidateQueries({ queryKey: myCampaignsQuery.queryKey });
	}
	const approve = api.useMutation(
		"post",
		"/api/admin/campaign-requests/{campaignRequestId}/approve",
		{
			onError: (error) => {
				if (fieldErrors(error).slug === undefined) {
					toast.error(problemMessage(error, "The request couldn't be approved."));
					refresh();
				}
			},
			onSuccess: (campaign) => {
				refresh();
				toast.success(`${campaign.name} is ready at /c/${campaign.slug}.`);
			},
		},
	);
	const deny = api.useMutation("post", "/api/admin/campaign-requests/{campaignRequestId}/deny", {
		onError: (error) => {
			toast.error(problemMessage(error, "The request couldn't be denied."));
			refresh();
		},
		onSuccess: (request) => {
			refresh();
			toast.success(`Said no to ${request.campaignName}.`);
		},
	});
	return { approve, deny };
}

type Decisions = ReturnType<typeof useDecisions>;

function RequestCard({
	entry,
	onApprove,
	onDeny,
}: {
	entry: AdminCampaignRequest;
	onApprove: (entry: AdminCampaignRequest) => void;
	onDeny: (entry: AdminCampaignRequest) => void;
}) {
	const { request } = entry;
	return (
		<article className="flex flex-col gap-4 rounded-2xl border border-border bg-card p-5 shadow-card">
			<div className="flex items-center gap-3">
				<MemberAvatar avatarUrl={entry.avatarUrl} name={entry.displayName} />
				<div className="min-w-0">
					<p className="truncate font-medium">{entry.displayName}</p>
					<p className="truncate text-muted-foreground text-sm">@{entry.discordUsername}</p>
				</div>
				<span className="ml-auto shrink-0 text-muted-foreground text-sm">
					{formatRelativeTime(request.createdAt)}
				</span>
			</div>
			<div className="min-w-0">
				<h3 className="font-display font-semibold text-xl tracking-wide [overflow-wrap:anywhere]">
					{request.campaignName}
				</h3>
				<p className="font-mono text-muted-foreground text-sm [overflow-wrap:anywhere]">
					/c/{request.slug}
				</p>
				{request.note ? (
					<p className="mt-3 whitespace-pre-line rounded-lg border border-border bg-surface-sunken/50 px-3 py-2 font-flavor italic [overflow-wrap:anywhere]">
						{request.note}
					</p>
				) : null}
			</div>
			<div className="flex flex-wrap gap-2">
				<Button
					onClick={() => {
						onApprove(entry);
					}}
				>
					<CheckIcon />
					Approve…
				</Button>
				<Button
					onClick={() => {
						onDeny(entry);
					}}
					variant="ghost-destructive"
				>
					<XIcon />
					Deny…
				</Button>
			</div>
		</article>
	);
}

/** Approval may adjust the address: it is fixed from then on (C2). */
function ApproveDialog({
	entry,
	approve,
	onOpenChange,
}: {
	entry: AdminCampaignRequest | null;
	approve: Decisions["approve"];
	onOpenChange: (open: boolean) => void;
}) {
	return (
		<Dialog onOpenChange={onOpenChange} open={entry !== null}>
			<DialogContent
				onCloseAutoFocus={(event) => {
					if (approve.isSuccess) {
						event.preventDefault();
						focusFirstVisible(requestsHeadingId);
					}
				}}
			>
				{entry ? (
					<ApproveForm
						approve={approve}
						entry={entry}
						key={entry.request.id}
						onDone={() => {
							onOpenChange(false);
						}}
					/>
				) : null}
			</DialogContent>
		</Dialog>
	);
}

function ApproveForm({
	entry,
	approve,
	onDone,
}: {
	entry: AdminCampaignRequest;
	approve: Decisions["approve"];
	onDone: () => void;
}) {
	const [slug, setSlug] = useState(entry.request.slug);
	const [localProblem, setLocalProblem] = useState<string | undefined>(undefined);
	const slugError = localProblem ?? fieldErrors(approve.error).slug;

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		const trimmed = slug.trim();
		const problem = slugProblem(trimmed);
		if (problem) {
			setLocalProblem(problem);
			return;
		}
		approve.mutate(
			{
				body: { slug: trimmed === entry.request.slug ? null : trimmed },
				params: { path: { campaignRequestId: entry.request.id } },
			},
			{ onSuccess: onDone },
		);
	}

	return (
		<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
			<DialogHeader>
				<DialogTitle>Approve {entry.request.campaignName}?</DialogTitle>
				<DialogDescription>
					{entry.displayName} becomes its DM. The campaign starts empty, on Day 0, with the default
					resources and roles.
				</DialogDescription>
			</DialogHeader>
			<Field
				description="Change it now if you need to; it can't change after approval."
				error={slugError}
				id="approve-slug"
				label="Address"
			>
				<div className="flex min-w-0 items-center rounded-md border border-input bg-surface-raised/70 focus-within:border-ring focus-within:ring-[3px] focus-within:ring-ring/35 dark:bg-surface-sunken/60">
					<span aria-hidden="true" className="pl-3 font-mono text-muted-foreground">
						/c/
					</span>
					<Input
						{...fieldInputProps("approve-slug", slugError, true)}
						autoCapitalize="none"
						autoComplete="off"
						className="border-0 bg-transparent pl-0.5 font-mono shadow-none focus-visible:ring-0 dark:bg-transparent"
						maxLength={40}
						onChange={(event) => {
							setSlug(event.target.value.toLowerCase());
							setLocalProblem(undefined);
						}}
						spellCheck={false}
						value={slug}
					/>
				</div>
			</Field>
			<DialogFooter>
				<Button onClick={onDone} type="button" variant="outline">
					Cancel
				</Button>
				<Button disabled={approve.isPending} type="submit">
					<CheckIcon />
					{approve.isPending ? "Approving…" : "Approve"}
				</Button>
			</DialogFooter>
		</form>
	);
}

type CampaignAction = { kind: "suspend" | "delete"; campaign: AdminCampaign };

function CampaignsSection() {
	const queryClient = useQueryClient();
	const campaigns = useQuery(adminCampaignsQuery);
	const [pending, setPending] = useState<CampaignAction | null>(null);

	function refresh() {
		queryClient.invalidateQueries({ queryKey: adminCampaignsQuery.queryKey });
		queryClient.invalidateQueries({ queryKey: myCampaignRequestQuery.queryKey });
		queryClient.invalidateQueries({ queryKey: myCampaignsQuery.queryKey });
	}
	const suspend = api.useMutation("post", "/api/admin/campaigns/{campaignId}/suspend", {
		onError: (error) => {
			toast.error(problemMessage(error, "The campaign couldn't be paused."));
		},
		onSuccess: (campaign) => {
			refresh();
			toast.success(`${campaign.name} is paused.`);
		},
	});
	const unsuspend = api.useMutation("post", "/api/admin/campaigns/{campaignId}/unsuspend", {
		onError: (error) => {
			toast.error(problemMessage(error, "The campaign couldn't be unpaused."));
		},
		onSuccess: (campaign) => {
			refresh();
			toast.success(`${campaign.name} is open again.`);
		},
	});
	const remove = api.useMutation("post", "/api/admin/campaigns/{campaignId}/delete", {
		onSuccess: () => {
			refresh();
		},
	});

	let body = (
		<div aria-busy="true" className="grid gap-3 md:grid-cols-2">
			<Skeleton className="h-36 rounded-2xl" />
			<Skeleton className="h-36 rounded-2xl" />
		</div>
	);
	if (campaigns.isError) {
		body = (
			<p className="text-muted-foreground">
				{problemMessage(campaigns.error, "The campaigns couldn't be loaded.")}
			</p>
		);
	} else if (campaigns.data?.campaigns.length === 0) {
		body = (
			<p className="rounded-2xl border border-border border-dashed px-5 py-6 text-center text-muted-foreground">
				No campaigns yet.
			</p>
		);
	} else if (campaigns.data) {
		body = (
			<ul className="grid gap-4 md:grid-cols-2">
				{campaigns.data.campaigns.map((campaign) => (
					<li key={campaign.id}>
						<CampaignCard
							campaign={campaign}
							onDelete={() => {
								remove.reset();
								setPending({ campaign, kind: "delete" });
							}}
							onSuspend={() => {
								setPending({ campaign, kind: "suspend" });
							}}
							onUnsuspend={() => {
								unsuspend.mutate({ params: { path: { campaignId: campaign.id } } });
							}}
						/>
					</li>
				))}
			</ul>
		);
	}

	const deleting = pending?.kind === "delete" ? pending.campaign : null;
	const suspending = pending?.kind === "suspend" ? pending.campaign : null;

	return (
		<section aria-labelledby={campaignsHeadingId} className="flex flex-col gap-4">
			<SectionHeading
				count={campaigns.data?.campaigns.length}
				id={campaignsHeadingId}
				title="Campaigns"
			/>
			{body}
			<AlertDialog
				onOpenChange={(open) => {
					if (!open) {
						setPending(null);
					}
				}}
				open={suspending !== null}
			>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>Pause {suspending?.name}?</AlertDialogTitle>
						<AlertDialogDescription>
							Everyone in {suspending?.name} loses access until you unsuspend it. Nothing is
							deleted.
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>Keep it open</AlertDialogCancel>
						<AlertDialogAction
							onClick={() => {
								if (suspending) {
									suspend.mutate({ params: { path: { campaignId: suspending.id } } });
								}
							}}
							variant="destructive"
						>
							Suspend
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
			{deleting ? (
				<DeleteCampaignDialog
					campaignName={deleting.name}
					error={remove.error}
					isPending={remove.isPending}
					onConfirm={(confirmSlug) => {
						remove.mutate(
							{ body: { confirmSlug }, params: { path: { campaignId: deleting.id } } },
							{
								onSuccess: () => {
									toast.success(`${deleting.name} was deleted.`);
									setPending(null);
									// The dialog unmounts with its campaign, so focus is moved here.
									requestAnimationFrame(() => {
										focusFirstVisible(campaignsHeadingId);
									});
								},
							},
						);
					}}
					onOpenChange={(open) => {
						if (!open) {
							setPending(null);
						}
					}}
					open
					slug={deleting.slug}
				/>
			) : null}
		</section>
	);
}

function CampaignCard({
	campaign,
	onSuspend,
	onUnsuspend,
	onDelete,
}: {
	campaign: AdminCampaign;
	onSuspend: () => void;
	onUnsuspend: () => void;
	onDelete: () => void;
}) {
	const isPaused = campaign.status === "SUSPENDED";
	return (
		<article className="flex h-full flex-col gap-3 rounded-2xl border border-border bg-card p-5 shadow-card">
			<div className="flex items-start gap-2">
				<div className="min-w-0 flex-1">
					<h3 className="font-display font-semibold text-xl tracking-wide [overflow-wrap:anywhere]">
						{campaign.name}
					</h3>
					<p className="font-mono text-muted-foreground text-sm [overflow-wrap:anywhere]">
						/c/{campaign.slug}
					</p>
				</div>
				{isPaused ? (
					<Badge variant="verdigris">
						<PauseCircleIcon />
						Paused
					</Badge>
				) : null}
				<CampaignMenu
					campaign={campaign}
					onDelete={onDelete}
					onSuspend={onSuspend}
					onUnsuspend={onUnsuspend}
				/>
			</div>
			<dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
				<dt className="text-muted-foreground">DM</dt>
				<dd className="min-w-0 truncate">
					{campaign.dmDisplayName}{" "}
					<span className="text-muted-foreground">@{campaign.dmDiscordUsername}</span>
				</dd>
				<dt className="text-muted-foreground">Players</dt>
				<dd>
					{countOf(campaign.playerCount, "player")} · {campaign.pendingCount} waiting
				</dd>
				<dt className="text-muted-foreground">Discord</dt>
				<dd>{campaign.discordLinked ? "linked" : "not linked"}</dd>
				<dt className="text-muted-foreground">Created</dt>
				<dd>{formatAbsoluteTime(campaign.createdAt)}</dd>
			</dl>
		</article>
	);
}

function CampaignMenu({
	campaign,
	onSuspend,
	onUnsuspend,
	onDelete,
}: {
	campaign: AdminCampaign;
	onSuspend: () => void;
	onUnsuspend: () => void;
	onDelete: () => void;
}) {
	const isPaused = campaign.status === "SUSPENDED";
	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button aria-label={`Actions for ${campaign.name}`} size="icon" variant="ghost">
					<EllipsisVerticalIcon />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end" className="w-56">
				<DropdownMenuItem
					onSelect={() => {
						copyToClipboard(campaign.id).then(
							() => {
								toast.success("Campaign ID copied.");
							},
							() => {
								toast.error("Copying didn't work. Try again.");
							},
						);
					}}
				>
					<CopyIcon />
					Copy ID
				</DropdownMenuItem>
				<DropdownMenuSeparator />
				{isPaused ? (
					<DropdownMenuItem onSelect={onUnsuspend}>
						<PlayCircleIcon />
						Unsuspend
					</DropdownMenuItem>
				) : (
					<DropdownMenuItem onSelect={onSuspend}>
						<PauseCircleIcon />
						Suspend…
					</DropdownMenuItem>
				)}
				{isPaused ? (
					<DropdownMenuItem onSelect={onDelete} variant="destructive">
						<Trash2Icon />
						Delete…
					</DropdownMenuItem>
				) : null}
			</DropdownMenuContent>
		</DropdownMenu>
	);
}
