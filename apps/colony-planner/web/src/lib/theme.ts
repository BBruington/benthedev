import { useSyncExternalStore } from "react";
import { readStored, writeStored } from "./storage";

/** "device" follows prefers-color-scheme; the other two force a theme with data-theme on <html>. */
export type ThemePreference = "device" | "light" | "dark";

// index.html reads the same key before first paint, so a forced theme never flashes.
const themeStorageKey = "colony-planner:theme";
const themeColors = { dark: "#0E1A1D", light: "#F3EAD6" } as const;

const themeListeners = new Set<() => void>();

function parsePreference(value: string | null): ThemePreference {
	if (value === "light" || value === "dark") {
		return value;
	}
	return "device";
}

let currentPreference: ThemePreference = parsePreference(readStored("local", themeStorageKey));

export function applyThemePreference(preference: ThemePreference) {
	const root = document.documentElement;
	if (preference === "device") {
		root.removeAttribute("data-theme");
	} else {
		root.setAttribute("data-theme", preference);
	}
	for (const meta of document.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]')) {
		const scheme = meta.dataset.scheme === "dark" ? "dark" : "light";
		if (preference === "device") {
			meta.content = themeColors[scheme];
		} else {
			meta.content = themeColors[preference];
		}
	}
}

export function setThemePreference(preference: ThemePreference) {
	currentPreference = preference;
	writeStored("local", themeStorageKey, preference === "device" ? null : preference);
	applyThemePreference(preference);
	for (const listener of themeListeners) {
		listener();
	}
}

function subscribeToTheme(listener: () => void) {
	themeListeners.add(listener);
	return () => {
		themeListeners.delete(listener);
	};
}

export function readThemePreference(): ThemePreference {
	return currentPreference;
}

export function useThemePreference(): ThemePreference {
	return useSyncExternalStore(subscribeToTheme, readThemePreference, readThemePreference);
}

const darkSchemeQuery = "(prefers-color-scheme: dark)";

function subscribeToDeviceScheme(listener: () => void) {
	const media = window.matchMedia(darkSchemeQuery);
	media.addEventListener("change", listener);
	return () => {
		media.removeEventListener("change", listener);
	};
}

/** Calls listener whenever the theme on screen may have changed: a new preference or device scheme. */
export function subscribeToResolvedTheme(listener: () => void) {
	const stopWatchingPreference = subscribeToTheme(listener);
	const stopWatchingDevice = subscribeToDeviceScheme(listener);
	return () => {
		stopWatchingPreference();
		stopWatchingDevice();
	};
}

function readDeviceSchemeIsDark() {
	return window.matchMedia(darkSchemeQuery).matches;
}

/** The theme actually on screen, for the few components (toasts) that need to be told it. */
export function useResolvedTheme(): "light" | "dark" {
	const preference = useThemePreference();
	const deviceIsDark = useSyncExternalStore(
		subscribeToDeviceScheme,
		readDeviceSchemeIsDark,
		() => false,
	);
	if (preference !== "device") {
		return preference;
	}
	return deviceIsDark ? "dark" : "light";
}
