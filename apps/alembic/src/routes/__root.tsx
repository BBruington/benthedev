import type { QueryClient } from "@tanstack/react-query";
import {
	createRootRouteWithContext,
	HeadContent,
	Scripts,
} from "@tanstack/react-router";
import Footer from "../components/Footer";
import Header from "../components/Header";
import { Toaster } from "../components/ui/sonner";
import { TooltipProvider } from "../components/ui/tooltip";
import ClerkProvider from "../integrations/clerk/provider";
import appCss from "../styles.css?url";

interface MyRouterContext {
	queryClient: QueryClient;
}

export const Route = createRootRouteWithContext<MyRouterContext>()({
	head: () => ({
		meta: [
			{ charSet: "utf-8" },
			{ name: "viewport", content: "width=device-width, initial-scale=1" },
			{ title: "Alembic" },
			{
				name: "description",
				content: "A potion-brewing workshop for Pathfinder Second Edition.",
			},
		],
		links: [
			{ rel: "stylesheet", href: appCss },
			{ rel: "icon", type: "image/svg+xml", href: "/favicon.svg" },
		],
	}),
	shellComponent: RootDocument,
});

function RootDocument({ children }: { children: React.ReactNode }) {
	return (
		<html lang="en" className="dark">
			<head>
				<HeadContent />
			</head>
			<body>
				<ClerkProvider>
					<TooltipProvider>
						<Header />
						{children}
						<Footer />
						<Toaster theme="dark" />
					</TooltipProvider>
				</ClerkProvider>
				<Scripts />
			</body>
		</html>
	);
}
