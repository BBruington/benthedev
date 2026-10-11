import { createFileRoute } from "@tanstack/react-router";
import { ComingSoon } from "@/components/shell/coming-soon";

export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell/quests")({
	component: () => <ComingSoon itemId="quests" />,
});
