import { type FocusEvent, useRef } from "react";

/**
 * The element focus should go back to after a dialog that `focused` opened. A menu item is gone by
 * the time the dialog closes, so a dialog opened from a menu returns to the button that opened the
 * menu (Radix labels a menu by its trigger).
 */
function returnTarget(focused: Element | null): HTMLElement | null {
	if (!(focused instanceof HTMLElement) || focused === document.body) {
		return null;
	}
	const menu = focused.closest('[role="menu"]');
	const menuTriggerId = menu?.getAttribute("aria-labelledby");
	if (menuTriggerId) {
		return document.getElementById(menuTriggerId);
	}
	return focused;
}

function canTakeFocus(element: HTMLElement): boolean {
	return (
		element.isConnected && element.getClientRects().length > 0 && !element.matches(":disabled")
	);
}

/**
 * Returns focus to whatever opened a dialog or sheet once it closes. Radix only returns focus to its
 * own Trigger, and these open from ordinary buttons and menu items, so without this a keyboard user
 * lands at the top of the page after every Cancel, Escape or Save. Spread the handlers onto the Radix
 * Content. A caller that already moved focus on purpose (Undo to the day heading) keeps its target.
 */
export function useFocusReturn() {
	const openerRef = useRef<HTMLElement | null>(null);
	return {
		onCloseAutoFocus: (event: Event) => {
			event.preventDefault();
			const opener = openerRef.current;
			openerRef.current = null;
			const focused = document.activeElement;
			if (focused && focused !== document.body) {
				return;
			}
			if (opener && canTakeFocus(opener)) {
				opener.focus();
			}
		},
		// The first focus into the dialog arrives from its opener. Radix's onOpenAutoFocus would be too
		// late: a field with autoFocus takes focus first, and Radix then skips that event.
		onFocus: (event: FocusEvent<HTMLElement>) => {
			const cameFrom = event.relatedTarget;
			if (
				openerRef.current !== null ||
				!(cameFrom instanceof HTMLElement) ||
				event.currentTarget.contains(cameFrom)
			) {
				return;
			}
			openerRef.current = returnTarget(cameFrom);
		},
	};
}
