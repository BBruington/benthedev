import type { FoundryUser, FoundryUsers } from "./foundry-types";

/** Only the active GM's client holds the stream, sends events and applies rows (design §6). */
export function isActiveGm(foundryGame: {
	user: FoundryUser;
	users: Pick<FoundryUsers, "activeGM">;
}): boolean {
	const activeGm = foundryGame.users.activeGM;
	return activeGm !== null && activeGm.id === foundryGame.user.id;
}
