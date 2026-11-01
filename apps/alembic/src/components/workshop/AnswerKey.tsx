import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Shield } from "lucide-react";
import { toast } from "sonner";
import { TRADITIONS } from "#/brewing/types";
import { Badge } from "#/components/ui/badge";
import { Button } from "#/components/ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "#/components/ui/card";
import { TRADITION_LABEL } from "#/lib/format";
import { orpc } from "#/orpc/client";
import { TRADITION_STYLE } from "./TraditionBars";

export function AnswerKey() {
	const queryClient = useQueryClient();
	const answerKey = useQuery(orpc.gm.answerKey.queryOptions());
	const workshop = useQuery(orpc.workshop.get.queryOptions());
	const reveal = useMutation(
		orpc.gm.reveal.mutationOptions({
			onSuccess: async () => {
				await Promise.all([
					queryClient.invalidateQueries({ queryKey: orpc.workshop.key() }),
					queryClient.invalidateQueries({ queryKey: orpc.gm.key() }),
				]);
			},
			onError: (error) => toast.error(error.message),
		}),
	);
	const ingredientNames = Object.fromEntries(
		(workshop.data?.ingredients ?? []).map((ingredient) => [
			ingredient.id,
			ingredient.name,
		]),
	);
	return (
		<Card className="gap-0 rounded-lg py-0 shadow-none">
			<CardHeader className="gap-1 border-b px-4 py-3 sm:px-4 [.border-b]:pb-3">
				<CardTitle className="flex flex-wrap items-center gap-2 text-lg">
					<Shield className="size-4 shrink-0 text-occult" aria-hidden="true" />
					Answer key
					<Badge variant="outline" className="font-sans text-occult">
						GM eyes only
					</Badge>
				</CardTitle>
				<CardDescription className="text-xs">
					Reveal a potion's identity or share its formula. Targets and matching
					rules always stay hidden. Hidden removes only the GM reveal, not
					brewed or mentor knowledge.
				</CardDescription>
			</CardHeader>
			<CardContent className="px-0 sm:px-0">
				{answerKey.isPending ? (
					<output className="block px-4 py-4 text-sm text-muted-foreground">
						Opening the answer key…
					</output>
				) : answerKey.isError ? (
					<div className="flex flex-wrap items-center gap-3 px-4 py-4">
						<p role="alert" className="text-sm text-destructive">
							{answerKey.error.message}
						</p>
						<Button
							variant="outline"
							size="sm"
							onClick={() => answerKey.refetch()}
						>
							Try again
						</Button>
					</div>
				) : (
					<section aria-label="GM potion answer key">
						{answerKey.data.map((entry) => {
							const saving =
								reveal.isPending &&
								reveal.variables?.potionId === entry.potion.id;
							const selected = saving
								? reveal.variables.reveal
								: entry.formulaRevealed
									? "formula"
									: entry.revealed
										? "potion"
										: "none";
							return (
								<article
									key={entry.potion.id}
									aria-label={entry.potion.name}
									className="grid gap-4 border-b px-4 py-4 last:border-0 sm:grid-cols-2 sm:gap-x-6 xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.25fr)_minmax(0,1fr)_auto]"
								>
									<div className="min-w-0 space-y-2">
										<h3 className="text-sm font-semibold">
											{entry.potion.name}
										</h3>
										<div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
											<span>Level {entry.potion.level}</span>
											<Badge variant="outline" className="capitalize">
												{entry.potion.rarity}
											</Badge>
											<span className="tabular-nums">DC {entry.dc}</span>
										</div>
										<div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
											{entry.known ? (
												<Badge variant="secondary">Known</Badge>
											) : (
												<span>Undiscovered</span>
											)}
											{entry.perfected && (
												<Badge variant="outline" className="gap-1 text-primal">
													<Check className="size-3" aria-hidden="true" />
													Perfected
												</Badge>
											)}
										</div>
									</div>
									<div className="min-w-0 space-y-2">
										<p className="text-xs text-muted-foreground">Target</p>
										<div className="grid grid-cols-4 gap-1.5">
											{TRADITIONS.map((tradition, index) => (
												<span
													key={tradition}
													className={`flex flex-col items-center rounded-md px-1 py-1 text-xs ${TRADITION_STYLE[tradition].chip} ${tradition === entry.primary ? "ring-1 ring-foreground ring-offset-2 ring-offset-card" : ""}`}
												>
													<span>{TRADITION_LABEL[tradition]}</span>
													<strong className="text-sm tabular-nums">
														{entry.potion.target[index]}
													</strong>
												</span>
											))}
										</div>
										<p className="text-xs leading-relaxed text-muted-foreground tabular-nums">
											<span className={TRADITION_STYLE[entry.primary].text}>
												{TRADITION_LABEL[entry.primary]} primary
											</span>
											{" · "}Room ±{entry.potion.primaryRoom}
											{" · "}Budget {entry.potion.secondaryBudget}
										</p>
									</div>
									<div className="min-w-0 space-y-2 text-xs leading-relaxed">
										<p className="text-muted-foreground">Perfect recipe</p>
										<p>
											{Object.entries(entry.potion.perfectRecipe)
												.map(
													([id, count]) =>
														`${ingredientNames[id] ?? id} ×${count}`,
												)
												.join(" + ")}
										</p>
										<p className="text-muted-foreground">
											Required:{" "}
											{entry.potion.requiredIngredient
												? (ingredientNames[entry.potion.requiredIngredient] ??
													entry.potion.requiredIngredient)
												: "None"}
										</p>
									</div>
									<div className="space-y-2">
										<p className="text-xs text-muted-foreground">
											Player reveal
											<output aria-live="polite">
												{saving ? " · Saving…" : ""}
											</output>
										</p>
										<fieldset
											className="flex w-fit gap-1 rounded-lg border bg-muted/40 p-1"
											aria-busy={saving}
										>
											<legend className="sr-only">
												Player reveal for {entry.potion.name}
											</legend>
											{(["none", "potion", "formula"] as const).map((value) => {
												const label =
													value === "none"
														? "Hidden"
														: value === "potion"
															? "Potion"
															: "Formula";
												return (
													<Button
														key={value}
														type="button"
														size="sm"
														variant={selected === value ? "default" : "ghost"}
														aria-pressed={selected === value}
														aria-label={`${label} for ${entry.potion.name}`}
														aria-disabled={reveal.isPending}
														className="aria-disabled:opacity-50"
														onClick={() => {
															if (reveal.isPending || selected === value)
																return;
															reveal.mutate({
																potionId: entry.potion.id,
																reveal: value,
															});
														}}
													>
														{label}
													</Button>
												);
											})}
										</fieldset>
									</div>
								</article>
							);
						})}
					</section>
				)}
			</CardContent>
		</Card>
	);
}
