import { useId } from "react";

export function BrewingLoader() {
	const id = useId();
	const bulb = `${id}-bulb`;
	const wave = `${id}-wave`;
	const glass = `${id}-glass`;
	const surface = `${id}-surface`;
	const depth = `${id}-depth`;

	return (
		<output className="brewing-loader" aria-live="polite">
			<svg viewBox="0 0 120 144" aria-hidden="true">
				<defs>
					<clipPath id={bulb}>
						<path d="M49 30h22v20a40 40 0 1 1-22 0Z" />
					</clipPath>
					<path
						id={wave}
						d="M-80 82q20-6 40 0t40 0t40 0t40 0t40 0t40 0t40 0t40 0v60H-80Z"
					/>
					<clipPath id={surface}>
						<use className="brewing-loader-wave" href={`#${wave}`} />
					</clipPath>
					<linearGradient id={glass} x1="0" y1="0" x2="1" y2="1">
						<stop className="brewing-loader-tint" />
						<stop className="brewing-loader-tint-clear" offset="1" />
					</linearGradient>
					<linearGradient id={depth} x1="0" y1="0" x2="0" y2="1">
						<stop className="brewing-loader-depth-top" />
						<stop className="brewing-loader-depth-bottom" offset="1" />
					</linearGradient>
				</defs>
				<g className="brewing-loader-steam">
					<path d="M54 26c-8-7 6-10 0-18" />
					<path d="M65 25c8-8-5-11 1-19" />
				</g>
				<path fill={`url(#${glass})`} d="M49 30h22v20a40 40 0 1 1-22 0Z" />
				<g clipPath={`url(#${bulb})`}>
					<g className="brewing-loader-wave">
						<use className="brewing-loader-liquid" href={`#${wave}`} />
						<use
							className="brewing-loader-liquid brewing-loader-liquid-primal"
							href={`#${wave}`}
						/>
						<use
							className="brewing-loader-liquid brewing-loader-liquid-occult"
							href={`#${wave}`}
						/>
						<use
							className="brewing-loader-liquid brewing-loader-liquid-divine"
							href={`#${wave}`}
						/>
						<use href={`#${wave}`} fill={`url(#${depth})`} />
					</g>
					<g className="brewing-loader-bubbles" clipPath={`url(#${surface})`}>
						<circle cx="36" cy="106" r="2" />
						<circle cx="48" cy="115" r="3" />
						<circle cx="61" cy="109" r="1.6" />
						<circle cx="76" cy="112" r="2.5" />
						<circle cx="87" cy="99" r="1.7" />
						<circle cx="55" cy="99" r="2.1" />
						<circle cx="70" cy="120" r="1.4" />
					</g>
					<g className="brewing-loader-rings">
						<ellipse cx="48" cy="83" rx="3" ry="1.2" />
						<ellipse cx="76" cy="82" rx="2.5" ry="1" />
					</g>
				</g>
				<path
					className="brewing-loader-glass"
					fill="none"
					d="M49 30h22v20a40 40 0 1 1-22 0Z"
				/>
				<path
					className="brewing-loader-highlight"
					d="M40 61a33 33 0 0 0-10 35"
				/>
				<path className="brewing-loader-rim" d="M46 29h28M47 33h26" />
			</svg>
			<span>Opening your workshop…</span>
		</output>
	);
}
