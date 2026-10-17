import type { QueryClient } from "@tanstack/react-query";
import { createRootRouteWithContext, Outlet } from "@tanstack/react-router";
import { AppToaster } from "@/components/app-toaster";
import { FullScreenError, NotFoundScreen } from "@/components/error-screens";

export type RouterContext = {
	queryClient: QueryClient;
};

export const Route = createRootRouteWithContext<RouterContext>()({
	component: RootLayout,
	errorComponent: FullScreenError,
	notFoundComponent: NotFoundScreen,
});

function RootLayout() {
	return (
		<>
			<Outlet />
			<AppToaster />
		</>
	);
}
