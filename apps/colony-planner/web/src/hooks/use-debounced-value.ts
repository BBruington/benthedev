import { useEffect, useState } from "react";

/** The value once it has stopped changing for `delayInMilliseconds`, so typing doesn't spam previews. */
export function useDebouncedValue<Value>(value: Value, delayInMilliseconds: number): Value {
	const [settled, setSettled] = useState(value);
	useEffect(() => {
		const timer = setTimeout(() => {
			setSettled(value);
		}, delayInMilliseconds);
		return () => {
			clearTimeout(timer);
		};
	}, [value, delayInMilliseconds]);
	return settled;
}
