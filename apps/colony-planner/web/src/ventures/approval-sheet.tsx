import { useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckIcon, ChevronDownIcon, CircleCheckIcon, CircleXIcon, XIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { api, type Schemas } from "@/api/client";
import { problemMessage } from "@/api/problem";
import {
	approveRequestPreviewQuery,
	colonyBoardQuery,
	membersQuery,
	patronQueryKeyPrefix,
	requestQueryKeyPrefix,
	requestsQueryKeyPrefix,
} from "@/api/queries";
import { type ColonyChanges, isConflict } from "@/colony/use-colony-changes";
import { BottomSheet } from "@/components/bottom-sheet";
import { Field } from "@/components/field";
import { TextArea } from "@/components/form-controls";
import { MemberAvatar } from "@/components/member-avatar";
import { Button } from "@/components/ui/button";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { formatNumber } from "@/lib/colony";
import { forgetFailedSaves } from "@/lib/forms";
import { draftToEffects, effectsToDraft, shortfallAdvice } from "@/lib/ventures";
import { ChangePreview, costLine, EffectsEditor, RequestStatusBadge } from "./venture-parts";

type Request = Schemas["Request"];

const previewDelayInMilliseconds = 350;
const noteMaxLength = 500;

/**
 * The DM's approval sheet (spec §6.5): full-screen on a phone. Who asked and why, the cost and
 * effects, before → after, whether it's affordable now, an optional tweak for this grant only, a note
 * to the player, and Grant or Decline. A grant that has fallen short (409) keeps the sheet open with
 * the shortfall named, so the DM can adjust favor or tweak and try again.
 */
export function ApprovalSheet({
	open,
	onOpenChange,
	request,
	changes,
	onDecided,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	request: Request;
	changes: ColonyChanges;
	onDecided: () => void;
}) {
	const queryClient = useQueryClient();
	const campaignId = changes.inCampaign.params.path.campaignId;
	const board = useQuery(colonyBoardQuery(campaignId));
	const members = useQuery(membersQuery(campaignId));
	const resources = board.data?.resources ?? [];
	const [isTweaking, setTweaking] = useState(false);
	const [tweakDraft, setTweakDraft] = useState(() => effectsToDraft(request.effects));
	const [note, setNote] = useState("");
	const tweak = draftToEffects(tweakDraft);
	const tweakIsValid = Object.keys(tweak.problems).length === 0;
	const appliedEffects = isTweaking && tweakIsValid ? tweak.effects : undefined;
	const previewInput = useDebouncedValue(
		appliedEffects ? { appliedEffects, note: "" } : { note: "" },
		previewDelayInMilliseconds,
	);
	const isPending = request.status === "PENDING";
	const preview = useQuery({
		...approveRequestPreviewQuery(campaignId, request.id, previewInput),
		enabled: open && isPending,
	});
	const grant = changes.approveRequest;
	const decline = api.useMutation(
		"post",
		"/api/campaigns/{campaignId}/requests/{requestId}/decline",
		{
			onSettled: () =>
				Promise.all([
					queryClient.invalidateQueries({ queryKey: requestsQueryKeyPrefix(campaignId) }),
					queryClient.invalidateQueries({ queryKey: requestQueryKeyPrefix(campaignId) }),
					queryClient.invalidateQueries({ queryKey: patronQueryKeyPrefix(campaignId) }),
				]),
		},
	);
	const isDeciding = grant.isPending || decline.isPending;
	const requester = request.onBehalfOf ?? request.requestedBy;
	const requesterMember = (members.data?.active ?? []).find(
		(member) => member.id === requester?.memberId,
	);
	const error = grant.error ?? decline.error;
	const errorMessage = problemMessage(error, "That request couldn't be decided.");
	const shortfallHint = isConflict(error) && grant.error ? shortfallAdvice(errorMessage) : null;

	function handleGrant() {
		grant.mutate(
			{
				body: appliedEffects ? { appliedEffects, note: note.trim() } : { note: note.trim() },
				params: { path: { campaignId, requestId: request.id } },
			},
			{
				onSuccess: () => {
					onOpenChange(false);
					onDecided();
				},
			},
		);
	}

	function handleDecline() {
		decline.mutate(
			{ body: { note: note.trim() }, params: { path: { campaignId, requestId: request.id } } },
			{
				onSuccess: (declined) => {
					toast.success(`Declined ${declined.title}.`);
					onOpenChange(false);
					onDecided();
				},
			},
		);
	}

	return (
		<BottomSheet
			className="h-dvh max-h-dvh rounded-t-none border-t-0 md:h-auto md:max-w-xl md:border-t"
			footer={
				isPending ? (
					<>
						<Button
							className="h-12 text-base sm:flex-1"
							disabled={isDeciding}
							onClick={handleDecline}
							size="lg"
							variant="outline"
						>
							<XIcon />
							{decline.isPending ? "Declining…" : "Decline"}
						</Button>
						<Button
							className="h-12 text-base sm:flex-1"
							disabled={isDeciding || (isTweaking && !tweakIsValid)}
							onClick={handleGrant}
							size="lg"
						>
							<CheckIcon />
							{grant.isPending ? "Granting…" : "Grant"}
						</Button>
					</>
				) : null
			}
			onOpenChange={(next) => {
				if (!next) {
					grant.reset();
				}
				onOpenChange(next);
			}}
			open={open}
			title={request.title}
		>
			<div className="grid gap-5">
				<div className="flex items-start gap-3">
					<MemberAvatar
						avatarUrl={requesterMember?.avatarUrl ?? null}
						name={requester?.name ?? "The DM"}
					/>
					<div className="min-w-0 flex-1">
						<p className="font-semibold [overflow-wrap:anywhere]">
							{requester?.name ?? "The DM"}
							<span className="font-normal text-muted-foreground">
								{" "}
								asks {request.patronName} · Day {formatNumber(request.requestedDay)}
							</span>
						</p>
						{request.note ? (
							<p className="mt-1 font-flavor text-lg italic [overflow-wrap:anywhere]">
								“{request.note}”
							</p>
						) : null}
					</div>
					{isPending ? null : <RequestStatusBadge status={request.status} />}
				</div>

				<p className="rounded-lg bg-surface-sunken/60 px-3 py-2 tabular-nums">
					{costLine(request.favorCost, request.effects, resources)}
				</p>

				{isPending ? (
					<>
						{/* A grant that can't happen has no before → after; the shortfall line says why. */}
						{preview.data?.affordable === false ? null : (
							<ChangePreview
								emptyText="It changes nothing in the stockpile."
								error={preview.error}
								isPending={preview.isPending}
								preview={preview.data}
							/>
						)}
						<ValidityLine preview={preview.data} />
						<section className="grid gap-3 rounded-lg border border-border">
							<button
								aria-expanded={isTweaking}
								className="flex min-h-11 cursor-pointer items-center justify-between gap-3 rounded-lg px-3 text-left font-medium outline-none focus-visible:ring-[3px] focus-visible:ring-ring/45"
								onClick={() => {
									setTweaking((current) => !current);
									forgetFailedSaves(grant, decline);
								}}
								type="button"
							>
								<span>
									Tweak this grant
									<span className="block font-normal text-muted-foreground text-sm">
										Changes the effects for this grant only; the action stays as it is.
									</span>
								</span>
								<ChevronDownIcon
									aria-hidden="true"
									className={isTweaking ? "size-5 rotate-180" : "size-5"}
								/>
							</button>
							{isTweaking ? (
								<div className="px-3 pb-3">
									<EffectsEditor
										draft={tweakDraft}
										idPrefix="grant-tweak"
										onChange={(next) => {
											setTweakDraft(next);
											forgetFailedSaves(grant, decline);
										}}
										problems={tweak.problems}
										resources={resources}
									/>
								</div>
							) : null}
						</section>
						<Field
							description="Only they and you see it."
							id="approval-note"
							label="Note to the player"
							optional
						>
							<TextArea
								aria-describedby="approval-note-description"
								id="approval-note"
								maxLength={noteMaxLength}
								onChange={(event) => {
									setNote(event.target.value);
									forgetFailedSaves(grant, decline);
								}}
								value={note}
							/>
						</Field>
						{error ? (
							<p
								className="rounded-lg border border-ember/40 bg-ember/10 px-3 py-2 text-ember-ink text-sm"
								role="alert"
							>
								{errorMessage}
								{shortfallHint ? ` ${shortfallHint}` : null}
							</p>
						) : null}
					</>
				) : (
					<p className="text-muted-foreground">This request has already been decided.</p>
				)}
			</div>
		</BottomSheet>
	);
}

function ValidityLine({ preview }: { preview: Schemas["RequestApprovalPreview"] | undefined }) {
	if (!preview) {
		return null;
	}
	if (preview.affordable) {
		return (
			<p className="inline-flex items-center gap-2 font-semibold text-moss-ink">
				<CircleCheckIcon aria-hidden="true" className="size-5" />
				Affordable
			</p>
		);
	}
	return (
		<p className="inline-flex items-start gap-2 font-semibold text-ember-ink">
			<CircleXIcon aria-hidden="true" className="mt-0.5 size-5 shrink-0" />
			{preview.shortfall}
		</p>
	);
}
