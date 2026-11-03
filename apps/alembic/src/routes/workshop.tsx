import { auth } from "@clerk/tanstack-react-start/server";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link, redirect } from "@tanstack/react-router";
import { createServerFn } from "@tanstack/react-start";
import { ArrowDown, ArrowUp, CircleHelp } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import type { BrewView, DeconstructView, ItemKind, Mix } from "#/brewing/types";
import { HouseRules } from "#/components/HouseRules";
import { Button } from "#/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "#/components/ui/tabs";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/ui/tooltip";
import { AnswerKey } from "#/components/workshop/AnswerKey";
import { BrewBench } from "#/components/workshop/BrewBench";
import { BrewingLoader } from "#/components/workshop/BrewingLoader";
import { BrewLog } from "#/components/workshop/BrewLog";
import { Codex } from "#/components/workshop/Codex";
import { CrafterCard } from "#/components/workshop/CrafterCard";
import { ResultCard } from "#/components/workshop/ResultCard";
import { Shelf } from "#/components/workshop/Shelf";
import { ViewingToggle } from "#/components/workshop/ViewingToggle";
import { TRADITION_LABEL } from "#/lib/format";
import { orpc } from "#/orpc/client";

const requireSession = createServerFn({ method: "GET" }).handler(async () => {
	const session = await auth();
	if (!session.userId) throw redirect({ to: "/sign-in/$" });
});

export const Route = createFileRoute("/workshop")({
	beforeLoad: () => requireSession(),
	component: Workshop,
});

