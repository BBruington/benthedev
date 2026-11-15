import { describe, expect, test } from "bun:test";
import { selectAdapter } from "./adapters/adapter";
import {
	createDepositLedger,
	createDeposits,
	type DepositEntry,
	type DepositLink,
	type DepositState,
	isWrittenByGameMaster,
	type Pf2eDepositor,
} from "./deposits";
import type { Pf2eCoins } from "./foundry-types";
import {
	createFoundryWorld,
	eachWriteHadFreshOptions,
	type FoundryWorld,
} from "./testing/foundry-world";
import type { CoinDepositEvent, InboundEvent, OutboxRow } from "./wire";

const partyId = "abcdefghij123456";
const characterId = "characterkyra123";

function linkThat(enqueue: (event: InboundEvent) => boolean): DepositLink {
	return { enqueue, id: () => "link-1" };
}
const startedAt = new Date("2026-11-14T10:00:00.000Z");

function fakeParty(options: {
	character?: boolean;
	copper: number;
	failAdd?: boolean;
	remove?: "throw" | "short";
}) {
	const removed: [Pf2eCoins, { byValue: boolean }][] = [];
	const added: Pf2eCoins[] = [];
	const party: Pf2eDepositor = {
		id: options.character ? characterId : partyId,
		inventory: {
			addCurrency: (coins) => {
				if (options.failAdd) {
					return Promise.reject(new Error("Foundry couldn't save the coins."));
				}
				added.push(coins);
				party.inventory.currency.copperValue += (coins.gp ?? 0) * 100;
				return Promise.resolve();
			},
			currency: { copperValue: options.copper },
			removeCurrency: (coins, removeOptions) => {
				removed.push([coins, removeOptions]);
				if (options.remove === "throw") {
					return Promise.reject(new Error("Foundry lost the connection."));
				}
				if (options.remove === "short") {
					return Promise.resolve(false);
				}
				party.inventory.currency.copperValue -= (coins.gp ?? 0) * 100;
				return Promise.resolve(true);
			},
		},
		name: options.character ? "Kyra" : "The Wayfarers",
		type: options.character ? "character" : "party",
	};
	return { added, party, removed };
}

function depositsIn(world: FoundryWorld) {
	const notices: string[] = [];
	const clock = { now: startedAt };
	const deposits = createDeposits({
		actors: world.dependencies.foundryGame.actors,
		createId: () => "deposit-1",
		ledger: world.depositLedger,
		notifications: {
			error: (message) => notices.push(`error: ${message}`),
			info: (message) => notices.push(`info: ${message}`),
			warn: (message) => notices.push(`warn: ${message}`),
		},
		now: () => clock.now,
	});
	return { clock, deposits, notices };
}

function depositEvent(id = "deposit-1", gold = 40): CoinDepositEvent {
	return {
		id,
		kind: "COIN_DEPOSIT",
		occurredAt: startedAt.toISOString(),
		payload: { actorId: partyId, actorName: "The Wayfarers", gold },
	};
}

function entry(state: DepositState, overrides: Partial<DepositEntry> = {}): DepositEntry {
	const event = overrides.event ?? depositEvent();
	return {
		actorId: partyId,
		event,
		gold: event.payload.gold,
		linkId: "link-1",
		state,
		updatedAt: startedAt.toISOString(),
		...overrides,
	};
}

function fakeQueue(held: string[] = []) {
	const added: InboundEvent[] = [];
	return {
		added,
		queue: {
			add: (event: InboundEvent) => {
				added.push(event);
			},
			holds: (eventId: string) => held.includes(eventId),
		},
	};
}

