import type { Bridge, BridgeStatus } from "./bridge";
import type { ApplicationV2 } from "./foundry-types";
import { connectWithCode } from "./pairing";

export interface SettingsMenuDependencies {
	bridge: () => Bridge | undefined;
	colonyOrigin: () => string | undefined;
	reportError: (error: unknown) => void;
}

/**
 * The GM-only settings menu (design §3.1): type the pairing code and press Connect, see the linked
 * campaign, or Disconnect this browser. Built as a factory because ApplicationV2 exists only inside
 * Foundry, once its scripts have loaded.
 */
export function defineSettingsMenu(
	applicationBase: typeof ApplicationV2,
	dependencies: SettingsMenuDependencies,
): typeof ApplicationV2 {
	class ColonyBridgeSettingsMenu extends applicationBase {
		static override DEFAULT_OPTIONS = {
			actions: {
				disconnect(this: unknown) {
					if (this instanceof ColonyBridgeSettingsMenu) {
						return this.disconnect();
					}
					return undefined;
				},
			},
			form: {
				closeOnSubmit: false,
				handler(this: unknown, _event: SubmitEvent, form: HTMLFormElement) {
					if (this instanceof ColonyBridgeSettingsMenu) {
						return this.connect(form);
					}
					return undefined;
				},
			},
			id: "colony-bridge-settings",
			position: { width: 440 },
			tag: "form",
			window: { title: "Colony Bridge" },
		};

		#code = "";
		#message = "";
		#busy = false;
		#unsubscribe: (() => void) | undefined;

		async connect(form: HTMLFormElement) {
			const code = new FormData(form).get("code");
			this.#code = typeof code === "string" ? code : "";
			// Checked before pairing: a code Colony Planner accepts revokes the old link at once, so the
			// new token must have somewhere to go.
			const bridge = dependencies.bridge();
			if (!bridge) {
				this.#message = "Foundry is still loading. Try again in a moment.";
				await this.render();
				return;
			}
			this.#busy = true;
			this.#message = "Connecting...";
			await this.render();
			const endPairing = bridge.beginPairing();
			const result = await connectWithCode({
				code: this.#code,
				colonyOrigin: dependencies.colonyOrigin(),
				saveLink: (link) => bridge.saveLink(link),
			});
			this.#busy = false;
			if (result.ok) {
				this.#code = "";
				this.#message = `Connected to ${result.campaignName}.`;
			} else {
				endPairing();
				this.#message = result.message;
			}
			await this.render();
		}

		async disconnect() {
			try {
				await dependencies.bridge()?.forgetLink();
				this.#message = "Disconnected in this browser.";
			} catch (error) {
				this.#message = error instanceof Error ? error.message : String(error);
			}
			await this.render();
		}

		protected override _renderHTML(): Promise<unknown> {
			// A re-render (a player joining, the stream reconnecting) keeps what the GM was typing.
			const codeInput = this.element?.querySelector('input[name="code"]');
			if (!this.#busy && codeInput instanceof HTMLInputElement) {
				this.#code = codeInput.value;
			}
			const bridge = dependencies.bridge();
			if (bridge && !this.#unsubscribe) {
				this.#unsubscribe = bridge.subscribe(() => {
					if (this.rendered) {
						this.render().catch(dependencies.reportError);
						return;
					}
					this.#unsubscribe?.();
					this.#unsubscribe = undefined;
				});
			}
			return Promise.resolve(
				renderMenuContent(bridge?.status(), {
					busy: this.#busy,
					code: this.#code,
					message: this.#message,
				}),
			);
		}

		protected override _replaceHTML(result: unknown, content: HTMLElement): void {
			if (result instanceof HTMLElement) {
				content.replaceChildren(result);
			}
		}
	}
	return ColonyBridgeSettingsMenu;
}

function element<Tag extends keyof HTMLElementTagNameMap>(
	tag: Tag,
	properties: Partial<HTMLElementTagNameMap[Tag]> = {},
	children: Node[] = [],
): HTMLElementTagNameMap[Tag] {
	const created = Object.assign(document.createElement(tag), properties);
	created.append(...children);
	return created;
}

function statusSentence(status: BridgeStatus): string {
	switch (status.condition) {
		case "turned-off":
			return "The bridge is off for this world. Turn on Bridge on under Configure Settings.";
		case "no-colony-address":
			return "This copy of Colony Bridge doesn't know Colony Planner's address. Install it with the manifest link on Colony Planner's Settings.";
		case "not-linked":
			return "Not connected to a campaign.";
		case "needs-new-code":
			return "Colony Planner ended this world's link. Make a new code on Colony Planner's Settings and connect again.";
		case "another-gm-is-active":
			return `Linked to ${status.campaignName}, but another GM is the active GM, and only the active GM's browser runs the bridge.`;
		case "running":
			return runningSentence(status);
		default:
			return "";
	}
}

function runningSentence(status: BridgeStatus): string {
	switch (status.connection) {
		case "live":
			return `Live: connected to ${status.campaignName}.`;
		case "reconnecting":
			return `Reconnecting to ${status.campaignName}.`;
		default:
			return `Connecting to ${status.campaignName}.`;
	}
}

function renderMenuContent(
	status: BridgeStatus | undefined,
	view: { busy: boolean; code: string; message: string },
): HTMLElement {
	if (!status) {
		return element("p", {
			textContent: "Foundry is still loading. Open this menu again in a moment.",
		});
	}
	const children: Node[] = [element("p", { textContent: statusSentence(status) })];
	if (status.lastProblem) {
		children.push(
			element("p", { className: "notes", textContent: `Last problem: ${status.lastProblem}` }),
		);
	}
	if (status.linked) {
		const disconnectButton = element("button", {
			disabled: view.busy,
			textContent: "Disconnect",
			type: "button",
		});
		disconnectButton.dataset.action = "disconnect";
		children.push(
			element("p", {
				className: "hint",
				textContent:
					"Disconnect forgets the link in this browser only. To end it for good, press Disconnect on Colony Planner's Settings.",
			}),
			disconnectButton,
		);
	} else {
		children.push(
			element("div", { className: "form-group" }, [
				element("label", { htmlFor: "colony-bridge-code", textContent: "Pairing code" }),
				element("input", {
					autocomplete: "off",
					disabled: view.busy,
					id: "colony-bridge-code",
					name: "code",
					placeholder: "K7Q-4XM",
					type: "text",
					value: view.code,
				}),
			]),
			element("p", {
				className: "hint",
				textContent:
					"On Colony Planner, open your campaign's Settings and press Connect Foundry for a code that lasts 10 minutes.",
			}),
			element("button", { disabled: view.busy, textContent: "Connect", type: "submit" }),
		);
	}
	if (view.message) {
		children.push(element("p", { role: "status", textContent: view.message }));
	}
	return element("section", {}, children);
}
