import { type LiveState, useLiveState } from "@/api/stream";
import { cn } from "@/lib/utils";

const stateLabels: Record<LiveState, string> = {
	connecting: "Connecting",
	live: "Live",
	reconnecting: "Reconnecting",
};

const stateDescriptions: Record<LiveState, string> = {
	connecting: "Connecting to live updates",
	live: "Live: changes appear as they happen",
	reconnecting: "Reconnecting to live updates",
};

/** Green while the live stream is open, grey while it connects or reconnects (spec §6.1). */
export function LiveDot({
	className,
	labelClassName,
}: {
	className?: string;
	/** Classes for the visible word; it is always read out to screen readers. */
	labelClassName?: string;
}) {
	const state = useLiveState();
	const isLive = state === "live";
	return (
		<span
			className={cn("inline-flex items-center gap-2", className)}
			data-state={state}
			role="status"
			title={stateDescriptions[state]}
		>
			<span aria-hidden="true" className="relative inline-flex size-2.5">
				{isLive ? (
					<span className="absolute inset-0 animate-live-ring rounded-full bg-moss" />
				) : null}
				<span
					className={cn(
						"relative inline-flex size-2.5 rounded-full transition-colors duration-300",
						isLive ? "bg-moss shadow-[0_0_0_2px_var(--bg)]" : "bg-ink-muted/55",
					)}
				/>
			</span>
			<span
				className={cn(
					"font-medium text-xs uppercase tracking-[0.14em]",
					isLive ? "text-moss-ink" : "text-muted-foreground",
					labelClassName,
				)}
			>
				{stateLabels[state]}
			</span>
		</span>
	);
}
