import { keepPreviousData, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { PlusIcon, ScrollTextIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { problemMessage } from "@/api/problem";
import {
	colonyBoardQuery,
	recentTimelineQuery,
	timelineQuery,
	timelineQueryKeyPrefix,
} from "@/api/queries";
import { AdjustSheet, type CargoArrival } from "@/colony/adjust-sheet";
import { markAnnounced } from "@/colony/live-toasts";
import { type EntryEditing, TimelineEntrySheet } from "@/colony/timeline-entry-sheet";
import {
	dayHeadingId,
	type EntryActions,
	filterEntries,
	TimelineDays,
	TimelineFilters,
	TimelineToolbar,
	timelinePageHeadingId,
} from "@/colony/timeline-parts";
import { useColonyChanges } from "@/colony/use-colony-changes";
import { PageHeader } from "@/components/page-header";
import { StatusPanel } from "@/components/status-panel";
import type { TimelineKind } from "@/components/timeline-entry";
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
import { Skeleton } from "@/components/ui/skeleton";
import { fallbackCampaignName, useCampaignId } from "@/hooks/use-campaign";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { useMe } from "@/hooks/use-me";
import { useSheetState } from "@/hooks/use-sheet-state";
import { canUseDmTools } from "@/lib/access";
import { type DayPage, dayName, nextPage, pageForDay, previousPage } from "@/lib/colony";
import { focusFirstVisible } from "@/lib/focus";

type Entry = Schemas["TimelineEntry"];
type TimelinePage = Schemas["TimelinePage"];

type TimelineSearch = { day?: number };

export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell/timeline")({
	component: TimelineRoute,
	loader: ({ context: { campaignId, queryClient } }) => {
		if (campaignId) {
			queryClient.prefetchQuery(colonyBoardQuery(campaignId));
		}
	},
	validateSearch: (search: Record<string, unknown>): TimelineSearch => {
		const day = Number(search.day);
		if (search.day === undefined || !Number.isSafeInteger(day) || day < 0) {
			return {};
		}
		return { day };
	},
});

function TimelineRoute() {
	const board = useQuery(colonyBoardQuery(useCampaignId()));
	const { day } = Route.useSearch();
	useDocumentTitle("Timeline", board.data?.colonyName ?? fallbackCampaignName);

	if (board.isError) {
		return (
			<StatusPanel
				description={problemMessage(board.error, "The timeline couldn't be loaded.")}
				icon={ScrollTextIcon}
				title="The timeline couldn't be loaded"
				tone="ember"
			/>
		);
	}
	if (!board.data) {
		return <TimelineSkeleton />;
	}
	return <TimelinePageView board={board.data} page={pageForDay(day ?? board.data.day)} />;
}

function TimelineSkeleton() {
	return (
		<div aria-busy="true" className="flex flex-col gap-4">
			<Skeleton className="h-16 w-1/2" />
			<Skeleton className="h-16 rounded-xl" />
			<Skeleton className="h-24 rounded-xl" />
			<Skeleton className="h-24 rounded-xl" />
		</div>
	);
}

/** Where Earlier goes: the nearest page with an entry, skipping empty stretches, else the page before. */
function earlierPage(page: DayPage, data: TimelinePage | undefined): DayPage | null {
	if (data && data.earlierDay !== null) {
		return pageForDay(data.earlierDay);
	}
	return previousPage(page);
}

/** Where Later goes: the nearest page with an entry, else today's page, else (for the DM) the next. */
function laterPage(
	page: DayPage,
	data: TimelinePage | undefined,
	today: number,
	isDm: boolean,
): DayPage | null {
	if (data && data.laterDay !== null) {
		return pageForDay(data.laterDay);
	}
	if (today > page.toDay) {
		return pageForDay(today);
	}
	if (isDm) {
		return nextPage(page);
	}
	return null;
}

