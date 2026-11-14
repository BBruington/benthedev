/**
 * The small part of Foundry's client API (v13 and v14) the module uses, written by hand so the module
 * needs no Foundry types package. Every shape here is a subset of the real one, so the real globals
 * satisfy it and tests can pass small fakes.
 */

export interface FoundryUser {
	id: string;
	isGM: boolean;
}

export interface FlaggedDocument {
	getFlag(scope: string, key: string): unknown;
}

/** Who may see or change a document: `default` and per-user levels, 0 None to 3 Owner. */
export type DocumentOwnership = Record<string, number | undefined>;

/** A world document a player might have made or changed: Foundry's server sets `_stats`. */
export interface OwnedDocument extends FlaggedDocument {
	_stats?: { lastModifiedBy?: string | null } | null;
	ownership?: DocumentOwnership;
}

export interface FoundryCollection<Document> {
	filter(predicate: (document: Document) => boolean): Document[];
	find(predicate: (document: Document) => boolean): Document | undefined;
}

export interface FoundryUsers {
	activeGM: FoundryUser | null;
	filter(predicate: (user: FoundryUser) => boolean): FoundryUser[];
}

export interface FoundryChatMessage extends FlaggedDocument {
	/** The User who posted it; Foundry's server allows only a GM to post as someone else. */
	author?: FoundryUser | null;
	update(data: object, options?: object): Promise<unknown>;
}

export interface FoundryFolder extends OwnedDocument {
	id: string;
}

/** What fromUuid gives for an Item: any document, checked by `documentName` before use. */
export interface FoundryItem extends FlaggedDocument {
	documentName: string;
	system: unknown;
	toObject(): Record<string, unknown>;
}

/** PF2e's party actor, `game.actors.party`. */
export interface FoundryPartyActor {
	createEmbeddedDocuments(embeddedName: "Item", data: object[], options?: object): Promise<unknown>;
	items: FoundryCollection<FlaggedDocument>;
	name: string;
}

export interface FoundryJournalPage extends FlaggedDocument {
	text?: { content?: string | null } | null;
	update(data: object, options?: object): Promise<unknown>;
}

export interface FoundryJournalEntry extends OwnedDocument {
	id: string;
	/** The documents made; a hook that cancels the create leaves them out. */
	createEmbeddedDocuments(
		embeddedName: "JournalEntryPage",
		data: object[],
		options?: object,
	): Promise<unknown[]>;
	name: string;
	pages: FoundryCollection<FoundryJournalPage>;
	update(data: object, options?: object): Promise<unknown>;
}

export interface FoundryModule {
	manifest: string;
	version: string;
}

export interface SettingConfig {
	config: boolean;
	default: unknown;
	hint?: string;
	name?: string;
	onChange?: (value: unknown) => void;
	scope: "client" | "world";
	type: BooleanConstructor | NumberConstructor | ObjectConstructor | StringConstructor;
}

export interface SettingMenuConfig {
	hint: string;
	icon: string;
	label: string;
	name: string;
	restricted: boolean;
	type: unknown;
}

export interface FoundrySettings {
	get(namespace: string, key: string): unknown;
	register(namespace: string, key: string, config: SettingConfig): void;
	registerMenu(namespace: string, key: string, config: SettingMenuConfig): void;
	set(namespace: string, key: string, value: unknown): Promise<unknown>;
}

export interface FoundryGame {
	/** `party` is PF2e's; other systems have none. */
	actors: { party?: FoundryPartyActor | null };
	folders: FoundryCollection<FoundryFolder>;
	journal: FoundryCollection<FoundryJournalEntry>;
	messages: FoundryCollection<FoundryChatMessage>;
	modules: { get(id: string): FoundryModule | undefined };
	settings: FoundrySettings;
	system: { id: string; version: string };
	time: {
		advance(seconds: number, options?: object): Promise<unknown>;
		worldTime: number;
	};
	user: FoundryUser;
	users: FoundryUsers;
	version: string;
	world: { id: string; title: string };
}

export interface FoundryNotifications {
	error(message: string): unknown;
	info(message: string): unknown;
	warn(message: string): unknown;
}

export interface DocumentClass<Document> {
	/** Undefined when a hook cancels the create. */
	create(data: object, options?: object): Promise<Document | undefined>;
}

export interface JournalEntryClass extends DocumentClass<FoundryJournalEntry> {
	updateDocuments(updates: object[], options?: object): Promise<unknown>;
}

export interface ApplicationOptions {
	actions?: Record<string, (this: unknown, event: PointerEvent, target: HTMLElement) => unknown>;
	form?: {
		closeOnSubmit: boolean;
		handler: (this: unknown, event: SubmitEvent, form: HTMLFormElement) => unknown;
	};
	id?: string;
	position?: { width: number };
	tag?: string;
	window?: { title: string };
}

export declare class ApplicationV2 {
	static DEFAULT_OPTIONS: ApplicationOptions;
	constructor(options?: ApplicationOptions);
	readonly element: HTMLElement | null;
	readonly rendered: boolean;
	close(): Promise<unknown>;
	render(options?: { force: boolean }): Promise<unknown>;
	protected _renderHTML(context: unknown, options: unknown): Promise<unknown>;
	protected _replaceHTML(result: unknown, content: HTMLElement, options: unknown): void;
}

export type HookCallback = (...parameters: never[]) => unknown;

export interface FoundryHooks {
	on(name: string, callback: HookCallback): number;
	once(name: string, callback: HookCallback): number;
}

declare global {
	const game: FoundryGame;
	const Hooks: FoundryHooks;
	const ui: { notifications: FoundryNotifications };
	const ChatMessage: DocumentClass<FoundryChatMessage>;
	const JournalEntry: JournalEntryClass;
	const Folder: DocumentClass<FoundryFolder>;
	function fromUuid(uuid: string): Promise<unknown>;
	const foundry: { applications: { api: { ApplicationV2: typeof ApplicationV2 } } };
}
