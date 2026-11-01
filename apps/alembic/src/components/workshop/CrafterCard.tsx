import type { CrafterView } from "#/brewing/types";
import { Badge } from "#/components/ui/badge";
import { CrafterSheet, TOOL_LABELS } from "#/components/workshop/CrafterSheet";

export function CrafterCard({ crafter }: { crafter: CrafterView }) {
	return (
		<section className="crafter-strip" aria-label="Your crafter">
			<div className="flex flex-wrap items-center gap-x-4 gap-y-1">
				<h2 className="font-semibold">{crafter.name}</h2>
				<p className="text-xs text-muted-foreground">
					Level {crafter.level} {crafter.className} · Int{" "}
					{crafter.intMod >= 0 ? "+" : ""}
					{crafter.intMod} ·{" "}
					<span className="capitalize">{crafter.craftingRank}</span> Crafting
				</p>
			</div>
			<div className="flex flex-wrap items-center gap-3 text-xs">
				<div className="crafter-tools flex flex-wrap gap-x-3">
					<p>
						{crafter.alchemicalCrafting
							? "Alchemical Crafting"
							: "No Alchemical Crafting"}
					</p>
					<p className="text-muted-foreground">{TOOL_LABELS[crafter.tools]}</p>
				</div>
				<Badge variant="secondary" className="tabular-nums">
					Crafting {crafter.modifier >= 0 ? "+" : ""}
					{crafter.modifier}
				</Badge>
				<span className="text-muted-foreground tabular-nums">
					{crafter.downtimeDays} downtime{" "}
					{crafter.downtimeDays === 1 ? "day" : "days"}
				</span>
				<CrafterSheet crafter={crafter} />
			</div>
		</section>
	);
}
