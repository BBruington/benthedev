// The old app's pin, ported per spec §6.6: a dark-brown teardrop with a gold star. One drawing feeds
// both Leaflet's marker (an HTML string) and the place page's preview (React).
const teardropPath =
	"M14 0C6.268 0 0 6.268 0 14C0 24.5 14 36 14 36C14 36 28 24.5 28 14C28 6.268 21.732 0 14 0Z";
const starPath =
	"M14 7 L15.2 11.2 L19.5 11.2 L16 13.8 L17.2 18 L14 15.4 L10.8 18 L12 13.8 L8.5 11.2 L12.8 11.2 Z";

export const pinMarkup = `<svg aria-hidden="true" focusable="false" width="34" height="44" viewBox="0 0 28 36" xmlns="http://www.w3.org/2000/svg">
<ellipse cx="14" cy="34" rx="5" ry="2" fill="rgba(0,0,0,0.35)"/>
<path d="${teardropPath}" fill="#1a0e00" stroke="#a07830" stroke-width="1.5"/>
<circle cx="14" cy="13" r="8" fill="#3d2000" stroke="#c4922a" stroke-width="1.5"/>
<path d="${starPath}" fill="#f0c040"/>
</svg>`;

export function PinGlyph({ className }: { className?: string }) {
	return (
		<svg aria-hidden="true" className={className} focusable="false" viewBox="0 0 28 36">
			<ellipse cx="14" cy="34" fill="rgba(0,0,0,0.35)" rx="5" ry="2" />
			<path d={teardropPath} fill="#1a0e00" stroke="#a07830" strokeWidth="1.5" />
			<circle cx="14" cy="13" fill="#3d2000" r="8" stroke="#c4922a" strokeWidth="1.5" />
			<path d={starPath} fill="#f0c040" />
		</svg>
	);
}
