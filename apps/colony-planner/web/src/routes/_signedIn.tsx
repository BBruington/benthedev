import { createFileRoute, Outlet, redirect } from "@tanstack/react-router";
import { useEffect } from "react";
import { FullScreenError } from "@/components/error-screens";
import { loadAccount } from "@/lib/access";
import { markAppLanded } from "@/lib/last-campaign";
import { rememberReturnTo } from "@/lib/return-to";

// Every screen behind sign-in (docs/design/multi-campaign.md §5.1): visitors go to /login, and come
// back here afterwards. What a signed-in account may see inside a campaign is decided below, per
// campaign.
export const Route = createFileRoute("/_signedIn")({
	beforeLoad: async ({ context, location }) => {
		const account = await loadAccount(context.queryClient, "cached");
		if (!account) {
			rememberReturnTo(location.href);
			throw redirect({ replace: true, to: "/login" });
		}
	},
	component: SignedInLayout,
	errorComponent: FullScreenError,
});

function SignedInLayout() {
	useEffect(() => {
		markAppLanded();
	}, []);
	return <Outlet />;
}
