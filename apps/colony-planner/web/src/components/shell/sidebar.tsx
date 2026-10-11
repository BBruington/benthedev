import { Link } from "@tanstack/react-router";
import { GiltRule } from "@/components/gilt-rule";
import { canUseDmTools, type Me } from "@/lib/access";
import { cn } from "@/lib/utils";
import { type NavigationItem, navigationGroups, navigationItems } from "./navigation";

export function Sidebar({
	me,
	pendingCount,
	className,
}: {
	me: Me;
	pendingCount: number;
	className?: string;
}) {
	const showDmGroup = canUseDmTools(me);
	return (
		<nav
			aria-label="Main"
			className={cn(
				"sticky top-(--shell-header-height,4rem) h-[calc(100dvh-var(--shell-header-height,4rem))] w-60 shrink-0 overflow-y-auto border-border border-r px-3 py-5 [scrollbar-width:thin] lg:w-64",
				className,
			)}
		>
			<div className="flex flex-col gap-5">
				{navigationGroups
					.filter((group) => showDmGroup || !group.dmOnly)
					.map((group) => (
						<section aria-labelledby={`navigation-${group.label}`} key={group.label}>
							<h2
								className="px-3 pb-2 font-medium text-muted-foreground text-xs uppercase tracking-[0.18em]"
								id={`navigation-${group.label}`}
							>
								{group.label}
							</h2>
							<ul className="flex flex-col gap-0.5">
								{group.items.map((itemId) => (
									<li key={itemId}>
										<SidebarLink
											badge={itemId === "players" ? pendingCount : 0}
											item={navigationItems[itemId]}
										/>
									</li>
								))}
							</ul>
						</section>
					))}
			</div>
			<GiltRule className="mx-3 mt-8 opacity-70" />
			<p className="mt-3 px-3 text-center font-flavor text-muted-foreground text-sm italic">
				Ships still sail from Eldari.
			</p>
		</nav>
	);
}

function SidebarLink({ item, badge }: { item: NavigationItem; badge: number }) {
	const Icon = item.icon;
	return (
		<Link
			activeOptions={{ exact: item.to === "/" }}
			className="group relative flex min-h-9 items-center gap-3 rounded-md px-3 text-[0.97rem] text-ink-muted outline-none transition-colors hover:bg-accent hover:text-foreground pointer-coarse:min-h-11 focus-visible:ring-[3px] focus-visible:ring-ring/45 data-[status=active]:bg-accent data-[status=active]:font-medium data-[status=active]:text-gilt-ink"
			to={item.to}
		>
			<span
				aria-hidden="true"
				className="absolute inset-y-2 left-0 w-0.5 rounded-full bg-gilt opacity-0 transition-opacity group-data-[status=active]:opacity-100"
			/>
			<Icon aria-hidden="true" className="size-[1.125rem] shrink-0" />
			<span className="flex-1 truncate">{item.label}</span>
			{badge > 0 ? (
				<span className="inline-flex min-w-6 items-center justify-center rounded-full bg-primary px-1.5 font-semibold text-primary-foreground text-xs leading-5">
					{badge}
					<span className="sr-only"> waiting</span>
				</span>
			) : null}
			{item.arrivesIn ? (
				<span className="font-medium text-[0.7rem] text-muted-foreground/80 tracking-wider">
					{item.arrivesIn}
				</span>
			) : null}
		</Link>
	);
}
