import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
	ArrowRightIcon,
	CircleCheckIcon,
	CircleXIcon,
	HourglassIcon,
	SendIcon,
	SparklesIcon,
	Undo2Icon,
} from "lucide-react";
import { type FormEvent, useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { fieldErrors, problemMessage } from "@/api/problem";
import { myCampaignRequestQuery, myCampaignsQuery } from "@/api/queries";
import { Field, fieldInputProps } from "@/components/field";
import { PageHeader } from "@/components/page-header";
import { StatusPanel } from "@/components/status-panel";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useDocumentTitle } from "@/hooks/use-document-title";
import { useAccount } from "@/hooks/use-me";
import { slugFromName, slugProblem } from "@/lib/campaigns";
import { focusFirstVisible } from "@/lib/focus";
import { formatAbsoluteTime } from "@/lib/time";
import { cn } from "@/lib/utils";

type CampaignRequestView = Schemas["CampaignRequestView"];

const campaignNameLimit = 80;
const noteLimit = 500;
const formHeadingId = "request-form-heading";
const statusHeadingId = "request-status-heading";

// Asking to run a campaign (docs/design/multi-campaign.md §5.2): the form, then the request's
// status. One open request at a time; withdrawing it is how to change it.
export const Route = createFileRoute("/_signedIn/_account/request")({
	component: RequestPage,
	loader: ({ context }) => {
		context.queryClient.prefetchQuery(myCampaignRequestQuery);
	},
});

function RequestPage() {
	const { isSiteOwner } = useAccount();
	const title = isSiteOwner ? "Create a campaign" : "Run your own campaign";
	useDocumentTitle(title, "Colony Planner");
	const myRequest = useQuery(myCampaignRequestQuery);

	let body = (
		<div aria-busy="true" className="grid gap-4">
			<Skeleton className="h-40 rounded-2xl" />
		</div>
	);
	if (myRequest.isError) {
		body = (
			<StatusPanel
				description={problemMessage(myRequest.error, "Your request couldn't be loaded.")}
				icon={CircleXIcon}
				title="Your request couldn't be loaded"
				tone="ember"
			/>
		);
	} else if (myRequest.data) {
		const request = myRequest.data.request;
		body =
			request?.status === "PENDING" ? (
				<WaitingForOwner request={request} />
			) : (
				<>
					{request ? <Outcome request={request} /> : null}
					<RequestForm
						askingAgain={request !== null && request.status !== "WITHDRAWN"}
						isSiteOwner={isSiteOwner}
					/>
				</>
			);
	}

	return (
		<div className="mx-auto flex w-full max-w-2xl flex-col gap-8">
			<PageHeader
				description={
					isSiteOwner
						? "A new campaign of your own, ready at once. You're its DM: you invite your players and keep the colony's books."
						: "Ask the site owner for a campaign of your own. Once they approve it, you're its DM: you invite your players and keep the colony's books."
				}
				eyebrow="Colony Planner"
				title={title}
			/>
			{body}
		</div>
	);
}

function Outcome({ request }: { request: CampaignRequestView }) {
	if (request.status === "APPROVED" && request.campaign) {
		return (
			<section
				aria-labelledby={statusHeadingId}
				className="flex flex-col gap-4 rounded-2xl border border-moss/45 bg-moss/8 p-5 sm:flex-row sm:items-center sm:p-6"
			>
				<CircleCheckIcon aria-hidden="true" className="size-6 shrink-0 text-moss-ink" />
				<div className="min-w-0 flex-1">
					<h2
						className="font-display font-semibold text-xl tracking-wide outline-none"
						id={statusHeadingId}
						tabIndex={-1}
					>
						{request.campaign.name} is ready
					</h2>
					<p className="text-muted-foreground">
						You're its DM. Start in Settings: name your colony, pick your time zone, share your
						invite link.
					</p>
				</div>
				<Button asChild className="shrink-0">
					<Link params={{ campaignSlug: request.campaign.slug }} to="/c/$campaignSlug">
						Open it
						<ArrowRightIcon aria-hidden="true" />
					</Link>
				</Button>
			</section>
		);
	}
	if (request.status === "DENIED") {
		return (
			<section
				aria-labelledby={statusHeadingId}
				className="flex items-start gap-4 rounded-2xl border border-border bg-card p-5 shadow-card sm:p-6"
			>
				<CircleXIcon aria-hidden="true" className="mt-0.5 size-6 shrink-0 text-ember-ink" />
				<div className="min-w-0">
					<h2
						className="font-display font-semibold text-xl tracking-wide outline-none"
						id={statusHeadingId}
						tabIndex={-1}
					>
						The site owner said no this time
					</h2>
					<p className="text-muted-foreground">
						Your request for {request.campaignName} wasn't approved. You can ask again below, or ask
						the site owner about it on Discord.
					</p>
				</div>
			</section>
		);
	}
	return null;
}

