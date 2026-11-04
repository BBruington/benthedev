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
	StoreIcon,
	SwordsIcon,
	UserCheckIcon,
	UsersIcon,
} from "lucide-react";

export type NavigationItemId =
	| "colony"
	| "people"
	| "market"
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
		| "/c/$campaignSlug/market"
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
	dmOnly?: boolean;
};

export const navigationItems: Record<NavigationItemId, NavigationItem> = {
	colony: {
		icon: CastleIcon,
		id: "colony",
		label: "Colony",
		to: "/c/$campaignSlug",
	},
	explore: {
		icon: HexagonIcon,
		id: "explore",
		label: "Explore",
		to: "/c/$campaignSlug/explore",
	},
	mainland: {
		icon: MapIcon,
		id: "mainland",
		label: "Mainland",
		to: "/c/$campaignSlug/maps",
	},
	market: {
		icon: StoreIcon,
		id: "market",
		label: "Market",
		to: "/c/$campaignSlug/market",
	},
	npcs: {
		icon: DramaIcon,
		id: "npcs",
		label: "NPCs",
		to: "/c/$campaignSlug/npcs",
	},
	patrons: {
		icon: CrownIcon,
		id: "patrons",
		label: "Patrons",
		to: "/c/$campaignSlug/patrons",
	},
	people: {
		icon: UsersIcon,
		id: "people",
		label: "People",
		to: "/c/$campaignSlug/people",
	},
	places: {
		icon: MapPinIcon,
		id: "places",
		label: "Places",
		to: "/c/$campaignSlug/places",
	},
	players: {
		dmOnly: true,
		icon: UserCheckIcon,
		id: "players",
		label: "Players",
		to: "/c/$campaignSlug/players",
	},
	quests: {
		icon: SwordsIcon,
		id: "quests",
		label: "Quests",
		to: "/c/$campaignSlug/quests",
	},
	requests: {
		icon: InboxIcon,
		id: "requests",
		label: "Requests",
		to: "/c/$campaignSlug/requests",
	},
	sessions: {
		icon: CalendarDaysIcon,
		id: "sessions",
		label: "Sessions",
		to: "/c/$campaignSlug/sessions",
	},
	settings: {
		dmOnly: true,
		icon: Settings2Icon,
		id: "settings",
		label: "Settings",
		to: "/c/$campaignSlug/settings",
	},
	timeline: {
		icon: ScrollTextIcon,
		id: "timeline",
		label: "Timeline",
		to: "/c/$campaignSlug/timeline",
	},
};

/** The sidebar's sections (tablet and desktop). */
export const navigationGroups: { label: string; items: NavigationItemId[]; dmOnly?: boolean }[] = [
	{ items: ["colony", "people", "market", "timeline"], label: "Colony" },
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
