import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowRightIcon, ShipIcon } from "lucide-react";
import { marketQuery } from "@/api/queries";
import { Button } from "@/components/ui/button";
import { useCampaign } from "@/hooks/use-campaign";
import { formatNumber, inDays } from "@/lib/colony";
import { isDocked } from "@/lib/market";

/** The board's goods ship, only while an order is out: when it docks, or what it waits for. */
export function BoardMarket({ day }: { day: number }) {
	const { campaignId, slug } = useCampaign();
	const order = useQuery(marketQuery(campaignId)).data?.order;
	if (!order) {
		return null;
	}
	return (
		<section
			aria-labelledby="board-market-heading"
			className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-verdigris/40 bg-card p-4 shadow-card sm:p-5"
		>
			<h2
				className="flex min-w-0 items-center gap-2 font-display font-semibold text-xl tracking-wide"
				id="board-market-heading"
			>
				<ShipIcon aria-hidden="true" className="size-5 shrink-0 text-verdigris-ink" />
				{isDocked(order, day) ? (
					<span>
						Goods ship docked
						<span className="font-sans font-normal text-base text-muted-foreground">
							{" "}
							· {formatNumber(order.totalGold)} gold due
						</span>
					</span>
				) : (
					<span>
						Goods ship
						<span className="font-sans font-normal text-base text-muted-foreground">
							{" "}
							· docks Day {formatNumber(order.arrivesDay)} ({inDays(order.arrivesDay - day)})
						</span>
					</span>
				)}
			</h2>
			<Button asChild size="sm" variant="ghost">
				<Link params={{ campaignSlug: slug }} to="/c/$campaignSlug/market">
					Market
					<ArrowRightIcon aria-hidden="true" />
				</Link>
			</Button>
		</section>
	);
}
