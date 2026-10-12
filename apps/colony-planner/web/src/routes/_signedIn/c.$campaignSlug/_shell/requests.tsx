import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link, useNavigate, useRouter } from "@tanstack/react-router";
import { InboxIcon, Undo2Icon } from "lucide-react";
import { type ReactNode, useRef } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { problemMessage } from "@/api/problem";
import {
	colonyBoardQuery,
	membersQuery,
	patronQueryKeyPrefix,
	requestQuery,
	requestQueryKeyPrefix,
	requestsQuery,
	requestsQueryKeyPrefix,
} from "@/api/queries";
import { useColonyChanges } from "@/colony/use-colony-changes";
import { PageHeader } from "@/components/page-header";
import { StatusPanel } from "@/components/status-panel";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
	fallbackCampaignName,
	useCampaign,
	useCampaignId,
	useCampaignName,
} from "@/hooks/use-campaign";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { useMe } from "@/hooks/use-me";
import { cachedMe, canUseDmTools } from "@/lib/access";
import { focusFirstVisible } from "@/lib/focus";
import { focusAfterClose } from "@/people/colonist-actions";
import { ApprovalSheet } from "@/ventures/approval-sheet";
import { requestsPageHeadingId } from "@/ventures/ids";
import { RequestCard } from "@/ventures/venture-parts";

type Request = Schemas["Request"];
type RequestsSearch = { review?: string };

export const Route = createFileRoute("/_signedIn/c/$campaignSlug/_shell/requests")({
	component: RequestsPage,
	loader: ({ context: { campaignId, queryClient } }) => {
		if (!campaignId) {
			return;
		}
		queryClient.prefetchQuery(requestsQuery(campaignId));
		queryClient.prefetchQuery(colonyBoardQuery(campaignId));
		const me = cachedMe(queryClient, campaignId);
		if (me && canUseDmTools(me)) {
			queryClient.prefetchQuery(membersQuery(campaignId));
		}
	},
	validateSearch: (search: Record<string, unknown>): RequestsSearch => {
		if (typeof search.review === "string" && search.review !== "") {
			return { review: search.review };
		}
		return {};
	},
});

const myRequestsHeadingId = "my-requests-heading";
const queueHeadingId = "request-queue-heading";

/**
 * /requests (spec §6.2): the DM's approval queue (pending first) and history; a player's own requests
 * (withdraw while pending) and the party's. ?review={id} opens the approval sheet, for the badge, the
 * toast and the board's Review buttons.
 */
function RequestsPage() {
	useDocumentTitle("Requests", useCampaignName() ?? fallbackCampaignName);
	const me = useMe();
	const isDm = canUseDmTools(me);
	const campaignId = useCampaignId();
	const requests = useQuery(requestsQuery(campaignId));
	const board = useQuery(colonyBoardQuery(campaignId));
	const resources = board.data?.resources ?? [];

	let body: ReactNode;
	if (requests.isPending) {
		body = (
			<div aria-busy="true" className="grid gap-2">
				<Skeleton className="h-24 rounded-xl" />
				<Skeleton className="h-24 rounded-xl" />
			</div>
		);
	} else if (requests.isError) {
		body = (
			<StatusPanel
				description={problemMessage(requests.error, "The requests couldn't be loaded.")}
				icon={InboxIcon}
				title="The requests couldn't be loaded"
				tone="ember"
			/>
		);
	} else if (isDm) {
		body = <DmRequests requests={requests.data.requests} resources={resources} />;
	} else {
		body = <PlayerRequests requests={requests.data.requests} resources={resources} />;
	}

	return (
		<div className="flex flex-col gap-8">
			<PageHeader
				description={
					isDm
						? "What the party asks of its patrons. Grant or decline each one; nothing is spent until you grant it."
						: "What you and the party have asked of the colony's patrons, and what the DM decided."
				}
				headingId={requestsPageHeadingId}
				title="Requests"
			/>
			{body}
		</div>
	);
}

function RequestSection({
	headingId,
	title,
	empty,
	children,
	isEmpty,
}: {
	headingId: string;
	title: string;
	empty: ReactNode;
	children: ReactNode;
	isEmpty: boolean;
}) {
	return (
		<section aria-labelledby={headingId} className="flex flex-col gap-3">
			<h2
				className="font-display font-semibold text-2xl tracking-wide outline-none"
				id={headingId}
				tabIndex={-1}
			>
				{title}
			</h2>
			{isEmpty ? (
				<p className="rounded-xl border border-border border-dashed px-4 py-4 text-muted-foreground">
					{empty}
				</p>
			) : (
				<ul className="grid gap-2">{children}</ul>
			)}
		</section>
	);
}

