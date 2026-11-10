import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useEffect, useRef } from "react";
import { toast } from "sonner";
import { requestsQuery } from "@/api/queries";

const toastDurationInMilliseconds = 12_000;

/** The DM's pending requests, for the top bar's badge, the board and the request toasts. */
export function pendingRequestsQuery(campaignId: string) {
	return requestsQuery(campaignId, { status: "PENDING" });
}

/**
 * Toasts each request that arrives while the DM has the campaign open, with Review opening the
 * approval sheet (spec §6.5: one tap from the toast). Requests already waiting when the tab opened
 * are on the board and the badge, not news. Only for the DM outside view as player.
 */
export function useRequestToasts(campaignId: string, slug: string, isEnabled: boolean) {
	const navigate = useNavigate();
	const { data } = useQuery({ ...pendingRequestsQuery(campaignId), enabled: isEnabled });
	// Keyed by campaign, so another campaign's waiting requests are never news here.
	const seen = useRef<{ campaignId: string; requestIds: Set<string> } | null>(null);

	useEffect(() => {
		if (!(data && isEnabled)) {
			return;
		}
		if (seen.current?.campaignId !== campaignId) {
			seen.current = {
				campaignId,
				requestIds: new Set(data.requests.map((request) => request.id)),
			};
			return;
		}
		for (const request of data.requests) {
			if (seen.current.requestIds.has(request.id)) {
				continue;
			}
			seen.current.requestIds.add(request.id);
			const requester = request.requestedBy?.name ?? "A player";
			toast(`${requester} requests ${request.title}`, {
				action: {
					label: "Review",
					onClick: () => {
						navigate({
							params: { campaignSlug: slug },
							search: { review: request.id },
							to: "/c/$campaignSlug/requests",
						});
					},
				},
				description: `From ${request.patronName}${request.note ? `: “${request.note}”` : ""}`,
				duration: toastDurationInMilliseconds,
			});
		}
	}, [campaignId, data, isEnabled, navigate, slug]);
}
