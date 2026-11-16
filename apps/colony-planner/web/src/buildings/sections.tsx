import { ChevronDownIcon } from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import type { Schemas } from "@/api/client";
import { formatNumber } from "@/lib/colony";
import { otherWorkCount, roleCount } from "@/lib/construction";

type ConstructionRole = Schemas["ConstructionRole"];

/** One of the Buildings page's sections: a heading focus can land on, and its content. */
export function BuildingsSection({
	headingId,
	title,
	description,
	actions,
	children,
}: {
	headingId: string;
	title: string;
	description?: ReactNode;
	actions?: ReactNode;
	children: ReactNode;
}) {
	return (
		<section aria-labelledby={headingId} className="flex flex-col gap-3">
			<div className="flex flex-wrap items-end justify-between gap-3">
				<div className="min-w-0 space-y-1">
					<h2
						className="font-display font-semibold text-2xl tracking-wide outline-none"
						id={headingId}
						tabIndex={-1}
					>
						{title}
					</h2>
					{description ? <p className="text-muted-foreground text-sm">{description}</p> : null}
				</div>
				{actions}
			</div>
			{children}
		</section>
	);
}

/** A group that starts closed, such as past buildings or archived blueprints. */
export function CollapsibleGroup({ label, children }: { label: string; children: ReactNode }) {
	const [isOpen, setOpen] = useState(false);
	const contentId = useId();
	return (
		<section className="grid gap-2 rounded-lg border border-border">
			<button
				aria-controls={contentId}
				aria-expanded={isOpen}
				className="flex min-h-11 cursor-pointer items-center justify-between gap-3 rounded-lg px-3 text-left font-medium outline-none focus-visible:ring-[3px] focus-visible:ring-ring/45"
				onClick={() => {
					setOpen((current) => !current);
				}}
				type="button"
			>
				{label}
				<ChevronDownIcon aria-hidden="true" className={isOpen ? "size-5 rotate-180" : "size-5"} />
			</button>
			{isOpen ? (
				<div className="border-border border-t p-3" id={contentId}>
					{children}
				</div>
			) : null}
		</section>
	);
}

/**
 * Each labor role's colonists: how many there are, how many are on the projects listed here and how
 * many are idle. Colonists on a project the viewer can't see show only as other work.
 */
export function RoleAvailability({ roles }: { roles: readonly ConstructionRole[] }) {
	if (roles.length === 0) {
		return (
			<p className="text-muted-foreground">
				No role does construction work yet. A blueprint's labor names the roles it needs.
			</p>
		);
	}
	return (
		<ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
			{roles.map((role) => {
				const otherWork = otherWorkCount(role);
				return (
					<li
						className="flex flex-col gap-1 rounded-xl border border-border bg-card p-4 shadow-card"
						key={role.roleKey}
					>
						<p className="flex items-baseline justify-between gap-3">
							<span className="font-semibold text-lg [overflow-wrap:anywhere]">{role.label}</span>
							<span className="text-muted-foreground text-sm tabular-nums">
								{roleCount(role.headcount, role.label)}
							</span>
						</p>
						<p className="tabular-nums">
							<span className="font-semibold">{formatNumber(role.crewed)}</span> on projects ·{" "}
							<span className="font-semibold">{formatNumber(role.idle)}</span> idle
						</p>
						{otherWork > 0 ? (
							<p className="text-muted-foreground text-sm tabular-nums">
								{formatNumber(otherWork)} on other work
							</p>
						) : null}
					</li>
				);
			})}
		</ul>
	);
}
