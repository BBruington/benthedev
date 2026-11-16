import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import {
	CalendarClockIcon,
	HammerIcon,
	MegaphoneIcon,
	RotateCcwIcon,
	SaveIcon,
	ScrollIcon,
	Settings2Icon,
	ShieldIcon,
} from "lucide-react";
import { type FormEvent, useEffect, useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import {
	bridgeStatusQuery,
	calendarLinkQuery,
	campaignMeQuery,
	colonyBoardQuery,
	discordChannelsQuery,
	inviteLinkQuery,
	myCampaignRequestQuery,
	myCampaignsQuery,
	resourcesQuery,
	rolesQuery,
	settingsQuery,
} from "@/api/queries";
import { FoundryInbox } from "@/colony/foundry-inbox";
import { ResourceSettingsCard } from "@/colony/resource-settings";
import { Field, fieldInputProps, fieldMessageIds } from "@/components/field";
import { NativeSelect } from "@/components/form-controls";
import { PageHeader } from "@/components/page-header";
import { ChannelSelect } from "@/components/settings/channel-select";
import { DangerZone } from "@/components/settings/danger-zone";
import { DiscordServerCard } from "@/components/settings/discord-server-card";
import { FoundryCard, foundryCardId, foundryHeadingId } from "@/components/settings/foundry-card";
import { InviteLinkCard } from "@/components/settings/invite-link-card";
import { SettingsCard } from "@/components/settings/settings-card";
import { campaignPickerHeadingId } from "@/components/shell/account-ids";
import { Forbidden } from "@/components/shell/forbidden";
import { StatusPanel } from "@/components/status-panel";
import { TimezonePicker } from "@/components/timezone-picker";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useCampaign } from "@/hooks/use-campaign";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { useMe } from "@/hooks/use-me";
import { cachedMe, canUseDmTools } from "@/lib/access";
import { inviteLink } from "@/lib/campaigns";
import { copyToClipboard } from "@/lib/clipboard";
import { focusFirstVisible } from "@/lib/focus";
import { forgetLastCampaign } from "@/lib/last-campaign";
import { cn } from "@/lib/utils";
import { RoleSettingsCard } from "@/people/role-settings";
import { CalendarLinkCard } from "@/sessions/session-parts";

type Settings = Schemas["Settings"];
type SettingsInput = Schemas["SettingsInput"];
type LinkCode = Schemas["LinkCode"];
type BridgePairingCode = Schemas["BridgePairingCode"];

// Selects hold strings; the API takes null for an unset channel.
type SettingsDraft = {
	campaignName: string;
	colonyName: string;
	timezone: string;
	constructionRoleKey: string;
	discordAnnounceChannelId: string;
	discordReminderChannelId: string;
	discordDmChannelId: string;
	discordEventLocation: string;
};

type ChannelField = "discordAnnounceChannelId" | "discordReminderChannelId" | "discordDmChannelId";

export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell/settings")({
	component: SettingsRoute,
	loader: ({ context: { campaignId, queryClient } }) => {
		if (!campaignId) {
			return;
		}
		const me = cachedMe(queryClient, campaignId);
		if (me && canUseDmTools(me)) {
			queryClient.prefetchQuery(settingsQuery(campaignId));
			queryClient.prefetchQuery(resourcesQuery(campaignId));
			queryClient.prefetchQuery(rolesQuery(campaignId));
			queryClient.prefetchQuery(inviteLinkQuery(campaignId));
			queryClient.prefetchQuery(calendarLinkQuery(campaignId));
			queryClient.prefetchQuery(bridgeStatusQuery(campaignId));
		}
	},
});

function SettingsRoute() {
	const me = useMe();
	if (!canUseDmTools(me)) {
		return <Forbidden me={me} />;
	}
	return <SettingsPage />;
}

function SettingsPage() {
	const { campaignId } = useCampaign();
	const settings = useQuery(settingsQuery(campaignId));
	useDocumentTitle("Settings", settings.data?.colonyName);

	return (
		<div className="flex flex-col gap-8 md:gap-10">
			<PageHeader
				description="The campaign's and colony's names, the time zone, the construction role, who can join, the calendar link, the colony's resources, Beny Bot's Discord server, and your Foundry world."
				eyebrow="Dungeon Master"
				title="Settings"
			/>
			<SettingsBody settings={settings} />
		</div>
	);
}

