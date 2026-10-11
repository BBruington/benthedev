import { Link, useLocation } from "@tanstack/react-router";
import { ChevronRightIcon, EllipsisIcon } from "lucide-react";
import { useState } from "react";
import { BottomSheet } from "@/components/bottom-sheet";
import { canUseDmTools, type Me } from "@/lib/access";
import { cn } from "@/lib/utils";
import { moreGroups, type NavigationItem, navigationItems, phoneTabs } from "./navigation";

const tabClasses =
	"group relative flex h-16 w-full flex-col items-center justify-center gap-1 text-[0.8rem] text-ink-muted outline-none transition-colors focus-visible:bg-accent data-[status=active]:text-gilt-ink";

/** The phone's tab bar (spec §6.1): Colony · Explore · Quests · Timeline · More. */
export function BottomTabs({
	me,
	pendingCount,
	className,
}: {
	me: Me;
	pendingCount: number;
	className?: string;
}) {
	const [isMoreOpen, setMoreOpen] = useState(false);
	const pathname = useLocation({ select: (location) => location.pathname });
	const visibleMoreGroups = moreGroups.filter((group) => canUseDmTools(me) || !group.dmOnly);
	const isMoreActive = visibleMoreGroups.some((group) =>
		group.items.some((itemId) => pathname.startsWith(navigationItems[itemId].to)),
	);

	return (
		<>
			<nav
				aria-label="Main"
				className={cn(
					"fixed inset-x-0 bottom-0 z-40 border-border border-t bg-bg/92 pb-safe backdrop-blur-md supports-[backdrop-filter]:bg-bg/80",
					className,
				)}
			>
				<ul className="mx-auto grid max-w-lg grid-cols-5">
					{phoneTabs.map((itemId) => (
						<li key={itemId}>
							<TabLink item={navigationItems[itemId]} />
						</li>
					))}
					<li>
						<button
							aria-expanded={isMoreOpen}
							aria-haspopup="dialog"
							className={tabClasses}
							data-status={isMoreActive ? "active" : undefined}
							onClick={() => {
								setMoreOpen(true);
							}}
							type="button"
						>
							<TabIndicator />
							<span className="relative">
								<EllipsisIcon aria-hidden="true" className="size-6" />
								{pendingCount > 0 ? (
									<span className="absolute -top-0.5 -right-1 size-2.5 rounded-full bg-gilt ring-2 ring-bg" />
								) : null}
							</span>
							More
							{pendingCount > 0 ? (
								<span className="sr-only">, {pendingCount} waiting to join</span>
							) : null}
						</button>
					</li>
				</ul>
			</nav>
			<BottomSheet onOpenChange={setMoreOpen} open={isMoreOpen} title="More">
				<div className="flex flex-col gap-5">
					{visibleMoreGroups.map((group) => (
						<section aria-labelledby={`more-${group.label}`} key={group.label}>
							<h3
								className="pb-1.5 font-medium text-muted-foreground text-xs uppercase tracking-[0.18em]"
								id={`more-${group.label}`}
							>
								{group.label}
							</h3>
							<ul className="overflow-hidden rounded-xl border border-border bg-card">
								{group.items.map((itemId) => (
									<li className="border-border border-b last:border-b-0" key={itemId}>
										<MoreLink
											badge={itemId === "players" ? pendingCount : 0}
											item={navigationItems[itemId]}
											onNavigate={() => {
												setMoreOpen(false);
											}}
										/>
									</li>
								))}
							</ul>
						</section>
					))}
				</div>
			</BottomSheet>
		</>
	);
}

function TabIndicator() {
	return (
		<span
			aria-hidden="true"
			className="absolute top-0 left-1/2 h-0.5 w-8 -translate-x-1/2 rounded-full bg-gilt opacity-0 transition-opacity group-data-[status=active]:opacity-100"
		/>
	);
}

function TabLink({ item }: { item: NavigationItem }) {
	const Icon = item.icon;
	return (
		<Link activeOptions={{ exact: item.to === "/" }} className={tabClasses} to={item.to}>
			<TabIndicator />
			<Icon aria-hidden="true" className="size-6" />
			{item.label}
		</Link>
	);
}

function MoreLink({
	item,
	badge,
	onNavigate,
}: {
	item: NavigationItem;
	badge: number;
	onNavigate: () => void;
}) {
	const Icon = item.icon;
	return (
		<Link
			className="flex min-h-14 items-center gap-3 px-4 outline-none transition-colors hover:bg-accent focus-visible:bg-accent data-[status=active]:text-gilt-ink"
			onClick={onNavigate}
			to={item.to}
		>
			<span
				aria-hidden="true"
				className="inline-flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted text-gilt-ink"
			>
				<Icon className="size-[1.125rem]" />
			</span>
			<span className="flex-1 truncate font-medium text-base">{item.label}</span>
			{badge > 0 ? (
				<span className="inline-flex min-w-6 items-center justify-center rounded-full bg-primary px-1.5 font-semibold text-primary-foreground text-xs leading-5">
					{badge}
					<span className="sr-only"> waiting</span>
				</span>
			) : null}
			{item.arrivesIn ? (
				<span className="text-muted-foreground text-xs tracking-wider">{item.arrivesIn}</span>
			) : null}
			<ChevronRightIcon aria-hidden="true" className="size-4 text-muted-foreground" />
		</Link>
	);
}
