import { Minus, Plus, Star, X } from "lucide-react";
import { Fragment, useEffect, useRef, useState } from "react";
import {
	type Ingredient,
	type Inventory,
	type Mix,
	TRADITIONS,
} from "#/brewing/types";
import { Badge } from "#/components/ui/badge";
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
import { ScrollArea } from "#/components/ui/scroll-area";
import { StockStepper } from "#/components/workshop/StockStepper";
import { TRADITION_STYLE } from "#/components/workshop/TraditionBars";
import { useFavouriteIngredients } from "#/components/workshop/use-favourite-ingredients";
import { TRADITION_LABEL } from "#/lib/format";

interface ShelfProps {
	ingredients: readonly Ingredient[];
	inventory: Inventory;
	mix: Mix;
	units: number;
	onChange: (ingredientId: string, delta: number) => void;
}

export function Shelf({
	ingredients,
	inventory,
	mix,
	units,
	onChange,
}: ShelfProps) {
	const [search, setSearch] = useState("");
	const { favourites, toggleFavourite } = useFavouriteIngredients();
	const searchRef = useRef<HTMLInputElement>(null);
	const toggledStar = useRef<HTMLButtonElement>(null);
	useEffect(() => {
		const button = toggledStar.current;
		if (!button) return;
		button.focus({ preventScroll: true });
		button.scrollIntoView({ block: "nearest" });
		toggledStar.current = null;
	});
	const query = search.trim().toLowerCase();
	const matches = ingredients
		.filter((ingredient) =>
			[
				ingredient.name,
				ingredient.type,
				ingredient.form,
				ingredient.biome,
				ingredient.rarity,
			].some((value) => value.toLowerCase().includes(query)),
		)
		.sort(
			(left, right) =>
				Number(favourites.includes(right.id)) -
				Number(favourites.includes(left.id)),
		);
	const hasFavourites = matches.some((ingredient) =>
		favourites.includes(ingredient.id),
	);
	return (
		<Card className="ingredient-shelf gap-4 py-4 shadow-none">
			<CardHeader>
				<div className="flex flex-wrap items-baseline justify-between gap-2">
					<CardTitle className="text-lg">Ingredients</CardTitle>
					<span className="text-xs text-muted-foreground">
						{matches.length} of {ingredients.length}
					</span>
				</div>
				<CardDescription>
					Choose up to four units. Brewing uses up what goes in.
				</CardDescription>
				<div className="mt-3 flex items-center gap-2">
					<Label htmlFor="ingredient-search" className="sr-only">
						Search ingredients
					</Label>
					<Input
						id="ingredient-search"
						ref={searchRef}
						type="search"
						placeholder="Search ingredients…"
						value={search}
						onChange={(event) => setSearch(event.target.value)}
					/>
					{search && (
						<Button
							type="button"
							size="icon"
							variant="ghost"
							aria-label="Clear search"
							onClick={() => {
								setSearch("");
								searchRef.current?.focus();
							}}
						>
							<X className="size-4" />
						</Button>
					)}
				</div>
			</CardHeader>
			<CardContent className="px-0">
				<ScrollArea className="ingredient-list">
					{matches.length === 0 ? (
						<p className="px-6 py-12 text-center text-sm text-muted-foreground">
							No ingredients match “{search}”. Try another name or clear your
							search.
						</p>
					) : (
						matches.map((ingredient, index) => {
							const favourite = favourites.includes(ingredient.id);
							const groupStart =
								index === 0 ||
								favourite !== favourites.includes(matches[index - 1].id);
							return (
								<Fragment key={ingredient.id}>
									{hasFavourites && groupStart && (
										<p className="ingredient-group">
											{favourite ? "Favourites" : "Catalogue"}
										</p>
									)}
									<div
										className={`ingredient-row ${mix[ingredient.id] ? "ingredient-selected" : ""}`}
									>
										<Button
											type="button"
											size="icon-sm"
											variant="ghost"
											aria-label={`Favourite ${ingredient.name}`}
											aria-pressed={favourite}
											onClick={(event) => {
												toggledStar.current = event.currentTarget;
												toggleFavourite(ingredient.id);
											}}
										>
											<Star
												className={`size-4 ${favourite ? "fill-primary text-primary" : "text-muted-foreground"}`}
											/>
										</Button>
										<div className="ingredient-detail min-w-0">
											<div className="flex flex-wrap items-center gap-2">
												<h3 className="text-sm font-semibold">
													{ingredient.name}
												</h3>
												<Badge variant="outline" className="text-xs capitalize">
													{ingredient.rarity}
												</Badge>
											</div>
											<p className="mt-1 text-xs capitalize text-muted-foreground">
												{ingredient.type} · {ingredient.form} ·{" "}
												{ingredient.biome}
											</p>
											<StockStepper
												ingredientId={ingredient.id}
												ingredientName={ingredient.name}
												amount={inventory[ingredient.id] ?? 0}
											/>
										</div>
										<div className="ingredient-values grid grid-cols-4 gap-3">
											{TRADITIONS.map((tradition, axis) => (
												<div
													key={tradition}
													className={TRADITION_STYLE[tradition].text}
												>
													<p className="text-[0.6875rem]">
														{TRADITION_LABEL[tradition]}
													</p>
													<p className="mt-1 font-mono text-sm tabular-nums">
														{ingredient.values[axis]}
													</p>
												</div>
											))}
										</div>
										<div className="ingredient-controls flex items-center gap-1">
											<Button
												type="button"
												size="icon-sm"
												variant="outline"
												aria-label={`Remove ${ingredient.name}`}
												disabled={!mix[ingredient.id]}
												onClick={() => onChange(ingredient.id, -1)}
											>
												<Minus className="size-3" />
											</Button>
											<span className="w-4 text-center font-mono text-xs tabular-nums">
												{mix[ingredient.id] ?? 0}
											</span>
											<Button
												type="button"
												size="icon-sm"
												variant="outline"
												aria-label={`Add ${ingredient.name}`}
												disabled={
													units >= 4 ||
													(mix[ingredient.id] ?? 0) >=
														(inventory[ingredient.id] ?? 0)
												}
												onClick={() => onChange(ingredient.id, 1)}
											>
												<Plus className="size-3" />
											</Button>
										</div>
									</div>
								</Fragment>
							);
						})
					)}
				</ScrollArea>
			</CardContent>
		</Card>
	);
}
