import type { ReactNode } from "react";

type SectionProps = {
	id: string;
	title: string;
	children: ReactNode;
};

export function Section({ id, title, children }: SectionProps) {
	return (
		<section
			aria-labelledby={`${id}-title`}
			className="grid scroll-mt-20 gap-6 border-border border-t pt-12 pb-20 md:grid-cols-[9rem_1fr] md:gap-12"
			id={id}
		>
			<h2
				className="font-mono text-muted-foreground text-xs uppercase tracking-[0.18em] md:sticky md:top-24 md:self-start"
				id={`${id}-title`}
			>
				{title}
			</h2>
			<div className="min-w-0">{children}</div>
		</section>
	);
}
