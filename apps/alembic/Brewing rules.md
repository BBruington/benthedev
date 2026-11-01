# Brewing rules

Homebrew potion crafting for PF2e. All ingredient and potion content here is fictional sample data.

## 1. Traditions

Every ingredient carries four tradition values, one per PF2e magical tradition: **Arcane, Primal, Occult, Divine** (always in that order, written `A/P/O/D`).

## 2. The mix

A brew is 1-4 units of ingredients, repeats allowed. The mix's **totals** are the per-tradition sums of every unit. Every unit comes out of the campaign's ingredient inventory, so a brew needs at least as many of each ingredient in stock as it uses.

## 3. Potions

Each potion has a hidden **target** (`A/P/O/D`) and:

- **primary**: the tradition with the largest target value;
- **primary room**: how far the mix's primary total may sit from the target's primary value;
- **secondary budget**: the most the mix's other three traditions may deviate from the target, summed;
- level, rarity (common, uncommon, rare) and optionally a **required ingredient**.

**Off by** = the sum over all four traditions of `|total - target|`. A mix is **perfect** when off by is 0.

## 4. Resolution order

1. **Sum** the units into totals.
2. **Shortlist** every potion whose primary is within its primary room: `|total[primary] - target[primary]| <= primaryRoom`.
3. **Secondary budget**: drop shortlisted potions whose summed deviation on the three non-primary traditions exceeds the budget. If none remain, the brew is **Inert** (§8).
4. **Ingredient gates**: an uncommon potion needs at least one uncommon-or-rarer unit in the mix; a rare potion needs at least one rare unit. A required ingredient must be in the mix.
5. **Crafter gates**:
   - **feat**: every potion needs the Alchemical Crafting feat;
   - **level**: potion level must be at most the crafter's level;
   - **rank**: Crafting must be at least trained; rare potions need master.
6. Among the remaining candidates, the one with the lowest off by forms (ties: higher level, then catalog order). If step 3 left candidates but none pass steps 4-5, the brew is **Beyond** (§9). Otherwise roll the **Crafting check** against `DC = level DC (§6) + rarity adjustment` (uncommon +2, rare +5).
7. Apply the **outcome table** (§7).

Every brew, whatever its result, costs one downtime day.

## 5. The Crafting check

`modifier = proficiency + Int + tools`

- proficiency: untrained +0; trained, expert, master and legendary are crafter level + 2, + 4, + 6 and + 8;
- tools: none -2, basic alchemist's tools +0, expanded alchemist's lab +1.

The d20 is rolled with crypto unless the table's d20 (1-20) is entered. Degree of success is PF2e's:

- total ≥ DC + 10: critical success;
- total ≥ DC: success;
- total ≤ DC - 10: critical failure;
- otherwise failure;
- a natural 20 improves the degree one step, a natural 1 worsens it one step.

**Odds** are the probability of each degree over the 20 faces of the d20.

## 6. Level DCs

| Level | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 |
|---|---|---|---|---|---|---|---|---|---|---|---|
| DC | 14 | 15 | 16 | 18 | 19 | 20 | 22 | 23 | 24 | 26 | 27 |

## 7. Outcome table

The potion **forms** in every row (it becomes Known). `1d3` is always rolled with crypto.

| Degree | Imperfect mix | Perfect mix |
|---|---|---|
| Critical success | 2 potions, plus the exact target of the worst-off tradition | 2 + 1d3 potions |
| Success | 1 potion | 1 + 1d3 potions |
| Failure | 0 potions (the batch curdles) | 1 potion (salvaged) |
| Critical failure | 0 potions | 0 potions |

The result shows the off by and one arrow per tradition: **on** (equal), **low** (below target) or **high** (above). A perfect mix marks the potion **Perfected** in the codex whatever the degree.

## 8. Inert

At most one hint. Take the potion with the lowest off by against the totals; if that off by is 20 or less, hint at its worst-off tradition ("the mix wants more Occult" / "less Arcane") without naming the potion. Otherwise no hint.

## 9. Beyond

Name the closest blocked candidate (lowest off by after step 3) and **every** gate it fails, e.g. "Truth Potion is level 6 and Wren is level 5".

## 10. Codex

- Known: a potion that has formed, been revealed by the GM, or arrived with a mentor's note.
- Perfected: any formed brew with a perfect mix.
- Known entries show name, rarity, level, DC, the crafter's odds and the best off by so far. Players never see targets, rooms or required ingredients. The GM may reveal a perfect formula; the codex then offers it for Add to bench. Without a revealed formula, the player's own best formed combination takes precedence over the mentor's starting mix.

## 11. House rules

1. Potions are found by measuring traditions, not by learning formulas.
2. Brewing uses up its ingredients; you can't brew with more than you have.
3. Every brew costs one downtime day.
4. A perfect mix yields 1d3 extra potions on a success.
5. A failed check with a perfect mix still salvages one potion.
6. Rare potions need master Crafting.
7. The GM can reveal a potion by name or share its formula; its target stays hidden. Hiding a reveal never removes brewed or mentor knowledge.

## 12. Worked examples

Wren Ashby: level 5, Int +4, expert Crafting (+9), Alchemical Crafting, expanded lab (+1): **modifier +14**.

1. **Heartmoss + Marsh Toad Gland** = `7/26/1/8`. Healing Potion (Minor) (target `7/26/10/13`, Primal room 4, budget 16): primary 26 is on; secondary deviation 0 + 9 + 5 = 14 ≤ 16. Forms against DC 15, off by 14; arrows Arcane on, Primal on, Occult low, Divine low.
2. **+ Glowcap** = `7/26/10/13`: off by 0, perfect. With d20 = 6: 20 vs DC 15, success: 1 + 1d3 potions; Perfected.
3. Example 1 with d20 = 11: 25 ≥ DC 15 + 10, critical success: 2 potions, worst-off tradition Occult, target 10.
4. Example 1 with d20 = 1: 15 would succeed, the natural 1 drops it to failure: 0 potions. Example 2 with d20 = 1: failure, perfect: 1 potion.
5. **Silverleaf + Ghost Orchid + Moonwell Water** = `6/6/20/25`: Truth Potion (level 6) shortlists, passes its uncommon gate, and is Beyond: "level 6, Wren is level 5".
6. **Ember Salt + Wyrm Scale Dust + Sunpetal** = `24/14/2/13`: Potion of Fire Resistance (level 5, uncommon, DC 22). d20 = 1: 15 is failure, the natural 1 makes it a critical failure. d20 = 18: 32 ≥ 32, critical success.
7. **Ember Salt** alone = `10/3/0/1`: nothing shortlists. Closest potion is Potion of Leaping (`20/6/0/2`, off by 14): hint "more Arcane".
8. Wren's odds against DC 15: critical success on 11-20 (50%), success on 2-10 (45%), failure on 1 (5%).