function WaitingForOwner({ request }: { request: CampaignRequestView }) {
	const queryClient = useQueryClient();
	const withdraw = api.useMutation("post", "/api/campaign-requests/{campaignRequestId}/withdraw", {
		onError: (error) => {
			toast.error(problemMessage(error, "Your request couldn't be withdrawn."));
		},
		onSuccess: (withdrawn) => {
			queryClient.setQueryData(myCampaignRequestQuery.queryKey, { request: withdrawn });
			toast.success("Request withdrawn. Change it and ask again whenever you like.");
			// The status card, and its button, give way to the form once React has drawn it.
			requestAnimationFrame(() => {
				focusFirstVisible(formHeadingId);
			});
		},
	});
	return (
		<section
			aria-labelledby={statusHeadingId}
			className="flex flex-col gap-5 rounded-2xl border border-gilt/45 bg-card p-5 shadow-card sm:p-6"
		>
			<div className="flex items-start gap-4">
				<HourglassIcon aria-hidden="true" className="mt-1 size-6 shrink-0 text-gilt-ink" />
				<div className="min-w-0">
					<h2
						className="font-display font-semibold text-xl tracking-wide outline-none"
						id={statusHeadingId}
						tabIndex={-1}
					>
						Waiting for the site owner
					</h2>
					<p className="text-muted-foreground">
						They'll see your request the next time they look. This page shows their answer when you
						come back to it.
					</p>
				</div>
			</div>
			<dl className="grid gap-3 rounded-xl border border-border bg-surface-sunken/50 p-4 text-sm sm:grid-cols-[auto_1fr] sm:gap-x-6">
				<dt className="text-muted-foreground">Campaign</dt>
				<dd className="font-medium [overflow-wrap:anywhere]">{request.campaignName}</dd>
				<dt className="text-muted-foreground">Address</dt>
				<dd className="font-mono [overflow-wrap:anywhere]">/c/{request.slug}</dd>
				{request.note ? (
					<>
						<dt className="text-muted-foreground">Your note</dt>
						<dd className="whitespace-pre-line [overflow-wrap:anywhere]">{request.note}</dd>
					</>
				) : null}
				<dt className="text-muted-foreground">Asked</dt>
				<dd>{formatAbsoluteTime(request.createdAt)}</dd>
			</dl>
			<div>
				<Button
					disabled={withdraw.isPending}
					onClick={() => {
						withdraw.mutate({ params: { path: { campaignRequestId: request.id } } });
					}}
					variant="outline"
				>
					<Undo2Icon />
					{withdraw.isPending ? "Withdrawing…" : "Withdraw to change it"}
				</Button>
			</div>
		</section>
	);
}

