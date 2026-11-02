import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Minus, Plus } from "lucide-react";
import { toast } from "sonner";
import type { ItemKind } from "#/brewing/types";
import { Button } from "#/components/ui/button";
import { orpc } from "#/orpc/client";

const MAX_STOCK = 999;

interface StockStepperProps {
	kind: ItemKind;
	itemId: string;
	itemName: string;
	amount: number;
}

export function StockStepper({
	kind,
	itemId,
	itemName,
	amount,
}: StockStepperProps) {
	const queryClient = useQueryClient();
	const mutation = useMutation(
		orpc.inventory.adjust.mutationOptions({
			onMutate: async ({ delta }) => {
				await queryClient.cancelQueries({ queryKey: orpc.workshop.key() });
				queryClient.setQueryData(orpc.workshop.get.queryKey(), (current) => {
					if (!current) return current;
					const clamp = (stocked: number) =>
						Math.min(MAX_STOCK, Math.max(0, stocked + delta));
					if (kind === "potion")
						return {
							...current,
							codex: current.codex.map((entry) =>
								entry.potionId === itemId
									? { ...entry, stock: clamp(entry.stock) }
									: entry,
							),
						};
					return {
						...current,
						inventory: {
							...current.inventory,
							[itemId]: clamp(current.inventory[itemId] ?? 0),
						},
					};
				});
			},
			onError: (error) => {
				toast.error(error.message);
				return queryClient.invalidateQueries({
					queryKey: orpc.workshop.key(),
				});
			},
		}),
	);

	return (
		<div className="flex items-center gap-1 text-xs text-muted-foreground">
			<span className="mr-1">In stock</span>
			<Button
				type="button"
				size="icon-xs"
				variant="outline"
				aria-label={`Remove one ${itemName} from stock`}
				disabled={amount <= 0}
				onClick={() => mutation.mutate({ kind, itemId, delta: -1 })}
			>
				<Minus />
			</Button>
			<span
				className="min-w-6 text-center font-mono tabular-nums text-foreground"
				aria-live="polite"
			>
				{amount}
			</span>
			<Button
				type="button"
				size="icon-xs"
				variant="outline"
				aria-label={`Add one ${itemName} to stock`}
				disabled={amount >= MAX_STOCK}
				onClick={() => mutation.mutate({ kind, itemId, delta: 1 })}
			>
				<Plus />
			</Button>
		</div>
	);
}