function Workshop() {
	const queryClient = useQueryClient();
	const workshop = useQuery(orpc.workshop.get.queryOptions());
	const [mix, setMix] = useState<Mix>({});
	const [previewMix, setPreviewMix] = useState<Mix | null>(null);
	const [result, setResult] = useState<BrewView | DeconstructView | null>(null);
	const benchRef = useRef<HTMLElement>(null);
	const [benchVisible, setBenchVisible] = useState(true);
	const hasWorkshop = workshop.data !== undefined;
	useEffect(() => {
		if (!hasWorkshop) return;
		const bench = benchRef.current;
		if (!bench) return;
		const observer = new IntersectionObserver(([entry]) => {
			setBenchVisible(entry.isIntersecting);
		});
		observer.observe(bench);
		return () => observer.disconnect();
	}, [hasWorkshop]);
	const brew = useMutation(
		orpc.brew.attempt.mutationOptions({
			onMutate: () => setResult(null),
			onSuccess: (nextResult) => {
				setResult(nextResult);
				toastBrew(nextResult);
			},
			onSettled: () =>
				queryClient.invalidateQueries({ queryKey: orpc.workshop.key() }),
		}),
	);
	const deconstruct = useMutation(
		orpc.deconstruct.attempt.mutationOptions({
			onMutate: () => setResult(null),
			onSuccess: (nextResult) => {
				setResult(nextResult);
				toast(
					nextResult.learned
						? `${nextResult.name}: ${TRADITION_LABEL[nextResult.learned.tradition]} is ${nextResult.learned.value}`
						: `Deconstructing ${nextResult.name} taught nothing`,
					{ position: "top-right" },
				);
			},
			onError: (error) => toast.error(error.message, { position: "top-right" }),
			onSettled: () =>
				queryClient.invalidateQueries({ queryKey: orpc.workshop.key() }),
		}),
	);
	function deconstructItem(kind: ItemKind, itemId: string) {
		deconstruct.mutate({ kind, itemId });
	}
	const units = Object.values(mix).reduce((sum, quantity) => sum + quantity, 0);

	function changeIngredient(id: string, delta: number) {
		setMix((current) => {
			const count = Object.values(current).reduce(
				(sum, quantity) => sum + quantity,
				0,
			);
			if (delta > 0 && count >= 4) return current;
			const next = { ...current };
			const quantity = Math.max(0, (next[id] ?? 0) + delta);
			if (quantity === 0) delete next[id];
			else next[id] = quantity;
			return next;
		});
	}

	function useMix(nextMix: Mix) {
		setMix(nextMix);
		benchRef.current?.scrollIntoView({ block: "start" });
		benchRef.current?.focus({ preventScroll: true });
	}

	if (workshop.isPending)
		return (
			<main className="page-wrap px-4 py-12">
				<BrewingLoader />
			</main>
		);
	if (!workshop.data)
		return (
			<main className="page-wrap space-y-4 px-4 py-12">
				<h1 className="font-serif text-3xl">The workshop could not open</h1>
				<p role="alert" className="text-sm text-destructive">
					{workshop.error?.message ?? "Please try again."}
				</p>
				<Button onClick={() => workshop.refetch()}>Try again</Button>
			</main>
		);
	const view = workshop.data;
	const mentorNote = view.codex.find(
		(entry) => entry.source === "mentor",
	)?.note;

	return (
		<main className="workshop-layout page-wrap">
			<div className="workshop-heading">
				<h1 className="text-2xl font-semibold tracking-tight">The Workshop</h1>
				<div className="flex flex-wrap items-center gap-3">
					<Tooltip>
						<TooltipTrigger asChild>
							<Button asChild variant="ghost" size="sm">
								<Link
									to="/"
									hash="first-brew"
									aria-label="New here? Learn how brewing works"
								>
									New here?
									<CircleHelp className="size-4" aria-hidden="true" />
								</Link>
							</Button>
						</TooltipTrigger>
						<TooltipContent>Learn how brewing works</TooltipContent>
					</Tooltip>
					<ViewingToggle viewingAs={view.viewingAs} isGm={view.isGm} />
				</div>
			</div>
			<CrafterCard crafter={view.crafter} />
			<div className="workshop-workspace">
				<div className="workshop-library" id="ingredients">
					<Shelf
						ingredients={view.ingredients}
						inventory={view.inventory}
						mix={mix}
						units={units}
						viewingAs={view.viewingAs}
						deconstructing={deconstruct.isPending}
						onDeconstruct={deconstructItem}
						onChange={changeIngredient}
					/>
					<Tabs defaultValue="codex" className="mt-5">
						<TabsList variant="line" aria-label="Workshop records">
							<TabsTrigger value="codex">Potion codex</TabsTrigger>
							<TabsTrigger value="log">
								Brew log · {view.log.length}
							</TabsTrigger>
						</TabsList>
						<TabsContent value="codex">
							<Codex
								workshop={view}
								onUseMix={useMix}
								onPreviewMix={setPreviewMix}
								deconstructing={deconstruct.isPending}
								onDeconstruct={deconstructItem}
							/>
						</TabsContent>
						<TabsContent value="log">
							<BrewLog log={view.log} ingredients={view.ingredients} />
						</TabsContent>
					</Tabs>
				</div>
				<section
					ref={benchRef}
					className="workshop-bench"
					id="brew-bench"
					aria-label="Brew bench"
					tabIndex={-1}
				>
					{mentorNote && (
						<p className="rounded-lg border border-primary/30 bg-primary/10 px-4 py-3 text-sm">
							<span className="font-semibold">Mentor's note:</span> {mentorNote}
						</p>
					)}
					<BrewBench
						ingredients={view.ingredients}
						inventory={view.inventory}
						mix={previewMix ?? mix}
						units={
							previewMix
								? Object.values(previewMix).reduce((sum, n) => sum + n, 0)
								: units
						}
						previewing={previewMix !== null}
						pending={brew.isPending}
						error={brew.error?.message ?? null}
						onChange={changeIngredient}
						onClear={() => setMix({})}
						onBrew={(d20) =>
							brew.mutate({ mix, ...(d20 === undefined ? {} : { d20 }) })
						}
					/>
					<div aria-live="polite">
						<ResultCard result={result} crafterName={view.crafter.name} />
					</div>
					<a
						href="#ingredients"
						className="bench-mobile-link items-center gap-2 text-sm text-primary underline underline-offset-4"
					>
						Choose ingredients{" "}
						<ArrowDown className="size-4" aria-hidden="true" />
					</a>
				</section>
			</div>
			{view.viewingAs === "gm" && <AnswerKey />}
			<HouseRules />
			<a
				href="#brew-bench"
				className="mobile-bench-shortcut"
				hidden={benchVisible}
			>
				<span>Brew bench</span>
				<span className="flex items-center gap-2">
					{units} / 4 units · Review mix
					<ArrowUp className="size-4" aria-hidden="true" />
				</span>
			</a>
		</main>
	);
}

function toastBrew(result: BrewView) {
	const position = "top-right";
	const potionName = result.potionName ?? "Potion";
	if (result.kind === "inert")
		return toast("The mix stays inert", { position });
	if (result.kind === "beyond")
		return toast.warning(`${potionName} is beyond your reach`, { position });
	const potions = `${result.quantity} ${result.quantity === 1 ? "potion" : "potions"}`;
	return toast.success(`Brewed ${potionName}`, {
		position,
		description: result.perfect ? `${potions} · Perfect mix` : potions,
	});
}
