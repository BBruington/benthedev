import { CopyIcon, DicesIcon, KeyRoundIcon, PlugIcon, UnplugIcon } from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";
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
import { formatNumber } from "@/lib/colony";
import { focusFirstVisible } from "@/lib/focus";
import { formatAbsoluteTime } from "@/lib/time";
import { SettingsCard } from "./settings-card";
import { StepHeading } from "./step-heading";

type BridgeStatus = Schemas["BridgeStatus"];
type BridgeLink = NonNullable<BridgeStatus["link"]>;
type BridgePairingCode = Schemas["BridgePairingCode"];
type BridgeOutboxKind = Schemas["BridgeOutboxKind"];

export const foundryCardId = "foundry";
export const foundryHeadingId = `${foundryCardId}-heading`;

const clockTime = new Intl.DateTimeFormat(undefined, { timeStyle: "short" });
// lastContactOn is a UTC calendar day, so it is shown as that day wherever the DM is.
const calendarDay = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeZone: "UTC" });

const outboxKindNames: Record<BridgeOutboxKind, string> = {
	COLONY_SUMMARY: "The Colony journal page",
	DAYS_ADVANCED: "A days-passed chat message",
};

function originHost(origin: string): string {
	return URL.canParse(origin) ? new URL(origin).host : origin;
}

/**
 * Colony Bridge, the Foundry module (docs/design/foundry-bridge.md §3.1): install it from the
 * manifest address, make a one-time code here, type it into the module's settings in Foundry. Once
 * connected, the card shows the world and what is waiting for it; the inbox comes in as children.
 */
export function FoundryCard({
	status,
	pairingCode,
	pairingCodeExpired,
	isMakingCode,
	onMakeCode,
	isDisconnecting,
	onDisconnect,
	isClearingFailed,
	onClearFailed,
	onCopyManifest,
	children,
}: {
	status: BridgeStatus;
	/** The code this page made; the server returns it only once. */
	pairingCode: BridgePairingCode | null;
	pairingCodeExpired: boolean;
	isMakingCode: boolean;
	onMakeCode: () => void;
	isDisconnecting: boolean;
	onDisconnect: () => void;
	isClearingFailed: boolean;
	onClearFailed: () => void;
	onCopyManifest: (manifestUrl: string) => void;
	/** The Foundry inbox, while it has items. */
	children: ReactNode;
}) {
	useFocusHeadingWhenConnected(status.link !== null);
	const codePanel = (
		<PairingCodePanel
			expired={pairingCodeExpired}
			isConnected={status.link !== null}
			isMakingCode={isMakingCode}
			onMakeCode={onMakeCode}
			openCodeExpiresAt={status.code?.expiresAt ?? null}
			pairingCode={pairingCode}
		/>
	);
	return (
		<SettingsCard
			description="Colony Bridge is a Foundry module. It keeps a Colony journal page in your Foundry world up to date, and tells you here when Foundry's clock moves on."
			icon={DicesIcon}
			id={foundryCardId}
			title="Foundry"
		>
			{status.link ? (
				<>
					<ConnectedWorld
						isClearingFailed={isClearingFailed}
						isDisconnecting={isDisconnecting}
						link={status.link}
						onClearFailed={onClearFailed}
						onDisconnect={onDisconnect}
					/>
					{codePanel}
				</>
			) : (
				<ol className="grid gap-4">
					<li className="grid gap-2">
						<StepHeading number={1}>Install Colony Bridge in Foundry</StepHeading>
						<ManifestAddress manifestUrl={status.moduleManifestUrl} onCopy={onCopyManifest} />
					</li>
					<li className="grid gap-2">
						<StepHeading number={2}>Connect it to this campaign</StepHeading>
						{codePanel}
					</li>
				</ol>
			)}
			{children}
		</SettingsCard>
	);
}

/**
 * Pairing finishes in Foundry, so the steps list (and the button focus was on) gives way to the
 * connected world without anything here being pressed: focus that was in the card goes to its heading.
 */
