import { toast } from "sonner";
import { problemMessage } from "@/api/problem";
import { type SignOutScope, signOut } from "@/api/session";

/** Signs out and leaves with a full page load, so nothing from the session stays in memory. */
export async function signOutAndLeave(scope: SignOutScope) {
	try {
		await signOut(scope);
		window.location.assign("/login");
	} catch (error) {
		toast.error(problemMessage(error, "Signing out didn't work. Try again."));
	}
}
