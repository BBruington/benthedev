import { ClerkProvider } from "@clerk/tanstack-react-start";

export default function AppClerkProvider({
	children,
}: {
	children: React.ReactNode;
}) {
	return (
		<ClerkProvider
			signInUrl={import.meta.env.VITE_CLERK_SIGN_IN_URL || "/sign-in"}
			signUpUrl={import.meta.env.VITE_CLERK_SIGN_UP_URL || "/sign-up"}
			afterSignOutUrl="/"
			localization={{
				signIn: {
					start: {
						title: "Sign in to Alembic",
						titleCombined: "Sign in to Alembic",
					},
				},
				signUp: {
					start: {
						title: "Create your Alembic account",
						titleCombined: "Create your Alembic account",
					},
				},
			}}
			appearance={{
				variables: {
					colorPrimary: "#d3af68",
					colorBackground: "#172331",
					colorNeutral: "#f1e6cc",
					colorPrimaryForeground: "#17202a",
					colorForeground: "#f1e6cc",
					colorMutedForeground: "#b5b8b6",
					colorInput: "#0b1622",
					colorInputForeground: "#f1e6cc",
					colorDanger: "#e78b81",
					fontFamily: "Manrope, sans-serif",
					borderRadius: "0.75rem",
				},
				elements: {
					formButtonPrimary: { color: "#17202a" },
					socialButtonsBlockButton: {
						color: "#f1e6cc",
						borderColor: "#52606c",
					},
					footerActionLink: { color: "#d3af68" },
				},
			}}
		>
			{children}
		</ClerkProvider>
	);
}
