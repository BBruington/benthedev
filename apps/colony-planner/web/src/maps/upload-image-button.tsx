import { useMutation } from "@tanstack/react-query";
import { ImageUpIcon, LoaderCircleIcon } from "lucide-react";
import { type ChangeEvent, useId, useRef, useState } from "react";
import type { Schemas } from "@/api/client";
import { uploadImageMutation } from "@/api/images";
import { Button } from "@/components/ui/button";
import { useCampaignId } from "@/hooks/use-campaign";
import { cn } from "@/lib/utils";
import { imageFileProblem, imageInputAccept, uploadFailureMessage } from "./uploads";

/**
 * Picks an image and uploads it (DM only), showing "Uploading…" while it goes and a plain reason
 * under the button when the file or the server refuses it. The caller decides what the image is for.
 */
export function UploadImageButton({
	label,
	onUploaded,
	busy = false,
	busyLabel = "Saving…",
	variant = "outline",
	className,
	id,
}: {
	label: string;
	onUploaded: (image: Schemas["UploadedImage"]) => void;
	/** The caller's own follow-up save is running, such as setting the uploaded art on a map. */
	busy?: boolean;
	busyLabel?: string;
	variant?: "default" | "outline";
	className?: string;
	/** For moving focus here after a control next to it goes away. */
	id?: string;
}) {
	const campaignId = useCampaignId();
	const inputRef = useRef<HTMLInputElement>(null);
	const messageId = useId();
	const [problem, setProblem] = useState<string | null>(null);
	const upload = useMutation(uploadImageMutation(campaignId));

	function handleChange(event: ChangeEvent<HTMLInputElement>) {
		const file = event.target.files?.[0];
		// Cleared, so choosing the same file again after fixing it still fires a change.
		event.target.value = "";
		if (!file) {
			return;
		}
		const fileProblem = imageFileProblem(file);
		setProblem(fileProblem);
		if (fileProblem) {
			return;
		}
		upload.mutate(file, {
			onError: (error) => {
				setProblem(uploadFailureMessage(error));
			},
			onSuccess: onUploaded,
		});
	}

	let buttonLabel = label;
	if (upload.isPending) {
		buttonLabel = "Uploading…";
	} else if (busy) {
		buttonLabel = busyLabel;
	}
	const isWorking = upload.isPending || busy;
	return (
		<div className={cn("grid content-start gap-1.5", className)}>
			<input
				accept={imageInputAccept}
				aria-hidden="true"
				className="hidden"
				onChange={handleChange}
				ref={inputRef}
				tabIndex={-1}
				type="file"
			/>
			<Button
				aria-describedby={problem ? messageId : undefined}
				disabled={isWorking}
				id={id}
				onClick={() => {
					inputRef.current?.click();
				}}
				type="button"
				variant={variant}
			>
				{isWorking ? <LoaderCircleIcon className="motion-safe:animate-spin" /> : <ImageUpIcon />}
				{buttonLabel}
			</Button>
			<p aria-live="polite" className="text-ember-ink text-sm" id={messageId}>
				{problem}
			</p>
		</div>
	);
}
