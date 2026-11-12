import type { BridgeClient } from "./bridge-client";
import { type FailedRow, maximumFailureReasonLength, type OutboxRow } from "./wire";

export type RowApplier = (row: OutboxRow) => Promise<void>;

export interface OutboxCursor {
	load(): number;
	save(seq: number): Promise<unknown>;
}

/**
 * Reads the outbox from the last acknowledged seq until `more` is false, applies each page's rows in
 * order and acknowledges the page (design §4.2). A row that fails is acknowledged as failed with its
 * reason, so Colony Planner shows it to the DM and it is never retried in a loop. The cursor moves only
 * after an acknowledgement is accepted, so a lost one means the page is applied again, which the kinds
 * allow (state rows replace, one-shot rows check their flag).
 */
export async function syncOutbox(options: {
	appliers: Partial<Record<string, RowApplier>>;
	client: Pick<BridgeClient, "acknowledge" | "outbox">;
	cursor: OutboxCursor;
	/** Aborted when the session ends, so a sync of an old link never writes over a new link's cursor. */
	signal: AbortSignal;
}): Promise<void> {
	const page = await options.client.outbox(options.cursor.load());
	const lastRow = page.rows.at(-1);
	if (!lastRow) {
		return;
	}
	const failed = await applyInOrder(page.rows, options.appliers, options.signal);
	options.signal.throwIfAborted();
	await options.client.acknowledge({ failed, through: lastRow.seq });
	options.signal.throwIfAborted();
	await options.cursor.save(lastRow.seq);
	if (page.more) {
		await syncOutbox(options);
	}
}

async function applyInOrder(
	rows: OutboxRow[],
	appliers: Partial<Record<string, RowApplier>>,
	signal: AbortSignal,
): Promise<FailedRow[]> {
	const failed: FailedRow[] = [];
	// One row at a time, oldest first: a later row may describe the same target as an earlier one.
	await rows.reduce(async (previousRow, row) => {
		await previousRow;
		signal.throwIfAborted();
		const reason = await applyRow(row, appliers[row.kind]);
		if (reason !== undefined) {
			failed.push({ reason, seq: row.seq });
		}
	}, Promise.resolve());
	return failed;
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