describe("isWrittenByGameMaster", () => {
	const gameMasters = new Set(["gm-1"]);
	function storedSetting(stats: unknown) {
		return {
			_id: "settingabcdef123",
			_stats: stats,
			key: "colony-bridge.deposits",
			user: null,
			value: { entries: {} },
		};
	}

	test("trusts a stored Setting only when Foundry's server recorded a GM as its last writer", () => {
		const stats = {
			coreVersion: "13.351",
			createdTime: 1763100000000,
			lastModifiedBy: "gm-1",
			modifiedTime: 1763110000000,
			systemId: "pf2e",
			systemVersion: "7.12.2",
		};
		expect(isWrittenByGameMaster(storedSetting(stats), gameMasters)).toBe(true);
		expect(
			isWrittenByGameMaster(storedSetting({ ...stats, lastModifiedBy: "player-1" }), gameMasters),
		).toBe(false);
		expect(
			isWrittenByGameMaster(storedSetting({ ...stats, lastModifiedBy: null }), gameMasters),
		).toBe(false);
		expect(isWrittenByGameMaster(storedSetting({ ...stats, lastModifiedBy: 7 }), gameMasters)).toBe(
			false,
		);
		expect(isWrittenByGameMaster(storedSetting(undefined), gameMasters)).toBe(false);
	});

	test("trusts an unstored Setting, which holds only the registered default", () => {
		const unstored = {
			_id: null,
			_stats: { lastModifiedBy: null },
			key: "colony-bridge.deposits",
			value: { entries: {} },
		};
		expect(isWrittenByGameMaster(unstored, gameMasters)).toBe(true);
	});
});

describe("createDepositLedger", () => {
	function ledgerOver(initial: unknown) {
		const stored: { value: unknown } = { value: initial };
		const ledger = createDepositLedger(
			{
				lastWriterIsGm: () => true,
				load: () => stored.value,
				save: (value) => {
					stored.value = structuredClone(value);
					return Promise.resolve();
				},
			},
			{ error: () => undefined },
			() => new Date("2026-11-14T11:00:00.000Z"),
		);
		return { ledger, stored };
	}

	test("changes one entry at a time and leaves malformed or foreign entries alone", async () => {
		const others = { broken: { state: "HELD" }, kept: { note: "someone else's" } };
		const { ledger, stored } = ledgerOver({ entries: others });
		expect(await ledger.create(entry("REMOVING"))).toBe(true);
		expect(await ledger.create(entry("HELD"))).toBe(false);
		expect(await ledger.move("deposit-1", ["REMOVING"], "HELD")).toBe(true);
		expect(ledger.get("deposit-1")).toEqual({
			...entry("HELD"),
			updatedAt: "2026-11-14T11:00:00.000Z",
		});
		expect(ledger.entries()).toHaveLength(1);
		expect(await ledger.move("deposit-1", ["HELD"], null)).toBe(true);
		expect(stored.value).toEqual({ entries: others });
	});

	test("a move from a state the entry has left writes nothing, even when queued behind the write that left it", async () => {
		const { ledger } = ledgerOver({ entries: {} });
		await ledger.create(entry("HELD"));
		const [toReturning, toSent] = await Promise.all([
			ledger.move("deposit-1", ["HELD", "SENT"], "RETURNING"),
			ledger.move("deposit-1", ["HELD"], "SENT"),
		]);
		expect([toReturning, toSent]).toEqual([true, false]);
		expect(ledger.get("deposit-1")?.state).toBe("RETURNING");
		expect(await ledger.move("missing", ["HELD"], "SENT")).toBe(false);
	});

	test("an entry under another deposit's key, or naming another amount, isn't one", () => {
		const { ledger } = ledgerOver({
			entries: { "deposit-1": { ...entry("HELD"), gold: 400 }, "deposit-2": entry("HELD") },
		});
		expect(ledger.entries()).toEqual([]);
	});
});

