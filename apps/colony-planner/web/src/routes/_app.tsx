import { createFileRoute, Outlet, redirect, useNavigate } from "@tanstack/react-router";
import { useEffect, useRef } from "react";
import { useLiveUpdates } from "@/api/stream";
import { useTimelineToasts } from "@/colony/live-toasts";
import { FullScreenError } from "@/components/error-screens";
import { BottomTabs } from "@/components/shell/bottom-tabs";
import { Sidebar } from "@/components/shell/sidebar";
import { TopBar } from "@/components/shell/top-bar";
import { usePendingCount } from "@/hooks/use-campaign";
import { useMe } from "@/hooks/use-me";
import { useShellHeaderHeight } from "@/hooks/use-shell-header-height";
import { loadMe } from "@/lib/access";
import { rememberReturnTo, takeReturnTo } from "@/lib/return-to";

// Every screen behind sign-in: visitors go to /login, and anyone the DM hasn't let in (or has
// removed) goes to /pending (spec §2.1).
export const Route = createFileRoute("/_app")({
	beforeLoad: async ({ context, location }) => {
		const me = await loadMe(context.queryClient, "cached");
		if (!me) {
			rememberReturnTo(location.href);
			throw redirect({ replace: true, to: "/login" });
		}
		if (me.status !== "ACTIVE") {
			throw redirect({ replace: true, to: "/pending" });
		}
		if (location.pathname === "/") {
			const returnTo = takeReturnTo();
			if (returnTo) {
				throw redirect({ href: returnTo, replace: true });
			}
		}
	},
	component: AppLayout,
	errorComponent: FullScreenError,
});

function AppLayout() {
	const me = useMe();
	const navigate = useNavigate();
	const headerRef = useRef<HTMLElement>(null);
	const pendingCount = usePendingCount(me);
	useLiveUpdates();
	useTimelineToasts();
	useShellHeaderHeight(headerRef);

	useEffect(() => {
		if (me.status !== "ACTIVE") {
			navigate({ replace: true, to: "/pending" });
		}
	}, [me.status, navigate]);

	return (
		<div className="min-h-dvh">
			<a
				className="sr-only z-50 rounded-md bg-primary px-4 py-2 text-primary-foreground focus:not-sr-only focus:fixed focus:top-3 focus:left-3"
				href="#main"
			>
				Skip to content
			</a>
			<TopBar me={me} ref={headerRef} />
			<div className="md:flex">
				<Sidebar className="hidden md:block" me={me} pendingCount={pendingCount} />
				<main
					className="min-w-0 flex-1 px-4 pt-6 pb-[calc(6.5rem+env(safe-area-inset-bottom))] outline-none md:px-8 md:pt-10 md:pb-16 lg:px-12"
					id="main"
					tabIndex={-1}
				>
					<div className="mx-auto w-full max-w-5xl">
						<Outlet />
					</div>
				</main>
			</div>
			<BottomTabs className="md:hidden" me={me} pendingCount={pendingCount} />
		</div>
	);
}
