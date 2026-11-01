export function DevBadge() {
	if (!import.meta.env.VITE_CLERK_PUBLISHABLE_KEY?.startsWith("pk_test_")) {
		return null;
	}

	return (
		<span
			className="whitespace-nowrap rounded-full border border-primary/40 px-2.5 py-1 text-xs font-semibold text-primary"
			title="This sandbox uses a Clerk development instance."
		>
			Dev sandbox
		</span>
	);
}
