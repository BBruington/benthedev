import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { ChevronDownIcon, ShipIcon, StoreIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { problemMessage } from "@/api/problem";
import {
	colonyBoardQuery,
	marketQuery,
	recentTimelineQuery,
	timelineQueryKeyPrefix,
} from "@/api/queries";
import { useColonyChanges } from "@/colony/use-colony-changes";
import { PageHeader } from "@/components/page-header";
import { StatusPanel } from "@/components/status-panel";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { fallbackCampaignName, useCampaignId, useCampaignName } from "@/hooks/use-campaign";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { useMe } from "@/hooks/use-me";
import { useSheetState } from "@/hooks/use-sheet-state";
import { canUseDmTools } from "@/lib/access";
import { formatNumber } from "@/lib/colony";
import { orderWaitText } from "@/lib/market";
import { countOf } from "@/lib/words";
import { CartPanel } from "@/market/cart-panel";
import { CatalogList, type OfferTarget } from "@/market/catalog-list";
import { ItemSheet } from "@/market/item-sheet";
import { OfferSheet } from "@/market/offer-sheet";
import { PaySheet } from "@/market/pay-sheet";
import { PlaceOrderSheet } from "@/market/place-order-sheet";
import { CancelOrderDialog, ShipList } from "@/market/ship-card";
import { ShippingSheet } from "@/market/shipping-sheet";
import { focusAfterClose } from "@/people/colonist-actions";

type MarketItem = Schemas["MarketItem"];
type MarketOrder = Schemas["MarketOrder"];
type MarketPage = Schemas["MarketPage"];

export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell/shipments")({
	component: MarketScreen,
	loader: ({ context: { campaignId, queryClient } }) => {
		if (!campaignId) {
			return;
		}
		queryClient.prefetchQuery(marketQuery(campaignId));
		queryClient.prefetchQuery(colonyBoardQuery(campaignId));
	},
});

const marketHeadingId = "market-heading";

/**
 * /shipments: the party's orders from the mainland. Any member fills the shared draft and places an
 * order while a ship is free, anyone pays when one docks, and the DM keeps the catalog, the voyage
 * length and how many ships may be out at once.
 */
