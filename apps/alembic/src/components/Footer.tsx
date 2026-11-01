export default function Footer() {
	return (
		<footer className="mt-16 border-t border-border py-8">
			<div className="page-wrap max-w-5xl space-y-3 text-xs leading-relaxed text-muted-foreground">
				<p>
					This app uses trademarks and/or copyrights owned by Paizo Inc., used
					under Paizo's Community Use Policy (
					<a
						href="https://paizo.com/licenses/communityuse"
						className="underline underline-offset-4 hover:text-foreground"
					>
						paizo.com/licenses/communityuse
					</a>
					). We are expressly prohibited from charging you to use or access this
					content. This app is not published, endorsed, or specifically approved
					by Paizo.
				</p>
				<p>Rules mechanics are under the ORC License.</p>
				<p>All sample content is fictional.</p>
			</div>
		</footer>
	);
}
