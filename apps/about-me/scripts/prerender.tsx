import { renderToString } from "react-dom/server";
import { App } from "../src/app";

const indexPath = `${import.meta.dir}/../dist/index.html`;
const emptyRoot = '<div id="root"></div>';

const indexHtml = await Bun.file(indexPath).text();
if (!indexHtml.includes(emptyRoot)) {
	throw new Error(`${indexPath} has no empty #root to prerender into`);
}

const appHtml = renderToString(<App />);
await Bun.write(
	indexPath,
	indexHtml.replace(emptyRoot, () => `<div id="root">${appHtml}</div>`),
);
