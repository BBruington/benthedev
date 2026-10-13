package core

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/BBruington/colony-planner/internal/store"
)

// WorldMap is one image map as a projection sees it (spec §4.4, §5.9): the Mainland and any others the
// DM adds. Players get only revealed maps. Its art is the campaign's own upload (ImageID, served at
// /api/campaigns/{campaignId}/images/{imageId}) or an https address (ExternalURL); never the bundled
// /eldari.webp, which belongs to the sign-in page (C6).
type WorldMap struct {
	ID          uuid.UUID  `json:"id" format:"uuid"`
	Name        string     `json:"name"`
	ImageID     *uuid.UUID `json:"imageId" format:"uuid" nullable:"true" doc:"The uploaded art; null when the map uses externalUrl"`
	ExternalURL string     `json:"externalUrl" doc:"An https address of the art; blank when the map uses imageId"`
	Width       int64      `json:"width" doc:"The art's natural width in pixels"`
	Height      int64      `json:"height" doc:"The art's natural height in pixels"`
	IsMainland  bool       `json:"isMainland"`
	// Pins are the places pinned here, revealed ones only for players. Only GetMap fills it.
	Pins []MapPin    `json:"pins" doc:"Filled on the map's own page; empty in lists"`
	Dm   *WorldMapDm `json:"dm,omitempty"`
}

type WorldMapDm struct {
	Revealed bool `json:"revealed"`
}

// MapPin is a place on a map, as fractions of the art (y from the top), so it survives a new image.
type MapPin struct {
	PlaceID   uuid.UUID `json:"placeId" format:"uuid"`
	PlaceName string    `json:"placeName"`
	X         float64   `json:"x" doc:"0 (left edge) to 1 (right edge)"`
	Y         float64   `json:"y" doc:"0 (top edge) to 1 (bottom edge)"`
}

type WorldMapList struct {
	Maps []WorldMap `json:"maps"`
}

// MapArtInput is a map's art: an uploaded image of this campaign, or an https address with the art's
// natural size (the browser reads it, since the server fetches nothing).
type MapArtInput struct {
	ImageID     *uuid.UUID `json:"imageId" format:"uuid" nullable:"true" doc:"An image from POST .../images; null to use externalUrl"`
	ExternalURL string     `json:"externalUrl" doc:"An https address, at most 2000 characters; blank to use imageId"`
	Width       int64      `json:"width" doc:"The external art's natural width in pixels; ignored for an uploaded image"`
	Height      int64      `json:"height" doc:"The external art's natural height in pixels; ignored for an uploaded image"`
}

// MapInput is what an edit changes: the name. Visibility, the Mainland, the order and the art each have
// their own operation, so a stale edit form can't undo them.
type MapInput struct {
	Name string `json:"name" doc:"1 to 80 characters"`
}

// NewMapInput adds a map: its name and art, whether players start out seeing it, and whether it is the
// Mainland (which takes the flag from any other map).
type NewMapInput struct {
	MapInput
	MapArtInput
	Revealed bool `json:"revealed" doc:"Visible to players; the create form starts it on"`
	Mainland bool `json:"mainland" doc:"Make this the campaign's Mainland"`
}

// MapOrderInput lists every map of the campaign in the order the DM wants.
type MapOrderInput struct {
	MapIDs []uuid.UUID `json:"mapIds" format:"uuid" doc:"Every map of the campaign, each once, in the new order"`
}

// PinInput places a place on a map, as fractions of the art.
type PinInput struct {
	MapID uuid.UUID `json:"mapId" format:"uuid"`
	X     float64   `json:"x" doc:"0 (left edge) to 1 (right edge)"`
	Y     float64   `json:"y" doc:"0 (top edge) to 1 (bottom edge)"`
}

const (
	mapNameMaxLength     = 80
	externalURLMaxLength = 2000
	externalArtMaxSide   = 1 << 20
)

var errMapNotFound = NotFound("That map doesn't exist.")

// ListMaps is the map list in the DM's order.
func (service *Service) ListMaps(requestContext context.Context, viewer Viewer) (WorldMapList, error) {
	if err := requireActive(viewer); err != nil {
		return WorldMapList{}, err
	}
	rows, err := service.queries.ListMaps(requestContext, store.ListMapsParams{CampaignID: viewer.campaignID, DmProjection: viewer.SeesDmFields()})
	if err != nil {
		return WorldMapList{}, err
	}
	maps := make([]WorldMap, 0, len(rows))
	for _, row := range rows {
		maps = append(maps, toWorldMap(row))
	}
	return WorldMapList{Maps: maps}, nil
}

