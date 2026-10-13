package core

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/BBruington/colony-planner/internal/store"
)

// Npc is one NPC as a projection sees it (spec §4.3, §5.9). Players get only revealed NPCs, under
// displayName; the true name, the known name, the revealed switch and dm_notes live in Dm (Open
// Question 8). Every other field stays visible once the NPC is revealed. Without a portrait, the avatar
// is displayName's initials.
type Npc struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	DisplayName string    `json:"displayName" doc:"The known name when the DM set one, else the true name: the only name players get"`
	// PortraitImageID is served at /api/campaigns/{campaignId}/images/{imageId}.
	PortraitImageID *uuid.UUID  `json:"portraitImageId" format:"uuid" nullable:"true" doc:"The portrait image; null shows initials"`
	Aliases         []string    `json:"aliases"`
	Labels          []string    `json:"labels"`
	Race            string      `json:"race"`
	CharacterClass  string      `json:"characterClass"`
	Level           *int64      `json:"level" doc:"null when unknown"`
	Age             string      `json:"age"`
	Appearance      string      `json:"appearance"`
	Personality     string      `json:"personality"`
	Backstory       string      `json:"backstory"`
	PlayerNotes     string      `json:"playerNotes"`
	Status          NpcStatus   `json:"status"`
	Health          NpcHealth   `json:"health"`
	Relation        NpcRelation `json:"relation"`
	// CurrentPlace and OriginPlace are null when unset or when the place is hidden from this viewer.
	CurrentPlace *PlaceReference `json:"currentPlace" doc:"null when unset or hidden from this viewer"`
	OriginPlace  *PlaceReference `json:"originPlace" doc:"null when unset or hidden from this viewer"`
	// ColonyRole is set when the NPC is a named colonist (spec §5.5).
	ColonyRole *ColonyRoleReference `json:"colonyRole" doc:"null when not a colonist"`
	// FirstSession is where the party first met them (Open Question 10). Sessions have no secrets, so
	// players see it like the NPC's other fields.
	FirstSession *SessionReference `json:"firstSession" doc:"The session where the party first met them; null when unset"`
	Dm           *NpcDm            `json:"dm,omitempty"`
}

// NpcDm is what only the DM sees of an NPC.
type NpcDm struct {
	Name      string `json:"name" doc:"The true name"`
	KnownName string `json:"knownName" doc:"What the party calls them; blank when they know the true name"`
	Revealed  bool   `json:"revealed"`
	DmNotes   string `json:"dmNotes"`
}

// PlaceReference names a place beside another record.
type PlaceReference struct {
	// The schema is nullable: huma can't mark one object field nullable, and every use may be null.
	_    struct{}  `nullable:"true"`
	ID   uuid.UUID `json:"id" format:"uuid"`
	Name string    `json:"name"`
}

// ColonyRoleReference names a named colonist's role.
type ColonyRoleReference struct {
	_     struct{}   `nullable:"true"`
	Key   string     `json:"key"`
	Label string     `json:"label"`
	Icon  ColonyIcon `json:"icon"`
}

// NpcReference names an NPC in a list that belongs to something else (a role's named colonists, the
// NPCs at a place). Players get only revealed NPCs here too.
type NpcReference struct {
	ID          uuid.UUID       `json:"id" format:"uuid"`
	DisplayName string          `json:"displayName"`
	Dm          *NpcReferenceDm `json:"dm,omitempty"`
}

type NpcReferenceDm struct {
	Name     string `json:"name" doc:"The true name"`
	Revealed bool   `json:"revealed"`
}

// NpcList is the roster.
type NpcList struct {
	Npcs []Npc `json:"npcs"`
}

