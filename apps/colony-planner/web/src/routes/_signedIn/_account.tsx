import { createFileRoute, Link, Outlet } from "@tanstack/react-router";
import { Crest } from "@/components/crest";
import { FullScreenError } from "@/components/error-screens";
import { ProfileMenu } from "@/components/shell/profile-menu";
import { useAccount } from "@/hooks/use-me";

// The pages outside every campaign (docs/design/multi-campaign.md §5.1): the picker, /request,
// /join and /admin. They have no stream; their reads refetch when the window regains focus.
export const Route = createFileRoute("/_signedIn/_account")({
	component: AccountLayout,
	errorComponent: FullScreenError,
});

function AccountLayout() {
	const account = useAccount();
	return (
		<div className="min-h-dvh">
			<a
				className="sr-only z-50 rounded-md bg-primary px-4 py-2 text-primary-foreground focus:not-sr-only focus:fixed focus:top-3 focus:left-3"
				href="#main"
			>
				Skip to content
			</a>
			<header className="sticky top-0 z-40 border-border border-b bg-bg/88 pt-[env(safe-area-inset-top)] backdrop-blur-md supports-[backdrop-filter]:bg-bg/72">
				<div className="mx-auto flex h-14 max-w-5xl items-center gap-2 pr-1.5 pl-3 md:h-16 md:pr-4 md:pl-5">
					<Link
						className="-ml-1 flex min-h-11 min-w-0 items-center gap-2.5 rounded-md px-1 outline-none focus-visible:ring-[3px] focus-visible:ring-ring/45"
						to="/"
					>
						<Crest className="size-7 text-gilt sm:size-8 md:size-9" />
						<span className="truncate font-display font-semibold text-base text-gilt-ink tracking-wide sm:text-lg md:text-xl">
							Colony Planner
						</span>
					</Link>
					<div className="ml-auto">
						<ProfileMenu account={account} />
					</div>
				</div>
			</header>
			<main
				className="mx-auto w-full max-w-5xl px-4 pt-6 pb-16 outline-none md:px-8 md:pt-10"
				id="main"
				tabIndex={-1}
			>
				<Outlet />
			</main>
		</div>
	);
}