function SettingsBody({
	settings,
}: {
	settings: { data?: Settings; isPending: boolean; isError: boolean; error: unknown };
}) {
	if (settings.isPending) {
		return (
			<div aria-busy="true" className="flex flex-col gap-6">
				<Skeleton className="h-64 rounded-2xl" />
				<Skeleton className="h-80 rounded-2xl" />
			</div>
		);
	}
	if (settings.isError || !settings.data) {
		return (
			<StatusPanel
				description={problemMessage(settings.error, "The settings couldn't be loaded.")}
				icon={Settings2Icon}
				title="Settings couldn't be loaded"
				tone="ember"
			/>
		);
	}
	return (
		<>
			<SettingsForm settings={settings.data} />
			<FoundryBridge />
			<InviteLink />
			<CalendarLinkCard canRegenerate />
			<ResourceSettingsCard />
			<RoleSettingsCard />
			<DeleteCampaign campaignName={settings.data.campaignName} />
		</>
	);
}

function copyText(text: string, success: string) {
	copyToClipboard(text).then(
		() => {
			toast.success(success);
		},
		() => {
			toast.error("Copying didn't work. Select the text and copy it yourself.");
		},
	);
}

const pairingCodeCheckInMilliseconds = 15_000;

/**
 * The Foundry card's data and actions. The pairing code comes back only from the request that made
 * it, so it lives in this component; pairing happens in Foundry, and the bridge topic then brings
 * the connected world here.
 */
function FoundryBridge() {
	const { campaignId } = useCampaign();
	const queryClient = useQueryClient();
	const status = useQuery(bridgeStatusQuery(campaignId));
	const [pairingCode, setPairingCode] = useState<BridgePairingCode | null>(null);
	const [now, setNow] = useState(() => Date.now());
	const linkedAt = status.data?.link?.linkedAt;
	const [codeMadeBefore, setCodeMadeBefore] = useState(linkedAt);

	// A code that was used is no longer worth showing: the world it connected is on the card now.
	if (linkedAt !== codeMadeBefore) {
		setCodeMadeBefore(linkedAt);
		setPairingCode(null);
	}

	useEffect(() => {
		if (pairingCode === null) {
			return;
		}
		const timer = setInterval(() => {
			setNow(Date.now());
		}, pairingCodeCheckInMilliseconds);
		return () => {
			clearInterval(timer);
		};
	}, [pairingCode]);

	const makeCode = api.useMutation("post", "/api/campaigns/{campaignId}/bridge/codes", {
		onError: (error) => {
			toast.error(problemMessage(error, "A code couldn't be made. Try again."));
		},
		// The card shows the code only while the status names it as the open code, so the status is
		// read again first; the button says "Making a code…" until both are in.
		onSuccess: async (code) => {
			await queryClient.invalidateQueries({ queryKey: bridgeStatusQuery(campaignId).queryKey });
			setNow(Date.now());
			setPairingCode(code);
		},
	});
	const disconnect = api.useMutation("delete", "/api/campaigns/{campaignId}/bridge", {
		onError: (error) => {
			toast.error(problemMessage(error, "Foundry couldn't be disconnected."));
		},
		onSuccess: () => {
			setPairingCode(null);
			queryClient.invalidateQueries({ queryKey: bridgeStatusQuery(campaignId).queryKey });
			toast.success("Disconnected. Foundry no longer hears from this campaign.");
			focusFirstVisible(foundryHeadingId);
		},
	});

	const clearFailed = api.useMutation("post", "/api/campaigns/{campaignId}/bridge/failed/clear", {
		onError: (error) => {
			toast.error(problemMessage(error, "The list couldn't be cleared. Try again."));
		},
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: bridgeStatusQuery(campaignId).queryKey });
			focusFirstVisible(foundryHeadingId);
		},
	});

	if (status.isPending) {
		return <Skeleton className="h-64 rounded-2xl" id={foundryCardId} />;
	}
	if (status.isError) {
		return (
			<StatusPanel
				description={problemMessage(status.error, "The Foundry connection couldn't be loaded.")}
				icon={Settings2Icon}
				title="Foundry couldn't be loaded"
				tone="ember"
			/>
		);
	}
	return (
		<FoundryCard
			isClearingFailed={clearFailed.isPending}
			isDisconnecting={disconnect.isPending}
			isMakingCode={makeCode.isPending}
			onClearFailed={() => {
				clearFailed.mutate({ params: { path: { campaignId } } });
			}}
			onCopyManifest={(manifestUrl) => {
				copyText(manifestUrl, "Address copied. Paste it into Foundry's Manifest URL.");
			}}
			onDisconnect={() => {
				disconnect.mutate({ params: { path: { campaignId } } });
			}}
			onMakeCode={() => {
				makeCode.mutate({ params: { path: { campaignId } } });
			}}
			pairingCode={pairingCode}
			pairingCodeExpired={pairingCode !== null && Date.parse(pairingCode.expiresAt) <= now}
			status={status.data}
		>
			{status.data.inbox.length > 0 ? (
				<section aria-labelledby="foundry-inbox-settings-heading" className="grid gap-3">
					<h3 className="font-medium" id="foundry-inbox-settings-heading">
						Waiting for you
					</h3>
					<FoundryInbox fallbackFocusIds={[foundryHeadingId]} items={status.data.inbox} />
				</section>
			) : null}
		</FoundryCard>
	);
}

