package core

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/BBruington/colony-planner/internal/store"
)

// Place is one place as a projection sees it (spec §4.4, §5.9). Players get only revealed places;
// dm_notes and the revealed switch live in Dm.
type Place struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	PlayerNotes string    `json:"playerNotes"`
	// Pin is null when the place has none, or for players while its map is hidden.
	Pin *PlacePin `json:"pin" doc:"null when unpinned or when its map is hidden from this viewer"`
	// NpcsHere lists the NPCs whose current place this is, revealed ones only for players. Only
	// GetPlace fills it; lists leave it empty.
	NpcsHere []NpcReference `json:"npcsHere" doc:"Filled on the place's own page; empty in lists"`
	Dm       *PlaceDm       `json:"dm,omitempty"`
}

type PlaceDm struct {
	DmNotes  string `json:"dmNotes"`
	Revealed bool   `json:"revealed"`
}

// PlacePin is a place's spot on a map, as fractions of the art (y from the top), so it survives a new
// image.
type PlacePin struct {
	_       struct{}  `nullable:"true"`
	MapID   uuid.UUID `json:"mapId" format:"uuid"`
	MapName string    `json:"mapName"`
	X       float64   `json:"x" doc:"0 (left edge) to 1 (right edge)"`
	Y       float64   `json:"y" doc:"0 (top edge) to 1 (bottom edge)"`
}

type PlaceList struct {
	Places []Place `json:"places"`
}

// PlaceInput replaces a place's record (PUT semantics). Only SetPlaceRevealed changes visibility, so
// an edit form opened before the DM hid the place can't reveal it again.
type PlaceInput struct {
	Name        string `json:"name" doc:"1 to 80 characters"`
	Description string `json:"description" doc:"At most 4000 characters"`
	PlayerNotes string `json:"playerNotes" doc:"At most 4000 characters"`
	DmNotes     string `json:"dmNotes" doc:"At most 4000 characters"`
}

// NewPlaceInput adds a place: its record and whether players start out seeing it.
type NewPlaceInput struct {
	PlaceInput
	Revealed bool `json:"revealed" doc:"Visible to players; the create form starts it on"`
}

const placeNameMaxLength = 80

var errPlaceNotFound = NotFound("That place doesn't exist.")

// ListPlaces is /places, optionally narrowed to names containing search.
func (service *Service) ListPlaces(requestContext context.Context, viewer Viewer, search string) (PlaceList, error) {
	if err := requireActive(viewer); err != nil {
		return PlaceList{}, err
	}
	parameters := store.ListPlacesParams{CampaignID: viewer.campaignID, DmProjection: viewer.SeesDmFields()}
	cleaned, fieldError := cleanText("search", search, placeNameMaxLength)
	if fieldError != nil {
		return PlaceList{}, Invalid("The search needs fixing.", *fieldError)
	}
	if cleaned != "" {
		pattern := "%" + escapeLike(cleaned) + "%"
		parameters.Search = &pattern
	}
	rows, err := service.queries.ListPlaces(requestContext, parameters)
	if err != nil {
		return PlaceList{}, err
	}
	places := make([]Place, 0, len(rows))
	for _, row := range rows {
		places = append(places, toPlace(store.GetPlaceViewRow(row)))
	}
	return PlaceList{Places: places}, nil
}

// GetPlace is one place's page with the NPCs who are here. A hidden place is not found for players.
func (service *Service) GetPlace(requestContext context.Context, viewer Viewer, placeID uuid.UUID) (Place, error) {
	if err := requireActive(viewer); err != nil {
		return Place{}, err
	}
	return readPlace(requestContext, service.queries, viewer.campaignID, placeID, viewer.SeesDmFields())
}

// CreatePlace adds a place. It is not a colony mutation.
func (service *Service) CreatePlace(requestContext context.Context, viewer Viewer, input NewPlaceInput) (Place, error) {
	if err := requireDm(viewer); err != nil {
		return Place{}, err
	}
	cleaned, err := cleanPlaceInput(input.PlaceInput)
	if err != nil {
		return Place{}, err
	}
	var place Place
	err = service.inTransaction(requestContext, func(queries *store.Queries) error {
		// Locked first, so concurrent creates can't each pass the count and go over the limit.
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		count, err := queries.CountPlaces(requestContext, viewer.campaignID)
		if err != nil {
			return err
		}
		if count >= PlaceLimit {
			return Conflict("This campaign has " + formatNumber(PlaceLimit) + " places, the most it can hold. Delete one first.")
		}
		row, err := queries.CreatePlace(requestContext, store.CreatePlaceParams{
			CampaignID:  viewer.campaignID,
			Name:        cleaned.Name,
			Description: cleaned.Description,
			PlayerNotes: cleaned.PlayerNotes,
			DmNotes:     cleaned.DmNotes,
			Revealed:    input.Revealed,
		})
		if err != nil {
			return err
		}
		place, err = readPlace(requestContext, queries, viewer.campaignID, row.ID, true)
		return err
	})
	if err != nil {
		return Place{}, err
	}
	service.publishPlaceChange(viewer.campaignID)
	return place, nil
}

