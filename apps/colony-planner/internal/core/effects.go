package core

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/game"
	"github.com/BBruington/colony-planner/internal/store"
)

// cleanEffects checks Effects from a request body (a quest's reward or failure cost, an action's
// effects, a grant's tweak) against this campaign's own resources (Invariant 11): every key names an
// existing, unarchived resource, each delta fits a stored amount and colonists are 0 or more. Zero
// deltas are dropped, so a stored Effects names only what changes.
func cleanEffects(field string, effects Effects, resources map[string]store.Resource) (Effects, []FieldError) {
	cleaned := Effects{Resources: map[string]int64{}, Colonists: effects.Colonists}
	var fieldErrors []FieldError
	for _, key := range slices.Sorted(maps.Keys(effects.Resources)) {
		delta := effects.Resources[key]
		resource, found := resources[key]
		switch {
		case !found:
			fieldErrors = append(fieldErrors, FieldError{Field: field, Message: `There is no resource "` + key + `".`})
		case resource.Archived:
			fieldErrors = append(fieldErrors, FieldError{Field: field, Message: resource.Label + " is archived."})
		case delta < game.MinStored || delta > game.MaxStored:
			fieldErrors = append(fieldErrors, FieldError{Field: field, Message: "That much " + resource.Label + " can't be stored."})
		case delta != 0:
			cleaned.Resources[key] = delta
		}
	}
	if effects.Colonists < 0 {
		fieldErrors = append(fieldErrors, FieldError{Field: field, Message: game.MessageNegativeColonist + "."})
	} else if effects.Colonists > game.MaxStored {
		fieldErrors = append(fieldErrors, FieldError{Field: field, Message: game.MessageTooManyColonists + "."})
	}
	return cleaned, fieldErrors
}

// campaignResources is this campaign's resources by key, for cleanEffects.
func campaignResources(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID) (map[string]store.Resource, error) {
	rows, err := queries.ListResources(requestContext, campaignID)
	if err != nil {
		return nil, err
	}
	resources := make(map[string]store.Resource, len(rows))
	for _, row := range rows {
		resources[row.Key] = row
	}
	return resources, nil
}

func encodeEffects(effects Effects) ([]byte, error) {
	if effects.Resources == nil {
		effects.Resources = map[string]int64{}
	}
	encoded, err := json.Marshal(effects)
	if err != nil {
		return nil, fmt.Errorf("core: encode effects: %w", err)
	}
	return encoded, nil
}

// decodeEffects reads stored Effects JSON; Resources is never nil, so the API never sends null.
func decodeEffects(encoded []byte) (Effects, error) {
	var effects Effects
	if len(encoded) > 0 {
		if err := json.Unmarshal(encoded, &effects); err != nil {
			return Effects{}, fmt.Errorf("core: decode effects: %w", err)
		}
	}
	if effects.Resources == nil {
		effects.Resources = map[string]int64{}
	}
	return effects, nil
}

// FavorChangePreview is a patron's line in a preview: "Lady Vex favor 1 → 3".
type FavorChangePreview struct {
	_          struct{}  `nullable:"true"`
	PatronID   uuid.UUID `json:"patronId" format:"uuid"`
	PatronName string    `json:"patronName"`
	Before     int64     `json:"before"`
	After      int64     `json:"after"`
}

// effectsPreview turns a rule's result into preview lines and their summary parts.
func effectsPreview(colony colonySnapshot, result game.EffectsResult) ([]ResourceChangePreview, []string) {
	resources := make([]ResourceChangePreview, 0, len(result.Changes))
	parts := make([]string, 0, len(result.Changes)+1)
	for _, change := range result.Changes {
		resources = append(resources, ResourceChangePreview{
			Key:       change.Key,
			Label:     colony.labels.of(change.Key),
			Icon:      colony.resources[change.Key].Icon,
			Before:    change.Before,
			After:     change.After,
			Requested: change.Requested,
			Clamped:   change.Clamped,
		})
		parts = append(parts, changeText(colony.labels.of(change.Key), change.Before, change.After, change.Requested, change.Clamped))
	}
	if result.Applied.Colonists > 0 {
		parts = append(parts, colonistsText(result.Applied.Colonists))
	}
	return resources, parts
}

func favorLine(patronName string, delta int64) string {
	return patronName + " favor " + formatDelta(delta)
}

func favorChangeText(patronName string, before int64, after int64) string {
	return patronName + " favor " + formatNumber(before) + " → " + formatNumber(after)
}

// writeLooseEntry writes an entry that belongs to no colony mutation (QUEST_POSTED), so it has no group
// and undo never touches it.
func writeLooseEntry(requestContext context.Context, queries *store.Queries, viewer Viewer, entry plannedEntry) error {
	_, err := queries.CreateTimelineEvent(requestContext, store.CreateTimelineEventParams{
		CampaignID: viewer.campaignID,
		Day:        int32(entry.day),
		Kind:       string(entry.kind),
		Title:      entry.title,
		Body:       entry.body,
		Visibility: entry.visibility,
		Deltas:     []byte("{}"),
		Data:       []byte("{}"),
		CreatedBy:  memberReference(viewer),
		QuestID:    entry.questID,
		PatronID:   entry.patronID,
	})
	return err
}