/** The invite link card's data: the code is read only here, and replaced by Reset (§4.2). */
function InviteLink() {
	const { campaignId } = useCampaign();
	const queryClient = useQueryClient();
	const invite = useQuery(inviteLinkQuery(campaignId));
	const reset = api.useMutation("post", "/api/campaigns/{campaignId}/invite/reset", {
		onError: (error) => {
			toast.error(problemMessage(error, "The invite link couldn't be reset."));
		},
		onSuccess: (fresh) => {
			queryClient.setQueryData(inviteLinkQuery(campaignId).queryKey, fresh);
			toast.success("New invite link ready. The old one stopped working.");
		},
	});
	return (
		<InviteLinkCard
			inviteUrl={
				invite.data ? inviteLink(window.location.origin, invite.data.inviteCode) : undefined
			}
			isResetting={reset.isPending}
			loadError={
				invite.isError
					? problemMessage(invite.error, "The invite link couldn't be loaded.")
					: undefined
			}
			onCopy={(inviteUrl) => {
				copyText(inviteUrl, "Invite link copied.");
			}}
			onReset={() => {
				reset.mutate({ params: { path: { campaignId } } });
			}}
		/>
	);
}

/** C3: the DM deletes the campaign; everyone in it lands on "No such campaign" (§5.5). */
function DeleteCampaign({ campaignName }: { campaignName: string }) {
	const { campaignId, slug } = useCampaign();
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const remove = api.useMutation("post", "/api/campaigns/{campaignId}/delete", {
		onSuccess: async () => {
			forgetLastCampaign();
			await navigate({ to: "/" });
			queryClient.removeQueries({
				predicate: (query) => JSON.stringify(query.queryKey).includes(campaignId),
			});
			queryClient.invalidateQueries({ queryKey: myCampaignsQuery.queryKey });
			// The picker's "Run your own campaign" card would still offer the deleted campaign.
			queryClient.invalidateQueries({ queryKey: myCampaignRequestQuery.queryKey });
			toast.success(`${campaignName} was deleted.`);
			document.getElementById(campaignPickerHeadingId)?.focus();
		},
	});
	return (
		<DangerZone
			campaignName={campaignName}
			error={remove.error}
			isDeleting={remove.isPending}
			onDelete={(confirmSlug) => {
				remove.mutate({ body: { confirmSlug }, params: { path: { campaignId } } });
			}}
			onOpen={() => {
				remove.reset();
			}}
			slug={slug}
		/>
	);
}

function toDraft(settings: Settings): SettingsDraft {
	return {
		campaignName: settings.campaignName,
		colonyName: settings.colonyName,
		constructionRoleKey: settings.constructionRoleKey ?? "",
		discordAnnounceChannelId: settings.discordAnnounceChannelId ?? "",
		discordDmChannelId: settings.discordDmChannelId ?? "",
		discordEventLocation: settings.discordEventLocation,
		discordReminderChannelId: settings.discordReminderChannelId ?? "",
		timezone: settings.timezone,
	};
}

