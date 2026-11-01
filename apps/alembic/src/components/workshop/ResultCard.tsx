import { ArrowDown, ArrowUp, Check, FlaskConical } from "lucide-react";
import { type BrewView, TRADITIONS } from "#/brewing/types";
import { Badge } from "#/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "#/components/ui/card";
import { TRADITION_STYLE } from "#/components/workshop/TraditionBars";
import { formatCheck, formatGate, TRADITION_LABEL } from "#/lib/format";

export function ResultCard({
	result,
	crafterName,
}: {
	result: BrewView | null;
	crafterName: string;
}) {
	if (!result) return null;
	const potionName = result.potionName ?? "Potion";
	return (
		<Card className="min-w-0 gap-3 rounded-lg py-4 shadow-none">
			<CardHeader className="px-4 sm:px-4">
				<CardTitle className="flex items-start gap-2 font-sans text-base font-semibold leading-snug">
					<FlaskConical
						className="mt-0.5 size-4 shrink-0 text-primary"
						aria-hidden="true"
					/>
					<span className="min-w-0 break-words">
						{result.kind === "inert" ? "The mix stays inert" : potionName}
					</span>
				</CardTitle>
			</CardHeader>
			<CardContent className="space-y-3 px-4 sm:px-4">
				{result.kind === "formed" && (
					<>
						<div className="flex flex-wrap items-center gap-x-3 gap-y-2">
							<Badge variant={result.perfect ? "default" : "secondary"}>
								{result.perfect ? "Perfect" : `Off by ${result.offBy}`}
							</Badge>
							<span className="min-w-0 text-sm font-semibold tabular-nums">
								{result.bonusRoll !== null
									? `${result.quantity - result.bonusRoll} + 1d3 (${result.bonusRoll}) = ${result.quantity} potions`
									: `${result.quantity} ${result.quantity === 1 ? "potion" : "potions"}`}
							</span>
						</div>
						<p className="border-y py-3 text-sm leading-relaxed tabular-nums">
							{formatCheck(result.check)}
						</p>
						{result.quantity === 0 && (
							<p className="text-sm leading-relaxed text-muted-foreground">
								The potion is discovered, but this batch yields no usable
								potions.
							</p>
						)}
						{result.perfect && result.check.degree === "failure" && (
							<p className="text-sm leading-relaxed text-muted-foreground">
								Your perfect mixture salvaged one potion.
							</p>
						)}
						<div className="grid grid-cols-2 gap-x-4 gap-y-2">
							{TRADITIONS.map((tradition) => {
								const arrow = result.arrows[tradition];
								const Icon =
									arrow === "on"
										? Check
										: arrow === "low"
											? ArrowUp
											: ArrowDown;
								return (
									<div
										key={tradition}
										className={`flex min-w-0 items-center gap-1.5 py-1 text-xs ${TRADITION_STYLE[tradition].text}`}
									>
										<Icon className="size-3.5 shrink-0" aria-hidden="true" />
										<span className="font-medium">
											{TRADITION_LABEL[tradition]}
										</span>
										<span className="ml-auto text-foreground">{arrow}</span>
									</div>
								);
							})}
						</div>
						{result.worstAxis && (
							<p className="flex flex-wrap items-baseline justify-between gap-x-2 gap-y-1 border-t pt-3 text-sm">
								<span>
									{TRADITION_LABEL[result.worstAxis.tradition]} target
								</span>
								<strong className="tabular-nums">
									{result.worstAxis.target}
								</strong>
							</p>
						)}
					</>
				)}
				{result.kind === "inert" && (
					<p className="text-sm leading-relaxed text-muted-foreground">
						{result.hint
							? `The mix wants ${result.hint.direction} ${TRADITION_LABEL[result.hint.tradition]}.`
							: "The traditions have not settled into a potion. Try a different balance."}
					</p>
				)}
				{result.kind === "beyond" && (
					<>
						<Badge variant="outline">Beyond your reach</Badge>
						<ul className="list-disc space-y-2 pl-4 text-sm leading-relaxed marker:text-muted-foreground">
							{result.gates.map((gate) => (
								<li key={gate.kind}>
									{formatGate(gate, crafterName, potionName)}
								</li>
							))}
						</ul>
					</>
				)}
			</CardContent>
		</Card>
	);
}
