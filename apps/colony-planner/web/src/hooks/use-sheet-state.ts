import { useState } from "react";

/**
 * Open state for a sheet that edits one thing. The value outlives closing so the sheet keeps its
 * text while it animates away, and each opening gets a new session number: keying the sheet on it
 * starts every opening with a fresh form.
 */
export function useSheetState<Value>() {
	const [state, setState] = useState<{ open: boolean; value: Value | null; session: number }>({
		open: false,
		session: 0,
		value: null,
	});
	return {
		...state,
		onOpenChange: (open: boolean) => {
			setState((current) => ({ ...current, open }));
		},
		show: (value: Value) => {
			setState((current) => ({ open: true, session: current.session + 1, value }));
		},
	};
}
