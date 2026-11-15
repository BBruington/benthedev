import type { AdapterDependencies } from "../adapters/adapter";
import { createDepositLedger } from "../deposits";
import type {
	DocumentOwnership,
	FoundryChatMessage,
	FoundryCollection,
	FoundryFolder,
	FoundryItem,
	FoundryJournalEntry,
	FoundryJournalPage,
	FoundryPartyActor,
	FoundryUser,
} from "../foundry-types";
import { readField } from "../wire";
import { createFakeLocks } from "./fakes";

/** Each Foundry write the fake world can be told to refuse once, as Foundry's server would. */
export type WriteKind =
	| "chat create"
	| "chat update"
	| "entry create"
	| "entry update"
	| "entries update"
	| "folder create"
	| "items create"
	| "page create"
	| "page update";

export interface RecordedWrite {
	data: unknown;
	kind: WriteKind;
	/** The options object as the bridge passed it, before the fake stamps it as Foundry does. */
	options: unknown;
	optionsObject: object | undefined;
	target: string;
}

export interface FakeDocument {
	_stats?: { lastModifiedBy?: string | null } | null;
	author: FoundryUser | null;
	data: Record<string, unknown>;
	getFlag(scope: string, key: string): unknown;
	id: string;
	name: string;
	ownership?: DocumentOwnership;
	text?: { content?: string | null } | null;
	update(changes: object, options?: object): Promise<unknown>;
}

export interface FakeEntry extends FakeDocument, FoundryJournalEntry {
	pageList: FakeDocument[];
}

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === "object" && value !== null && !Array.isArray(value);
}

function childRecord(node: Record<string, unknown>, key: string): Record<string, unknown> {
	const existing = node[key];
	if (isRecord(existing)) {
		return existing;
	}
	const created: Record<string, unknown> = {};
	node[key] = created;
	return created;
}

/** Applies a Foundry update: dotted keys set one path, and nested objects merge. */
function mergeInto(target: Record<string, unknown>, changes: object): void {
	for (const [path, value] of Object.entries(changes)) {
		const keys = path.split(".");
		const last = keys.pop() ?? path;
		const node = keys.reduce(childRecord, target);
		const existing = node[last];
		if (isRecord(value) && isRecord(existing)) {
			mergeInto(existing, value);
		} else {
			node[last] = structuredClone(value);
		}
	}
}

function withoutKey(data: object, key: string): Record<string, unknown> {
	return Object.fromEntries(Object.entries(data).filter(([field]) => field !== key));
}

function collection<Document>(documents: Document[]): FoundryCollection<Document> {
	return {
		filter: (predicate) => documents.filter(predicate),
		find: (predicate) => documents.find(predicate),
	};
}

export interface CompendiumItemOptions {
	documentName?: string;
	quantity?: unknown;
}