// NpcInput replaces an NPC's record (PUT semantics). The colony role isn't here: joining and leaving
// the colony are colony mutations (JoinColony, ChangeColonyRole, LeaveColony). Nor is visibility: only
// SetNpcRevealed changes it, so an edit form opened before the DM hid the NPC can't reveal them again.
type NpcInput struct {
	Name           string      `json:"name" doc:"The true name, 1 to 80 characters"`
	KnownName      string      `json:"knownName" doc:"What the party calls them, at most 80 characters; blank when they know the true name"`
	Aliases        []string    `json:"aliases" doc:"At most 20, each at most 80 characters"`
	Labels         []string    `json:"labels" doc:"At most 20, each at most 40 characters"`
	Race           string      `json:"race" doc:"At most 80 characters"`
	CharacterClass string      `json:"characterClass" doc:"At most 80 characters"`
	Level          *int64      `json:"level" doc:"0 to 100, or null when unknown"`
	Age            string      `json:"age" doc:"Free text, at most 80 characters"`
	Appearance     string      `json:"appearance" doc:"At most 4000 characters"`
	Personality    string      `json:"personality" doc:"At most 4000 characters"`
	Backstory      string      `json:"backstory" doc:"At most 4000 characters"`
	PlayerNotes    string      `json:"playerNotes" doc:"At most 4000 characters"`
	DmNotes        string      `json:"dmNotes" doc:"At most 4000 characters"`
	Status         NpcStatus   `json:"status" doc:"Blank means UNKNOWN"`
	Health         NpcHealth   `json:"health" doc:"Blank means HEALTHY"`
	Relation       NpcRelation `json:"relation" doc:"Blank means UNKNOWN"`
	CurrentPlaceID *uuid.UUID  `json:"currentPlaceId" format:"uuid" nullable:"true" doc:"null for none"`
	OriginPlaceID  *uuid.UUID  `json:"originPlaceId" format:"uuid" nullable:"true" doc:"null for none"`
}

// NpcPortraitInput sets an NPC's portrait to one of the campaign's images, or clears it with null.
type NpcPortraitInput struct {
	ImageID *uuid.UUID `json:"imageId" format:"uuid" nullable:"true" doc:"An image from POST .../images; null clears the portrait"`
}

// NewNpcInput adds an NPC: its record and whether players start out seeing them.
type NewNpcInput struct {
	NpcInput
	Revealed bool `json:"revealed" doc:"Visible to players; the create form starts it off"`
}

// NpcFilter narrows the roster; every zero value is off. Players search display names and aliases
// over revealed NPCs; the DM also searches true and known names, and may filter by Revealed.
type NpcFilter struct {
	Search        string
	Status        NpcStatus
	Health        NpcHealth
	Relation      NpcRelation
	Label         string
	RoleKey       string
	PlaceID       *uuid.UUID
	ColonistsOnly bool
	// Revealed is the DM's Hidden filter; players never see hidden NPCs, so it is ignored for them.
	Revealed *bool
}

const (
	npcNameMaxLength   = 80
	npcShortMaxLength  = 80
	npcLabelMaxLength  = 40
	npcListMaxItems    = 20
	npcProseMaxLength  = 4000
	npcLevelMax        = 100
	npcSearchMaxLength = 80
	// BotNpcSuggestionLimit is Discord's cap on autocomplete choices.
	BotNpcSuggestionLimit = 25
)

var errNpcNotFound = NotFound("That NPC doesn't exist.")

// ListNpcs is the roster (spec §6.2 /npcs) as the viewer's projection sees it.
func (service *Service) ListNpcs(requestContext context.Context, viewer Viewer, filter NpcFilter) (NpcList, error) {
	if err := requireActive(viewer); err != nil {
		return NpcList{}, err
	}
	parameters, err := npcFilterParameters(viewer, filter)
	if err != nil {
		return NpcList{}, err
	}
	rows, err := service.queries.ListNpcs(requestContext, parameters)
	if err != nil {
		return NpcList{}, err
	}
	npcs := make([]Npc, 0, len(rows))
	for _, row := range rows {
		npcs = append(npcs, npcFromView(store.GetNpcViewRow(row)))
	}
	return NpcList{Npcs: npcs}, nil
}

// GetNpc is one NPC's page. A hidden NPC is not found for players, exactly like a missing one.
func (service *Service) GetNpc(requestContext context.Context, viewer Viewer, npcID uuid.UUID) (Npc, error) {
	if err := requireActive(viewer); err != nil {
		return Npc{}, err
	}
	return readNpc(requestContext, service.queries, viewer.campaignID, npcID, viewer.SeesDmFields())
}

