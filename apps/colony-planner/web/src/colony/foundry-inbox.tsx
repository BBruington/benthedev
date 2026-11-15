import { useQueryClient } from "@tanstack/react-query";
import {
	CheckIcon,
	CoinsIcon,
	HourglassIcon,
	type LucideIcon,
	PackageIcon,
	StoreIcon,
	SunriseIcon,
	UserPlusIcon,
	XIcon,
} from "lucide-react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { problemMessage } from "@/api/problem";
import { bridgeStatusQuery, colonyBoardQuery } from "@/api/queries";
import { Button } from "@/components/ui/button";
import { useCampaignId } from "@/hooks/use-campaign";
import { useSheetState } from "@/hooks/use-sheet-state";
import { formatNumber } from "@/lib/colony";
import { formatRelativeTime } from "@/lib/time";
import { ItemSheet } from "@/market/item-sheet";
import { NpcSheet } from "@/npcs/npc-sheet";
import { focusAfterClose } from "@/people/colonist-actions";
import { AdvanceSheet } from "./advance-sheet";
import { isConflict, useColonyChanges } from "./use-colony-changes";

type BridgeInboxItem = Schemas["BridgeInboxItem"];
type BridgeEventKind = Schemas["BridgeEventKind"];

const alreadyHandledMessage = "That Foundry inbox item was already handled.";
const boardInboxHeadingId = "foundry-inbox-heading";

/** "Foundry moved 3 days.", "The Wayfarers deposited 40 gp.", "Foundry sent an NPC: Brother Aldo." */
export function foundryInboxText(item: BridgeInboxItem): string {
	if (item.kind === "COIN_DEPOSIT" && item.deposit) {
		return `${item.deposit.actorName} deposited ${formatNumber(item.deposit.gold)} gp.`;
	}
	if (item.kind === "NPC_IMPORT" && item.npc) {
		return `Foundry sent an NPC: ${item.npc.name}.`;
	}
	if (item.kind === "ITEM_IMPORT" && item.item) {
		return `Foundry sent an item: ${item.item.name}.`;
	}
	return item.days === 1
		? "Foundry moved 1 day."
		: `Foundry moved ${formatNumber(item.days)} days.`;
}

function dismissedMessage(kind: BridgeEventKind | undefined): string {
	if (kind === "COIN_DEPOSIT") {
		return "Dismissed. Foundry returns the coins to whoever paid them.";
	}
	if (kind === "NPC_IMPORT" || kind === "ITEM_IMPORT") {
		return "Dismissed. Nothing was added.";
	}
	return "Dismissed. The colony's clock stays where it is.";
}

const kindIcons: Record<BridgeEventKind, LucideIcon> = {
	COIN_DEPOSIT: CoinsIcon,
	ITEM_IMPORT: PackageIcon,
	NPC_IMPORT: UserPlusIcon,
	STATUS: HourglassIcon,
	TIME_ADVANCED: HourglassIcon,
};

function primaryButtonId(item: BridgeInboxItem): string {
	return `foundry-inbox-${item.id}-primary`;
}

/** Where focus goes once an item leaves the list: a neighbor's first button, else the caller's targets. */
function focusTargetsAfter(
	items: BridgeInboxItem[],
	item: BridgeInboxItem,
	fallbackIds: string[],
): string[] {
	const index = items.findIndex((candidate) => candidate.id === item.id);
	const neighbors = [items[index + 1], items[index - 1]].filter(
		(neighbor): neighbor is BridgeInboxItem => neighbor !== undefined,
	);
	return [...neighbors.map(primaryButtonId), ...fallbackIds];
}

