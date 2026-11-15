import { type CoreAdapterDependencies, postOnce } from "./adapters/core";
import { formatNumber } from "./adapters/render";
import type { EventQueue } from "./event-queue";
import type { FoundryNotifications, Pf2eInventory } from "./foundry-types";
import { cleanLine, escapeHtml } from "./html";
import type { RowApplier } from "./outbox";
import {
	type CoinDepositEvent,
	foundryActorIdPattern,
	type InboundEvent,
	isCoinDepositEvent,
	isDepositRefundPayload,
	isDepositUndonePayload,
	maximumActorNameLength,
	type OutboxRow,
	readField,
} from "./wire";

/**
 * The escrow (design §14.10): every deposit's record, kept in a world setting so a new link, a reload
 * or another GM's browser still knows which coins Colony Planner holds.
 */
export const depositStates = [
	"REMOVING",
	"HELD",
	"SENT",
	"RETURNING",
	"RETURNED",
	"INTERRUPTED",
] as const;

export type DepositState = (typeof depositStates)[number];

export interface DepositEntry {
	event: CoinDepositEvent;
	gold: number;
	/** The link the deposit was made under: a random ID, never the token, since players read this. */
	linkId: string;
	actorId: string;
	/** Set once the coins are back: a DM's dismissal (REFUND) or Colony Planner's refusal (REFUSED). */
	returnedBy?: "REFUND" | "REFUSED";
	state: DepositState;
	updatedAt: string;
}

export interface DepositLedgerStorage {
	/** False once a user who isn't a GM wrote the setting last, as one granted SETTINGS_MODIFY can. */
	lastWriterIsGm(): boolean;
	load(): unknown;
	save(value: { entries: Record<string, unknown> }): Promise<unknown>;
}

/**
 * Whether the ledger's world Setting document, as `game.settings.get(..., {document: true})` gives it, was
 * last written by a GM: Foundry's server sets `_stats.lastModifiedBy` on every write, and clients can't.
 * An unstored Setting (no `_id`) holds only the registered default; a stored one with no writer is refused.
 */
export function isWrittenByGameMaster(
	setting: unknown,
	gameMasterIds: ReadonlySet<string>,
): boolean {
	if (typeof readField(setting, "_id") !== "string") {
		return true;
	}
	const writer = readField(readField(setting, "_stats"), "lastModifiedBy");
	return typeof writer === "string" && gameMasterIds.has(writer);
}

export interface DepositLedger {
	entries(): DepositEntry[];
	/** False when the ledger may hold an entry a non-GM planted, so nothing in it is resent or refunded. */
	writtenByGm(): boolean;
	get(depositId: string): DepositEntry | undefined;
	/** Writes a new entry; false, writing nothing, when its deposit already has one. */
	create(entry: DepositEntry): Promise<boolean>;
	/** Moves an entry only while it is in a `from` state (null `to` deletes); false when it wasn't. */
	move(
		depositId: string,
		from: readonly DepositState[],
		to: DepositState | null,
		returnedBy?: DepositEntry["returnedBy"],
	): Promise<boolean>;
}

function isDepositState(value: unknown): value is DepositState {
	return depositStates.some((state) => state === value);
}

function isDepositEntry(depositId: string, value: unknown): value is DepositEntry {
	const event = readField(value, "event");
	const updatedAt = readField(value, "updatedAt");
	const returnedBy = readField(value, "returnedBy");
	return (
		isCoinDepositEvent(event) &&
		event.id === depositId &&
		readField(value, "gold") === event.payload.gold &&
		readField(value, "actorId") === event.payload.actorId &&
		typeof readField(value, "linkId") === "string" &&
		isDepositState(readField(value, "state")) &&
		typeof updatedAt === "string" &&
		!Number.isNaN(Date.parse(updatedAt)) &&
		(returnedBy === undefined || returnedBy === "REFUND" || returnedBy === "REFUSED")
	);
}

function storedEntries(storage: DepositLedgerStorage): Record<string, unknown> {
	const entries = readField(storage.load(), "entries");
	return typeof entries === "object" && entries !== null && !Array.isArray(entries)
		? Object.fromEntries(Object.entries(entries))
		: {};
}

