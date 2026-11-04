import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link, redirect, useNavigate } from "@tanstack/react-router";
import {
	ArrowRightIcon,
	CrownIcon,
	HourglassIcon,
	LinkIcon,
	MapIcon,
	PauseCircleIcon,
	ShieldXIcon,
	SparklesIcon,
} from "lucide-react";
import { type FormEvent, type ReactNode, useState } from "react";
import type { Schemas } from "@/api/client";
import { problemMessage } from "@/api/problem";
import {
	adminCampaignRequestsQuery,
	myCampaignRequestQuery,
	myCampaignsQuery,
} from "@/api/queries";
import { Field, fieldInputProps } from "@/components/field";
import { campaignPickerHeadingId } from "@/components/shell/account-ids";
import { StatusPanel } from "@/components/status-panel";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { useAccount } from "@/hooks/use-me";
import {
	type CampaignEntry,
	campaignCardLine,
	campaignEntry,
	campaignEntryLabels,
	findCampaignBySlug,
	inviteCodeFromText,
	type MyCampaign,
	sortCampaigns,
} from "@/lib/campaigns";
import { hasAppLanded, readLastCampaign } from "@/lib/last-campaign";
import { takeReturnTo } from "@/lib/return-to";
import { cn } from "@/lib/utils";

type CampaignRequestView = Schemas["CampaignRequestView"];

// The campaign picker (docs/design/multi-campaign.md §5.1). Landing here (opening the site, or
// after sign-in) resumes a deep link remembered before sign-in, or else goes straight to the last
// campaign opened when the caller is still let in there.
export const Route = createFileRoute("/_signedIn/_account/")({
	beforeLoad: async ({ context, preload }) => {
		if (preload || hasAppLanded()) {
			return;
		}
		const returnTo = takeReturnTo();
		if (returnTo) {
			throw redirect({ href: returnTo, replace: true });
		}
		const lastSlug = readLastCampaign();
		if (!lastSlug) {
			return;
		}
		const { campaigns } = await context.queryClient.ensureQueryData(myCampaignsQuery);
		const membership = findCampaignBySlug(campaigns, lastSlug);
		if (membership && campaignEntry(membership) === "open") {
			throw redirect({ params: { campaignSlug: lastSlug }, replace: true, to: "/c/$campaignSlug" });
		}
	},
	component: PickerPage,
	loader: ({ context }) => {
		context.queryClient.prefetchQuery(myCampaignsQuery);
		context.queryClient.prefetchQuery(myCampaignRequestQuery);
	},
});

function PickerPage() {
	useDocumentTitle("Your campaigns", "Colony Planner");
	const account = useAccount();
	const campaigns = useQuery(myCampaignsQuery);
	const myRequest = useQuery(myCampaignRequestQuery);

	return (
		<div className="flex flex-col gap-8 md:gap-10">
			<header className="space-y-1.5">
				<p className="font-medium text-gilt-ink text-xs uppercase tracking-[0.2em]">
					Welcome, {account.displayName}
				</p>
				<h1
					className="font-display font-semibold text-3xl tracking-wide outline-none md:text-4xl"
					id={campaignPickerHeadingId}
					tabIndex={-1}
				>
					Your campaigns
				</h1>
			</header>
			{account.isSiteOwner ? <SiteAdminCard /> : null}
			<CampaignList campaigns={campaigns} />
			<div className="grid gap-4 lg:grid-cols-2">
				<InviteLinkCard />
				<RunYourOwnCard
					isSiteOwner={account.isSiteOwner}
					request={myRequest.data?.request ?? null}
				/>
			</div>
		</div>
	);
}

function CampaignList({
	campaigns,
}: {
	campaigns: {
		data?: Schemas["MyCampaigns"];
		isPending: boolean;
		isError: boolean;
		error: unknown;
	};
}) {
	if (campaigns.isPending) {
		return (
			<div aria-busy="true" className="grid gap-4 sm:grid-cols-2">
				<Skeleton className="h-28 rounded-2xl" />
				<Skeleton className="h-28 rounded-2xl" />
			</div>
		);
	}
	if (campaigns.isError || !campaigns.data) {
		return (
			<StatusPanel
				description={problemMessage(campaigns.error, "Your campaigns couldn't be loaded.")}
				icon={MapIcon}
				title="Your campaigns couldn't be loaded"
				tone="ember"
			/>
		);
	}
	if (campaigns.data.campaigns.length === 0) {
		return (
			<p className="rounded-2xl border border-border border-dashed px-5 py-8 text-center text-muted-foreground">
				No campaigns yet. Open a DM's invite link to ask to join theirs, or run your own.
			</p>
		);
	}
	return (
		<ul aria-labelledby={campaignPickerHeadingId} className="grid gap-4 sm:grid-cols-2">
			{sortCampaigns(campaigns.data.campaigns).map((membership) => (
				<li key={membership.campaign.id}>
					<CampaignCard membership={membership} />
				</li>
			))}
		</ul>
	);
}

const entryBadges: Record<
	Exclude<CampaignEntry, "open">,
	{ icon: ReactNode; variant: "gilt" | "ember" | "verdigris" }
> = {
	"no-access": { icon: <ShieldXIcon />, variant: "ember" },
	paused: { icon: <PauseCircleIcon />, variant: "verdigris" },
	waiting: { icon: <HourglassIcon />, variant: "gilt" },
};

