import { useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute, redirect, useNavigate } from "@tanstack/react-router";
import { HourglassIcon, LogOutIcon, ShieldXIcon } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";
import { toast } from "sonner";
import { meQuery } from "@/api/queries";
import { useLiveUpdates } from "@/api/stream";
import { EldariBackdrop } from "@/components/eldari-backdrop";
import { FullScreenError } from "@/components/error-screens";
import { GiltRule } from "@/components/gilt-rule";
import { LiveDot } from "@/components/live-dot";
import { MemberAvatar } from "@/components/member-avatar";
import { Button } from "@/components/ui/button";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { loadMe, type Me } from "@/lib/access";
import { signOutAndLeave } from "@/lib/sign-out";
import { cn } from "@/lib/utils";

// Signed in, but not let in: waiting for the DM, or removed or denied (spec §2.1). The live stream
// carries this member's "me" topic, so approval opens the app without a refresh.
export const Route = createFileRoute("/pending")({
	beforeLoad: async ({ context }) => {
		const me = await loadMe(context.queryClient, "fresh");
		if (!me) {
			throw redirect({ replace: true, to: "/login" });
		}
		if (me.status === "ACTIVE") {
			throw redirect({ replace: true, to: "/" });
		}
	},
	component: PendingPage,
	errorComponent: FullScreenError,
});

function PendingPage() {
	const me = useSuspenseQuery(meQuery).data;
	const navigate = useNavigate();
	useLiveUpdates();

	useEffect(() => {
		if (me.status === "ACTIVE") {
			toast.success("You're in. Welcome to the colony!");
			navigate({ replace: true, to: "/" });
		}
	}, [me.status, navigate]);

	const isRemoved = me.status === "REMOVED";
	useDocumentTitle(isRemoved ? "No access" : "Waiting for the DM", "Colony Planner");

	return (
		<EldariBackdrop>
			<main className="w-full max-w-md">
				<section className="rounded-2xl border border-border bg-surface/88 px-6 pt-8 pb-6 text-center shadow-float backdrop-blur-md sm:px-9">
					<SignedInAs me={me} />
					{isRemoved ? <NoAccess /> : <Waiting />}
					<GiltRule className="mt-8 mb-5" />
					<SignOutButton />
				</section>
			</main>
		</EldariBackdrop>
	);
}

function SignedInAs({ me }: { me: Me }) {
	return (
		<div className="mx-auto flex w-fit items-center gap-2.5 rounded-full border border-border bg-card/70 py-1 pr-4 pl-1">
			<MemberAvatar avatarUrl={me.member.avatarUrl} name={me.member.displayName} size="sm" />
			<span className="text-muted-foreground text-sm">
				Signed in as <span className="font-medium text-foreground">{me.member.displayName}</span>
			</span>
		</div>
	);
}

function Waiting() {
	return (
		<>
			<Emblem className="border-gilt/40 text-gilt-ink" glow="bg-gilt/15">
				<HourglassIcon className="size-9 animate-hourglass" strokeWidth={1.5} />
			</Emblem>
			<h1 className="text-balance font-display font-semibold text-2xl text-gilt-ink tracking-wide sm:text-3xl">
				Waiting for the DM to let you in
			</h1>
			<p className="mt-3 text-balance text-muted-foreground">
				The DM knows you're here. This page opens on its own the moment they say yes, so you can
				leave it open or come back later.
			</p>
			<div className="mt-6 flex justify-center">
				<LiveDot className="rounded-full border border-border bg-card/70 px-3 py-1.5" />
			</div>
		</>
	);
}

function NoAccess() {
	return (
		<>
			<Emblem className="border-ember/45 text-ember-ink" glow="bg-ember/15">
				<ShieldXIcon className="size-9" strokeWidth={1.5} />
			</Emblem>
			<h1 className="font-display font-semibold text-2xl text-foreground tracking-wide sm:text-3xl">
				You don't have access
			</h1>
			<p className="mt-3 text-balance text-muted-foreground">
				The DM hasn't let this Discord account into the campaign. If you think that's a mistake, ask
				the DM; only they can let you back in.
			</p>
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
