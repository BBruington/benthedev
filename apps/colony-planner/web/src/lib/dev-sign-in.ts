import type { Schemas } from "@/api/client";

type DevSignInAccount = Schemas["DevSignInAccount"];
type DevSignInMembership = Schemas["DevSignInMembership"];

function describeMembership(membership: DevSignInMembership): string {
	const campaignName = membership.campaignName;
	if (membership.role === "DM") {
		return `DM of ${campaignName}`;
	}
	if (membership.status === "PENDING") {
		return `waiting to join ${campaignName}`;
	}
	if (membership.status === "REMOVED") {
		return `removed from ${campaignName}`;
	}
	const characterName = membership.characterName?.trim();
	if (characterName) {
		return `${characterName} in ${campaignName}`;
	}
	return `player in ${campaignName}`;
}

/**
 * One line per account for the dev sign-in list (docs/design/multi-campaign.md §7.3), such as
 * "site owner · DM of Sample Colony" or "Thorin in Sample Colony · Brisa in Second Sample".
 */
export function devSignInDescription(account: DevSignInAccount): string {
	const parts: string[] = [];
	if (account.isSiteOwner) {
		parts.push("site owner");
	}
	const dmFirst = [...account.memberships].sort((first, second) => {
		if (first.role !== second.role) {
			return first.role === "DM" ? -1 : 1;
		}
		return first.campaignName.localeCompare(second.campaignName);
	});
	for (const membership of dmFirst) {
		parts.push(describeMembership(membership));
	}
	if (account.askingToRunCampaign) {
		parts.push("asking to run a campaign");
	}
	if (parts.length === 0) {
		return "no campaigns yet";
	}
	return parts.join(" · ");
}
