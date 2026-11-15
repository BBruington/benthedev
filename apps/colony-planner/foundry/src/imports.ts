import type { Bridge } from "./bridge";
import { type Deposits, isPf2eDepositor } from "./deposits";
import type {
	ContextMenuEntry,
	ContextMenuTarget,
	ContextOptionsHook,
	DialogInputOptions,
	FoundryNotifications,
	FoundrySourceDocument,
} from "./foundry-types";
import { cleanLine, clipText, normalizeProse, type PlainText, type SecretBlocks } from "./html";
import {
	compendiumItemUuidPattern,
	type InboundPayloads,
	type ItemImportPayload,
	importedActorUuidPattern,
	importedItemUuidPattern,
	maximumDepositGold,
	maximumFoundryUuidLength,
	maximumItemNameLength,
	maximumNpcLevel,
	maximumNpcNameLength,
	maximumNpcShortTextLength,
	maximumPriceGold,
	maximumProseLength,
	type NpcImportPayload,
	readField,
} from "./wire";

/** Foundry's rich text as plain text; `html.ts`'s htmlToPlainText with the browser's parser. */
export type RichTextReader = (html: string, secretBlocks: SecretBlocks) => PlainText;

export interface ImportReaders {
	item(item: FoundrySourceDocument): ItemImportPayload;
	npc(actor: FoundrySourceDocument): NpcImportPayload;
}

function readPath(value: unknown, path: string[]): unknown {
	return path.reduce<unknown>(readField, value);
}

function stringAt(value: unknown, path: string[]): string {
	const found = readPath(value, path);
	return typeof found === "string" ? found : "";
}

function shortText(text: string): string {
	return cleanLine(text, maximumNpcShortTextLength);
}

function wholeNumberIn(value: unknown, minimum: number, maximum: number): number | null {
	return typeof value === "number" &&
		Number.isSafeInteger(value) &&
		value >= minimum &&
		value <= maximum
		? value
		: null;
}

/** The parts of an NPC import each system fills; the rest come from the actor itself. */
interface NpcDetails {
	/** HTML; its secret blocks move to the DM notes, since an appearance becomes player-visible. */
	appearanceHtml: string;
	characterClass: string;
	/** HTML; DM-only, so its secret blocks stay. */
	dmNotesHtml: string;
	level: number | null;
	race: string;
}

const noNpcDetails: NpcDetails = {
	appearanceHtml: "",
	characterClass: "",
	dmNotesHtml: "",
	level: null,
	race: "",
};

/**
 * pf2e 7.12.2: a character's ancestry and class are items, and it has no GM notes field; an NPC has
 * only traits, which the module doesn't guess an ancestry or class from. A level -1 creature arrives
 * blank.
 */
function pf2eNpcDetails(actor: FoundrySourceDocument): NpcDetails {
	const level = wholeNumberIn(
		readPath(actor.system, ["details", "level", "value"]),
		0,
		maximumNpcLevel,
	);
	if (actor.type === "character") {
		return {
			appearanceHtml: stringAt(actor.system, ["details", "biography", "appearance"]),
			characterClass: stringAt(actor, ["class", "name"]),
			dmNotesHtml: "",
			level,
			race: stringAt(actor, ["ancestry", "name"]),
		};
	}
	if (actor.type === "npc") {
		return {
			...noNpcDetails,
			appearanceHtml: stringAt(actor.system, ["details", "publicNotes"]),
			dmNotesHtml: stringAt(actor.system, ["details", "privateNotes"]),
			level,
		};
	}
	return { ...noNpcDetails, level };
}

