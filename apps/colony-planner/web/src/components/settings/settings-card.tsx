import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

/** One card on the DM's Settings page: an icon, a heading, what it's for, and its fields. */
export function SettingsCard({
	id,
	icon: Icon,
	title,
	description,
	tone = "gilt",
	children,
}: {
	id?: string;
	icon: LucideIcon;
	title: string;
	description: ReactNode;
	tone?: "gilt" | "ember";
	children: ReactNode;
}) {
	const headingId = `${id ?? title.toLowerCase().replaceAll(" ", "-")}-heading`;
	return (
		<section
			aria-labelledby={headingId}
			className={cn(
				"scroll-mt-28 rounded-2xl border bg-card shadow-card",
				tone === "ember" ? "border-ember/45" : "border-border",
			)}
			id={id}
		>
			<div className="flex items-start gap-4 border-border border-b px-5 py-5 md:px-7">
				<span
					aria-hidden="true"
					className={cn(
						"inline-flex size-10 shrink-0 items-center justify-center rounded-lg",
						tone === "ember" ? "bg-ember/12 text-ember-ink" : "bg-gilt/12 text-gilt-ink",
					)}
				>
					<Icon className="size-5" />
				</span>
				<div className="min-w-0">
					<h2
						className="font-display font-semibold text-xl tracking-wide outline-none"
						id={headingId}
						tabIndex={-1}
					>
						{title}
					</h2>
					<p className="mt-1 max-w-prose text-muted-foreground text-sm">{description}</p>
				</div>
			</div>
			<div className="grid gap-6 px-5 py-6 md:px-7">{children}</div>
		</section>
	);
}
