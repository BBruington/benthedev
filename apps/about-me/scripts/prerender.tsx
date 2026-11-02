import { renderToString } from "react-dom/server";
import { App } from "../src/app";
import { EMAIL, FULL_NAME, GITHUB_URL, LINKEDIN_URL, SITE_URL } from "../src/profile";

const indexPath = `${import.meta.dir}/../dist/index.html`;
const emptyRoot = '<div id="root"></div>';
const headEnd = "</head>";

const indexHtml = await Bun.file(indexPath).text();
if (!indexHtml.includes(emptyRoot)) {
	throw new Error(`${indexPath} has no empty #root to prerender into`);
}
if (!indexHtml.includes(headEnd)) {
	throw new Error(`${indexPath} has no </head> to add structured data to`);
}

const structuredData = {
	"@context": "https://schema.org",
	"@type": "ProfilePage",
	mainEntity: {
		"@type": "Person",
		additionalName: "Isaac",
		alternateName: ["Benjamin Bruington", "Ben Bruington"],
		email: `mailto:${EMAIL}`,
		familyName: "Bruington",
		givenName: "Benjamin",
		jobTitle: "Full-stack engineer",
		name: FULL_NAME,
		sameAs: [GITHUB_URL, LINKEDIN_URL],
		url: SITE_URL,
	},
	url: SITE_URL,
};
// Escaping "<" keeps the JSON from ever closing the script element early.
const structuredDataJson = JSON.stringify(structuredData).replaceAll("<", "\\u003c");

const appHtml = renderToString(<App />);
await Bun.write(
	indexPath,
	indexHtml
		.replace(
			headEnd,
			() => `<script type="application/ld+json">${structuredDataJson}</script>${headEnd}`,
		)
		.replace(emptyRoot, () => `<div id="root">${appHtml}</div>`),
);