function optionalText(value: string): string | null {
	const trimmed = value.trim();
	return trimmed === "" ? null : trimmed;
}

function toInput(draft: SettingsDraft): SettingsInput {
	return {
		campaignName: draft.campaignName.trim(),
		colonyName: draft.colonyName.trim(),
		constructionRoleKey: optionalText(draft.constructionRoleKey),
		discordAnnounceChannelId: optionalText(draft.discordAnnounceChannelId),
		discordDmChannelId: optionalText(draft.discordDmChannelId),
		// Always sent: the server reads a missing location as a reset to the default.
		discordEventLocation: draft.discordEventLocation.trim(),
		discordReminderChannelId: optionalText(draft.discordReminderChannelId),
		timezone: draft.timezone,
	};
}

function sameSettings(first: SettingsDraft, second: SettingsDraft): boolean {
	return JSON.stringify(toInput(first)) === JSON.stringify(toInput(second));
}

function SettingsForm({ settings }: { settings: Settings }) {
	const queryClient = useQueryClient();
	const { campaignId } = useCampaign();
	const [draft, setDraft] = useState(() => toDraft(settings));
	const [baseline, setBaseline] = useState(settings);
	const [changedElsewhere, setChangedElsewhere] = useState(false);
	const discord = useDiscordLink(settings);

	// Settings can change on another device (the live stream refetches them). Untouched forms follow
	// along; a form with edits keeps them and says so, rather than losing someone's typing.
	if (settings !== baseline) {
		setBaseline(settings);
		if (sameSettings(draft, toDraft(baseline))) {
			setDraft(toDraft(settings));
		} else if (!sameSettings(draft, toDraft(settings))) {
			setChangedElsewhere(true);
		}
	}

	const mutation = api.useMutation("put", "/api/campaigns/{campaignId}/settings", {
		onError: (error) => {
			if (Object.keys(fieldErrors(error)).length === 0) {
				toast.error(problemMessage(error, "The settings couldn't be saved."));
			}
		},
		onSuccess: (saved) => {
			setDraft(toDraft(saved));
			setChangedElsewhere(false);
			queryClient.setQueryData(settingsQuery(campaignId).queryKey, saved);
			// The names also head the board, the switcher and the picker.
			queryClient.invalidateQueries({ queryKey: colonyBoardQuery(campaignId).queryKey });
			queryClient.invalidateQueries({ queryKey: campaignMeQuery(campaignId).queryKey });
			queryClient.invalidateQueries({ queryKey: myCampaignsQuery.queryKey });
			toast.success("Settings saved.");
		},
	});

	const errors = fieldErrors(mutation.error);
	const isDirty = !sameSettings(draft, toDraft(settings));

	function update<Key extends keyof SettingsDraft>(key: Key, value: SettingsDraft[Key]) {
		setDraft((current) => ({ ...current, [key]: value }));
	}

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		mutation.mutate({ body: toInput(draft), params: { path: { campaignId } } });
	}

	function discard() {
		setDraft(toDraft(settings));
		setChangedElsewhere(false);
		mutation.reset();
	}

	const channelProps = (field: ChannelField) => ({
		channels: discord.channels,
		disabledReason: discord.channelsDisabledReason,
		error: errors[field],
		id: `settings-${field}`,
		onChange: (value: string) => {
			update(field, value);
		},
		value: draft[field],
	});

	return (
		<form className="flex flex-col gap-6" noValidate onSubmit={handleSubmit}>
			<SettingsCard
				description="How the campaign appears across the site, and the clock its sessions keep."
				icon={ScrollIcon}
				title="Campaign"
			>
				<Field
					description="The site owner sees this name."
					error={errors.campaignName}
					id="settings-campaign-name"
					label="Campaign name"
				>
					<Input
						{...fieldInputProps("settings-campaign-name", errors.campaignName, true)}
						autoComplete="off"
						maxLength={80}
						onChange={(event) => {
							update("campaignName", event.target.value);
						}}
						required
						value={draft.campaignName}
					/>
				</Field>
				<Field
					description="Everyone sees it at the top of the Colony page."
					error={errors.colonyName}
					id="settings-colony-name"
					label="Colony name"
				>
					<Input
						{...fieldInputProps("settings-colony-name", errors.colonyName, true)}
						autoComplete="off"
						maxLength={80}
						onChange={(event) => {
							update("colonyName", event.target.value);
						}}
						required
						value={draft.colonyName}
					/>
				</Field>
				<Field
					description="Game nights are scheduled in this zone. Everyone sees times in their own."
					error={errors.timezone}
					id="settings-timezone"
					label="Time zone"
				>
					<TimezonePicker
						describedBy={
							errors.timezone
								? fieldMessageIds("settings-timezone").error
								: fieldMessageIds("settings-timezone").description
						}
						id="settings-timezone"
						invalid={errors.timezone !== undefined}
						onChange={(timezone) => {
							update("timezone", timezone);
						}}
						value={draft.timezone}
					/>
				</Field>
			</SettingsCard>

			<SettingsCard
				description="The work new buildings need, on the Buildings page."
				icon={HammerIcon}
				title="Construction"
			>
				<ConstructionRoleSelect
					error={errors.constructionRoleKey}
					onChange={(roleKey) => {
						update("constructionRoleKey", roleKey);
					}}
					value={draft.constructionRoleKey}
				/>
			</SettingsCard>

			<DiscordServerCard
				botInviteUrl={settings.botInviteUrl}
				isGettingCode={discord.isGettingCode}
				isUnlinking={discord.isUnlinking}
				linkCode={discord.linkCode}
				linkCodeExpired={discord.linkCodeExpired}
				onCopyCommand={(command) => {
					copyText(command, "Command copied. Paste it in your server.");
				}}
				onGetCode={discord.getCode}
				onUnlink={discord.unlink}
				server={settings.discordServer}
			>
				<ChannelSelect
					{...channelProps("discordAnnounceChannelId")}
					description="Game-night events and notices for everyone."
					icon={MegaphoneIcon}
					label="Announcements channel"
				/>
				<ChannelSelect
					{...channelProps("discordReminderChannelId")}
					description="A reminder about a day before each game."
					icon={CalendarClockIcon}
					label="Reminders channel"
				/>
				<ChannelSelect
					{...channelProps("discordDmChannelId")}
					description="Join requests and patron requests, for your eyes only. Keep this channel private."
					icon={ShieldIcon}
					label="DM channel"
				/>
				<Field
					description="Shown as the place on Beny Bot's game-night events. Leave it blank for Online."
					error={errors.discordEventLocation}
					id="settings-event-location"
					label="Event location"
				>
					<Input
						{...fieldInputProps("settings-event-location", errors.discordEventLocation, true)}
						autoComplete="off"
						maxLength={100}
						onChange={(event) => {
							update("discordEventLocation", event.target.value);
						}}
						value={draft.discordEventLocation}
					/>
				</Field>
			</DiscordServerCard>

			<SaveBar
				changedElsewhere={changedElsewhere}
				isDirty={isDirty}
				isSaving={mutation.isPending}
				onDiscard={discard}
			/>
		</form>
	);
}

