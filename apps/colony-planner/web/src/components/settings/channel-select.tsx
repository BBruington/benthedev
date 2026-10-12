import type { LucideIcon } from "lucide-react";
import type { Schemas } from "@/api/client";
import { Field, fieldInputProps } from "@/components/field";
import { cn } from "@/lib/utils";

type DiscordChannel = Schemas["DiscordChannel"];

/**
 * A channel of the linked server, picked from Beny Bot's list rather than typed as an ID
 * (docs/design/multi-campaign.md §6.4). A saved channel that is no longer listed stays selectable,
 * so opening Settings never changes it silently.
 */
export function ChannelSelect({
	id,
	label,
	description,
	icon: Icon,
	value,
	error,
	channels,
	disabledReason,
	onChange,
}: {
	id: string;
	label: string;
	description: string;
	icon: LucideIcon;
	value: string;
	error: string | undefined;
	channels: readonly DiscordChannel[];
	/** Why the picker can't be used yet; it is disabled and says so instead of the description. */
	disabledReason: string | undefined;
	onChange: (value: string) => void;
}) {
	const isListed = value === "" || channels.some((channel) => channel.id === value);
	return (
		<Field
			description={disabledReason ?? description}
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
			<select
				{...fieldInputProps(id, error, true)}
				className={cn(
					"h-11 w-full min-w-0 rounded-md border border-input bg-surface-raised/70 px-3 text-base text-foreground outline-none transition-[color,border-color,box-shadow] disabled:cursor-not-allowed disabled:opacity-50 dark:bg-surface-sunken/60",
					"focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/35",
					"aria-invalid:border-ember-ink aria-invalid:ring-[3px] aria-invalid:ring-destructive/20",
				)}
				disabled={disabledReason !== undefined}
				onChange={(event) => {
					onChange(event.target.value);
				}}
				value={value}
			>
				<option value="">Not set: Beny Bot stays quiet</option>
				{channels.map((channel) => (
					<option key={channel.id} value={channel.id}>
						#{channel.name}
					</option>
				))}
				{isListed ? null : <option value={value}>A channel Beny Bot can't see ({value})</option>}
			</select>
		</Field>
	);
}