export function createDepositLedger(
	storage: DepositLedgerStorage,
	now: () => Date = () => new Date(),
): DepositLedger {
	// Each write rereads its entry after earlier writes land, so a stale read can't undo a later state.
	let writes: Promise<unknown> = Promise.resolve();

	function write(
		depositId: string,
		decide: (current: DepositEntry | undefined) => DepositEntry | null | undefined,
	): Promise<boolean> {
		const result = writes.then(async () => {
			const stored = storedEntries(storage);
			const current = stored[depositId];
			const next = decide(isDepositEntry(depositId, current) ? current : undefined);
			if (next === undefined) {
				return false;
			}
			const others = Object.entries(stored).filter(([id]) => id !== depositId);
			await storage.save({
				entries: Object.fromEntries(next ? [...others, [depositId, next]] : others),
			});
			return true;
		});
		writes = result.catch(() => undefined);
		return result;
	}

	function get(depositId: string): DepositEntry | undefined {
		const entry = storedEntries(storage)[depositId];
		return isDepositEntry(depositId, entry) ? entry : undefined;
	}

	return {
		create: (entry) => write(entry.event.id, (current) => (current ? undefined : entry)),
		entries: () =>
			Object.entries(storedEntries(storage)).flatMap(([depositId, entry]) =>
				isDepositEntry(depositId, entry) ? [entry] : [],
			),
		get,
		move: (depositId, from, to, returnedBy) =>
			write(depositId, (current) => {
				if (!(current && from.includes(current.state))) {
					return undefined;
				}
				if (to === null) {
					return null;
				}
				return {
					...current,
					...(returnedBy && { returnedBy }),
					state: to,
					updatedAt: now().toISOString(),
				};
			}),
		writtenByGm: () => storage.lastWriterIsGm(),
	};
}

/** A PF2e party or character actor, which pays a deposit from its own coins. */
export interface Pf2eDepositor {
	id: string;
	inventory: Pf2eInventory;
	name: string;
	type: "party" | "character";
}

function hasCoinInventory(value: unknown): value is { inventory: Pf2eInventory } {
	const inventory = readField(value, "inventory");
	return (
		typeof readField(inventory, "addCurrency") === "function" &&
		typeof readField(inventory, "removeCurrency") === "function" &&
		typeof readField(readField(inventory, "currency"), "copperValue") === "number"
	);
}

export function isPf2eDepositor(value: unknown): value is Pf2eDepositor {
	const id = readField(value, "id");
	const type = readField(value, "type");
	return (
		(type === "party" || type === "character") &&
		typeof id === "string" &&
		foundryActorIdPattern.test(id) &&
		typeof readField(value, "name") === "string" &&
		hasCoinInventory(value)
	);
}

/** What the bridge tells the escrow about deposit events in its queue. */
export interface DepositEvents {
	/** A 2xx: Colony Planner recorded the deposit, or already had. */
	markSent(events: InboundEvent[]): Promise<void>;
	/**
	 * False for a deposit whose entry isn't HELD under this link: one marked INTERRUPTED or returned in
	 * another browser must never reach Colony Planner from this browser's queue.
	 */
	maySend(event: InboundEvent, linkId: string): boolean;
	/** At every session start: resends what waits under this link, and gives up on the rest. */
	resume(queue: Pick<EventQueue, "add" | "holds">, linkId: string): Promise<void>;
	/**
	 * Colony Planner refused the batch for good. Only a 409 or a 422 means it stored nothing, so only
	 * then do the coins go back.
	 */
	returnRefused(events: InboundEvent[], reason: string, status: number): Promise<void>;
}

export interface DepositLink {
	enqueue(event: InboundEvent): boolean;
	/** Read when the entry is written and again just before it is queued, since a re-pair may land between. */
	id(): string;
}

export interface Deposits extends DepositEvents {
	/** Design §14.10's steps 1 to 3, under the running link; `enqueue` is false once it stopped. */
	deposit(actor: Pf2eDepositor, gold: number, link: DepositLink): Promise<void>;
	forget(depositId: string): Promise<void>;
	interrupted(): { depositId: string; gold: number; actorName: string }[];
}

export interface DepositDependencies {
	actors: { get(id: string): unknown };
	createId?: () => string;
	ledger: DepositLedger;
	notifications: FoundryNotifications;
	now?: () => Date;
}

const interruptedReturn =
	"This deposit's return was interrupted. Check the paying actor's coins and fix them by hand.";
const untrustedLedger =
	"A user who isn't a GM changed this world's deposit records last, so no deposit was sent again and no coins were added. Check the coins of the actors that deposited, and who may change world settings.";
const staleAfterMilliseconds = 10 * 60 * 1000;
const returnedKeptForMilliseconds = 30 * 24 * 60 * 60 * 1000;
const copperPerGold = 100;