// GetMap is one map with its pins. A hidden map is not found for players, and so are its pins.
func (service *Service) GetMap(requestContext context.Context, viewer Viewer, mapID uuid.UUID) (WorldMap, error) {
	if err := requireActive(viewer); err != nil {
		return WorldMap{}, err
	}
	return readMap(requestContext, service.queries, viewer.campaignID, mapID, viewer.SeesDmFields())
}

// CreateMap adds a map. It is not a colony mutation and writes no timeline entry.
func (service *Service) CreateMap(requestContext context.Context, viewer Viewer, input NewMapInput) (WorldMap, error) {
	if err := requireDm(viewer); err != nil {
		return WorldMap{}, err
	}
	name, err := cleanMapName(input.Name)
	if err != nil {
		return WorldMap{}, err
	}
	var worldMap WorldMap
	err = service.inTransaction(requestContext, func(queries *store.Queries) error {
		// Locked first, so concurrent creates can't each pass the count and go over the limit.
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		count, err := queries.CountMaps(requestContext, viewer.campaignID)
		if err != nil {
			return err
		}
		if count >= MapLimit {
			return Conflict("This campaign has " + formatNumber(MapLimit) + " maps, the most it can hold. Delete one first.")
		}
		art, err := cleanMapArt(requestContext, queries, viewer.campaignID, service.siteHost, input.MapArtInput)
		if err != nil {
			return err
		}
		row, err := queries.CreateMap(requestContext, store.CreateMapParams{
			CampaignID:  viewer.campaignID,
			Name:        name,
			ImageID:     art.ImageID,
			ExternalUrl: art.ExternalUrl,
			Width:       art.Width,
			Height:      art.Height,
			Revealed:    input.Revealed,
		})
		if err != nil {
			return err
		}
		if input.Mainland {
			if err := setMainland(requestContext, queries, viewer.campaignID, row.ID, true); err != nil {
				return err
			}
		}
		worldMap, err = readMap(requestContext, queries, viewer.campaignID, row.ID, true)
		return err
	})
	if err != nil {
		return WorldMap{}, err
	}
	service.publishMapChange(viewer.campaignID)
	return worldMap, nil
}

// RenameMap is the map's edit form: only the name.
func (service *Service) RenameMap(requestContext context.Context, viewer Viewer, mapID uuid.UUID, input MapInput) (WorldMap, error) {
	if err := requireDm(viewer); err != nil {
		return WorldMap{}, err
	}
	name, err := cleanMapName(input.Name)
	if err != nil {
		return WorldMap{}, err
	}
	return service.changeMap(requestContext, viewer, mapID, func(queries *store.Queries) error {
		_, err := queries.RenameMap(requestContext, store.RenameMapParams{CampaignID: viewer.campaignID, ID: mapID, Name: name})
		return mapGone(err)
	})
}

// ReplaceMapArt swaps a map's art. Pins are fractions of the art, so they stay where they were; the old
// image is deleted once nothing else uses it.
func (service *Service) ReplaceMapArt(requestContext context.Context, viewer Viewer, mapID uuid.UUID, input MapArtInput) (WorldMap, error) {
	if err := requireDm(viewer); err != nil {
		return WorldMap{}, err
	}
	return service.changeMap(requestContext, viewer, mapID, func(queries *store.Queries) error {
		// Locked first, so a concurrent upload can't count the old image as in use and then lose it.
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		previous, err := queries.GetMap(requestContext, store.GetMapParams{CampaignID: viewer.campaignID, ID: mapID})
		if err != nil {
			return mapGone(err)
		}
		art, err := cleanMapArt(requestContext, queries, viewer.campaignID, service.siteHost, input)
		if err != nil {
			return err
		}
		art.CampaignID = viewer.campaignID
		art.ID = mapID
		if _, err := queries.SetMapArt(requestContext, art); err != nil {
			return err
		}
		return releaseImage(requestContext, queries, viewer.campaignID, previous.ImageID)
	})
}

// SetMapRevealed is the DM's Visible to players switch. Hiding a map hides its pins from players too.
func (service *Service) SetMapRevealed(requestContext context.Context, viewer Viewer, mapID uuid.UUID, revealed bool) (WorldMap, error) {
	if err := requireDm(viewer); err != nil {
		return WorldMap{}, err
	}
	return service.changeMap(requestContext, viewer, mapID, func(queries *store.Queries) error {
		_, err := queries.SetMapRevealed(requestContext, store.SetMapRevealedParams{CampaignID: viewer.campaignID, ID: mapID, Revealed: revealed})
		return mapGone(err)
	})
}

