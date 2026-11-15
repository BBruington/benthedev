import { isActiveGm } from "./active-gm";
import { selectAdapter } from "./adapters/adapter";
import { type Bridge, createBridge } from "./bridge";
import { colonyOrigin } from "./colony-origin";
import {
	createDepositLedger,
	createDeposits,
	type Deposits,
	isWrittenByGameMaster,
} from "./deposits";
import { htmlToPlainText } from "./html";
import { type ImportContext, registerContextMenus, selectImportReaders } from "./imports";
import { scheduleWithTimeout } from "./schedule";
import {
	assignMissingLinkId,
	createBridgeSettings,
	depositsSettingKey,
	moduleId,
	registerSettings,
} from "./settings";
import { defineSettingsMenu } from "./settings-menu";

let bridge: Bridge | undefined;
let deposits: Deposits | undefined;
let importContext: ImportContext | undefined;

const installedColonyOrigin = () => colonyOrigin(game.modules.get(moduleId));

const reportError = (error: unknown) =>
	ui.notifications.error(
		`Colony Bridge: ${error instanceof Error ? error.message : String(error)}`,
	);

Hooks.once("init", () => {
	registerSettings(game.settings, () => bridge?.evaluate());
	game.settings.registerMenu(moduleId, "connection", {
		hint: "Connect this world to a Colony Planner campaign with a pairing code.",
		icon: "fa-solid fa-link",
		label: "Connect",
		name: "Colony Planner",
		restricted: true,
		type: defineSettingsMenu(foundry.applications.api.ApplicationV2, {
			bridge: () => bridge,
			colonyOrigin: installedColonyOrigin,
			// Only the active GM writes the escrow, so only its menu lists the entries to forget.
			deposits: () => (isActiveGm(game) ? deposits : undefined),
			reportError,
		}),
	});
	registerContextMenus(Hooks, game.release.generation, () => importContext, reportError);
});

Hooks.once("ready", () => {
	// Players' browsers load the module and do nothing: the token, the stream and every write are the
	// active GM's.
	if (!game.user.isGM) {
		return;
	}
	// First, so neither the bridge nor a context menu records a deposit under a pre-0.3.0 link's blank ID.
	assignMissingLinkId(game.settings).then(startBridge, reportError);
});

function startBridge() {
	const settings = createBridgeSettings(game.settings);
	const depositLedger = createDepositLedger(
		{
			lastWriterIsGm: () =>
				isWrittenByGameMaster(
					game.settings.get(moduleId, depositsSettingKey, { document: true }),
					new Set(game.users.filter((user) => user.isGM).map((user) => user.id)),
				),
			load: () => game.settings.get(moduleId, depositsSettingKey),
			save: (value) => game.settings.set(moduleId, depositsSettingKey, value),
		},
		ui.notifications,
	);
	deposits = createDeposits({
		actors: game.actors,
		ledger: depositLedger,
		notifications: ui.notifications,
	});
	bridge = createBridge({
		adapter: selectAdapter(game.system.id, {
			chatMessage: ChatMessage,
			depositLedger,
			folder: Folder,
			foundryGame: game,
			fromUuid,
			journalEntry: JournalEntry,
			moveClockWithColonyDays: settings.moveClockWithColonyDays,
		}),
		colonyOrigin: installedColonyOrigin(),
		deposits,
		foundryGame: game,
		notifications: ui.notifications,
		outboxLocks: "locks" in navigator ? navigator.locks : undefined,
		schedule: scheduleWithTimeout,
		settings,
	});
	importContext = {
		bridge,
		deposits,
		inputDialog: (options) => foundry.applications.api.DialogV2.input(options),
		isCompendium: (application) =>
			application instanceof foundry.applications.sidebar.apps.Compendium,
		notifications: ui.notifications,
		readers: selectImportReaders(game.system.id, (html, secretBlocks) =>
			htmlToPlainText(html, secretBlocks, new DOMParser()),
		),
		systemId: game.system.id,
	};
	// A turn later, so Foundry has picked the new active GM before the bridge asks who it is.
	Hooks.on("userConnected", () => scheduleWithTimeout(() => bridge?.evaluate(), 0));
	Hooks.on("updateWorldTime", (worldTime: number, _delta: number, options: unknown) =>
		bridge?.observeWorldTime(worldTime, options),
	);
	bridge.evaluate();
}
