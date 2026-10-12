import type { ComponentProps } from "react";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { cn } from "@/lib/utils";

const controlClasses = [
	"w-full min-w-0 rounded-md border border-input bg-surface-raised/70 px-3 text-base text-foreground outline-none transition-[color,border-color,box-shadow] disabled:cursor-not-allowed disabled:opacity-50 dark:bg-surface-sunken/60",
	"focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/35",
	"aria-invalid:border-ember-ink aria-invalid:ring-[3px] aria-invalid:ring-destructive/20",
];

/** A native select styled like Input: on a phone it opens the system picker, which thumbs prefer. */
export function NativeSelect({ className, ...props }: ComponentProps<"select">) {
	return <select className={cn("h-11", controlClasses, className)} {...props} />;
}

export function TextArea({ className, ...props }: ComponentProps<"textarea">) {
	return (
		<textarea
			className={cn(
				"min-h-24 py-2 placeholder:text-muted-foreground/70",
				controlClasses,
				className,
			)}
			{...props}
		/>
	);
}

/**
 * The "Visible to players" switch every create form carries (spec §5.9), or any other on/off choice
 * with a line saying what the current setting means.
 */
export function SwitchRow({
	id,
	label,
	description,
	checked,
	disabled = false,
	onCheckedChange,
}: {
	id: string;
	label: string;
	description: string;
	checked: boolean;
	disabled?: boolean;
	onCheckedChange: (checked: boolean) => void;
}) {
	return (
		<div className="flex items-center justify-between gap-4 rounded-lg border border-border px-3 py-3">
			<Label className="grid gap-0.5" htmlFor={id}>
				<span>{label}</span>
				<span className="font-normal text-muted-foreground text-sm">{description}</span>
			</Label>
			<Switch checked={checked} disabled={disabled} id={id} onCheckedChange={onCheckedChange} />
		</div>
	);
}