// UpdatePlace replaces a place's record, leaving its visibility as it is.
func (service *Service) UpdatePlace(requestContext context.Context, viewer Viewer, placeID uuid.UUID, input PlaceInput) (Place, error) {
	if err := requireDm(viewer); err != nil {
		return Place{}, err
	}
	cleaned, err := cleanPlaceInput(input)
	if err != nil {
		return Place{}, err
	}
	var place Place
	err = service.inTransaction(requestContext, func(queries *store.Queries) error {
		cleaned.CampaignID = viewer.campaignID
		cleaned.ID = placeID
		if _, err := queries.UpdatePlace(requestContext, cleaned); err != nil {
			return placeGone(err)
		}
		var err error
		place, err = readPlace(requestContext, queries, viewer.campaignID, placeID, true)
		return err
	})
	if err != nil {
		return Place{}, err
	}
	service.publishPlaceChange(viewer.campaignID)
	return place, nil
}

// SetPlaceRevealed is the DM's Visible to players switch.
func (service *Service) SetPlaceRevealed(requestContext context.Context, viewer Viewer, placeID uuid.UUID, revealed bool) (Place, error) {
	if err := requireDm(viewer); err != nil {
		return Place{}, err
	}
	var place Place
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		if _, err := queries.SetPlaceRevealed(requestContext, store.SetPlaceRevealedParams{CampaignID: viewer.campaignID, ID: placeID, Revealed: revealed}); err != nil {
			return placeGone(err)
		}
		var err error
		place, err = readPlace(requestContext, queries, viewer.campaignID, placeID, true)
		return err
	})
	if err != nil {
		return Place{}, err
	}
	service.publishPlaceChange(viewer.campaignID)
	return place, nil
}

// DeletePlace deletes a place; NPCs here or from here keep their record with that place cleared.
func (service *Service) DeletePlace(requestContext context.Context, viewer Viewer, placeID uuid.UUID) error {
	if err := requireDm(viewer); err != nil {
		return err
	}
	deleted, err := service.queries.DeletePlace(requestContext, store.DeletePlaceParams{CampaignID: viewer.campaignID, ID: placeID})
	if err != nil {
		return err
	}
	if deleted == 0 {
		return errPlaceNotFound
	}
	service.publishPlaceChange(viewer.campaignID)
	return nil
}

// SetPlacePin puts a place on a map, or moves it there (DM only). The map ID is loaded with the viewer's
// campaign, so another campaign's map is a field error.
func (service *Service) SetPlacePin(requestContext context.Context, viewer Viewer, placeID uuid.UUID, input PinInput) (Place, error) {
	if err := requireDm(viewer); err != nil {
		return Place{}, err
	}
	var fieldErrors []FieldError
	if !(input.X >= 0 && input.X <= 1) {
		fieldErrors = append(fieldErrors, FieldError{Field: "x", Message: "Use a fraction from 0 to 1."})
	}
	if !(input.Y >= 0 && input.Y <= 1) {
		fieldErrors = append(fieldErrors, FieldError{Field: "y", Message: "Use a fraction from 0 to 1."})
	}
	if len(fieldErrors) > 0 {
		return Place{}, Invalid("The pin needs fixing.", fieldErrors...)
	}
	return service.changePlacePin(requestContext, viewer, placeID, func(queries *store.Queries) error {
		if _, err := queries.GetMap(requestContext, store.GetMapParams{CampaignID: viewer.campaignID, ID: input.MapID}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Invalid("That map doesn't exist.", FieldError{Field: "mapId", Message: "That map doesn't exist."})
			}
			return err
		}
		mapID := input.MapID
		pinX, pinY := float32(input.X), float32(input.Y)
		_, err := queries.SetPlacePin(requestContext, store.SetPlacePinParams{CampaignID: viewer.campaignID, ID: placeID, MapID: &mapID, PinX: &pinX, PinY: &pinY})
		return placeGone(err)
	})
}

