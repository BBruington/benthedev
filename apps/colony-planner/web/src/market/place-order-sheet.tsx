import { ShipIcon } from "lucide-react";
import type { Schemas } from "@/api/client";
import { problemMessage } from "@/api/problem";
import { BottomSheet } from "@/components/bottom-sheet";
import { Button } from "@/components/ui/button";
import { formatNumber, inDays } from "@/lib/colony";
import { lotSize, unitsText } from "@/lib/market";
import { useReviewedVersion } from "@/people/workforce-sheets";

type MarketCart = Schemas["MarketCart"];

/**
 * Confirm the cart as one order. Gold is only checked when the ship docks, so too little gold now is a
 * warning, never a block. The order is placed at the total the sheet opened with, so a cart another
 * member changes meanwhile is refused instead of bought unseen.
 */
export function PlaceOrderSheet({
	open,
	onOpenChange,
	cart,
	today,
	voyageDays,
	goldOnHand,
	mutation,
	onConfirm,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	cart: MarketCart;
	today: number;
	voyageDays: number;
	goldOnHand: number;
	mutation: { error: unknown; isPending: boolean; reset: () => void };
	onConfirm: (expectedTotalGold: number) => void;
}) {
	const reviewedTotal = useReviewedVersion(cart.totalGold, mutation);
	const isPlacing = mutation.isPending;
	const error = mutation.error;
	const docksDay = today + voyageDays;
	return (
		<BottomSheet
			className="md:max-w-xl"
			footer={
				<>
					<Button
						className="h-12 text-base sm:flex-1"
						onClick={() => {
							onOpenChange(false);
						}}
						size="lg"
						variant="outline"
					>
						Keep shopping
					</Button>
					<Button
						className="h-12 text-base sm:flex-1"
						disabled={isPlacing || cart.lines.length === 0}
						onClick={() => {
							onConfirm(reviewedTotal);
						}}
						size="lg"
					>
						<ShipIcon />
						{isPlacing ? "Placing…" : "Place order"}
					</Button>
				</>
			}
			onOpenChange={onOpenChange}
			open={open}
			title="Place this order?"
		>
			<div className="grid gap-4">
				<ul className="flex flex-col divide-y divide-border rounded-lg bg-surface-sunken/60 px-3">
					{cart.lines.map((line) => (
						<li
							className="flex items-baseline justify-between gap-3 py-2 tabular-nums"
							key={line.itemId}
						>
							<span className="min-w-0 [overflow-wrap:anywhere]">
								{unitsText(line.quantity * lotSize(line), line)}
							</span>
							<span className="shrink-0 font-semibold">{formatNumber(line.lineTotal)} gp</span>
						</li>
					))}
				</ul>
				<p className="font-semibold tabular-nums">Total {formatNumber(cart.totalGold)} gold</p>
				<p className="inline-flex items-center gap-2 text-verdigris-ink">
					<ShipIcon aria-hidden="true" className="size-5 shrink-0" />
					Docks Day {formatNumber(docksDay)}, {inDays(voyageDays)}
				</p>
				{goldOnHand < cart.totalGold ? (
					<p className="rounded-lg border border-ember/40 bg-ember/10 px-3 py-2 text-ember-ink text-sm">
						The colony has {formatNumber(goldOnHand)} gold. You'll need{" "}
						{formatNumber(cart.totalGold)} when it docks.
					</p>
				) : null}
				{reviewedTotal === cart.totalGold ? null : (
					<p
						className="rounded-lg border border-ember/40 bg-ember/10 px-3 py-2 text-ember-ink text-sm"
						role="status"
					>
						The cart changed while this was open: it was {formatNumber(reviewedTotal)} gold.
					</p>
				)}
				{error ? (
					<p
						className="rounded-lg border border-ember/40 bg-ember/10 px-3 py-2 text-ember-ink text-sm"
						role="alert"
					>
						{problemMessage(error, "The order couldn't be placed.")}
					</p>
				) : null}
			</div>
		</BottomSheet>
	);
}
