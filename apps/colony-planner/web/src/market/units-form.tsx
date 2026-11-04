import { type FormEvent, type ReactNode, useId, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { formatNumber } from "@/lib/colony";
import { lotSize, parseUnits } from "@/lib/market";

type Sold = {
	name: string;
	priceGold: number;
	resourceKey: string | null;
	amount: number | null;
	roleKey: string | null;
};

/**
 * A typed number of units for a supply or colonist offer, so a party buying 500 food types 500 instead
 * of pressing a button 500 times. A form, so Enter confirms; when adding, the cost shows once the number is valid.
 */
export function UnitsForm({
	item,
	inputLabel,
	submitLabel,
	initialText = "",
	limitLots,
	inCartLots = 0,
	currentLots,
	isUpdating,
	onSubmit,
	children,
}: {
	item: Sold;
	inputLabel: string;
	submitLabel: string;
	initialText?: string;
	limitLots: number;
	inCartLots?: number;
	/** The line's lots today: submitting the same number again is disabled. */
	currentLots?: number;
	isUpdating: boolean;
	onSubmit: (lots: number, clear: () => void) => void;
	children?: ReactNode;
}) {
	const [text, setText] = useState(initialText);
	const [isEmptyHintShown, setEmptyHintShown] = useState(false);
	const errorId = useId();
	const entry = parseUnits(text, item, { inCartLots, limitLots });
	const size = lotSize(item);
	const isUnchanged = entry.kind === "VALID" && entry.lots === currentLots;
	const isEmptyEdit = entry.kind === "EMPTY" && currentLots !== undefined;
	const isDisabled = isUpdating || entry.kind === "INVALID" || isUnchanged || isEmptyEdit;
	let errorMessage: string | null = null;
	if (entry.kind === "INVALID") {
		errorMessage = entry.message;
	} else if (entry.kind === "EMPTY" && isEmptyHintShown) {
		errorMessage = `Enter a whole number, ${formatNumber(size)} or more.`;
	}

	function handleSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		if (entry.kind === "EMPTY") {
			setEmptyHintShown(true);
			return;
		}
		if (entry.kind !== "VALID" || isDisabled) {
			return;
		}
		onSubmit(entry.lots, () => {
			setText("");
		});
	}

	return (
		<form className="ml-auto flex flex-col items-end gap-1" noValidate onSubmit={handleSubmit}>
			<div className="flex items-center gap-2">
				{entry.kind === "VALID" && currentLots === undefined ? (
					<output aria-live="polite" className="text-muted-foreground text-sm tabular-nums">
						{formatNumber(entry.lots * item.priceGold)} gp
					</output>
				) : null}
				<Input
					aria-describedby={errorMessage ? errorId : undefined}
					aria-invalid={errorMessage !== null}
					aria-label={inputLabel}
					autoComplete="off"
					className="w-24 tabular-nums"
					inputMode="numeric"
					onChange={(event) => {
						setText(event.target.value);
						setEmptyHintShown(false);
					}}
					placeholder={formatNumber(size)}
					type="text"
					value={text}
				/>
				<Button disabled={isDisabled} size="sm" type="submit" variant="outline">
					{submitLabel}
				</Button>
				{children}
			</div>
			{errorMessage ? (
				<p className="w-0 min-w-full text-right text-ember-ink text-sm" id={errorId}>
					{errorMessage}
				</p>
			) : null}
		</form>
	);
}
