import { ChevronDownIcon, PencilIcon, PlusIcon } from "lucide-react";
import { type ReactNode, useState } from "react";
import type { Schemas } from "@/api/client";
import { Button } from "@/components/ui/button";
import { formatNumber } from "@/lib/colony";
import { cartLimit, itemKind, offerText, unsellableResourceKeys } from "@/lib/market";
import { UnitsForm } from "@/market/units-form";
import { ArchivedBadge } from "@/ventures/venture-parts";

type MarketItem = Schemas["MarketItem"];
type Labelled = { key: string; label: string };

/** A resource or role the DM prices: its offer, or null while it has never been sold. */
export type OfferTarget =
	| { kind: "SUPPLY"; key: string; label: string; offer: MarketItem | null }
	| { kind: "COLONIST"; key: string; label: string; offer: MarketItem | null };

export const catalogHeadingId = "market-catalog-heading";

/**
 * Supplies (one row per buyable resource), Colonists (one per role) and Special items. Players see only
 * what is for sale; the DM also sees what isn't, and prices it from here.
 */
export function CatalogList({
	items,
	cartQuantities,
	resources,
	roles,
	isDm,
	isUpdatingCart,
	onSetQuantity,
	onEditItem,
	onCreate,
	onEditOffer,
}: {
	items: readonly MarketItem[];
	cartQuantities: ReadonlyMap<string, number>;
	resources: readonly Labelled[];
	roles: readonly Labelled[];
	isDm: boolean;
	isUpdatingCart: boolean;
	onSetQuantity: (itemId: string, quantity: number, onSuccess?: () => void) => void;
	onEditItem: (item: MarketItem) => void;
	onCreate: () => void;
	onEditOffer: (target: OfferTarget) => void;
}) {
	const [showArchived, setShowArchived] = useState(false);
	const supplies: OfferTarget[] = resources
		.filter((resource) => !unsellableResourceKeys.includes(resource.key))
		.map((resource) => ({
			key: resource.key,
			kind: "SUPPLY",
			label: resource.label,
			offer: items.find((item) => item.resourceKey === resource.key) ?? null,
		}));
	const colonists: OfferTarget[] = roles.map((role) => ({
		key: role.key,
		kind: "COLONIST",
		label: role.label,
		offer: items.find((item) => item.roleKey === role.key) ?? null,
	}));
	const visibleSupplies = supplies.filter(
		(target) => isDm || (target.offer !== null && !target.offer.archived),
	);
	const visibleColonists = colonists.filter(
		(target) => isDm || (target.offer !== null && !target.offer.archived),
	);
	const specials = items.filter((item) => itemKind(item) === "SPECIAL");
	const specialsForSale = specials.filter((item) => !item.archived);
	const archivedSpecials = specials.filter((item) => item.archived);
	const isEmpty =
		visibleSupplies.length === 0 && visibleColonists.length === 0 && specialsForSale.length === 0;

	function offerRow(target: OfferTarget) {
		return (
			<OfferRow
				cartQuantity={target.offer ? (cartQuantities.get(target.offer.id) ?? 0) : 0}
				isDm={isDm}
				isUpdatingCart={isUpdatingCart}
				key={target.key}
				onEditOffer={onEditOffer}
				onSetQuantity={onSetQuantity}
				target={target}
			/>
		);
	}

	function specialRow(item: MarketItem) {
		return (
			<SpecialRow
				cartQuantity={cartQuantities.get(item.id) ?? 0}
				isDm={isDm}
				isUpdatingCart={isUpdatingCart}
				item={item}
				key={item.id}
				onEditItem={onEditItem}
				onSetQuantity={onSetQuantity}
			/>
		);
	}

	return (
		<section aria-labelledby={catalogHeadingId} className="flex flex-col gap-4">
			<h2
				className="font-display font-semibold text-2xl tracking-wide outline-none"
				id={catalogHeadingId}
				tabIndex={-1}
			>
				Catalog
			</h2>
			{isEmpty && !isDm ? (
				<p className="rounded-xl border border-border border-dashed px-4 py-4 text-muted-foreground">
					Nothing for sale yet. Ask the DM what the mainland sells.
				</p>
			) : null}
			{visibleSupplies.length > 0 ? (
				<CatalogSection title="Supplies">{visibleSupplies.map(offerRow)}</CatalogSection>
			) : null}
			{visibleColonists.length > 0 ? (
				<CatalogSection title="Colonists">{visibleColonists.map(offerRow)}</CatalogSection>
			) : null}
			{specialsForSale.length > 0 || isDm ? (
				<CatalogSection
					action={
						isDm ? (
							<Button onClick={onCreate} size="sm" variant="outline">
								<PlusIcon />
								New special item
							</Button>
						) : null
					}
					title="Special items"
				>
					{specialsForSale.length > 0 ? (
						specialsForSale.map(specialRow)
					) : (
						<li className="px-4 py-3 text-muted-foreground text-sm">
							Magic and one-of-a-kind goods the party asks for. You hand them over at the table.
						</li>
					)}
				</CatalogSection>
			) : null}
			{isDm && archivedSpecials.length > 0 ? (
				<div className="grid gap-2 rounded-lg border border-border">
					<button
						aria-expanded={showArchived}
						className="flex min-h-11 cursor-pointer items-center justify-between gap-3 rounded-lg px-3 text-left font-medium outline-none focus-visible:ring-[3px] focus-visible:ring-ring/45"
						onClick={() => {
							setShowArchived((current) => !current);
						}}
						type="button"
					>
						Archived special items ({formatNumber(archivedSpecials.length)})
						<ChevronDownIcon
							aria-hidden="true"
							className={showArchived ? "size-5 rotate-180" : "size-5"}
						/>
					</button>
					{showArchived ? (
						<ul className="flex flex-col divide-y divide-border border-border border-t">
							{archivedSpecials.map(specialRow)}
						</ul>
					) : null}
				</div>
			) : null}
		</section>
	);
}

