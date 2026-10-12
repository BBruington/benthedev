import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

export function PageHeader({
	eyebrow,
	title,
	description,
	actions,
	className,
	headingId,
}: {
	eyebrow?: ReactNode;
	title: ReactNode;
	description?: ReactNode;
	actions?: ReactNode;
	className?: string;
	/** Makes the heading a focus target for an action that lands on this page. */
	headingId?: string;
}) {
	return (
		<header
			className={cn(
				"flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between sm:gap-6",
				className,
			)}
		>
			<div className="min-w-0 space-y-1.5">
				{eyebrow ? (
					<p className="font-medium text-gilt-ink text-xs uppercase tracking-[0.2em]">{eyebrow}</p>
				) : null}
				<h1
					className="font-display font-semibold text-3xl text-foreground tracking-wide outline-none md:text-4xl"
					id={headingId}
					tabIndex={headingId ? -1 : undefined}
				>
					{title}
				</h1>
				{description ? (
					<p className="max-w-prose text-base text-muted-foreground">{description}</p>
				) : null}
			</div>
			{actions ? <div className="flex shrink-0 flex-wrap gap-2">{actions}</div> : null}
		</header>
	);
}
