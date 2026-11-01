import type {
	Check,
	Degree,
	Gate,
	Odds,
	Tradition,
	Values,
} from "#/brewing/types";

export const TRADITION_LABEL: Record<Tradition, string> = {
	arcane: "Arcane",
	primal: "Primal",
	occult: "Occult",
	divine: "Divine",
};

export function formatValues(values: Values): string {
	return values.join("/");
}

export function degreeLabel(degree: Degree): string {
	return {
		"critical-success": "Critical success",
		success: "Success",
		failure: "Failure",
		"critical-failure": "Critical failure",
	}[degree];
}

export function formatOdds(odds: Odds): string {
	return `Critical success ${Math.round(odds.criticalSuccess * 100)}% · Success ${Math.round(odds.success * 100)}% · Failure ${Math.round(odds.failure * 100)}% · Critical failure ${Math.round(odds.criticalFailure * 100)}%`;
}

export function formatGate(
	gate: Gate,
	crafterName: string,
	potionName: string,
): string {
	switch (gate.kind) {
		case "feat":
			return `${potionName} needs ${gate.feat}`;
		case "level":
			return `${potionName} is level ${gate.need} and ${crafterName} is level ${gate.have}`;
		case "rank":
			return `${gate.need === "master" ? "Rare potions" : potionName} need${gate.need === "master" ? "" : "s"} ${gate.need} Crafting (${crafterName} is ${gate.have})`;
		case "rarity":
			return `${potionName} needs ${gate.need === "uncommon" ? "an uncommon-or-rarer" : "a rare"} ingredient`;
		case "ingredient":
			return `${potionName} needs ${gate.ingredientId
				.split("-")
				.map((word) => word.charAt(0).toUpperCase() + word.slice(1))
				.join(" ")}`;
	}
}

export function formatCheck(check: Check): string {
	return `d20 ${check.d20} ${check.modifier < 0 ? "−" : "+"} ${Math.abs(check.modifier)} = ${check.total} vs DC ${check.dc} → ${degreeLabel(check.degree)}${check.tableRoll ? " (table roll)" : ""}`;
}
