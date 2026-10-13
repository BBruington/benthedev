import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { GiltRule } from "@/components/gilt-rule";
import { cn } from "@/lib/utils";

/** A centred icon, heading and message: empty, forbidden and error states. */
export function StatusPanel({
	icon: Icon,
	tone = "gilt",
	eyebrow,
	title,
	description,
	flavor,
	actions,
	className,
}: {
	icon: LucideIcon;
	tone?: "gilt" | "verdigris" | "ember";
	eyebrow?: ReactNode;
	title: ReactNode;
	description?: ReactNode;
	/** One line of narration in italics, under a gilt rule. */
	flavor?: ReactNode;
	actions?: ReactNode;
	className?: string;
}) {
	const toneClasses = {
		ember: "border-ember/40 text-ember-ink",
		gilt: "border-gilt/40 text-gilt-ink",
		verdigris: "border-verdigris/40 text-verdigris-ink",
	}[tone];
	const glowClasses = { ember: "bg-ember/15", gilt: "bg-gilt/15", verdigris: "bg-verdigris/15" }[
		tone
	];
	return (
		<section
			className={cn(
				"mx-auto flex max-w-xl flex-col items-center px-2 py-10 text-center md:py-16",
				className,
			)}
		>
			<div aria-hidden="true" className="relative mb-6">
				<span className={cn("absolute -inset-3 rounded-full blur-xl", glowClasses)} />
				<span
					className={cn(
						"relative inline-flex size-20 items-center justify-center rounded-full border bg-card shadow-card",
						toneClasses,
					)}
				>
					<Icon className="size-9" strokeWidth={1.5} />
				</span>
			</div>
			{eyebrow ? <div className="mb-3">{eyebrow}</div> : null}
			<h1 className="text-balance font-display font-semibold text-3xl tracking-wide md:text-4xl">
				{title}
			</h1>
			{description ? (
				<div className="mt-3 text-balance text-lg text-muted-foreground">{description}</div>
			) : null}
			{actions ? (
				<div className="mt-8 flex flex-wrap items-center justify-center gap-3">{actions}</div>
			) : null}
			{flavor ? (
				<>
					<GiltRule className="mt-10 w-48 opacity-80" />
					<p className="mt-4 max-w-md font-flavor text-base text-muted-foreground italic">
						{flavor}
					</p>
				</>
			) : null}
		</section>
	);
}
