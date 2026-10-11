import { CheckIcon, ChevronsUpDownIcon, GlobeIcon, LocateFixedIcon } from "lucide-react";
import { useMemo, useState } from "react";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
	CommandSeparator,
} from "@/components/ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import {
	describeTimezone,
	deviceTimezone,
	type TimezoneOption,
	timezoneOptions,
	timezoneSearchScore,
} from "@/lib/timezones";
import { cn } from "@/lib/utils";

function groupByRegion(options: TimezoneOption[]): [string, TimezoneOption[]][] {
	const groups = new Map<string, TimezoneOption[]>();
	for (const option of options) {
		const group = groups.get(option.region);
		if (group) {
			group.push(option);
		} else {
			groups.set(option.region, [option]);
		}
	}
	return [...groups.entries()];
}

/** A searchable list of every IANA time zone, with each zone's current UTC offset. */
export function TimezonePicker({
	id,
	value,
	onChange,
	invalid = false,
	describedBy,
}: {
	id: string;
	value: string;
	onChange: (timezone: string) => void;
	invalid?: boolean;
	describedBy?: string;
}) {
	const [open, setOpen] = useState(false);
	// The list is built on first open: hundreds of zones, each with its offset worked out.
	const groups = useMemo(() => (open ? groupByRegion(timezoneOptions()) : []), [open]);
	const selected = value ? describeTimezone(value) : undefined;
	const device = deviceTimezone();
	const deviceOption = device && device !== value ? describeTimezone(device) : undefined;

	function choose(timezone: string) {
		onChange(timezone);
		setOpen(false);
	}

	return (
		<Popover onOpenChange={setOpen} open={open}>
			<PopoverTrigger asChild>
				<button
					aria-describedby={describedBy}
					aria-expanded={open}
					aria-invalid={invalid ? true : undefined}
					className={cn(
						"flex h-11 w-full min-w-0 items-center gap-3 rounded-md border border-input bg-surface-raised/70 px-3 text-left text-base outline-none transition-[border-color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/35 aria-invalid:border-ember-ink aria-invalid:ring-[3px] aria-invalid:ring-destructive/20 dark:bg-surface-sunken/60",
					)}
					id={id}
					role="combobox"
					type="button"
				>
					<GlobeIcon aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
					{selected ? (
						<span className="flex min-w-0 flex-1 items-baseline gap-2">
							<span className="truncate">{selected.city}</span>
							<span className="truncate text-muted-foreground text-sm">
								{selected.region} · {selected.offset}
							</span>
						</span>
					) : (
						<span className="flex-1 text-muted-foreground">Choose a time zone</span>
					)}
					<ChevronsUpDownIcon
						aria-hidden="true"
						className="size-4 shrink-0 text-muted-foreground"
					/>
				</button>
			</PopoverTrigger>
			<PopoverContent align="start" className="w-(--radix-popover-trigger-width) min-w-[18rem] p-0">
				<Command filter={timezoneSearchScore}>
					<CommandInput placeholder="Search a city, region or offset" />
					<CommandList>
						<CommandEmpty>No time zone matches that.</CommandEmpty>
						{deviceOption ? (
							<>
								<CommandGroup heading="This device">
									<CommandItem
										keywords={[deviceOption.city, deviceOption.region, "device", "local"]}
										onSelect={() => {
											choose(deviceOption.name);
										}}
										value={`device ${deviceOption.name}`}
									>
										<LocateFixedIcon />
										<span className="flex-1 truncate">{deviceOption.city}</span>
										<span className="text-muted-foreground text-sm">{deviceOption.offset}</span>
									</CommandItem>
								</CommandGroup>
								<CommandSeparator />
							</>
						) : null}
						{groups.map(([region, zones]) => (
							<CommandGroup heading={region} key={region}>
								{zones.map((zone) => (
									<CommandItem
										key={zone.name}
										keywords={[zone.city, zone.region, zone.offset]}
										onSelect={() => {
											choose(zone.name);
										}}
										value={zone.name}
									>
										<span className="flex-1 truncate">{zone.city}</span>
										<span className="text-muted-foreground text-sm tabular-nums">
											{zone.offset}
										</span>
										<CheckIcon
											className={cn(
												"size-4 text-gilt-ink",
												zone.name === value ? "opacity-100" : "opacity-0",
											)}
										/>
									</CommandItem>
								))}
							</CommandGroup>
						))}
					</CommandList>
				</Command>
			</PopoverContent>
		</Popover>
	);
}