// SuggestRevealedNpcs is Beny Bot's /npc view autocomplete: revealed NPCs whose display name starts
// with prefix (ignoring case), in roster order, at most limit (1 to BotNpcSuggestionLimit). Replies
// come from the player projection whoever asks (Open Question 11), so Dm is always nil and a hidden
// NPC's true name finds nothing.
func (service *Service) SuggestRevealedNpcs(requestContext context.Context, viewer Viewer, prefix string, limit int) ([]Npc, error) {
	if err := requireActive(viewer); err != nil {
		return nil, err
	}
	limit = min(max(limit, 1), BotNpcSuggestionLimit)
	pattern := escapeLike(strings.ToLower(strings.TrimSpace(prefix))) + "%"
	return service.revealedNpcsByDisplayName(requestContext, viewer, pattern, limit)
}

// FindRevealedNpc is Beny Bot's /npc view: the revealed NPC whose display name is displayName
// (ignoring case and surrounding spaces), from the player projection with Dm nil. When two share it,
// the first in roster order wins. Not found is a NotFound error saying so.
func (service *Service) FindRevealedNpc(requestContext context.Context, viewer Viewer, displayName string) (Npc, error) {
	if err := requireActive(viewer); err != nil {
		return Npc{}, err
	}
	pattern := escapeLike(strings.ToLower(strings.TrimSpace(displayName)))
	npcs, err := service.revealedNpcsByDisplayName(requestContext, viewer, pattern, 1)
	if err != nil {
		return Npc{}, err
	}
	if len(npcs) == 0 {
		return Npc{}, NotFound("No NPC the party knows goes by that name.")
	}
	return npcs[0], nil
}

func (service *Service) revealedNpcsByDisplayName(requestContext context.Context, viewer Viewer, pattern string, limit int) ([]Npc, error) {
	rows, err := service.queries.ListNpcs(requestContext, store.ListNpcsParams{
		CampaignID:         viewer.campaignID,
		DmProjection:       false,
		DisplayNamePattern: &pattern,
		ResultLimit:        int32(limit),
	})
	if err != nil {
		return nil, err
	}
	npcs := make([]Npc, 0, len(rows))
	for _, row := range rows {
		npc := npcFromView(store.GetNpcViewRow(row))
		npc.Dm = nil
		npcs = append(npcs, npc)
	}
	return npcs, nil
}

// CreateNpc adds an NPC. It is not a colony mutation.
func (service *Service) CreateNpc(requestContext context.Context, viewer Viewer, input NewNpcInput) (Npc, error) {
	if err := requireDm(viewer); err != nil {
		return Npc{}, err
	}
	var npc Npc
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		// Locked first, so concurrent creates can't each pass the count and go over the limit.
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		count, err := queries.CountNpcs(requestContext, viewer.campaignID)
		if err != nil {
			return err
		}
		if count >= NpcLimit {
			return Conflict("This campaign has " + formatNumber(NpcLimit) + " NPCs, the most it can hold. Delete one first.")
		}
		cleaned, err := cleanNpcInput(requestContext, queries, viewer.campaignID, input.NpcInput)
		if err != nil {
			return err
		}
		row, err := queries.CreateNpc(requestContext, store.CreateNpcParams{
			CampaignID:     cleaned.CampaignID,
			Name:           cleaned.Name,
			KnownName:      cleaned.KnownName,
			Revealed:       input.Revealed,
			Aliases:        cleaned.Aliases,
			Labels:         cleaned.Labels,
			Race:           cleaned.Race,
			CharacterClass: cleaned.CharacterClass,
			Level:          cleaned.Level,
			Age:            cleaned.Age,
			Appearance:     cleaned.Appearance,
			Personality:    cleaned.Personality,
			Backstory:      cleaned.Backstory,
			PlayerNotes:    cleaned.PlayerNotes,
			DmNotes:        cleaned.DmNotes,
			Status:         cleaned.Status,
			Health:         cleaned.Health,
			Relation:       cleaned.Relation,
			CurrentPlaceID: cleaned.CurrentPlaceID,
			OriginPlaceID:  cleaned.OriginPlaceID,
		})
		if err != nil {
			return err
		}
		npc, err = readNpc(requestContext, queries, viewer.campaignID, row.ID, true)
		return err
	})
	if err != nil {
		return Npc{}, err
	}
	service.publishNpcChange(viewer.campaignID)
	return npc, nil
}

