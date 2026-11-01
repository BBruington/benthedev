import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { toast } from "sonner";
import { CRAFTING_RANKS, type CrafterView, TOOLS } from "#/brewing/types";
import { Button } from "#/components/ui/button";
import { Input } from "#/components/ui/input";
import { Label } from "#/components/ui/label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/ui/select";
import {
	Sheet,
	SheetContent,
	SheetDescription,
	SheetHeader,
	SheetTitle,
	SheetTrigger,
} from "#/components/ui/sheet";
import { Switch } from "#/components/ui/switch";
import { orpc } from "#/orpc/client";
import { type CrafterEdit, crafterEditSchema } from "#/orpc/schema";

export const TOOL_LABELS = {
	none: "No tools (−2)",
	basic: "Alchemist's tools (+0)",
	expanded: "Expanded alchemist's lab (+1)",
};

export function CrafterSheet({ crafter }: { crafter: CrafterView }) {
	const [open, setOpen] = useState(false);
	const id = useId();
	const queryClient = useQueryClient();
	const form = useForm<CrafterEdit>({
		resolver: zodResolver(crafterEditSchema),
		defaultValues: crafter,
	});
	const mutation = useMutation(
		orpc.crafter.update.mutationOptions({
			onSuccess: async () => {
				await queryClient.invalidateQueries({ queryKey: orpc.workshop.key() });
				toast.success("Crafter updated. Odds and gates recalculated.");
				setOpen(false);
			},
			onError: (error) => toast.error(error.message),
		}),
	);
	const errors = form.formState.errors;
	return (
		<Sheet
			open={open}
			onOpenChange={(next) => {
				if (next) form.reset(crafter);
				setOpen(next);
			}}
		>
			<SheetTrigger asChild>
				<Button size="sm" variant="outline">
					Edit crafter
				</Button>
			</SheetTrigger>
			<SheetContent className="w-full overflow-y-auto">
				<SheetHeader>
					<SheetTitle>Edit {crafter.name}</SheetTitle>
					<SheetDescription>
						Your Crafting modifier, potion odds and brewing gates update when
						you save.
					</SheetDescription>
				</SheetHeader>
				<form
					className="space-y-6 px-4 pb-6"
					onSubmit={form.handleSubmit((values) => mutation.mutate(values))}
					noValidate
				>
					<fieldset className="space-y-6" disabled={mutation.isPending}>
						<div className="space-y-2">
							<Label htmlFor={`${id}-level`}>Level</Label>
							<Input
								id={`${id}-level`}
								type="number"
								min={1}
								max={20}
								aria-invalid={!!errors.level}
								aria-describedby={
									errors.level ? `${id}-level-error` : undefined
								}
								{...form.register("level", { valueAsNumber: true })}
							/>
							{errors.level && (
								<p
									id={`${id}-level-error`}
									className="text-sm text-destructive"
								>
									{errors.level.message}
								</p>
							)}
						</div>
						<div className="space-y-2">
							<Label htmlFor={`${id}-int`}>Intelligence modifier</Label>
							<Input
								id={`${id}-int`}
								type="number"
								min={-1}
								max={7}
								aria-invalid={!!errors.intMod}
								aria-describedby={errors.intMod ? `${id}-int-error` : undefined}
								{...form.register("intMod", { valueAsNumber: true })}
							/>
							{errors.intMod && (
								<p id={`${id}-int-error`} className="text-sm text-destructive">
									{errors.intMod.message}
								</p>
							)}
						</div>
						<Controller
							control={form.control}
							name="craftingRank"
							rules={{ deps: "alchemicalCrafting" }}
							render={({ field }) => (
								<div className="space-y-2">
									<Label htmlFor={`${id}-rank`}>Crafting rank</Label>
									<Select
										value={field.value}
										onValueChange={field.onChange}
										disabled={mutation.isPending}
									>
										<SelectTrigger
											id={`${id}-rank`}
											className="w-full"
											ref={field.ref}
											onBlur={field.onBlur}
										>
											<SelectValue />
										</SelectTrigger>
										<SelectContent>
											{CRAFTING_RANKS.map((rank) => (
												<SelectItem
													key={rank}
													value={rank}
													className="capitalize"
												>
													{rank}
												</SelectItem>
											))}
										</SelectContent>
									</Select>
									{errors.craftingRank && (
										<p className="text-sm text-destructive">
											{errors.craftingRank.message}
										</p>
									)}
								</div>
							)}
						/>
						<Controller
							control={form.control}
							name="alchemicalCrafting"
							render={({ field }) => (
								<div className="space-y-2">
									<div className="flex items-center justify-between gap-3">
										<Label htmlFor={`${id}-feat`}>Alchemical Crafting</Label>
										<Switch
											id={`${id}-feat`}
											checked={field.value}
											onCheckedChange={field.onChange}
											onBlur={field.onBlur}
											ref={field.ref}
											disabled={mutation.isPending}
											aria-invalid={!!errors.alchemicalCrafting}
											aria-describedby={
												errors.alchemicalCrafting
													? `${id}-feat-error`
													: undefined
											}
										/>
									</div>
									{errors.alchemicalCrafting && (
										<p
											id={`${id}-feat-error`}
											role="alert"
											className="text-sm text-destructive"
										>
											{errors.alchemicalCrafting.message}
										</p>
									)}
								</div>
							)}
						/>
						<Controller
							control={form.control}
							name="tools"
							render={({ field }) => (
								<div className="space-y-2">
									<Label htmlFor={`${id}-tools`}>Tools</Label>
									<Select
										value={field.value}
										onValueChange={field.onChange}
										disabled={mutation.isPending}
									>
										<SelectTrigger
											id={`${id}-tools`}
											className="w-full"
											ref={field.ref}
											onBlur={field.onBlur}
										>
											<SelectValue />
										</SelectTrigger>
										<SelectContent>
											{TOOLS.map((tools) => (
												<SelectItem key={tools} value={tools}>
													{TOOL_LABELS[tools]}
												</SelectItem>
											))}
										</SelectContent>
									</Select>
									{errors.tools && (
										<p className="text-sm text-destructive">
											{errors.tools.message}
										</p>
									)}
								</div>
							)}
						/>
						<Button
							type="submit"
							className="w-full"
							disabled={mutation.isPending}
						>
							{mutation.isPending ? "Saving…" : "Save crafter"}
						</Button>
					</fieldset>
				</form>
			</SheetContent>
		</Sheet>
	);
}
