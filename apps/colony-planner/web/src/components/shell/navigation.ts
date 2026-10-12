import {
	CalendarDaysIcon,
	CastleIcon,
	CrownIcon,
	DramaIcon,
	HexagonIcon,
	InboxIcon,
	type LucideIcon,
	MapIcon,
	MapPinIcon,
	ScrollTextIcon,
	Settings2Icon,
	SwordsIcon,
	UserCheckIcon,
	UsersIcon,
} from "lucide-react";

export type Milestone = "M1" | "M2" | "M3" | "M4" | "M5";

export type NavigationItemId =
	| "colony"
	| "people"
	| "timeline"
	| "quests"
	| "patrons"
	| "requests"
	| "explore"
	| "mainland"
	| "places"
	| "npcs"
	| "sessions"
	| "players"
	| "settings";

export type NavigationItem = {
	id: NavigationItemId;
	/** Inside the campaign on screen: links pass its slug as the campaignSlug parameter. */
	to:
		| "/c/$campaignSlug"
		| "/c/$campaignSlug/people"
		| "/c/$campaignSlug/timeline"
		| "/c/$campaignSlug/quests"
		| "/c/$campaignSlug/patrons"
		| "/c/$campaignSlug/requests"
		| "/c/$campaignSlug/explore"
		| "/c/$campaignSlug/maps"
		| "/c/$campaignSlug/places"
		| "/c/$campaignSlug/npcs"
		| "/c/$campaignSlug/sessions"
		| "/c/$campaignSlug/players"
		| "/c/$campaignSlug/settings";
	label: string;
	icon: LucideIcon;
	/** The milestone that builds the screen; until then it shows a "Coming in" page. */
	arrivesIn?: Milestone;
	/** What the screen will hold, for its placeholder page. */
	summary: string;
	dmOnly?: boolean;
};

export const navigationItems: Record<NavigationItemId, NavigationItem> = {
	colony: {
		icon: CastleIcon,
		id: "colony",
		label: "Colony",
		summary: "The day clock, the stockpile, shipments and who works the colony.",
		to: "/c/$campaignSlug",
	},
	explore: {
		arrivesIn: "M4",
		icon: HexagonIcon,
		id: "explore",
		label: "Explore",
		summary:
			"The shared hex map of the new world, charted stroke by stroke by the players themselves.",
		to: "/c/$campaignSlug/explore",
	},
	mainland: {
		arrivesIn: "M4",
		icon: MapIcon,
		id: "mainland",
		label: "Mainland",
		summary: "The map of Eldari, with pins for every place the party knows.",
		to: "/c/$campaignSlug/maps",
	},
	npcs: {
		arrivesIn: "M2",
		icon: DramaIcon,
		id: "npcs",
		label: "NPCs",
		summary: "The people of Eldari and the colony, as far as the party knows them.",
		to: "/c/$campaignSlug/npcs",
	},
	patrons: {
		arrivesIn: "M3",
		icon: CrownIcon,
		id: "patrons",
		label: "Patrons",
		summary: "The colony's backers on the mainland, their favor, and what they can be asked for.",
		to: "/c/$campaignSlug/patrons",
	},
	people: {
		arrivesIn: "M2",
		icon: UsersIcon,
		id: "people",
		label: "People",
		summary:
			"Every role's headcount, the unassigned, and the named colonists who keep things running.",
		to: "/c/$campaignSlug/people",
	},
	places: {
		arrivesIn: "M2",
		icon: MapPinIcon,
		id: "places",
		label: "Places",
		summary: "Settlements, ruins and landmarks the party has found, and who can be met there.",
		to: "/c/$campaignSlug/places",
	},
	players: {
		dmOnly: true,
		icon: UserCheckIcon,
		id: "players",
		label: "Players",
		summary: "Who is waiting to join, who plays, and who was removed.",
		to: "/c/$campaignSlug/players",
	},
	quests: {
		arrivesIn: "M3",
		icon: SwordsIcon,
		id: "quests",
		label: "Quests",
		summary: "Active, finished and failed quests, with their rewards and who gave them.",
		to: "/c/$campaignSlug/quests",
	},
	requests: {
		arrivesIn: "M3",
		icon: InboxIcon,
		id: "requests",
		label: "Requests",
		summary: "What the party has asked of its patrons, and the DM's grants and declines.",
		to: "/c/$campaignSlug/requests",
	},
	sessions: {
		arrivesIn: "M5",
		icon: CalendarDaysIcon,
		id: "sessions",
		label: "Sessions",
		summary:
			"Upcoming game nights in your own time zone, recaps of past ones, and a calendar link.",
		to: "/c/$campaignSlug/sessions",
	},
	settings: {
		dmOnly: true,
		icon: Settings2Icon,
		id: "settings",
		label: "Settings",
		summary: "The campaign's name, time zone, resources and Discord channels.",
		to: "/c/$campaignSlug/settings",
	},
	timeline: {
		icon: ScrollTextIcon,
		id: "timeline",
		label: "Timeline",
		summary:
			"The colony's ledger: every day advanced, every ship that landed, every session played.",
		to: "/c/$campaignSlug/timeline",
	},
};

/** The sidebar's sections (tablet and desktop). */
export const navigationGroups: { label: string; items: NavigationItemId[]; dmOnly?: boolean }[] = [
	{ items: ["colony", "people", "timeline"], label: "Colony" },
	{ items: ["quests", "patrons", "requests"], label: "Ventures" },
	{ items: ["explore", "mainland", "places", "npcs"], label: "World" },
	{ items: ["sessions"], label: "Table" },
	{ dmOnly: true, items: ["players", "settings"], label: "Dungeon Master" },
];

/** The phone's bottom tabs, before More (spec §6.1). */
export const phoneTabs: NavigationItemId[] = ["colony", "explore", "quests", "timeline"];

/** Everything else, in the More sheet, in sidebar order. */
export const moreGroups = navigationGroups
	.map((group) => ({
		...group,
		items: group.items.filter((itemId) => !phoneTabs.includes(itemId)),
	}))
	.filter((group) => group.items.length > 0);