// UpdateNpc replaces an NPC's record, leaving their visibility as it is. Renaming a named colonist
// changes what /people and the board show players, so it publishes those too; counts never change here.
func (service *Service) UpdateNpc(requestContext context.Context, viewer Viewer, npcID uuid.UUID, input NpcInput) (Npc, error) {
	if err := requireDm(viewer); err != nil {
		return Npc{}, err
	}
	var npc Npc
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		cleaned, err := cleanNpcInput(requestContext, queries, viewer.campaignID, input)
		if err != nil {
			return err
		}
		cleaned.ID = npcID
		if _, err := queries.UpdateNpc(requestContext, cleaned); err != nil {
			return npcGone(err)
		}
		if err := hidePublicEntriesNamingUnknownNames(requestContext, queries, viewer.campaignID, npcID); err != nil {
			return err
		}
		npc, err = readNpc(requestContext, queries, viewer.campaignID, npcID, true)
		return err
	})
	if err != nil {
		return Npc{}, err
	}
	service.publishNpcChange(viewer.campaignID)
	return npc, nil
}

// SetNpcRevealed is the DM's Visible to players switch.
func (service *Service) SetNpcRevealed(requestContext context.Context, viewer Viewer, npcID uuid.UUID, revealed bool) (Npc, error) {
	if err := requireDm(viewer); err != nil {
		return Npc{}, err
	}
	var npc Npc
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		if _, err := queries.SetNpcRevealed(requestContext, store.SetNpcRevealedParams{CampaignID: viewer.campaignID, ID: npcID, Revealed: revealed}); err != nil {
			return npcGone(err)
		}
		var err error
		npc, err = readNpc(requestContext, queries, viewer.campaignID, npcID, true)
		return err
	})
	if err != nil {
		return Npc{}, err
	}
	service.publishNpcChange(viewer.campaignID)
	return npc, nil
}

// SetNpcPortrait sets or clears an NPC's portrait (DM only). Players see it only while the NPC is
// revealed; the replaced image is deleted once nothing else uses it.
func (service *Service) SetNpcPortrait(requestContext context.Context, viewer Viewer, npcID uuid.UUID, input NpcPortraitInput) (Npc, error) {
	if err := requireDm(viewer); err != nil {
		return Npc{}, err
	}
	var npc Npc
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		// Locked first, so a concurrent upload can't count the old image as in use and then lose it.
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		var portraitImageID *uuid.UUID
		if input.ImageID != nil {
			image, err := loadImage(requestContext, queries, viewer.campaignID, *input.ImageID, "imageId")
			if err != nil {
				return err
			}
			portraitImageID = &image.ID
		}
		previous, err := queries.SetNpcPortrait(requestContext, store.SetNpcPortraitParams{CampaignID: viewer.campaignID, ID: npcID, PortraitImageID: portraitImageID})
		if err != nil {
			return npcGone(err)
		}
		if err := releaseImage(requestContext, queries, viewer.campaignID, previous); err != nil {
			return err
		}
		npc, err = readNpc(requestContext, queries, viewer.campaignID, npcID, true)
		return err
	})
	if err != nil {
		return Npc{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicNpcs)
	return npc, nil
}

// DeleteNpc deletes an NPC who isn't a colonist. A colonist must leave the colony first, because
// leaving is a colony mutation with its own entry and undo, so the colonist total never changes
// silently (spec §5.5).
func (service *Service) DeleteNpc(requestContext context.Context, viewer Viewer, npcID uuid.UUID) error {
	if err := requireDm(viewer); err != nil {
		return err
	}
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		// Locked first, so no join can make them a colonist between the check and the delete.
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		row, err := queries.GetNpc(requestContext, store.GetNpcParams{CampaignID: viewer.campaignID, ID: npcID})
		if err != nil {
			return npcGone(err)
		}
		if row.ColonyRoleKey != nil {
			return Conflict(row.DisplayName + " is a colonist. Remove them from the colony first.")
		}
		// The foreign key clears npc_id, which would show players a hidden NPC's entries again.
		if err := queries.PrepareNpcTimelineEventsForDelete(requestContext, store.PrepareNpcTimelineEventsForDeleteParams{
			CampaignID:  viewer.campaignID,
			NpcID:       &npcID,
			NpcRevealed: row.Revealed,
			DisplayName: row.DisplayName,
		}); err != nil {
			return err
		}
		if _, err := queries.DeleteNpc(requestContext, store.DeleteNpcParams{CampaignID: viewer.campaignID, ID: npcID}); err != nil {
			return err
		}
		return releaseImage(requestContext, queries, viewer.campaignID, row.PortraitImageID)
	})
	if err != nil {
		return err
	}
	service.publishNpcChange(viewer.campaignID)
	return nil
}