describe("deposit", () => {
	test("takes the gold's worth from the party, holds it in escrow and queues the event", async () => {
		const world = createFoundryWorld();
		const { deposits, notices } = depositsIn(world);
		const { party, removed } = fakeParty({ copper: 5000 });
		const queued: InboundEvent[] = [];
		await deposits.deposit(
			party,
			40,
			linkThat((event) => queued.push(event) > 0),
		);
		expect(removed).toEqual([[{ gp: 40 }, { byValue: true }]]);
		expect(queued).toEqual([depositEvent()]);
		expect(world.depositLedger.get("deposit-1")).toMatchObject({
			event: depositEvent(),
			gold: 40,
			state: "HELD",
		});
		expect(notices).toEqual([
			"info: Queued The Wayfarers' 40 gp deposit for Colony Planner's inbox.",
		]);
	});

	test("a re-pair while the coins were taken interrupts the deposit at once, never queueing it", async () => {
		const world = createFoundryWorld();
		const { deposits, notices } = depositsIn(world);
		const { party } = fakeParty({ copper: 5000 });
		let linkId = "link-1";
		party.inventory.removeCurrency = () => {
			linkId = "link-2";
			party.inventory.currency.copperValue -= 4000;
			return Promise.resolve(true);
		};
		const queued: InboundEvent[] = [];
		await deposits.deposit(party, 40, {
			enqueue: (event) => queued.push(event) > 0,
			id: () => linkId,
		});
		expect(queued).toEqual([]);
		expect(world.depositLedger.get("deposit-1")?.state).toBe("INTERRUPTED");
		expect(notices).toEqual([
			"error: Colony Bridge: This world's link changed while Colony Bridge took 40 gp from The Wayfarers, so the deposit went to no campaign. Fix The Wayfarers' coins by hand, then press Forget in Colony Bridge's settings.",
		]);
	});

	test("a bridge that stopped meanwhile leaves the deposit HELD for the next session, and says so", async () => {
		const world = createFoundryWorld();
		const { deposits, notices } = depositsIn(world);
		await deposits.deposit(
			fakeParty({ copper: 5000 }).party,
			40,
			linkThat(() => false),
		);
		expect(world.depositLedger.get("deposit-1")?.state).toBe("HELD");
		expect(notices).toEqual([
			"warn: Colony Bridge: Took The Wayfarers' 40 gp deposit, and it goes to Colony Planner's inbox once the bridge runs again in the active GM's browser.",
		]);
	});

	test("a party short of the gold loses nothing and nothing is made", async () => {
		const world = createFoundryWorld();
		const { deposits, notices } = depositsIn(world);
		const { party, removed } = fakeParty({ copper: 2350 });
		await deposits.deposit(
			party,
			40,
			linkThat(() => true),
		);
		expect(removed).toEqual([]);
		expect(world.depositLedger.entries()).toEqual([]);
		expect(notices).toEqual(["warn: The Wayfarers has only 23 gp worth of coins."]);
	});

	test("a party that spent meanwhile is refused by pf2e, and the entry goes", async () => {
		const world = createFoundryWorld();
		const { deposits, notices } = depositsIn(world);
		const { party } = fakeParty({ copper: 5000, remove: "short" });
		const queued: InboundEvent[] = [];
		await deposits.deposit(
			party,
			40,
			linkThat((event) => queued.push(event) > 0),
		);
		expect(queued).toEqual([]);
		expect(world.depositLedger.entries()).toEqual([]);
		expect(notices).toEqual(["warn: The Wayfarers has only 50 gp worth of coins."]);
	});

	test("a removal that throws leaves the coins unknown: INTERRUPTED, a GM error, nothing sent", async () => {
		const world = createFoundryWorld();
		const { deposits, notices } = depositsIn(world);
		const { party } = fakeParty({ copper: 5000, remove: "throw" });
		const queued: InboundEvent[] = [];
		await deposits.deposit(
			party,
			40,
			linkThat((event) => queued.push(event) > 0),
		);
		expect(queued).toEqual([]);
		expect(world.depositLedger.get("deposit-1")?.state).toBe("INTERRUPTED");
		expect(notices).toHaveLength(1);
		expect(notices[0]).toStartWith("error: Colony Bridge: Foundry stopped part way");
		expect(notices[0]).toEndWith("Foundry lost the connection.");
		expect(deposits.interrupted()).toEqual([
			{ actorName: "The Wayfarers", depositId: "deposit-1", gold: 40 },
		]);
		await deposits.forget("deposit-1");
		expect(deposits.interrupted()).toEqual([]);
	});
});