function TimelinePageView({ board, page }: { board: Schemas["ColonyBoard"]; page: DayPage }) {
	const me = useMe();
	const isDm = canUseDmTools(me);
	const navigate = useNavigate({ from: Route.fullPath });
	const timeline = useQuery({
		...timelineQuery(useCampaignId(), page.fromDay, page.toDay),
		placeholderData: keepPreviousData,
	});
	const [shownKinds, setShownKinds] = useState<ReadonlySet<TimelineKind>>(new Set());
	const [hideDmOnly, setHideDmOnly] = useState(false);
	const today = timeline.data?.today ?? board.day;
	const isShowingPage = timeline.data?.fromDay === page.fromDay;

	function go(day: number | undefined) {
		navigate({ search: day === undefined ? {} : { day } }).then(() => {
			focusFirstVisible(timelinePageHeadingId);
		});
	}

	return (
		<div className="flex flex-col gap-6">
			<PageHeader
				actions={isDm ? <AddEntryButton day={today} /> : null}
				description={
					isDm
						? "Every entry, yours included. Entries hidden from players carry an eye-slash; undone ones are struck through."
						: "Everything that has happened to the colony, a week to a page."
				}
				eyebrow={board.colonyName}
				title="Timeline"
			/>
			<TimelineToolbar
				earlier={earlierPage(page, timeline.data)}
				isOnToday={page.fromDay === pageForDay(today).fromDay}
				later={laterPage(page, timeline.data, today, isDm)}
				onGo={go}
				onToday={() => {
					go(undefined);
				}}
				page={page}
			/>
			{isDm ? (
				<TimelineFilters
					hideDmOnly={hideDmOnly}
					onHideDmOnlyChange={setHideDmOnly}
					onToggleKind={(kind) => {
						setShownKinds((current) => {
							const next = new Set(current);
							if (next.has(kind)) {
								next.delete(kind);
							} else {
								next.add(kind);
							}
							return next;
						});
					}}
					shownKinds={shownKinds}
				/>
			) : null}
			<TimelineBody
				board={board}
				entries={timeline.data && isShowingPage ? timeline.data.entries : undefined}
				error={timeline.error}
				filter={(entries) => filterEntries(entries, shownKinds, hideDmOnly)}
				isDm={isDm}
				page={page}
				today={today}
			/>
		</div>
	);
}

function TimelineBody({
	board,
	page,
	today,
	entries,
	error,
	isDm,
	filter,
}: {
	board: Schemas["ColonyBoard"];
	page: DayPage;
	today: number;
	entries: readonly Entry[] | undefined;
	error: unknown;
	isDm: boolean;
	filter: (entries: readonly Entry[]) => Entry[];
}) {
	if (error) {
		return (
			<p className="rounded-xl border border-ember/40 bg-ember/10 px-4 py-3 text-ember-ink">
				{problemMessage(error, "These days couldn't be loaded.")}
			</p>
		);
	}
	if (!entries) {
		return (
			<div aria-busy="true" className="flex flex-col gap-3">
				<Skeleton className="h-24 rounded-xl" />
				<Skeleton className="h-12 rounded-xl" />
				<Skeleton className="h-24 rounded-xl" />
			</div>
		);
	}
	if (isDm) {
		return <DmTimelineDays board={board} entries={filter(entries)} page={page} today={today} />;
	}
	return <TimelineDays actions={null} entries={entries} page={page} today={today} />;
}

/** The page's days with the DM's entry controls and the sheets they open. */
function DmTimelineDays({
	board,
	page,
	today,
	entries,
}: {
	board: Schemas["ColonyBoard"];
	page: DayPage;
	today: number;
	entries: readonly Entry[];
}) {
	const entryMutations = useTimelineEntryMutations();
	const changes = useColonyChanges();
	const editing = useSheetState<EntryEditing>();
	const cargo = useSheetState<CargoArrival>();
	const [deleting, setDeleting] = useState<Entry | null>(null);

	const actions: EntryActions = {
		onAdd: (day) => {
			editing.show({ day, kind: "new" });
		},
		onDelete: setDeleting,
		onEdit: (entry) => {
			editing.show({ entry, kind: "edit" });
		},
		onRecordCargo: (entry) => {
			cargo.show({ arrivalEntryId: entry.id, day: entry.day, title: entry.title });
		},
	};

	return (
		<>
			<TimelineDays actions={actions} entries={entries} page={page} today={today} />
			{editing.value ? (
				<TimelineEntrySheet
					editing={editing.value}
					error={entryMutations.saveError}
					isSaving={entryMutations.isSaving}
					key={editing.session}
					onOpenChange={(open) => {
						editing.onOpenChange(open);
						if (!open) {
							entryMutations.reset();
						}
					}}
					onSave={(input) => {
						const current = editing.value;
						if (!current) {
							return;
						}
						entryMutations.save(current, input, () => {
							editing.onOpenChange(false);
						});
					}}
					open={editing.open}
				/>
			) : null}
			{cargo.value ? (
				<AdjustSheet
					cargo={cargo.value}
					changes={changes}
					key={cargo.session}
					onOpenChange={cargo.onOpenChange}
					open={cargo.open}
					resources={board.resources}
					version={board.version}
				/>
			) : null}
			<DeleteEntryDialog
				entry={deleting}
				onConfirm={(entry) => {
					setDeleting(null);
					entryMutations.remove(entry);
				}}
				onOpenChange={(open) => {
					if (!open) {
						setDeleting(null);
					}
				}}
			/>
		</>
	);
}