export function createFoundryWorld(options: { party?: boolean; users?: FoundryUser[] } = {}) {
	const writes: RecordedWrite[] = [];
	const failNext = new Set<WriteKind>();
	// A hook returning false cancels a write: Foundry resolves with nothing instead of throwing.
	const cancelNext = new Set<WriteKind>();
	const entries: FakeEntry[] = [];
	const folders: FakeDocument[] = [];
	const messages: FakeDocument[] = [];
	const partyItems: FakeDocument[] = [];
	const compendium = new Map<string, FoundryItem>();
	const users: FoundryUser[] = options.users ?? [
		{ id: "gm-1", isGM: true },
		{ id: "player-1", isGM: false },
		{ id: "gm-2", isGM: true },
	];
	let nextId = 1;

	/** Records the write, or reports a cancelled one by returning false. */
	function write(kind: WriteKind, target: string, data: unknown, writeOptions?: object): boolean {
		if (failNext.delete(kind)) {
			throw new Error(`Foundry refused the ${kind}.`);
		}
		if (cancelNext.delete(kind)) {
			return false;
		}
		writes.push({
			data: structuredClone(data),
			kind,
			options: structuredClone(writeOptions),
			optionsObject: writeOptions,
			target,
		});
		// Foundry's Document API writes into the options object it was given.
		Object.assign(writeOptions ?? {}, { parent: target, render: true });
		return true;
	}

	const actingGm = users.find((user) => user.isGM) ?? null;

	/** Read live from `data`, as Foundry's documents expose their fields. */
	function fakeDocument(kind: WriteKind, data: object, author = actingGm): FakeDocument {
		const id = `document${nextId++}`;
		const state: Record<string, unknown> = structuredClone({ ...data });
		const numbers = (value: unknown) =>
			isRecord(value)
				? Object.fromEntries(
						Object.entries(value).map(([key, level]) => [
							key,
							typeof level === "number" ? level : undefined,
						]),
					)
				: {};
		return {
			get _stats() {
				const stats = state._stats;
				const lastModifiedBy = readField(stats, "lastModifiedBy");
				return isRecord(stats)
					? { lastModifiedBy: typeof lastModifiedBy === "string" ? lastModifiedBy : null }
					: null;
			},
			author,
			data: state,
			getFlag: (scope, key) => readField(readField(state.flags, scope), key),
			id,
			get name() {
				return typeof state.name === "string" ? state.name : "";
			},
			get ownership() {
				return numbers(state.ownership);
			},
			get text() {
				const content = readField(state.text, "content");
				return isRecord(state.text)
					? { content: typeof content === "string" ? content : null }
					: null;
			},
			update: (changes, writeOptions) => {
				try {
					if (!write(kind, id, changes, writeOptions)) {
						return Promise.resolve(undefined);
					}
				} catch (error) {
					return Promise.reject(error);
				}
				mergeInto(state, changes);
				return Promise.resolve(changes);
			},
		};
	}

	function fakeEntry(data: object): FakeEntry {
		const pages: unknown = readField(data, "pages");
		const document = fakeDocument("entry update", withoutKey(data, "pages"));
		const pageList: FakeDocument[] = Array.isArray(pages)
			? pages.filter(isRecord).map((page) => fakeDocument("page update", page))
			: [];
		const embedded: Pick<FakeEntry, "createEmbeddedDocuments" | "pageList" | "pages"> = {
			createEmbeddedDocuments: (_name, pageData, writeOptions) => {
				try {
					if (!write("page create", document.id, pageData, writeOptions)) {
						return Promise.resolve([]);
					}
				} catch (error) {
					return Promise.reject(error);
				}
				const created = pageData.map((page) => fakeDocument("page update", page));
				pageList.push(...created);
				return Promise.resolve(created);
			},
			pageList,
			pages: collection<FoundryJournalPage>(pageList),
		};
		// Assigned onto the document, not spread, so its live getters stay live.
		return Object.assign(document, embedded);
	}

	function createDocument<Document extends FakeDocument>(
		kind: WriteKind,
		data: object,
		writeOptions: object | undefined,
		make: (data: object) => Document,
		list: Document[],
	): Promise<Document | undefined> {
		try {
			if (!write(kind, "world", data, writeOptions)) {
				return Promise.resolve(undefined);
			}
		} catch (error) {
			return Promise.reject(error);
		}
		const document = make(data);
		list.push(document);
		return Promise.resolve(document);
	}

	const party: FoundryPartyActor = {
		createEmbeddedDocuments: (_name, itemData, writeOptions) => {
			try {
				if (!write("items create", "party", itemData, writeOptions)) {
					return Promise.resolve([]);
				}
			} catch (error) {
				return Promise.reject(error);
			}
			const created = itemData.map((item) => fakeDocument("entry update", item));
			partyItems.push(...created);
			return Promise.resolve(created);
		},
		items: collection<FakeDocument>(partyItems),
		name: "The <Wardens>",
	};
	const actorsById = new Map<string, unknown>();
	const actors = {
		get: (id: string) => actorsById.get(id),
		party: options.party === false ? null : party,
	};
	// The world setting's value; every module write is the GM's, until a test says a player wrote last.
	const ledgerSetting: { lastWriterIsGm: boolean; value: unknown } = {
		lastWriterIsGm: true,
		value: { entries: {} },
	};
	const ledgerErrors: string[] = [];
	const depositLedger = createDepositLedger(
		{
			lastWriterIsGm: () => ledgerSetting.lastWriterIsGm,
			load: () => ledgerSetting.value,
			save: (value) => {
				ledgerSetting.value = structuredClone(value);
				ledgerSetting.lastWriterIsGm = true;
				return Promise.resolve();
			},
		},
		{ error: (message) => ledgerErrors.push(`error: ${message}`) },
		createFakeLocks(),
	);

	const dependencies: AdapterDependencies = {
		chatMessage: {
			create: (data, writeOptions) =>
				createDocument(
					"chat create",
					data,
					writeOptions,
					(messageData) => fakeDocument("chat update", messageData),
					messages,
				),
		},
		depositLedger,
		folder: {
			create: (data, writeOptions) =>
				createDocument(
					"folder create",
					data,
					writeOptions,
					(folderData) => fakeDocument("entry update", folderData),
					folders,
				),
		},
		foundryGame: {
			actors,
			folders: collection<FoundryFolder>(folders),
			journal: collection<FoundryJournalEntry>(entries),
			messages: collection<FoundryChatMessage>(messages),
			time: { advance: () => Promise.resolve(), worldTime: 0 },
			users: { activeGM: users[0] ?? null, filter: (predicate) => users.filter(predicate) },
		},
		fromUuid: (uuid) => {
			if (uuid.includes("broken-pack")) {
				return Promise.reject(new Error("Foundry couldn't parse that UUID."));
			}
			return Promise.resolve(compendium.get(uuid) ?? null);
		},
		journalEntry: {
			create: (data, writeOptions) =>
				createDocument("entry create", data, writeOptions, fakeEntry, entries),
			updateDocuments: (updates, writeOptions) => {
				try {
					if (!write("entries update", "journal", updates, writeOptions)) {
						return Promise.resolve([]);
					}
				} catch (error) {
					return Promise.reject(error);
				}
				const updated = updates.flatMap((update) => {
					const entry = entries.find((candidate) => candidate.id === readField(update, "_id"));
					if (!entry) {
						return [];
					}
					mergeInto(entry.data, withoutKey(update, "_id"));
					return [entry];
				});
				return Promise.resolve(updated);
			},
		},
		moveClockWithColonyDays: () => false,
	};

	return {
		actors,
		actorsById,
		addCompendiumItem(uuid: string, name: string, itemOptions: CompendiumItemOptions = {}) {
			const source = {
				_id: uuid.slice(-16),
				flags: { core: { sheetClass: "" } },
				name,
				system: {
					price: { value: { gp: 5 } },
					quantity: "quantity" in itemOptions ? itemOptions.quantity : 1,
				},
				type: "equipment",
			};
			compendium.set(uuid, {
				documentName: itemOptions.documentName ?? "Item",
				getFlag: () => undefined,
				system: source.system,
				toObject: () => structuredClone(source),
			});
		},
		/** Adds an entry as if the GM or an earlier version of the module made it. */
		addEntry(data: object): FakeEntry {
			const entry = fakeEntry(data);
			entries.push(entry);
			return entry;
		},
		/** Adds a folder as if someone made it outside the bridge; pass `_stats` to name its last editor. */
		addFolder(data: object): FakeDocument {
			const folder = fakeDocument("entry update", data);
			folders.push(folder);
			return folder;
		},
		/** Adds a chat message as if `author` posted it, a player by default. */
		addMessage(
			data: object,
			author: FoundryUser | null = users.find((user) => !user.isGM) ?? null,
		) {
			const message = fakeDocument("chat update", data, author);
			messages.push(message);
			return message;
		},
		cancelNext,
		dependencies,
		depositLedger,
		entries,
		entryFor: (externalId: string) =>
			entries.find((entry) => entry.getFlag("colony-bridge", "externalId") === externalId),
		failNext,
		folders,
		ledgerErrors,
		ledgerSetting,
		messages,
		partyItems,
		writes,
	};
}

export type FoundryWorld = ReturnType<typeof createFoundryWorld>;

/** Every write got its own `{colonyBridge: true}` object, as design §13.7 requires. */
export function eachWriteHadFreshOptions(world: FoundryWorld): boolean {
	const objects = new Set(world.writes.map((write) => write.optionsObject));
	return (
		objects.size === world.writes.length &&
		world.writes.every(
			(write) =>
				isRecord(write.options) &&
				Object.keys(write.options).length === 1 &&
				write.options.colonyBridge === true,
		)
	);
}
