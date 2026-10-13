import { useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import { topicQueryKeys } from "@/api/topics";
import { BottomSheet } from "@/components/bottom-sheet";
import { Field, fieldInputProps } from "@/components/field";
import { NativeSelect, SwitchRow, TextArea } from "@/components/form-controls";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useCampaignId } from "@/hooks/use-campaign";
import { focusFirstVisible } from "@/lib/focus";
import { forgetFailedSaves } from "@/lib/forms";
import {
	type Cadence,
	cadenceLabels,
	maximumDurationMinutes,
	minimumDurationMinutes,
	type SeriesDraft,
	type SeriesDraftErrors,
	seriesDescriptionMaxLength,
	seriesDraft,
	seriesDraftErrors,
	seriesInput,
	seriesTitleMaxLength,
	timezoneCity,
	todayLocalDate,
	weekdayNames,
	weekdayOf,
} from "@/lib/sessions";
import { cn } from "@/lib/utils";

type SessionSeries = Schemas["SessionSeries"];

const cadences: Cadence[] = ["weekly", "fortnightly", "once"];
// Where a failed Save sends focus: the first field, in form order, that still needs something.
const fieldOrder: [keyof SeriesDraftErrors, string][] = [
	["title", "series-title"],
	["weekdays", "series-weekday-1"],
	["firstDate", "series-first-date"],
	["lastDate", "series-last-date"],
	["startLocal", "series-start"],
	["durationMinutes", "series-duration"],
];
// Monday first reads like a calendar week; the API still numbers Sunday 0.
const weekdayOrder = [1, 2, 3, 4, 5, 6, 0];

/** Every query a series change can move: dates, the board's next session, the ICS-bound list. */
export function useSessionsSettled() {
	const queryClient = useQueryClient();
	const campaignId = useCampaignId();
	return () =>
		Promise.all(
			topicQueryKeys(campaignId).sessions.map((queryKey) =>
				queryClient.invalidateQueries({ queryKey }),
			),
		);
}

/**
 * Create or fully edit a series or a one-off session (spec §4.5, §5.11). Times are the table's wall
 * clock in the campaign's time zone, which the sheet names, so the DM never types their own by mistake.
 */