function gp(gold: number): string {
	return `${formatNumber(gold)} gp`;
}

function reasonOf(error: unknown): string {
	return error instanceof Error && error.message ? error.message : String(error);
}

function depositorName(actor: Pf2eDepositor): string {
	return cleanLine(actor.name, maximumActorNameLength) || "The actor";
}

function shortWarning(actor: Pf2eDepositor): string {
	const goldWorth = Math.floor(actor.inventory.currency.copperValue / copperPerGold);
	return `${depositorName(actor)} has only ${gp(goldWorth)} worth of coins.`;
}

/**
 * Adds the gold back, recording RETURNING first so a replay never adds it twice (design §14.8); false,
 * adding nothing, when the entry left `from` meanwhile.
 */
async function returnCoins(
	entry: DepositEntry,
	from: readonly DepositState[],
	returnedBy: "REFUND" | "REFUSED",
	dependencies: Pick<DepositDependencies, "actors" | "ledger">,
): Promise<boolean> {
	const { ledger } = dependencies;
	const depositId = entry.event.id;
	const actor = dependencies.actors.get(entry.actorId);
	if (!hasCoinInventory(actor)) {
		throw new DepositorGoneError(entry.gold);
	}
	if (!(await ledger.move(depositId, from, "RETURNING"))) {
		return false;
	}
	try {
		await actor.inventory.addCurrency({ gp: entry.gold });
	} catch (error) {
		await ledger.move(depositId, ["RETURNING"], "INTERRUPTED");
		throw error;
	}
	await ledger.move(depositId, ["RETURNING"], "RETURNED", returnedBy);
	return true;
}

class DepositorGoneError extends Error {
	constructor(gold: number) {
		super(`The actor that paid this deposit is gone. Add ${gp(gold)} by hand.`);
		this.name = "DepositorGoneError";
	}
}

function depositEvents(events: InboundEvent[]): CoinDepositEvent[] {
	return events.filter(isCoinDepositEvent);
}

