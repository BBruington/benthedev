import { useEffect, useState } from "react";
import type { BrewLogEntry, Ingredient } from "#/brewing/types";
import { Badge } from "#/components/ui/badge";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "#/components/ui/card";
import { formatCheck, formatValues } from "#/lib/format";

function relativeTime(date: string, now: number): string {
	const minutes = Math.max(
		0,
		Math.floor((now - new Date(date).getTime()) / 60000),
	);
	if (minutes < 1) return "Just now";
	if (minutes < 60) return `${minutes}m ago`;
	if (minutes < 1440) return `${Math.floor(minutes / 60)}h ago`;
	return `${Math.floor(minutes / 1440)}d ago`;
}

export function BrewLog({
	log,
	ingredients,
}: {
	log: BrewLogEntry[];
	ingredients: readonly Ingredient[];
}) {
	const [now, setNow] = useState<number | null>(null);
	useEffect(() => {
		setNow(Date.now());
		const timer = window.setInterval(() => setNow(Date.now()), 60000);
		return () => window.clearInterval(timer);
	}, []);
	const entries = [...log].sort(
		(left, right) =>
			new Date(right.createdAt).getTime() - new Date(left.createdAt).getTime(),
	);
	return (
		<Card className="gap-0 rounded-lg py-0 shadow-none">
			<CardHeader className="gap-1 border-b px-4 py-3 sm:px-4 [.border-b]:pb-3">
				<CardTitle className="text-lg">Brew log</CardTitle>
				<CardDescription className="text-xs">
					{entries.length} {entries.length === 1 ? "experiment" : "experiments"}{" "}
					· Newest first · Totals: Arcane / Primal / Occult / Divine
				</CardDescription>
			</CardHeader>
			<CardContent className="px-0 sm:px-0">
				{entries.length === 0 ? (
					<p className="px-4 py-4 text-sm text-muted-foreground">
						No experiments yet. Your first brew starts the record.
					</p>
				) : (
					<section
						className="brew-log-region rounded-b-lg"
						aria-label="Brew log experiments"
					>
						<table className="brew-log-table w-full min-w-[640px] text-left text-xs">
							<caption className="sr-only">
								Experiments, newest first, including ingredients, totals,
								results and time
							</caption>
							<thead className="border-b bg-muted/30 text-muted-foreground">
								<tr>
									{[
										"Mixture",
										"A / P / O / D",
										"Outcome & check",
										"Recorded",
									].map((label) => (
										<th
											key={label}
											scope="col"
											className="px-4 py-2 font-medium"
										>
											{label}
										</th>
									))}
								</tr>
							</thead>
							<tbody className="divide-y">
								{entries.map((entry) => (
									<tr key={entry.id} className="align-top hover:bg-muted/20">
										<th
											scope="row"
											className="min-w-48 px-4 py-3 font-medium leading-relaxed"
										>
											{Object.entries(entry.mix)
												.map(
													([id, units]) =>
														`${ingredients.find((ingredient) => ingredient.id === id)?.name ?? id} ×${units}`,
												)
												.join(" + ")}
										</th>
										<td
											data-label="A / P / O / D"
											className="whitespace-nowrap px-4 py-3 font-mono text-muted-foreground tabular-nums"
										>
											{formatValues(entry.result.totals)}
										</td>
										<td className="min-w-56 space-y-1 px-4 py-3">
											<div className="flex flex-wrap items-center gap-2">
												<Badge variant="outline" className="capitalize">
													{entry.result.kind}
												</Badge>
												<span className="font-medium">
													{entry.result.kind === "formed"
														? `${entry.result.potionName} · ${entry.result.quantity} ${entry.result.quantity === 1 ? "potion" : "potions"} · ${entry.result.perfect ? "Perfect" : `off by ${entry.result.offBy}`}`
														: entry.result.kind === "beyond"
															? `Beyond · ${entry.result.potionName}`
															: "Inert"}
												</span>
											</div>
											{entry.result.kind === "formed" && (
												<p className="text-muted-foreground">
													{formatCheck(entry.result.check)}
												</p>
											)}
										</td>
										<td
											data-label="Recorded"
											className="whitespace-nowrap px-4 py-3 text-muted-foreground tabular-nums"
										>
											<time dateTime={entry.createdAt} title={entry.createdAt}>
												{now === null
													? entry.createdAt
													: relativeTime(entry.createdAt, now)}
											</time>
										</td>
									</tr>
								))}
							</tbody>
						</table>
					</section>
				)}
			</CardContent>
		</Card>
	);
}
