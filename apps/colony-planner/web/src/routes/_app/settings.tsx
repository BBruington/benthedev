import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import {
	BellRingIcon,
	BotIcon,
	CalendarClockIcon,
	ExternalLinkIcon,
	type LucideIcon,
	MegaphoneIcon,
	RotateCcwIcon,
	SaveIcon,
	ScrollIcon,
	Settings2Icon,
	ShieldIcon,
} from "lucide-react";
import { type FormEvent, type ReactNode, useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import { resourcesQuery, settingsQuery } from "@/api/queries";
import { ResourceSettingsCard } from "@/colony/resource-settings";
import { Field, fieldInputProps, fieldMessageIds } from "@/components/field";
import { PageHeader } from "@/components/page-header";
import { Forbidden } from "@/components/shell/forbidden";
import { StatusPanel } from "@/components/status-panel";
import { TimezonePicker } from "@/components/timezone-picker";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { useMe } from "@/hooks/use-me";
import { cachedMe, canUseDmTools } from "@/lib/access";
import { cn } from "@/lib/utils";

type Settings = Schemas["Settings"];
type SettingsInput = Schemas["SettingsInput"];

// Inputs hold strings; the API takes null for an unset channel.
type SettingsDraft = {
	campaignName: string;
	colonyName: string;
	timezone: string;
	discordAnnounceChannelId: string;
	discordReminderChannelId: string;
	discordDmChannelId: string;
};

type ChannelField = "discordAnnounceChannelId" | "discordReminderChannelId" | "discordDmChannelId";

export const Route = createFileRoute("/_app/settings")({
	component: SettingsRoute,
	loader: ({ context }) => {
		const me = cachedMe(context.queryClient);
		if (me && canUseDmTools(me)) {
			context.queryClient.prefetchQuery(settingsQuery);
			context.queryClient.prefetchQuery(resourcesQuery);
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
	const settings = useQuery(settingsQuery);
	useDocumentTitle("Settings", settings.data?.colonyName);

	return (
		<div className="flex flex-col gap-8 md:gap-10">
			<PageHeader
				description="The campaign's and colony's names, the time zone, the colony's resources, and the Discord channels Beny Bot posts in."
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
			<ResourceSettingsCard />
			<ComingLater />
		</>
	);
}

function toDraft(settings: Settings): SettingsDraft {
	return {
		campaignName: settings.campaignName,
		colonyName: settings.colonyName,
		discordAnnounceChannelId: settings.discordAnnounceChannelId ?? "",
		discordDmChannelId: settings.discordDmChannelId ?? "",
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
		discordAnnounceChannelId: optionalText(draft.discordAnnounceChannelId),
		discordDmChannelId: optionalText(draft.discordDmChannelId),
		discordReminderChannelId: optionalText(draft.discordReminderChannelId),
		timezone: draft.timezone,
	};
}

function sameSettings(first: SettingsDraft, second: SettingsDraft): boolean {
	return JSON.stringify(toInput(first)) === JSON.stringify(toInput(second));
}

function SettingsForm({ settings }: { settings: Settings }) {
	const queryClient = useQueryClient();
	const [draft, setDraft] = useState(() => toDraft(settings));
	const [baseline, setBaseline] = useState(settings);
	const [changedElsewhere, setChangedElsewhere] = useState(false);

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

	const mutation = api.useMutation("put", "/api/settings", {
		onError: (error) => {
			if (Object.keys(fieldErrors(error)).length === 0) {
				toast.error(problemMessage(error, "The settings couldn't be saved."));
			}
		},
		onSuccess: (saved) => {
			setDraft(toDraft(saved));
			setChangedElsewhere(false);
			queryClient.setQueryData(settingsQuery.queryKey, saved);
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
		mutation.mutate({ body: toInput(draft) });
	}

	function discard() {
		setDraft(toDraft(settings));
		setChangedElsewhere(false);
		mutation.reset();
	}

	return (
		<form className="flex flex-col gap-6" noValidate onSubmit={handleSubmit}>
			<SettingsCard
				description="How the campaign appears across the site, and the clock its sessions keep."
				icon={ScrollIcon}
				title="Campaign"
			>
				<Field error={errors.campaignName} id="settings-campaign-name" label="Campaign name">
					<Input
						{...fieldInputProps("settings-campaign-name", errors.campaignName, false)}
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
				description={
					<>
						Paste channel IDs from your Discord server: turn on Developer Mode (User Settings ›
						Advanced), then right-click a channel and choose <em>Copy Channel ID</em>. Leave one
						empty and Beny Bot stays quiet there.
					</>
				}
				icon={BellRingIcon}
				id="discord"
				title="Discord"
			>
				<BenyBotInvite inviteUrl={settings.botInviteUrl} />
				<ChannelInput
					description="Game-night events and notices for everyone."
					error={errors.discordAnnounceChannelId}
					field="discordAnnounceChannelId"
					icon={MegaphoneIcon}
					label="Announcements channel"
					onChange={update}
					value={draft.discordAnnounceChannelId}
				/>
				<ChannelInput
					description="A reminder about a day before each game."
					error={errors.discordReminderChannelId}
					field="discordReminderChannelId"
					icon={CalendarClockIcon}
					label="Reminders channel"
					onChange={update}
					value={draft.discordReminderChannelId}
				/>
				<ChannelInput
					description="Join requests and patron requests, for your eyes only. Keep this channel private."
					error={errors.discordDmChannelId}
					field="discordDmChannelId"
					icon={ShieldIcon}
					label="DM channel"
					onChange={update}
					value={draft.discordDmChannelId}
				/>
			</SettingsCard>

			<SaveBar
				changedElsewhere={changedElsewhere}
				isDirty={isDirty}
				isSaving={mutation.isPending}
				onDiscard={discard}
			/>
		</form>
	);
}

function SettingsCard({
	id,
	icon: Icon,
	title,
	description,
	children,
}: {
	id?: string;
	icon: LucideIcon;
	title: string;
	description: ReactNode;
	children: ReactNode;
}) {
	const headingId = `${id ?? title.toLowerCase()}-heading`;
	return (
		<section
			aria-labelledby={headingId}
			className="scroll-mt-28 rounded-2xl border border-border bg-card shadow-card"
			id={id}
		>
			<div className="flex items-start gap-4 border-border border-b px-5 py-5 md:px-7">
				<span
					aria-hidden="true"
					className="inline-flex size-10 shrink-0 items-center justify-center rounded-lg bg-gilt/12 text-gilt-ink"
				>
					<Icon className="size-5" />
				</span>
				<div className="min-w-0">
					<h2 className="font-display font-semibold text-xl tracking-wide" id={headingId}>
						{title}
					</h2>
					<p className="mt-1 max-w-prose text-muted-foreground text-sm">{description}</p>
				</div>
			</div>
			<div className="grid gap-6 px-5 py-6 md:px-7">{children}</div>
		</section>
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
			<div className="min-w-0">
				<p className="font-medium">Invite Beny Bot</p>
				<p className="mt-0.5 text-muted-foreground text-sm">
					Adds Beny Bot to your Discord server with the permissions it needs. Someone who manages
					the server has to accept.
				</p>
			</div>
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

function ChannelInput({
	field,
	label,
	description,
	icon: Icon,
	value,
	error,
	onChange,
}: {
	field: ChannelField;
	label: string;
	description: string;
	icon: LucideIcon;
	value: string;
	error: string | undefined;
	onChange: (field: ChannelField, value: string) => void;
}) {
	const id = `settings-${field}`;
	return (
		<Field
			description={description}
			error={error}
			id={id}
			label={
				<>
					<Icon aria-hidden="true" className="size-4 text-muted-foreground" />
					{label}
				</>
			}
			optional
		>
			<Input
				{...fieldInputProps(id, error, true)}
				autoComplete="off"
				className="font-mono tracking-wide placeholder:font-sans placeholder:tracking-normal"
				inputMode="numeric"
				onChange={(event) => {
					onChange(field, event.target.value);
				}}
				placeholder="Not set"
				spellCheck={false}
				value={value}
			/>
		</Field>
	);
}

const laterSettings: { label: string; arrivesIn: string }[] = [
	{ arrivesIn: "M2", label: "Colonist roles" },
	{ arrivesIn: "M5", label: "Discord event location and the calendar (ICS) link" },
];

function ComingLater() {
	return (
		<section
			aria-labelledby="settings-later-heading"
			className="rounded-2xl border border-border border-dashed px-5 py-5 md:px-7"
		>
			<h2
				className="font-medium text-muted-foreground text-xs uppercase tracking-[0.18em]"
				id="settings-later-heading"
			>
				Arriving with later milestones
			</h2>
			<ul className="mt-3 grid gap-2 lg:grid-cols-2">
				{laterSettings.map((setting) => (
					<li className="flex items-center gap-2 text-muted-foreground" key={setting.label}>
						<Badge variant="muted">{setting.arrivesIn}</Badge>
						<span className="text-sm">{setting.label}</span>
					</li>
				))}
			</ul>
		</section>
	);
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
