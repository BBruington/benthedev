import { Crest } from "@/components/crest";

/** Shown while a route guard is still asking the server who this is. */
export function Splash() {
	return (
		<div className="flex min-h-dvh flex-col items-center justify-center gap-4" role="status">
			<Crest className="size-14 animate-pulse text-gilt" />
			<p className="font-display text-muted-foreground text-sm uppercase tracking-[0.3em]">
				Charting
			</p>
			<span className="sr-only">Loading</span>
		</div>
	);
}