const constructionRoleId = "settings-construction-role";

/** Unarchived roles only: the server refuses an archived one, and archiving refuses this role. */
function ConstructionRoleSelect({
	value,
	error,
	onChange,
}: {
	value: string;
	error: string | undefined;
	onChange: (roleKey: string) => void;
}) {
	const roles = useQuery(rolesQuery(useCampaign().campaignId));
	const choices = (roles.data?.roles ?? []).filter((role) => !role.archived || role.key === value);
	const isListed = value === "" || choices.some((role) => role.key === value);
	return (
		<Field
			description="New blueprints start with this role's labor."
			error={error}
			id={constructionRoleId}
			label="Construction role"
			optional
		>
			<NativeSelect
				{...fieldInputProps(constructionRoleId, error, true)}
				disabled={roles.isPending}
				onChange={(event) => {
					onChange(event.target.value);
				}}
				value={value}
			>
				<option value="">None</option>
				{choices.map((role) => (
					<option key={role.key} value={role.key}>
						{role.label}
					</option>
				))}
				{isListed ? null : <option value={value}>{value}</option>}
			</NativeSelect>
		</Field>
	);
}

const linkCodeCheckInMilliseconds = 15_000;

/**
 * The Discord card's data and actions: the one-time link code (returned only to this request,
 * so it lives in this component), unlinking, and the linked server's channels. Linking happens in
 * Discord; the settings topic then brings the linked server here (§6.2).
 */
