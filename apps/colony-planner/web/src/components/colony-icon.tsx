import {
	AnchorIcon,
	AppleIcon,
	AxeIcon,
	BeefIcon,
	BeerIcon,
	BoneIcon,
	BookOpenIcon,
	BoxesIcon,
	CoinsIcon,
	CrownIcon,
	DropletsIcon,
	FeatherIcon,
	FishIcon,
	FlameIcon,
	FlaskConicalIcon,
	GemIcon,
	HammerIcon,
	HardHatIcon,
	HeartHandshakeIcon,
	HeartIcon,
	HeartPulseIcon,
	LeafIcon,
	type LucideIcon,
	type LucideProps,
	MountainIcon,
	PackageIcon,
	PickaxeIcon,
	ScrollIcon,
	ShieldIcon,
	ShipIcon,
	ShirtIcon,
	SparklesIcon,
	StarIcon,
	SwordIcon,
	TentIcon,
	TreesIcon,
	UsersIcon,
	WandSparklesIcon,
	WheatIcon,
	WrenchIcon,
} from "lucide-react";
import type { Schemas } from "@/api/client";

export type ColonyIconName = Schemas["ColonyIcon"];

const wordBoundary = /([a-z])([A-Z])/g;

/**
 * The curated icons resources and roles may use (spec §4.2). The Record makes a name added to the
 * Postgres enum fail typechecking here until it has its lucide component.
 */
export const colonyIcons: Record<ColonyIconName, LucideIcon> = {
	Anchor: AnchorIcon,
	Apple: AppleIcon,
	Axe: AxeIcon,
	Beef: BeefIcon,
	Beer: BeerIcon,
	Bone: BoneIcon,
	BookOpen: BookOpenIcon,
	Boxes: BoxesIcon,
	Coins: CoinsIcon,
	Crown: CrownIcon,
	Droplets: DropletsIcon,
	Feather: FeatherIcon,
	Fish: FishIcon,
	Flame: FlameIcon,
	FlaskConical: FlaskConicalIcon,
	Gem: GemIcon,
	Hammer: HammerIcon,
	HardHat: HardHatIcon,
	Heart: HeartIcon,
	HeartHandshake: HeartHandshakeIcon,
	HeartPulse: HeartPulseIcon,
	Leaf: LeafIcon,
	Mountain: MountainIcon,
	Package: PackageIcon,
	Pickaxe: PickaxeIcon,
	Scroll: ScrollIcon,
	Shield: ShieldIcon,
	Ship: ShipIcon,
	Shirt: ShirtIcon,
	Sparkles: SparklesIcon,
	Star: StarIcon,
	Sword: SwordIcon,
	Tent: TentIcon,
	Trees: TreesIcon,
	Users: UsersIcon,
	WandSparkles: WandSparklesIcon,
	Wheat: WheatIcon,
	Wrench: WrenchIcon,
};

/** Every curated icon name, in a stable order for pickers. */
export const colonyIconNames = Object.keys(colonyIcons).filter(isColonyIconName).sort();

function isColonyIconName(name: string): name is ColonyIconName {
	return Object.hasOwn(colonyIcons, name);
}

/** "HeartHandshake" → "Heart handshake", for screen readers and tooltips. */
export function colonyIconLabel(name: ColonyIconName): string {
	const words = name.replace(wordBoundary, "$1 $2").toLowerCase();
	return words.charAt(0).toUpperCase() + words.slice(1);
}

export function ColonyIcon({ name, ...props }: { name: ColonyIconName } & LucideProps) {
	const Icon = colonyIcons[name];
	return <Icon aria-hidden="true" {...props} />;
}
