import { createDepositAppliers, type DepositRowDependencies } from "../deposits";
import type { RowApplier } from "../outbox";
import { createCoreAdapter } from "./core";
import { createPf2eAdapter, type Pf2eAdapterDependencies } from "./pf2e";

/** How one kind of world applies outbox rows, keyed by row kind. */
export interface BridgeAdapter {
	appliers: Partial<Record<string, RowApplier>>;
	name: string;
	/** Withdraws every NPC, place and quest entry the bridge made, as their withdraw rows do. */
	withdrawBridgedEntries(): Promise<void>;
}

/** Everything any adapter needs; each takes the part it uses. */
export type AdapterDependencies = Pf2eAdapterDependencies & DepositRowDependencies;

type AdapterFactory = (dependencies: AdapterDependencies) => BridgeAdapter;

const systemAdapters: Partial<Record<string, AdapterFactory>> = { pf2e: createPf2eAdapter };

/**
 * Chosen once at `ready` from game.system.id; any system without its own adapter gets the core one.
 * Every adapter takes the escrow's rows, and only PF2e's moves coins.
 */
export function selectAdapter(systemId: string, dependencies: AdapterDependencies): BridgeAdapter {
	const createSystemAdapter = systemAdapters[systemId] ?? createCoreAdapter;
	const adapter = createSystemAdapter(dependencies);
	return {
		...adapter,
		appliers: {
			...adapter.appliers,
			...createDepositAppliers(dependencies, systemId === "pf2e"),
		},
	};
}