function DmRequests({
	requests,
	resources,
}: {
	requests: readonly Request[];
	resources: readonly { key: string; label: string }[];
}) {
	const campaignId = useCampaignId();
	const { slug } = useCampaign();
	const navigate = useNavigate();
	const router = useRouter();
	const { review } = Route.useSearch();
	const changes = useColonyChanges();
	// A review opened here pushed its own history entry, so closing it steps back over that entry
	// rather than leaving a second /requests that Back would land on first.
	const reviewOpenedHere = useRef<string | null>(null);
	const pending = requests.filter((request) => request.status === "PENDING");
	const decided = requests.filter((request) => request.status !== "PENDING");
	const listed = requests.find((request) => request.id === review);
	const fetched = useQuery({
		...requestQuery(campaignId, review ?? ""),
		enabled: review !== undefined && listed === undefined,
	});
	const reviewing = listed ?? fetched.data;

	function openReview(requestId: string) {
		reviewOpenedHere.current = requestId;
		navigate({
			params: { campaignSlug: slug },
			search: { review: requestId },
			to: "/c/$campaignSlug/requests",
		});
	}

	function closeReview() {
		if (review !== undefined && reviewOpenedHere.current === review) {
			reviewOpenedHere.current = null;
			router.history.back();
			return;
		}
		navigate({
			params: { campaignSlug: slug },
			replace: true,
			search: {},
			to: "/c/$campaignSlug/requests",
		});
	}

	return (
		<>
			<RequestSection
				empty="Nothing waiting. New requests appear here and on the board."
				headingId={queueHeadingId}
				isEmpty={pending.length === 0}
				title="Waiting for you"
			>
				{pending.map((request) => (
					<RequestCard
						actions={
							<Button
								onClick={() => {
									openReview(request.id);
								}}
							>
								Review
							</Button>
						}
						key={request.id}
						request={request}
						resources={resources}
					/>
				))}
			</RequestSection>
			<RequestSection
				empty="No decisions yet."
				headingId="request-history-heading"
				isEmpty={decided.length === 0}
				title="History"
			>
				{decided.map((request) => (
					<RequestCard key={request.id} request={request} resources={resources} />
				))}
			</RequestSection>
			{review !== undefined && fetched.isError ? (
				<p className="text-ember-ink" role="alert">
					{problemMessage(fetched.error, "That request couldn't be found.")}
				</p>
			) : null}
			{reviewing ? (
				<ApprovalSheet
					changes={changes}
					key={reviewing.id}
					onDecided={() => {
						focusAfterClose(queueHeadingId);
					}}
					onOpenChange={(open) => {
						if (!open) {
							closeReview();
						}
					}}
					open
					request={reviewing}
				/>
			) : null}
		</>
	);
}

function PlayerRequests({
	requests,
	resources,
}: {
	requests: readonly Request[];
	resources: readonly { key: string; label: string }[];
}) {
	const { slug } = useCampaign();
	const mine = requests.filter((request) => request.mine);
	const others = requests.filter((request) => !request.mine);
	return (
		<>
			<RequestSection
				empty={
					<>
						You haven't asked for anything yet.{" "}
						<Link
							className="text-gilt-ink underline underline-offset-4"
							params={{ campaignSlug: slug }}
							to="/c/$campaignSlug/patrons"
						>
							Ask a patron
						</Link>
						.
					</>
				}
				headingId={myRequestsHeadingId}
				isEmpty={mine.length === 0}
				title="My requests"
			>
				{mine.map((request) => (
					<RequestCard
						actions={request.status === "PENDING" ? <WithdrawButton request={request} /> : null}
						key={request.id}
						request={request}
						resources={resources}
					/>
				))}
			</RequestSection>
			<RequestSection
				empty="Nobody else has asked for anything yet."
				headingId="party-requests-heading"
				isEmpty={others.length === 0}
				title="The party's requests"
			>
				{others.map((request) => (
					<RequestCard key={request.id} request={request} resources={resources} />
				))}
			</RequestSection>
		</>
	);
}

function WithdrawButton({ request }: { request: Request }) {
	const campaignId = useCampaignId();
	const queryClient = useQueryClient();
	const mutation = api.useMutation(
		"post",
		"/api/campaigns/{campaignId}/requests/{requestId}/withdraw",
		{
			onError: (error) => {
				toast.error(problemMessage(error, "The request couldn't be withdrawn."));
			},
			onSettled: () =>
				Promise.all([
					queryClient.invalidateQueries({ queryKey: requestsQueryKeyPrefix(campaignId) }),
					queryClient.invalidateQueries({ queryKey: requestQueryKeyPrefix(campaignId) }),
					queryClient.invalidateQueries({ queryKey: patronQueryKeyPrefix(campaignId) }),
				]),
			onSuccess: (withdrawn) => {
				toast.success(`Withdrew your request for ${withdrawn.title}.`);
				focusFirstVisible(myRequestsHeadingId);
			},
		},
	);
	return (
		<Button
			disabled={mutation.isPending}
			onClick={() => {
				mutation.mutate({ params: { path: { campaignId, requestId: request.id } } });
			}}
			variant="outline"
		>
			<Undo2Icon />
			{mutation.isPending ? "Withdrawing…" : "Withdraw"}
		</Button>
	);
}