// SetMapMainland makes this map the Mainland, taking the flag from any other, or with false leaves the
// campaign without one.
func (service *Service) SetMapMainland(requestContext context.Context, viewer Viewer, mapID uuid.UUID, mainland bool) (WorldMap, error) {
	if err := requireDm(viewer); err != nil {
		return WorldMap{}, err
	}
	return service.changeMap(requestContext, viewer, mapID, func(queries *store.Queries) error {
		return setMainland(requestContext, queries, viewer.campaignID, mapID, mainland)
	})
}

// ReorderMaps sets the list's order. The body names every map once, so a list that changed since the
// DM loaded it is refused instead of half applied.
func (service *Service) ReorderMaps(requestContext context.Context, viewer Viewer, input MapOrderInput) (WorldMapList, error) {
	if err := requireDm(viewer); err != nil {
		return WorldMapList{}, err
	}
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		existing, err := queries.ListMapIDs(requestContext, viewer.campaignID)
		if err != nil {
			return err
		}
		requested := slices.Clone(input.MapIDs)
		slices.SortFunc(requested, compareUUIDs)
		slices.SortFunc(existing, compareUUIDs)
		if !slices.Equal(requested, existing) {
			return Conflict("The maps changed since you opened this list. Reload it and try again.")
		}
		for index, mapID := range input.MapIDs {
			if err := queries.SetMapSortOrder(requestContext, store.SetMapSortOrderParams{CampaignID: viewer.campaignID, ID: mapID, SortOrder: int32(index)}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return WorldMapList{}, err
	}
	service.publishMapChange(viewer.campaignID)
	return service.ListMaps(requestContext, viewer)
}

// DeleteMap deletes a map. Its places keep their record and lose only their pin; its image is deleted
// once nothing else uses it.
func (service *Service) DeleteMap(requestContext context.Context, viewer Viewer, mapID uuid.UUID) error {
	if err := requireDm(viewer); err != nil {
		return err
	}
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		if err := queries.ClearMapPins(requestContext, store.ClearMapPinsParams{CampaignID: viewer.campaignID, MapID: &mapID}); err != nil {
			return err
		}
		imageID, err := queries.DeleteMap(requestContext, store.DeleteMapParams{CampaignID: viewer.campaignID, ID: mapID})
		if err != nil {
			return mapGone(err)
		}
		return releaseImage(requestContext, queries, viewer.campaignID, imageID)
	})
	if err != nil {
		return err
	}
	service.publishMapChange(viewer.campaignID)
	return nil
}

// changeMap runs one map write in a transaction, reads the map back for the DM and publishes.
func (service *Service) changeMap(requestContext context.Context, viewer Viewer, mapID uuid.UUID, write func(queries *store.Queries) error) (WorldMap, error) {
	var worldMap WorldMap
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		if err := write(queries); err != nil {
			return err
		}
		var err error
		worldMap, err = readMap(requestContext, queries, viewer.campaignID, mapID, true)
		return err
	})
	if err != nil {
		return WorldMap{}, err
	}
	service.publishMapChange(viewer.campaignID)
	return worldMap, nil
}

func setMainland(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, mapID uuid.UUID, mainland bool) error {
	// Locked first, so two maps made the Mainland at once can't both clear the flag and then collide.
	if _, err := queries.GetColonyForUpdate(requestContext, campaignID); err != nil {
		return campaignGone(err)
	}
	if _, err := queries.GetMap(requestContext, store.GetMapParams{CampaignID: campaignID, ID: mapID}); err != nil {
		return mapGone(err)
	}
	if mainland {
		if err := queries.ClearMainland(requestContext, campaignID); err != nil {
			return err
		}
	}
	_, err := queries.SetMapMainland(requestContext, store.SetMapMainlandParams{CampaignID: campaignID, ID: mapID, IsMainland: mainland})
	return mapGone(err)
}

// publishMapChange: place pages show their pin and its map, so they follow the map too.
func (service *Service) publishMapChange(campaignID uuid.UUID) {
	service.broker.Publish(campaignID, TopicMaps, TopicPlaces)
}

func readMap(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, mapID uuid.UUID, dmProjection bool) (WorldMap, error) {
	row, err := queries.GetMapView(requestContext, store.GetMapViewParams{CampaignID: campaignID, ID: mapID, DmProjection: dmProjection})
	if err != nil {
		return WorldMap{}, mapGone(err)
	}
	pins, err := queries.ListMapPins(requestContext, store.ListMapPinsParams{CampaignID: campaignID, MapID: &mapID, DmProjection: dmProjection})
	if err != nil {
		return WorldMap{}, err
	}
	worldMap := toWorldMap(row)
	for _, pin := range pins {
		worldMap.Pins = append(worldMap.Pins, MapPin{PlaceID: pin.ID, PlaceName: pin.Name, X: float64(pin.PinX), Y: float64(pin.PinY)})
	}
	return worldMap, nil
}

