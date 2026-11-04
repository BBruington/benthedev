import { MinusIcon, PlusIcon, ShoppingCartIcon, Trash2Icon } from "lucide-react";
import type { Schemas } from "@/api/client";
import { Button } from "@/components/ui/button";
import { formatNumber } from "@/lib/colony";
import { cartLimit, itemKind, lotSize, offerText } from "@/lib/market";
import { UnitsForm } from "@/market/units-form";

type MarketCart = Schemas["MarketCart"];
type MarketItem = Schemas["MarketItem"];

export const cartHeadingId = "market-cart-heading";

/**
 * The party's one shared cart: any member changes a line, and anyone places the order. Supplies and
 * colonists are typed in units; a special item steps one at a time.
 */
export function CartPanel({
	cart,
	items,
	hasOpenOrder,
	isUpdating,
	onSetQuantity,
	onPlace,
}: {
	cart: MarketCart;
	items: readonly MarketItem[];
	hasOpenOrder: boolean;
	isUpdating: boolean;
	onSetQuantity: (itemId: string, quantity: number) => void;
	onPlace: () => void;
}) {
	return (
		<section
			aria-labelledby={cartHeadingId}
			className="flex flex-col gap-4 rounded-xl border border-gilt/40 bg-card p-4 shadow-card sm:p-5"
		>
			<h2
				className="flex items-center gap-2 font-display font-semibold text-2xl tracking-wide outline-none"
				id={cartHeadingId}
				tabIndex={-1}
			>
				<ShoppingCartIcon aria-hidden="true" className="size-5 text-gilt-ink" />
				Party cart
			</h2>
			{cart.lines.length === 0 ? (
				<p className="text-muted-foreground">
					The cart is empty. Add something from the catalog below.
				</p>
			) : (
				<ul className="flex flex-col divide-y divide-border">
					{cart.lines.map((line) => {
						const limit = cartLimit(items.find((item) => item.id === line.itemId));
						const sold = { ...line, priceGold: line.unitPrice };
						const isOffer = itemKind(line) !== "SPECIAL";
						return (
							<li className="flex flex-wrap items-center gap-x-3 gap-y-2 py-2" key={line.itemId}>
								<div className="min-w-0 flex-1">
									<p className="font-semibold [overflow-wrap:anywhere]">{line.name}</p>
									<p className="text-muted-foreground text-sm tabular-nums">
										{isOffer ? offerText(sold) : `${formatNumber(line.unitPrice)} gp each`}
									</p>
								</div>
								<p className="shrink-0 text-right font-semibold tabular-nums">
									{formatNumber(line.lineTotal)} gp
								</p>
								{isOffer ? (
									<UnitsForm
										currentLots={line.quantity}
										initialText={String(line.quantity * lotSize(line))}
										inputLabel={`${line.name} in the cart`}
										isUpdating={isUpdating}
										item={sold}
										key={line.quantity}
										limitLots={limit}
										onSubmit={(lots) => {
											onSetQuantity(line.itemId, lots);
										}}
										submitLabel="Update"
									>
										<Button
											aria-label={`Remove ${line.name}`}
											disabled={isUpdating}
											onClick={() => {
												onSetQuantity(line.itemId, 0);
											}}
											size="icon"
											type="button"
											variant="ghost"
										>
											<Trash2Icon />
										</Button>
									</UnitsForm>
								) : (
									<div className="flex items-center gap-1">
										<Button
											aria-label={
												line.quantity === 1 ? `Remove ${line.name}` : `One fewer ${line.name}`
											}
											disabled={isUpdating}
											onClick={() => {
												onSetQuantity(line.itemId, line.quantity - 1);
											}}
											size="icon"
											variant="outline"
										>
											<MinusIcon />
										</Button>
										<output
											aria-live="polite"
											className="min-w-10 text-center font-semibold tabular-nums"
										>
											{formatNumber(line.quantity)}
										</output>
										<Button
											aria-label={`One more ${line.name}`}
											disabled={isUpdating || line.quantity >= limit}
											onClick={() => {
												onSetQuantity(line.itemId, line.quantity + 1);
											}}
											size="icon"
											variant="outline"
										>
											<PlusIcon />
										</Button>
									</div>
								)}
							</li>
						);
					})}
				</ul>
			)}
			{cart.updatedByName ? (
				<p className="text-muted-foreground text-sm">Last changed by {cart.updatedByName}</p>
			) : null}
			<div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
				<p className="font-semibold tabular-nums">
					Total {formatNumber(cart.totalGold)} gold
					<span className="font-normal text-muted-foreground">, paid when the ship docks</span>
				</p>
				<Button
					className="h-12 text-base"
					disabled={cart.lines.length === 0 || hasOpenOrder || isUpdating}
					onClick={onPlace}
					size="lg"
				>
					Place order
				</Button>
			</div>
			{hasOpenOrder && cart.lines.length > 0 ? (
				<p className="text-muted-foreground text-sm">Pay for the ship at the docks first.</p>
			) : null}
		</section>
	);
}
