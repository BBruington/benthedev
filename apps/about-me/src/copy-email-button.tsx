import { useEffect, useState } from "react";
import { CheckIcon, CopyIcon } from "./icons";
import { EMAIL } from "./profile";

type CopyState = "idle" | "copied" | "failed";

const COPY_FEEDBACK: Record<CopyState, { label: string; announcement: string }> = {
	copied: { announcement: "Email address copied", label: "Copied" },
	failed: { announcement: "Couldn't copy the email address", label: "Copy failed" },
	idle: { announcement: "", label: "Copy email" },
};

export function CopyEmailButton() {
	const [copyState, setCopyState] = useState<CopyState>("idle");

	useEffect(() => {
		if (copyState === "idle") {
			return;
		}
		const resetTimeout = window.setTimeout(() => setCopyState("idle"), 2000);
		return () => window.clearTimeout(resetTimeout);
	}, [copyState]);

	const copyEmail = async () => {
		try {
			await navigator.clipboard.writeText(EMAIL);
			setCopyState("copied");
		} catch {
			setCopyState("failed");
		}
	};

	return (
		<>
			<button
				className="inline-flex items-center gap-2 rounded-md border border-border px-3 py-2 font-mono text-muted-foreground text-xs uppercase tracking-[0.18em] transition duration-200 hover:border-primary hover:text-primary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-primary active:translate-y-px"
				onClick={copyEmail}
				type="button"
			>
				{copyState === "copied" ? (
					<CheckIcon className="h-3.5 w-3.5 text-primary" />
				) : (
					<CopyIcon className="h-3.5 w-3.5" />
				)}
				{COPY_FEEDBACK[copyState].label}
			</button>
			<span className="sr-only" role="status">
				{COPY_FEEDBACK[copyState].announcement}
			</span>
		</>
	);
}
