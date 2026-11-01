import { SignIn } from "@clerk/tanstack-react-start";
import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/sign-in/$")({ component: SignInPage });

function SignInPage() {
	return (
		<main className="auth-page">
			<SignIn
				routing="path"
				path="/sign-in"
				signUpUrl="/sign-up"
				forceRedirectUrl="/workshop"
				fallbackRedirectUrl="/workshop"
			/>
		</main>
	);
}
