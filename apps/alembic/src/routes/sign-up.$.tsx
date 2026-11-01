import { SignUp } from "@clerk/tanstack-react-start";
import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/sign-up/$")({ component: SignUpPage });

function SignUpPage() {
	return (
		<main className="auth-page">
			<SignUp
				routing="path"
				path="/sign-up"
				signInUrl="/sign-in"
				forceRedirectUrl="/workshop"
				fallbackRedirectUrl="/workshop"
			/>
		</main>
	);
}
