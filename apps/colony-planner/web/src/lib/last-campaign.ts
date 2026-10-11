import { readStored, writeStored } from "./storage";

// "/" goes straight to the campaign opened last (docs/design/multi-campaign.md §5.1), when the
// caller is still let in there. Only the slug is kept: it is in the address bar anyway.
const lastCampaignStorageKey = "colony-planner:last-campaign";

export function rememberLastCampaign(slug: string) {
	writeStored("local", lastCampaignStorageKey, slug);
}

export function readLastCampaign(): string | null {
	return readStored("local", lastCampaignStorageKey);
}

export function forgetLastCampaign() {
	writeStored("local", lastCampaignStorageKey, null);
}

// "/" jumps to the last campaign only when it is where the app lands (opening the site, or after
// sign-in), so "All campaigns" from inside a campaign shows the picker.
let appHasLanded = false;

export function markAppLanded() {
	appHasLanded = true;
}

export function hasAppLanded(): boolean {
	return appHasLanded;
}
