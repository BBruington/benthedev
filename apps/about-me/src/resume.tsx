import { Section } from "./section";

const ROLES = [
	{
		company: "Axiaras",
		dates: "Jun 2025 – Nov 2025",
		highlights: [
			"Built an LLM pipeline that turns raw Jira and GitHub activity into plain-language client updates",
			"Shipped an AI timeline endpoint that summarizes any date range as a summary or weekly digest",
			"Owned the Jira integration end to end: OAuth install, token refresh and per-project webhooks",
			"Built multi-tenant org onboarding on Clerk: invitations, join requests and role management",
			"Scaffolded a Cloudflare Workers integrations service with rate limiting and Terraform",
			"Wrote 200+ automated tests across TypeScript services and Go webhook handlers",
		],
		location: "Melbourne, Australia",
		title: "Full-Stack Developer",
	},
	{
		company: "Squared",
		dates: "Nov 2024 – Jun 2025",
		highlights: [
			"Introduced Prismic CMS so non-engineers could publish docs without a deploy",
			"Built task dependencies end to end, from schema and RPC service to “blocked by” UI",
			"Extended the Slate rich-text editor to 3 surfaces with task autocomplete, code blocks and MDX conversion",
			"Built @mention notifications with hover-card previews for tasks and comments",
			"Automated Postgres backups to S3 with GitHub Actions and seeded local DBs from Neon",
		],
		location: "Illinois, United States",
		title: "Full-Stack Developer",
	},
];

type EarlierProject = {
	name: string;
	href?: string;
	description: string;
};

const EARLIER_PROJECTS: EarlierProject[] = [
	{
		description: "Open-source social app connecting photographers with cosplayers",
		href: "https://github.com/Ionia-Devs/chameleon",
		name: "Chameleon",
	},
	{
		description: "Live multiplayer puzzles: combine ingredients to craft potions",
		name: "Chains of Fate Trials",
	},
	{
		description: "Finds nearby coffee shops with Foursquare geolocation",
		href: "https://discover-coffee-sigma.vercel.app/",
		name: "Discover Coffee",
	},
];

const SKILLS = [
	{ group: "Languages", items: "TypeScript, Go" },
	{ group: "AI", items: "OpenAI API, LLM prompt design, Claude Code" },
	{ group: "Frontend", items: "React, Next.js, Tailwind CSS, TanStack Query/Router, Zustand" },
	{ group: "Backend", items: "Node.js, Hono, ConnectRPC, Protobuf, Drizzle, Prisma" },
	{
		group: "Infra",
		items: "Cloudflare Workers, AWS, Terraform, Docker, GitHub Actions, Neon Postgres",
	},
];

export function Experience() {
	return (
		<Section id="experience" title="Experience">
			<ol className="space-y-14">
				{ROLES.map((role) => (
					<li key={role.company}>
						<div className="flex flex-wrap items-baseline justify-between gap-x-6 gap-y-1">
							<h3 className="font-medium font-serif text-2xl tracking-tight">
								{role.company}
								<span className="text-muted-foreground"> · {role.title}</span>
							</h3>
							<p className="font-mono text-muted-foreground text-xs tabular-nums">{role.dates}</p>
						</div>
						<p className="mt-1 text-muted-foreground text-sm">{role.location}</p>
						<ul className="mt-5 max-w-prose space-y-2.5 text-[15px] leading-relaxed">
							{role.highlights.map((highlight) => (
								<li
									className="relative pl-5 before:absolute before:top-[0.7em] before:left-0 before:h-px before:w-2.5 before:bg-primary"
									key={highlight}
								>
									{highlight}
								</li>
							))}
						</ul>
					</li>
				))}
			</ol>
		</Section>
	);
}

export function Projects() {
	return (
		<Section id="projects" title="Projects">
			<article className="rounded-lg border border-border bg-card p-6 sm:p-8">
				<div className="flex flex-wrap items-baseline justify-between gap-x-6 gap-y-1">
					<h3 className="font-medium font-serif text-2xl tracking-tight">Party Planner</h3>
					<p className="font-mono text-muted-foreground text-xs tabular-nums">Nov 2025 – Present</p>
				</div>
				<p className="mt-3 max-w-prose text-muted-foreground leading-relaxed">
					A tabletop campaign manager built with TanStack Start and a Go ConnectRPC API on Neon.
					Developed AI-first with Claude Code, guided by a CLAUDE.md playbook that agents follow.
				</p>
			</article>
			<h3 className="mt-12 font-serif text-lg text-muted-foreground italic">Earlier work</h3>
			<ul className="mt-4 divide-y divide-border border-border border-y">
				{EARLIER_PROJECTS.map((project) => (
					<li
						className="flex flex-col gap-1 py-4 sm:flex-row sm:items-baseline sm:gap-6"
						key={project.name}
					>
						{project.href ? (
							<a
								className="link-underlined shrink-0 font-medium sm:w-48"
								href={project.href}
								rel="noreferrer"
								target="_blank"
							>
								{project.name}
							</a>
						) : (
							<span className="shrink-0 font-medium sm:w-48">{project.name}</span>
						)}
						<p className="text-muted-foreground text-sm">{project.description}</p>
					</li>
				))}
			</ul>
		</Section>
	);
}

export function Skills() {
	return (
		<Section id="skills" title="Skills">
			<dl className="divide-y divide-border">
				{SKILLS.map(({ group, items }) => (
					<div className="grid gap-1 py-3 first:pt-0 sm:grid-cols-[8rem_1fr] sm:gap-6" key={group}>
						<dt className="font-medium">{group}</dt>
						<dd className="text-muted-foreground">{items}</dd>
					</div>
				))}
			</dl>
		</Section>
	);
}