// hidePublicEntriesNamingUnknownNames moves an NPC's public entries to DM once a new name makes their
// text name the NPC by a name the party doesn't know, such as the true name after a known name is set.
func hidePublicEntriesNamingUnknownNames(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, npcID uuid.UUID) error {
	entries, err := queries.ListPublicNpcTimelineEvents(requestContext, store.ListPublicNpcTimelineEventsParams{CampaignID: campaignID, NpcID: &npcID})
	if err != nil {
		return err
	}
	for _, entry := range entries {
		refusal, err := npcEntryRefusal(requestContext, queries, campaignID, npcEntry{npcID: entry.NpcID, npcName: entry.NpcName, title: entry.Title, body: entry.Body}, false)
		if err != nil {
			return err
		}
		if refusal == "" {
			continue
		}
		if err := queries.HideTimelineEvent(requestContext, store.HideTimelineEventParams{CampaignID: campaignID, ID: entry.ID}); err != nil {
			return err
		}
	}
	return nil
}

// publishNpcChange: NPC pages, the places that list them, /people and the board, which list and
// count revealed named colonists, and the timeline, whose entries about an NPC follow their visibility.
func (service *Service) publishNpcChange(campaignID uuid.UUID) {
	service.broker.Publish(campaignID, TopicNpcs, TopicPlaces, TopicPeople, TopicColony, TopicTimeline)
}

func readNpc(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, npcID uuid.UUID, dmProjection bool) (Npc, error) {
	row, err := queries.GetNpcView(requestContext, store.GetNpcViewParams{CampaignID: campaignID, ID: npcID, DmProjection: dmProjection})
	if err != nil {
		return Npc{}, npcGone(err)
	}
	return npcFromView(row), nil
}

func npcGone(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return errNpcNotFound
	}
	return err
}

// npcFromView fills Dm unconditionally; the Transformer decides who keeps it. The place names are
// already NULL for places the projection can't see, so a hidden place's ID never leaves here.
func npcFromView(row store.GetNpcViewRow) Npc {
	npc := Npc{
		ID:              row.Npc.ID,
		DisplayName:     row.Npc.DisplayName,
		PortraitImageID: row.Npc.PortraitImageID,
		Aliases:         nonNilStrings(row.Npc.Aliases),
		Labels:          nonNilStrings(row.Npc.Labels),
		Race:            row.Npc.Race,
		CharacterClass:  row.Npc.CharacterClass,
		Level:           int64Pointer(row.Npc.Level),
		Age:             row.Npc.Age,
		Appearance:      row.Npc.Appearance,
		Personality:     row.Npc.Personality,
		Backstory:       row.Npc.Backstory,
		PlayerNotes:     row.Npc.PlayerNotes,
		Status:          row.Npc.Status,
		Health:          row.Npc.Health,
		Relation:        row.Npc.Relation,
		Dm: &NpcDm{
			Name:      row.Npc.Name,
			KnownName: row.Npc.KnownName,
			Revealed:  row.Npc.Revealed,
			DmNotes:   row.Npc.DmNotes,
		},
	}
	if row.Npc.CurrentPlaceID != nil && row.CurrentPlaceName != nil {
		npc.CurrentPlace = &PlaceReference{ID: *row.Npc.CurrentPlaceID, Name: *row.CurrentPlaceName}
	}
	if row.Npc.OriginPlaceID != nil && row.OriginPlaceName != nil {
		npc.OriginPlace = &PlaceReference{ID: *row.Npc.OriginPlaceID, Name: *row.OriginPlaceName}
	}
	if row.Npc.ColonyRoleKey != nil && row.ColonyRoleLabel != nil && row.ColonyRoleIcon != nil {
		npc.ColonyRole = &ColonyRoleReference{Key: *row.Npc.ColonyRoleKey, Label: *row.ColonyRoleLabel, Icon: *row.ColonyRoleIcon}
	}
	if row.Npc.FirstSessionID != nil && row.FirstSessionTitle != nil && row.FirstSessionStartsAt != nil && row.FirstSessionLocalDate.Valid {
		npc.FirstSession = &SessionReference{
			ID:        *row.Npc.FirstSessionID,
			Title:     *row.FirstSessionTitle,
			LocalDate: localDateText(row.FirstSessionLocalDate),
			StartsAt:  row.FirstSessionStartsAt.UTC(),
		}
	}
	return npc
}

