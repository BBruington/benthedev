import type { Schemas } from "@/api/client";
import { ColonyIcon } from "@/components/colony-icon";
import { formatDelta, formatNumber } from "@/lib/colony";
import { cn } from "@/lib/utils";

type ResourceChange = Schemas["ResourceChangePreview"];

/** "(capped)" when the maximum stopped a gain, "(at the minimum)" when the floor stopped a loss. */
function boundNote(change: ResourceChange): string | null {
	if (!change.clamped) {
		return null;
	}
	const applied = change.after - change.before;
	return change.requested > applied ? "capped" : "at the minimum";
}

/** Before → after for each resource a preview names, with the bound that stopped it (spec §5.4). */
export function ResourceChangeList({ changes }: { changes: readonly ResourceChange[] }) {
	if (changes.length === 0) {
		return null;
	}
	return (
		<ul className="grid gap-1.5">
			{changes.map((change) => {
				const applied = change.after - change.before;
				const note = boundNote(change);
				return (
					<li
						className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-lg bg-surface-sunken/60 px-3 py-2"
						key={change.key}
					>
						<span className="flex min-w-0 flex-1 items-center gap-2 font-medium">
							<ColonyIcon className="size-4 shrink-0 text-gilt-ink" name={change.icon} />
							<span className="truncate">{change.label}</span>
						</span>
						<span className="tabular-nums">
							{formatNumber(change.before)}
							<span aria-hidden="true" className="mx-1.5 text-muted-foreground">
								→
							</span>
							<span className="sr-only"> becomes </span>
							<span className="font-semibold">{formatNumber(change.after)}</span>
						</span>
						<span
							className={cn(
								"rounded-full px-2 py-0.5 font-semibold text-xs tabular-nums",
								applied >= 0 ? "bg-moss/15 text-moss-ink" : "bg-ember/15 text-ember-ink",
							)}
						>
							{formatDelta(applied)}
						</span>
						{note ? <span className="w-full text-gilt-ink text-sm sm:w-auto">({note})</span> : null}
					</li>
				);
			})}
		</ul>
	);
}
