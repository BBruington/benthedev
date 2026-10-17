import { Contact } from "./contact";
import { GitHubIcon, LinkedInIcon } from "./icons";
import { EMAIL, GITHUB_URL, LINKEDIN_URL, RESUME_PATH } from "./profile";
import { Experience, Projects, Skills } from "./resume";

const NAVIGATION = [
	{ href: "#experience", label: "Experience", visibleOnPhones: true },
	{ href: "#projects", label: "Projects", visibleOnPhones: true },
	{ href: "#skills", label: "Skills", visibleOnPhones: false },
	{ href: "#contact", label: "Contact", visibleOnPhones: true },
];

export function App() {
	return (
		<>
			<a
				className="sr-only focus:not-sr-only focus:fixed focus:top-4 focus:left-4 focus:z-20 focus:rounded-md focus:bg-primary focus:px-4 focus:py-2 focus:text-primary-foreground"
				href="#main"
			>
				Skip to content
			</a>
			<header className="sticky top-0 z-10 border-border/60 border-b bg-background/80 backdrop-blur">
				<nav
					aria-label="Sections"
					className="mx-auto flex max-w-5xl items-center justify-between gap-6 px-6 py-4"
				>
					<a className="link font-mono text-xs uppercase tracking-[0.18em]" href="#main">
						BB
					</a>
					<ul className="flex gap-5 text-muted-foreground text-sm">
						{NAVIGATION.map((item) => (
							<li className={item.visibleOnPhones ? undefined : "hidden sm:block"} key={item.href}>
								<a className="link" href={item.href}>
									{item.label}
								</a>
							</li>
						))}
					</ul>
				</nav>
			</header>

			<main className="mx-auto max-w-5xl px-6" id="main">
				<section className="pt-20 pb-20 sm:pt-28">
					<p className="font-mono text-primary text-xs uppercase tracking-[0.18em]">
						Full-stack engineer
					</p>
					<h1 className="mt-5 font-medium font-serif text-5xl leading-[1.02] tracking-tight sm:text-7xl">
						Benjamin Bruington
					</h1>
					<p className="mt-8 max-w-2xl text-pretty text-lg text-muted-foreground leading-relaxed sm:text-xl">
						I build AI features and tools that make technical systems usable by non-engineers, like
						an LLM pipeline that turns engineering activity into client updates. I work in{" "}
						<span className="text-foreground">React, Next.js, TypeScript and Go</span>.
					</p>
					<ul className="mt-10 flex flex-wrap items-center gap-x-6 gap-y-3 text-sm">
						<li>
							<a className="link-underlined" href={`mailto:${EMAIL}`}>
								{EMAIL}
							</a>
						</li>
						<li>
							<a className="link-underlined" download={true} href={RESUME_PATH}>
								Resume (PDF)
							</a>
						</li>
						<li className="flex items-center gap-x-6">
							<a
								aria-label="GitHub"
								className="link inline-flex"
								href={GITHUB_URL}
								rel="noreferrer"
								target="_blank"
							>
								<GitHubIcon className="h-5 w-5" />
							</a>
							<a
								aria-label="LinkedIn"
								className="link inline-flex"
								href={LINKEDIN_URL}
								rel="noreferrer"
								target="_blank"
							>
								<LinkedInIcon className="h-5 w-5" />
							</a>
						</li>
					</ul>
				</section>

				<Experience />
				<Projects />
				<Skills />
				<Contact />
			</main>

			<footer className="mx-auto max-w-5xl border-border border-t px-6 py-8 font-mono text-muted-foreground text-xs">
				© <span suppressHydrationWarning={true}>{new Date().getFullYear()}</span> Benjamin Bruington
			</footer>
		</>
	);
}
