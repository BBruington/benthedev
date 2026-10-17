import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { ArrowRightLeftIcon, UsersIcon } from "lucide-react";
import { problemMessage } from "@/api/problem";
import { npcsQuery, workforceQuery } from "@/api/queries";
import { useColonyChanges } from "@/colony/use-colony-changes";
import { PageHeader } from "@/components/page-header";
import { StatusPanel } from "@/components/status-panel";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { fallbackCampaignName, useCampaignId, useCampaignName } from "@/hooks/use-campaign";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { useMe } from "@/hooks/use-me";
import { useSheetState } from "@/hooks/use-sheet-state";
import { cachedMe, canUseDmTools } from "@/lib/access";
import {
	ChangeRoleSheet,
	type ColonistTarget,
	JoinColonySheet,
	LeaveColonyDialog,
} from "@/people/colonist-actions";
import {
	NamedColonists,
	namedColonistsHeadingId,
	RoleTable,
	roleTableHeadingId,
} from "@/people/people-sections";
import { SetCountSheet, TransferSheet } from "@/people/workforce-sheets";

export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell/people")({
	component: PeoplePage,
	loader: ({ context: { campaignId, queryClient } }) => {
		if (!campaignId) {
			return;
		}
		queryClient.prefetchQuery(workforceQuery(campaignId));
		const me = cachedMe(queryClient, campaignId);
		if (me && canUseDmTools(me)) {
			queryClient.prefetchQuery(npcsQuery(campaignId));
		}
	},
});

type LeaveTarget = ColonistTarget & { roleLabel: string };

/** /people (spec §5.5, §6.2): every role's headcount, and the colonists with names. */
function PeoplePage() {
	useDocumentTitle("People", useCampaignName() ?? fallbackCampaignName);
	const me = useMe();
	const isDm = canUseDmTools(me);
	const campaignId = useCampaignId();
	const workforce = useQuery(workforceQuery(campaignId));
	const npcs = useQuery({ ...npcsQuery(campaignId), enabled: isDm });
	const changes = useColonyChanges();
	const transfer = useSheetState<{ fromKey?: string }>();
	const setCount = useSheetState<{ roleKey?: string }>();
	const join = useSheetState<ColonistTarget | null>();
	const changeRole = useSheetState<ColonistTarget>();
	const leave = useSheetState<LeaveTarget>();

	if (workforce.isPending) {
		return (
			<div aria-busy="true" className="flex flex-col gap-6">
				<Skeleton className="h-16 w-2/3" />
				<Skeleton className="h-80 rounded-2xl" />
				<Skeleton className="h-40 rounded-2xl" />
			</div>
		);
	}
	if (workforce.isError) {
		return (
			<StatusPanel
				description={problemMessage(workforce.error, "The people couldn't be loaded.")}
				icon={UsersIcon}
				title="People couldn't be loaded"
				tone="ember"
			/>
		);
	}
	const data = workforce.data;
	const candidates: ColonistTarget[] = (npcs.data?.npcs ?? [])
		.filter((npc) => npc.colonyRole === null)
		.map((npc) => ({
			displayName: npc.displayName,
			id: npc.id,
			isHidden: npc.dm?.revealed === false,
			roleKey: null,
		}));
	const focusAfterColonistChange = [namedColonistsHeadingId, roleTableHeadingId];

	return (
		<div className="flex flex-col gap-6 md:gap-8">
			<PageHeader
				actions={
					isDm ? (
						<>
							<Button
								onClick={() => {
									transfer.show({});
								}}
								variant="outline"
							>
								<ArrowRightLeftIcon />
								Transfer
							</Button>
							<Button
								onClick={() => {
									setCount.show({});
								}}
								variant="outline"
							>
								<UsersIcon />
								Set count
							</Button>
						</>
					) : null
				}
				description={
					isDm
						? "Who works the colony. Workforce changes are yours alone: nothing moves colonists on its own."
						: "Who works the colony, role by role."
				}
				title="People"
			/>
			<RoleTable
				isDm={isDm}
				onSetCount={(roleKey) => {
					setCount.show({ roleKey });
				}}
				onTransferFrom={(fromKey) => {
					transfer.show({ fromKey });
				}}
				workforce={data}
			/>
			<NamedColonists
				actions={
					isDm
						? {
								onChangeRole: (target) => {
									changeRole.show(target);
								},
								onJoin: () => {
									join.show(null);
								},
								onLeave: (target, roleLabel) => {
									leave.show({ ...target, roleLabel });
								},
							}
						: null
				}
				workforce={data}
			/>
			{isDm && transfer.value ? (
				<TransferSheet
					changes={changes}
					initialFromKey={transfer.value.fromKey}
					key={transfer.session}
					onOpenChange={transfer.onOpenChange}
					open={transfer.open}
					workforce={data}
				/>
			) : null}
			{isDm && setCount.value ? (
				<SetCountSheet
					changes={changes}
					initialRoleKey={setCount.value.roleKey}
					key={setCount.session}
					onOpenChange={setCount.onOpenChange}
					open={setCount.open}
					workforce={data}
				/>
			) : null}
			{isDm && join.session > 0 ? (
				<JoinColonySheet
					candidates={candidates}
					changes={changes}
					focusTargetIds={focusAfterColonistChange}
					key={join.session}
					npc={join.value}
					onOpenChange={join.onOpenChange}
					open={join.open}
					workforce={data}
				/>
			) : null}
			{isDm && changeRole.value ? (
				<ChangeRoleSheet
					changes={changes}
					key={changeRole.session}
					npc={changeRole.value}
					onOpenChange={changeRole.onOpenChange}
					open={changeRole.open}
					workforce={data}
				/>
			) : null}
			{isDm ? (
				<LeaveColonyDialog
					changes={changes}
					focusTargetIds={focusAfterColonistChange}
					npc={leave.value}
					onOpenChange={leave.onOpenChange}
					open={leave.open}
					roleLabel={leave.value?.roleLabel ?? "their role"}
					workforce={data}
				/>
			) : null}
		</div>
	);
}