function useFocusHeadingWhenConnected(isConnected: boolean) {
	const focusWasInCard = useRef(false);
	const wasConnected = useRef(isConnected);

	useEffect(() => {
		// Removing the focused element moves focus to body without a focusin, so this keeps the last real move.
		function trackFocus(event: FocusEvent) {
			const card = document.getElementById(foundryCardId);
			focusWasInCard.current =
				event.target instanceof Node && card !== null && card.contains(event.target);
		}
		document.addEventListener("focusin", trackFocus);
		return () => {
			document.removeEventListener("focusin", trackFocus);
		};
	}, []);

	useEffect(() => {
		const linkAppeared = isConnected && !wasConnected.current;
		wasConnected.current = isConnected;
		const focusNow = document.activeElement;
		if (
			linkAppeared &&
			focusWasInCard.current &&
			(focusNow === null || focusNow === document.body)
		) {
			focusFirstVisible(foundryHeadingId);
		}
	}, [isConnected]);
}

function ManifestAddress({
	manifestUrl,
	onCopy,
}: {
	manifestUrl: string;
	onCopy: (manifestUrl: string) => void;
}) {
	return (
		<div className="grid gap-3 rounded-xl border border-border bg-surface-sunken/60 px-4 py-4">
			<p className="text-muted-foreground text-sm">
				In Foundry: Add-on Modules, Install Module, paste into Manifest URL. Then turn it on in your
				world's Manage Modules.
			</p>
			<p className="rounded-md border border-border bg-card px-3 py-2 font-mono text-sm [overflow-wrap:anywhere]">
				{manifestUrl}
			</p>
			<div>
				<Button
					onClick={() => {
						onCopy(manifestUrl);
					}}
					type="button"
					variant="outline"
				>
					<CopyIcon />
					Copy the address
				</Button>
			</div>
		</div>
	);
}

function PairingCodePanel({
	pairingCode,
	expired,
	openCodeExpiresAt,
	isConnected,
	isMakingCode,
	onMakeCode,
}: {
	pairingCode: BridgePairingCode | null;
	expired: boolean;
	openCodeExpiresAt: string | null;
	isConnected: boolean;
	isMakingCode: boolean;
	onMakeCode: () => void;
}) {
	const showCode = pairingCode !== null && !expired;
	let hint = "Make a one-time code, then type it into Colony Bridge's settings in Foundry.";
	if (expired) {
		hint = "That code expired. Make a new one.";
	} else if (openCodeExpiresAt) {
		hint = `A code made earlier works until ${clockTime.format(new Date(openCodeExpiresAt))}. Make a new one to see it here; the old one stops working.`;
	} else if (isConnected) {
		hint =
			"Connecting from another browser or world? Make a new code. Once it's used, it replaces this connection.";
	}
	let buttonLabel = "Connect Foundry";
	if (showCode) {
		buttonLabel = "Make a new code";
	} else if (isConnected) {
		buttonLabel = "Connect again";
	}
	return (
		<div className="grid gap-3 rounded-xl border border-border bg-surface-sunken/60 px-4 py-4">
			{showCode ? (
				<div aria-live="polite" className="grid gap-2">
					<p className="text-muted-foreground text-sm">
						Type this code into Colony Bridge's settings in Foundry:
					</p>
					<p className="font-mono font-semibold text-4xl text-gilt-ink tracking-[0.2em]">
						{pairingCode.code}
					</p>
					<p className="text-muted-foreground text-sm">
						It works once, until {clockTime.format(new Date(pairingCode.expiresAt))}.
					</p>
				</div>
			) : (
				<p className="text-muted-foreground text-sm">{hint}</p>
			)}
			<div>
				<Button
					disabled={isMakingCode}
					onClick={onMakeCode}
					type="button"
					variant={showCode || isConnected ? "outline" : "default"}
				>
					<KeyRoundIcon />
					{isMakingCode ? "Making a code…" : buttonLabel}
				</Button>
			</div>
		</div>
	);
}

