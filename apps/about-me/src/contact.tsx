import { CopyEmailButton } from "./copy-email-button";
import { EMAIL, PHONE, PHONE_HREF } from "./profile";
import { Section } from "./section";

export function Contact() {
	return (
		<Section id="contact" title="Contact">
			<p className="max-w-md text-balance font-serif text-2xl leading-snug tracking-tight">
				Open to full-time roles. Send a note and I'll get back to you.
			</p>
			<div className="mt-8 flex flex-wrap items-center gap-x-5 gap-y-3">
				<a className="link-underlined font-serif text-2xl sm:text-3xl" href={`mailto:${EMAIL}`}>
					{EMAIL}
				</a>
				<CopyEmailButton />
			</div>
			<a
				className="link-underlined mt-5 inline-block text-muted-foreground text-sm"
				href={PHONE_HREF}
			>
				{PHONE}
			</a>
		</Section>
	);
}
