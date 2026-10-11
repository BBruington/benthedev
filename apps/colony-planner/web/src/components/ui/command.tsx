import { Command as CommandPrimitive } from "cmdk";
import { SearchIcon } from "lucide-react";
import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

function Command({ className, ...props }: ComponentProps<typeof CommandPrimitive>) {
	return (
		<CommandPrimitive
			className={cn(
				"flex h-full w-full flex-col overflow-hidden rounded-lg bg-popover text-popover-foreground",
				className,
			)}
			data-slot="command"
			{...props}
		/>
	);
}

function CommandInput({ className, ...props }: ComponentProps<typeof CommandPrimitive.Input>) {
	return (
		<div
			className="flex h-12 items-center gap-2 border-border border-b px-3"
			data-slot="command-input-wrapper"
		>
			<SearchIcon aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
			<CommandPrimitive.Input
				className={cn(
					"flex h-11 w-full bg-transparent py-3 text-base outline-hidden placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-50",
					className,
				)}
				data-slot="command-input"
				{...props}
			/>
		</div>
	);
}

function CommandList({ className, ...props }: ComponentProps<typeof CommandPrimitive.List>) {
	return (
		<CommandPrimitive.List
			className={cn("max-h-80 scroll-py-1 overflow-y-auto overflow-x-hidden", className)}
			data-slot="command-list"
			{...props}
		/>
	);
}

function CommandEmpty(props: ComponentProps<typeof CommandPrimitive.Empty>) {
	return (
		<CommandPrimitive.Empty
			className="py-8 text-center text-muted-foreground text-sm"
			data-slot="command-empty"
			{...props}
		/>
	);
}

function CommandGroup({ className, ...props }: ComponentProps<typeof CommandPrimitive.Group>) {
	return (
		<CommandPrimitive.Group
			className={cn(
				"overflow-hidden p-1.5 text-foreground [&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:pt-2 [&_[cmdk-group-heading]]:pb-1 [&_[cmdk-group-heading]]:font-medium [&_[cmdk-group-heading]]:text-muted-foreground [&_[cmdk-group-heading]]:text-xs [&_[cmdk-group-heading]]:uppercase [&_[cmdk-group-heading]]:tracking-[0.14em]",
				className,
			)}
			data-slot="command-group"
			{...props}
		/>
	);
}

function CommandSeparator({
	className,
	...props
}: ComponentProps<typeof CommandPrimitive.Separator>) {
	return (
		<CommandPrimitive.Separator
			className={cn("-mx-1 h-px bg-border", className)}
			data-slot="command-separator"
			{...props}
		/>
	);
}

function CommandItem({ className, ...props }: ComponentProps<typeof CommandPrimitive.Item>) {
	return (
		<CommandPrimitive.Item
			className={cn(
				"relative flex min-h-10 cursor-pointer select-none items-center gap-2 rounded-md px-2.5 py-2 text-base outline-hidden pointer-coarse:min-h-11 data-[disabled=true]:pointer-events-none data-[selected=true]:bg-accent data-[selected=true]:text-accent-foreground data-[disabled=true]:opacity-50 [&_svg:not([class*='size-'])]:size-4 [&_svg:not([class*='text-'])]:text-muted-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0",
				className,
			)}
			data-slot="command-item"
			{...props}
		/>
	);
}

export {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
	CommandSeparator,
};
