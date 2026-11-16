import { ArchiveIcon, XIcon } from "lucide-react";
import type { Schemas } from "@/api/client";
import { ColonyIcon } from "@/components/colony-icon";
import { fieldInputProps } from "@/components/field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { labelFromResourceKey } from "@/lib/colony";
import type { NumberDraft } from "@/lib/construction";
import { focusFirstVisible } from "@/lib/focus";

type ColonyResource = Schemas["ColonyResource"];

/**
 * An amount of 0 or more per unarchived resource: a blueprint's cost or a refund. An amount saved for
 * a resource archived since gets its own row to clear, since the server refuses it.
 */
export function AmountsEditor({
	idPrefix,
	resources,
	draft,
	problems,
	onChange,
}: {
	idPrefix: string;
	resources: readonly ColonyResource[];
	draft: NumberDraft;
	problems: Record<string, string>;
	onChange: (draft: NumberDraft) => void;
}) {
	// The board always lists food, which can't be archived, so an empty list means it hasn't loaded.
	const archivedKeys =
		resources.length === 0
			? []
			: Object.keys(draft)
					.filter((key) => !resources.some((resource) => resource.key === key))
					.sort();
	return (
		<div className="grid gap-2 sm:grid-cols-2">
			{archivedKeys.map((key) => {
				const label = labelFromResourceKey(key);
				return (
					<div
						className="flex items-center gap-2 rounded-lg border border-ember/40 border-dashed bg-ember/5 py-1 pr-1 pl-3"
						key={key}
					>
						<ArchiveIcon aria-hidden="true" className="size-4 shrink-0 text-ember-ink" />
						<span className="min-w-0 flex-1 truncate">
							{label}{" "}
							<span className="text-muted-foreground text-sm">
								{draft[key]} · archived, clear it to save
							</span>
						</span>
						<Button
							aria-label={`Clear ${label}`}
							onClick={() => {
								onChange(
									Object.fromEntries(
										Object.entries(draft).filter(([draftKey]) => draftKey !== key),
									),
								);
								requestAnimationFrame(() => {
									focusFirstVisible(...resources.map((resource) => `${idPrefix}-${resource.key}`));
								});
							}}
							size="sm"
							type="button"
							variant="ghost"
						>
							<XIcon />
							Clear
						</Button>
					</div>
				);
			})}
			{resources.map((resource) => {
				const id = `${idPrefix}-${resource.key}`;
				const problem = problems[resource.key];
				return (
					<div className="grid gap-1" key={resource.key}>
						<div className="flex items-center gap-2 rounded-lg border border-border bg-surface/60 py-1 pr-1 pl-3">
							<ColonyIcon className="size-4 shrink-0 text-gilt-ink" name={resource.icon} />
							<Label className="min-w-0 flex-1 truncate" htmlFor={id}>
								{resource.label}
							</Label>
							<Input
								{...fieldInputProps(id, problem, false)}
								autoComplete="off"
								className="w-24 text-right"
								inputMode="numeric"
								onChange={(event) => {
									onChange({ ...draft, [resource.key]: event.target.value });
								}}
								placeholder="0"
								value={draft[resource.key] ?? ""}
							/>
						</div>
						{problem ? (
							<p className="text-ember-ink text-sm" id={`${id}-error`}>
								{problem}
							</p>
						) : null}
					</div>
				);
			})}
		</div>
	);
}
