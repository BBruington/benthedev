import { EyeIcon } from "lucide-react";
import { useEffect, useRef } from "react";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useViewAsPlayer } from "@/hooks/use-view-as-player";
import type { Me } from "@/lib/access";
import { focusFirstVisible } from "@/lib/focus";

const viewAsPlayerButtonId = "view-as-player-button";
const viewAsPlayerSwitchId = "view-as-player-switch";

/** The DM's switch in the top bar: an eye on phones, a labelled switch on wider screens. */
export function ViewAsPlayerToggle({ me }: { me: Me }) {
	const { setViewAsPlayer, isSwitching } = useViewAsPlayer();
	return (
		<>
			<Tooltip>
				<TooltipTrigger asChild>
					<Button
						aria-label="View as player"
						aria-pressed={me.viewAsPlayer}
						className="aria-pressed:bg-verdigris/20 aria-pressed:text-verdigris-ink md:hidden"
						disabled={isSwitching}
						id={viewAsPlayerButtonId}
						onClick={() => {
							setViewAsPlayer(!me.viewAsPlayer);
						}}
						size="icon"
						variant="ghost"
					>
						<EyeIcon />
					</Button>
				</TooltipTrigger>
				<TooltipContent>
					{me.viewAsPlayer ? "Back to the DM view" : "View as player"}
				</TooltipContent>
			</Tooltip>
			<label
				className="hidden h-11 cursor-pointer select-none items-center gap-2.5 rounded-md px-3 text-muted-foreground text-sm transition-colors hover:bg-accent hover:text-foreground md:inline-flex"
				htmlFor={viewAsPlayerSwitchId}
			>
				<Switch
					checked={me.viewAsPlayer}
					disabled={isSwitching}
					id={viewAsPlayerSwitchId}
					onCheckedChange={setViewAsPlayer}
				/>
				View as player
			</label>
		</>
	);
}

/** Shown under the top bar for as long as view as player is on (spec §2.1). */
export function ViewAsPlayerBanner() {
	const { setViewAsPlayer, isSwitching } = useViewAsPlayer();
	// Back to DM view removes this banner and its button, so focus goes to the top bar's switch (the
	// eye on phones), which is back in the page by the time the banner unmounts.
	const switchingBack = useRef(false);
	useEffect(
		() => () => {
			if (switchingBack.current) {
				focusFirstVisible(viewAsPlayerButtonId, viewAsPlayerSwitchId);
			}
		},
		[],
	);
	return (
		<div className="border-verdigris/35 border-t bg-verdigris/12" role="status">
			<div className="flex items-center gap-3 px-4 py-2 md:px-6">
				<span
					aria-hidden="true"
					className="hidden size-8 shrink-0 items-center justify-center rounded-full bg-verdigris/20 text-verdigris-ink sm:inline-flex"
				>
					<EyeIcon className="size-4" />
				</span>
				<p className="min-w-0 flex-1 text-sm">
					<span className="font-semibold text-verdigris-ink">Viewing as a player.</span>{" "}
					<span className="text-muted-foreground">
						You see what the table sees; DM tools and secrets are hidden.
					</span>
				</p>
				<Button
					className="shrink-0"
					disabled={isSwitching}
					onClick={() => {
						switchingBack.current = true;
						setViewAsPlayer(false);
					}}
					size="sm"
					variant="outline"
				>
					Back to DM view
				</Button>
			</div>
		</div>
	);
}