describe("the escrow at session start and after a send", () => {
	test("resends each HELD deposit the queue doesn't hold, under its own ID", async () => {
		const world = createFoundryWorld();
		const { deposits } = depositsIn(world);
		await world.depositLedger.create(entry("HELD"));
		await world.depositLedger.create(entry("HELD", { event: depositEvent("deposit-2") }));
		await world.depositLedger.create(entry("SENT", { event: depositEvent("deposit-3") }));
		const { added, queue } = fakeQueue(["deposit-2"]);
		await deposits.resume(queue, "link-1");
		expect(added.map((event) => event.id)).toEqual(["deposit-1"]);
	});

	test("a HELD deposit made under another link is INTERRUPTED, never sent to the new link", async () => {
		const world = createFoundryWorld();
		const { deposits, notices } = depositsIn(world);
		await world.depositLedger.create(entry("HELD", { linkId: "old-link" }));
		const { added, queue } = fakeQueue();
		await deposits.resume(queue, "link-1");
		expect(added).toEqual([]);
		expect(world.depositLedger.get("deposit-1")?.state).toBe("INTERRUPTED");
		expect(notices).toEqual([
			"error: Colony Bridge: This world's link changed before Colony Planner confirmed a deposit of 40 gp from The Wayfarers, so it wasn't sent again. Check the old campaign's inbox, fix The Wayfarers' coins by hand, then press Forget in Colony Bridge's settings.",
		]);
	});

	test("a ledger a non-GM wrote last resends nothing and returns no coins", async () => {
		const world = createFoundryWorld();
		const { deposits, notices } = depositsIn(world);
		const { added, party } = fakeParty({ copper: 0 });
		world.actorsById.set(partyId, party);
		await world.depositLedger.create(entry("HELD"));
		await world.depositLedger.create(entry("REMOVING", { event: depositEvent("deposit-2") }));
		world.ledgerSetting.lastWriterIsGm = false;
		const before = structuredClone(world.ledgerSetting.value);
		const { added: resent, queue } = fakeQueue();
		await deposits.resume(queue, "link-1");
		await deposits.returnRefused([depositEvent()], "actorName is too long", 422);
		await expect(applyRow(world, refundRow)).rejects.toThrow(
			"A user who isn't a GM changed this world's deposit records last",
		);
		expect(resent).toEqual([]);
		expect(added).toEqual([]);
		expect(world.ledgerSetting.value).toEqual(before);
		expect(notices).toHaveLength(2);
		expect(notices[0]).toStartWith(
			"error: Colony Bridge: A user who isn't a GM changed this world's deposit records last",
		);
	});

	test.each(["markSent", "deposit"] as const)(
		"a GM's %s after a non-GM's write first stops every open entry, so a planted deposit is never sent or refunded",
		async (firstWrite) => {
			const world = createFoundryWorld();
			const { deposits } = depositsIn(world);
			const { added, party } = fakeParty({ copper: 5000 });
			world.actorsById.set(partyId, party);
			// The GM's own deposit-0 waits; a non-GM then adds a HELD entry under the running link.
			world.ledgerSetting.value = {
				entries: {
					"deposit-0": entry("HELD", { event: depositEvent("deposit-0") }),
					planted: entry("HELD", { event: depositEvent("planted") }),
				},
			};
			world.ledgerSetting.lastWriterIsGm = false;
			const queued: InboundEvent[] = [];
			const depositNow = () =>
				deposits.deposit(
					party,
					40,
					linkThat((event) => queued.push(event) > 0),
				);
			const markOwnSent = () => deposits.markSent([depositEvent("deposit-0")]);
			if (firstWrite === "markSent") {
				await markOwnSent();
				await depositNow();
			} else {
				await depositNow();
				await markOwnSent();
			}
			await deposits.markSent(queued);
			const { added: resent, queue } = fakeQueue();
			await deposits.resume(queue, "link-1");
			await deposits.returnRefused([depositEvent("planted")], "Too big", 422);
			await expect(
				applyRow(world, {
					...refundRow,
					payload: { ...refundPayload, depositId: "planted" },
					targetKey: "deposit-refund:planted",
				}),
			).rejects.toThrow("This deposit's return was interrupted.");
			expect(resent).toEqual([]);
			expect(added).toEqual([]);
			expect(deposits.maySend(depositEvent("planted"), "link-1")).toBe(false);
			expect(
				world.depositLedger.entries().map((stored) => [stored.event.id, stored.state]),
			).toEqual([
				["deposit-0", "INTERRUPTED"],
				["planted", "INTERRUPTED"],
				["deposit-1", "SENT"],
			]);
			expect(world.ledgerErrors).toHaveLength(1);
			expect(world.ledgerErrors[0]).toStartWith(
				"error: Colony Bridge: A user who isn't a GM changed this world's deposit records,",
			);
		},
	);

	test("a deposit records the link it was made under, and never the token", async () => {
		const world = createFoundryWorld();
		const { deposits } = depositsIn(world);
		await deposits.deposit(
			fakeParty({ copper: 5000 }).party,
			40,
			linkThat(() => true),
		);
		expect(world.depositLedger.get("deposit-1")?.linkId).toBe("link-1");
		expect(JSON.stringify(world.ledgerSetting.value)).not.toContain("secret-token");
	});

	test("marks REMOVING and RETURNING entries older than 10 minutes INTERRUPTED, once, and never retries", async () => {
		const world = createFoundryWorld();
		const { clock, deposits, notices } = depositsIn(world);
		await world.depositLedger.create(entry("REMOVING"));
		await world.depositLedger.create(entry("RETURNING", { event: depositEvent("deposit-2") }));
		clock.now = new Date(startedAt.getTime() + 5 * 60 * 1000);
		await deposits.resume(fakeQueue().queue, "link-1");
		expect(notices).toEqual([]);
		clock.now = new Date(startedAt.getTime() + 11 * 60 * 1000);
		await deposits.resume(fakeQueue().queue, "link-1");
		await deposits.resume(fakeQueue().queue, "link-1");
		expect(world.depositLedger.entries().map((stored) => stored.state)).toEqual([
			"INTERRUPTED",
			"INTERRUPTED",
		]);
		expect(notices).toHaveLength(2);
		expect(notices[0]).toContain("A deposit of 40 gp from The Wayfarers stopped part way.");
	});

	test("deletes RETURNED entries 30 days after they return, and keeps SENT ones", async () => {
		const world = createFoundryWorld();
		const { clock, deposits } = depositsIn(world);
		await world.depositLedger.create(entry("RETURNED", { returnedBy: "REFUND" }));
		await world.depositLedger.create(entry("SENT", { event: depositEvent("deposit-2") }));
		clock.now = new Date(startedAt.getTime() + 29 * 24 * 60 * 60 * 1000);
		await deposits.resume(fakeQueue().queue, "link-1");
		expect(world.depositLedger.entries()).toHaveLength(2);
		clock.now = new Date(startedAt.getTime() + 31 * 24 * 60 * 60 * 1000);
		await deposits.resume(fakeQueue().queue, "link-1");
		expect(world.depositLedger.entries().map((stored) => stored.event.id)).toEqual(["deposit-2"]);
	});

	test("a 2xx marks a HELD deposit SENT, and leaves other events and states alone", async () => {
		const world = createFoundryWorld();
		const { deposits } = depositsIn(world);
		await world.depositLedger.create(entry("HELD"));
		await world.depositLedger.create(entry("RETURNED", { event: depositEvent("deposit-2") }));
		await deposits.markSent([
			depositEvent(),
			depositEvent("deposit-2"),
			{ id: "time", kind: "TIME_ADVANCED", occurredAt: "", payload: { days: 1 } },
		]);
		expect(world.depositLedger.get("deposit-1")?.state).toBe("SENT");
		expect(world.depositLedger.get("deposit-2")?.state).toBe("RETURNED");
	});

	test("a refused deposit's coins go back to the party as gold pieces", async () => {
		const world = createFoundryWorld();
		const { deposits, notices } = depositsIn(world);
		const { added, party } = fakeParty({ copper: 0 });
		world.actorsById.set(partyId, party);
		await world.depositLedger.create(entry("HELD"));
		await deposits.returnRefused([depositEvent()], "actorName is too long", 422);
		expect(added).toEqual([{ gp: 40 }]);
		expect(world.depositLedger.get("deposit-1")).toMatchObject({
			returnedBy: "REFUSED",
			state: "RETURNED",
		});
		expect(notices).toEqual([
			"warn: Colony Bridge: Colony Planner refused the deposit (actorName is too long). Its 40 gp went back to The Wayfarers.",
		]);
	});

	test("a refused deposit this world isn't holding moves no coins and says so", async () => {
		const world = createFoundryWorld();
		const { added, party } = fakeParty({ copper: 0 });
		world.actorsById.set(partyId, party);
		const { deposits, notices } = depositsIn(world);
		await world.depositLedger.create(entry("SENT"));
		await deposits.returnRefused([depositEvent(), depositEvent("deposit-2")], "Too big", 409);
		expect(added).toEqual([]);
		expect(notices).toHaveLength(2);
		expect(notices[0]).toContain("isn't waiting to be sent, so no coins moved");
	});

	test("a 4xx but 409 or 422 may answer a stored deposit's retry, so its coins stay held", async () => {
		const world = createFoundryWorld();
		const { deposits, notices } = depositsIn(world);
		const { added, party } = fakeParty({ copper: 0 });
		world.actorsById.set(partyId, party);
		await world.depositLedger.create(entry("HELD"));
		await deposits.returnRefused([depositEvent()], "Not Found", 404);
		expect(added).toEqual([]);
		expect(world.depositLedger.get("deposit-1")?.state).toBe("HELD");
		expect(notices).toEqual([
			"error: Colony Bridge: Colony Planner answered a deposit of 40 gp with 404 (Not Found), so this world can't tell whether it was recorded. Its coins stay held, and the bridge sends it again when it next starts.",
		]);
		const { added: resent, queue } = fakeQueue();
		await deposits.resume(queue, "link-1");
		expect(resent.map((event) => event.id)).toEqual(["deposit-1"]);
	});

	test("only a deposit HELD under this link may leave the queue", async () => {
		const world = createFoundryWorld();
		const { deposits } = depositsIn(world);
		await world.depositLedger.create(entry("HELD"));
		await world.depositLedger.create(
			entry("HELD", { event: depositEvent("deposit-2"), linkId: "old-link" }),
		);
		await world.depositLedger.create(entry("INTERRUPTED", { event: depositEvent("deposit-3") }));
		await world.depositLedger.create(
			entry("RETURNED", { event: depositEvent("deposit-4"), returnedBy: "REFUND" }),
		);
		const maySend = (id: string) => deposits.maySend(depositEvent(id), "link-1");
		expect(["deposit-1", "deposit-2", "deposit-3", "deposit-4", "missing"].map(maySend)).toEqual([
			true,
			false,
			false,
			false,
			false,
		]);
		expect(
			deposits.maySend(
				{ id: "time", kind: "TIME_ADVANCED", occurredAt: "", payload: { days: 1 } },
				"link-1",
			),
		).toBe(true);
	});

	test("a refused deposit whose party is gone is INTERRUPTED for the GM to fix", async () => {
		const world = createFoundryWorld();
		const { deposits, notices } = depositsIn(world);
		await world.depositLedger.create(entry("HELD"));
		await deposits.returnRefused([depositEvent()], "Too big", 422);
		expect(world.depositLedger.get("deposit-1")?.state).toBe("INTERRUPTED");
		expect(notices).toEqual([
			"error: Colony Bridge: Colony Planner refused a deposit of 40 gp (Too big), and Foundry couldn't give the coins back. The actor that paid this deposit is gone. Add 40 gp by hand.",
		]);
	});
});

