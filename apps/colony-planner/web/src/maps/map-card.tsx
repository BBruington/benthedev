import { Link } from "@tanstack/react-router";
import { StarIcon } from "lucide-react";
import type { ReactNode } from "react";
import type { Schemas } from "@/api/client";
import { Badge } from "@/components/ui/badge";
import { useCampaign } from "@/hooks/use-campaign";
import { HiddenBadge } from "@/people/people-sections";

/** One map on /maps: its art and name open it; the DM's controls sit underneath. */
export function MapCard({
	map,
	artUrl,
	children,
}: {
	map: Schemas["WorldMap"];
	artUrl: string;
	children?: ReactNode;
}) {
	const { slug } = useCampaign();
	return (
		<li className="flex flex-col overflow-hidden rounded-2xl border border-border bg-card shadow-card">
			<div className="relative transition-colors focus-within:ring-[3px] focus-within:ring-ring/40 focus-within:ring-inset hover:bg-accent/40">
				<img
					alt=""
					className="aspect-video w-full border-border border-b bg-surface-sunken object-cover"
					loading="lazy"
					src={artUrl}
				/>
				<div className="flex flex-wrap items-center gap-2 px-4 py-3">
					<h2 className="min-w-0 font-display font-semibold text-xl tracking-wide [overflow-wrap:anywhere]">
						<Link
							className="outline-none after:absolute after:inset-0"
							params={{ campaignSlug: slug, mapId: map.id }}
							to="/c/$campaignSlug/maps/$mapId"
						>
							{map.name}
						</Link>
					</h2>
					{map.isMainland ? (
						<Badge variant="gilt">
							<StarIcon />
							Mainland
						</Badge>
					) : null}
					<HiddenBadge item={map} />
				</div>
			</div>
			{children ? (
				<div className="grid gap-3 border-border border-t px-4 py-3">{children}</div>
			) : null}
		</li>
	);
}
