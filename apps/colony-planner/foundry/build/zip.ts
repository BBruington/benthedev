export interface ZipEntry {
	data: Uint8Array;
	name: string;
}

const localFileHeaderSignature = 0x04034b50;
const centralDirectorySignature = 0x02014b50;
const endOfCentralDirectorySignature = 0x06054b50;
const zipVersion = 20;
const utf8NamesFlag = 0x0800;
const storedMethod = 0;
// 1980-01-01 00:00, the earliest date a zip can hold: a fixed date keeps the archive byte-for-byte
// the same for the same files, so the embedded zip changes only when the module does.
const dosTime = 0;
const dosDate = (1 << 5) | 1;

/** A zip of stored (uncompressed) entries in the given order, with no timestamps or extra fields. */
export function createZip(entries: ZipEntry[]): Uint8Array {
	const encoder = new TextEncoder();
	const localParts: Uint8Array[] = [];
	const centralParts: Uint8Array[] = [];
	let offset = 0;
	for (const entry of entries) {
		const name = encoder.encode(entry.name);
		const crc = Bun.hash.crc32(entry.data);
		const local = new DataView(new ArrayBuffer(30));
		local.setUint32(0, localFileHeaderSignature, true);
		local.setUint16(4, zipVersion, true);
		local.setUint16(6, utf8NamesFlag, true);
		local.setUint16(8, storedMethod, true);
		local.setUint16(10, dosTime, true);
		local.setUint16(12, dosDate, true);
		local.setUint32(14, crc, true);
		local.setUint32(18, entry.data.length, true);
		local.setUint32(22, entry.data.length, true);
		local.setUint16(26, name.length, true);
		local.setUint16(28, 0, true);
		localParts.push(new Uint8Array(local.buffer), name, entry.data);

		const central = new DataView(new ArrayBuffer(46));
		central.setUint32(0, centralDirectorySignature, true);
		central.setUint16(4, zipVersion, true);
		central.setUint16(6, zipVersion, true);
		central.setUint16(8, utf8NamesFlag, true);
		central.setUint16(10, storedMethod, true);
		central.setUint16(12, dosTime, true);
		central.setUint16(14, dosDate, true);
		central.setUint32(16, crc, true);
		central.setUint32(20, entry.data.length, true);
		central.setUint32(24, entry.data.length, true);
		central.setUint16(28, name.length, true);
		central.setUint32(42, offset, true);
		centralParts.push(new Uint8Array(central.buffer), name);

		offset += 30 + name.length + entry.data.length;
	}
	const centralSize = centralParts.reduce((size, part) => size + part.length, 0);
	const end = new DataView(new ArrayBuffer(22));
	end.setUint32(0, endOfCentralDirectorySignature, true);
	end.setUint16(8, entries.length, true);
	end.setUint16(10, entries.length, true);
	end.setUint32(12, centralSize, true);
	end.setUint32(16, offset, true);
	return concatenate([...localParts, ...centralParts, new Uint8Array(end.buffer)]);
}

function concatenate(parts: Uint8Array[]): Uint8Array {
	const joined = new Uint8Array(parts.reduce((size, part) => size + part.length, 0));
	let position = 0;
	for (const part of parts) {
		joined.set(part, position);
		position += part.length;
	}
	return joined;
}
