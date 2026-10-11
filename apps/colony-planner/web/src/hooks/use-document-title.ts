import { useEffect } from "react";

export function useDocumentTitle(...parts: (string | undefined)[]) {
	const title = parts.filter(Boolean).join(" · ");
	useEffect(() => {
		document.title = title;
	}, [title]);
}