export function createDeposits(dependencies: DepositDependencies): Deposits {
	const { ledger, notifications } = dependencies;
	const createId = dependencies.createId ?? (() => crypto.randomUUID());
	const now = dependencies.now ?? (() => new Date());

	async function interrupt(
		depositId: string,
		from: readonly DepositState[],
		message: string,
	): Promise<void> {
		await ledger.move(depositId, from, "INTERRUPTED");
		notifications.error(`Colony Bridge: ${message}`);
	}

	async function returnRefusedDeposit(
		event: CoinDepositEvent,
		reason: string,
		status: number,
	): Promise<void> {
		// Any other 4xx (an edge's 404 during a bad deploy) may answer a retry of a send that was stored.
		if (status !== 409 && status !== 422) {
			notifications.error(
				`Colony Bridge: Colony Planner answered a deposit of ${gp(event.payload.gold)} with ${status} (${reason}), so this world can't tell whether it was recorded. Its coins stay held, and the bridge sends it again when it next starts.`,
			);
			return;
		}
		if (!ledger.writtenByGm()) {
			notifications.error(`Colony Bridge: ${untrustedLedger}`);
			return;
		}
		const entry = ledger.get(event.id);
		const refused = `Colony Planner refused a deposit of ${gp(event.payload.gold)} (${reason})`;
		try {
			if (
				entry?.state === "HELD" &&
				(await returnCoins(entry, ["HELD"], "REFUSED", dependencies))
			) {
				notifications.warn(
					`Colony Bridge: Colony Planner refused the deposit (${reason}). Its ${gp(entry.gold)} went back to ${entry.event.payload.actorName}.`,
				);
				return;
			}
			notifications.warn(
				`Colony Bridge: ${refused}, and this world's record of it isn't waiting to be sent, so no coins moved. Check ${possessive(event.payload.actorName)} coins.`,
			);
		} catch (error) {
			await interrupt(
				event.id,
				["HELD", "RETURNING"],
				`${refused}, and Foundry couldn't give the coins back. ${reasonOf(error)}`,
			);
		}
	}

	async function resumeEntry(
		entry: DepositEntry,
		queue: Pick<EventQueue, "add" | "holds">,
		linkId: string,
	): Promise<void> {
		const depositId = entry.event.id;
		const age = now().getTime() - Date.parse(entry.updatedAt);
		if (
			(entry.state === "REMOVING" || entry.state === "RETURNING") &&
			age > staleAfterMilliseconds
		) {
			await interrupt(
				depositId,
				[entry.state],
				`A deposit of ${gp(entry.gold)} from ${entry.event.payload.actorName} stopped part way. Check ${possessive(entry.event.payload.actorName)} coins and fix them by hand, then press Forget in Colony Bridge's settings.`,
			);
			return;
		}
		// Another link may name another campaign, whose inbox would then hold it beside the first.
		if (entry.state === "HELD" && entry.linkId !== linkId) {
			await interrupt(
				depositId,
				["HELD"],
				`This world's link changed before Colony Planner confirmed a deposit of ${gp(entry.gold)} from ${entry.event.payload.actorName}, so it wasn't sent again. Check the old campaign's inbox, fix ${possessive(entry.event.payload.actorName)} coins by hand, then press Forget in Colony Bridge's settings.`,
			);
			return;
		}
		if (entry.state === "HELD" && !queue.holds(depositId)) {
			queue.add(entry.event);
			return;
		}
		if (entry.state === "RETURNED" && age > returnedKeptForMilliseconds) {
			await ledger.move(depositId, ["RETURNED"], null);
		}
	}

	return {
		async deposit(actor, gold, link) {
			if (actor.inventory.currency.copperValue < gold * copperPerGold) {
				notifications.warn(shortWarning(actor));
				return;
			}
			const event: CoinDepositEvent = {
				id: createId(),
				kind: "COIN_DEPOSIT",
				occurredAt: now().toISOString(),
				payload: {
					actorId: actor.id,
					actorName: depositorName(actor),
					gold,
				},
			};
			const linkId = link.id();
			const created = await ledger.create({
				actorId: actor.id,
				event,
				gold,
				linkId,
				state: "REMOVING",
				updatedAt: now().toISOString(),
			});
			if (!created) {
				throw new Error("This world already has a deposit with that ID. Try again.");
			}
			let removed: boolean;
			try {
				removed = await actor.inventory.removeCurrency({ gp: gold }, { byValue: true });
			} catch (error) {
				// It isn't one update (it adds the change first, then up to four writes).
				await interrupt(
					event.id,
					["REMOVING"],
					`Foundry stopped part way through taking ${gp(gold)} from ${event.payload.actorName}, so nothing went to Colony Planner. Check ${possessive(event.payload.actorName)} coins and fix them by hand, then press Forget in Colony Bridge's settings. ${reasonOf(error)}`,
				);
				return;
			}
			if (!removed) {
				await ledger.move(event.id, ["REMOVING"], null);
				notifications.warn(shortWarning(actor));
				return;
			}
			if (!(await ledger.move(event.id, ["REMOVING"], "HELD"))) {
				notifications.error(
					`Colony Bridge: ${gp(gold)} left ${event.payload.actorName}, but its deposit record changed meanwhile, so nothing went to Colony Planner. Check ${possessive(event.payload.actorName)} coins and fix them by hand.`,
				);
				return;
			}
			if (link.id() !== linkId) {
				await interrupt(
					event.id,
					["HELD"],
					`This world's link changed while Colony Bridge took ${gp(gold)} from ${event.payload.actorName}, so the deposit went to no campaign. Fix ${possessive(event.payload.actorName)} coins by hand, then press Forget in Colony Bridge's settings.`,
				);
				return;
			}
			const deposited = `${possessive(event.payload.actorName)} ${gp(gold)} deposit`;
			if (link.enqueue(event)) {
				notifications.info(`Queued ${deposited} for Colony Planner's inbox.`);
				return;
			}
			notifications.warn(
				`Colony Bridge: Took ${deposited}, and it goes to Colony Planner's inbox once the bridge runs again in the active GM's browser.`,
			);
		},
		async forget(depositId) {
			await ledger.move(depositId, ["INTERRUPTED"], null);
		},
		interrupted: () =>
			ledger
				.entries()
				.filter((entry) => entry.state === "INTERRUPTED")
				.map((entry) => ({
					actorName: entry.event.payload.actorName,
					depositId: entry.event.id,
					gold: entry.gold,
				})),
		async markSent(events) {
			await Promise.all(
				depositEvents(events).map((event) => ledger.move(event.id, ["HELD"], "SENT")),
			);
		},
		maySend(event, linkId) {
			if (!isCoinDepositEvent(event)) {
				return true;
			}
			const entry = ledger.get(event.id);
			return entry?.state === "HELD" && entry.linkId === linkId;
		},
		async resume(queue, linkId) {
			if (!ledger.writtenByGm()) {
				notifications.error(`Colony Bridge: ${untrustedLedger}`);
				return;
			}
			await Promise.all(ledger.entries().map((entry) => resumeEntry(entry, queue, linkId)));
		},
		async returnRefused(events, reason, status) {
			await Promise.all(
				depositEvents(events).map((event) => returnRefusedDeposit(event, reason, status)),
			);
		},
	};
}

