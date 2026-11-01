import { Show } from "@clerk/tanstack-react-start";
import { createFileRoute, Link } from "@tanstack/react-router";
import { ArrowRight } from "lucide-react";
import { TRADITIONS } from "#/brewing/types";
import { BrewExample } from "#/components/landing/BrewExample";
import { buttonVariants } from "#/components/ui/button";
import { TRADITION_STYLE } from "#/components/workshop/TraditionBars";
import { TRADITION_LABEL } from "#/lib/format";

export const Route = createFileRoute("/")({ component: Landing });

function WorkshopLink() {
	return (
		<>
			<Show when="signed-out">
				<Link to="/sign-in/$" className={buttonVariants({ size: "lg" })}>
					Sign in <ArrowRight aria-hidden="true" />
				</Link>
			</Show>
			<Show when="signed-in">
				<Link to="/workshop" className={buttonVariants({ size: "lg" })}>
					Enter the workshop <ArrowRight aria-hidden="true" />
				</Link>
			</Show>
		</>
	);
}

function Landing() {
	return (
		<main className="page-wrap max-w-6xl pb-16 sm:pb-24">
			<section className="grid gap-10 py-12 sm:py-20 lg:grid-cols-[1.2fr_1fr] lg:items-center lg:gap-16">
				<div>
					<h1 className="font-display text-6xl leading-none tracking-tight sm:text-8xl">
						Alembic
					</h1>
					<p className="mt-5 max-w-xl font-display text-3xl leading-tight text-primary sm:text-4xl">
						Turn a handful of ingredients into a discovery.
					</p>
					<p className="mt-6 max-w-xl text-base leading-relaxed text-muted-foreground sm:text-lg">
						A potion-brewing workshop for your Pathfinder Second Edition (PF2e)
						table’s crafter. Experiment with magical ingredients, follow the
						clues, and find a better brew. No Pathfinder experience needed.
					</p>
					<div className="mt-8 flex flex-wrap items-center gap-5">
						<WorkshopLink />
						<a
							href="#first-brew"
							className="text-sm font-semibold text-foreground underline decoration-border underline-offset-4 hover:decoration-primary"
						>
							See how a brew works
						</a>
					</div>
				</div>
				<aside
					className="parchment rounded-lg p-7 sm:p-9"
					aria-label="Your first day in the workshop"
				>
					<h2 className="text-2xl sm:text-3xl">
						A mentor’s note. A place to start.
					</h2>
					<p className="mt-5 text-base leading-relaxed">
						Meet Wren Ashby, your ready-to-play crafter. Your first potion
						discovery and a mentor’s note are already waiting. You bring the
						curiosity.
					</p>
					<div className="mt-6 border-t border-current/20 pt-6 font-display text-3xl leading-snug">
						“Heartmoss +<br />
						Marsh Toad Gland”
					</div>
					<p className="mt-5 text-sm leading-relaxed">
						Brewing uses up your ingredients. Each brew takes one in-game
						downtime day, not a day of waiting in the app.
					</p>
				</aside>
			</section>

			<section
				className="border-t py-10 sm:py-14"
				aria-labelledby="traditions-heading"
			>
				<div className="grid gap-6 lg:grid-cols-[1fr_1.3fr] lg:gap-16">
					<h2
						id="traditions-heading"
						className="text-3xl leading-tight sm:text-4xl"
					>
						Four values.
						<br />A hidden target.
					</h2>
					<div>
						<p className="max-w-2xl leading-relaxed text-muted-foreground">
							Every ingredient carries four values, one for each magical
							tradition: a different source of magic. Mix 1–4 ingredient units
							and their values add up. Each potion has a hidden target mix; land
							close enough and it forms. Its strongest tradition has its own
							tolerance; the other three share an allowance for error. You can
							repeat ingredients.
						</p>
						<div className="mt-6 flex flex-wrap gap-3">
							{TRADITIONS.map((tradition) => (
								<span
									key={tradition}
									className={`rounded-md px-3 py-2 text-sm font-semibold ${TRADITION_STYLE[tradition].chip}`}
								>
									{TRADITION_LABEL[tradition]}
								</span>
							))}
						</div>
					</div>
				</div>
			</section>

			<section
				id="first-brew"
				className="scroll-mt-8 border-t py-10 sm:py-14"
				aria-labelledby="example-heading"
			>
				<h2 id="example-heading" className="text-3xl sm:text-4xl">
					Your first brew, then a better one.
				</h2>
				<p className="mb-8 mt-4 max-w-2xl leading-relaxed text-muted-foreground">
					The mentor’s mix makes a Healing Potion (Minor). Follow the feedback,
					then try adding Glowcap to see what changes.
				</p>
				<BrewExample />
			</section>

			<section
				className="border-t py-10 sm:py-14"
				aria-labelledby="roll-heading"
			>
				<div className="grid gap-6 lg:grid-cols-[1fr_1.3fr] lg:gap-16">
					<div>
						<h2 id="roll-heading" className="text-3xl sm:text-4xl">
							Find the mix.
							<br />
							Roll for the batch.
						</h2>
						<p className="mt-4 leading-relaxed text-muted-foreground">
							Once a potion forms, roll a d20 (a 20-sided die) and add Wren’s
							Crafting modifier, the bonus for crafting skill. Meet the DC
							(difficulty number) to succeed. Alembic rolls for you, or you can
							enter a die roll from your table.
						</p>
						<p className="mt-4 text-sm leading-relaxed text-muted-foreground">
							Wren starts at +14. In the example: 6 + 14 = 20 against DC 15, a
							success. A perfect mix rewards precision, not just luck.
						</p>
					</div>
					<div>
						<div className="overflow-x-auto">
							<table className="w-full text-left text-sm leading-relaxed">
								<caption className="sr-only">
									Potions made by degree of success and mix quality
								</caption>
								<thead>
									<tr className="border-b">
										<th scope="col" className="pb-3 pr-3 font-semibold">
											Result
										</th>
										<th scope="col" className="pb-3 pr-3 font-semibold">
											Imperfect
										</th>
										<th scope="col" className="pb-3 font-semibold text-primary">
											Perfect
										</th>
									</tr>
								</thead>
								<tbody>
									{[
										["Critical success", "2 + a target clue", "2 + 1d3"],
										["Success", "1 potion", "1 + 1d3"],
										["Failure", "0 (curdled)", "1 salvaged"],
										["Critical failure", "0 potions", "0 potions"],
									].map(([degree, imperfect, perfect]) => (
										<tr key={degree} className="border-b">
											<th scope="row" className="py-4 pr-3 font-medium">
												{degree}
											</th>
											<td className="py-4 pr-3 text-muted-foreground">
												{imperfect}
											</td>
											<td className="py-4 text-primary">{perfect}</td>
										</tr>
									))}
								</tbody>
							</table>
						</div>
						<p className="mt-4 text-sm leading-relaxed text-muted-foreground">
							Meet the DC to succeed; 10 or more above it is a critical success.
							Below is failure; 10 or more below is critical failure. A natural
							20 improves the result one step; a natural 1 worsens it one step.
						</p>
						<p className="mt-3 text-sm leading-relaxed text-muted-foreground">
							1d3 means one roll yielding 1–3 extra potions. An imperfect
							critical success reveals the exact target for your most off-track
							tradition. Even a failed batch discovers the potion; a perfect mix
							marks it Perfected.
						</p>
					</div>
				</div>
			</section>

			<section
				className="border-t py-10 sm:py-14"
				aria-labelledby="gates-heading"
			>
				<div className="grid gap-6 lg:grid-cols-[1fr_1.3fr] lg:gap-16">
					<div>
						<h2 id="gates-heading" className="text-3xl sm:text-4xl">
							Not every discovery
							<br />
							is ready to bottle.
						</h2>
						<p className="mt-4 leading-relaxed text-muted-foreground">
							“Beyond” means you found a potion but cannot make it yet: the
							result tells you what is blocking it. “Inert” means nothing
							formed; a nearby target may leave a hint about which tradition
							needs more or less.
						</p>
					</div>
					<dl className="space-y-4 text-sm leading-relaxed">
						{[
							[
								"Rarity",
								"How unusual a potion is. Uncommon potions need an uncommon-or-rarer ingredient; rare potions need a rare one. Some also require a specific ingredient.",
							],
							[
								"Level",
								"A measure of power. The potion’s level cannot be higher than your crafter’s.",
							],
							[
								"Crafting rank",
								"Your skill tier. You need at least trained Crafting; rare potions require master.",
							],
							[
								"Alchemical Crafting",
								"A feat (a learned character ability) required to brew any potion. Wren already has it.",
							],
						].map(([term, definition]) => (
							<div key={term} className="border-b pb-4">
								<dt className="font-semibold">{term}</dt>
								<dd className="mt-1 text-muted-foreground">{definition}</dd>
							</div>
						))}
					</dl>
				</div>
			</section>

			<section
				className="border-t pt-10 sm:pt-14"
				aria-labelledby="loop-heading"
			>
				<div className="grid gap-8 lg:grid-cols-[1fr_1.3fr] lg:gap-16">
					<div>
						<h2 id="loop-heading" className="text-3xl sm:text-4xl">
							Keep your discoveries.
							<br />
							Make the next one.
						</h2>
						<div className="mt-7">
							<WorkshopLink />
						</div>
					</div>
					<div className="space-y-5 leading-relaxed text-muted-foreground">
						<p>
							Your{" "}
							<strong className="font-semibold text-foreground">codex</strong>{" "}
							is your potion discovery book. The{" "}
							<strong className="font-semibold text-foreground">
								brew log
							</strong>{" "}
							keeps your recent experiments. Your codex automatically saves your
							best combinations, ready to add back to the bench without
							rebuilding the mix.
						</p>
						<p>
							Running the game? Switch to{" "}
							<strong className="font-semibold text-foreground">
								GM (Game Master) view
							</strong>{" "}
							for the person guiding the story. See the answer key and reveal
							potions to the player without giving away their hidden targets.
						</p>
						<p className="text-sm">
							This is a homebrew crafting system: custom rules for your table,
							not the standard Pathfinder crafting rules.
						</p>
					</div>
				</div>
			</section>
		</main>
	);
}
