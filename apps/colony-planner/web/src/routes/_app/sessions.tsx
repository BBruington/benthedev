import { createFileRoute } from "@tanstack/react-router";
import { ComingSoon } from "@/components/shell/coming-soon";

export const Route = createFileRoute("/_app/sessions")({
	component: () => <ComingSoon itemId="sessions" />,
});
