import { CheckIcon, ExternalLinkIcon } from "lucide-react";
import { type FormEvent, type ReactNode, useState } from "react";
import type { Schemas } from "@/api/client";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { formatNumber } from "@/lib/colony";
import { optionShare, sameChoice, votersText } from "@/lib/polls";
import { cn } from "@/lib/utils";

type Poll = Schemas["Poll"];
type PollOption = Schemas["PollOption"];

function OptionTally({
	option,
	poll,
	isMine,
	showVoters,
}: {
	option: PollOption;
	poll: Poll;
	isMine: boolean;
	showVoters: boolean;
}) {
	const share = optionShare(option.count, poll.voterCount);
	const voters = showVoters ? votersText(option) : null;
	return (
		<span className="grid min-w-0 flex-1 gap-1.5">
			<span className="flex items-baseline justify-between gap-3">
				<span className="min-w-0 break-words font-medium">
					{option.label}
					{isMine ? (
						<Badge className="ml-2 align-middle" variant="gilt">
							<CheckIcon aria-hidden="true" />
							Your vote
						</Badge>
					) : null}
				</span>
				<span className="shrink-0 text-muted-foreground text-sm tabular-nums">
					{formatNumber(option.count)}
				</span>
			</span>
			<span aria-hidden="true" className="h-1.5 overflow-hidden rounded-full bg-muted">
				<span
					className="block h-full rounded-full bg-gilt transition-[width] duration-300 ease-out"
					style={{ width: `${share * 100}%` }}
				/>
			</span>
			{voters ? <span className="text-muted-foreground text-sm">{voters}</span> : null}
		</span>
	);
}

/** An open Discord poll's way to vote: the link once Beny Bot has posted it, a waiting line before. */
export function DiscordVoteLink({ poll }: { poll: Poll }) {
	if (!poll.discordUrl) {
		return (
			<p className="text-muted-foreground text-sm">Waiting for Beny Bot to post this in Discord.</p>
		);
	}
	return (
		<div className="flex justify-end">
			<Button asChild className="w-full sm:w-auto">
				<a href={poll.discordUrl} rel="noopener noreferrer" target="_blank">
					Vote in Discord
					<ExternalLinkIcon aria-hidden="true" />
				</a>
			</Button>
		</div>
	);
}

/** Counts and voters, for the DM, a closed poll, or a player's board line once they've voted. */
export function PollResults({ poll }: { poll: Poll }) {
	return (
		<ul className="grid gap-3">
			{poll.options.map((option) => (
				<li className="flex" key={option.id}>
					<OptionTally
						isMine={poll.myOptionIds.includes(option.id)}
						option={option}
						poll={poll}
						showVoters
					/>
				</li>
			))}
		</ul>
	);
}

/**
 * A player's vote: radio buttons for a single-answer poll, checkboxes for several. Saving sends the
 * whole choice, and Withdraw sends none. Key it on the saved vote so another device's change resets it.
 */
export function PollVoteForm({
	poll,
	labelledBy,
	isSaving,
	showVoters = true,
	onVote,
	children,
}: {
	poll: Poll;
	labelledBy: string;
	isSaving: boolean;
	showVoters?: boolean;
	onVote: (optionIds: string[]) => void;
	/** Shown beside the buttons, such as the close time. */
	children?: ReactNode;
}) {
	const [chosen, setChosen] = useState<string[]>(poll.myOptionIds);
	const hasVoted = poll.myOptionIds.length > 0;
	const canSave = chosen.length > 0 && !sameChoice(chosen, poll.myOptionIds) && !isSaving;

	function toggle(optionId: string) {
		if (!poll.allowMultiple) {
			setChosen([optionId]);
			return;
		}
		setChosen((current) =>
			current.includes(optionId)
				? current.filter((chosenId) => chosenId !== optionId)
				: [...current, optionId],
		);
	}

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (canSave) {
			onVote(chosen);
		}
	}

	return (
		<form className="grid gap-4" onSubmit={handleSubmit}>
			<fieldset aria-labelledby={labelledBy} className="grid gap-2" disabled={isSaving}>
				{poll.options.map((option) => {
					const isChosen = chosen.includes(option.id);
					return (
						<label
							className={cn(
								"flex min-h-11 cursor-pointer items-start gap-3 rounded-lg border px-3 py-3 transition-colors has-[:focus-visible]:ring-[3px] has-[:focus-visible]:ring-ring/45",
								isChosen ? "border-gilt bg-gilt/10" : "border-border hover:bg-muted/50",
							)}
							key={option.id}
						>
							<input
								checked={isChosen}
								className="mt-1 size-4 shrink-0 accent-gilt"
								name={`poll-${poll.id}`}
								onChange={() => {
									toggle(option.id);
								}}
								type={poll.allowMultiple ? "checkbox" : "radio"}
								value={option.id}
							/>
							<OptionTally
								isMine={poll.myOptionIds.includes(option.id)}
								option={option}
								poll={poll}
								showVoters={showVoters}
							/>
						</label>
					);
				})}
			</fieldset>
			<div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
				{children}
				<div className="flex w-full flex-col-reverse gap-2 sm:ml-auto sm:w-auto sm:flex-row">
					{hasVoted ? (
						<Button
							disabled={isSaving}
							onClick={() => {
								onVote([]);
							}}
							type="button"
							variant="ghost"
						>
							Withdraw vote
						</Button>
					) : null}
					<Button disabled={!canSave} type="submit">
						{hasVoted ? "Change vote" : "Vote"}
					</Button>
				</div>
			</div>
		</form>
	);
}