/** dnd5e 5.2.5: an NPC's challenge rating isn't a level, so only a character's level is read. */
function dnd5eNpcDetails(actor: FoundrySourceDocument): NpcDetails {
	const race = readPath(actor.system, ["details", "race"]);
	const classes = readField(actor, "classes");
	const classNames =
		typeof classes === "object" && classes !== null
			? Object.values(classes).flatMap((item) => {
					const name = readField(item, "name");
					return typeof name === "string" && name.trim() !== "" ? [name] : [];
				})
			: [];
	return {
		appearanceHtml: stringAt(actor.system, ["details", "biography", "public"]),
		characterClass: classNames.join(", "),
		dmNotesHtml: stringAt(actor.system, ["details", "biography", "value"]),
		level:
			actor.type === "character"
				? wholeNumberIn(readPath(actor.system, ["details", "level"]), 1, maximumNpcLevel)
				: null,
		race: typeof race === "string" ? race : stringAt(race, ["name"]),
	};
}

function joinProse(texts: string[]): string {
	return normalizeProse(texts.filter((text) => text !== "").join("\n\n"));
}

function readNpc(
	actor: FoundrySourceDocument,
	details: NpcDetails,
	readRichText: RichTextReader,
): NpcImportPayload {
	const appearance = readRichText(details.appearanceHtml, "omit");
	const dmNotes = readRichText(details.dmNotesHtml, "keep");
	return {
		appearance: clipText(appearance.text, maximumProseLength),
		characterClass: shortText(details.characterClass),
		dmNotes: clipText(joinProse([dmNotes.text, appearance.secrets]), maximumProseLength),
		foundryUuid: actor.uuid,
		level: details.level,
		name: cleanLine(actor.name, maximumNpcNameLength),
		race: shortText(details.race),
	};
}

const copperPerCoin: Partial<Record<string, number>> = { cp: 1, gp: 100, pp: 1000, sp: 10 };
const copperPerGold = 100;

/** Whole gold, or null: a price in copper that isn't a whole number of gold arrives blank. */
function goldFromCopper(copper: number): number | null {
	if (!Number.isSafeInteger(copper) || copper < 0 || copper % copperPerGold !== 0) {
		return null;
	}
	return wholeNumberIn(copper / copperPerGold, 0, maximumPriceGold);
}

