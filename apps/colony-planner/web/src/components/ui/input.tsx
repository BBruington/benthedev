import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

function Input({ className, type, ...props }: ComponentProps<"input">) {
	return (
		<input
			className={cn(
				"h-11 w-full min-w-0 rounded-md border border-input bg-surface-raised/70 px-3 py-1 text-base text-foreground shadow-[0_1px_0_rgb(0_0_0/0.04)_inset] outline-none transition-[color,border-color,box-shadow] selection:bg-primary selection:text-primary-foreground placeholder:text-muted-foreground/70 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 dark:bg-surface-sunken/60",
				"focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/35",
				"aria-invalid:border-ember-ink aria-invalid:ring-[3px] aria-invalid:ring-destructive/20",
				className,
			)}
			data-slot="input"
			type={type}
			{...props}
		/>
	);
}

export { Input };
