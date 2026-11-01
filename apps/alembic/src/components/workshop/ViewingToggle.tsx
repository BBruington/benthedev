import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Eye, Shield } from "lucide-react";
import { toast } from "sonner";
import type { ViewingAs } from "#/brewing/types";
import { Button } from "#/components/ui/button";
import { orpc } from "#/orpc/client";

export function ViewingToggle({
	viewingAs,
	isGm,
}: {
	viewingAs: ViewingAs;
	isGm: boolean;
}) {
	const queryClient = useQueryClient();
	const mutation = useMutation(
		orpc.viewing.set.mutationOptions({
			onSuccess: ({ viewingAs: next }) => {
				queryClient.setQueryData(orpc.workshop.get.queryKey(), (current) =>
					current ? { ...current, viewingAs: next } : current,
				);
				if (next === "gm")
					return queryClient.invalidateQueries({ queryKey: orpc.gm.key() });
			},
			onError: (error) => toast.error(error.message),
		}),
	);
	const shown = mutation.isPending ? mutation.variables.as : viewingAs;
	return (
		<div className="flex flex-wrap items-center gap-2 text-sm">
			<span
				aria-hidden="true"
				className="hidden text-muted-foreground sm:inline"
			>
				Viewing as:
			</span>
			<fieldset className="flex gap-1 rounded-lg border bg-muted/40 p-1">
				<legend className="sr-only">Viewing as</legend>
				{isGm && (
					<Button
						type="button"
						size="sm"
						variant={shown === "gm" ? "default" : "ghost"}
						aria-pressed={shown === "gm"}
						disabled={mutation.isPending}
						onClick={() => mutation.mutate({ as: "gm" })}
					>
						<Shield className="hidden size-3.5 sm:block" />
						GM
					</Button>
				)}
				<Button
					type="button"
					size="sm"
					variant={shown === "player" ? "default" : "ghost"}
					aria-pressed={shown === "player"}
					disabled={mutation.isPending}
					onClick={() => mutation.mutate({ as: "player" })}
				>
					<Eye className="hidden size-3.5 sm:block" />
					Player
				</Button>
			</fieldset>
		</div>
	);
}
