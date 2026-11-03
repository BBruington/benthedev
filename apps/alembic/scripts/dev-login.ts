import { chromium } from "playwright-core";

interface PageClerk {
	loaded: boolean;
	client?: {
		signIn: {
			create(params: {
				strategy: "ticket";
				ticket: string;
			}): Promise<{ createdSessionId: string | null }>;
		};
	};
	setActive(params: { session: string | null }): Promise<void>;
	user?: { id: string } | null;
}

type ClerkWindow = Window & { Clerk?: PageClerk };

const APP_URL = process.env.APP_URL ?? "http://localhost:3000";
const STATE_PATH = ".auth/alembic-tour.json";
const EXTERNAL_ID = "alembic-tour";

const secretKey = process.env.CLERK_SECRET_KEY;
if (!secretKey?.startsWith("sk_test_")) {
	throw new Error("CLERK_SECRET_KEY must be a development key (sk_test_)");
}

async function clerkApi<T>(path: string, init?: RequestInit): Promise<T> {
	const response = await fetch(`https://api.clerk.com/v1${path}`, {
		...init,
		headers: {
			Authorization: `Bearer ${secretKey}`,
			"Content-Type": "application/json",
		},
	});
	if (!response.ok) {
		throw new Error(
			`Clerk ${path}: ${response.status} ${await response.text()}`,
		);
	}
	return (await response.json()) as T;
}

const [user] = await clerkApi<{ id: string }[]>(
	`/users?external_id=${EXTERNAL_ID}`,
);
if (!user) throw new Error(`No Clerk user with external_id ${EXTERNAL_ID}`);
const { token } = await clerkApi<{ token: string }>("/sign_in_tokens", {
	method: "POST",
	body: JSON.stringify({ user_id: user.id, expires_in_seconds: 300 }),
});

const browser = await chromium.launch({ channel: "chrome" });
try {
	const page = await browser.newPage();
	await page.goto(APP_URL);
	await page.waitForFunction(() => (window as ClerkWindow).Clerk?.loaded);
	const signedInAs = await page.evaluate(async (ticket) => {
		const clerk = (window as ClerkWindow).Clerk;
		if (!clerk?.client) throw new Error("Clerk did not load");
		const signIn = await clerk.client.signIn.create({
			strategy: "ticket",
			ticket,
		});
		await clerk.setActive({ session: signIn.createdSessionId });
		return clerk.user?.id;
	}, token);
	if (signedInAs !== user.id) {
		throw new Error(`Signed in as ${signedInAs}, expected ${user.id}`);
	}
	await page.context().storageState({ path: STATE_PATH });
	console.log(`Saved ${EXTERNAL_ID} browser state to ${STATE_PATH}`);
} finally {
	await browser.close();
}
