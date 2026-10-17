import { XIcon } from "lucide-react";
import { Dialog as DialogPrimitive } from "radix-ui";
import { type PointerEvent, type ReactNode, useRef } from "react";
import { useFocusReturn } from "@/hooks/use-focus-return";
import { cn } from "@/lib/utils";

// Past either of these the sheet closes when the finger lifts; short of them it springs back.
const dismissDistanceInPixels = 96;
const dismissVelocityInPixelsPerMillisecond = 0.6;
const dismissAnimationInMilliseconds = 200;
const phoneLayoutQuery = "(max-width: 767.98px)";

type DragState = { startY: number; startedAt: number; offset: number };

/**
 * A sheet that rises from the bottom on a phone and sits centred as a dialog on wider screens
 * (spec §6.1: the DM edits in place with dialogs and bottom sheets). Drag the handle down to close.
 * Closing hands focus back to the button that opened it.
 */
export function BottomSheet({
	open,
	onOpenChange,
	title,
	description,
	children,
	footer,
	className,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	title: ReactNode;
	description?: ReactNode;
	children?: ReactNode;
	/** Actions pinned under the scrolling body, such as Save and Cancel. */
	footer?: ReactNode;
	className?: string;
}) {
	const contentRef = useRef<HTMLDivElement>(null);
	const dragRef = useRef<DragState | null>(null);
	const focusReturn = useFocusReturn();

	function handlePointerDown(event: PointerEvent<HTMLDivElement>) {
		if (event.pointerType === "mouse" && event.button !== 0) {
			return;
		}
		if (!window.matchMedia(phoneLayoutQuery).matches) {
			return;
		}
		// Capturing the pointer would swallow taps on the close button.
		if (event.target instanceof Element && event.target.closest("button, a")) {
			return;
		}
		dragRef.current = { offset: 0, startedAt: event.timeStamp, startY: event.clientY };
		event.currentTarget.setPointerCapture(event.pointerId);
	}

	function handlePointerMove(event: PointerEvent<HTMLDivElement>) {
		const drag = dragRef.current;
		const content = contentRef.current;
		if (!(drag && content)) {
			return;
		}
		drag.offset = Math.max(0, event.clientY - drag.startY);
		content.style.transition = "none";
		content.style.transform = `translateY(${drag.offset}px)`;
	}

	function handlePointerEnd(event: PointerEvent<HTMLDivElement>) {
		const drag = dragRef.current;
		const content = contentRef.current;
		dragRef.current = null;
		if (!(drag && content)) {
			return;
		}
		const elapsed = Math.max(1, event.timeStamp - drag.startedAt);
		const isDismissed =
			drag.offset > dismissDistanceInPixels ||
			drag.offset / elapsed > dismissVelocityInPixelsPerMillisecond;
		content.style.transition = `transform ${dismissAnimationInMilliseconds}ms ease-out`;
		if (!isDismissed) {
			content.style.transform = "";
			return;
		}
		// Finish the slide from where the finger left it; the usual exit animation would restart
		// from the top and jump.
		content.style.transform = "translateY(100%)";
		content.dataset.dismissed = "drag";
		setTimeout(() => {
			onOpenChange(false);
		}, dismissAnimationInMilliseconds);
	}

	// Radix links the description itself; without one it needs an explicit undefined to stay quiet.
	const describedBy = description ? {} : { "aria-describedby": undefined };

	return (
		<DialogPrimitive.Root onOpenChange={onOpenChange} open={open}>
			<DialogPrimitive.Portal>
				<DialogPrimitive.Overlay className="fixed inset-0 z-50 bg-scrim backdrop-blur-[2px] data-[state=closed]:animate-out data-[state=open]:animate-in data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0" />
				<DialogPrimitive.Content
					{...describedBy}
					className={cn(
						"fixed inset-x-0 bottom-0 z-50 flex max-h-[88dvh] flex-col rounded-t-2xl border-border border-t bg-popover pb-safe text-popover-foreground shadow-float outline-none",
						"duration-300 data-[state=closed]:animate-out data-[state=open]:animate-in data-[state=closed]:slide-out-to-bottom data-[state=open]:slide-in-from-bottom data-[dismissed=drag]:animate-none",
						"md:inset-x-auto md:top-1/2 md:bottom-auto md:left-1/2 md:max-h-[min(44rem,calc(100dvh-4rem))] md:w-full md:max-w-lg md:-translate-x-1/2 md:-translate-y-1/2 md:rounded-2xl md:border md:pb-0 md:duration-200 md:data-[state=closed]:fade-out-0 md:data-[state=open]:fade-in-0 md:data-[state=closed]:zoom-out-95 md:data-[state=open]:zoom-in-95 md:data-[state=closed]:slide-out-to-bottom-2 md:data-[state=open]:slide-in-from-bottom-2",
						className,
					)}
					onCloseAutoFocus={focusReturn.onCloseAutoFocus}
					onFocus={focusReturn.onFocus}
					ref={contentRef}
				>
					<div
						className="flex shrink-0 cursor-grab touch-none flex-col active:cursor-grabbing md:cursor-auto md:touch-auto"
						onPointerCancel={handlePointerEnd}
						onPointerDown={handlePointerDown}
						onPointerMove={handlePointerMove}
						onPointerUp={handlePointerEnd}
					>
						<div aria-hidden="true" className="flex justify-center pt-3 pb-1 md:hidden">
							<span className="h-1.5 w-11 rounded-full bg-ink-muted/35" />
						</div>
						<div className="flex items-start gap-3 px-5 pt-2 pb-3 md:px-6 md:pt-6">
							<div className="min-w-0 flex-1 space-y-1">
								<DialogPrimitive.Title className="font-display font-semibold text-gilt-ink text-xl tracking-wide">
									{title}
								</DialogPrimitive.Title>
								{description ? (
									<DialogPrimitive.Description className="text-base text-muted-foreground">
										{description}
									</DialogPrimitive.Description>
								) : null}
							</div>
							<DialogPrimitive.Close className="-mt-1 -mr-2 inline-flex size-11 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/45">
								<XIcon className="size-5" />
								<span className="sr-only">Close</span>
							</DialogPrimitive.Close>
						</div>
					</div>
					{children ? (
						<div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-5 pb-5 md:px-6 md:pb-6">
							{children}
						</div>
					) : null}
					{footer ? (
						<div className="flex shrink-0 flex-col-reverse gap-2 border-border border-t px-5 py-4 sm:flex-row sm:justify-end md:px-6">
							{footer}
						</div>
					) : null}
				</DialogPrimitive.Content>
			</DialogPrimitive.Portal>
		</DialogPrimitive.Root>
	);
}
