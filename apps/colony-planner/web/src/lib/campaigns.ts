import type { Schemas } from "@/api/client";

// Pure helpers for the campaign picker, the switcher, /request and /join
// (docs/design/multi-campaign.md §3.1, §5).

export type MyCampaign = Schemas["MyCampaign"];

/** Where a membership leaves its holder: in the game, at the door, shut out, or paused. */
export type CampaignEntry = "open" | "waiting" | "no-access" | "paused";

const slugMaximumLength = 40;
const slugMinimumLength = 3;
const notSlugCharacters = /[^a-z0-9]+/g;
const edgeHyphens = /^-+|-+$/g;
const slugPattern = /^[a-z0-9]+(-[a-z0-9]+)*$/;
const inviteCodePattern = /^[A-Za-z0-9_-]{22,64}$/;
const joinPathPattern = /\/join\/([A-Za-z0-9_-]+)/;

/** The suggested address for a campaign name: the server's rule, so the suggestion is accepted. */
export function slugFromName(name: string): string {
	return name
		.toLowerCase()
		.replace(notSlugCharacters, "-")
		.slice(0, slugMaximumLength)
		.replace(edgeHyphens, "");
}

/** Why an address won't do, in the server's words, or undefined when it will. */
export function slugProblem(slug: string): string | undefined {
	if (slug.length < slugMinimumLength || slug.length > slugMaximumLength) {
		return `Use ${slugMinimumLength} to ${slugMaximumLength} characters.`;
	}
	if (!slugPattern.test(slug)) {
		return "Use lowercase letters, digits and single hyphens, with no hyphen at either end.";
	}
	return undefined;
}

/**
 * The invite code in whatever was pasted: a whole /join link (any host) or the bare code. Undefined
 * when there is none, so the picker can say so instead of opening a dead link.
 */
export function inviteCodeFromText(text: string): string | undefined {
	const trimmed = text.trim();
	const fromPath = joinPathPattern.exec(trimmed)?.[1];
	const candidate = fromPath ?? trimmed;
	if (inviteCodePattern.test(candidate)) {
		return candidate;
	}
	return undefined;
}

export function campaignEntry(membership: MyCampaign): CampaignEntry {
	if (membership.campaign.status === "SUSPENDED") {
		return "paused";
	}
	if (membership.status === "ACTIVE") {
		return "open";
	}
	if (membership.status === "PENDING") {
		return "waiting";
	}
	return "no-access";
}

/** What the picker and the switcher say about the holder's place in a campaign. */
export function membershipLabel(membership: MyCampaign): string {
	if (membership.role === "DM") {
		return "You're the DM";
	}
	const characterName = membership.characterName?.trim();
	if (characterName) {
		return `You play ${characterName}`;
	}
	return "You're a player";
}

/** The picker's and switcher's second line: the role where it's open, else why it isn't. */
export function campaignCardLine(membership: MyCampaign): string {
	const entry = campaignEntry(membership);
	if (entry === "waiting") {
		return "Waiting for the DM to let you in";
	}
	if (entry === "no-access") {
		return "Only the DM can let you back in";
	}
	if (entry === "paused") {
		return `Paused by the site owner · ${membershipLabel(membership)}`;
	}
	return membershipLabel(membership);
}

export const campaignEntryLabels: Record<CampaignEntry, string> = {
	"no-access": "No access",
	open: "Open",
	paused: "Paused",
	waiting: "Waiting",
};

export function findCampaignBySlug(
	campaigns: readonly MyCampaign[],
	slug: string,
): MyCampaign | undefined {
	return campaigns.find((membership) => membership.campaign.slug === slug);
}

/** Open campaigns first, then the ones waiting, then the rest; each group by name. */
export function sortCampaigns(campaigns: readonly MyCampaign[]): MyCampaign[] {
	const order: Record<CampaignEntry, number> = { "no-access": 3, open: 0, paused: 2, waiting: 1 };
	return [...campaigns].sort((first, second) => {
		const byEntry = order[campaignEntry(first)] - order[campaignEntry(second)];
		if (byEntry !== 0) {
			return byEntry;
		}
		return first.campaign.name.localeCompare(second.campaign.name);
	});
}

/** The full invite link the DM shares; the site's own origin, so it works wherever the app runs. */
export function inviteLink(origin: string, inviteCode: string): string {
	return `${origin}/join/${encodeURIComponent(inviteCode)}`;
}