function CatalogSection({
	title,
	action,
	children,
}: {
	title: string;
	action?: ReactNode;
	children: ReactNode;
}) {
	return (
		<div className="flex flex-col gap-2">
			<div className="flex min-h-9 items-center justify-between gap-3">
				<h3 className="font-medium text-muted-foreground text-sm">{title}</h3>
				{action}
			</div>
			<ul className="flex flex-col divide-y divide-border rounded-xl border border-border bg-card">
				{children}
			</ul>
		</div>
	);
}

function OfferRow({
	target,
	cartQuantity,
	isDm,
	isUpdatingCart,
	onSetQuantity,
	onEditOffer,
}: {
	target: OfferTarget;
	cartQuantity: number;
	isDm: boolean;
	isUpdatingCart: boolean;
	onSetQuantity: (itemId: string, quantity: number, onSuccess?: () => void) => void;
	onEditOffer: (target: OfferTarget) => void;
}) {
	const offer = target.offer;
	if (offer === null || offer.archived) {
		return (
			<li className="flex flex-wrap items-center gap-3 px-4 py-3">
				<div className="min-w-0 flex-1">
					<p className="font-semibold [overflow-wrap:anywhere]">{target.label}</p>
					<p className="text-muted-foreground text-sm">Not for sale</p>
				</div>
				<Button
					aria-label={`Set a price for ${target.label}`}
					onClick={() => {
						onEditOffer(target);
					}}
					size="sm"
					variant="outline"
				>
					Set price
				</Button>
			</li>
		);
	}
	return (
		<li className="flex flex-wrap items-center gap-x-3 gap-y-2 px-4 py-3">
			<div className="min-w-0 grow basis-24">
				<p className="font-semibold [overflow-wrap:anywhere]">{offer.name}</p>
				<p className="text-muted-foreground text-sm tabular-nums">{offerText(offer)}</p>
			</div>
			<UnitsForm
				inCartLots={cartQuantity}
				inputLabel={`${offer.name} to add`}
				isUpdating={isUpdatingCart}
				item={offer}
				limitLots={cartLimit(offer)}
				onSubmit={(lots, clear) => {
					onSetQuantity(offer.id, cartQuantity + lots, clear);
				}}
				submitLabel="Add"
			>
				{isDm ? (
					<Button
						aria-label={`Edit ${offer.name} price`}
						onClick={() => {
							onEditOffer(target);
						}}
						size="icon"
						type="button"
						variant="ghost"
					>
						<PencilIcon />
					</Button>
				) : null}
			</UnitsForm>
		</li>
	);
}

function SpecialRow({
	item,
	cartQuantity,
	isDm,
	isUpdatingCart,
	onSetQuantity,
	onEditItem,
}: {
	item: MarketItem;
	cartQuantity: number;
	isDm: boolean;
	isUpdatingCart: boolean;
	onSetQuantity: (itemId: string, quantity: number, onSuccess?: () => void) => void;
	onEditItem: (item: MarketItem) => void;
}) {
	const isAtLimit = cartQuantity >= cartLimit(item);
	return (
		<li className="flex items-center gap-3 px-4 py-3">
			<div className="min-w-0 flex-1">
				<p className="flex flex-wrap items-center gap-2 font-semibold [overflow-wrap:anywhere]">
					{item.name}
					<ArchivedBadge archived={item.archived} />
				</p>
				<p className="text-muted-foreground text-sm tabular-nums">
					{formatNumber(item.priceGold)} gp
					{item.maxPerOrder === null ? null : ` (${formatNumber(item.maxPerOrder)} per order)`}
				</p>
				{item.description ? (
					<p className="mt-1 text-sm [overflow-wrap:anywhere]">{item.description}</p>
				) : null}
			</div>
			<div className="flex shrink-0 items-center gap-1">
				{item.archived ? null : (
					<Button
						disabled={isAtLimit || isUpdatingCart}
						onClick={() => {
							onSetQuantity(item.id, cartQuantity + 1);
						}}
						size="sm"
						variant="outline"
					>
						{isAtLimit ? "In cart" : "Add"}
					</Button>
				)}
				{isDm ? (
					<Button
						aria-label={`Edit ${item.name}`}
						onClick={() => {
							onEditItem(item);
						}}
						size="icon"
						variant="ghost"
					>
						<PencilIcon />
					</Button>
				) : null}
			</div>
		</li>
	);
}