function MarketScreen() {
	useDocumentTitle("Shipments", useCampaignName() ?? fallbackCampaignName);
	const me = useMe();
	const isDm = canUseDmTools(me);
	const campaignId = useCampaignId();
	const queryClient = useQueryClient();
	const market = useQuery(marketQuery(campaignId));
	const board = useQuery(colonyBoardQuery(campaignId));
	const changes = useColonyChanges();
	const itemSheet = useSheetState<MarketItem | "new">();
	const shippingSheet = useSheetState<{ voyageDays: number; shipLimit: number }>();
	const offerSheet = useSheetState<OfferTarget>();
	const paySheet = useSheetState<MarketOrder>();
	const placeSheet = useSheetState<true>();
	const [cancelTarget, setCancelTarget] = useState<MarketOrder | null>(null);

	function showPage(page: MarketPage) {
		queryClient.setQueryData(marketQuery(campaignId).queryKey, page);
	}

	function refreshColony() {
		return Promise.all([
			queryClient.invalidateQueries({ queryKey: colonyBoardQuery(campaignId).queryKey }),
			queryClient.invalidateQueries({ queryKey: recentTimelineQuery(campaignId).queryKey }),
			queryClient.invalidateQueries({ queryKey: timelineQueryKeyPrefix(campaignId) }),
		]);
	}

	const setQuantity = api.useMutation("put", "/api/campaigns/{campaignId}/market/cart/{itemId}", {
		onError: (error) => {
			toast.error(problemMessage(error, "The cart couldn't be changed."));
			queryClient.invalidateQueries({ queryKey: marketQuery(campaignId).queryKey });
		},
		onSuccess: showPage,
	});
	// A refused place or cancel means another device changed the market, so its numbers are refetched.
	function refreshMarket() {
		return queryClient.invalidateQueries({ queryKey: marketQuery(campaignId).queryKey });
	}

	const place = api.useMutation("post", "/api/campaigns/{campaignId}/market/orders", {
		onError: refreshMarket,
		onSettled: refreshColony,
		onSuccess: showPage,
	});
	const cancel = api.useMutation(
		"post",
		"/api/campaigns/{campaignId}/market/orders/{orderId}/cancel",
		{
			onError: (error) => {
				toast.error(problemMessage(error, "The order couldn't be cancelled."));
				refreshMarket();
			},
			onSettled: refreshColony,
			onSuccess: showPage,
		},
	);

	function handleSetQuantity(itemId: string, quantity: number, onSuccess?: () => void) {
		// The quantity this change was made from, so two members adding at once don't overwrite each other.
		const expectedQuantity =
			market.data?.cart.lines.find((line) => line.itemId === itemId)?.quantity ?? 0;
		setQuantity.mutate(
			{ body: { expectedQuantity, quantity }, params: { path: { campaignId, itemId } } },
			{ onSuccess },
		);
	}

	function handlePlace(expectedTotalGold: number) {
		place.mutate(
			{ body: { expectedTotalGold }, params: { path: { campaignId } } },
			{
				onSuccess: () => {
					placeSheet.onOpenChange(false);
					focusAfterClose(marketHeadingId);
				},
			},
		);
	}

	function handleCancel(order: MarketOrder) {
		cancel.mutate(
			{ params: { path: { campaignId, orderId: order.id } } },
			{
				onSettled: () => {
					focusAfterClose(marketHeadingId);
				},
			},
		);
	}

	let body: React.ReactNode;
	if (market.isPending || board.isPending) {
		body = (
			<div aria-busy="true" className="flex flex-col gap-6">
				<Skeleton className="h-36 rounded-xl" />
				<Skeleton className="h-48 rounded-xl" />
				<Skeleton className="h-64 rounded-xl" />
			</div>
		);
	} else if (market.isError || board.isError) {
		body = (
			<StatusPanel
				description={problemMessage(market.error ?? board.error, "Shipments couldn't be loaded.")}
				icon={StoreIcon}
				title="Shipments couldn't be loaded"
				tone="ember"
			/>
		);
	} else {
		const page = market.data;
		const colony = board.data;
		const cartQuantities = new Map(page.cart.lines.map((line) => [line.itemId, line.quantity]));
		body = (
			<div className="flex flex-col gap-8">
				<ShipList
					goldOnHand={page.goldOnHand}
					isDm={isDm}
					onCancel={setCancelTarget}
					onPay={paySheet.show}
					orders={page.orders}
					today={colony.day}
					voyageDays={page.voyageDays}
				/>
				<CartPanel
					cart={page.cart}
					isUpdating={setQuantity.isPending}
					items={page.items}
					onPlace={() => {
						placeSheet.show(true);
					}}
					onSetQuantity={handleSetQuantity}
					orderWaitNote={orderWaitText(page.orders, page.shipLimit, colony.day, isDm)}
				/>
				<CatalogList
					cartQuantities={cartQuantities}
					isDm={isDm}
					isUpdatingCart={setQuantity.isPending}
					items={page.items}
					onCreate={() => {
						itemSheet.show("new");
					}}
					onEditItem={itemSheet.show}
					onEditOffer={offerSheet.show}
					onSetQuantity={handleSetQuantity}
					resources={colony.resources}
					roles={colony.roles}
				/>
				<PastOrders history={page.history} />
				{placeSheet.value ? (
					<PlaceOrderSheet
						cart={page.cart}
						goldOnHand={page.goldOnHand}
						key={placeSheet.session}
						mutation={place}
						onConfirm={handlePlace}
						onOpenChange={placeSheet.onOpenChange}
						open={placeSheet.open}
						today={colony.day}
						voyageDays={page.voyageDays}
					/>
				) : null}
				{paySheet.value ? (
					<PaySheet
						changes={changes}
						goldOnHand={page.goldOnHand}
						key={paySheet.session}
						liveVersion={colony.version}
						onOpenChange={paySheet.onOpenChange}
						onPaid={() => {
							focusAfterClose(marketHeadingId);
						}}
						open={paySheet.open}
						order={paySheet.value}
					/>
				) : null}
			</div>
		);
	}

	return (
		<div className="flex flex-col gap-8">
			<PageHeader
				actions={
					isDm && market.data ? (
						<Button
							onClick={() => {
								shippingSheet.show({
									shipLimit: market.data.shipLimit,
									voyageDays: market.data.voyageDays,
								});
							}}
							variant="outline"
						>
							<ShipIcon />
							Shipping · {countOf(market.data.voyageDays, "day")},{" "}
							{countOf(market.data.shipLimit, "ship")}
						</Button>
					) : null
				}
				description="Order additional supplies from the Mainland"
				headingId={marketHeadingId}
				title="Shipments"
			/>
			{body}
			{isDm ? (
				<>
					<ItemSheet
						campaignId={campaignId}
						item={itemSheet.value === "new" ? null : itemSheet.value}
						key={`item-${itemSheet.session}`}
						onOpenChange={itemSheet.onOpenChange}
						open={itemSheet.open}
					/>
					{shippingSheet.value ? (
						<ShippingSheet
							campaignId={campaignId}
							key={`shipping-${shippingSheet.session}`}
							onOpenChange={shippingSheet.onOpenChange}
							open={shippingSheet.open}
							shipLimit={shippingSheet.value.shipLimit}
							voyageDays={shippingSheet.value.voyageDays}
						/>
					) : null}
					{offerSheet.value ? (
						<OfferSheet
							campaignId={campaignId}
							key={`offer-${offerSheet.session}`}
							onOpenChange={offerSheet.onOpenChange}
							open={offerSheet.open}
							target={offerSheet.value}
						/>
					) : null}
					<CancelOrderDialog
						onConfirm={() => {
							if (cancelTarget) {
								handleCancel(cancelTarget);
							}
						}}
						onOpenChange={(open) => {
							if (!open) {
								setCancelTarget(null);
							}
						}}
						open={cancelTarget !== null}
					/>
				</>
			) : null}
		</div>
	);
}