function ConnectedWorld({
	link,
	isDisconnecting,
	onDisconnect,
	isClearingFailed,
	onClearFailed,
}: {
	link: BridgeLink;
	isDisconnecting: boolean;
	onDisconnect: () => void;
	isClearingFailed: boolean;
	onClearFailed: () => void;
}) {
	const [isConfirmOpen, setConfirmOpen] = useState(false);
	const [confirmed, setConfirmed] = useState(false);
	const host = originHost(link.origin);
	const system = [link.systemId, link.systemVersion].filter(Boolean).join(" ");
	const facts = [
		{ label: "World", value: link.worldTitle },
		{ label: "Foundry", value: link.foundryVersion },
		{ label: "Game system", value: system },
		{ label: "Colony Bridge", value: link.moduleVersion },
	].filter((fact) => fact.value !== "");
	let waiting = "Foundry is up to date.";
	if (link.unappliedRows === 1) {
		waiting = "1 change is waiting for Foundry.";
	} else if (link.unappliedRows > 1) {
		waiting = `${formatNumber(link.unappliedRows)} changes are waiting for Foundry.`;
	}

	return (
		<div className="grid gap-4 rounded-xl border border-moss/40 bg-moss/8 px-4 py-4">
			<div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
				<div className="flex min-w-0 items-start gap-3">
					<PlugIcon aria-hidden="true" className="mt-0.5 size-5 shrink-0 text-moss-ink" />
					<div className="min-w-0">
						<p aria-live="polite" className="font-medium [overflow-wrap:anywhere]">
							Connected to {host}
						</p>
						<p className="text-muted-foreground text-sm">
							Since {formatAbsoluteTime(link.linkedAt)}
						</p>
						<p className="text-muted-foreground text-sm">
							{link.lastContactOn
								? `Last heard from ${calendarDay.format(Date.parse(link.lastContactOn))}`
								: "Not heard from yet"}
						</p>
					</div>
				</div>
				<Button
					className="shrink-0"
					disabled={isDisconnecting}
					onClick={() => {
						setConfirmOpen(true);
					}}
					type="button"
					variant="outline"
				>
					<UnplugIcon />
					{isDisconnecting ? "Disconnecting…" : "Disconnect…"}
				</Button>
			</div>
			{facts.length > 0 ? (
				<dl className="grid gap-x-6 gap-y-1 text-sm sm:grid-cols-[auto_1fr]">
					{facts.map((fact) => (
						<div className="contents" key={fact.label}>
							<dt className="text-muted-foreground">{fact.label}</dt>
							<dd className="mb-1 [overflow-wrap:anywhere] sm:mb-0">{fact.value}</dd>
						</div>
					))}
				</dl>
			) : null}
			<p className="text-sm">{waiting}</p>
			{link.failedRows.length > 0 ? (
				<div className="grid gap-2">
					<div className="flex items-center justify-between gap-3">
						<p className="font-medium text-ember-ink text-sm">Foundry couldn't apply these:</p>
						<Button
							disabled={isClearingFailed}
							onClick={onClearFailed}
							size="sm"
							type="button"
							variant="ghost"
						>
							{isClearingFailed ? "Clearing…" : "Clear"}
						</Button>
					</div>
					<ul className="grid gap-2">
						{link.failedRows.map((row) => (
							<li
								className="rounded-lg border border-ember/40 bg-ember/10 px-3 py-2 text-sm"
								key={row.seq}
							>
								<span className="font-medium">{outboxKindNames[row.kind]}</span>
								<span className="text-muted-foreground"> · {formatAbsoluteTime(row.failedAt)}</span>
								<p className="[overflow-wrap:anywhere]">{row.reason}</p>
							</li>
						))}
					</ul>
				</div>
			) : null}
			<AlertDialog onOpenChange={setConfirmOpen} open={isConfirmOpen}>
				<AlertDialogContent
					onCloseAutoFocus={(event) => {
						if (confirmed) {
							event.preventDefault();
							setConfirmed(false);
							focusFirstVisible(foundryHeadingId);
						}
					}}
				>
					<AlertDialogHeader>
						<AlertDialogTitle>Disconnect {host}?</AlertDialogTitle>
						<AlertDialogDescription>
							Foundry's Colony page stops updating and this campaign stops hearing from Foundry. The
							page stays in your world. Connecting again needs a new code.
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>Stay connected</AlertDialogCancel>
						<AlertDialogAction
							onClick={() => {
								setConfirmed(true);
								onDisconnect();
							}}
							variant="destructive"
						>
							Disconnect
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</div>
	);
}
