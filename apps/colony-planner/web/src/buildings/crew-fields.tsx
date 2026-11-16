import type { ReactNode } from "react";
import type { Schemas } from "@/api/client";
import { fieldInputProps } from "@/components/field";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { formatNumber } from "@/lib/colony";
import { type CrewLine, freeColonists, type NumberDraft } from "@/lib/construction";

type ConstructionRole = Schemas["ConstructionRole"];

/**
 * A number per labor line: how many of that role work on it. Each row says how many are free (the
 * idle ones plus this line's own crew) and the line's cap, so nobody has to guess.
 */
export function CrewFields({
	idPrefix,
	lines,
	roles,
	labelOf,
	detailOf,
	draft,
	problems,
	onChange,
}: {
	idPrefix: string;
	lines: readonly CrewLine[];
	roles: readonly ConstructionRole[];
	labelOf: (roleKey: string) => string;
	/** A line's progress or work, under its name. */
	detailOf: (line: CrewLine) => ReactNode;
	draft: NumberDraft;
	problems: Record<string, string>;
	onChange: (draft: NumberDraft) => void;
}) {
	return (
		<ul className="grid gap-2">
			{lines.map((line) => {
				const id = `${idPrefix}-${line.roleKey}`;
				const problem = problems[line.roleKey];
				const free = freeColonists(line, roles);
				const limits = [`${formatNumber(free)} free`];
				if (line.maxCrew !== null) {
					limits.push(`up to ${formatNumber(line.maxCrew)}`);
				}
				return (
					<li className="grid gap-1" key={line.roleKey}>
						<div className="flex items-center gap-3 rounded-lg border border-border bg-surface/60 py-2 pr-2 pl-3">
							<div className="grid min-w-0 flex-1 gap-0.5">
								<Label className="[overflow-wrap:anywhere]" htmlFor={id}>
									{labelOf(line.roleKey)} crew
								</Label>
								<span className="text-muted-foreground text-sm tabular-nums">
									{detailOf(line)} · {limits.join(", ")}
								</span>
							</div>
							<Input
								{...fieldInputProps(id, problem, false)}
								autoComplete="off"
								className="w-20 text-right"
								inputMode="numeric"
								onChange={(event) => {
									onChange({ ...draft, [line.roleKey]: event.target.value });
								}}
								placeholder="0"
								value={draft[line.roleKey] ?? ""}
							/>
						</div>
						{problem ? (
							<p className="text-ember-ink text-sm" id={`${id}-error`}>
								{problem}
							</p>
						) : null}
					</li>
				);
			})}
		</ul>
	);
}
