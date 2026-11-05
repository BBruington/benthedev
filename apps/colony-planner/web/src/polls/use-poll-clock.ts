import { useEffect, useState } from "react";

/**
 * The time polls are judged against, refreshed every 30 seconds, so close times read "in 5 minutes"
 * and a poll past its time stops taking votes without a refetch.
 */
export function usePollClock() {
	const [now, setNow] = useState(() => new Date());
	useEffect(() => {
		const timer = setInterval(() => {
			setNow(new Date());
		}, 30_000);
		return () => {
			clearInterval(timer);
		};
	}, []);
	return now;
}
