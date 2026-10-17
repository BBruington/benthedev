import {
	BellRingIcon,
	BotIcon,
	CopyIcon,
	ExternalLinkIcon,
	KeyRoundIcon,
	ServerIcon,
	UnlinkIcon,
} from "lucide-react";
import { type ReactNode, useState } from "react";
import type { Schemas } from "@/api/client";
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { formatAbsoluteTime } from "@/lib/time";
import { SettingsCard } from "./settings-card";

type LinkedDiscordServer = NonNullable<Schemas["Settings"]["discordServer"]>;
type LinkCode = Schemas["LinkCode"];

const clockTime = new Intl.DateTimeFormat(undefined, { timeStyle: "short" });

/** The /link command the DM runs in their server, with the one-time code. */
export function linkCommand(code: string): string {
	return `/link code:${code}`;
}

/**
 * The campaign's Discord server (docs/design/multi-campaign.md §6.2): invite Beny Bot, get a
 * one-time code, run /link with it in the server; then pick the channels Beny Bot posts in.
 */
export function DiscordServerCard({
	server,
	botInviteUrl,
	linkCode,
	linkCodeExpired,
	isGettingCode,
	onGetCode,
	onCopyCommand,
	isUnlinking,
	onUnlink,
	children,
}: {
	server: LinkedDiscordServer | null;
	botInviteUrl: string | null;
	linkCode: LinkCode | null;
	linkCodeExpired: boolean;
	isGettingCode: boolean;
	onGetCode: () => void;
	onCopyCommand: (command: string) => void;
	isUnlinking: boolean;
	onUnlink: () => void;
	/** The channel pickers, which need a linked server. */
	children: ReactNode;
}) {
	return (
		<SettingsCard
			description="Beny Bot posts join requests, notices and reminders in the channels you pick here, in one Discord server linked to this campaign."
			icon={BellRingIcon}
			id="discord"
			title="Discord server"
		>
			{server ? (
				<LinkedServer isUnlinking={isUnlinking} onUnlink={onUnlink} server={server} />
			) : (
				<ol className="grid gap-4">
					<li className="grid gap-2">
						<StepHeading number={1}>Invite Beny Bot to your server</StepHeading>
						<BenyBotInvite inviteUrl={botInviteUrl} />
					</li>
					<li className="grid gap-2">
						<StepHeading number={2}>Link it to this campaign</StepHeading>
						<LinkCodePanel
							expired={linkCodeExpired}
							isGettingCode={isGettingCode}
							linkCode={linkCode}
							onCopyCommand={onCopyCommand}
							onGetCode={onGetCode}
						/>
					</li>
				</ol>
			)}
			{children}
		</SettingsCard>
	);
}

function StepHeading({ number, children }: { number: number; children: ReactNode }) {
	return (
		<h3 className="flex items-center gap-2.5 font-medium">
			<span
				aria-hidden="true"
				className="inline-flex size-6 shrink-0 items-center justify-center rounded-full bg-gilt/15 font-semibold text-gilt-ink text-xs"
			>
				{number}
			</span>
			<span className="sr-only">Step {number}: </span>
			{children}
		</h3>
	);
}

function LinkedServer({
	server,
	isUnlinking,
	onUnlink,
}: {
	server: LinkedDiscordServer;
	isUnlinking: boolean;
	onUnlink: () => void;
}) {
	const [isConfirmOpen, setConfirmOpen] = useState(false);
	return (
		<div className="flex flex-col gap-3 rounded-xl border border-moss/40 bg-moss/8 px-4 py-4 sm:flex-row sm:items-center sm:justify-between">
			<div className="flex min-w-0 items-center gap-3">
				<ServerIcon aria-hidden="true" className="size-5 shrink-0 text-moss-ink" />
				<div className="min-w-0">
					<p className="font-medium [overflow-wrap:anywhere]">Linked to {server.guildName}</p>
					<p className="text-muted-foreground text-sm">
						Since {formatAbsoluteTime(server.linkedAt)}
					</p>
				</div>
			</div>
			<Button
				className="shrink-0"
				disabled={isUnlinking}
				onClick={() => {
					setConfirmOpen(true);
				}}
				type="button"
				variant="outline"
			>
				<UnlinkIcon />
				{isUnlinking ? "Unlinking…" : "Unlink…"}
			</Button>
			<AlertDialog onOpenChange={setConfirmOpen} open={isConfirmOpen}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>Unlink {server.guildName}?</AlertDialogTitle>
						<AlertDialogDescription>
							Beny Bot stops posting for this campaign and your channel choices are cleared. Beny
							Bot stays in the server; linking again needs a new code.
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>Stay linked</AlertDialogCancel>
						<AlertDialogAction onClick={onUnlink} variant="destructive">
							Unlink
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</div>
	);
}

function LinkCodePanel({
	linkCode,
	expired,
	isGettingCode,
	onGetCode,
	onCopyCommand,
}: {
	linkCode: LinkCode | null;
	expired: boolean;
	isGettingCode: boolean;
	onGetCode: () => void;
	onCopyCommand: (command: string) => void;
}) {
	const showCode = linkCode !== null && !expired;
	return (
		<div className="grid gap-3 rounded-xl border border-border bg-surface-sunken/60 px-4 py-4">
			{showCode ? (
				<div aria-live="polite" className="grid gap-2">
					<p className="text-muted-foreground text-sm">
						In your server, someone who can manage it runs:
					</p>
					<p className="rounded-md border border-border bg-card px-3 py-2 font-mono text-base [overflow-wrap:anywhere]">
						{linkCommand(linkCode.code)}
					</p>
					<p className="text-muted-foreground text-sm">
						The code works once, until {clockTime.format(new Date(linkCode.expiresAt))}.
					</p>
				</div>
			) : (
				<p className="text-muted-foreground text-sm">
					{expired
						? "That code expired. Get a new one."
						: "Get a one-time code, then run /link with it in your server. It works for 15 minutes."}
				</p>
			)}
			<div className="flex flex-wrap gap-2">
				{showCode ? (
					<Button
						onClick={() => {
							onCopyCommand(linkCommand(linkCode.code));
						}}
						type="button"
					>
						<CopyIcon />
						Copy the command
					</Button>
				) : null}
				<Button
					disabled={isGettingCode}
					onClick={onGetCode}
					type="button"
					variant={showCode ? "outline" : "default"}
				>
					<KeyRoundIcon />
					{showCode ? "Get a new code" : "Get a link code"}
				</Button>
			</div>
		</div>
	);
}

function BenyBotInvite({ inviteUrl }: { inviteUrl: string | null }) {
	if (!inviteUrl) {
		return (
			<p className="rounded-xl border border-border border-dashed px-4 py-3 text-muted-foreground text-sm">
				Beny Bot's invite link appears here once{" "}
				<code className="font-mono text-foreground">DISCORD_CLIENT_ID</code> is set in{" "}
				<code className="font-mono text-foreground">.env</code>. The Discord setup guide walks
				through it.
			</p>
		);
	}
	return (
		<div className="flex flex-col gap-3 rounded-xl border border-border bg-surface-sunken/60 px-4 py-4 sm:flex-row sm:items-center sm:justify-between">
			<p className="min-w-0 text-muted-foreground text-sm">
				Adds Beny Bot to your Discord server with the permissions it needs. Someone who manages the
				server has to accept.
			</p>
			<Button asChild className="shrink-0" variant="outline">
				<a href={inviteUrl} rel="noopener noreferrer" target="_blank">
					<BotIcon />
					Invite Beny Bot
					<ExternalLinkIcon aria-hidden="true" />
				</a>
			</Button>
		</div>
	);
}