function RequestForm({ askingAgain, isSiteOwner }: { askingAgain: boolean; isSiteOwner: boolean }) {
	const queryClient = useQueryClient();
	const [campaignName, setCampaignName] = useState("");
	const [slug, setSlug] = useState("");
	const [slugEdited, setSlugEdited] = useState(false);
	const [note, setNote] = useState("");
	const [showSlugProblem, setShowSlugProblem] = useState(false);

	const mutation = api.useMutation("post", "/api/campaign-requests", {
		onError: (error) => {
			if (Object.keys(fieldErrors(error)).length === 0) {
				toast.error(problemMessage(error, "Your request couldn't be sent."));
			}
		},
		onSuccess: async (request) => {
			queryClient.setQueryData(myCampaignRequestQuery.queryKey, { request });
			if (request.status === "APPROVED") {
				// The site owner's own request is approved on the spot.
				await queryClient.invalidateQueries({ queryKey: myCampaignsQuery.queryKey });
				toast.success(`${request.campaignName} is ready.`);
			} else {
				toast.success("Request sent to the site owner.");
			}
			requestAnimationFrame(() => {
				focusFirstVisible(statusHeadingId);
			});
		},
	});

	const address = slugEdited ? slug : slugFromName(campaignName);
	const serverErrors = fieldErrors(mutation.error);
	const localSlugProblem = showSlugProblem ? slugProblem(address) : undefined;
	const slugError = localSlugProblem ?? serverErrors.slug;
	const canSend = campaignName.trim() !== "" && !mutation.isPending;

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (slugProblem(address)) {
			setShowSlugProblem(true);
			return;
		}
		if (!canSend) {
			return;
		}
		mutation.mutate({
			body: { campaignName: campaignName.trim(), note: note.trim(), slug: address },
		});
	}

	return (
		<section
			aria-labelledby={formHeadingId}
			className="rounded-2xl border border-border bg-card shadow-card"
		>
			<div className="border-border border-b px-5 py-5 md:px-7">
				<h2
					className="font-display font-semibold text-xl tracking-wide outline-none"
					id={formHeadingId}
					tabIndex={-1}
				>
					{askingAgain && !isSiteOwner ? "Ask again" : "Your campaign"}
				</h2>
				{isSiteOwner ? null : (
					<p className="mt-1 max-w-prose text-muted-foreground text-sm">
						The site owner runs the server and Beny Bot. The app never shows them what happens in
						your campaign.
					</p>
				)}
			</div>
			<form className="grid gap-6 px-5 py-6 md:px-7" noValidate onSubmit={handleSubmit}>
				<Field
					description={
						isSiteOwner
							? "You can rename it later in Settings."
							: "You can rename it later in Settings. The site owner sees this name."
					}
					error={serverErrors.campaignName}
					id="request-campaign-name"
					label="Campaign name"
				>
					<Input
						{...fieldInputProps("request-campaign-name", serverErrors.campaignName, true)}
						autoComplete="off"
						maxLength={campaignNameLimit}
						onChange={(event) => {
							setCampaignName(event.target.value);
						}}
						placeholder="For example, Sky Isles"
						required
						value={campaignName}
					/>
				</Field>
				<Field
					description={
						isSiteOwner
							? "Your campaign's address on this site. It can't change later."
							: "Your campaign's address on this site. It can't change once approved."
					}
					error={slugError}
					id="request-slug"
					label="Address"
				>
					<div className="flex min-w-0 items-center rounded-md border border-input bg-surface-raised/70 focus-within:border-ring focus-within:ring-[3px] focus-within:ring-ring/35 dark:bg-surface-sunken/60">
						<span aria-hidden="true" className="pl-3 font-mono text-muted-foreground">
							/c/
						</span>
						<Input
							{...fieldInputProps("request-slug", slugError, true)}
							autoCapitalize="none"
							autoComplete="off"
							className="border-0 bg-transparent pl-0.5 font-mono shadow-none focus-visible:ring-0 dark:bg-transparent"
							maxLength={40}
							onChange={(event) => {
								setSlugEdited(true);
								setSlug(event.target.value.toLowerCase());
								setShowSlugProblem(false);
							}}
							placeholder="sky-isles"
							spellCheck={false}
							value={address}
						/>
					</div>
				</Field>
				{isSiteOwner ? null : (
					<Field
						description={`Up to ${noteLimit} characters, for the site owner only.`}
						error={serverErrors.note}
						id="request-note"
						label="A note to the site owner"
						optional
					>
						<textarea
							{...fieldInputProps("request-note", serverErrors.note, true)}
							className={cn(
								"min-h-24 w-full min-w-0 rounded-md border border-input bg-surface-raised/70 px-3 py-2 text-base text-foreground outline-none transition-[color,border-color,box-shadow] placeholder:text-muted-foreground/70 dark:bg-surface-sunken/60",
								"focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/35",
								"aria-invalid:border-ember-ink aria-invalid:ring-[3px] aria-invalid:ring-destructive/20",
							)}
							maxLength={noteLimit}
							onChange={(event) => {
								setNote(event.target.value);
							}}
							placeholder="Who you'll play with, what kind of campaign it is…"
							value={note}
						/>
					</Field>
				)}
				<div>
					<Button disabled={!canSend} type="submit">
						{isSiteOwner ? <SparklesIcon /> : <SendIcon />}
						{isSiteOwner ? createLabel(mutation.isPending) : sendLabel(mutation.isPending)}
					</Button>
				</div>
			</form>
		</section>
	);
}

function createLabel(pending: boolean): string {
	return pending ? "Creating…" : "Create campaign";
}

function sendLabel(pending: boolean): string {
	return pending ? "Sending…" : "Ask the site owner";
}
