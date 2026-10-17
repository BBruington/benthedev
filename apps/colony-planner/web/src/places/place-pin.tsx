import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowRightIcon, MapIcon } from "lucide-react";
import type { Schemas } from "@/api/client";
import { mapQuery } from "@/api/queries";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useCampaign } from "@/hooks/use-campaign";
import { artSource } from "@/maps/art";
import { PinGlyph } from "@/maps/pin-art";

/**
 * Where a place is pinned (spec §6.2): the map's art with the pin on it, linking to the map. Players
 * get a pin only while its map is visible to them; the DM also hears how to pin an unpinned place.
 */
export function PlacePinSection({ place, isDm }: { place: Schemas["Place"]; isDm: boolean }) {
	const { slug } = useCampaign();
	if (!place.pin) {
		if (!isDm) {
			return null;
		}
		return (
			<section aria-labelledby="place-pin-heading" className="flex flex-col gap-3">
				<h2 className="font-display font-semibold text-2xl tracking-wide" id="place-pin-heading">
					On the map
				</h2>
				<div className="flex flex-col gap-3 rounded-xl border border-border border-dashed px-4 py-4 sm:flex-row sm:items-center sm:justify-between">
					<p className="text-muted-foreground">
						Not pinned. Open a map, choose Edit pins, and tap where {place.name} is.
					</p>
					<Button asChild className="shrink-0" variant="outline">
						<Link params={{ campaignSlug: slug }} to="/c/$campaignSlug/maps">
							<MapIcon />
							Maps
						</Link>
					</Button>
				</div>
			</section>
		);
	}
	return <PinnedOn pin={place.pin} placeName={place.name} />;
}

function PinnedOn({
	pin,
	placeName,
}: {
	pin: NonNullable<Schemas["Place"]["pin"]>;
	placeName: string;
}) {
	const { campaignId, slug } = useCampaign();
	const map = useQuery(mapQuery(campaignId, pin.mapId));
	return (
		<section aria-labelledby="place-pin-heading" className="flex flex-col gap-3">
			<h2 className="font-display font-semibold text-2xl tracking-wide" id="place-pin-heading">
				On the map
			</h2>
			<div className="flex max-w-2xl flex-col gap-3">
				{map.data ? (
					<div
						className="relative w-full overflow-hidden rounded-xl border border-border bg-surface-sunken shadow-card"
						style={{ aspectRatio: `${map.data.width} / ${map.data.height}` }}
					>
						<img
							alt={`${placeName} pinned on ${pin.mapName}`}
							className="absolute inset-0 size-full object-cover"
							src={artSource(campaignId, map.data)}
						/>
						<span
							className="absolute flex -translate-x-1/2 -translate-y-full"
							style={{ left: `${pin.x * 100}%`, top: `${pin.y * 100}%` }}
						>
							<PinGlyph className="h-11 w-[2.125rem] drop-shadow-sm" />
						</span>
					</div>
				) : null}
				{map.isPending ? <Skeleton className="aspect-video w-full rounded-xl" /> : null}
				<Button asChild className="self-start" variant="outline">
					<Link params={{ campaignSlug: slug, mapId: pin.mapId }} to="/c/$campaignSlug/maps/$mapId">
						Open {pin.mapName}
						<ArrowRightIcon />
					</Link>
				</Button>
			</div>
		</section>
	);
}
