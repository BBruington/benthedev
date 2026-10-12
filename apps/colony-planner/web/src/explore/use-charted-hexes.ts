import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import { api } from "@/api/client";
import { problemMessage } from "@/api/problem";
import { hexesQuery } from "@/api/queries";
import type { ChartedHex } from "./hex-canvas";
import { type HexCoordinate, hexKey } from "./hex-math";
import {
	batchStrokeQueue,
	emptyStrokeQueue,
	type QueuedCell,
	queueCells,
	type StrokeQueue,
	type StrokeTerrain,
	strokeSendIntervalInMilliseconds,
} from "./stroke-queue";

/** At most one hexes refetch a second while strokes stream in (spec §5.10, §8.8). */
export const hexesRefetchIntervalInMilliseconds = 1000;

/** A local stroke shown before the server confirms it; settledAt is when its call succeeded. */
type PendingCell = { q: number; r: number; terrain: StrokeTerrain; settledAt: number | undefined };

function waitUntil(time: number, signal: AbortSignal): Promise<void> {
	const delay = time - performance.now();
	if (delay <= 0) {
		return Promise.resolve();
	}
	return new Promise((resolve, reject) => {
		const timer = setTimeout(resolve, delay);
		signal.addEventListener(
			"abort",
			() => {
				clearTimeout(timer);
				reject(signal.reason);
			},
			{ once: true },
		);
	});
}

/**
 * The charted hexes with this tab's strokes laid over them until a read that started after each
 * stroke was saved shows it, plus the stroke sender: cells queue up and go out every 250 ms and when
 * the finger lifts, in calls of at most 200 cells, one call at a time so a later stroke over the same
 * hex lands last.
 */
