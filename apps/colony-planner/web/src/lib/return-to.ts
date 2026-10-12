import { readStored, writeStored } from "./storage";

// Sign-in always lands on "/", so a deep link opened while signed out (a Beny Bot ping linking to
// /c/{slug}/players, or an invite link) is remembered here for the length of the tab and resumed
// after sign-in.
const returnToStorageKey = "colony-planner:return-to";

export function rememberReturnTo(path: string) {
	if (isResumablePath(path)) {
		writeStored("session", returnToStorageKey, path);
	}
}

/** The remembered path, once; it is forgotten as it is read. */
export function takeReturnTo(): string | undefined {
	const path = readStored("session", returnToStorageKey);
	writeStored("session", returnToStorageKey, null);
	if (path && isResumablePath(path)) {
		return path;
	}
	return undefined;
}

function isResumablePath(path: string): boolean {
	if (!path.startsWith("/") || path.startsWith("//")) {
		return false;
	}
	return path !== "/" && !path.startsWith("/login");
}
