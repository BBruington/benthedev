import { Microscope } from "lucide-react";
import { useId } from "react";
import type { DeconstructOdds, KnownValues } from "#/brewing/types";
import { Button } from "#/components/ui/button";
import { formatOdds } from "#/lib/format";

export function DeconstructButton({
	name,
	values,
	stock,
	deconstruct,
	pending,
	onDeconstruct,
}: {
	name: string;
	values: KnownValues;
	stock: number;
	deconstruct: DeconstructOdds;
	pending: boolean;
	onDeconstruct: () => void;
}) {
	const id = useId();
	const blocked =
		stock < 1
			? `No ${name} in stock to deconstruct`
			: values.every((value) => value !== null)
				? `Every tradition of ${name} is already known`
				: null;
	const reason =
		blocked ??
		`Spends 1 ${name} to learn a tradition. DC ${deconstruct.dc} · ${formatOdds(deconstruct.odds)}`;
	return (
		<>
			<Button
				size="xs"
				variant="ghost"
				className="aria-disabled:cursor-not-allowed aria-disabled:opacity-50"
				aria-disabled={blocked !== null || pending}
				aria-describedby={id}
				title={reason}
				onClick={() => {
					if (!blocked && !pending) onDeconstruct();
				}}
			>
				<Microscope aria-hidden="true" />
				Deconstruct
			</Button>
			<span id={id} className="sr-only">
				{reason}
			</span>
		</>
	);
}
