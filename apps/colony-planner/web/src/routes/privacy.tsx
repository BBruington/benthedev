import { createFileRoute, Link } from "@tanstack/react-router";
import { ArrowLeftIcon } from "lucide-react";
import type { ReactNode } from "react";
import { Crest } from "@/components/crest";
import { GiltRule } from "@/components/gilt-rule";
import { Button } from "@/components/ui/button";
import { useDocumentTitle } from "@/hooks/use-document-title";

// Public on purpose: someone deciding whether to sign in must be able to read it first (spec §10 M6).
export const Route = createFileRoute("/privacy")({
	component: PrivacyPage,
});

function PrivacyPage() {
	useDocumentTitle("Privacy", "Colony Planner");
	return (
		<div className="min-h-dvh">
			<a
				className="sr-only z-50 rounded-md bg-primary px-4 py-2 text-primary-foreground focus:not-sr-only focus:fixed focus:top-3 focus:left-3"
				href="#main"
			>
				Skip to content
			</a>
			<header className="border-border border-b pt-[env(safe-area-inset-top)]">
				<div className="mx-auto flex h-14 max-w-3xl items-center px-3 md:h-16 md:px-5">
					<Link
						className="-ml-1 flex min-h-11 min-w-0 items-center gap-2.5 rounded-md px-1 outline-none focus-visible:ring-[3px] focus-visible:ring-ring/45"
						to="/"
					>
						<Crest className="size-7 text-gilt sm:size-8" />
						<span className="truncate font-display font-semibold text-base text-gilt-ink tracking-wide sm:text-lg">
							Colony Planner
						</span>
					</Link>
				</div>
			</header>
			<main
				className="mx-auto w-full max-w-3xl px-4 pt-8 pb-16 outline-none md:px-8 md:pt-12"
				id="main"
				tabIndex={-1}
			>
				<article className="grid gap-9">
					<header className="space-y-3">
						<p className="font-medium text-gilt-ink text-xs uppercase tracking-[0.2em]">Privacy</p>
						<h1 className="font-display font-semibold text-3xl tracking-wide md:text-4xl">
							What this site keeps, and who sees it
						</h1>
						<p className="max-w-prose font-flavor text-lg text-muted-foreground italic">
							Colony Planner is a hobby project. One person, the site owner, runs it for friends'
							tabletop campaigns. It isn't a company, it shows no ads, and it never sells or shares
							your data.
						</p>
					</header>

					<GiltRule />

					<PrivacySection title="What the site keeps">
						<ul className="grid list-disc gap-2 pl-5">
							<li>
								<strong>From Discord, when you sign in:</strong> your Discord user ID, username,
								display name and the address of your avatar picture. Sign-in asks Discord for
								nothing else: not your email, your servers or your messages.
							</li>
							<li>
								<strong>Your place in each campaign:</strong> whether you're its DM or a player,
								whether you've been let in or have left, and your character's name.
							</li>
							<li>
								<strong>Everything people enter in a campaign:</strong> the colony's books and
								timeline, NPCs, places, quests, patrons, requests, maps, the hex map, session dates
								and recaps. DMs and players type all of it themselves.
							</li>
							<li>
								<strong>Pictures</strong> uploaded to a campaign, such as maps and NPC portraits.
							</li>
							<li>
								<strong>If you ask to run a campaign:</strong> its name, its address and your note
								to the site owner.
							</li>
							<li>
								<strong>A sign-in cookie</strong> that keeps you signed in for up to 30 days. The
								server keeps only a scrambled fingerprint of it, and notes the day you last visited.
							</li>
						</ul>
						<p>
							Your browser also remembers a few conveniences on your own device: your theme, the
							last campaign you opened and where to return after signing in. They never leave it.
						</p>
						<p>
							There is no tracking, no advertising and no analytics. The site serves its own code
							and fonts. Two kinds of pictures come from elsewhere: your Discord avatar loads from
							Discord, and a map whose art is a link to another website loads from that website.
						</p>
					</PrivacySection>

					<PrivacySection title="Where it lives">
						<p>
							The site runs on <strong>Railway</strong>, a hosting company, and everything above is
							stored in a <strong>Neon</strong> Postgres database. Both store it for the site owner,
							under their own privacy policies. Connections to the site and to the database are
							encrypted.
						</p>
						<p>
							The server's logs record the pages and API addresses asked for (including campaign
							addresses), IDs, kinds of actions and counts, never what anyone wrote. Railway also
							keeps a log of requests to the site, with your IP address and browser, for 7 days, as
							most hosts do.
						</p>
					</PrivacySection>

					<PrivacySection title="Who can see what">
						<ul className="grid list-disc gap-2 pl-5">
							<li>
								<strong>A campaign's members</strong>, the way the game sets it up. The DM sees
								everything in their campaign. Players see what the DM has revealed: hidden NPCs,
								places, quests, patrons and maps, and anything marked DM-only, stay with the DM. A
								note on a declined or granted request is seen only by the player who asked and the
								DM.
							</li>
							<li>
								<strong>Someone waiting to be let in</strong> sees only that they're waiting, and
								someone removed from a campaign can't open it any more.
							</li>
							<li>
								<strong>Other campaigns</strong> see nothing. Each campaign is sealed off from the
								rest.
							</li>
							<li>
								<strong>The site owner</strong> approves new campaigns and can pause or delete them.
								The site shows them each campaign's name, address, DM, how many people are in it and
								its linked Discord server, never what happens inside a campaign they don't belong
								to. Because they run the database, they could look at it directly, as anyone running
								a website could; they do so only to fix a problem you ask about or to delete data
								you ask them to delete.
							</li>
							<li>
								<strong>Anyone with a campaign's calendar link</strong> can see its session titles,
								times and location. Members can copy the link, and the DM can replace it with a new
								one at any time.
							</li>
						</ul>
					</PrivacySection>

					<PrivacySection title="Beny Bot, the Discord bot">
						<p>
							Beny Bot joins a Discord server only when a DM invites it, and works only in the
							server linked to their campaign. It keeps that server's ID and name, who linked it,
							the channels the DM picked, and the IDs of the events and messages it posts, so it can
							update them later.
						</p>
						<p>
							It can't read the server's messages. It sees only the commands you send it, with your
							Discord user ID so it can find your campaign, and its replies to you are visible only
							to you and show only what players may see. It posts what the DM set up: session
							events, reminders and cancellation notices, and requests and new players in the DM's
							channel. It also messages the site owner when someone asks to run a campaign.
						</p>
					</PrivacySection>

					<PrivacySection title="Leaving, and deleting your data">
						<ul className="grid list-disc gap-2 pl-5">
							<li>
								<strong>Leave a campaign:</strong> inside it, open the menu behind your picture at
								the top right and choose <em>Leave this campaign</em>. You lose access at once; what
								happened in the campaign stays part of its story.
							</li>
							<li>
								<strong>Sign out everywhere</strong>, in the same menu, ends your sign-in on every
								device.
							</li>
							<li>
								<strong>A DM can delete their campaign</strong> from its Settings. Everything in it
								is deleted for good.
							</li>
							<li>
								<strong>Delete your account:</strong> ask the site owner on Discord. There's no
								button for it yet, so they do it by hand: your Discord details and sign-ins are
								deleted and you're taken out of every campaign. Things you wrote into a campaign,
								like a request, stay part of that campaign's story, and your Discord name can stay
								in timeline entries and Beny Bot's Discord messages written while you played.
							</li>
						</ul>
						<p>
							Deleted data can stay in the database's short restore history for up to a week before
							it's gone for good.
						</p>
					</PrivacySection>

					<PrivacySection title="Questions">
						<p>Ask the site owner on Discord. If you don't know how to reach them, your DM does.</p>
					</PrivacySection>

					<div>
						<Button asChild variant="outline">
							<Link to="/">
								<ArrowLeftIcon />
								Back to Colony Planner
							</Link>
						</Button>
					</div>
				</article>
			</main>
		</div>
	);
}

function PrivacySection({ title, children }: { title: string; children: ReactNode }) {
	return (
		<section className="grid max-w-prose gap-3 text-base leading-relaxed">
			<h2 className="font-display font-semibold text-gilt-ink text-xl tracking-wide">{title}</h2>
			{children}
		</section>
	);
}