function CampaignCard({ membership }: { membership: MyCampaign }) {
	const entry = campaignEntry(membership);
	return (
		<Link
			className={cn(
				"group flex min-h-28 flex-col gap-3 rounded-2xl border bg-card p-5 shadow-card outline-none transition-[border-color,transform] hover:-translate-y-0.5 hover:border-gilt/45 focus-visible:ring-[3px] focus-visible:ring-ring/45",
				entry === "open" ? "border-border" : "border-border border-dashed",
			)}
			params={{ campaignSlug: membership.campaign.slug }}
			to="/c/$campaignSlug"
		>
			<div className="flex items-start justify-between gap-3">
				<h2 className="min-w-0 font-display font-semibold text-xl tracking-wide [overflow-wrap:anywhere]">
					{membership.campaign.name}
				</h2>
				{entry === "open" ? null : (
					<Badge variant={entryBadges[entry].variant}>
						{entryBadges[entry].icon}
						{campaignEntryLabels[entry]}
					</Badge>
				)}
			</div>
			<p className="flex items-center justify-between gap-3 text-muted-foreground">
				<span className="min-w-0 truncate">{campaignCardLine(membership)}</span>
				<ArrowRightIcon
					aria-hidden="true"
					className="size-4 shrink-0 text-gilt-ink transition-transform group-hover:translate-x-0.5"
				/>
			</p>
		</Link>
	);
}

function SiteAdminCard() {
	const requests = useQuery(adminCampaignRequestsQuery);
	const openCount = requests.data?.requests.length ?? 0;
	let status = "Campaign requests and the campaign directory.";
	if (openCount === 1) {
		status = "1 campaign request is waiting for you.";
	} else if (openCount > 1) {
		status = `${openCount} campaign requests are waiting for you.`;
	}
	return (
		<Link
			className="flex min-h-16 items-center gap-4 rounded-2xl border border-gilt/45 bg-gilt/8 px-5 py-4 outline-none transition-colors hover:border-gilt/70 focus-visible:ring-[3px] focus-visible:ring-ring/45"
			to="/admin"
		>
			<CrownIcon aria-hidden="true" className="size-5 shrink-0 text-gilt-ink" />
			<span className="min-w-0 flex-1">
				<span className="block font-semibold">Site admin</span>
				<span className="block text-muted-foreground text-sm">{status}</span>
			</span>
			<ArrowRightIcon aria-hidden="true" className="size-4 shrink-0 text-gilt-ink" />
		</Link>
	);
}

function InviteLinkCard() {
	const navigate = useNavigate();
	const [text, setText] = useState("");
	const [problem, setProblem] = useState<string | undefined>(undefined);

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		const inviteCode = inviteCodeFromText(text);
		if (!inviteCode) {
			setProblem("That doesn't look like an invite link. Copy the whole link your DM sent.");
			return;
		}
		navigate({ params: { inviteCode }, to: "/join/$inviteCode" });
	}

	return (
		<section
			aria-labelledby="invite-link-heading"
			className="flex flex-col gap-4 rounded-2xl border border-border bg-card p-5 shadow-card"
		>
			<div className="flex items-center gap-2.5">
				<LinkIcon aria-hidden="true" className="size-5 text-gilt-ink" />
				<h2 className="font-display font-semibold text-xl tracking-wide" id="invite-link-heading">
					Have an invite link?
				</h2>
			</div>
			<form className="grid gap-3" noValidate onSubmit={handleSubmit}>
				<Field
					description="Paste the link your DM shared, and ask to join their campaign."
					error={problem}
					id="picker-invite-link"
					label="Invite link"
				>
					<Input
						{...fieldInputProps("picker-invite-link", problem, true)}
						autoComplete="off"
						inputMode="url"
						onChange={(event) => {
							setText(event.target.value);
							setProblem(undefined);
						}}
						placeholder="https://…/join/…"
						spellCheck={false}
						value={text}
					/>
				</Field>
				<Button className="sm:w-fit" disabled={text.trim() === ""} type="submit" variant="outline">
					Open the link
					<ArrowRightIcon aria-hidden="true" />
				</Button>
			</form>
		</section>
	);
}

function RunYourOwnCard({
	isSiteOwner,
	request,
}: {
	isSiteOwner: boolean;
	request: CampaignRequestView | null;
}) {
	let status: ReactNode = isSiteOwner
		? "Create one now, and you'll DM your own colony here."
		: "Ask the site owner, and you'll DM your own colony here.";
	let action = isSiteOwner ? "Create a campaign" : "Ask to run a campaign";
	if (request?.status === "PENDING") {
		status = (
			<>
				<span className="font-medium text-gilt-ink">Waiting for the site owner</span> to approve{" "}
				{request.campaignName}.
			</>
		);
		action = "See your request";
	} else if (request?.status === "DENIED") {
		status = `The site owner said no to ${request.campaignName} this time. You can ask again.`;
	} else if (request?.status === "APPROVED" && request.campaign) {
		status = `${request.campaign.name} is ready. You're its DM.`;
		action = isSiteOwner ? "Create another" : "Ask for another";
	}
	return (
		<section
			aria-labelledby="run-your-own-heading"
			className="flex flex-col gap-4 rounded-2xl border border-border bg-card p-5 shadow-card"
		>
			<div className="flex items-center gap-2.5">
				<SparklesIcon aria-hidden="true" className="size-5 text-gilt-ink" />
				<h2 className="font-display font-semibold text-xl tracking-wide" id="run-your-own-heading">
					{isSiteOwner ? "Create a campaign" : "Run your own campaign"}
				</h2>
			</div>
			<p className="flex-1 text-muted-foreground">{status}</p>
			<Button asChild className="sm:w-fit">
				<Link to="/request">
					{action}
					<ArrowRightIcon aria-hidden="true" />
				</Link>
			</Button>
		</section>
	);
}
