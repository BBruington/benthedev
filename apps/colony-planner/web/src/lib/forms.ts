/** The part of a save mutation that `forgetFailedSaves` needs. */
export type ResettableSave = { isError: boolean; reset: () => void };

/**
 * Forgets failed saves once the person edits the form, so a server field error stops showing and
 * blocking Save as soon as they correct it. A pending save is left alone: resetting it would drop
 * its success callback, and the sheet would never close.
 */
export function forgetFailedSaves(...saves: ResettableSave[]) {
	for (const save of saves) {
		if (save.isError) {
			save.reset();
		}
	}
}
