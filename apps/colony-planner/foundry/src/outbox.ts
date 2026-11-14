import type { BridgeClient } from "./bridge-client";
import { type FailedRow, maximumFailureReasonLength, type OutboxRow } from "./wire";

export type RowApplier = (row: OutboxRow) => Promise<void>;

export interface OutboxCursor {
	load(): number;
	save(seq: number): Promise<unknown>;
}

/**
 * A withdraw row that failed: acknowledging it would leave its entry in players' view for good, so
 * the sync stops before it and the row, and every row after it, waits for the next sync.
 */
export class WithdrawFailedError extends Error {}

const withdrawKindSuffix = "_WITHDRAW";

/**
 * Reads the outbox from the last acknowledged seq until `more` is false, applies each page's rows in
 * order and acknowledges the page (design §4.2). A row that fails is acknowledged as failed with its
 * reason, so Colony Planner shows it to the DM and it is never retried in a loop, except a withdraw:
 * that acknowledges only the rows before it and throws WithdrawFailedError, so the caller retries it
 * after a backoff. The cursor moves only after an acknowledgement is accepted, so a lost one means the
 * page is applied again, which the kinds allow (state rows replace, one-shot rows check their flag).
 */
export async function syncOutbox(options: {
	appliers: Partial<Record<string, RowApplier>>;
	client: Pick<BridgeClient, "acknowledge" | "outbox">;
	cursor: OutboxCursor;
	/** Aborted when the session ends, so a sync of an old link never writes over a new link's cursor. */
	signal: AbortSignal;
}): Promise<void> {
	const page = await options.client.outbox(options.cursor.load());
	const result = await applyInOrder(page.rows, options.appliers, options.signal);
	if (result.appliedThrough !== undefined) {
		options.signal.throwIfAborted();
		await options.client.acknowledge({ failed: result.failed, through: result.appliedThrough });
		options.signal.throwIfAborted();
		await options.cursor.save(result.appliedThrough);
	}
	if (result.withdrawFailure) {
		throw result.withdrawFailure;
	}
	if (page.more) {
		await syncOutbox(options);
	}
}

interface AppliedRows {
	/** The last row handled, applied or failed; undefined when none was. */
	appliedThrough: number | undefined;
	failed: FailedRow[];
	withdrawFailure: WithdrawFailedError | undefined;
}

async function applyInOrder(
	rows: OutboxRow[],
	appliers: Partial<Record<string, RowApplier>>,
	signal: AbortSignal,
): Promise<AppliedRows> {
	const result: AppliedRows = { appliedThrough: undefined, failed: [], withdrawFailure: undefined };
	// One row at a time, oldest first: a later row may describe the same target as an earlier one.
	await rows.reduce(async (previousRow, row) => {
		await previousRow;
		if (result.withdrawFailure) {
			return;
		}
		signal.throwIfAborted();
		const applier = appliers[row.kind];
		const reason = await applyRow(row, applier);
		if (reason !== undefined && applier && row.kind.endsWith(withdrawKindSuffix)) {
			result.withdrawFailure = new WithdrawFailedError(
				`Foundry couldn't hide a journal entry Colony Planner hid, so the bridge waits to sync until it can. ${reason}`,
			);
			return;
		}
		if (reason !== undefined) {
			result.failed.push({ reason, seq: row.seq });
		}
		result.appliedThrough = row.seq;
	}, Promise.resolve());
	return result;
}

async function applyRow(
	row: OutboxRow,
	applier: RowApplier | undefined,
): Promise<string | undefined> {
	if (!applier) {
		return `This version of Colony Bridge doesn't know ${row.kind} rows. Update the module.`;
	}
	try {
		await applier(row);
		return undefined;
	} catch (error) {
		const message = error instanceof Error && error.message ? error.message : String(error);
		return message.slice(0, maximumFailureReasonLength);
	}
}
