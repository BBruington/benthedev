const HOUSE_RULES = [
	"Potions are found by measuring traditions, not by learning formulas.",
	"Brewing uses up its ingredients; you can't brew with more than you have.",
	"Every brew costs one downtime day.",
	"A perfect mix yields 1d3 extra potions on a success.",
	"A failed check with a perfect mix still salvages one potion.",
	"Rare potions need master Crafting.",
	"The GM can reveal a potion by name, or its full formula; its target stays hidden.",
	"Traditions start hidden; deconstruct an ingredient or potion to reveal one.",
];

export function HouseRules() {
	return (
		<details open className="border-y py-4">
			<summary className="cursor-pointer text-sm font-semibold">
				<h2 className="inline text-sm font-semibold">The house rules</h2>
			</summary>
			<ol className="mt-4 grid list-decimal gap-x-10 gap-y-2 pl-5 text-sm leading-relaxed text-muted-foreground marker:font-semibold sm:grid-cols-2">
				{HOUSE_RULES.map((rule) => (
					<li key={rule} className="pl-1">
						{rule}
					</li>
				))}
			</ol>
		</details>
	);
}
