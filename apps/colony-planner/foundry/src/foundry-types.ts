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

export interface FoundryCollection<Document> {
	find(predicate: (document: Document) => boolean): Document | undefined;
}

export interface FoundryJournalPage extends FlaggedDocument {
	update(data: object, options?: object): Promise<unknown>;
}

export interface FoundryJournalEntry extends FlaggedDocument {
	createEmbeddedDocuments(
		embeddedName: "JournalEntryPage",
		data: object[],
		options?: object,
	): Promise<unknown>;
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
	journal: FoundryCollection<FoundryJournalEntry>;
	messages: FoundryCollection<FlaggedDocument>;
	modules: { get(id: string): FoundryModule | undefined };
	settings: FoundrySettings;
	system: { id: string; version: string };
	time: {
		advance(seconds: number, options?: object): Promise<unknown>;
		worldTime: number;
	};
	user: FoundryUser;
	users: { activeGM: FoundryUser | null };
	version: string;
	world: { title: string };
}

export interface FoundryNotifications {
	error(message: string): unknown;
	info(message: string): unknown;
	warn(message: string): unknown;
}

export interface DocumentClass<Document> {
	create(data: object, options?: object): Promise<Document | undefined>;
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
	const ChatMessage: DocumentClass<FlaggedDocument>;
	const JournalEntry: DocumentClass<FoundryJournalEntry>;
	const foundry: { applications: { api: { ApplicationV2: typeof ApplicationV2 } } };
}