export function SeriesSheet({
	open,
	onOpenChange,
	series,
	tableTimeZone,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	/** The series to edit, or null to add one. */
	series: SessionSeries | null;
	tableTimeZone: string | undefined;
}) {
	const campaignId = useCampaignId();
	const settle = useSessionsSettled();
	const [draft, setDraft] = useState(() => seriesDraft(series, todayLocalDate()));
	const [triedToSave, setTriedToSave] = useState(false);
	const create = api.useMutation("post", "/api/campaigns/{campaignId}/session-series", {
		onSettled: settle,
	});
	const update = api.useMutation("put", "/api/campaigns/{campaignId}/session-series/{seriesId}", {
		onSettled: settle,
	});
	const error = create.error ?? update.error;
	const isSaving = create.isPending || update.isPending;
	const serverErrors = fieldErrors(error);
	const draftErrors = triedToSave ? seriesDraftErrors(draft) : {};
	const errors: Record<string, string | undefined> = { ...draftErrors, ...serverErrors };
	const showGeneralError = error !== null && Object.keys(serverErrors).length === 0;
	const isOnce = draft.cadence === "once";
	const zoneName = tableTimeZone
		? `${timezoneCity(tableTimeZone)} time`
		: "the campaign's time zone";

	function set<Key extends keyof SeriesDraft>(key: Key, value: SeriesDraft[Key]) {
		setDraft((current) => ({ ...current, [key]: value }));
		forgetFailedSaves(create, update);
	}

	function toggleWeekday(weekday: number) {
		set(
			"weekdays",
			draft.weekdays.includes(weekday)
				? draft.weekdays.filter((day) => day !== weekday)
				: [...draft.weekdays, weekday],
		);
	}

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		setTriedToSave(true);
		const problems = seriesDraftErrors(draft);
		const invalidFieldIds = fieldOrder
			.filter(([field]) => problems[field] !== undefined)
			.map(([, elementId]) => elementId);
		if (invalidFieldIds.length > 0) {
			focusFirstVisible(...invalidFieldIds);
			return;
		}
		if (isSaving) {
			return;
		}
		const body = seriesInput(draft);
		const onSuccess = (saved: SessionSeries) => {
			toast.success(series ? `${saved.title} saved.` : `${saved.title} added.`);
			onOpenChange(false);
		};
		if (series) {
			update.mutate({ body, params: { path: { campaignId, seriesId: series.id } } }, { onSuccess });
			return;
		}
		create.mutate({ body, params: { path: { campaignId } } }, { onSuccess });
	}

	let saveLabel = series ? "Save" : "Add to the schedule";
	if (isSaving) {
		saveLabel = "Saving…";
	}
	const names = weekdayNames("short");
	const longNames = weekdayNames("long");

	return (
		<BottomSheet
			className="md:max-w-2xl"
			description={`Dates and times are the table's, in ${zoneName}. Everyone sees them in their own time too.`}
			onOpenChange={onOpenChange}
			open={open}
			title={series ? `Edit ${series.title}` : "Schedule game nights"}
		>
			<form className="grid gap-5" noValidate onSubmit={handleSubmit}>
				<Field error={errors.title} id="series-title" label="Title">
					<Input
						{...fieldInputProps("series-title", errors.title, false)}
						autoComplete="off"
						autoFocus={!series}
						maxLength={seriesTitleMaxLength}
						onChange={(event) => {
							set("title", event.target.value);
						}}
						placeholder="Friday game"
						value={draft.title}
					/>
				</Field>
				<fieldset className="grid gap-2">
					<legend className="mb-2 font-medium text-sm">How often</legend>
					<div className="grid grid-cols-3 gap-2">
						{cadences.map((cadence) => (
							<label
								className={cn(
									"flex min-h-11 cursor-pointer items-center justify-center rounded-md border px-2 text-center text-sm has-focus-visible:ring-[3px] has-focus-visible:ring-ring/40",
									draft.cadence === cadence
										? "border-gilt bg-gilt/12 font-semibold text-gilt-ink"
										: "border-input",
								)}
								key={cadence}
							>
								<input
									checked={draft.cadence === cadence}
									className="sr-only"
									name="series-cadence"
									onChange={() => {
										set("cadence", cadence);
									}}
									type="radio"
									value={cadence}
								/>
								{cadenceLabels[cadence]}
							</label>
						))}
					</div>
				</fieldset>
				{isOnce ? null : (
					<fieldset
						aria-describedby={errors.weekdays ? "series-weekdays-error" : undefined}
						className="grid gap-2"
					>
						<legend className="mb-2 font-medium text-sm">On</legend>
						<div className="grid grid-cols-7 gap-1">
							{weekdayOrder.map((weekday) => {
								const isChosen = draft.weekdays.includes(weekday);
								return (
									<label
										className={cn(
											"flex min-h-11 cursor-pointer items-center justify-center rounded-md border text-sm has-focus-visible:ring-[3px] has-focus-visible:ring-ring/40",
											isChosen
												? "border-gilt bg-gilt/12 font-semibold text-gilt-ink"
												: "border-input",
										)}
										key={weekday}
									>
										<input
											aria-label={longNames[weekday]}
											checked={isChosen}
											className="sr-only"
											id={`series-weekday-${weekday}`}
											onChange={() => {
												toggleWeekday(weekday);
											}}
											type="checkbox"
										/>
										<span aria-hidden="true">{names[weekday]}</span>
									</label>
								);
							})}
						</div>
						{errors.weekdays ? (
							<p className="text-ember-ink text-sm" id="series-weekdays-error">
								{errors.weekdays}
							</p>
						) : null}
					</fieldset>
				)}
				<div className="grid gap-5 sm:grid-cols-2">
					<Field
						description={
							isOnce ? undefined : "Every other week counts two weeks at a time from this date."
						}
						error={errors.firstDate}
						id="series-first-date"
						label={isOnce ? "Date" : "First date"}
					>
						<Input
							{...fieldInputProps("series-first-date", errors.firstDate, !isOnce)}
							onChange={(event) => {
								const firstDate = event.target.value;
								const weekday = weekdayOf(firstDate);
								setDraft((current) => ({
									...current,
									firstDate,
									// A one-weekday series follows its first date, the common case of moving the game night.
									weekdays:
										weekday !== null && current.weekdays.length <= 1 ? [weekday] : current.weekdays,
								}));
								forgetFailedSaves(create, update);
							}}
							type="date"
							value={draft.firstDate}
						/>
					</Field>
					{isOnce ? null : (
						<Field
							description="Leave it empty to keep going."
							error={errors.lastDate}
							id="series-last-date"
							label="Last date"
							optional
						>
							<Input
								{...fieldInputProps("series-last-date", errors.lastDate, true)}
								min={draft.firstDate}
								onChange={(event) => {
									set("lastDate", event.target.value);
								}}
								type="date"
								value={draft.lastDate}
							/>
						</Field>
					)}
					<Field
						description={`In ${zoneName}.`}
						error={errors.startLocal}
						id="series-start"
						label="Starts at"
					>
						<Input
							{...fieldInputProps("series-start", errors.startLocal, true)}
							onChange={(event) => {
								set("startLocal", event.target.value);
							}}
							step={300}
							type="time"
							value={draft.startLocal}
						/>
					</Field>
					<Field
						description="From 15 minutes to 8 hours."
						error={errors.durationMinutes}
						id="series-duration"
						label="Length"
					>
						<NativeSelect
							{...fieldInputProps("series-duration", errors.durationMinutes, true)}
							onChange={(event) => {
								set("durationMinutes", event.target.value);
							}}
							value={draft.durationMinutes}
						>
							{durationChoices(draft.durationMinutes).map((minutes) => (
								<option key={minutes} value={String(minutes)}>
									{durationChoiceLabel(minutes)}
								</option>
							))}
						</NativeSelect>
					</Field>
				</div>
				<Field error={errors.description} id="series-description" label="Description" optional>
					<TextArea
						{...fieldInputProps("series-description", errors.description, false)}
						maxLength={seriesDescriptionMaxLength}
						onChange={(event) => {
							set("description", event.target.value);
						}}
						value={draft.description}
					/>
				</Field>
				<SwitchRow
					checked={draft.announce}
					description={
						draft.announce
							? "Beny Bot keeps a Discord event on the next date."
							: "No Discord event for these dates."
					}
					id="series-announce"
					label="Discord event"
					onCheckedChange={(checked) => {
						set("announce", checked);
					}}
				/>
				<SwitchRow
					checked={draft.remind}
					description={
						draft.remind
							? "Beny Bot posts a reminder about a day before."
							: "No reminder before these dates."
					}
					id="series-remind"
					label="Reminder"
					onCheckedChange={(checked) => {
						set("remind", checked);
					}}
				/>
				{showGeneralError ? (
					<p className="text-ember-ink text-sm" role="alert">
						{problemMessage(error, "The schedule couldn't be saved.")}
					</p>
				) : null}
				<div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
					<Button
						onClick={() => {
							onOpenChange(false);
						}}
						type="button"
						variant="ghost"
					>
						Cancel
					</Button>
					<Button disabled={isSaving} type="submit">
						{saveLabel}
					</Button>
				</div>
			</form>
		</BottomSheet>
	);
}

const standardDurations = [60, 90, 120, 150, 180, 210, 240, 270, 300, 330, 360, 420, 480];

/** Half-hour steps, plus whatever the series already has, so an odd length isn't lost on save. */
function durationChoices(current: string): number[] {
	const minutes = Number(current);
	const choices = new Set(standardDurations);
	if (
		Number.isInteger(minutes) &&
		minutes >= minimumDurationMinutes &&
		minutes <= maximumDurationMinutes
	) {
		choices.add(minutes);
	}
	return [...choices].sort((first, second) => first - second);
}

function durationChoiceLabel(minutes: number): string {
	const hours = Math.floor(minutes / 60);
	const rest = minutes % 60;
	if (hours === 0) {
		return `${rest} minutes`;
	}
	const hoursText = hours === 1 ? "1 hour" : `${hours} hours`;
	return rest === 0 ? hoursText : `${hoursText} ${rest} minutes`;
}