/**
 * The DM's Foundry inbox (docs/design/foundry-bridge.md §5, §14.3): each item changes nothing until
 * the DM acts. Advance opens the Advance sheet with Foundry's day count, Accept adds a deposit's gold,
 * and Import as NPC and Add to Shipments open their create sheets filled from Foundry; each accepts
 * the item in the same write. Dismiss sets it aside. Used by the colony board and by Settings'
 * Foundry card.
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
	const sheet = useSheetState<BridgeInboxItem>();

	function refresh() {
		queryClient.invalidateQueries({ queryKey: colonyBoardQuery(campaignId).queryKey });
		queryClient.invalidateQueries({ queryKey: bridgeStatusQuery(campaignId).queryKey });
	}

	function focusAfterLeaving(item: BridgeInboxItem) {
		focusAfterClose(...focusTargetsAfter(items, item, fallbackFocusIds));
	}

	const dismiss = api.useMutation(
		"post",
		"/api/campaigns/{campaignId}/bridge/inbox/{eventId}/dismiss",
		{
			onError: (error) => {
				refresh();
				toast.error(problemMessage(error, "That item couldn't be dismissed."));
			},
			onSuccess: (_data, variables) => {
				refresh();
				const dismissed = items.find((item) => item.id === variables.params.path.eventId);
				toast.success(dismissedMessage(dismissed?.kind));
			},
		},
	);
	const accept = changes.acceptBridgeDeposit;

	/** Reads the board again: true while the item still waits, null when the board couldn't be read. */
	async function isStillPending(item: BridgeInboxItem): Promise<boolean | null> {
		queryClient.invalidateQueries({ queryKey: bridgeStatusQuery(campaignId).queryKey });
		const board = await queryClient
			.fetchQuery({ ...colonyBoardQuery(campaignId), staleTime: 0 })
			.catch((readError: unknown) => {
				toast.error(problemMessage(readError, "The colony board couldn't be read. Try again."));
				return null;
			});
		if (board === null) {
			return null;
		}
		return board.dm?.bridgeInbox.some((pending) => pending.id === item.id) ?? false;
	}

	// A 409 from a sheet is either the sheet's own conflict (it stays open and says so) or an item
	// handled elsewhere, which the fresh board tells apart.
	async function handleSheetError(item: BridgeInboxItem, error: unknown) {
		if (!isConflict(error) || (await isStillPending(item)) !== false) {
			return;
		}
		toast.error(alreadyHandledMessage);
		sheet.onOpenChange(false);
		focusAfterLeaving(item);
	}

	function handleCreated(item: BridgeInboxItem) {
		refresh();
		focusAfterLeaving(item);
	}

	// Accept's 409 is either an item handled elsewhere or gold that can't take the coins (missing,
	// archived or past its max), and then the item still waits and the server's message says why.
	async function handleAcceptError(item: BridgeInboxItem, error: unknown) {
		const fallback = "The deposit couldn't be accepted.";
		if (!isConflict(error)) {
			toast.error(problemMessage(error, fallback));
			return;
		}
		const stillPending = await isStillPending(item);
		if (stillPending === null) {
			return;
		}
		if (stillPending) {
			toast.error(problemMessage(error, fallback));
			return;
		}
		toast.error(alreadyHandledMessage);
		focusAfterLeaving(item);
	}

	function primaryAction(item: BridgeInboxItem) {
		if (item.kind === "COIN_DEPOSIT") {
			return {
				disabled: accept.isPending && accept.variables?.params.path.eventId === item.id,
				icon: <CheckIcon />,
				label: "Accept",
				onClick: () => {
					accept.mutate(
						{ params: { path: { campaignId, eventId: item.id } } },
						{
							onError: (error) => {
								handleAcceptError(item, error);
							},
							onSuccess: () => {
								queryClient.invalidateQueries({
									queryKey: bridgeStatusQuery(campaignId).queryKey,
								});
								focusAfterLeaving(item);
							},
						},
					);
				},
			};
		}
		if (item.kind === "NPC_IMPORT") {
			return {
				disabled: false,
				icon: <UserPlusIcon />,
				label: "Import as NPC",
				onClick: () => sheet.show(item),
			};
		}
		if (item.kind === "ITEM_IMPORT") {
			return {
				disabled: false,
				icon: <StoreIcon />,
				label: "Add to Shipments",
				onClick: () => sheet.show(item),
			};
		}
		return {
			disabled: false,
			icon: <SunriseIcon />,
			label: "Advance",
			onClick: () => sheet.show(item),
		};
	}

	const opened = sheet.value;

	return (
		<>
			<ul className="grid gap-2">
				{items.map((item) => {
					const KindIcon = kindIcons[item.kind];
					const action = primaryAction(item);
					return (
						<li
							className="flex flex-col gap-3 rounded-lg border border-border bg-surface-sunken/60 px-3 py-3 sm:flex-row sm:items-center sm:justify-between"
							key={item.id}
						>
							<div className="flex min-w-0 items-start gap-2.5">
								<KindIcon aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-gilt-ink" />
								<div className="min-w-0">
									<p className="font-medium [overflow-wrap:anywhere]">{foundryInboxText(item)}</p>
									<p className="text-muted-foreground text-sm">
										Received {formatRelativeTime(item.receivedAt)}
									</p>
								</div>
							</div>
							<div className="grid grid-cols-2 gap-2 sm:flex sm:shrink-0">
								<Button
									disabled={action.disabled}
									id={primaryButtonId(item)}
									onClick={action.onClick}
									size="sm"
								>
									{action.icon}
									{action.label}
								</Button>
								<Button
									disabled={dismiss.isPending && dismiss.variables?.params.path.eventId === item.id}
									onClick={() => {
										dismiss.mutate(
											{ params: { path: { campaignId, eventId: item.id } } },
											{
												// A 409 is an item handled elsewhere, which leaves the list too, or a
												// deposit that can't go back without a connected world, which stays.
												onError: async (error) => {
													if (isConflict(error) && (await isStillPending(item)) === false) {
														focusAfterLeaving(item);
													}
												},
												onSuccess: () => {
													focusAfterLeaving(item);
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
					);
				})}
			</ul>
			{opened?.kind === "TIME_ADVANCED" ? (
				<AdvanceSheet
					bridgeEventId={opened.id}
					changes={changes}
					initialDays={opened.days}
					key={sheet.session}
					onAdvanced={() => {
						queryClient.invalidateQueries({ queryKey: bridgeStatusQuery(campaignId).queryKey });
						focusAfterLeaving(opened);
					}}
					onAdvanceError={(error) => {
						handleSheetError(opened, error);
					}}
					onOpenChange={sheet.onOpenChange}
					open={sheet.open}
				/>
			) : null}
			{opened?.kind === "NPC_IMPORT" && opened.npc ? (
				<NpcSheet
					bridgeEventId={opened.id}
					key={sheet.session}
					npc={null}
					onCreated={() => {
						handleCreated(opened);
					}}
					onCreateError={(error) => {
						handleSheetError(opened, error);
					}}
					onOpenChange={sheet.onOpenChange}
					open={sheet.open}
					prefill={opened.npc}
				/>
			) : null}
			{opened?.kind === "ITEM_IMPORT" && opened.item ? (
				<ItemSheet
					bridgeEventId={opened.id}
					campaignId={campaignId}
					item={null}
					key={sheet.session}
					onCreated={() => {
						handleCreated(opened);
					}}
					onCreateError={(error) => {
						handleSheetError(opened, error);
					}}
					onOpenChange={sheet.onOpenChange}
					open={sheet.open}
					prefill={opened.item}
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
