import { ArchiveIcon, PlusIcon, Trash2Icon, XIcon } from "lucide-react";
import type { Schemas } from "@/api/client";
import { Field } from "@/components/field";
import { NativeSelect } from "@/components/form-controls";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { labelFromResourceKey } from "@/lib/colony";
import { newOutputDraft, type OutputDraft } from "@/lib/construction";
import { focusFirstVisible } from "@/lib/focus";

type ColonyResource = Schemas["ColonyResource"];

const outputAddId = "blueprint-output-add";

function rowIdOf(draft: OutputDraft): string {
	return `blueprint-output-${draft.rowKey}`;
}

/**
 * One line per resource: what a standing building makes (positive) or uses (negative) each day, and
 * how much more of it the colony can store. A line saved for a resource archived since gets its own
 * row to clear, since the server refuses it.
 */
export function OutputEditor({
	drafts,
	resources,
	problems,
	onChange,
}: {
	drafts: OutputDraft[];
	/** The unarchived resources a line may name. */
	resources: readonly ColonyResource[];
	problems: Record<string, string>;
	onChange: (drafts: OutputDraft[]) => void;
}) {
	const usedKeys = new Set(drafts.map((draft) => draft.resourceKey));
	const nextResource = resources.find((resource) => !usedKeys.has(resource.key));

	function update(position: number, change: Partial<OutputDraft>) {
		onChange(drafts.map((draft, index) => (index === position ? { ...draft, ...change } : draft)));
	}

	function remove(position: number) {
		const remaining = drafts.filter((_, index) => index !== position);
		onChange(remaining);
		const neighbour = remaining[Math.min(position, remaining.length - 1)];
		requestAnimationFrame(() => {
			focusFirstVisible(
				...(neighbour ? [`${rowIdOf(neighbour)}-resource`, `${rowIdOf(neighbour)}-clear`] : []),
				outputAddId,
			);
		});
	}

	return (
		<div className="grid gap-2">
			{drafts.length === 0 ? (
				<p className="text-muted-foreground text-sm">
					No output: a building of it changes nothing from day to day.
				</p>
			) : null}
			<ul className="grid gap-2">
				{drafts.map((draft, position) => {
					const rowId = rowIdOf(draft);
					// The board always lists food, which can't be archived, so an empty list means it hasn't loaded.
					const isArchived =
						resources.length > 0 &&
						!resources.some((resource) => resource.key === draft.resourceKey);
					if (isArchived) {
						const label = labelFromResourceKey(draft.resourceKey);
						const saved = [
							draft.dailyText ? `${draft.dailyText} a day` : "",
							draft.bonusText ? `stores ${draft.bonusText} more` : "",
						]
							.filter(Boolean)
							.join(", ");
						return (
							<li
								className="flex items-center gap-2 rounded-lg border border-ember/40 border-dashed bg-ember/5 py-1 pr-1 pl-3"
								key={draft.rowKey}
							>
								<ArchiveIcon aria-hidden="true" className="size-4 shrink-0 text-ember-ink" />
								<span className="min-w-0 flex-1 truncate">
									{label}{" "}
									<span className="text-muted-foreground text-sm">
										{saved} · archived, clear it to save
									</span>
								</span>
								<Button
									aria-label={`Clear ${label}`}
									id={`${rowId}-clear`}
									onClick={() => {
										remove(position);
									}}
									size="sm"
									type="button"
									variant="ghost"
								>
									<XIcon />
									Clear
								</Button>
							</li>
						);
					}
					const problem = problems[draft.rowKey];
					// A line with nothing typed yet reads as a hint, not an error.
					const isHint = draft.dailyText.trim() === "" && draft.bonusText.trim() === "";
					let problemProps = {};
					if (problem) {
						problemProps = isHint
							? { "aria-describedby": `${rowId}-error` }
							: { "aria-describedby": `${rowId}-error`, "aria-invalid": true };
					}
					return (
						<li className="grid gap-2 rounded-lg border border-border p-3" key={draft.rowKey}>
							<div className="flex gap-2">
								<NativeSelect
									{...problemProps}
									aria-label={`Output line ${position + 1} resource`}
									className="min-w-0 flex-1"
									id={`${rowId}-resource`}
									onChange={(event) => {
										update(position, { resourceKey: event.target.value });
									}}
									value={draft.resourceKey}
								>
									{resources.map((resource) => (
										<option
											disabled={resource.key !== draft.resourceKey && usedKeys.has(resource.key)}
											key={resource.key}
											value={resource.key}
										>
											{resource.label}
										</option>
									))}
								</NativeSelect>
								<Button
									aria-label={`Remove output line ${position + 1}`}
									onClick={() => {
										remove(position);
									}}
									size="icon"
									type="button"
									variant="ghost"
								>
									<Trash2Icon />
								</Button>
							</div>
							<div className="grid grid-cols-2 gap-2">
								<Field id={`${rowId}-daily`} label="Each day" optional>
									<Input
										{...problemProps}
										autoComplete="off"
										id={`${rowId}-daily`}
										inputMode="text"
										onChange={(event) => {
											update(position, { dailyText: event.target.value });
										}}
										placeholder="+0"
										value={draft.dailyText}
									/>
								</Field>
								<Field id={`${rowId}-bonus`} label="Stores more" optional>
									<Input
										{...problemProps}
										autoComplete="off"
										id={`${rowId}-bonus`}
										inputMode="numeric"
										onChange={(event) => {
											update(position, { bonusText: event.target.value });
										}}
										placeholder="0"
										value={draft.bonusText}
									/>
								</Field>
							</div>
							{problem ? (
								<p
									className={isHint ? "text-muted-foreground text-sm" : "text-ember-ink text-sm"}
									id={`${rowId}-error`}
								>
									{problem}
								</p>
							) : null}
						</li>
					);
				})}
			</ul>
			{nextResource ? (
				<Button
					className="justify-self-start"
					id={outputAddId}
					onClick={() => {
						const added = newOutputDraft(nextResource.key);
						onChange([...drafts, added]);
						requestAnimationFrame(() => {
							focusFirstVisible(`${rowIdOf(added)}-daily`);
						});
					}}
					type="button"
					variant="outline"
				>
					<PlusIcon />
					Add an output
				</Button>
			) : null}
		</div>
	);
}
