import type { Schemas } from "@/api/client";
import { imageUrl } from "@/api/images";

/** Where a map's art loads from: its uploaded image, or the https address the DM gave instead. */
export function artSource(
	campaignId: string,
	map: Pick<Schemas["WorldMap"], "externalUrl" | "imageId">,
): string {
	if (map.imageId) {
		return imageUrl(campaignId, map.imageId);
	}
	return map.externalUrl;
}
