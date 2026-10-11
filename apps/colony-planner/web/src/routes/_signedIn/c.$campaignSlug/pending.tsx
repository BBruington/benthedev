import { createFileRoute, Link, redirect, useNavigate } from "@tanstack/react-router";
import {
	DoorOpenIcon,
	HourglassIcon,
	LayoutGridIcon,
	LogOutIcon,
	PauseCircleIcon,
	ShieldXIcon,
} from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { campaignMeQuery } from "@/api/queries";
import { EldariBackdrop } from "@/components/eldari-backdrop";
import { FullScreenError } from "@/components/error-screens";
import { GiltRule } from "@/components/gilt-rule";
import { LiveDot } from "@/components/live-dot";
import { MemberAvatar } from "@/components/member-avatar";
import { LeaveCampaignDialog } from "@/components/shell/leave-campaign-dialog";
import { Button } from "@/components/ui/button";
import { useCampaign } from "@/hooks/use-campaign";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { useMe } from "@/hooks/use-me";
import { isLetIn, type Me } from "@/lib/access";
import { signOutAndLeave } from "@/lib/sign-out";
import { cn } from "@/lib/utils";

// Inside a campaign, but not let in (docs/design/multi-campaign.md §5.1): waiting for the DM,
// removed (or left), or the campaign is paused by the site owner. The campaign's stream carries this
// membership's "me" topic, so approval, re-admission and unsuspending open the colony without a
// refresh.
export const Route = createFileRoute("/_signedIn/c/$campaignSlug/pending")({
	beforeLoad: async ({ context, params }) => {
		if (!context.campaignId) {
			return;
		}
		const me = await context.queryClient.fetchQuery({
			...campaignMeQuery(context.campaignId),
			staleTime: 0,
		});
		if (isLetIn(me)) {
			throw redirect({
				params: { campaignSlug: params.campaignSlug },
				replace: true,
				to: "/c/$campaignSlug",
			});
		}
	},
	component: PendingPage,
	errorComponent: FullScreenError,
});

type Door = "waiting" | "no-access" | "paused";

function doorFor(me: Me): Door {
	if (me.campaign.status === "SUSPENDED") {
		return "paused";
	}
	if (me.status === "PENDING") {
		return "waiting";
	}
	return "no-access";
}

const doorTitles: Record<Door, string> = {
	"no-access": "No access",
	paused: "Paused",
	waiting: "Waiting for the DM",
};

function PendingPage() {
	const me = useMe();
	const { slug } = useCampaign();
	const navigate = useNavigate();
	const letIn = isLetIn(me);
	const door = doorFor(me);
	// What this page said before the door opened: unpausing reads differently from being let in.
	const lastDoor = useRef(door);
	useDocumentTitle(doorTitles[door], me.campaign.name);

	useEffect(() => {
		if (!letIn) {
			lastDoor.current = door;
		}
	}, [door, letIn]);

	useEffect(() => {
		if (letIn) {
			const message =
				lastDoor.current === "paused"
					? `${me.campaign.name} is open again.`
					: `You're in. Welcome to ${me.campaign.name}!`;
			toast.success(message);
			navigate({ params: { campaignSlug: slug }, replace: true, to: "/c/$campaignSlug" });
		}
	}, [letIn, me.campaign.name, navigate, slug]);

	return (
		<EldariBackdrop>
			<main className="w-full max-w-md">
				<section className="rounded-2xl border border-border bg-surface/88 px-6 pt-8 pb-6 text-center shadow-float backdrop-blur-md sm:px-9">
					<SignedInAs me={me} />
					<DoorMessage
						campaignName={me.campaign.name}
						dmDisplayName={me.dmDisplayName}
						door={door}
					/>
					<GiltRule className="mt-8 mb-5" />
					<div className="flex flex-col items-center gap-1 sm:flex-row sm:justify-center">
						<Button asChild variant="ghost">
							<Link to="/">
								<LayoutGridIcon />
								Your campaigns
							</Link>
						</Button>
						{door === "waiting" ? (
							<StopWaitingButton campaignId={me.campaign.id} campaignName={me.campaign.name} />
						) : null}
						<SignOutButton />
					</div>
				</section>
			</main>
		</EldariBackdrop>
	);
}

function DoorMessage({
	door,
	campaignName,
	dmDisplayName,
}: {
	door: Door;
	campaignName: string;
	dmDisplayName: string;
}) {
	if (door === "paused") {
		return <Paused campaignName={campaignName} />;
	}
	if (door === "waiting") {
		return <Waiting campaignName={campaignName} dmDisplayName={dmDisplayName} />;
	}
	return <NoAccess campaignName={campaignName} />;
}