const refundPayload = {
	actorId: partyId,
	actorName: "Forged <b>",
	depositId: "deposit-1",
	gold: 40,
};

const refundRow: OutboxRow = {
	kind: "DEPOSIT_REFUND",
	payload: refundPayload,
	seq: 9,
	targetKey: "deposit-refund:deposit-1",
};

const undoneRow: OutboxRow = {
	kind: "DEPOSIT_UNDONE",
	payload: { actorName: "The Wayfarers", depositId: "deposit-1", gold: 40 },
	seq: 10,
	targetKey: "deposit-undone:group-7",
};

function applyRow(world: FoundryWorld, row: OutboxRow, systemId = "pf2e") {
	const applier = selectAdapter(systemId, world.dependencies).appliers[row.kind];
	if (!applier) {
		throw new Error(`No applier for ${row.kind}.`);
	}
	return applier(row);
}

describe("DEPOSIT_REFUND", () => {
	test("adds the gold back once, records RETURNED and whispers the GMs once", async () => {
		const world = createFoundryWorld();
		const { added, party } = fakeParty({ copper: 0 });
		world.actorsById.set(partyId, party);
		await world.depositLedger.create(entry("SENT"));
		await applyRow(world, refundRow);
		await applyRow(world, refundRow);
		expect(added).toEqual([{ gp: 40 }]);
		expect(world.depositLedger.get("deposit-1")).toMatchObject({
			returnedBy: "REFUND",
			state: "RETURNED",
		});
		expect(world.messages).toHaveLength(1);
		expect(world.messages[0]?.data).toMatchObject({
			content: "<p>Colony Planner returned the 40 gp The Wayfarers deposited.</p>",
			flags: { "colony-bridge": { externalId: "deposit-refund:deposit-1" } },
			whisper: ["gm-1", "gm-2"],
		});
		expect(eachWriteHadFreshOptions(world)).toBe(true);
	});

	test("a 2xx arriving mid-refund can't undo RETURNING, so a replay adds nothing", async () => {
		const world = createFoundryWorld();
		const added: Pf2eCoins[] = [];
		let finishAdd: (() => void) | undefined;
		const { party } = fakeParty({ copper: 0 });
		party.inventory.addCurrency = (coins) => {
			added.push(coins);
			return new Promise((resolve) => {
				finishAdd = () => resolve(undefined);
			});
		};
		world.actorsById.set(partyId, party);
		await world.depositLedger.create(entry("HELD"));
		const refund = applyRow(world, refundRow);
		await new Promise((resolve) => setTimeout(resolve, 1));
		await depositsIn(world).deposits.markSent([depositEvent()]);
		expect(world.depositLedger.get("deposit-1")?.state).toBe("RETURNING");
		finishAdd?.();
		await refund;
		await applyRow(world, refundRow);
		expect(added).toEqual([{ gp: 40 }]);
		expect(world.depositLedger.get("deposit-1")?.state).toBe("RETURNED");
	});

	test("a HELD deposit (the 2xx was lost) is refunded too", async () => {
		const world = createFoundryWorld();
		const { added, party } = fakeParty({ copper: 0 });
		world.actorsById.set(partyId, party);
		await world.depositLedger.create(entry("HELD"));
		await applyRow(world, refundRow);
		expect(added).toEqual([{ gp: 40 }]);
	});

	test.each(["RETURNING", "INTERRUPTED", "REMOVING"] as const)(
		"a %s deposit's return adds nothing and fails",
		async (state) => {
			const world = createFoundryWorld();
			const { added, party } = fakeParty({ copper: 0 });
			world.actorsById.set(partyId, party);
			await world.depositLedger.create(entry(state));
			await expect(applyRow(world, refundRow)).rejects.toThrow(
				"This deposit's return was interrupted. Check the paying actor's coins and fix them by hand.",
			);
			expect(added).toEqual([]);
		},
	);

	test("a deposit this world doesn't hold, or one that doesn't match, adds nothing", async () => {
		const world = createFoundryWorld();
		const { added, party } = fakeParty({ copper: 0 });
		world.actorsById.set(partyId, party);
		await expect(applyRow(world, refundRow)).rejects.toThrow(
			"This world has no record of that deposit, so no coins were added.",
		);
		await world.depositLedger.create(entry("SENT"));
		await expect(
			applyRow(world, { ...refundRow, payload: { ...refundPayload, gold: 400 } }),
		).rejects.toThrow("doesn't match this world's record");
		await expect(
			applyRow(world, {
				...refundRow,
				payload: { ...refundPayload, actorId: "zzzzzzzzzzzzzzzz" },
			}),
		).rejects.toThrow("doesn't match this world's record");
		await expect(
			applyRow(world, { ...refundRow, targetKey: "deposit-refund:other" }),
		).rejects.toThrow("target key");
		expect(added).toEqual([]);
		expect(world.messages).toEqual([]);
	});

	test("a party that is gone fails the row and leaves the entry as it was", async () => {
		const world = createFoundryWorld();
		await world.depositLedger.create(entry("SENT"));
		await expect(applyRow(world, refundRow)).rejects.toThrow(
			"The actor that paid this deposit is gone. Add 40 gp by hand.",
		);
		expect(world.depositLedger.get("deposit-1")).toEqual(entry("SENT"));
	});

	test("an add that throws is INTERRUPTED and fails with Foundry's reason", async () => {
		const world = createFoundryWorld();
		world.actorsById.set(partyId, fakeParty({ copper: 0, failAdd: true }).party);
		await world.depositLedger.create(entry("SENT"));
		await expect(applyRow(world, refundRow)).rejects.toThrow("Foundry couldn't save the coins.");
		expect(world.depositLedger.get("deposit-1")?.state).toBe("INTERRUPTED");
	});

	test("a world whose system can't move coins fails the row", async () => {
		const world = createFoundryWorld();
		await world.depositLedger.create(entry("SENT"));
		await expect(applyRow(world, refundRow, "dnd5e")).rejects.toThrow(
			"This world's system can't move coins, and only a PF2e world makes deposits.",
		);
	});
});

