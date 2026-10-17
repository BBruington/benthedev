// Browser storage can be missing or throw (private windows, blocked site data), and nothing here
// is worth failing a page over, so every read and write tolerates that.

export function readStored(storage: "local" | "session", key: string): string | null {
	try {
		return pickStorage(storage).getItem(key);
	} catch {
		return null;
	}
}

export function writeStored(storage: "local" | "session", key: string, value: string | null) {
	try {
		if (value === null) {
			pickStorage(storage).removeItem(key);
			return;
		}
		pickStorage(storage).setItem(key, value);
	} catch {
		// The preference simply isn't remembered.
	}
}

function pickStorage(storage: "local" | "session"): Storage {
	if (storage === "local") {
		return window.localStorage;
	}
	return window.sessionStorage;
}
