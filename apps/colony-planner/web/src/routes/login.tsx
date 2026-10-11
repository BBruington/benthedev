import { useQuery } from "@tanstack/react-query";
import { createFileRoute, redirect } from "@tanstack/react-router";
import {
	CircleAlertIcon,
	CrownIcon,
	FlaskConicalIcon,
	LogInIcon,
	UserPlusIcon,
} from "lucide-react";
import { type FormEvent, useState } from "react";
import type { Schemas } from "@/api/client";
import { problemMessage } from "@/api/problem";
import { loginOptionsQuery } from "@/api/queries";
import { devSignInPath, discordSignInPath } from "@/api/session";
import { Crest } from "@/components/crest";
import { DiscordMark } from "@/components/discord-mark";
import { EldariBackdrop } from "@/components/eldari-backdrop";
import { GiltRule } from "@/components/gilt-rule";
import { MemberAvatar } from "@/components/member-avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { loadMe, tableName } from "@/lib/access";

type LoginSearch = { error?: string };

type DevSignInMember = Schemas["DevSignInMember"];

export const Route = createFileRoute("/login")({
	beforeLoad: async ({ context }) => {
		// Always ask the server: a cached answer could be from a session that has since ended.
		const me = await loadMe(context.queryClient, "fresh-once").catch(() => null);
		if (me?.status === "ACTIVE") {
			throw redirect({ replace: true, to: "/" });
		}
		if (me) {
			throw redirect({ replace: true, to: "/pending" });
		}
	},
	component: LoginPage,
	validateSearch: (search: Record<string, unknown>): LoginSearch => {
		if (typeof search.error === "string") {
			return { error: search.error };
		}
		return {};
	},
});

// The values the Discord callback sends back in ?error= when sign-in can't finish.
function signInErrorMessage(code: string): string {
	if (code === "full") {
		return "The DM isn't taking new players right now. Try again another day.";
	}
	if (code === "discord") {
		return "Discord didn't finish signing you in. Please try again.";
	}
	return "Signing in didn't work. Please try again.";
}

function LoginPage() {
	useDocumentTitle("Sign in", "Colony Planner");
	const { error: errorCode } = Route.useSearch();
	const loginOptions = useQuery(loginOptionsQuery);
	const devMembers = loginOptions.data?.devSignIn;

	return (
		<EldariBackdrop className="gap-6 lg:flex-row lg:items-center lg:gap-8">
			<main className="w-full max-w-md">
				<section className="rounded-2xl border border-border bg-surface/88 px-6 pt-9 pb-7 shadow-float backdrop-blur-md sm:px-9">
					<div className="flex flex-col items-center text-center">
						<Crest className="size-16 text-gilt" />
						<h1 className="mt-5 font-display font-semibold text-3xl text-gilt-ink tracking-[0.08em] sm:text-4xl">
							Colony Planner
						</h1>
						<p className="mt-2 font-flavor text-lg text-muted-foreground italic">
							The colony's ledger, kept between sessions.
						</p>
					</div>
					<GiltRule className="my-7" />
					{errorCode ? (
						<p
							className="mb-5 flex items-start gap-2.5 rounded-lg border border-ember/40 bg-ember/12 px-3.5 py-3 text-sm"
							role="alert"
						>
							<CircleAlertIcon
								aria-hidden="true"
								className="mt-0.5 size-4 shrink-0 text-ember-ink"
							/>
							{signInErrorMessage(errorCode)}
						</p>
					) : null}
					<SignInActions
						discordEnabled={loginOptions.data?.discordEnabled ?? false}
						hasDevSignIn={devMembers !== undefined}
						isLoading={loginOptions.isPending}
						loadError={loginOptions.error}
					/>
					<p className="mt-6 text-center text-muted-foreground text-sm">
						Discord shares only your username and avatar with this site. Nothing is ever posted as
						you.
					</p>
				</section>
			</main>
			{devMembers ? <DevSignInPanel members={devMembers} /> : null}
		</EldariBackdrop>
	);
}

function SignInActions({
	discordEnabled,
	hasDevSignIn,
	isLoading,
	loadError,
}: {
	discordEnabled: boolean;
	hasDevSignIn: boolean;
	isLoading: boolean;
	loadError: unknown;
}) {
	if (isLoading) {
		return <Skeleton className="h-12 w-full rounded-md" />;
	}
	if (loadError) {
		return (
			<div className="grid gap-1 text-center" role="alert">
				<p>Sign-in isn't available right now. Refresh the page in a moment.</p>
				<p className="text-muted-foreground text-sm">
					{problemMessage(loadError, "The server didn't answer.")}
				</p>
			</div>
		);
	}
	if (discordEnabled) {
		return (
			<Button asChild className="w-full" size="lg">
				<a href={discordSignInPath}>
					<DiscordMark className="size-5" />
					Sign in with Discord
				</a>
			</Button>
		);
	}
	if (hasDevSignIn) {
		return (
			<p className="text-center text-muted-foreground">
				Discord sign-in isn't set up on this server. Use dev sign-in instead.
			</p>
		);
	}
	return (
		<p className="text-center text-muted-foreground">Sign-in isn't set up on this server yet.</p>
	);
}

