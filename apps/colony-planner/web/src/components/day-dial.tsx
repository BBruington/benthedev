import { cn } from "@/lib/utils";

const tickCount = 48;
const ticks = Array.from({ length: tickCount }, (_, index) => {
	const angle = (index / tickCount) * Math.PI * 2;
	const isMajor = index % 4 === 0;
	const outer = 92;
	const inner = isMajor ? 83 : 87;
	return {
		isMajor,
		x1: 100 + Math.sin(angle) * inner,
		x2: 100 + Math.sin(angle) * outer,
		y1: 100 - Math.cos(angle) * inner,
		y2: 100 - Math.cos(angle) * outer,
	};
});

const numberFormat = new Intl.NumberFormat();

/**
 * The colony's day clock (spec §6.6). Static for now: with no day it shows an unlit dial. When the
 * colony arrives (M1) it is given the day and, if ships are scheduled, the share of the voyage done.
 */
export function DayDial({
	day,
	shipmentProgress,
	caption,
	className,
}: {
	day: number | null;
	/** 0 to 1: how far the next ship has come since the last one. */
	shipmentProgress?: number;
	caption?: string;
	className?: string;
}) {
	const isLit = day !== null;
	const progress = Math.min(1, Math.max(0, shipmentProgress ?? 0));
	const arcRadius = 76;
	const circumference = 2 * Math.PI * arcRadius;
	return (
		<figure className={cn("flex flex-col items-center gap-3", className)}>
			<div className="relative aspect-square w-44 sm:w-52">
				<svg
					aria-hidden="true"
					className="absolute inset-0 size-full"
					fill="none"
					viewBox="0 0 200 200"
					xmlns="http://www.w3.org/2000/svg"
				>
					<circle
						className="fill-surface-sunken/60"
						cx="100"
						cy="100"
						r="96"
						stroke="var(--gilt)"
						strokeOpacity="0.35"
					/>
					{ticks.map((tick) => (
						<line
							key={`${tick.x1}-${tick.y1}`}
							stroke="var(--gilt)"
							strokeLinecap="round"
							strokeOpacity={tick.isMajor ? 0.75 : 0.35}
							strokeWidth={tick.isMajor ? 1.6 : 1}
							x1={tick.x1}
							x2={tick.x2}
							y1={tick.y1}
							y2={tick.y2}
						/>
					))}
					<circle
						cx="100"
						cy="100"
						r={arcRadius}
						stroke="var(--ink-muted)"
						strokeDasharray={isLit ? undefined : "2 6"}
						strokeOpacity="0.3"
						strokeWidth="5"
					/>
					{isLit && progress > 0 ? (
						<circle
							cx="100"
							cy="100"
							r={arcRadius}
							stroke="var(--gilt)"
							strokeDasharray={`${circumference * progress} ${circumference}`}
							strokeLinecap="round"
							strokeWidth="5"
							transform="rotate(-90 100 100)"
						/>
					) : null}
					<path d="M100 2 104 10 100 18 96 10Z" fill="var(--gilt)" />
					<circle cx="100" cy="100" r="62" stroke="var(--gilt)" strokeOpacity="0.2" />
				</svg>
				<div className="absolute inset-0 flex flex-col items-center justify-center">
					<span className="font-medium text-muted-foreground text-xs uppercase tracking-[0.3em]">
						Day
					</span>
					<span
						className={cn(
							"font-display font-semibold text-5xl tabular-nums leading-tight tracking-wide",
							isLit ? "text-gilt-ink" : "text-muted-foreground/70",
						)}
					>
						{isLit ? numberFormat.format(day) : "—"}
					</span>
				</div>
			</div>
			{caption ? (
				<figcaption className="text-center text-muted-foreground text-sm">{caption}</figcaption>
			) : null}
			<span className="sr-only">{isLit ? `Day ${day}` : "The day clock hasn't started yet"}</span>
		</figure>
	);
}