export interface DepositRowDependencies extends CoreAdapterDependencies {
	foundryGame: CoreAdapterDependencies["foundryGame"] & { actors: { get(id: string): unknown } };
	depositLedger: DepositLedger;
}

const refundKeyPrefix = "deposit-refund:";

function possessive(name: string): string {
	return name.endsWith("s") ? `${name}'` : `${name}'s`;
}

async function whisperGameMasters(
	row: OutboxRow,
	content: string,
	dependencies: DepositRowDependencies,
): Promise<void> {
	const gameMasterIds = dependencies.foundryGame.users
		.filter((user) => user.isGM)
		.map((user) => user.id);
	// Foundry posts a message with no whisper recipients to everyone.
	if (gameMasterIds.length === 0) {
		throw new Error("Foundry has no GM to whisper to.");
	}
	await postOnce(row, { content, whisper: gameMasterIds }, dependencies);
}

/**
 * A DM's dismissal (design §14.8 and §14.10): only a deposit this world's ledger holds, for the same
 * amount and actor, gets coins back, and only once; the ledger is the one-shot flag.
 */
async function applyDepositRefund(
	row: OutboxRow,
	dependencies: DepositRowDependencies,
): Promise<void> {
	if (!isDepositRefundPayload(row.payload)) {
		throw new Error("The deposit refund's payload wasn't understood.");
	}
	const refund = row.payload;
	if (row.targetKey !== refundKeyPrefix + refund.depositId) {
		throw new Error("The deposit refund's target key wasn't understood.");
	}
	if (!dependencies.depositLedger.writtenByGm()) {
		throw new Error(untrustedLedger);
	}
	const entry = dependencies.depositLedger.get(refund.depositId);
	if (!entry) {
		throw new Error("This world has no record of that deposit, so no coins were added.");
	}
	if (entry.gold !== refund.gold || entry.actorId !== refund.actorId) {
		throw new Error(
			"This refund doesn't match this world's record of the deposit, so no coins were added.",
		);
	}
	if (entry.state !== "RETURNED") {
		if (entry.state !== "HELD" && entry.state !== "SENT") {
			throw new Error(interruptedReturn);
		}
		const returned = await returnCoins(entry, ["HELD", "SENT"], "REFUND", {
			actors: dependencies.foundryGame.actors,
			ledger: dependencies.depositLedger,
		});
		if (!returned && dependencies.depositLedger.get(refund.depositId)?.state !== "RETURNED") {
			throw new Error(interruptedReturn);
		}
	}
	await whisperGameMasters(
		row,
		`<p>Colony Planner returned the ${gp(entry.gold)} ${escapeHtml(entry.event.payload.actorName)} deposited.</p>`,
		dependencies,
	);
}

/** An undone Accept: the gold left the colony and the coins stay in escrow, so only the GMs hear. */
async function applyDepositUndone(
	row: OutboxRow,
	dependencies: DepositRowDependencies,
): Promise<void> {
	if (!isDepositUndonePayload(row.payload)) {
		throw new Error("The undone deposit's payload wasn't understood.");
	}
	const entry = dependencies.depositLedger.get(row.payload.depositId);
	if (!entry) {
		return;
	}
	const actorName = escapeHtml(possessive(entry.event.payload.actorName));
	await whisperGameMasters(
		row,
		`<p>Colony Planner undid accepting ${actorName} ${gp(entry.gold)} deposit. The gold left the colony, and the deposit waits in the DM's inbox again: dismissing it there returns the coins.</p>`,
		dependencies,
	);
}

/** DEPOSIT_REFUND moves coins only where deposits are made: PF2e worlds. */
export function createDepositAppliers(
	dependencies: DepositRowDependencies,
	movesCoins: boolean,
): Record<"DEPOSIT_REFUND" | "DEPOSIT_UNDONE", RowApplier> {
	return {
		DEPOSIT_REFUND: movesCoins
			? (row) => applyDepositRefund(row, dependencies)
			: () =>
					Promise.reject(
						new Error(
							"This world's system can't move coins, and only a PF2e world makes deposits.",
						),
					),
		DEPOSIT_UNDONE: (row) => applyDepositUndone(row, dependencies),
	};
}