const statusOrder: Record<Schemas["MemberStatus"], number> = { ACTIVE: 0, PENDING: 1, REMOVED: 2 };

function sortForDevSignIn(members: DevSignInMember[]): DevSignInMember[] {
	return [...members].sort((first, second) => {
		if (first.isDm !== second.isDm) {
			return first.isDm ? -1 : 1;
		}
		return (
			statusOrder[first.status] - statusOrder[second.status] ||
			tableName(first).localeCompare(tableName(second))
		);
	});
}

/** Local development only: the server lists every member only in devauth builds (spec §8.5). */
function DevSignInPanel({ members }: { members: DevSignInMember[] }) {
	const [newPlayerName, setNewPlayerName] = useState("");
	const sortedMembers = sortForDevSignIn(members);
	const hasDm = members.some((member) => member.isDm);

	function handleNewPlayer(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		const name = newPlayerName.trim();
		if (name) {
			window.location.assign(devSignInPath("new", name));
		}
	}

	return (
		<aside
			aria-labelledby="dev-sign-in-heading"
			className="w-full max-w-md rounded-2xl border border-gilt/35 border-dashed bg-surface-sunken/85 p-5 backdrop-blur-md sm:p-6"
		>
			<div className="flex items-center gap-2.5">
				<FlaskConicalIcon aria-hidden="true" className="size-5 text-gilt-ink" />
				<h2
					className="font-display font-semibold text-gilt-ink text-lg tracking-wide"
					id="dev-sign-in-heading"
				>
					Dev sign-in
				</h2>
				<Badge className="ml-auto" variant="muted">
					devauth build
				</Badge>
			</div>
			<p className="mt-1.5 text-muted-foreground text-sm">
				Sign in as anyone, no Discord needed. A private window is a second person.
			</p>

			{hasDm ? null : (
				<Button asChild className="mt-4 w-full" variant="outline">
					<a href={devSignInPath("dm")}>
						<CrownIcon />
						Sign in as the DM
					</a>
				</Button>
			)}

			{sortedMembers.length > 0 ? (
				<ul className="mt-4 max-h-80 overflow-y-auto rounded-xl border border-border bg-card/70">
					{sortedMembers.map((member) => (
						<li className="border-border border-b last:border-b-0" key={member.memberId}>
							<a
								className="flex min-h-14 items-center gap-3 px-3.5 py-2 outline-none transition-colors hover:bg-accent focus-visible:bg-accent"
								href={devSignInPath(member.memberId)}
							>
								<MemberAvatar avatarUrl={null} name={tableName(member)} size="sm" />
								<span className="min-w-0 flex-1">
									<span className="block truncate font-medium">{tableName(member)}</span>
									{member.characterName ? (
										<span className="block truncate text-muted-foreground text-sm">
											{member.displayName}
										</span>
									) : null}
								</span>
								<DevMemberBadge member={member} />
								<LogInIcon aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
							</a>
						</li>
					))}
				</ul>
			) : (
				<p className="mt-4 rounded-xl border border-border border-dashed px-4 py-5 text-center text-muted-foreground text-sm">
					No members yet. Run <code className="font-mono text-foreground">mise run seed</code> for
					the sample campaign.
				</p>
			)}

			<form className="mt-5 grid gap-2" onSubmit={handleNewPlayer}>
				<Label htmlFor="dev-new-player-name">A new player asking to join</Label>
				<div className="flex gap-2">
					<Input
						autoComplete="off"
						id="dev-new-player-name"
						maxLength={60}
						onChange={(event) => {
							setNewPlayerName(event.target.value);
						}}
						placeholder="Their Discord name"
						value={newPlayerName}
					/>
					<Button disabled={newPlayerName.trim() === ""} type="submit" variant="secondary">
						<UserPlusIcon />
						Ask
					</Button>
				</div>
			</form>
		</aside>
	);
}

function DevMemberBadge({ member }: { member: DevSignInMember }) {
	if (member.isDm) {
		return <Badge variant="gilt">DM</Badge>;
	}
	if (member.status === "PENDING") {
		return <Badge variant="verdigris">Waiting</Badge>;
	}
	if (member.status === "REMOVED") {
		return <Badge variant="ember">Removed</Badge>;
	}
	return <Badge variant="moss">Player</Badge>;
}
