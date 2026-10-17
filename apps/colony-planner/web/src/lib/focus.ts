/**
 * Moves keyboard focus to the first of these elements that is on screen. An action that removes its
 * own button (Let in moves the row, Back to DM view hides the banner) calls this, so focus lands
 * somewhere stable instead of falling back to the top of the page. A non-interactive target, such as
 * a section heading, needs tabIndex={-1}.
 */
export function focusFirstVisible(...elementIds: string[]): void {
	for (const elementId of elementIds) {
		const element = document.getElementById(elementId);
		if (element && element.getClientRects().length > 0) {
			element.focus();
			return;
		}
	}
}
