import type { ReactNode } from "react";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

/** Ids for a field's hint and error, so its input can point at them with aria-describedby. */
export function fieldMessageIds(id: string) {
	return { description: `${id}-description`, error: `${id}-error` };
}

/** The input's aria props for a field: invalid when the server said so, described by the message. */
export function fieldInputProps(id: string, error: string | undefined, hasDescription: boolean) {
	const ids = fieldMessageIds(id);
	let describedBy: string | undefined;
	if (error) {
		describedBy = ids.error;
	} else if (hasDescription) {
		describedBy = ids.description;
	}
	return { "aria-describedby": describedBy, "aria-invalid": error ? true : undefined, id };
}

export function Field({
	id,
	label,
	description,
	error,
	optional = false,
	className,
	children,
}: {
	id: string;
	label: ReactNode;
	description?: ReactNode;
	/** The server's message for this field (a 422 maps onto it); it replaces the description. */
	error?: string;
	optional?: boolean;
	className?: string;
	children: ReactNode;
}) {
	const ids = fieldMessageIds(id);
	let message: ReactNode = null;
	if (error) {
		message = (
			<p className="text-ember-ink text-sm" id={ids.error}>
				{error}
			</p>
		);
	} else if (description) {
		message = (
			<p className="text-muted-foreground text-sm" id={ids.description}>
				{description}
			</p>
		);
	}
	return (
		<div className={cn("grid content-start gap-2", className)}>
			<Label htmlFor={id}>
				{label}
				{optional ? (
					<>
						{" "}
						<span className="font-normal text-muted-foreground text-xs leading-none">Optional</span>
					</>
				) : null}
			</Label>
			{children}
			{message}
		</div>
	);
}
