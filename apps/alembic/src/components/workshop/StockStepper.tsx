import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Minus, Plus } from "lucide-react";
import { toast } from "sonner";
import { Button } from "#/components/ui/button";
import { orpc } from "#/orpc/client";

const MAX_STOCK = 999;

interface StockStepperProps {
	ingredientId: string;
	ingredientName: string;
	amount: number;
}

export function StockStepper({
	ingredientId,
	ingredientName,
	amount,
}: StockStepperProps) {
	const queryClient = useQueryClient();
	const mutation = useMutation(
		orpc.inventory.adjust.mutationOptions({
			onMutate: async ({ delta }) => {
				await queryClient.cancelQueries({ queryKey: orpc.workshop.key() });
				queryClient.setQueryData(orpc.workshop.get.queryKey(), (current) => {
					if (!current) return current;
					const stocked = current.inventory[ingredientId] ?? 0;
					return {
						...current,
						inventory: {
							...current.inventory,
							[ingredientId]: Math.min(MAX_STOCK, Math.max(0, stocked + delta)),
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
		<div className="mt-2 flex items-center gap-1 text-xs text-muted-foreground">
			<span className="mr-1">In stock</span>
			<Button
				type="button"
				size="icon-xs"
				variant="outline"
				aria-label={`Remove one ${ingredientName} from stock`}
				disabled={amount <= 0}
				onClick={() => mutation.mutate({ ingredientId, delta: -1 })}
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
				aria-label={`Add one ${ingredientName} to stock`}
				disabled={amount >= MAX_STOCK}
				onClick={() => mutation.mutate({ ingredientId, delta: 1 })}
			>
				<Plus />
			</Button>
		</div>
	);
}
