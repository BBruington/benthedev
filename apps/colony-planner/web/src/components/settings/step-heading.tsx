import type { ReactNode } from "react";

/** A numbered step's heading in a Settings card's setup list. */
export function StepHeading({ number, children }: { number: number; children: ReactNode }) {
	return (
		<h3 className="flex items-center gap-2.5 font-medium">
			<span
				aria-hidden="true"
				className="inline-flex size-6 shrink-0 items-center justify-center rounded-full bg-gilt/15 font-semibold text-gilt-ink text-xs"
			>
				{number}
			</span>
			<span className="sr-only">Step {number}: </span>
			{children}
		</h3>
	);
}
