import { type ErrorComponentProps, Link, useRouter } from "@tanstack/react-router";
import {
	CompassIcon,
	HammerIcon,
	type LucideIcon,
	RefreshCwIcon,
	ServerCrashIcon,
	WavesIcon,
} from "lucide-react";
import { problemMessage, problemStatus } from "@/api/problem";
import { EldariBackdrop } from "@/components/eldari-backdrop";
import { StatusPanel } from "@/components/status-panel";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

type Failure = { title: string; detail: string; status?: number; icon: LucideIcon };

function describeFailure(error: unknown): Failure {
	const status = problemStatus(error);
	const detail = problemMessage(error, "Something unexpected went wrong.");
	if (status === 501) {
		return { detail, icon: HammerIcon, status, title: "Not built yet" };
	}
	if (status === 502 || status === 503 || status === 504) {
		return { detail, icon: WavesIcon, status, title: "Can't reach the colony" };
	}
	return { detail, icon: ServerCrashIcon, status, title: "Something went wrong" };
}

function RetryButton() {
	const router = useRouter();
	return (
		<Button
			onClick={() => {
				router.invalidate();
			}}
		>
			<RefreshCwIcon />
			Try again
		</Button>
	);
}

/** A failed screen inside the app shell, so navigation stays usable. */
export function PanelError({ error }: ErrorComponentProps) {
	const failure = describeFailure(error);
	return (
		<StatusPanel
			actions={<RetryButton />}
			description={failure.detail}
			eyebrow={failure.status ? <Badge variant="ember">Error {failure.status}</Badge> : undefined}
			icon={failure.icon}
			title={failure.title}
			tone="ember"
		/>
	);
}

/** A failure before the shell could load: the guard couldn't learn who this is. */
export function FullScreenError({ error }: ErrorComponentProps) {
	const failure = describeFailure(error);
	return (
		<EldariBackdrop>
			<div className="w-full max-w-lg rounded-2xl border border-border bg-surface/92 px-6 py-2 shadow-float backdrop-blur-md">
				<StatusPanel
					actions={<RetryButton />}
					className="py-8 md:py-10"
					description={failure.detail}
					eyebrow={
						failure.status ? <Badge variant="ember">Error {failure.status}</Badge> : undefined
					}
					icon={failure.icon}
					title={failure.title}
					tone="ember"
				/>
			</div>
		</EldariBackdrop>
	);
}

export function NotFoundScreen() {
	return (
		<EldariBackdrop>
			<div className="w-full max-w-lg rounded-2xl border border-border bg-surface/92 px-6 py-2 shadow-float backdrop-blur-md">
				<StatusPanel
					actions={
						<Button asChild>
							<Link to="/">Back to the colony</Link>
						</Button>
					}
					className="py-8 md:py-10"
					description="No page lives at this address. It may have moved, or the link was mistyped."
					eyebrow={<Badge variant="gilt">404</Badge>}
					flavor="Beyond this point the map is blank."
					icon={CompassIcon}
					title="Uncharted waters"
				/>
			</div>
		</EldariBackdrop>
	);
}