// ClearPlacePin takes a place off its map; the place keeps its record.
func (service *Service) ClearPlacePin(requestContext context.Context, viewer Viewer, placeID uuid.UUID) (Place, error) {
	if err := requireDm(viewer); err != nil {
		return Place{}, err
	}
	return service.changePlacePin(requestContext, viewer, placeID, func(queries *store.Queries) error {
		_, err := queries.ClearPlacePin(requestContext, store.ClearPlacePinParams{CampaignID: viewer.campaignID, ID: placeID})
		return placeGone(err)
	})
}

// changePlacePin writes a pin, reads the place back for the DM and publishes: map pages list pins.
func (service *Service) changePlacePin(requestContext context.Context, viewer Viewer, placeID uuid.UUID, write func(queries *store.Queries) error) (Place, error) {
	var place Place
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		// Serialized with DeleteMap, so a pin can't land on a map between its pins clearing and its delete.
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		if err := write(queries); err != nil {
			return err
		}
		var err error
		place, err = readPlace(requestContext, queries, viewer.campaignID, placeID, true)
		return err
	})
	if err != nil {
		return Place{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicPlaces, TopicMaps)
	return place, nil
}

// publishPlaceChange: NPC pages name their places too, and map pages list pins by place name.
func (service *Service) publishPlaceChange(campaignID uuid.UUID) {
	service.broker.Publish(campaignID, TopicPlaces, TopicNpcs, TopicMaps)
}

func readPlace(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, placeID uuid.UUID, dmProjection bool) (Place, error) {
	row, err := queries.GetPlaceView(requestContext, store.GetPlaceViewParams{CampaignID: campaignID, ID: placeID, DmProjection: dmProjection})
	if err != nil {
		return Place{}, placeGone(err)
	}
	npcs, err := queries.ListNpcs(requestContext, store.ListNpcsParams{
		CampaignID:   campaignID,
		DmProjection: dmProjection,
		PlaceID:      &placeID,
		ResultLimit:  NpcLimit,
	})
	if err != nil {
		return Place{}, err
	}
	place := toPlace(row)
	for _, npc := range npcs {
		place.NpcsHere = append(place.NpcsHere, toNpcReference(npc.Npc))
	}
	return place, nil
}

func placeGone(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return errPlaceNotFound
	}
	return err
}

// toPlace fills Dm unconditionally; the Transformer decides who keeps it. The pin's map name is already
// NULL for a map the projection can't see, so a hidden map's ID never leaves here.
func toPlace(row store.GetPlaceViewRow) Place {
	place := Place{
		ID:          row.Place.ID,
		Name:        row.Place.Name,
		Description: row.Place.Description,
		PlayerNotes: row.Place.PlayerNotes,
		NpcsHere:    []NpcReference{},
		Dm:          &PlaceDm{DmNotes: row.Place.DmNotes, Revealed: row.Place.Revealed},
	}
	if row.Place.MapID != nil && row.Place.PinX != nil && row.Place.PinY != nil && row.PinMapName != nil {
		place.Pin = &PlacePin{MapID: *row.Place.MapID, MapName: *row.PinMapName, X: float64(*row.Place.PinX), Y: float64(*row.Place.PinY)}
	}
	return place
}

func cleanPlaceInput(input PlaceInput) (store.UpdatePlaceParams, error) {
	var fieldErrors []FieldError
	name, fieldError := cleanText("name", input.Name, placeNameMaxLength)
	switch {
	case fieldError != nil:
		fieldErrors = append(fieldErrors, *fieldError)
	case name == "":
		fieldErrors = append(fieldErrors, FieldError{Field: "name", Message: "Give the place a name."})
	}
	description, fieldError := cleanMultilineText("description", input.Description, npcProseMaxLength)
	if fieldError != nil {
		fieldErrors = append(fieldErrors, *fieldError)
	}
	playerNotes, fieldError := cleanMultilineText("playerNotes", input.PlayerNotes, npcProseMaxLength)
	if fieldError != nil {
		fieldErrors = append(fieldErrors, *fieldError)
	}
	dmNotes, fieldError := cleanMultilineText("dmNotes", input.DmNotes, npcProseMaxLength)
	if fieldError != nil {
		fieldErrors = append(fieldErrors, *fieldError)
	}
	if len(fieldErrors) > 0 {
		return store.UpdatePlaceParams{}, Invalid("The place needs fixing.", fieldErrors...)
	}
	return store.UpdatePlaceParams{
		Name:        name,
		Description: description,
		PlayerNotes: playerNotes,
		DmNotes:     dmNotes,
	}, nil
}
