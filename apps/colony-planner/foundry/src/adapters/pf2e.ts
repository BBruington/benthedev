import type { FoundryGame, FoundryItem, FoundryPartyActor } from "../foundry-types";
import { escapeHtml } from "../html";
import {
	compendiumItemUuidPattern,
	type DeliveryItem,
	type DeliveryPayload,
	type DeliveryUndonePayload,
	isDeliveryPayload,
	maximumFoundryUuidLength,
	type OutboxRow,
	readField,
} from "../wire";
import type { BridgeAdapter } from "./adapter";
import {
	applyDeliveryUndone,
	bridgeWriteOptions,
	type CoreAdapterDependencies,
	chatSpeaker,
	createCoreAdapter,
	externalIdFlag,
	findPostedMessage,
	hasExternalId,
} from "./core";
import { itemList, renderDeliveryLine } from "./render";

export interface Pf2eAdapterDependencies extends CoreAdapterDependencies {
	foundryGame: CoreAdapterDependencies["foundryGame"] & Pick<FoundryGame, "actors">;
	fromUuid: (uuid: string) => Promise<unknown>;
}

/** The core adapter, with DELIVERY adding compendium items to the party actor's inventory. */
export function createPf2eAdapter(dependencies: Pf2eAdapterDependencies): BridgeAdapter {
	const core = createCoreAdapter(dependencies);
	return {
		...core,
		appliers: {
			...core.appliers,
			DELIVERY: (row) => applyDelivery(row, dependencies),
			DELIVERY_UNDONE: (row) =>
				applyDeliveryUndone(row, dependencies, (deliveryKey, undone) =>
					partyHeldUndoneLines(deliveryKey, undone, dependencies),
				),
		},
		name: "pf2e",
	};
}

// Any other UUID (a world or actor item) could copy a document players can't see to the party.
function isCompendiumUuid(uuid: string | null): uuid is string {
	return (
		uuid !== null && uuid.length <= maximumFoundryUuidLength && compendiumItemUuidPattern.test(uuid)
	);
}

function isPhysicalItem(value: unknown): value is FoundryItem {
	return (
		readField(value, "documentName") === "Item" &&
		typeof readField(value, "toObject") === "function" &&
		typeof readField(readField(value, "system"), "quantity") === "number"
	);
}

async function resolveItem(
	line: DeliveryItem,
	fromUuid: Pf2eAdapterDependencies["fromUuid"],
): Promise<FoundryItem | undefined> {
	if (!isCompendiumUuid(line.compendiumUuid)) {
		return undefined;
	}
	try {
		const document = await fromUuid(line.compendiumUuid);
		return isPhysicalItem(document) ? document : undefined;
	} catch {
		// A UUID Foundry can't parse, or a compendium this world lacks: the GM hands it out instead.
		return undefined;
	}
}

function lineExternalId(row: Pick<OutboxRow, "targetKey">, index: number): string {
	return `${row.targetKey}:${index}`;
}

/**
 * The items go first and the message last, so a delivery whose message never posted can still have
 * put lines in the party's inventory: those are what an undone payment asks the GMs to remove.
 */
function partyHeldUndoneLines(
	deliveryKey: string,
	undone: DeliveryUndonePayload,
	dependencies: Pf2eAdapterDependencies,
): DeliveryUndonePayload["items"] {
	const party = dependencies.foundryGame.actors.party;
	if (!party) {
		return [];
	}
	return undone.items.filter((_line, index) =>
		party.items.find((item) =>
			hasExternalId(item, lineExternalId({ targetKey: deliveryKey }, index)),
		),
	);
}

function shallowCopy(value: unknown): Record<string, unknown> {
	return typeof value === "object" && value !== null
		? Object.fromEntries(Object.entries(value))
		: {};
}

function partyItemSource(item: FoundryItem, quantity: number, externalId: string): object {
	const { _id, ...source } = item.toObject();
	return {
		...source,
		flags: { ...shallowCopy(source.flags), ...externalIdFlag(externalId) },
		system: { ...shallowCopy(source.system), quantity },
	};
}

/** Lines already on the party actor, found by each line's own flag: what the message may call added. */
function heldLines(
	row: OutboxRow,
	delivery: DeliveryPayload,
	party: FoundryPartyActor | undefined,
): number[] {
	return delivery.items.flatMap((line, index) =>
		party &&
		isCompendiumUuid(line.compendiumUuid) &&
		party.items.find((item) => hasExternalId(item, lineExternalId(row, index)))
			? [index]
			: [],
	);
}

/** Adds every resolvable line the party doesn't hold yet, in one write. */
async function addMissingItems(
	row: OutboxRow,
	delivery: DeliveryPayload,
	party: FoundryPartyActor,
	dependencies: Pf2eAdapterDependencies,
): Promise<void> {
	const held = heldLines(row, delivery, party);
	const missing = delivery.items.flatMap((line, index) =>
		held.includes(index) ? [] : [{ index, line }],
	);
	const resolved = await Promise.all(
		missing.map(({ line }) => resolveItem(line, dependencies.fromUuid)),
	);
	const sources = missing.flatMap(({ index, line }, position) => {
		const item = resolved[position];
		return item ? [partyItemSource(item, line.quantity, lineExternalId(row, index))] : [];
	});
	if (sources.length > 0) {
		await party.createEmbeddedDocuments("Item", sources, bridgeWriteOptions());
	}
}

async function postDelivery(
	row: OutboxRow,
	delivery: DeliveryPayload,
	party: FoundryPartyActor | undefined,
	dependencies: Pf2eAdapterDependencies,
): Promise<void> {
	const added = heldLines(row, delivery, party);
	const addedLines = delivery.items.filter((_line, index) => added.includes(index));
	const handOut = delivery.items.filter((_line, index) => !added.includes(index));
	const partyName = escapeHtml(party?.name ?? "the party");
	const message = await dependencies.chatMessage.create(
		{
			content: [
				renderDeliveryLine(delivery),
				itemList(`Added to ${partyName}'s inventory:`, addedLines),
				itemList("Hand these out yourself:", handOut),
			].join(""),
			flags: externalIdFlag(row.targetKey),
			speaker: chatSpeaker,
		},
		bridgeWriteOptions(),
	);
	if (!message) {
		throw new Error("Foundry didn't post the delivery in chat.");
	}
}

/**
 * Adds the items first and posts the message last, naming as added only the lines the party holds,
 * so chat never claims an item that didn't arrive. A replay before the message posted adds only what
 * is still missing (each item carries its line's flag); once the GM's message exists the row is done.
 * When the items fail to save, the message still posts, with those lines to hand out, and the row
 * fails with Foundry's reason. The module never deletes an item.
 */
async function applyDelivery(row: OutboxRow, dependencies: Pf2eAdapterDependencies): Promise<void> {
	if (!isDeliveryPayload(row.payload)) {
		throw new Error("The delivery's payload wasn't understood.");
	}
	const delivery = row.payload;
	if (findPostedMessage(row, dependencies)) {
		return;
	}
	const party = dependencies.foundryGame.actors.party ?? undefined;
	let failure: unknown;
	if (party) {
		await addMissingItems(row, delivery, party, dependencies).catch((error: unknown) => {
			failure = error;
		});
	}
	await postDelivery(row, delivery, party, dependencies);
	if (failure !== undefined) {
		throw failure;
	}
}