function SignedInAs({ me }: { me: Me }) {
	return (
		<div className="mx-auto flex w-fit max-w-full items-center gap-2.5 rounded-full border border-border bg-card/70 py-1 pr-4 pl-1">
			<MemberAvatar avatarUrl={me.member.avatarUrl} name={me.member.displayName} size="sm" />
			<span className="truncate text-muted-foreground text-sm">
				Signed in as <span className="font-medium text-foreground">{me.member.displayName}</span>
			</span>
		</div>
	);
}

function Waiting({ campaignName, dmDisplayName }: { campaignName: string; dmDisplayName: string }) {
	return (
		<>
			<Emblem className="border-gilt/40 text-gilt-ink" glow="bg-gilt/15">
				<HourglassIcon className="size-9 animate-hourglass" strokeWidth={1.5} />
			</Emblem>
			<h1 className="text-balance font-display font-semibold text-2xl text-gilt-ink tracking-wide sm:text-3xl">
				Waiting for {dmDisplayName} to let you in to {campaignName}
			</h1>
			<p className="mt-3 text-balance text-muted-foreground">
				{dmDisplayName}, the DM, knows you're here. This page opens on its own the moment they say
				yes, so you can leave it open or come back later.
			</p>
			<div className="mt-6 flex justify-center">
				<LiveDot className="rounded-full border border-border bg-card/70 px-3 py-1.5" />
			</div>
		</>
	);
}

function NoAccess({ campaignName }: { campaignName: string }) {
	return (
		<>
			<Emblem className="border-ember/45 text-ember-ink" glow="bg-ember/15">
				<ShieldXIcon className="size-9" strokeWidth={1.5} />
			</Emblem>
			<h1 className="text-balance font-display font-semibold text-2xl text-foreground tracking-wide sm:text-3xl">
				You don't have access to {campaignName}
			</h1>
			<p className="mt-3 text-balance text-muted-foreground">
				Only the DM can let this Discord account back in. If you think that's a mistake, ask them.
				Your other campaigns aren't affected.
			</p>
		</>
	);
}

function Paused({ campaignName }: { campaignName: string }) {
	return (
		<>
			<Emblem className="border-verdigris/45 text-verdigris-ink" glow="bg-verdigris/15">
				<PauseCircleIcon className="size-9" strokeWidth={1.5} />
			</Emblem>
			<h1 className="text-balance font-display font-semibold text-2xl text-foreground tracking-wide sm:text-3xl">
				{campaignName} is paused by the site owner
			</h1>
			<p className="mt-3 text-balance text-muted-foreground">
				Nothing has been deleted. This page opens the colony again on its own when the site owner
				unpauses it.
			</p>
			<div className="mt-6 flex justify-center">
				<LiveDot className="rounded-full border border-border bg-card/70 px-3 py-1.5" />
			</div>
		</>
	);
}

/** Leaving from the waiting page withdraws the request to join (C5). */
function StopWaitingButton({
	campaignId,
	campaignName,
}: {
	campaignId: string;
	campaignName: string;
}) {
	const [isOpen, setOpen] = useState(false);
	return (
		<>
			<Button
				onClick={() => {
					setOpen(true);
				}}
				variant="ghost"
			>
				<DoorOpenIcon />
				Stop waiting
			</Button>
			<LeaveCampaignDialog
				campaignId={campaignId}
				campaignName={campaignName}
				isWaiting
				onOpenChange={setOpen}
				open={isOpen}
			/>
		</>
	);
}

function Emblem({
	children,
	className,
	glow,
}: {
	children: ReactNode;
	className: string;
	glow: string;
}) {
	return (
		<div aria-hidden="true" className="relative mx-auto mt-8 mb-6 w-fit">
			<span className={cn("absolute -inset-3 rounded-full blur-xl", glow)} />
			<span
				className={cn(
					"relative inline-flex size-20 items-center justify-center rounded-full border bg-card shadow-card",
					className,
				)}
			>
				{children}
			</span>
		</div>
	);
}

function SignOutButton() {
	const [isSigningOut, setSigningOut] = useState(false);
	return (
		<Button
			disabled={isSigningOut}
			onClick={async () => {
				setSigningOut(true);
				await signOutAndLeave("this-device");
				setSigningOut(false);
			}}
			variant="ghost"
		>
			<LogOutIcon />
			Sign out
		</Button>
	);
}
