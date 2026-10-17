package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"image"
	// The decoders image.DecodeConfig recognizes; WebP is read by readWebpSize.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/BBruington/colony-planner/internal/store"
)

// UploadedImage is an image in the campaign's store (spec §8.9): map art or an NPC portrait. The bytes
// are served at /api/campaigns/{campaignId}/images/{id}.
type UploadedImage struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	ContentType string    `json:"contentType" doc:"image/png, image/jpeg, image/webp or image/gif"`
	Width       int64     `json:"width" doc:"Natural width in pixels"`
	Height      int64     `json:"height" doc:"Natural height in pixels"`
	ByteSize    int64     `json:"byteSize"`
}

// ImageFile is an image's bytes as GetImage serves them. Images never change, so a URL can be cached
// for good.
type ImageFile struct {
	ContentType string
	Bytes       []byte
}

var (
	errImageNotFound = NotFound("That image doesn't exist.")
	errNotAnImage    = Invalid("Upload a PNG, JPEG, WebP or GIF image.", FieldError{Field: "image", Message: "Upload a PNG, JPEG, WebP or GIF image."})
)

// UploadImage stores an image for a map or a portrait (DM only). The type comes from the bytes, never a
// file name or a header: PNG, JPEG, WebP and GIF only, so an SVG (which can carry script) is refused.
// The same bytes uploaded again return the stored copy. Nothing shows the image until a map or a
// portrait uses it, and an upload nothing takes up within an hour stops counting against the budget.
func (service *Service) UploadImage(requestContext context.Context, viewer Viewer, data []byte) (UploadedImage, error) {
	if err := requireDm(viewer); err != nil {
		return UploadedImage{}, err
	}
	if len(data) > ImageMaxBytes {
		message := "Images can be at most " + formatMegabytes(ImageMaxBytes) + "."
		return UploadedImage{}, Invalid(message, FieldError{Field: "image", Message: message})
	}
	header, err := readImageHeader(data)
	if err != nil {
		return UploadedImage{}, err
	}
	digest := sha256.Sum256(data)
	var uploaded UploadedImage
	err = service.inTransaction(requestContext, func(queries *store.Queries) error {
		// Locked first, so concurrent uploads can't each pass the budget check and go over it.
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		if _, err := queries.DeleteStaleUnusedImages(requestContext, viewer.campaignID); err != nil {
			return err
		}
		existing, err := queries.ReofferImageBySha256(requestContext, store.ReofferImageBySha256Params{CampaignID: viewer.campaignID, Sha256: digest[:]})
		if err == nil {
			uploaded = uploadedImage(existing.ID, existing.ContentType, existing.Width, existing.Height, existing.ByteSize)
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		used, err := queries.SumImageBytes(requestContext, viewer.campaignID)
		if err != nil {
			return err
		}
		if used+int64(len(data)) > ImageBudgetBytes {
			return Conflict("This campaign's images use " + formatMegabytes(used) + " of " + formatMegabytes(ImageBudgetBytes) +
				". Replace or remove a map or portrait first.")
		}
		memberID := viewer.memberID
		row, err := queries.CreateImage(requestContext, store.CreateImageParams{
			CampaignID:  viewer.campaignID,
			ContentType: header.contentType,
			Bytes:       data,
			Sha256:      digest[:],
			Width:       header.width,
			Height:      header.height,
			UploadedBy:  &memberID,
		})
		if err != nil {
			return err
		}
		uploaded = uploadedImage(row.ID, row.ContentType, row.Width, row.Height, row.ByteSize)
		return nil
	})
	if err != nil {
		return UploadedImage{}, err
	}
	return uploaded, nil
}

// GetImage serves an image's bytes. Players get it only while a revealed map or a revealed NPC's
// portrait uses it; any other image is not found, exactly like a missing one.
func (service *Service) GetImage(requestContext context.Context, viewer Viewer, imageID uuid.UUID) (ImageFile, error) {
	if err := requireActive(viewer); err != nil {
		return ImageFile{}, err
	}
	row, err := service.queries.GetImageView(requestContext, store.GetImageViewParams{
		CampaignID:   viewer.campaignID,
		ID:           imageID,
		DmProjection: viewer.SeesDmFields(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ImageFile{}, errImageNotFound
	}
	if err != nil {
		return ImageFile{}, err
	}
	return ImageFile{ContentType: row.ContentType, Bytes: row.Bytes}, nil
}

// loadImage reads an image ID from a request body with the viewer's campaign, so another campaign's
// image is a field error.
func loadImage(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, imageID uuid.UUID, field string) (store.GetImageInfoRow, error) {
	row, err := queries.GetImageInfo(requestContext, store.GetImageInfoParams{CampaignID: campaignID, ID: imageID})
	if errors.Is(err, pgx.ErrNoRows) {
		return store.GetImageInfoRow{}, Invalid("That image doesn't exist. Upload it again.", FieldError{Field: field, Message: "That image doesn't exist. Upload it again."})
	}
	return row, err
}

// releaseImage deletes a replaced or removed image once nothing uses it, so the budget counts only art
// in use.
func releaseImage(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, imageID *uuid.UUID) error {
	if imageID == nil {
		return nil
	}
	_, err := queries.DeleteImageIfUnused(requestContext, store.DeleteImageIfUnusedParams{CampaignID: campaignID, ID: *imageID})
	return err
}

func uploadedImage(id uuid.UUID, contentType string, width int32, height int32, byteSize int32) UploadedImage {
	return UploadedImage{ID: id, ContentType: contentType, Width: int64(width), Height: int64(height), ByteSize: int64(byteSize)}
}

type imageHeader struct {
	contentType string
	width       int32
	height      int32
}

// readImageHeader sniffs the type from the bytes and reads the size from the header alone, so a huge
// image is never decoded on the server.
func readImageHeader(data []byte) (imageHeader, error) {
	if width, height, isWebp := readWebpSize(data); isWebp {
		return checkedImageHeader("image/webp", width, height)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return imageHeader{}, errNotAnImage
	}
	contentTypes := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "gif": "image/gif"}
	contentType, known := contentTypes[format]
	if !known {
		return imageHeader{}, errNotAnImage
	}
	return checkedImageHeader(contentType, config.Width, config.Height)
}

func checkedImageHeader(contentType string, width int, height int) (imageHeader, error) {
	if width <= 0 || height <= 0 {
		return imageHeader{}, errNotAnImage
	}
	if width > ImageMaxSide || height > ImageMaxSide || width*height > ImageMaxPixels {
		message := "Images can be at most " + formatNumber(ImageMaxSide) + " pixels on a side and " +
			formatNumber(ImageMaxPixels/1_000_000) + " megapixels in all. Make it smaller and upload it again."
		return imageHeader{}, Invalid(message, FieldError{Field: "image", Message: message})
	}
	return imageHeader{contentType: contentType, width: int32(width), height: int32(height)}, nil
}

// readWebpSize reads a WebP file's canvas size from its first chunk (lossy VP8, lossless VP8L or the
// extended VP8X), the three layouts of Google's container specification.
func readWebpSize(data []byte) (width int, height int, isWebp bool) {
	if len(data) < 30 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return 0, 0, false
	}
	chunk := data[20:]
	switch string(data[12:16]) {
	case "VP8 ":
		if chunk[3] != 0x9d || chunk[4] != 0x01 || chunk[5] != 0x2a {
			return 0, 0, false
		}
		return int(binary.LittleEndian.Uint16(chunk[6:8]) & 0x3fff), int(binary.LittleEndian.Uint16(chunk[8:10]) & 0x3fff), true
	case "VP8L":
		if chunk[0] != 0x2f {
			return 0, 0, false
		}
		bits := binary.LittleEndian.Uint32(chunk[1:5])
		return int(bits&0x3fff) + 1, int((bits>>14)&0x3fff) + 1, true
	case "VP8X":
		return int(uint32(chunk[4])|uint32(chunk[5])<<8|uint32(chunk[6])<<16) + 1, int(uint32(chunk[7])|uint32(chunk[8])<<8|uint32(chunk[9])<<16) + 1, true
	}
	return 0, 0, false
}

func formatMegabytes(byteCount int64) string {
	tenths := (byteCount*10 + 512*1024) / (1024 * 1024)
	whole := formatNumber(tenths / 10)
	if tenths%10 == 0 {
		return whole + " MB"
	}
	return whole + "." + formatNumber(tenths%10) + " MB"
}
