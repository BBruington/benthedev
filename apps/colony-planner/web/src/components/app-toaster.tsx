import { Toaster } from "sonner";
import { useResolvedTheme } from "@/lib/theme";

/** Sonner, dressed in the palette. Toasts sit at the top, clear of the phone tab bar. */
export function AppToaster() {
	const theme = useResolvedTheme();
	return (
		<Toaster
			gap={10}
			offset={{ top: "calc(env(safe-area-inset-top) + 0.75rem)" }}
			position="top-center"
			theme={theme}
			toastOptions={{
				classNames: {
					actionButton:
						"ml-auto inline-flex h-9 shrink-0 cursor-pointer items-center rounded-md bg-primary px-3 font-medium text-primary-foreground text-sm hover:bg-primary/90",
					content: "grid min-w-0 flex-1 gap-0.5",
					description: "text-muted-foreground text-sm",
					error: "[&_[data-icon]]:text-ember-ink",
					icon: "mt-0.5 flex size-5 shrink-0 items-center justify-center [&_svg]:size-5",
					info: "[&_[data-icon]]:text-verdigris-ink",
					success: "[&_[data-icon]]:text-moss-ink",
					title: "font-medium text-base leading-snug",
					toast:
						"flex w-full items-start gap-3 rounded-xl border border-border bg-popover p-4 font-sans text-popover-foreground shadow-float",
					warning: "[&_[data-icon]]:text-gilt-ink",
				},
				unstyled: true,
			}}
		/>
	);
}
