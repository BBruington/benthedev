import type { LucideIcon } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { cn } from "@/lib/utils";

type Tone = "gilt" | "verdigris" | "moss" | "ember" | "ink";

const toneClasses: Record<Tone, { icon: string; value: string }> = {
	ember: { icon: "bg-ember/12 text-ember-ink", value: "text-foreground" },
	gilt: { icon: "bg-gilt/12 text-gilt-ink", value: "text-foreground" },
	ink: { icon: "bg-muted text-muted-foreground", value: "text-foreground" },
	moss: { icon: "bg-moss/12 text-moss-ink", value: "text-foreground" },
	verdigris: { icon: "bg-verdigris/12 text-verdigris-ink", value: "text-foreground" },
};

const chipVisibleForMilliseconds = 15_000;
const countDurationInMilliseconds = 400;
const numberFormat = new Intl.NumberFormat();

function prefersReducedMotion(): boolean {
	return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

/** Counts from the previous value to the new one (spec §6.3: 400 ms, instant under reduced motion). */
function useCountedValue(target: number): number {
	const [displayed, setDisplayed] = useState(target);
	const displayedRef = useRef(target);
	useEffect(() => {
		const from = displayedRef.current;
		if (from === target) {
			return;
		}
		if (prefersReducedMotion()) {
			displayedRef.current = target;
			setDisplayed(target);
			return;
		}
		const startedAt = performance.now();
		let frame = 0;
		const step = (now: number) => {
			const progress = Math.min(1, (now - startedAt) / countDurationInMilliseconds);
			const eased = 1 - (1 - progress) ** 3;
			const current = Math.round(from + (target - from) * eased);
			displayedRef.current = current;
			setDisplayed(current);
			if (progress < 1) {
				frame = requestAnimationFrame(step);
			}
		};
		frame = requestAnimationFrame(step);
		return () => {
			cancelAnimationFrame(frame);
		};
	}, [target]);
	return displayed;
}

/** The change from the previous value, shown for 15 s; the client computes it (spec §6.3). */
function useDelta(value: number): { amount: number; id: number } | null {
	const previousValue = useRef(value);
	const [delta, setDelta] = useState<{ amount: number; id: number } | null>(null);
	useEffect(() => {
		const amount = value - previousValue.current;
		previousValue.current = value;
		if (amount === 0) {
			return;
		}
		setDelta((current) => ({ amount, id: (current?.id ?? 0) + 1 }));
		const timer = setTimeout(() => {
			setDelta(null);
		}, chipVisibleForMilliseconds);
		return () => {
			clearTimeout(timer);
		};
	}, [value]);
	return delta;
}

function formatDelta(amount: number): string {
	const magnitude = numberFormat.format(Math.abs(amount));
	return amount > 0 ? `+${magnitude}` : `−${magnitude}`;
}

export function StatTile({
	label,
	value,
	icon: Icon,
	tone = "gilt",
	caption,
	className,
}: {
	label: string;
	value: number;
	icon?: LucideIcon;
	tone?: Tone;
	caption?: string;
	className?: string;
}) {
	const displayed = useCountedValue(value);
	const delta = useDelta(value);
	const tones = toneClasses[tone];
	return (
		<div
			className={cn(
				"relative flex min-w-0 flex-col gap-3 rounded-xl border border-border bg-card p-3.5 shadow-card sm:p-4",
				className,
			)}
			data-slot="stat-tile"
		>
			<div className="flex items-center justify-between gap-2">
				<p className="truncate font-medium text-muted-foreground text-xs uppercase tracking-[0.1em] sm:text-sm sm:tracking-[0.12em]">
					{label}
				</p>
				{Icon ? (
					<span
						aria-hidden="true"
						className={cn(
							"hidden size-8 shrink-0 items-center justify-center rounded-lg sm:inline-flex",
							tones.icon,
						)}
					>
						<Icon className="size-4" />
					</span>
				) : null}
			</div>
			<div className="flex items-baseline gap-2">
				<p className={cn("font-bold font-sans text-3xl tabular-nums leading-none", tones.value)}>
					<span aria-hidden="true">{numberFormat.format(displayed)}</span>
					<span className="sr-only">{numberFormat.format(value)}</span>
				</p>
				{delta ? (
					<span
						className={cn(
							"inline-flex animate-chip-in items-center rounded-full px-2 py-0.5 font-semibold text-xs tabular-nums",
							delta.amount > 0 ? "bg-moss/15 text-moss-ink" : "bg-ember/15 text-ember-ink",
						)}
						key={delta.id}
					>
						{formatDelta(delta.amount)}
					</span>
				) : null}
			</div>
			{caption ? <p className="text-muted-foreground text-sm">{caption}</p> : null}
		</div>
	);
}