func toNpcReference(row store.Npc) NpcReference {
	return NpcReference{ID: row.ID, DisplayName: row.DisplayName, Dm: &NpcReferenceDm{Name: row.Name, Revealed: row.Revealed}}
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// cleanedNpc is an NpcInput checked and trimmed, in the shape UpdateNpc saves.
type cleanedNpc = store.UpdateNpcParams

// cleanNpcInput checks every field and loads each place ID from the body with the campaign, so another
// campaign's place is "not found" (CLAUDE.md Invariant 11).
func cleanNpcInput(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, input NpcInput) (cleanedNpc, error) {
	var fieldErrors []FieldError
	collect := func(fieldError *FieldError) {
		if fieldError != nil {
			fieldErrors = append(fieldErrors, *fieldError)
		}
	}
	name, fieldError := cleanText("name", input.Name, npcNameMaxLength)
	collect(fieldError)
	if fieldError == nil && name == "" {
		collect(&FieldError{Field: "name", Message: "Give the NPC a name."})
	}
	knownName, fieldError := cleanText("knownName", input.KnownName, npcNameMaxLength)
	collect(fieldError)
	aliases, fieldError := cleanList("aliases", input.Aliases, npcShortMaxLength)
	collect(fieldError)
	labels, fieldError := cleanList("labels", input.Labels, npcLabelMaxLength)
	collect(fieldError)
	race, fieldError := cleanText("race", input.Race, npcShortMaxLength)
	collect(fieldError)
	characterClass, fieldError := cleanText("characterClass", input.CharacterClass, npcShortMaxLength)
	collect(fieldError)
	age, fieldError := cleanText("age", input.Age, npcShortMaxLength)
	collect(fieldError)
	appearance, fieldError := cleanMultilineText("appearance", input.Appearance, npcProseMaxLength)
	collect(fieldError)
	personality, fieldError := cleanMultilineText("personality", input.Personality, npcProseMaxLength)
	collect(fieldError)
	backstory, fieldError := cleanMultilineText("backstory", input.Backstory, npcProseMaxLength)
	collect(fieldError)
	playerNotes, fieldError := cleanMultilineText("playerNotes", input.PlayerNotes, npcProseMaxLength)
	collect(fieldError)
	dmNotes, fieldError := cleanMultilineText("dmNotes", input.DmNotes, npcProseMaxLength)
	collect(fieldError)
	var level *int32
	if input.Level != nil {
		if *input.Level < 0 || *input.Level > npcLevelMax {
			collect(&FieldError{Field: "level", Message: "Use a level from 0 to 100, or leave it blank."})
		} else {
			stored := int32(*input.Level)
			level = &stored
		}
	}
	status := cmp.Or(input.Status, NpcStatusUnknown)
	if !status.Valid() {
		collect(&FieldError{Field: "status", Message: "Pick one of the listed statuses."})
	}
	health := cmp.Or(input.Health, NpcHealthHealthy)
	if !health.Valid() {
		collect(&FieldError{Field: "health", Message: "Pick one of the listed health states."})
	}
	relation := cmp.Or(input.Relation, NpcRelationUnknown)
	if !relation.Valid() {
		collect(&FieldError{Field: "relation", Message: "Pick one of the listed relations."})
	}
	for _, reference := range []struct {
		field   string
		placeID *uuid.UUID
	}{{"currentPlaceId", input.CurrentPlaceID}, {"originPlaceId", input.OriginPlaceID}} {
		if reference.placeID == nil {
			continue
		}
		_, err := queries.GetPlace(requestContext, store.GetPlaceParams{CampaignID: campaignID, ID: *reference.placeID})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			collect(&FieldError{Field: reference.field, Message: "That place doesn't exist."})
		case err != nil:
			return cleanedNpc{}, err
		}
	}
	if len(fieldErrors) > 0 {
		return cleanedNpc{}, Invalid("The NPC needs fixing.", fieldErrors...)
	}
	return cleanedNpc{
		CampaignID:     campaignID,
		Name:           name,
		KnownName:      knownName,
		Aliases:        aliases,
		Labels:         labels,
		Race:           race,
		CharacterClass: characterClass,
		Level:          level,
		Age:            age,
		Appearance:     appearance,
		Personality:    personality,
		Backstory:      backstory,
		PlayerNotes:    playerNotes,
		DmNotes:        dmNotes,
		Status:         status,
		Health:         health,
		Relation:       relation,
		CurrentPlaceID: input.CurrentPlaceID,
		OriginPlaceID:  input.OriginPlaceID,
	}, nil
}

