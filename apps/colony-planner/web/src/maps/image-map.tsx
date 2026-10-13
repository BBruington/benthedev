import "leaflet/dist/leaflet.css";
import "./image-map.css";
import "./zoom-control.css";
import { CRS, divIcon, type Map as LeafletMap, type Marker as LeafletMarker } from "leaflet";
import { type Ref, useEffect, useMemo, useRef } from "react";
import { ImageOverlay, MapContainer, Marker, useMap, useMapEvents } from "react-leaflet";
import type { Schemas } from "@/api/client";
import { cn } from "@/lib/utils";
import { pinMarkup } from "./pin-art";
import { type ArtSize, artBounds, isOnArt, latLngToPin, type PinPoint, pinToLatLng } from "./pins";

type MapPin = Schemas["MapPin"];

// The pin's bottom centre is the pinned spot, inside a 44 px target.
const pinIcon = divIcon({
	className: "map-pin",
	html: pinMarkup,
	iconAnchor: [22, 48],
	iconSize: [44, 48],
});

// How far past the art the view may drift, as a share of its size, so edge pins clear the controls.
const panMargin = 0.15;
const maxZoom = 2;
// Far enough out for any art up to the server's size limits on a phone.
const lowestZoom = -8;

/**
 * A map's art at its natural size on Leaflet's CRS.Simple (spec §8.3), with its pins. Tapping a pin
 * activates it; while editing, pins drag and a tap on the art reports where, as 0..1 fractions.
 */
export function ImageMap({
	artUrl,
	width,
	height,
	pins,
	editing,
	label,
	onPinActivate,
	onPinMoved,
	onArtTap,
	mapRef,
}: {
	artUrl: string;
	width: number;
	height: number;
	pins: MapPin[];
	editing: boolean;
	label: string;
	onPinActivate: (pin: MapPin) => void;
	onPinMoved?: (pin: MapPin, point: PinPoint) => void;
	onArtTap?: (point: PinPoint) => void;
	mapRef?: Ref<LeafletMap>;
}) {
	const size = useMemo(() => ({ height, width }), [height, width]);
	const bounds = artBounds(size);
	const maxBounds: [[number, number], [number, number]] = [
		[-height * panMargin, -width * panMargin],
		[height * (1 + panMargin), width * (1 + panMargin)],
	];
	return (
		<section aria-label={label} className="h-full w-full">
			<MapContainer
				attributionControl={false}
				bounds={bounds}
				className={cn("image-map h-full w-full", editing && "is-editing")}
				crs={CRS.Simple}
				key={`${width}x${height}`}
				maxBounds={maxBounds}
				maxBoundsViscosity={1}
				maxZoom={maxZoom}
				minZoom={lowestZoom}
				ref={mapRef}
				zoomDelta={0.5}
				zoomSnap={0.25}
			>
				<ImageOverlay bounds={bounds} url={artUrl} />
				<FitArt height={height} width={width} />
				{editing && onArtTap ? <ArtTaps onTap={onArtTap} size={size} /> : null}
				{pins.map((pin) => (
					<PinMarker
						draggable={editing && onPinMoved !== undefined}
						// Leaflet sets a marker's title once, so a renamed place gets a new marker.
						key={`${pin.placeId}:${pin.placeName}`}
						onActivate={onPinActivate}
						onMoved={onPinMoved}
						pin={pin}
						size={size}
					/>
				))}
			</MapContainer>
		</section>
	);
}

/** Starts with the whole art in view, and never lets it zoom out much past that, at any size. */
function FitArt({ width, height }: { width: number; height: number }) {
	const map = useMap();
	useEffect(() => {
		const bounds = artBounds({ height, width });
		function limitZoomOut() {
			map.invalidateSize();
			// getBoundsZoom never answers below the current minimum, so lift that first.
			map.setMinZoom(lowestZoom);
			map.setMinZoom(map.getBoundsZoom(bounds) - 0.5);
		}
		limitZoomOut();
		map.fitBounds(bounds);
		const observer = new ResizeObserver(limitZoomOut);
		observer.observe(map.getContainer());
		return () => {
			observer.disconnect();
		};
	}, [map, width, height]);
	return null;
}

function ArtTaps({ size, onTap }: { size: ArtSize; onTap: (point: PinPoint) => void }) {
	useMapEvents({
		click: (event) => {
			if (isOnArt(event.latlng, size)) {
				onTap(latLngToPin(event.latlng, size));
			}
		},
	});
	return null;
}

function PinMarker({
	pin,
	size,
	draggable,
	onActivate,
	onMoved,
}: {
	pin: MapPin;
	size: ArtSize;
	draggable: boolean;
	onActivate: (pin: MapPin) => void;
	onMoved?: (pin: MapPin, point: PinPoint) => void;
}) {
	const markerRef = useRef<LeafletMarker>(null);
	const position = useMemo(() => pinToLatLng(pin, size), [pin, size]);
	return (
		<Marker
			draggable={draggable}
			eventHandlers={{
				click: () => {
					onActivate(pin);
				},
				dragend: () => {
					const marker = markerRef.current;
					if (marker) {
						onMoved?.(pin, latLngToPin(marker.getLatLng(), size));
					}
				},
				// Leaflet gives its markers role="button" but turns no key into a click.
				keydown: (event) => {
					if (event.originalEvent.key === "Enter" || event.originalEvent.key === " ") {
						event.originalEvent.preventDefault();
						onActivate(pin);
					}
				},
			}}
			icon={pinIcon}
			keyboard
			position={position}
			ref={markerRef}
			title={pin.placeName}
		/>
	);
}