describe("DEPOSIT_UNDONE", () => {
	test.each(["pf2e", "dnd5e"])(
		"%s whispers the GMs once when this world holds the deposit",
		async (systemId) => {
			const world = createFoundryWorld();
			await world.depositLedger.create(entry("SENT"));
			await applyRow(world, undoneRow, systemId);
			await applyRow(world, undoneRow, systemId);
			expect(world.messages).toHaveLength(1);
			expect(world.messages[0]?.data).toMatchObject({
				content:
					"<p>Colony Planner undid accepting The Wayfarers&#39; 40 gp deposit. The gold left the colony, and the deposit waits in the DM's inbox again: dismissing it there returns the coins.</p>",
				flags: { "colony-bridge": { externalId: "deposit-undone:group-7" } },
				whisper: ["gm-1", "gm-2"],
			});
		},
	);

	test("succeeds with no message when this world doesn't hold the deposit", async () => {
		const world = createFoundryWorld();
		await applyRow(world, undoneRow);
		expect(world.messages).toEqual([]);
	});

	test("a payload it doesn't understand fails", async () => {
		const world = createFoundryWorld();
		await expect(applyRow(world, { ...undoneRow, payload: { gold: "40" } })).rejects.toThrow(
			"The undone deposit's payload wasn't understood.",
		);
	});
});

