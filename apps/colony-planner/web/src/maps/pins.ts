/** A pin's spot on a map's art, as fractions of its width and height with y from the top (spec §4.4). */
export type PinPoint = { x: number; y: number };

/** A map's art at its natural size in pixels; the image map draws it one pixel per unit at zoom 0. */
export type ArtSize = { width: number; height: number };

type LatLngLike = { lat: number; lng: number };

export function clampUnit(value: number): number {
	if (Number.isNaN(value)) {
		return 0;
	}
	return Math.min(1, Math.max(0, value));
}

// Leaflet's CRS.Simple counts latitude upward, so the art spans latitude 0 at its bottom edge to its
// height at the top, while pins count y downward from the top edge.
export function artBounds(size: ArtSize): [[number, number], [number, number]] {
	return [
		[0, 0],
		[size.height, size.width],
	];
}

export function pinToLatLng(point: PinPoint, size: ArtSize): [number, number] {
	return [(1 - point.y) * size.height, point.x * size.width];
}

/** Where a tap or a dropped pin lands on the art, kept inside its edges. */
export function latLngToPin(latLng: LatLngLike, size: ArtSize): PinPoint {
	return {
		x: clampUnit(latLng.lng / size.width),
		y: clampUnit(1 - latLng.lat / size.height),
	};
}

/** A tap on the margin around the art places nothing. */
export function isOnArt(latLng: LatLngLike, size: ArtSize): boolean {
	return (
		latLng.lat >= 0 && latLng.lat <= size.height && latLng.lng >= 0 && latLng.lng <= size.width
	);
}
