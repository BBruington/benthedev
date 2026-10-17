import { cn } from "@/lib/utils";

/** A hairline in gilt with a small diamond at its centre, the way the Eldari map frames its title. */
export function GiltRule({ className }: { className?: string }) {
	return (
		<div aria-hidden="true" className={cn("flex items-center gap-3", className)}>
			<span className="rule-gilt flex-1" />
			<span className="size-1.5 rotate-45 bg-gilt/70" />
			<span className="rule-gilt flex-1" />
		</div>
	);
}
