import type { RowApplier } from "../outbox";
import { type CoreAdapterDependencies, createCoreAdapter } from "./core";

/** How one kind of world applies outbox rows, keyed by row kind. */
export interface BridgeAdapter {
	appliers: Partial<Record<string, RowApplier>>;
	name: string;
}

type AdapterFactory = (dependencies: CoreAdapterDependencies) => BridgeAdapter;

// Phase 2 adds pf2e here, spreading the core appliers and adding its own kinds (design §6).
const systemAdapters: Partial<Record<string, AdapterFactory>> = {};

/** Chosen once at `ready` from game.system.id; any system without its own adapter gets the core one. */
export function selectAdapter(
	systemId: string,
	dependencies: CoreAdapterDependencies,
): BridgeAdapter {
	const createSystemAdapter = systemAdapters[systemId] ?? createCoreAdapter;
	return createSystemAdapter(dependencies);
}
