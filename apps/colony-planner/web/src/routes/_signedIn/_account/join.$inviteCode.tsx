import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import {
	ArrowRightIcon,
	CompassIcon,
	DoorOpenIcon,
	HourglassIcon,
	LayoutGridIcon,
	PauseCircleIcon,
	ServerCrashIcon,
	ShieldXIcon,
} from "lucide-react";
import type { ReactNode } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { problemMessage, problemStatus } from "@/api/problem";
import { campaignMeQuery, inviteQuery, myCampaignsQuery } from "@/api/queries";
import { StatusPanel } from "@/components/status-panel";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useDocumentTitle } from "@/hooks/use-document-title";

type InviteInfo = Schemas["InviteInfo"];
type MyCampaign = Schemas["MyCampaign"];

// An invite link (docs/design/multi-campaign.md §5.3): who runs the campaign and where the caller
// stands there, then "Ask to join". A visitor signs in first and comes back here.
export const Route = createFileRoute("/_signedIn/_account/join/$inviteCode")({
	component: JoinPage,
	loader: ({ context, params }) => {
		context.queryClient.prefetchQuery(inviteQuery(params.inviteCode));
	},
});

function JoinPage() {
	const { inviteCode } = Route.useParams();
	const invite = useQuery(inviteQuery(inviteCode));
	useDocumentTitle(
		invite.data ? `Join ${invite.data.campaignName}` : "Invite link",
		"Colony Planner",
	);

	if (invite.isPending) {
		return (
			<div aria-busy="true" className="mx-auto flex max-w-xl flex-col items-center gap-4 py-16">
				<Skeleton className="size-20 rounded-full" />
				<Skeleton className="h-9 w-3/4" />
				<Skeleton className="h-5 w-2/3" />
			</div>
		);
	}
	if (invite.isError) {
		if (problemStatus(invite.error) === 404) {
			return (
				<StatusPanel
					actions={<YourCampaignsButton />}
					description="This invite link doesn't work any more. Ask your DM for a new one."
					eyebrow={<Badge variant="gilt">Invite link</Badge>}
					icon={CompassIcon}
					title="This invite link doesn't work"
				/>
			);
		}
		return (
			<StatusPanel
				actions={<YourCampaignsButton />}
				description={problemMessage(invite.error, "The invite couldn't be read. Try again.")}
				icon={ServerCrashIcon}
				title="The invite couldn't be read"
				tone="ember"
			/>
		);
	}
	return <InviteAnswer invite={invite.data} inviteCode={inviteCode} />;
}

function YourCampaignsButton({ variant = "outline" }: { variant?: "outline" | "default" }) {
	return (
		<Button asChild variant={variant}>
			<Link to="/">
				<LayoutGridIcon />
				Your campaigns
			</Link>
		</Button>
	);
}

function OpenCampaignButton({ membership, label }: { membership: MyCampaign; label: string }) {
	return (
		<Button asChild>
			<Link params={{ campaignSlug: membership.campaign.slug }} to="/c/$campaignSlug">
				{label}
				<ArrowRightIcon aria-hidden="true" />
			</Link>
		</Button>
	);
}

function InviteAnswer({ invite, inviteCode }: { invite: InviteInfo; inviteCode: string }) {
	const { campaignName, dmDisplayName, membership } = invite;
	// A campaign the caller already belongs to answers with that membership, unchanged (§5.3).
	if (membership?.status === "ACTIVE") {
		return (
			<Answer
				actions={
					<>
						<OpenCampaignButton label={`Open ${campaignName}`} membership={membership} />
						<YourCampaignsButton />
					</>
				}
				description={`${dmDisplayName} already let you in.`}
				icon={DoorOpenIcon}
				title={`You're already in ${campaignName}`}
				tone="verdigris"
			/>
		);
	}
	if (membership?.status === "REMOVED") {
		return (
			<Answer
				actions={<YourCampaignsButton />}
				description={`Only ${dmDisplayName} can let you back in. Ask them if you think that's a mistake.`}
				icon={ShieldXIcon}
				title={`You don't have access to ${campaignName}`}
				tone="ember"
			/>
		);
	}
	if (invite.campaignStatus === "SUSPENDED") {
		return (
			<Answer
				actions={<YourCampaignsButton />}
				description="Nobody can join while it's paused. Try the link again later."
				icon={PauseCircleIcon}
				title={`${campaignName} is paused by the site owner`}
				tone="verdigris"
			/>
		);
	}
	if (membership?.status === "PENDING") {
		return (
			<Answer
				actions={
					<>
						<OpenCampaignButton label="Open the waiting page" membership={membership} />
						<YourCampaignsButton />
					</>
				}
				description={`You already asked. ${dmDisplayName} knows you're waiting.`}
				icon={HourglassIcon}
				title={`Waiting for ${dmDisplayName}`}
			/>
		);
	}
	return <AskToJoin invite={invite} inviteCode={inviteCode} />;
}

function AskToJoin({ invite, inviteCode }: { invite: InviteInfo; inviteCode: string }) {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const join = api.useMutation("post", "/api/invites/{inviteCode}/join", {
		onError: (error) => {
			queryClient.invalidateQueries({ queryKey: inviteQuery(inviteCode).queryKey });
			toast.error(problemMessage(error, "Asking to join didn't work. Try again."));
		},
		onSuccess: async (membership) => {
			await queryClient.invalidateQueries({ queryKey: myCampaignsQuery.queryKey });
			queryClient.removeQueries({ queryKey: campaignMeQuery(membership.campaign.id).queryKey });
			navigate({ params: { campaignSlug: membership.campaign.slug }, to: "/c/$campaignSlug" });
		},
	});
	return (
		<Answer
			actions={
				<>
					<Button
						disabled={join.isPending}
						onClick={() => {
							join.mutate({ params: { path: { inviteCode } } });
						}}
						size="lg"
					>
						<DoorOpenIcon />
						{join.isPending ? "Asking…" : "Ask to join"}
					</Button>
					<YourCampaignsButton />
				</>
			}
			description={`${invite.dmDisplayName} decides who joins. You'll wait on a page that opens by itself once they let you in.`}
			icon={DoorOpenIcon}
			title={
				<>
					Join {invite.campaignName}, run by {invite.dmDisplayName}?
				</>
			}
		/>
	);
}

function Answer({
	icon,
	title,
	description,
	actions,
	tone = "gilt",
}: {
	icon: typeof DoorOpenIcon;
	title: ReactNode;
	description: string;
	actions: ReactNode;
	tone?: "gilt" | "verdigris" | "ember";
}) {
	return (
		<StatusPanel
			actions={actions}
			description={description}
			eyebrow={<Badge variant="gilt">Invite link</Badge>}
			icon={icon}
			title={title}
			tone={tone}
		/>
	);
}
