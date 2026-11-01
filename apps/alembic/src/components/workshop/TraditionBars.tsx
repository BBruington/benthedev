import { TRADITIONS, type Tradition, type Values } from "#/brewing/types";
import { TRADITION_LABEL } from "#/lib/format";

export const TRADITION_STYLE: Record<
	Tradition,
	{ chip: string; bar: string; text: string }
> = {
	arcane: {
		chip: "bg-arcane text-arcane-foreground",
		bar: "bg-arcane",
		text: "text-arcane",
	},
	primal: {
		chip: "bg-primal text-primal-foreground",
		bar: "bg-primal",
		text: "text-primal",
	},
	occult: {
		chip: "bg-occult text-occult-foreground",
		bar: "bg-occult",
		text: "text-occult",
	},
	divine: {
		chip: "bg-divine text-divine-foreground",
		bar: "bg-divine",
		text: "text-divine",
	},
};

export function TraditionBars({ values }: { values: Values }) {
	return (
		<section className="space-y-2.5" aria-label="Live tradition totals">
			{TRADITIONS.map((tradition, index) => (
				<div
					key={tradition}
					className="grid grid-cols-[3.5rem_minmax(0,1fr)_2rem] items-center gap-3 text-xs"
				>
					<span className="font-medium">{TRADITION_LABEL[tradition]}</span>
					<div
						className="h-1.5 overflow-hidden rounded-full bg-muted"
						aria-hidden="true"
					>
						<div
							className={`h-full rounded-full motion-safe:transition-[width] ${TRADITION_STYLE[tradition].bar}`}
							style={{ width: `${Math.min((values[index] / 45) * 100, 100)}%` }}
						/>
					</div>
					<span className="text-right text-sm font-semibold tabular-nums">
						{values[index]}
					</span>
				</div>
			))}
		</section>
	);
}
