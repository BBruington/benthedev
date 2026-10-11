import "./styles.css";
import { MutationCache, QueryCache, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createRouter, RouterProvider } from "@tanstack/react-router";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { isUnauthorized, problemStatus } from "./api/problem";
import { PanelError } from "./components/error-screens";
import { Splash } from "./components/splash";
import { TooltipProvider } from "./components/ui/tooltip";
import { applyThemePreference, readThemePreference } from "./lib/theme";
import { routeTree } from "./routeTree.gen";

// A 401 outside navigation means the session ended underneath the page: signed out elsewhere, or
// removed by the DM (spec §2.1: "signed out at once"). A full page load leaves nothing behind.
// During navigation the route guards redirect on their own.
function handleSessionEnded(error: unknown) {
	if (!isUnauthorized(error) || router.state.status !== "idle") {
		return;
	}
	if (window.location.pathname !== "/login") {
		window.location.replace("/login");
	}
}

// Client errors won't change on a retry, and a 501 is a stub that isn't built yet.
function shouldRetry(failureCount: number, error: unknown): boolean {
	const status = problemStatus(error);
	if (status !== undefined && (status < 500 || status === 501)) {
		return false;
	}
	return failureCount < 2;
}

// Live updates arrive as invalidations over /api/stream (spec §8.8), so data stays fresh until the
// server says otherwise and nothing refetches on a timer or on focus.
const queryClient = new QueryClient({
	defaultOptions: {
		mutations: {
			retry: false,
		},
		queries: {
			refetchOnReconnect: false,
			refetchOnWindowFocus: false,
			retry: shouldRetry,
			staleTime: Number.POSITIVE_INFINITY,
		},
	},
	mutationCache: new MutationCache({ onError: handleSessionEnded }),
	queryCache: new QueryCache({ onError: handleSessionEnded }),
});

const router = createRouter({
	context: { queryClient },
	defaultErrorComponent: PanelError,
	defaultPendingComponent: Splash,
	defaultPreload: "intent",
	// The query cache already holds what loaders fetch, so the router keeps none of its own.
	defaultPreloadStaleTime: 0,
	routeTree,
	scrollRestoration: true,
});

declare module "@tanstack/react-router" {
	interface Register {
		router: typeof router;
	}
}

applyThemePreference(readThemePreference());

const rootElement = document.getElementById("root");
if (!rootElement) {
	throw new Error("index.html is missing its #root element");
}

createRoot(rootElement).render(
	<StrictMode>
		<QueryClientProvider client={queryClient}>
			<TooltipProvider>
				<RouterProvider router={router} />
			</TooltipProvider>
		</QueryClientProvider>
	</StrictMode>,
);
