import { Switch as SwitchPrimitive } from "radix-ui";
import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

function Switch({ className, ...props }: ComponentProps<typeof SwitchPrimitive.Root>) {
	return (
		<SwitchPrimitive.Root
			className={cn(
				"peer group/switch relative inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full border border-input outline-none transition-colors focus-visible:ring-[3px] focus-visible:ring-ring/45 disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:border-primary data-[state=checked]:bg-primary data-[state=unchecked]:bg-muted",
				"before:absolute before:-inset-2.5 before:content-['']",
				className,
			)}
			data-slot="switch"
			{...props}
		>
			<SwitchPrimitive.Thumb
				className="pointer-events-none block size-[1.125rem] rounded-full bg-ink-muted shadow-sm ring-0 transition-transform data-[state=checked]:translate-x-[1.3rem] data-[state=unchecked]:translate-x-0.5 data-[state=checked]:bg-primary-foreground"
				data-slot="switch-thumb"
			/>
		</SwitchPrimitive.Root>
	);
}

export { Switch };
