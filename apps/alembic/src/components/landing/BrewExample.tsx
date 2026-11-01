import { Plus, RotateCcw } from "lucide-react";
import { useState } from "react";
import type { BrewView } from "#/brewing/types";
import { Button } from "#/components/ui/button";
import { ResultCard } from "#/components/workshop/ResultCard";
import { TraditionBars } from "#/components/workshop/TraditionBars";

export function BrewExample() {
	const [perfect, setPerfect] = useState(false);
	const result: BrewView = {
		kind: "formed",
		potionId: "healing-potion-minor",
		potionName: "Healing Potion (Minor)",
		totals: perfect ? [7, 26, 10, 13] : [7, 26, 1, 8],
		offBy: perfect ? 0 : 14,
		perfect,
		arrows: {
			arcane: "on",
			primal: "on",
			occult: perfect ? "on" : "low",
			divine: perfect ? "on" : "low",
		},
		check: {
			d20: 6,
			tableRoll: true,
			modifier: 14,
			total: 20,
			dc: 15,
			degree: "success",
		},
		bonusRoll: perfect ? 2 : null,
		quantity: perfect ? 3 : 1,
		worstAxis: null,
	};

	return (
		<div className="grid gap-6 lg:grid-cols-2 lg:gap-10">
			<div className="flex flex-col items-start">
				<p className="parchment w-full rounded-lg p-6 font-display text-2xl leading-snug sm:p-8 sm:text-3xl">
					Heartmoss +<br /> Marsh Toad Gland
					{perfect && <span className="block">+ Glowcap</span>}
				</p>
				<div className="my-6 w-full">
					<TraditionBars values={result.totals} />
				</div>
				<Button
					variant={perfect ? "outline" : "default"}
					onClick={() => setPerfect(!perfect)}
				>
					{perfect ? (
						<RotateCcw aria-hidden="true" />
					) : (
						<Plus aria-hidden="true" />
					)}
					{perfect ? "Try the original mix" : "Add Glowcap"}
				</Button>
			</div>
			<div className="space-y-4">
				<ResultCard result={result} crafterName="Wren" />
				<p
					aria-live="polite"
					aria-atomic="true"
					className="text-sm leading-relaxed text-muted-foreground"
				>
					{perfect
						? "Glowcap adds 9 Occult and 5 Divine. The totals are now Arcane 7, Primal 26, Occult 10, Divine 13. All four values match: a Perfect mix."
						: "The totals are Arcane 7, Primal 26, Occult 1, Divine 8. Close enough to form! “Off by 14” is the total distance from the target. The arrows show Occult and Divine are low: add more of those traditions."}
				</p>
				<p className="text-xs leading-relaxed text-muted-foreground">
					Worked example: a die roll of 6; the perfect mix’s bonus die rolls 2.
				</p>
			</div>
		</div>
	);
}
