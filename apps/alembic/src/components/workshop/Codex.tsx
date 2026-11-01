import { BookOpen, Check, FlaskConical, LockKeyhole } from "lucide-react";
import type { Mix, WorkshopView } from "#/brewing/types";
import { Badge } from "#/components/ui/badge";
import { Button } from "#/components/ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "#/components/ui/card";
import { formatGate, formatOdds } from "#/lib/format";

export function Codex({
	workshop,
	onUseMix,
	onPreviewMix,
}: {
	workshop: WorkshopView;
	onUseMix: (mix: Mix) => void;
	onPreviewMix: (mix: Mix | null) => void;
}) {
	return (
		<Card className="h-fit gap-0 rounded-lg py-0 shadow-none">
			<CardHeader className="gap-1 border-b px-4 py-3 sm:px-4 [.border-b]:pb-3">
				<CardTitle className="flex items-center gap-2 text-lg">
					<BookOpen
						className="size-4 shrink-0 text-muted-foreground"
						aria-hidden="true"
					/>
					Potion codex
				</CardTitle>
				<CardDescription className="text-xs tabular-nums">
					{workshop.codex.length} known · {workshop.undiscoveredCount}{" "}
					undiscovered
				</CardDescription>
			</CardHeader>
			<CardContent className="px-0 sm:px-0">
				{workshop.codex.map((entry) => {
					const segments = [
						{
							label: "Critical success",
							value: entry.odds.criticalSuccess,
							color: "bg-primal",
						},
						{ label: "Success", value: entry.odds.success, color: "bg-arcane" },
						{ label: "Failure", value: entry.odds.failure, color: "bg-occult" },
						{
							label: "Critical failure",
							value: entry.odds.criticalFailure,
							color: "bg-destructive",
						},
					];
					return (
						<article
							key={entry.potionId}
							className="grid gap-2 border-b px-4 py-3 last:border-0 sm:grid-cols-[minmax(0,1fr)_minmax(9rem,0.7fr)] sm:gap-x-5"
						>
							<div className="min-w-0 space-y-1.5">
								<div className="flex flex-wrap items-center gap-2">
									<h3 className="text-sm font-semibold">{entry.name}</h3>
									<Badge
										variant="outline"
										className={
											entry.rarity === "rare"
												? "text-occult"
												: entry.rarity === "uncommon"
													? "text-arcane"
													: "text-muted-foreground"
										}
									>
										{entry.rarity}
									</Badge>
								</div>
								<div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
									<span>Level {entry.level}</span>
									<span>DC {entry.dc}</span>
									{entry.perfected && (
										<Badge variant="secondary" className="gap-1 text-primal">
											<Check className="size-3" aria-hidden="true" />
											Perfected
										</Badge>
									)}
								</div>
								<p className="text-xs text-muted-foreground">
									Best off-by:{" "}
									<span className="font-medium text-foreground tabular-nums">
										{entry.bestOffBy ?? "Not brewed yet"}
									</span>
								</p>
								{entry.solvedMix && (
									<p className="text-xs">
										{entry.formulaRevealed && (
											<span className="text-muted-foreground">
												Formula from your GM:{" "}
											</span>
										)}
										{Object.entries(entry.solvedMix)
											.map(
												([id, units]) =>
													`${workshop.ingredients.find((ingredient) => ingredient.id === id)?.name ?? id} ×${units}`,
											)
											.join(" + ")}
									</p>
								)}
								{entry.source === "mentor" && entry.note && (
									<p className="text-xs text-muted-foreground">
										Mentor's note: {entry.note}
									</p>
								)}
								{entry.source === "revealed" && !entry.formulaRevealed && (
									<p className="text-xs text-muted-foreground">
										Revealed by GM
									</p>
								)}
								{entry.source === "brewed" && (
									<p className="text-xs text-muted-foreground">
										Discovered at the bench
									</p>
								)}
							</div>
							<div className="flex flex-col gap-1.5">
								<p className="flex items-baseline gap-2 tabular-nums">
									<span className="text-xl font-semibold text-foreground">
										{Math.round(
											(entry.odds.criticalSuccess + entry.odds.success) * 100,
										)}
										%
									</span>
									<span className="text-sm text-muted-foreground">
										chance of success
									</span>
								</p>
								<div
									className="flex h-1 w-full overflow-hidden rounded-full bg-muted"
									aria-hidden="true"
								>
									{segments.map((segment) => (
										<span
											key={segment.label}
											className={segment.color}
											style={{ width: `${segment.value * 100}%` }}
											title={`${segment.label}: ${Math.round(segment.value * 100)}%`}
										/>
									))}
								</div>
								<p className="text-xs text-muted-foreground tabular-nums">
									{formatOdds(entry.odds)}
								</p>
								{entry.solvedMix && (
									<Button
										variant="outline"
										size="sm"
										className="mt-auto self-end"
										onClick={() => {
											if (!entry.solvedMix) return;
											onPreviewMix(null);
											onUseMix(entry.solvedMix);
										}}
										onPointerEnter={() => onPreviewMix(entry.solvedMix)}
										onPointerLeave={() => onPreviewMix(null)}
										onFocus={() => onPreviewMix(entry.solvedMix)}
										onBlur={() => onPreviewMix(null)}
										aria-label={`Add ${entry.name} combination to bench`}
									>
										<FlaskConical className="size-3.5" aria-hidden="true" />
										Add to bench
									</Button>
								)}
							</div>
							{entry.crafterGates.length > 0 && (
								<div className="space-y-1 border-t border-destructive/20 pt-2 text-xs text-destructive sm:col-span-2">
									<p className="font-medium">
										Beyond {workshop.crafter.name.split(" ")[0]}
									</p>
									{entry.crafterGates.map((gate) => (
										<p key={gate.kind}>
											{formatGate(gate, workshop.crafter.name, entry.name)}
										</p>
									))}
								</div>
							)}
						</article>
					);
				})}
				{workshop.undiscoveredCount > 0 && (
					<div className="space-y-2 border-t px-4 py-3">
						<p className="flex items-center gap-2 text-xs text-muted-foreground">
							<LockKeyhole className="size-3.5" aria-hidden="true" />
							{workshop.undiscoveredCount} undiscovered
						</p>
						<ul
							className="flex flex-wrap gap-1"
							aria-label={`${workshop.undiscoveredCount} undiscovered potion slots`}
						>
							{Array.from(
								{ length: workshop.undiscoveredCount },
								(_, index) => `Undiscovered potion ${index + 1}`,
							).map((slot) => (
								<li
									key={slot}
									className="size-3 rounded-sm border border-dashed border-muted-foreground/50"
								>
									<span className="sr-only">{slot}</span>
								</li>
							))}
						</ul>
					</div>
				)}
			</CardContent>
		</Card>
	);
}