function AddEntryButton({ day }: { day: number }) {
	const entryMutations = useTimelineEntryMutations();
	const editing = useSheetState<EntryEditing>();
	return (
		<>
			<Button
				onClick={() => {
					editing.show({ day, kind: "new" });
				}}
			>
				<PlusIcon />
				Add an entry
			</Button>
			{editing.value ? (
				<TimelineEntrySheet
					editing={editing.value}
					error={entryMutations.saveError}
					isSaving={entryMutations.isSaving}
					key={editing.session}
					onOpenChange={(open) => {
						editing.onOpenChange(open);
						if (!open) {
							entryMutations.reset();
						}
					}}
					onSave={(input) => {
						const current = editing.value;
						if (current) {
							entryMutations.save(current, input, () => {
								editing.onOpenChange(false);
							});
						}
					}}
					open={editing.open}
				/>
			) : null}
		</>
	);
}

/** Create, edit and delete for timeline entries; each refreshes the timeline when it lands. */
function useTimelineEntryMutations() {
	const queryClient = useQueryClient();
	const campaignId = useCampaignId();
	function refresh() {
		queryClient.invalidateQueries({ queryKey: timelineQueryKeyPrefix(campaignId) });
		queryClient.invalidateQueries({ queryKey: recentTimelineQuery(campaignId).queryKey });
	}
	const create = api.useMutation("post", "/api/campaigns/{campaignId}/timeline", {
		// The sheet's own "Added to Day N" toast announces it; the live toast must not repeat it.
		onSettled: refresh,
		onSuccess: (entry) => {
			markAnnounced([entry.id]);
		},
	});
	const update = api.useMutation("put", "/api/campaigns/{campaignId}/timeline/{timelineEntryId}", {
		onSettled: refresh,
	});
	const remove = api.useMutation(
		"delete",
		"/api/campaigns/{campaignId}/timeline/{timelineEntryId}",
		{ onSettled: refresh },
	);

	return {
		isSaving: create.isPending || update.isPending,
		remove: (entry: Entry) => {
			remove.mutate(
				{ params: { path: { campaignId, timelineEntryId: entry.id } } },
				{
					onError: (error) => {
						toast.error(problemMessage(error, "The entry couldn't be deleted."));
					},
					onSuccess: () => {
						toast.success(`“${entry.title}” was deleted.`);
						// The entry and its menu are gone; its day's heading is the stable place to land.
						focusFirstVisible(dayHeadingId(entry.day), timelinePageHeadingId);
					},
				},
			);
		},
		reset: () => {
			create.reset();
			update.reset();
		},
		save: (editing: EntryEditing, input: Schemas["TimelineEntryInput"], onDone: () => void) => {
			if (editing.kind === "new") {
				create.mutate(
					{ body: input, params: { path: { campaignId } } },
					{
						onSuccess: (entry) => {
							toast.success(`Added to ${dayName(entry.day)}.`);
							onDone();
						},
					},
				);
				return;
			}
			update.mutate(
				{ body: input, params: { path: { campaignId, timelineEntryId: editing.entry.id } } },
				{
					onSuccess: () => {
						toast.success("Entry saved.");
						onDone();
					},
				},
			);
		},
		saveError: create.error ?? update.error,
	};
}

function DeleteEntryDialog({
	entry,
	onOpenChange,
	onConfirm,
}: {
	entry: Entry | null;
	onOpenChange: (open: boolean) => void;
	onConfirm: (entry: Entry) => void;
}) {
	const [shown, setShown] = useState(entry);
	if (entry && entry !== shown) {
		setShown(entry);
	}
	const [confirmed, setConfirmed] = useState(false);
	return (
		<AlertDialog
			onOpenChange={(open) => {
				if (open) {
					setConfirmed(false);
				}
				onOpenChange(open);
			}}
			open={entry !== null}
		>
			<AlertDialogContent
				onCloseAutoFocus={(event) => {
					// The menu that opened this dialog goes with the entry; the delete moves focus itself.
					if (confirmed) {
						event.preventDefault();
					}
				}}
			>
				<AlertDialogHeader>
					<AlertDialogTitle>Delete “{shown?.title}”?</AlertDialogTitle>
					<AlertDialogDescription>
						It is removed from the timeline for good. Only notes you added can be deleted.
					</AlertDialogDescription>
				</AlertDialogHeader>
				<AlertDialogFooter>
					<AlertDialogCancel>Keep it</AlertDialogCancel>
					<AlertDialogAction
						onClick={() => {
							if (shown) {
								setConfirmed(true);
								onConfirm(shown);
							}
						}}
						variant="destructive"
					>
						Delete
					</AlertDialogAction>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	);
}
