import { fieldErrors, problemMessage, problemStatus } from "@/api/problem";

export const maxImageBytes = 5 * 1024 * 1024;

const acceptedImageTypes = new Set(["image/png", "image/jpeg", "image/webp", "image/gif"]);

/** For a file input's accept attribute, so the picker offers only images the server takes. */
export const imageInputAccept = [...acceptedImageTypes].join(",");

const bytesPerKilobyte = 1024;
const bytesPerMegabyte = 1024 * 1024;

export function megabytes(byteCount: number): string {
	const rounded = Math.round((byteCount / bytesPerMegabyte) * 10) / 10;
	return rounded.toLocaleString("en", { maximumFractionDigits: 1 });
}

/** A file's size the way people say it: KB under a megabyte, MB from there. */
export function fileSize(byteCount: number): string {
	if (byteCount < bytesPerMegabyte) {
		return `${Math.max(1, Math.round(byteCount / bytesPerKilobyte))} KB`;
	}
	return `${megabytes(byteCount)} MB`;
}

/**
 * What's wrong with a chosen file before it's sent, in plain words, or null when it may go. The
 * server reads the real type from the bytes and has the last word; this only saves a wasted upload.
 */
export function imageFileProblem(file: {
	name: string;
	size: number;
	type: string;
}): string | null {
	if (!acceptedImageTypes.has(file.type)) {
		return `${file.name} isn't an image the site can use. Choose a PNG, JPEG, WebP or GIF image.`;
	}
	if (file.size === 0) {
		return `${file.name} is empty. Choose another image.`;
	}
	if (file.size > maxImageBytes) {
		return `${file.name} is ${megabytes(file.size)} MB. Images can be at most 5 MB, so shrink or crop it first.`;
	}
	return null;
}

/** The server's reason an upload failed: its field message for the file, else its detail. */
export function uploadFailureMessage(error: unknown): string {
	const imageError = fieldErrors(error).image;
	if (imageError) {
		return imageError;
	}
	if (problemStatus(error) === 413) {
		return "Images can be at most 5 MB, so shrink or crop it first.";
	}
	return problemMessage(error, "The image couldn't be uploaded. Try again.");
}