function coinCount(value: unknown): number | undefined {
	if (value === undefined || value === null) {
		return 0;
	}
	return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

/** pf2e: `{pp, gp, sp, cp}`, blank for a bundle's price or a Starfinder amount. */
function pf2ePriceGold(item: FoundrySourceDocument): number | null {
	const price = readField(item.system, "price");
	const per = readField(price, "per");
	if (typeof per === "number" && per > 1) {
		return null;
	}
	const value = readField(price, "value");
	if (["credits", "upb"].some((coin) => coinCount(readField(value, coin)) !== 0)) {
		return null;
	}
	const counts = ["pp", "gp", "sp", "cp"].map((coin) => ({
		copper: copperPerCoin[coin] ?? 0,
		count: coinCount(readField(value, coin)),
	}));
	if (counts.some(({ count }) => count === undefined)) {
		return null;
	}
	return goldFromCopper(counts.reduce((sum, { count, copper }) => sum + (count ?? 0) * copper, 0));
}

/** dnd5e: one value in one denomination; electrum arrives blank (design §14.14, question 3). */
function dnd5ePriceGold(item: FoundrySourceDocument): number | null {
	const price = readField(item.system, "price");
	const value = readField(price, "value");
	const denomination = readField(price, "denomination");
	const copper = typeof denomination === "string" ? copperPerCoin[denomination] : undefined;
	if (typeof value !== "number" || copper === undefined) {
		return null;
	}
	return goldFromCopper(value * copper);
}

/**
 * The pack's own UUID when opened from a pack, else the compendium item a world item came from
 * (v13 also sets `compendiumSource` on dropped world items, as `Item.<id>`).
 */
function compendiumUuidOf(item: FoundrySourceDocument): string | null {
	const uuid = item.pack ? item.uuid : (item._stats?.compendiumSource ?? "");
	return uuid.length <= maximumFoundryUuidLength && compendiumItemUuidPattern.test(uuid)
		? uuid
		: null;
}

function readItem(
	item: FoundrySourceDocument,
	descriptionHtml: string,
	priceGold: number | null,
	readRichText: RichTextReader,
): ItemImportPayload {
	return {
		compendiumUuid: compendiumUuidOf(item),
		// Player-visible, so secret blocks stay out, and PF2e's `system.description.gm` is never read.
		description: clipText(readRichText(descriptionHtml, "omit").text, maximumProseLength),
		foundryUuid: item.uuid,
		name: cleanLine(item.name, maximumItemNameLength),
		priceGold,
	};
}

/** Chosen once at `ready` from game.system.id, beside the adapter. */
export function selectImportReaders(systemId: string, readRichText: RichTextReader): ImportReaders {
	const description = (item: FoundrySourceDocument) =>
		stringAt(item.system, ["description", "value"]);
	if (systemId === "pf2e") {
		return {
			item: (item) => readItem(item, description(item), pf2ePriceGold(item), readRichText),
			npc: (actor) => readNpc(actor, pf2eNpcDetails(actor), readRichText),
		};
	}
	if (systemId === "dnd5e") {
		return {
			item: (item) => readItem(item, description(item), dnd5ePriceGold(item), readRichText),
			npc: (actor) => readNpc(actor, dnd5eNpcDetails(actor), readRichText),
		};
	}
	return {
		item: (item) => readItem(item, "", null, readRichText),
		npc: (actor) => readNpc(actor, noNpcDetails, readRichText),
	};
}

export function isSourceDocument(value: unknown): value is FoundrySourceDocument {
	return (
		typeof readField(value, "name") === "string" &&
		typeof readField(value, "type") === "string" &&
		typeof readField(value, "uuid") === "string"
	);
}

/** What the context menu entries need, built at `ready` in the active GM's world. */
export interface ImportContext {
	bridge: Pick<Bridge, "linkId" | "queueEvent" | "status">;
	createId?: () => string;
	deposits: Pick<Deposits, "deposit">;
	inputDialog: (options: DialogInputOptions) => Promise<unknown>;
	/** `application instanceof foundry.applications.sidebar.apps.Compendium`. */
	isCompendium: (application: unknown) => boolean;
	notifications: FoundryNotifications;
	now?: () => Date;
	readers: ImportReaders;
	systemId: string;
}

interface MenuAction {
	icon: string;
	label: string;
	run: (target: ContextMenuTarget) => Promise<void>;
	visible: (target: ContextMenuTarget) => boolean;
}

/** v13 and v14 name the same entry's fields differently; v14's aliases for v13's aren't relied on. */
export function contextMenuEntry(generation: number, action: MenuAction): ContextMenuEntry {
	if (generation >= 14) {
		return {
			icon: action.icon,
			label: action.label,
			onClick: (_event, target) => action.run(target),
			visible: action.visible,
		};
	}
	return {
		callback: (target) => action.run(target),
		condition: action.visible,
		icon: action.icon,
		name: action.label,
	};
}

const sendIcon = '<i class="fa-solid fa-paper-plane"></i>';
const depositIcon = '<i class="fa-solid fa-coins"></i>';
const notRunningWarning =
	"Colony Bridge isn't running in this browser, so nothing was sent. Only the active GM's browser sends to Colony Planner.";
// Another link may name another campaign, so what was read under the first never goes to the second.
const linkChangedWarning =
	"This world's Colony Planner link changed while Foundry was busy, so nothing was sent. Try again.";

function entryId(target: ContextMenuTarget): string {
	return target.dataset.entryId ?? "";
}

function isLookup(value: unknown): value is { get(id: string): unknown } {
	return typeof readField(value, "get") === "function";
}

function isPackLookup(value: unknown): value is { getDocument(id: string): Promise<unknown> } {
	return typeof readField(value, "getDocument") === "function";
}

/** The sidebar's document now, without waiting; a pack's entries are only an index until loaded. */
function directoryDocument(application: unknown, target: ContextMenuTarget): unknown {
	const collection = readField(application, "collection");
	const id = entryId(target);
	return id !== "" && isLookup(collection) ? collection.get(id) : undefined;
}

async function entryDocument(
	application: unknown,
	target: ContextMenuTarget,
	context: ImportContext,
): Promise<unknown> {
	if (!context.isCompendium(application)) {
		return directoryDocument(application, target);
	}
	const collection = readField(application, "collection");
	const id = entryId(target);
	return id !== "" && isPackLookup(collection) ? await collection.getDocument(id) : undefined;
}

function isPf2eWorldParty(document: unknown, context: ImportContext): boolean {
	return context.systemId === "pf2e" && readField(document, "type") === "party";
}

function queueImport<Kind extends "NPC_IMPORT" | "ITEM_IMPORT">(
	kind: Kind,
	payload: InboundPayloads[Kind],
	linkId: string,
	context: ImportContext,
): void {
	if (context.bridge.linkId() !== linkId) {
		context.notifications.warn(linkChangedWarning);
		return;
	}
	const queued = context.bridge.queueEvent({
		id: (context.createId ?? (() => crypto.randomUUID()))(),
		kind,
		occurredAt: (context.now ?? (() => new Date()))().toISOString(),
		payload,
	});
	if (!queued) {
		context.notifications.warn(notRunningWarning);
		return;
	}
	context.notifications.info(`Queued ${payload.name} for Colony Planner's inbox.`);
}

function sendsTo(payload: { foundryUuid: string; name: string }, uuidPattern: RegExp): boolean {
	return (
		payload.name !== "" &&
		payload.foundryUuid.length <= maximumFoundryUuidLength &&
		uuidPattern.test(payload.foundryUuid)
	);
}

async function sendActor(
	application: unknown,
	target: ContextMenuTarget,
	context: ImportContext,
): Promise<void> {
	const linkId = context.bridge.linkId();
	const actor = await entryDocument(application, target, context);
	if (!isSourceDocument(actor)) {
		context.notifications.warn("Foundry couldn't find that actor.");
		return;
	}
	if (isPf2eWorldParty(actor, context)) {
		context.notifications.warn(
			"A party can't become an NPC. Send its members to Colony Planner one at a time.",
		);
		return;
	}
	const payload = context.readers.npc(actor);
	if (!sendsTo(payload, importedActorUuidPattern)) {
		context.notifications.warn(
			"Colony Planner takes an actor from the Actors tab or a compendium, with a name.",
		);
		return;
	}
	queueImport("NPC_IMPORT", payload, linkId, context);
}

async function sendItem(
	application: unknown,
	target: ContextMenuTarget,
	context: ImportContext,
): Promise<void> {
	const linkId = context.bridge.linkId();
	const item = await entryDocument(application, target, context);
	if (!isSourceDocument(item)) {
		context.notifications.warn("Foundry couldn't find that item.");
		return;
	}
	const payload = context.readers.item(item);
	if (!sendsTo(payload, importedItemUuidPattern)) {
		context.notifications.warn(
			"Colony Planner takes an item from the Items tab or a compendium, with a name.",
		);
		return;
	}
	queueImport("ITEM_IMPORT", payload, linkId, context);
}

const wholeNumber = /^\d+$/;

/** A whole number of gold from 1 to 1,000,000, or undefined; the form gives a number or its text. */
export function readDepositGold(value: unknown): number | undefined {
	const typed = typeof value === "string" ? value.trim() : value;
	const gold = typeof typed === "string" && wholeNumber.test(typed) ? Number(typed) : typed;
	return wholeNumberIn(gold, 1, maximumDepositGold) ?? undefined;
}

const depositDialog: DialogInputOptions = {
	content: [
		'<div class="form-group">',
		'<label for="colony-bridge-deposit-gold">Gold pieces to deposit</label>',
		'<div class="form-fields"><input id="colony-bridge-deposit-gold" max="1000000" min="1" name="gold" required step="1" type="number"></div>',
		'<p class="hint">Paid with any coins worth exactly this much.</p>',
		"</div>",
	].join(""),
	ok: { label: "Deposit" },
	window: { title: "Deposit to colony" },
};

async function depositFromActor(
	application: unknown,
	target: ContextMenuTarget,
	context: ImportContext,
): Promise<void> {
	const actor = directoryDocument(application, target);
	if (!isPf2eDepositor(actor)) {
		context.notifications.warn("Foundry couldn't find that party or character.");
		return;
	}
	const linkId = context.bridge.linkId();
	const answer = await context.inputDialog(depositDialog);
	if (answer === null || answer === undefined) {
		return;
	}
	const gold = readDepositGold(readField(answer, "gold"));
	if (gold === undefined) {
		context.notifications.warn("Type a whole number of gold pieces from 1 to 1,000,000.");
		return;
	}
	// The dialog may have stayed open while another GM took over.
	if (context.bridge.status().condition !== "running") {
		context.notifications.warn(notRunningWarning);
		return;
	}
	if (context.bridge.linkId() !== linkId) {
		context.notifications.warn(linkChangedWarning);
		return;
	}
	await context.deposits.deposit(actor, gold, {
		enqueue: (event) => context.bridge.queueEvent(event),
		id: () => context.bridge.linkId(),
	});
}

/** Foundry's Hooks, as the two context menu hooks call back. */
export interface ContextMenuHooks {
	on(
		name: "getActorContextOptions" | "getItemContextOptions",
		callback: ContextOptionsHook,
	): unknown;
}

/**
 * Registered at `init`, since each directory and compendium builds its menu once, at its first render:
 * every entry decides at click time, and shows only while the bridge runs in this browser, so
 * players' and other GMs' menus are unchanged (design §14.1).
 */
export function registerContextMenus(
	hooks: ContextMenuHooks,
	generation: number,
	importContext: () => ImportContext | undefined,
	reportError: (error: unknown) => void,
): void {
	const running = () => {
		const context = importContext();
		return context?.bridge.status().condition === "running" ? context : undefined;
	};
	const action = (
		label: string,
		icon: string,
		application: unknown,
		visible: (context: ImportContext, target: ContextMenuTarget) => boolean,
		run: (application: unknown, target: ContextMenuTarget, context: ImportContext) => Promise<void>,
	) =>
		contextMenuEntry(generation, {
			icon,
			label,
			run: async (target) => {
				const context = running();
				if (!context) {
					importContext()?.notifications.warn(notRunningWarning);
					return;
				}
				await run(application, target, context).catch(reportError);
			},
			visible: (target) => {
				const context = running();
				return context !== undefined && visible(context, target);
			},
		});

	const actorOptions: ContextOptionsHook = (application, menuItems) => {
		menuItems.push(
			action(
				"Send to Colony Planner",
				sendIcon,
				application,
				(context, target) => {
					if (context.isCompendium(application)) {
						return true;
					}
					const actor = directoryDocument(application, target);
					return isSourceDocument(actor) && !isPf2eWorldParty(actor, context);
				},
				sendActor,
			),
			action(
				"Deposit to colony…",
				depositIcon,
				application,
				(context, target) =>
					context.systemId === "pf2e" &&
					!context.isCompendium(application) &&
					isPf2eDepositor(directoryDocument(application, target)),
				depositFromActor,
			),
		);
	};
	const itemOptions: ContextOptionsHook = (application, menuItems) => {
		menuItems.push(
			action(
				"Send to Colony Planner",
				sendIcon,
				application,
				(context, target) =>
					context.isCompendium(application) ||
					isSourceDocument(directoryDocument(application, target)),
				sendItem,
			),
		);
	};
	hooks.on("getActorContextOptions", actorOptions);
	hooks.on("getItemContextOptions", itemOptions);
}