describe("a PF2e character's deposit", () => {
	test("takes the character's own coins, and a refund puts them back on that character", async () => {
		const world = createFoundryWorld();
		const { deposits, notices } = depositsIn(world);
		const { added, party: character, removed } = fakeParty({ character: true, copper: 1500 });
		world.actorsById.set(characterId, character);
		const queued: InboundEvent[] = [];
		await deposits.deposit(
			character,
			12,
			linkThat((event) => queued.push(event) > 0),
		);
		expect(removed).toEqual([[{ gp: 12 }, { byValue: true }]]);
		expect(queued.map((event) => event.payload)).toEqual([
			{ actorId: characterId, actorName: "Kyra", gold: 12 },
		]);
		expect(notices).toEqual(["info: Queued Kyra's 12 gp deposit for Colony Planner's inbox."]);
		await deposits.markSent(queued);
		await applyRow(world, {
			...refundRow,
			payload: { actorId: characterId, actorName: "Kyra", depositId: "deposit-1", gold: 12 },
		});
		expect(added).toEqual([{ gp: 12 }]);
		expect(world.depositLedger.get("deposit-1")).toMatchObject({
			actorId: characterId,
			returnedBy: "REFUND",
			state: "RETURNED",
		});
		expect(world.messages[0]?.data).toMatchObject({
			content: "<p>Colony Planner returned the 12 gp Kyra deposited.</p>",
		});
	});

	test("a refused deposit goes back to the character that paid, and a short one names it", async () => {
		const world = createFoundryWorld();
		const { deposits, notices } = depositsIn(world);
		const { added, party: character } = fakeParty({ character: true, copper: 1500 });
		world.actorsById.set(characterId, character);
		await deposits.deposit(
			character,
			20,
			linkThat(() => true),
		);
		const queued: InboundEvent[] = [];
		await deposits.deposit(
			character,
			12,
			linkThat((event) => queued.push(event) > 0),
		);
		await deposits.returnRefused(queued, "Too big", 409);
		expect(added).toEqual([{ gp: 12 }]);
		expect(character.inventory.currency.copperValue).toBe(1500);
		expect(notices).toEqual([
			"warn: Kyra has only 15 gp worth of coins.",
			"info: Queued Kyra's 12 gp deposit for Colony Planner's inbox.",
			"warn: Colony Bridge: Colony Planner refused the deposit (Too big). Its 12 gp went back to Kyra.",
		]);
	});
});
