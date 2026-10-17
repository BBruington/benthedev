package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

// imageUploadMaxBodyBytes is the largest upload body: one image at core's limit plus the multipart
// framing around it. requireJSONBodies refuses a longer body before anything reads it.
const imageUploadMaxBodyBytes = core.ImageMaxBytes + 64*1024

var imageTooLargeMessage = fmt.Sprintf("Images can be at most %d MB.", core.ImageMaxBytes/(1024*1024))

// defaultImageUploadReadTimeout is long enough for 5 MB over a slow phone uplink, and finite so an
// upload that stalls frees its connection.
const defaultImageUploadReadTimeout = 2 * time.Minute

// imageFileCacheControl is private because an image is member-only, and no-cache because the same URL
// is 200 for the DM and 404 for a player: a browser shared at the table must ask again before reusing
// the DM's copy. An image ID never gets other bytes, so the ID is the ETag and the revalidation a 304.
const imageFileCacheControl = "private, no-cache"

// imageFileContentSecurityPolicy keeps an image opened on its own from running anything, whatever a
// browser makes of its bytes.
const imageFileContentSecurityPolicy = "default-src 'none'; sandbox"

type imageUploadForm struct {
	Image huma.FormFile `form:"image" required:"true" doc:"A PNG, JPEG, WebP or GIF image of at most 5 MB; the type is read from the bytes"`
}

type imageUploadInput struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	RawBody    huma.MultipartFormFiles[imageUploadForm]
}

type imageFileInput struct {
	CampaignID  uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	ImageID     uuid.UUID `path:"imageId" format:"uuid"`
	IfNoneMatch string    `header:"If-None-Match" doc:"The ETag the browser holds; a match is a 304 once the caller may still read the image"`
}

type imageFileOutput struct {
	Status                int
	ETag                  string `header:"ETag"`
	ContentType           string `header:"Content-Type"`
	CacheControl          string `header:"Cache-Control"`
	ContentSecurityPolicy string `header:"Content-Security-Policy"`
	Body                  []byte
}

func (handlers *handlers) registerImages(api huma.API) {
	Register(api, AccessDm, huma.Operation{
		OperationID:   "uploadImage",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/images",
		Summary:       "Upload map art or a portrait; the same bytes again return the stored image",
		Tags:          []string{"images"},
		DefaultStatus: http.StatusCreated,
		MaxBodyBytes:  imageUploadMaxBodyBytes,
		Middlewares:   huma.Middlewares{handlers.readImageUpload(api)},
	}, func(requestContext context.Context, input *imageUploadInput) (*bodyOutput[core.UploadedImage], error) {
		file := input.RawBody.Data().Image
		defer func() { _ = file.Close() }()
		// One byte past the limit is enough for core to refuse the file with its own message.
		data, err := io.ReadAll(io.LimitReader(file, core.ImageMaxBytes+1))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		uploaded, err := handlers.core.UploadImage(requestContext, viewerFrom(requestContext), data)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.UploadedImage]{Body: uploaded}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID: "getImage",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/images/{imageId}",
		Summary:     "An image's bytes; players get only images a revealed map or a revealed NPC's portrait uses",
		Tags:        []string{"images"},
		Responses: map[string]*huma.Response{
			"200": {
				Description: "The image, with the type read from its bytes at upload",
				Content: map[string]*huma.MediaType{
					"image/*": {Schema: &huma.Schema{Type: huma.TypeString, Format: "binary"}},
				},
			},
		},
	}, func(requestContext context.Context, input *imageFileInput) (*imageFileOutput, error) {
		file, err := handlers.core.GetImage(requestContext, viewerFrom(requestContext), input.ImageID)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		entityTag := `"` + input.ImageID.String() + `"`
		if matchesEntityTag(input.IfNoneMatch, entityTag) {
			return &imageFileOutput{Status: http.StatusNotModified, ETag: entityTag, CacheControl: imageFileCacheControl}, nil
		}
		return &imageFileOutput{
			Status:                http.StatusOK,
			ETag:                  entityTag,
			ContentType:           file.ContentType,
			CacheControl:          imageFileCacheControl,
			ContentSecurityPolicy: imageFileContentSecurityPolicy,
			Body:                  file.Bytes,
		}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "setNpcPortrait",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/npcs/{npcId}/portrait",
		Summary:     "Set an NPC's portrait to an uploaded image, or clear it with null",
		Tags:        []string{"npcs"},
	}, func(requestContext context.Context, input *npcBodyInput[core.NpcPortraitInput]) (*bodyOutput[core.Npc], error) {
		npc, err := handlers.core.SetNpcPortrait(requestContext, viewerFrom(requestContext), input.NpcID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Npc]{Body: npc}, nil
	})
}

// readImageUpload reads the upload's form before huma does, under a deadline of its own: huma sets
// none for a RawBody input, and a body over the cap sent without a Content-Length would otherwise be
// huma's generic 422. It runs after the access check, so only the DM's uploads are read.
func (handlers *handlers) readImageUpload(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(humaContext huma.Context, next func(huma.Context)) {
		deadlineErr := humaContext.SetReadDeadline(time.Now().Add(handlers.imageUploadReadTimeout))
		form, err := humaContext.GetMultipartForm()
		if form != nil {
			// A file over 8 KiB spills to a temporary file, and net/http removes only the forms of the
			// request it created; the Caller middleware's copy is this one, so nothing else deletes it.
			defer func() { _ = form.RemoveAll() }()
		}
		if deadlineErr == nil {
			// Cleared once the body is read, as huma does, so it can't cut the connection mid-handler.
			_ = humaContext.SetReadDeadline(time.Time{})
		}
		var tooLarge *http.MaxBytesError
		switch {
		case err == nil:
			next(humaContext)
		case errors.As(err, &tooLarge):
			_ = huma.WriteErr(api, humaContext, http.StatusRequestEntityTooLarge, imageTooLargeMessage)
		case errors.Is(err, os.ErrDeadlineExceeded):
			_ = huma.WriteErr(api, humaContext, http.StatusRequestTimeout, "The upload took too long. Try again on a faster connection.")
		default:
			_ = huma.WriteErr(api, humaContext, http.StatusUnprocessableEntity, "Upload the image as multipart/form-data with one file field named image.")
		}
	}
}

// matchesEntityTag reports whether an If-None-Match header names entityTag (weakly, as RFC 9110
// compares for If-None-Match) or is "*".
func matchesEntityTag(ifNoneMatch string, entityTag string) bool {
	for candidate := range strings.SplitSeq(ifNoneMatch, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || strings.TrimPrefix(candidate, "W/") == entityTag {
			return true
		}
	}
	return false
}

// isImageUpload reports whether a request is POST /api/campaigns/{campaignId}/images, the one route
// whose body is a file rather than JSON.
func isImageUpload(request *http.Request) bool {
	if request.Method != http.MethodPost {
		return false
	}
	rest, found := strings.CutPrefix(request.URL.Path, "/api/campaigns/")
	if !found {
		return false
	}
	campaignID, suffix, _ := strings.Cut(rest, "/")
	return campaignID != "" && suffix == "images"
}
