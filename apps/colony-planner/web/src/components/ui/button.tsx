import { cva, type VariantProps } from "class-variance-authority";
import { Slot } from "radix-ui";
import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

const buttonVariants = cva(
	"inline-flex shrink-0 cursor-pointer select-none items-center justify-center gap-2 whitespace-nowrap rounded-md font-medium outline-none transition-[color,background-color,border-color,box-shadow,transform] duration-150 focus-visible:ring-[3px] focus-visible:ring-ring/45 active:translate-y-px disabled:pointer-events-none disabled:opacity-50 aria-invalid:ring-destructive/30 [&_svg:not([class*='size-'])]:size-[1.125rem] [&_svg]:pointer-events-none [&_svg]:shrink-0",
	{
		defaultVariants: {
			size: "default",
			variant: "default",
		},
		variants: {
			size: {
				default: "h-11 px-5 text-base has-[>svg]:px-4",
				icon: "size-11",
				"icon-sm": "size-9 pointer-coarse:size-11",
				lg: "h-12 px-6 text-base has-[>svg]:px-5",
				sm: "h-9 gap-1.5 px-3 text-sm pointer-coarse:h-11 has-[>svg]:px-2.5",
			},
			variant: {
				default:
					"bg-primary text-primary-foreground shadow-[0_1px_0_rgb(255_255_255/0.18)_inset,0_1px_2px_rgb(0_0_0/0.2)] hover:bg-primary/90",
				destructive:
					"bg-destructive text-destructive-foreground shadow-sm hover:bg-destructive/90 focus-visible:ring-destructive/40",
				ghost: "text-foreground hover:bg-accent hover:text-accent-foreground",
				"ghost-destructive":
					"text-ember-ink hover:bg-destructive/12 focus-visible:ring-destructive/40",
				link: "h-auto px-0 text-gilt-ink underline-offset-4 hover:underline has-[>svg]:px-0",
				outline:
					"border border-border bg-surface/60 text-foreground hover:border-gilt/40 hover:bg-accent",
				secondary: "bg-secondary text-secondary-foreground shadow-sm hover:bg-secondary/90",
			},
		},
	},
);

type ButtonProps = ComponentProps<"button"> &
	VariantProps<typeof buttonVariants> & {
		asChild?: boolean;
	};

function Button({
	className,
	variant = "default",
	size = "default",
	asChild = false,
	...props
}: ButtonProps) {
	const Component = asChild ? Slot.Root : "button";
	return (
		<Component
			className={cn(buttonVariants({ className, size, variant }))}
			data-size={size}
			data-slot="button"
			data-variant={variant}
			{...props}
		/>
	);
}

export { Button, type ButtonProps, buttonVariants };