const pastOrdersListId = "market-past-orders";

function PastOrders({ history }: { history: readonly Schemas["MarketOrderSummary"][] }) {
	const [isOpen, setOpen] = useState(false);
	if (history.length === 0) {
		return null;
	}
	return (
		<section className="grid gap-2 rounded-lg border border-border">
			<button
				aria-controls={pastOrdersListId}
				aria-expanded={isOpen}
				className="flex min-h-11 cursor-pointer items-center justify-between gap-3 rounded-lg px-3 text-left font-medium outline-none focus-visible:ring-[3px] focus-visible:ring-ring/45"
				onClick={() => {
					setOpen((current) => !current);
				}}
				type="button"
			>
				Past orders ({formatNumber(history.length)})
				<ChevronDownIcon aria-hidden="true" className={isOpen ? "size-5 rotate-180" : "size-5"} />
			</button>
			{isOpen ? (
				<ul
					className="flex flex-col divide-y divide-border border-border border-t"
					id={pastOrdersListId}
				>
					{history.map((order) => (
						<li className="px-3 py-2" key={order.id}>
							<p className="font-semibold tabular-nums">
								Day {formatNumber(order.placedDay)} ·{" "}
								{order.status === "PAID"
									? `paid ${formatNumber(order.totalGold)} gold on Day ${formatNumber(order.paidDay ?? order.arrivesDay)}`
									: `cancelled on Day ${formatNumber(order.cancelledDay ?? order.placedDay)}`}
							</p>
							<p className="text-muted-foreground text-sm [overflow-wrap:anywhere]">
								{order.summary}
							</p>
						</li>
					))}
				</ul>
			) : null}
		</section>
	);
}
