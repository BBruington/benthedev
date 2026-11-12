import { expect, test } from "bun:test";
import { createZip } from "./zip";

const encoder = new TextEncoder();
const decoder = new TextDecoder();

const entries = [
	{ data: encoder.encode('{"id": "colony-bridge"}\n'), name: "module.json" },
	{ data: encoder.encode("export {};\n"), name: "scripts/colony-bridge.js" },
];

/** Reads the archive back through its central directory, as an unzip tool does. */
function readZip(zip: Uint8Array) {
	const view = new DataView(zip.buffer, zip.byteOffset, zip.byteLength);
	const end = zip.length - 22;
	expect(view.getUint32(end, true)).toBe(0x06054b50);
	const count = view.getUint16(end + 10, true);
	let position = view.getUint32(end + 16, true);
	return Array.from({ length: count }, () => {
		expect(view.getUint32(position, true)).toBe(0x02014b50);
		const method = view.getUint16(position + 10, true);
		const crc = view.getUint32(position + 16, true);
		const size = view.getUint32(position + 24, true);
		const nameLength = view.getUint16(position + 28, true);
		const localOffset = view.getUint32(position + 42, true);
		const name = decoder.decode(zip.slice(position + 46, position + 46 + nameLength));
		position += 46 + nameLength;
		expect(view.getUint32(localOffset, true)).toBe(0x04034b50);
		const dataStart = localOffset + 30 + view.getUint16(localOffset + 26, true);
		const data = zip.slice(dataStart, dataStart + size);
		return { crcMatches: Bun.hash.crc32(data) === crc, data: decoder.decode(data), method, name };
	});
}

test("the zip holds each entry, stored, in order, with matching checksums", () => {
	expect(readZip(createZip(entries))).toEqual([
		{ crcMatches: true, data: '{"id": "colony-bridge"}\n', method: 0, name: "module.json" },
		{ crcMatches: true, data: "export {};\n", method: 0, name: "scripts/colony-bridge.js" },
	]);
});

test("the same files always make the same bytes", () => {
	expect(createZip(entries)).toEqual(createZip(entries));
});
