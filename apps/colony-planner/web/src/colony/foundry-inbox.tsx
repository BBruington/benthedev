import { useQueryClient } from "@tanstack/react-query";
import { HourglassIcon, SunriseIcon, XIcon } from "lucide-react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { problemMessage } from "@/api/problem";
import { bridgeStatusQuery, colonyBoardQuery } from "@/api/queries";
import { Button } from "@/components/ui/button";
import { useCampaignId } from "@/hooks/use-campaign";
import { useSheetState } from "@/hooks/use-sheet-state";
import { formatNumber } from "@/lib/colony";
import { formatRelativeTime } from "@/lib/time";
import { focusAfterClose } from "@/people/colonist-actions";
import { AdvanceSheet } from "./advance-sheet";
import { isConflict, useColonyChanges } from "./use-colony-changes";

type BridgeInboxItem = Schemas["BridgeInboxItem"];

const alreadyHandledMessage = "That Foundry inbox item was already handled.";
const boardInboxHeadingId = "foundry-inbox-heading";

/** "Foundry moved 3 days." */
export function foundryInboxText(item: BridgeInboxItem): string {
	return item.days === 1
		? "Foundry moved 1 day."
		: `Foundry moved ${formatNumber(item.days)} days.`;
}

function advanceButtonId(item: BridgeInboxItem): string {
	return `foundry-inbox-${item.id}-advance`;
}

/** Where focus goes once an item leaves the list: a neighbor's Advance, else the caller's targets. */
function focusTargetsAfter(
	items: BridgeInboxItem[],
	item: BridgeInboxItem,
	fallbackIds: string[],
): string[] {
	const index = items.findIndex((candidate) => candidate.id === item.id);
	const neighbors = [items[index + 1], items[index - 1]].filter(
		(neighbor): neighbor is BridgeInboxItem => neighbor !== undefined,
	);
	return [...neighbors.map(advanceButtonId), ...fallbackIds];
}

/**
 * The DM's Foundry inbox (docs/design/foundry-bridge.md §5): each item changes nothing until the DM
 * acts. Advance opens the Advance sheet with Foundry's day count and accepts the item with the
 * advance; Dismiss sets it aside. Used by the colony board and by Settings' Foundry card.
 */
export function FoundryInbox({
	items,
	fallbackFocusIds,
}: {
	items: BridgeInboxItem[];
	/** Stable targets for focus once the last item is gone. */
	fallbackFocusIds: string[];
}) {
	const campaignId = useCampaignId();
	const queryClient = useQueryClient();
	const changes = useColonyChanges();
	const advance = useSheetState<BridgeInboxItem>();

	function refresh() {
		queryClient.invalidateQueries({ queryKey: colonyBoardQuery(campaignId).queryKey });
		queryClient.invalidateQueries({ queryKey: bridgeStatusQuery(campaignId).queryKey });
	}

	const dismiss = api.useMutation(
		"post",
		"/api/campaigns/{campaignId}/bridge/inbox/{eventId}/dismiss",
		{
			onError: (error) => {
				refresh();
				toast.error(problemMessage(error, "That item couldn't be dismissed."));
			},
			onSuccess: () => {
				refresh();
				toast.success("Dismissed. The colony's clock stays where it is.");
			},
		},
	);

	// A 409 is either a stale preview (the sheet stays open with a fresh one) or an item handled
	// elsewhere, which the fresh board tells apart.
	async function handleAdvanceError(item: BridgeInboxItem, error: unknown) {
		if (!isConflict(error)) {
			return;
		}
		queryClient.invalidateQueries({ queryKey: bridgeStatusQuery(campaignId).queryKey });
		const board = await queryClient
			.fetchQuery({ ...colonyBoardQuery(campaignId), staleTime: 0 })
			.catch((readError: unknown) => {
				toast.error(problemMessage(readError, "The colony board couldn't be read. Try again."));
				return null;
			});
		if (board === null) {
			return;
		}
		const stillPending = board.dm?.bridgeInbox.some((pending) => pending.id === item.id) ?? false;
		if (stillPending) {
			return;
		}
		toast.error(alreadyHandledMessage);
		advance.onOpenChange(false);
		focusAfterClose(...focusTargetsAfter(items, item, fallbackFocusIds));
	}

	return (
		<>
			<ul className="grid gap-2">
				{items.map((item) => (
					<li
						className="flex flex-col gap-3 rounded-lg border border-border bg-surface-sunken/60 px-3 py-3 sm:flex-row sm:items-center sm:justify-between"
						key={item.id}
					>
						<div className="flex min-w-0 items-start gap-2.5">
							<HourglassIcon aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-gilt-ink" />
							<div className="min-w-0">
								<p className="font-medium">{foundryInboxText(item)}</p>
								<p className="text-muted-foreground text-sm">
									Received {formatRelativeTime(item.receivedAt)}
								</p>
							</div>
						</div>
						<div className="grid grid-cols-2 gap-2 sm:flex sm:shrink-0">
							<Button
								id={advanceButtonId(item)}
								onClick={() => {
									advance.show(item);
								}}
								size="sm"
							>
								<SunriseIcon />
								Advance
							</Button>
							<Button
								disabled={dismiss.isPending && dismiss.variables?.params.path.eventId === item.id}
								onClick={() => {
									dismiss.mutate(
										{ params: { path: { campaignId, eventId: item.id } } },
										{
											// A 409 means the item was handled elsewhere, so it leaves the list too.
											onError: (error) => {
												if (isConflict(error)) {
													focusAfterClose(...focusTargetsAfter(items, item, fallbackFocusIds));
												}
											},
											onSuccess: () => {
												focusAfterClose(...focusTargetsAfter(items, item, fallbackFocusIds));
											},
										},
									);
								}}
								size="sm"
								variant="outline"
							>
								<XIcon />
								Dismiss
							</Button>
						</div>
					</li>
				))}
			</ul>
			{advance.value ? (
				<AdvanceSheet
					bridgeEventId={advance.value.id}
					changes={changes}
					initialDays={advance.value.days}
					key={advance.session}
					onAdvanced={() => {
						queryClient.invalidateQueries({ queryKey: bridgeStatusQuery(campaignId).queryKey });
						if (advance.value) {
							focusAfterClose(...focusTargetsAfter(items, advance.value, fallbackFocusIds));
						}
					}}
					onAdvanceError={(error) => {
						if (advance.value) {
							handleAdvanceError(advance.value, error);
						}
					}}
					onOpenChange={advance.onOpenChange}
					open={advance.open}
				/>
			) : null}
		</>
	);
}

/** The board's card for the DM: absent while the inbox is empty, and in View as player. */
export function BoardFoundryInbox({
	items,
	fallbackFocusId,
}: {
	items: BridgeInboxItem[];
	/** The board's own heading, which stays when this card goes. */
	fallbackFocusId: string;
}) {
	if (items.length === 0) {
		return null;
	}
	return (
		<section
			aria-labelledby={boardInboxHeadingId}
			className="flex flex-col gap-4 rounded-xl border border-gilt/45 bg-card p-4 shadow-card sm:p-5"
		>
			<div>
				<h2
					className="font-display font-semibold text-xl tracking-wide outline-none"
					id={boardInboxHeadingId}
					tabIndex={-1}
				>
					From Foundry
				</h2>
				<p className="text-muted-foreground text-sm">
					Nothing changes here until you choose. Only you see this.
				</p>
			</div>
			<FoundryInbox fallbackFocusIds={[fallbackFocusId]} items={items} />
		</section>
	);
}
