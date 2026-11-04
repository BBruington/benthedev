import { useQuery } from "@tanstack/react-query";
import { CoinsIcon } from "lucide-react";
import type { Schemas } from "@/api/client";
import { payMarketOrderPreviewQuery } from "@/api/queries";
import { ResourceChangeList } from "@/colony/resource-change-list";
import type { ColonyChanges } from "@/colony/use-colony-changes";
import { BottomSheet } from "@/components/bottom-sheet";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { formatNumber } from "@/lib/colony";
import { ChangeError, useReviewedVersion } from "@/people/workforce-sheets";

type MarketOrder = Schemas["MarketOrder"];

/**
 * Pay for a docked ship: full-screen on a phone, like the approval sheet. The server's preview shows
 * before → after for gold, every grant and every role; a shortfall disables Pay, since gold is strict.
 */
export function PaySheet({
	open,
	onOpenChange,
	order,
	goldOnHand,
	liveVersion,
	changes,
	onPaid,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	order: MarketOrder;
	goldOnHand: number;
	liveVersion: number;
	changes: ColonyChanges;
	onPaid: () => void;
}) {
	const campaignId = changes.inCampaign.params.path.campaignId;
	const pay = changes.payMarketOrder;
	const reviewedVersion = useReviewedVersion(liveVersion, pay);
	const preview = useQuery({
		...payMarketOrderPreviewQuery(campaignId, order.id),
		enabled: open,
	});
	const isAffordable = preview.data?.affordable === true;

	function handlePay() {
		pay.mutate(
			{
				body: { expectedVersion: reviewedVersion },
				params: { path: { campaignId, orderId: order.id } },
			},
			{
				onSuccess: () => {
					onOpenChange(false);
					onPaid();
				},
			},
		);
	}

	return (
		<BottomSheet
			className="h-dvh max-h-dvh rounded-t-none border-t-0 md:h-auto md:max-w-xl md:border-t"
			description={`${order.summary} · ${formatNumber(order.totalGold)} gold`}
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
						Not yet
					</Button>
					<Button
						className="h-12 text-base sm:flex-1"
						disabled={!isAffordable || pay.isPending}
						onClick={handlePay}
						size="lg"
					>
						<CoinsIcon />
						{pay.isPending ? "Paying…" : `Pay ${formatNumber(order.totalGold)} gold`}
					</Button>
				</>
			}
			onOpenChange={(next) => {
				if (!next) {
					pay.reset();
				}
				onOpenChange(next);
			}}
			open={open}
			title="Pay for the goods"
		>
			<div className="grid gap-5">
				<PayPreview
					error={preview.error}
					goldOnHand={goldOnHand}
					isPending={preview.isPending}
					preview={preview.data}
					totalGold={order.totalGold}
				/>
				<ChangeError error={pay.error} fallback="The goods couldn't be paid for." />
			</div>
		</BottomSheet>
	);
}

function PayPreview({
	preview,
	isPending,
	error,
	goldOnHand,
	totalGold,
}: {
	preview: Schemas["MarketPayPreview"] | undefined;
	isPending: boolean;
	error: unknown;
	goldOnHand: number;
	totalGold: number;
}) {
	if (error) {
		return <ChangeError error={error} fallback="The preview couldn't be worked out." />;
	}
	if (isPending || !preview) {
		return (
			<div aria-busy="true" className="grid gap-2">
				<Skeleton className="h-10" />
				<Skeleton className="h-5 w-2/3" />
			</div>
		);
	}
	if (!preview.affordable) {
		return (
			<p
				className="rounded-lg border border-ember/40 bg-ember/10 px-3 py-2 text-ember-ink text-sm"
				role="alert"
			>
				{preview.shortfall ||
					`Not enough gold: the colony has ${formatNumber(goldOnHand)}, needs ${formatNumber(totalGold)}.`}
			</p>
		);
	}
	return (
		<section aria-label="Preview" aria-live="polite" className="grid gap-2">
			<ResourceChangeList changes={preview.resources} />
			{preview.roles.length > 0 ? (
				<ul className="grid gap-1 text-sm tabular-nums">
					{preview.roles.map((role) => (
						<li key={role.key}>
							{role.label} {formatNumber(role.before)} → {formatNumber(role.after)}
						</li>
					))}
				</ul>
			) : null}
			{preview.summary ? (
				<p className="font-flavor text-muted-foreground italic">{preview.summary}</p>
			) : null}
		</section>
	);
}
