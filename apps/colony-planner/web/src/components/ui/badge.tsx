import { cva, type VariantProps } from "class-variance-authority";
import { Slot } from "radix-ui";
import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

const badgeVariants = cva(
	"inline-flex w-fit shrink-0 items-center justify-center gap-1 overflow-hidden whitespace-nowrap rounded-full border px-2 py-0.5 font-medium text-xs leading-4 tracking-wide transition-[color,box-shadow] focus-visible:ring-[3px] focus-visible:ring-ring/45 [&>svg]:pointer-events-none [&>svg]:size-3",
	{
		defaultVariants: {
			variant: "default",
		},
		variants: {
			variant: {
				default: "border-transparent bg-primary text-primary-foreground",
				destructive: "border-transparent bg-destructive text-destructive-foreground",
				ember: "border-ember/35 bg-ember/10 text-ember-ink",
				gilt: "border-gilt/40 bg-gilt/10 text-gilt-ink",
				moss: "border-moss/40 bg-moss/10 text-moss-ink",
				muted: "border-border bg-muted text-muted-foreground",
				outline: "border-border text-foreground",
				secondary: "border-transparent bg-secondary text-secondary-foreground",
				verdigris: "border-verdigris/40 bg-verdigris/10 text-verdigris-ink",
			},
		},
	},
);

type BadgeProps = ComponentProps<"span"> &
	VariantProps<typeof badgeVariants> & { asChild?: boolean };

function Badge({ className, variant = "default", asChild = false, ...props }: BadgeProps) {
	const Component = asChild ? Slot.Root : "span";
	return (
		<Component
			className={cn(badgeVariants({ variant }), className)}
			data-slot="badge"
			data-variant={variant}
			{...props}
		/>
	);
}

export { Badge, badgeVariants };
