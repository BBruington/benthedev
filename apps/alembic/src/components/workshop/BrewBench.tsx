import { FlaskConical, Minus, Plus } from "lucide-react";
import { useState } from "react";
import { describeShortages, shortagesOf } from "#/brewing/inventory";
import type { Ingredient, Inventory, Mix } from "#/brewing/types";
import { Button } from "#/components/ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "#/components/ui/card";
import { Input } from "#/components/ui/input";
import { Label } from "#/components/ui/label";
import { TraditionBars } from "#/components/workshop/TraditionBars";
import { cn } from "#/lib/utils";

interface BrewBenchProps {
	ingredients: readonly Ingredient[];
	inventory: Inventory;
	mix: Mix;
	units: number;
	pending: boolean;
	error: string | null;
	onChange: (ingredientId: string, delta: number) => void;
	onClear: () => void;
	onBrew: (d20?: number) => void;
	previewing?: boolean;
}

export function BrewBench({
	ingredients,
	inventory,
	mix,
	units,
	pending,
	error,
	onChange,
	onClear,
	onBrew,
	previewing = false,
}: BrewBenchProps) {
	const [tableRoll, setTableRoll] = useState("");
	const d20 = tableRoll === "" ? undefined : Number(tableRoll);
	const validRoll =
		d20 === undefined ||
		(/^\d+$/.test(tableRoll) && Number.isInteger(d20) && d20 >= 1 && d20 <= 20);
	const selected = ingredients.filter(
		(ingredient) => (mix[ingredient.id] ?? 0) > 0,
	);
	const totals: [number, number, number, number] = [0, 0, 0, 0];
	for (const ingredient of selected) {
		for (const index of [0, 1, 2, 3]) {
			totals[index] += ingredient.values[index] * mix[ingredient.id];
		}
	}
	const shortages = shortagesOf(mix, inventory);
	return (
		<Card
			className={cn(
				"gap-4 border-primary/40 shadow-none transition-[border-color,box-shadow] duration-150",
				previewing && "border-primary ring-2 ring-primary/40",
			)}
		>
			<CardHeader>
				<div className="flex items-center justify-between gap-3">
					<CardTitle className="text-lg">Brew bench</CardTitle>
					<span className="font-mono text-xs text-muted-foreground">
						{previewing ? "Preview · " : ""}
						{units} / 4 units
					</span>
				</div>
				<CardDescription>
					Measure the traditions. Follow what the mixture tells you.
				</CardDescription>
			</CardHeader>
			<CardContent className="space-y-4" inert={previewing}>
				<div className="min-h-16 space-y-2">
					{selected.length === 0 ? (
						<p className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">
							Your vessel is empty. Start with your mentor’s note above.
						</p>
					) : (
						selected.map((ingredient) => (
							<div
								key={ingredient.id}
								className="flex items-center justify-between gap-3 text-sm"
							>
								<span>{ingredient.name}</span>
								<div className="flex shrink-0 items-center gap-2">
									<Button
										size="icon-sm"
										variant="outline"
										aria-label={`Remove ${ingredient.name} from bench`}
										onClick={() => onChange(ingredient.id, -1)}
									>
										<Minus className="size-3" />
									</Button>
									<span className="w-3 text-center font-mono">
										{mix[ingredient.id]}
									</span>
									<Button
										size="icon-sm"
										variant="outline"
										aria-label={`Add ${ingredient.name} to bench`}
										disabled={
											units >= 4 ||
											mix[ingredient.id] >= (inventory[ingredient.id] ?? 0)
										}
										onClick={() => onChange(ingredient.id, 1)}
									>
										<Plus className="size-3" />
									</Button>
								</div>
							</div>
						))
					)}
				</div>
				<TraditionBars values={totals} />
				<div className="space-y-2 border-t pt-4">
					<Label htmlFor="table-d20">
						Table d20{" "}
						<span className="font-normal text-muted-foreground">
							(optional)
						</span>
					</Label>
					<Input
						id="table-d20"
						type="text"
						inputMode="numeric"
						placeholder="Roll for me"
						value={tableRoll}
						onChange={(event) => setTableRoll(event.target.value)}
						aria-invalid={!validRoll}
						aria-describedby="table-d20-help"
					/>
					<p
						id="table-d20-help"
						className={`text-xs ${validRoll ? "text-muted-foreground" : "text-destructive"}`}
					>
						{validRoll
							? "Enter a whole number 1–20, or leave empty for a server roll."
							: "Use a whole number from 1 to 20."}
					</p>
				</div>
				{shortages.length > 0 && (
					<p role="alert" className="text-sm text-destructive">
						{describeShortages(shortages, ingredients)}
					</p>
				)}
				{error && (
					<p role="alert" className="text-sm text-destructive">
						{error}
					</p>
				)}
				<div className="flex gap-2">
					<Button
						className="flex-1"
						disabled={
							pending || units === 0 || !validRoll || shortages.length > 0
						}
						onClick={() => onBrew(d20)}
					>
						<FlaskConical className="size-4" />
						{pending ? "Brewing…" : "Brew potion"}
					</Button>
					<Button
						variant="outline"
						disabled={pending || units === 0}
						onClick={onClear}
					>
						Clear
					</Button>
				</div>
				<p className="text-xs text-muted-foreground">
					Every attempt costs one downtime day.
				</p>
			</CardContent>
		</Card>
	);
}
