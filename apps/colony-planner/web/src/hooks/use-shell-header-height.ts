import { type RefObject, useEffect } from "react";

/**
 * Publishes the sticky header's height as --shell-header-height, so the sidebar can stick just
 * below it whether or not the View as player banner is showing.
 */
export function useShellHeaderHeight(headerRef: RefObject<HTMLElement | null>) {
	useEffect(() => {
		const header = headerRef.current;
		if (!header) {
			return;
		}
		const root = document.documentElement;
		const observer = new ResizeObserver(() => {
			root.style.setProperty("--shell-header-height", `${header.offsetHeight}px`);
		});
		observer.observe(header);
		return () => {
			observer.disconnect();
			root.style.removeProperty("--shell-header-height");
		};
	}, [headerRef]);
}