function useDiscordLink(settings: Settings) {
	const { campaignId } = useCampaign();
	const queryClient = useQueryClient();
	const [linkCode, setLinkCode] = useState<LinkCode | null>(null);
	const [now, setNow] = useState(() => Date.now());
	const isLinked = settings.discordServer !== null;
	const channels = useQuery({ ...discordChannelsQuery(campaignId), enabled: isLinked });

	useEffect(() => {
		if (linkCode === null) {
			return;
		}
		const timer = setInterval(() => {
			setNow(Date.now());
		}, linkCodeCheckInMilliseconds);
		return () => {
			clearInterval(timer);
		};
	}, [linkCode]);

	const getCode = api.useMutation("post", "/api/campaigns/{campaignId}/discord/link-code", {
		onError: (error) => {
			toast.error(problemMessage(error, "A link code couldn't be made. Try again."));
		},
		onSuccess: (code) => {
			setNow(Date.now());
			setLinkCode(code);
		},
	});
	const unlink = api.useMutation("post", "/api/campaigns/{campaignId}/discord/unlink", {
		onError: (error) => {
			toast.error(problemMessage(error, "The server couldn't be unlinked."));
		},
		onSuccess: () => {
			setLinkCode(null);
			queryClient.invalidateQueries({ queryKey: settingsQuery(campaignId).queryKey });
			queryClient.invalidateQueries({ queryKey: discordChannelsQuery(campaignId).queryKey });
			toast.success("Unlinked. Beny Bot no longer posts for this campaign.");
		},
	});

	let channelsDisabledReason: string | undefined;
	if (!isLinked) {
		channelsDisabledReason = "Link a Discord server first.";
	} else if (channels.isSuccess && channels.data.length === 0) {
		channelsDisabledReason =
			"Beny Bot can't list this server's channels right now. Is it running, and still in the server?";
	}

	return {
		channels: channels.data ?? [],
		channelsDisabledReason,
		getCode: () => {
			getCode.mutate({ params: { path: { campaignId } } });
		},
		isGettingCode: getCode.isPending,
		isUnlinking: unlink.isPending,
		linkCode: isLinked ? null : linkCode,
		linkCodeExpired: linkCode !== null && Date.parse(linkCode.expiresAt) <= now,
		unlink: () => {
			unlink.mutate({ params: { path: { campaignId } } });
		},
	};
}

function SaveBar({
	isDirty,
	isSaving,
	changedElsewhere,
	onDiscard,
}: {
	isDirty: boolean;
	isSaving: boolean;
	changedElsewhere: boolean;
	onDiscard: () => void;
}) {
	let message = "All changes saved.";
	if (changedElsewhere) {
		message = "Changed on another device too. Saving replaces those changes.";
	} else if (isDirty) {
		message = "You have unsaved changes.";
	}
	return (
		<div
			className={cn(
				"z-30 flex flex-col gap-3 rounded-2xl border px-4 py-3 transition-colors sm:flex-row sm:items-center",
				isDirty
					? "sticky bottom-[calc(4.75rem+env(safe-area-inset-bottom))] border-gilt/50 bg-popover/95 shadow-float backdrop-blur-md md:bottom-6"
					: "border-border bg-card/60",
			)}
		>
			<p
				aria-live="polite"
				className={cn("flex-1 text-sm", isDirty ? "text-foreground" : "text-muted-foreground")}
			>
				{message}
			</p>
			<div className="flex gap-2">
				<Button
					className="flex-1 sm:flex-none"
					disabled={!isDirty || isSaving}
					onClick={onDiscard}
					type="button"
					variant="ghost"
				>
					<RotateCcwIcon />
					Discard
				</Button>
				<Button className="flex-1 sm:flex-none" disabled={!isDirty || isSaving} type="submit">
					<SaveIcon />
					{isSaving ? "Saving…" : "Save settings"}
				</Button>
			</div>
		</div>
	);
}