func mapGone(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return errMapNotFound
	}
	return err
}

// toWorldMap fills Dm unconditionally; the Transformer decides who keeps it.
func toWorldMap(row store.Map) WorldMap {
	worldMap := WorldMap{
		ID:         row.ID,
		Name:       row.Name,
		ImageID:    row.ImageID,
		Width:      int64(row.Width),
		Height:     int64(row.Height),
		IsMainland: row.IsMainland,
		Pins:       []MapPin{},
		Dm:         &WorldMapDm{Revealed: row.Revealed},
	}
	if row.ExternalUrl != nil {
		worldMap.ExternalURL = *row.ExternalUrl
	}
	return worldMap
}

func cleanMapName(value string) (string, error) {
	name, fieldError := cleanText("name", value, mapNameMaxLength)
	if fieldError != nil {
		return "", Invalid("The map needs fixing.", *fieldError)
	}
	if name == "" {
		return "", Invalid("The map needs fixing.", FieldError{Field: "name", Message: "Give the map a name."})
	}
	return name, nil
}

// cleanMapArt checks the art: exactly one of an image of this campaign (whose size is read from the
// image) and an https address on another site, with its size.
func cleanMapArt(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, siteHost string, input MapArtInput) (store.SetMapArtParams, error) {
	externalURL, fieldError := cleanText("externalUrl", input.ExternalURL, externalURLMaxLength)
	if fieldError != nil {
		return store.SetMapArtParams{}, Invalid("The map's art needs fixing.", *fieldError)
	}
	switch {
	case input.ImageID != nil && externalURL != "":
		return store.SetMapArtParams{}, Invalid("Use an uploaded image or an address, not both.",
			FieldError{Field: "externalUrl", Message: "Clear the address to use the uploaded image."})
	case input.ImageID != nil:
		image, err := loadImage(requestContext, queries, campaignID, *input.ImageID, "imageId")
		if err != nil {
			return store.SetMapArtParams{}, err
		}
		imageID := image.ID
		return store.SetMapArtParams{ImageID: &imageID, Width: image.Width, Height: image.Height}, nil
	case externalURL == "":
		return store.SetMapArtParams{}, Invalid("Upload the map's art or give its address.",
			FieldError{Field: "imageId", Message: "Upload the map's art or give its address."})
	}
	var fieldErrors []FieldError
	// Only an absolute https address on another site: the bundled /eldari.webp and every other path on
	// this site are refused, whatever the port or case, so a map can't borrow the sign-in page's art (C6).
	parsed, err := url.Parse(externalURL)
	switch {
	case err != nil || parsed.Scheme != "https" || parsed.Hostname() == "":
		fieldErrors = append(fieldErrors, FieldError{Field: "externalUrl", Message: "Use an https:// address."})
	case !isPlainHostName(parsed.Hostname()):
		fieldErrors = append(fieldErrors, FieldError{Field: "externalUrl", Message: "Use an address whose site name is plain letters, digits, dots and hyphens."})
	case siteHost != "" && strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".") == siteHost:
		fieldErrors = append(fieldErrors, FieldError{Field: "externalUrl", Message: "Use an address on another site, or upload the art."})
	}
	if input.Width <= 0 || input.Width > externalArtMaxSide {
		fieldErrors = append(fieldErrors, FieldError{Field: "width", Message: "Give the art's width in pixels."})
	}
	if input.Height <= 0 || input.Height > externalArtMaxSide {
		fieldErrors = append(fieldErrors, FieldError{Field: "height", Message: "Give the art's height in pixels."})
	}
	if len(fieldErrors) > 0 {
		return store.SetMapArtParams{}, Invalid("The map's art needs fixing.", fieldErrors...)
	}
	return store.SetMapArtParams{ExternalUrl: &externalURL, Width: int32(input.Width), Height: int32(input.Height)}, nil
}

// isPlainHostName reports whether a host name needs no mapping before a browser connects to it. A
// browser folds fullwidth letters, drops soft hyphens and decodes percent escapes in a host, so a name
// that passes the site check here could still load this site; only an ASCII name compares as loaded.
func isPlainHostName(host string) bool {
	for _, character := range host {
		isLetter := (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z')
		isDigit := character >= '0' && character <= '9'
		if !isLetter && !isDigit && character != '.' && character != '-' {
			return false
		}
	}
	return host != ""
}

func compareUUIDs(first uuid.UUID, second uuid.UUID) int {
	return bytes.Compare(first[:], second[:])
}
