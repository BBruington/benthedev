import { selectAdapter } from "./adapters/adapter";
import { type Bridge, createBridge } from "./bridge";
import { colonyOrigin } from "./colony-origin";
import { scheduleWithTimeout } from "./schedule";
import { createBridgeSettings, moduleId, registerSettings } from "./settings";
import { defineSettingsMenu } from "./settings-menu";

let bridge: Bridge | undefined;

const installedColonyOrigin = () => colonyOrigin(game.modules.get(moduleId));

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
			reportError: (error) =>
				ui.notifications.error(
					`Colony Bridge: ${error instanceof Error ? error.message : String(error)}`,
				),
		}),
	});
});

Hooks.once("ready", () => {
	// Players' browsers load the module and do nothing: the token, the stream and every write are the
	// active GM's.
	if (!game.user.isGM) {
		return;
	}
	const settings = createBridgeSettings(game.settings);
	bridge = createBridge({
		adapter: selectAdapter(game.system.id, {
			chatMessage: ChatMessage,
			folder: Folder,
			foundryGame: game,
			fromUuid,
			journalEntry: JournalEntry,
			moveClockWithColonyDays: settings.moveClockWithColonyDays,
		}),
		colonyOrigin: installedColonyOrigin(),
		foundryGame: game,
		notifications: ui.notifications,
		schedule: scheduleWithTimeout,
		settings,
	});
	// A turn later, so Foundry has picked the new active GM before the bridge asks who it is.
	Hooks.on("userConnected", () => scheduleWithTimeout(() => bridge?.evaluate(), 0));
	Hooks.on("updateWorldTime", (worldTime: number, _delta: number, options: unknown) =>
		bridge?.observeWorldTime(worldTime, options),
	);
	bridge.evaluate();
});
