import { expect, test } from "bun:test";
import { devSignInDescription } from "./dev-sign-in";

test("lists the owner flag, DM roles first, characters and an open request", () => {
	expect(
		devSignInDescription({
			accountId: "sample-dm",
			askingToRunCampaign: false,
			displayName: "Sample DM",
			isSiteOwner: true,
			memberships: [
				{
					campaignName: "Sample Colony",
					campaignSlug: "sample-colony",
					characterName: null,
					role: "DM",
					status: "ACTIVE",
				},
			],
		}),
	).toBe("site owner · DM of Sample Colony");
	expect(
		devSignInDescription({
			accountId: "hollis",
			askingToRunCampaign: false,
			displayName: "Hollis",
			isSiteOwner: false,
			memberships: [
				{
					campaignName: "Sample Colony",
					campaignSlug: "sample-colony",
					characterName: "Pell",
					role: "PLAYER",
					status: "ACTIVE",
				},
				{
					campaignName: "Second Sample",
					campaignSlug: "second-sample",
					characterName: null,
					role: "DM",
					status: "ACTIVE",
				},
			],
		}),
	).toBe("DM of Second Sample · Pell in Sample Colony");
	expect(
		devSignInDescription({
			accountId: "wren",
			askingToRunCampaign: true,
			displayName: "Wren",
			isSiteOwner: false,
			memberships: [],
		}),
	).toBe("asking to run a campaign");
});

test("names waiting and removed memberships", () => {
	expect(
		devSignInDescription({
			accountId: "jun",
			askingToRunCampaign: false,
			displayName: "Jun",
			isSiteOwner: false,
			memberships: [
				{
					campaignName: "Second Sample",
					campaignSlug: "second-sample",
					characterName: null,
					role: "PLAYER",
					status: "PENDING",
				},
			],
		}),
	).toBe("waiting to join Second Sample");
});
