import { cn } from "@/lib/utils";

/** The app's mark: an eight-point compass rose in a ring, drawn in the current text colour. */
export function Crest({ className }: { className?: string }) {
	return (
		<svg
			aria-hidden="true"
			className={cn("shrink-0", className)}
			fill="none"
			viewBox="0 0 32 32"
			xmlns="http://www.w3.org/2000/svg"
		>
			<circle cx="16" cy="16" opacity="0.55" r="14.5" stroke="currentColor" strokeWidth="1" />
			<circle cx="16" cy="16" opacity="0.35" r="11.5" stroke="currentColor" strokeWidth="0.75" />
			<path
				d="M16 3.5 18.2 13.8 28.5 16 18.2 18.2 16 28.5 13.8 18.2 3.5 16 13.8 13.8Z"
				fill="currentColor"
				opacity="0.95"
			/>
			<path
				d="M16 8.5 17.1 14.9 23.5 16 17.1 17.1 16 23.5 14.9 17.1 8.5 16 14.9 14.9Z"
				fill="currentColor"
				opacity="0.5"
				transform="rotate(45 16 16)"
			/>
			<circle cx="16" cy="16" fill="var(--bg)" r="1.6" />
		</svg>
	);
}
