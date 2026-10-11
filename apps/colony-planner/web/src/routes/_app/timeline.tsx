import { createFileRoute } from "@tanstack/react-router";
import { ComingSoon } from "@/components/shell/coming-soon";

export const Route = createFileRoute("/_app/timeline")({
	component: () => <ComingSoon itemId="timeline" />,
});
