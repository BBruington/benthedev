import { describe, expect, test } from "bun:test";
import {
	campaignCardLine,
	campaignEntry,
	findCampaignBySlug,
	inviteCodeFromText,
	inviteLink,
	type MyCampaign,
	membershipLabel,
	slugFromName,
	slugProblem,
	sortCampaigns,
} from "./campaigns";

function membership(overrides: {
	name?: string;
	slug?: string;
	role?: MyCampaign["role"];
	status?: MyCampaign["status"];
	campaignStatus?: MyCampaign["campaign"]["status"];
	characterName?: string | null;
}): MyCampaign {
	return {
		campaign: {
			id: `id-${overrides.slug ?? "sample-colony"}`,
			name: overrides.name ?? "Sample Colony",
			slug: overrides.slug ?? "sample-colony",
			status: overrides.campaignStatus ?? "ACTIVE",
		},
		characterName: overrides.characterName ?? null,
		memberId: "member",
		role: overrides.role ?? "PLAYER",
		status: overrides.status ?? "ACTIVE",
	};
}

describe("slugFromName", () => {
	test("lower-cases and joins words with hyphens", () => {
		expect(slugFromName("Sky Isles")).toBe("sky-isles");
	});

	test("cuts to 40 characters before trimming hyphens, as the migration does", () => {
		const slug = slugFromName("The Iron Coast of the Far Northern Reach - Year 2");
		expect(slug.length).toBeLessThanOrEqual(40);
		expect(slug.endsWith("-")).toBe(false);
		expect(slugProblem(slug)).toBeUndefined();
	});

	test("drops leading and trailing punctuation", () => {
		expect(slugFromName("  ¡Gull Haven!  ")).toBe("gull-haven");
	});
});

describe("slugProblem", () => {
	test("accepts a good address", () => {
		expect(slugProblem("second-sample")).toBeUndefined();
	});

	test("refuses short, long and badly formed addresses", () => {
		expect(slugProblem("ab")).toBeDefined();
		expect(slugProblem("a".repeat(41))).toBeDefined();
		expect(slugProblem("Sky-Isles")).toBeDefined();
		expect(slugProblem("sky--isles")).toBeDefined();
		expect(slugProblem("-sky")).toBeDefined();
	});
});

describe("inviteCodeFromText", () => {
	const code = "AbCdEfGhIjKlMnOpQrStUv";

	test("takes the code out of a whole link", () => {
		expect(inviteCodeFromText(`https://colony.example/join/${code}`)).toBe(code);
		expect(inviteCodeFromText(`  localhost:5173/join/${code}?ref=discord `)).toBe(code);
	});

	test("accepts a bare code", () => {
		expect(inviteCodeFromText(code)).toBe(code);
	});

	test("refuses anything else", () => {
		expect(inviteCodeFromText("")).toBeUndefined();
		expect(inviteCodeFromText("hello there")).toBeUndefined();
		expect(inviteCodeFromText("https://colony.example/c/sample-colony")).toBeUndefined();
	});
});

describe("campaignEntry", () => {
	test("a paused campaign is paused whatever the membership", () => {
		expect(campaignEntry(membership({ campaignStatus: "SUSPENDED", role: "DM" }))).toBe("paused");
		expect(campaignEntry(membership({ campaignStatus: "SUSPENDED", status: "PENDING" }))).toBe(
			"paused",
		);
	});

	test("follows the membership in an active campaign", () => {
		expect(campaignEntry(membership({}))).toBe("open");
		expect(campaignEntry(membership({ status: "PENDING" }))).toBe("waiting");
		expect(campaignEntry(membership({ status: "REMOVED" }))).toBe("no-access");
	});
});

describe("membershipLabel", () => {
	test("names the DM, the character, or a plain player", () => {
		expect(membershipLabel(membership({ role: "DM" }))).toBe("You're the DM");
		expect(membershipLabel(membership({ characterName: "Thorin" }))).toBe("You play Thorin");
		expect(membershipLabel(membership({ characterName: "  " }))).toBe("You're a player");
	});
});

describe("campaignCardLine", () => {
	test("says why a campaign isn't open, and the role where it is", () => {
		expect(campaignCardLine(membership({ characterName: "Thorin" }))).toBe("You play Thorin");
		expect(campaignCardLine(membership({ status: "PENDING" }))).toBe(
			"Waiting for the DM to let you in",
		);
		expect(campaignCardLine(membership({ status: "REMOVED" }))).toBe(
			"Only the DM can let you back in",
		);
		expect(campaignCardLine(membership({ campaignStatus: "SUSPENDED", role: "DM" }))).toBe(
			"Paused by the site owner · You're the DM",
		);
	});
});

describe("sortCampaigns and findCampaignBySlug", () => {
	test("open campaigns come first, then by name", () => {
		const sorted = sortCampaigns([
			membership({ name: "Zeta", slug: "zeta", status: "REMOVED" }),
			membership({ name: "Beta", slug: "beta", status: "PENDING" }),
			membership({ name: "Gamma", slug: "gamma" }),
			membership({ name: "Alpha", slug: "alpha" }),
		]);
		expect(sorted.map((entry) => entry.campaign.slug)).toEqual(["alpha", "gamma", "beta", "zeta"]);
	});

	test("finds by slug only", () => {
		const campaigns = [membership({ slug: "alpha" }), membership({ slug: "beta" })];
		expect(findCampaignBySlug(campaigns, "beta")?.campaign.id).toBe("id-beta");
		expect(findCampaignBySlug(campaigns, "id-beta")).toBeUndefined();
	});
});

test("inviteLink builds the shareable link", () => {
	expect(inviteLink("http://localhost:5173", "abc_DEF-123")).toBe(
		"http://localhost:5173/join/abc_DEF-123",
	);
});