export function useChartedHexes(campaignId: string) {
	const queryClient = useQueryClient();
	const lastRequestStartedAtRef = useRef(Number.NEGATIVE_INFINITY);
	const shownRequestStartedAtRef = useRef(Number.NEGATIVE_INFINITY);
	const options = useMemo(() => {
		const base = hexesQuery(campaignId);
		return {
			...base,
			queryFn: async (context: Parameters<typeof base.queryFn>[0]) => {
				// Live updates mark the read stale without aborting it and read once more after it lands, so
				// the wait is measured from the last read that went out, not from this one.
				await waitUntil(
					lastRequestStartedAtRef.current + hexesRefetchIntervalInMilliseconds,
					context.signal,
				);
				const startedAt = performance.now();
				lastRequestStartedAtRef.current = startedAt;
				const result = await base.queryFn(context);
				if (!context.signal.aborted) {
					shownRequestStartedAtRef.current = startedAt;
				}
				return result;
			},
		};
	}, [campaignId]);
	const hexes = useQuery(options);

	const pendingRef = useRef(new Map<string, PendingCell>());
	const queueRef = useRef<StrokeQueue>(emptyStrokeQueue);
	const sendTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
	const isSendingRef = useRef(false);
	const redrawRequestRef = useRef(0);
	const [pendingVersion, setPendingVersion] = useState(0);

	const requestRedraw = useCallback(() => {
		if (redrawRequestRef.current !== 0) {
			return;
		}
		// Many pointer moves land in one frame; the merged map is rebuilt at most once per frame.
		redrawRequestRef.current = requestAnimationFrame(() => {
			redrawRequestRef.current = 0;
			setPendingVersion((version) => version + 1);
		});
	}, []);

	useEffect(() => () => cancelAnimationFrame(redrawRequestRef.current), []);

	// pendingRef changes in place, so pendingVersion is what tells this to merge again.
	const displayed = useMemo(() => {
		const byKey = new Map<string, ChartedHex>();
		for (const hex of hexes.data?.hexes ?? []) {
			byKey.set(hexKey(hex.q, hex.r), hex);
		}
		for (const [key, pending] of pendingRef.current) {
			if (pending.terrain === null) {
				byKey.delete(key);
				continue;
			}
			const label = byKey.get(key)?.label ?? "";
			byKey.set(key, { label, q: pending.q, r: pending.r, terrain: pending.terrain });
		}
		return { byKey, hexes: Array.from(byKey.values()), version: pendingVersion };
	}, [hexes.data, pendingVersion]);
	const displayedRef = useRef(displayed);
	displayedRef.current = displayed;

	useEffect(() => {
		if (!hexes.data) {
			return;
		}
		let pruned = false;
		for (const [key, pending] of pendingRef.current) {
			if (
				pending.settledAt !== undefined &&
				pending.settledAt <= shownRequestStartedAtRef.current
			) {
				pendingRef.current.delete(key);
				pruned = true;
			}
		}
		if (pruned) {
			requestRedraw();
		}
	}, [hexes.data, requestRedraw]);

	const paintMutation = api.useMutation("post", "/api/campaigns/{campaignId}/hexes/paint");
	const eraseMutation = api.useMutation("post", "/api/campaigns/{campaignId}/hexes/erase");
	const sendersRef = useRef({ erase: eraseMutation.mutateAsync, paint: paintMutation.mutateAsync });
	sendersRef.current = { erase: eraseMutation.mutateAsync, paint: paintMutation.mutateAsync };

	const settleCells = useCallback(
		(cells: readonly QueuedCell[], saved: boolean) => {
			const now = performance.now();
			for (const cell of cells) {
				const key = hexKey(cell.q, cell.r);
				const pending = pendingRef.current.get(key);
				// A newer stroke over this hex owns the entry now; it settles with its own call.
				if (!pending || pending.settledAt !== undefined || pending.terrain !== cell.terrain) {
					continue;
				}
				if (saved) {
					pendingRef.current.set(key, { ...pending, settledAt: now });
				} else {
					pendingRef.current.delete(key);
				}
			}
			if (!saved) {
				requestRedraw();
			}
		},
		[requestRedraw],
	);

	const sendQueue = useCallback(
		(queue: StrokeQueue) => {
			const calls = batchStrokeQueue(queue);
			const path = { params: { path: { campaignId } } };
			let failed = false;
			const onFailure = (error: unknown) => {
				if (!failed) {
					failed = true;
					toast.error(
						problemMessage(error, "Those hexes couldn't be saved. Try painting them again."),
					);
				}
				queryClient.invalidateQueries({ queryKey: hexesQuery(campaignId).queryKey });
			};
			const settleAfter = (cells: readonly QueuedCell[], call: Promise<unknown>) =>
				call
					.then(() => settleCells(cells, true))
					.catch((error: unknown) => {
						settleCells(cells, false);
						onFailure(error);
					});
			const sends: (() => Promise<void>)[] = [
				...calls.erase.map(
					(cells) => () =>
						settleAfter(
							cells.map((cell) => ({ ...cell, terrain: null })),
							sendersRef.current.erase({ ...path, body: { cells } }),
						),
				),
				...calls.paint.map(
					(cells) => () =>
						settleAfter(cells, sendersRef.current.paint({ ...path, body: { cells } })),
				),
			];
			return sends.reduce<Promise<void>>(
				(previous, send) => previous.then(send),
				Promise.resolve(),
			);
		},
		[campaignId, queryClient, settleCells],
	);

	const flush = useCallback(() => {
		clearTimeout(sendTimerRef.current);
		sendTimerRef.current = undefined;
		if (isSendingRef.current || queueRef.current.size === 0) {
			return;
		}
		const queue = queueRef.current;
		queueRef.current = emptyStrokeQueue;
		isSendingRef.current = true;
		sendQueue(queue).finally(() => {
			isSendingRef.current = false;
			if (queueRef.current.size > 0) {
				flush();
			}
		});
	}, [sendQueue]);

	/** Lays the brush on these hexes now and queues them; hexes that already match are skipped. */
	const paint = useCallback(
		(cells: readonly HexCoordinate[], terrain: StrokeTerrain) => {
			const changed = cells.filter((cell) => {
				const current = displayedRef.current.byKey.get(hexKey(cell.q, cell.r));
				const pending = pendingRef.current.get(hexKey(cell.q, cell.r));
				const shownTerrain = pending ? pending.terrain : (current?.terrain ?? null);
				return shownTerrain !== terrain;
			});
			if (changed.length === 0) {
				return;
			}
			for (const cell of changed) {
				pendingRef.current.set(hexKey(cell.q, cell.r), {
					q: cell.q,
					r: cell.r,
					settledAt: undefined,
					terrain,
				});
			}
			queueRef.current = queueCells(queueRef.current, changed, terrain);
			requestRedraw();
			sendTimerRef.current ??= setTimeout(flush, strokeSendIntervalInMilliseconds);
		},
		[flush, requestRedraw],
	);

	useEffect(() => () => flush(), [flush]);

	return { displayed, flush, hexes, paint };
}
