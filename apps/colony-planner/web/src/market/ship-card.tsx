import { CoinsIcon, ShipIcon } from "lucide-react";
import { useState } from "react";
import type { Schemas } from "@/api/client";
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { formatNumber } from "@/lib/colony";
import { isDocked, shipLine } from "@/lib/market";
import { countOf } from "@/lib/words";

type MarketOrder = Schemas["MarketOrder"];

const shipHeadingId = "market-ship-heading";

/** The one open order: at sea with its cargo and what it will cost, or docked and waiting for gold. */
export function ShipCard({
	order,
	today,
	goldOnHand,
	voyageDays,
	isDm,
	onPay,
	onCancel,
}: {
	order: MarketOrder | null;
	today: number;
	goldOnHand: number;
	voyageDays: number;
	isDm: boolean;
	onPay: (order: MarketOrder) => void;
	onCancel: (order: MarketOrder) => void;
}) {
	const docked = order !== null && isDocked(order, today);
	return (
		<section aria-labelledby={shipHeadingId} className="flex flex-col gap-3">
			<h2 className="font-display font-semibold text-2xl tracking-wide" id={shipHeadingId}>
				The ship
			</h2>
			{order ? (
				<div className="flex flex-col gap-3 rounded-xl border border-verdigris/40 bg-card p-4 shadow-card sm:p-5">
					<p className="flex items-center gap-2 font-semibold">
						<ShipIcon aria-hidden="true" className="size-5 shrink-0 text-verdigris-ink" />
						{shipLine(order, today)}
					</p>
					<p className="[overflow-wrap:anywhere]">{order.summary}</p>
					<p className="text-muted-foreground text-sm tabular-nums">
						{formatNumber(order.totalGold)} gold due {docked ? "now" : "on arrival"} · the colony
						has {formatNumber(goldOnHand)} · ordered by {order.placedByName} on Day{" "}
						{formatNumber(order.placedDay)}
					</p>
					<div className="flex flex-wrap justify-end gap-2">
						{isDm ? (
							<Button
								onClick={() => {
									onCancel(order);
								}}
								variant="ghost"
							>
								Cancel order
							</Button>
						) : null}
						{docked ? (
							<Button
								onClick={() => {
									onPay(order);
								}}
							>
								<CoinsIcon />
								Pay {formatNumber(order.totalGold)} gold
							</Button>
						) : null}
					</div>
				</div>
			) : (
				<p className="rounded-xl border border-border border-dashed px-4 py-4 text-muted-foreground">
					No ship ordered. Orders take {countOf(voyageDays, "day")}.
				</p>
			)}
		</section>
	);
}

/** Only the DM cancels a stuck order. Nothing is charged and the cart stays as it is. */
export function CancelOrderDialog({
	open,
	onOpenChange,
	onConfirm,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	onConfirm: () => void;
}) {
	const [confirmed, setConfirmed] = useState(false);
	return (
		<AlertDialog onOpenChange={onOpenChange} open={open}>
			<AlertDialogContent
				onCloseAutoFocus={(event) => {
					if (confirmed) {
						event.preventDefault();
						setConfirmed(false);
					}
				}}
			>
				<AlertDialogHeader>
					<AlertDialogTitle>Cancel this order?</AlertDialogTitle>
					<AlertDialogDescription>
						The ship never arrives and nothing is charged. The party can place a new order right
						away.
					</AlertDialogDescription>
				</AlertDialogHeader>
				<AlertDialogFooter>
					<AlertDialogCancel>Keep it</AlertDialogCancel>
					<AlertDialogAction
						onClick={() => {
							setConfirmed(true);
							onConfirm();
						}}
						variant="destructive"
					>
						Cancel order
					</AlertDialogAction>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	);
}