// cleanList trims each entry, drops blanks and repeats (ignoring case), and caps the list.
func cleanList(field string, values []string, maxLength int) ([]string, *FieldError) {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		entry, fieldError := cleanText(field, value, maxLength)
		if fieldError != nil {
			return nil, fieldError
		}
		if entry == "" || slices.ContainsFunc(cleaned, func(existing string) bool { return strings.EqualFold(existing, entry) }) {
			continue
		}
		cleaned = append(cleaned, entry)
	}
	if len(cleaned) > npcListMaxItems {
		return nil, &FieldError{Field: field, Message: "Use at most 20."}
	}
	return cleaned, nil
}

// npcFilterParameters turns a filter into the query's parameters for this viewer's projection.
func npcFilterParameters(viewer Viewer, filter NpcFilter) (store.ListNpcsParams, error) {
	parameters := store.ListNpcsParams{
		CampaignID:    viewer.campaignID,
		DmProjection:  viewer.SeesDmFields(),
		ColonistsOnly: filter.ColonistsOnly,
		PlaceID:       filter.PlaceID,
		ResultLimit:   NpcLimit,
	}
	var fieldErrors []FieldError
	search, fieldError := cleanText("search", filter.Search, npcSearchMaxLength)
	if fieldError != nil {
		fieldErrors = append(fieldErrors, *fieldError)
	}
	if search != "" {
		pattern := "%" + escapeLike(search) + "%"
		parameters.Search = &pattern
	}
	if filter.Status != "" {
		if !filter.Status.Valid() {
			fieldErrors = append(fieldErrors, FieldError{Field: "status", Message: "Pick one of the listed statuses."})
		}
		parameters.Status = &filter.Status
	}
	if filter.Health != "" {
		if !filter.Health.Valid() {
			fieldErrors = append(fieldErrors, FieldError{Field: "health", Message: "Pick one of the listed health states."})
		}
		parameters.Health = &filter.Health
	}
	if filter.Relation != "" {
		if !filter.Relation.Valid() {
			fieldErrors = append(fieldErrors, FieldError{Field: "relation", Message: "Pick one of the listed relations."})
		}
		parameters.Relation = &filter.Relation
	}
	if label := strings.TrimSpace(filter.Label); label != "" {
		parameters.Label = &label
	}
	if roleKey := strings.TrimSpace(filter.RoleKey); roleKey != "" {
		parameters.ColonyRoleKey = &roleKey
	}
	if viewer.SeesDmFields() {
		parameters.Revealed = filter.Revealed
	}
	if len(fieldErrors) > 0 {
		return store.ListNpcsParams{}, Invalid("The search needs fixing.", fieldErrors...)
	}
	return parameters, nil
}

// escapeLike makes text match itself inside a LIKE pattern (backslash is Postgres's default escape).
func escapeLike(text string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(text)
}
