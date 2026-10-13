import { mutationOptions } from "@tanstack/react-query";
import { fetchClient, type Schemas } from "./client";

/**
 * Where an uploaded image is served, for an <img> or a Leaflet image overlay. Members only, and players
 * get only images a revealed map or a revealed NPC's portrait uses.
 */
export function imageUrl(campaignId: string, imageId: string): string {
	return `/api/campaigns/${encodeURIComponent(campaignId)}/images/${encodeURIComponent(imageId)}`;
}

/**
 * Uploads map art or a portrait (DM only) as multipart form data. The server reads the type from the
 * bytes (PNG, JPEG, WebP or GIF, at most 5 MB) and returns the stored image for the same bytes again.
 * Like every other mutation, it rejects with the server's problem.
 */
export async function uploadImage(
	campaignId: string,
	file: Blob,
): Promise<Schemas["UploadedImage"]> {
	const form = new FormData();
	form.append("image", file);
	const { data, error } = await fetchClient.POST("/api/campaigns/{campaignId}/images", {
		// The generated type calls the file part a string; the serializer sends the form with the file.
		body: { image: "" },
		bodySerializer: () => form,
		params: { path: { campaignId } },
	});
	if (error !== undefined) {
		throw error;
	}
	return data;
}

/** For useMutation: the variable is the chosen file. */
export function uploadImageMutation(campaignId: string) {
	return mutationOptions({
		mutationFn: (file: Blob) => uploadImage(campaignId, file),
		mutationKey: [
			"post",
			"/api/campaigns/{campaignId}/images",
			{ params: { path: { campaignId } } },
		],
	});
}
