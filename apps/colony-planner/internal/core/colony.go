package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/BBruington/colony-planner/internal/game"
	"github.com/BBruington/colony-planner/internal/store"
)

// ColonyBoard is the Colony page (spec §6.2). Every active member reads it, and players see the exact
// numbers (open question 5, resolved). A per-player role hierarchy (say, only a treasurer sees gold)
// would move fields into a sub-struct like Dm; nothing here depends on everyone seeing everything.
type ColonyBoard struct {
	// CampaignName and ColonyName are here so every member's top bar can show them; settings is DM-only.
	CampaignName string `json:"campaignName"`
	ColonyName   string `json:"colonyName"`
	Day          int64  `json:"day"`
	// Version goes back as expectedVersion on every form that changes the colony (spec §5.1).
	Version   int64             `json:"version"`
	Shipments *ShipmentSchedule `json:"shipments" doc:"null when no shipment schedule is set"`
	Resources []ColonyResource  `json:"resources" doc:"Unarchived resources in board order"`
	// Colonists is never stored: game.Colonists sums every role's headcount, Unassigned included.
	Colonists  int64           `json:"colonists"`
	Unassigned int64           `json:"unassigned"`
	Roles      []RoleHeadcount `json:"roles" doc:"Unarchived roles in display order, Unassigned included"`
	Dm         *ColonyBoardDm  `json:"dm,omitempty"`
}

// ShipmentSchedule is the cadence (spec §5.3): "Next shipment: Day D (in X days)".
type ShipmentSchedule struct {
	// The schema is nullable: huma can't mark one object field nullable, and every use may be null.
	_            struct{} `nullable:"true"`
	IntervalDays int64    `json:"intervalDays"`
	NextDay      int64    `json:"nextDay"`
	DaysUntil    int64    `json:"daysUntil" doc:"nextDay minus day; always at least 1"`
}

type ColonyResource struct {
	Key       string     `json:"key"`
	Label     string     `json:"label"`
	Icon      ColonyIcon `json:"icon"`
	Amount    int64      `json:"amount"`
	MinAmount int64      `json:"minAmount"`
	MaxAmount *int64     `json:"maxAmount" doc:"null when unbounded"`
	// IsSystem marks a resource a rule reads (food, for upkeep).
	IsSystem bool `json:"isSystem"`
}

type RoleHeadcount struct {
	Key          string     `json:"key"`
	Label        string     `json:"label"`
	Icon         ColonyIcon `json:"icon"`
	UnnamedCount int64      `json:"unnamedCount"`
	// NamedCount is 0 until named NPC colonists arrive in M2.
	NamedCount int64 `json:"namedCount"`
	Total      int64 `json:"total"`
	// IsSystem marks UNASSIGNED, where new colonists land.
	IsSystem bool `json:"isSystem"`
}

type ColonyBoardDm struct {
	LastChange *LastChange `json:"lastChange" doc:"What Undo would rewind; null when there is nothing to undo"`
}

// LastChange is the undo slot as the DM's Undo button shows it (spec §5.1).
type LastChange struct {
	_ struct{} `nullable:"true"`
	// GroupID goes back in UndoInput, so a stale button can't undo a newer change.
	GroupID   uuid.UUID `json:"groupId" format:"uuid"`
	Label     string    `json:"label" doc:"What the change did, e.g. 3 days pass · Day 44"`
	Day       int64     `json:"day" doc:"The colony day after the change"`
	ChangedAt time.Time `json:"changedAt"`
}

// ColonyChange answers every colony mutation, undo included.
type ColonyChange struct {
	// GroupID is shared by the entries the change wrote; UndoInput takes it.
	GroupID uuid.UUID `json:"groupId" format:"uuid"`
	Label   string    `json:"label"`
	// Entries are what the change wrote, in write order; the DM's toast comes from them.
	Entries []TimelineEntry `json:"entries"`
	Board   ColonyBoard     `json:"board" doc:"The board after the change"`
}

// AdvancePreviewInput is GET /api/colony/advance-preview?days=N.
type AdvancePreviewInput struct {
	Days int64
}

// AdvanceInput advances the clock (spec §5.2). Days has no upper limit beyond the overflow guard.
type AdvanceInput struct {
	Days            int64 `json:"days" doc:"1 or more; no limit beyond what the database can store"`
	ExpectedVersion int64 `json:"expectedVersion"`
}

// AdvancePreview runs the same rule as Advance without writing (spec §5.2).
type AdvancePreview struct {
	Days int64 `json:"days"`
	// Version is the colony version the preview saw; Confirm sends it as expectedVersion.
	Version   int64 `json:"version"`
	FromDay   int64 `json:"fromDay"`
	ToDay     int64 `json:"toDay"`
	Colonists int64 `json:"colonists" doc:"The headcount upkeep used"`
	// Resources lists every resource the advance changes (food, in v1).
	Resources      []ResourceChangePreview `json:"resources"`
	ArrivalCount   int64                   `json:"arrivalCount"`
	Arrivals       []ArrivalRun            `json:"arrivals" doc:"Every arrival day in the span, as runs"`
	ShipmentsAfter *ShipmentSchedule       `json:"shipmentsAfter" doc:"null when no shipment schedule is set"`
	Summary        string                  `json:"summary" doc:"e.g. Day 12 → 17 · Food 140 → 80 · 1 shipment arrival (Day 14)"`
}

// ArrivalRun is the arrival days first, first+every, ... (count of them), ending on last.
type ArrivalRun struct {
	First int64 `json:"first"`
	Every int64 `json:"every"`
	Count int64 `json:"count"`
	Last  int64 `json:"last"`
}

// ResourceChangePreview is one resource's line in a preview, e.g. "Morale 98 → 100 (capped)".
type ResourceChangePreview struct {
	Key       string     `json:"key"`
	Label     string     `json:"label"`
	Icon      ColonyIcon `json:"icon"`
	Before    int64      `json:"before"`
	After     int64      `json:"after"`
	Requested int64      `json:"requested" doc:"The change asked for; after minus before is what applies"`
	Clamped   bool       `json:"clamped" doc:"A bound stopped the change (capped at the maximum or floored at the minimum)"`
}

// AdjustInput changes stock by hand (spec §5.4) or records a shipment's cargo (spec §5.3). Both use
// CLAMP: only the applied delta is logged.
type AdjustInput struct {
	ExpectedVersion int64        `json:"expectedVersion"`
	Source          AdjustSource `json:"source"`
	// ArrivalEntryID links cargo to its SHIPMENT_ARRIVED entry, folded ones included.
	ArrivalEntryID *uuid.UUID       `json:"arrivalEntryId" format:"uuid" nullable:"true" doc:"The arrival this cargo came on: required for SHIPMENT, null for ADJUSTMENT"`
	Deltas         map[string]int64 `json:"deltas" doc:"Resource key to change; {} for none"`
	SetTo          map[string]int64 `json:"setTo" doc:"Resource key to new amount, turned into a delta under the colony lock; {} for none. A key may not also be in deltas."`
	Colonists      int64            `json:"colonists" doc:"New colonists, 0 or more; they land in Unassigned"`
	Reason         string           `json:"reason" doc:"At most 200 characters after trimming; may be blank"`
	// Public logs the entry on the public timeline. The form defaults it off for ADJUSTMENT and on
	// for SHIPMENT (spec §5.8).
	Public bool `json:"public"`
}

// AdjustPreview shows an adjustment before Confirm, caps included.
type AdjustPreview struct {
	Version         int64                   `json:"version" doc:"The colony version the preview saw; send it as expectedVersion"`
	Resources       []ResourceChangePreview `json:"resources" doc:"Every resource the adjustment names, sorted by key"`
	ColonistsBefore int64                   `json:"colonistsBefore"`
	ColonistsAfter  int64                   `json:"colonistsAfter"`
	Summary         string                  `json:"summary" doc:"e.g. Food +30 · Morale 98 → 100 (capped) · 4 colonists (Unassigned)"`
}

// ShipmentScheduleInput sets the cadence (spec §5.3).
type ShipmentScheduleInput struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	IntervalDays    int64  `json:"intervalDays" doc:"1 or more"`
	NextDay         *int64 `json:"nextDay" doc:"After today; null keeps the current next arrival, or day + interval when there is none"`
}

type ClearShipmentScheduleInput struct {
	ExpectedVersion int64 `json:"expectedVersion"`
}

// CorrectDayInput sets the day directly: no upkeep, no arrivals (spec §5.3).
type CorrectDayInput struct {
	ExpectedVersion int64 `json:"expectedVersion"`
	Day             int64 `json:"day" doc:"0 or more"`
}

// UndoInput names the change the DM saw on the Undo button (LastChange.GroupID).
type UndoInput struct {
	GroupID uuid.UUID `json:"groupId" format:"uuid"`
}

// lastChangeRecord is colony.last_change, the undo slot (spec §5.1). Its shape is core's own; the
// colony mutations write it and undo reads it.
type lastChangeRecord struct {
	GroupID      uuid.UUID    `json:"groupId"`
	Label        string       `json:"label"`
	Day          int64        `json:"day"`
	VersionAfter int64        `json:"versionAfter"`
	ChangedAt    time.Time    `json:"changedAt"`
	Before       undoSnapshot `json:"before"`
}

// undoSnapshot holds every value a colony mutation may change, as it was before the change.
type undoSnapshot struct {
	Day                  int64            `json:"day"`
	ShipmentIntervalDays *int64           `json:"shipmentIntervalDays"`
	NextShipmentDay      *int64           `json:"nextShipmentDay"`
	ResourceAmounts      map[string]int64 `json:"resourceAmounts"`
	RoleUnnamedCounts    map[string]int64 `json:"roleUnnamedCounts"`
}

// GetColonyBoard is the board every active member reads.
func (service *Service) GetColonyBoard(requestContext context.Context, viewer Viewer) (ColonyBoard, error) {
	if err := requireActive(viewer); err != nil {
		return ColonyBoard{}, err
	}
	return service.readBoard(requestContext, service.queries)
}

// PreviewAdvance runs game.Advance without writing (spec §5.2) and returns the version it saw.
func (service *Service) PreviewAdvance(requestContext context.Context, viewer Viewer, input AdvancePreviewInput) (AdvancePreview, error) {
	if err := requireDm(viewer); err != nil {
		return AdvancePreview{}, err
	}
	colony, err := readColony(requestContext, service.queries, false)
	if err != nil {
		return AdvancePreview{}, err
	}
	result, err := game.Advance(colony.state, input.Days)
	if err != nil {
		return AdvancePreview{}, gameError(err, nil)
	}
	return advancePreview(colony, result), nil
}

// Advance moves the clock N days: upkeep, then shipments (spec §5.2), on the write path of §5.1.
func (service *Service) Advance(requestContext context.Context, viewer Viewer, input AdvanceInput) (ColonyChange, error) {
	return service.mutateColony(requestContext, viewer, input.ExpectedVersion, func(colony colonySnapshot, _ *store.Queries) (colonyOutcome, error) {
		result, err := game.Advance(colony.state, input.Days)
		if err != nil {
			return colonyOutcome{}, gameError(err, nil)
		}
		entries := make([]plannedEntry, 0, len(result.ArrivalEntries)+1)
		for _, arrival := range result.ArrivalEntries {
			title := "Shipment arrived · " + dayText(arrival.Day)
			if arrival.Folded {
				title = foldedArrivalTitle(arrival.Run)
			}
			entries = append(entries, plannedEntry{
				day:        arrival.Day,
				kind:       TimelineKindShipmentArrived,
				title:      title,
				visibility: TimelineVisibilityPublic,
				data:       arrivalData{First: arrival.Run.First, Every: arrival.Run.Every, Count: arrival.Run.Count},
			})
		}
		title := daysPassTitle(result.Days, result.After.Day)
		entries = append(entries, plannedEntry{
			day:        result.After.Day,
			kind:       TimelineKindDaysAdvanced,
			title:      title,
			body:       joinParts(appliedEffectsText(result.Applied, colony.order, colony.labels), arrivalsSentence(result.Arrivals, result.ArrivalCount)),
			visibility: TimelineVisibilityPublic,
			deltas:     result.Applied,
		})
		return colonyOutcome{after: result.After, label: title, entries: entries}, nil
	})
}

// PreviewAdjustStock shows what AdjustStock would apply, caps included, without writing.
func (service *Service) PreviewAdjustStock(requestContext context.Context, viewer Viewer, input AdjustInput) (AdjustPreview, error) {
	if err := requireDm(viewer); err != nil {
		return AdjustPreview{}, err
	}
	colony, err := readColony(requestContext, service.queries, false)
	if err != nil {
		return AdjustPreview{}, err
	}
	planned, err := planAdjustment(requestContext, service.queries, colony, input)
	if err != nil {
		return AdjustPreview{}, err
	}
	preview := AdjustPreview{
		Version:         colony.row.Version,
		Resources:       make([]ResourceChangePreview, 0, len(planned.result.Changes)),
		ColonistsBefore: game.Colonists(planned.result.Before.Roles),
		ColonistsAfter:  game.Colonists(planned.result.After.Roles),
	}
	parts := make([]string, 0, len(planned.result.Changes)+1)
	for _, change := range planned.result.Changes {
		preview.Resources = append(preview.Resources, ResourceChangePreview{
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
	if input.Colonists > 0 {
		parts = append(parts, colonistsText(input.Colonists))
	}
	preview.Summary = joinParts(parts...)
	if preview.Summary == "" {
		preview.Summary = "Nothing would change"
	}
	return preview, nil
}

// AdjustStock writes STOCK_ADJUSTED (Source ADJUSTMENT, DM-only unless Public) or CARGO_RECORDED
// linked to its arrival (Source SHIPMENT). Both clamp, and only the applied deltas are logged.
func (service *Service) AdjustStock(requestContext context.Context, viewer Viewer, input AdjustInput) (ColonyChange, error) {
	return service.mutateColony(requestContext, viewer, input.ExpectedVersion, func(colony colonySnapshot, queries *store.Queries) (colonyOutcome, error) {
		planned, err := planAdjustment(requestContext, queries, colony, input)
		if err != nil {
			return colonyOutcome{}, err
		}
		applied := planned.result.Applied
		if len(applied.Resources) == 0 && applied.Colonists == 0 {
			return colonyOutcome{}, Invalid("Nothing would change: every amount is already where that would put it.")
		}
		summary := appliedEffectsText(applied, colony.order, colony.labels)
		entry := plannedEntry{
			day:        colony.state.Day,
			kind:       TimelineKindStockAdjusted,
			title:      "Stock adjusted",
			body:       joinLines(summary, planned.reason),
			visibility: TimelineVisibilityDm,
			deltas:     applied,
		}
		if input.Public {
			entry.visibility = TimelineVisibilityPublic
		}
		if planned.arrival != nil {
			entry.kind = TimelineKindCargoRecorded
			entry.title = cargoTitle(*planned.arrival)
			entry.data = cargoData{ArrivalEntryID: *input.ArrivalEntryID}
		}
		return colonyOutcome{after: planned.result.After, label: entry.title + " · " + summary, entries: []plannedEntry{entry}}, nil
	})
}

// SetShipmentSchedule writes SHIPMENT_SCHEDULE_CHANGED, PUBLIC.
func (service *Service) SetShipmentSchedule(requestContext context.Context, viewer Viewer, input ShipmentScheduleInput) (ColonyChange, error) {
	return service.mutateColony(requestContext, viewer, input.ExpectedVersion, func(colony colonySnapshot, _ *store.Queries) (colonyOutcome, error) {
		result, err := game.SetSchedule(colony.state, game.ScheduleInput{IntervalDays: input.IntervalDays, NextDay: input.NextDay})
		if err != nil {
			return colonyOutcome{}, gameError(err, map[string]string{"nextShipmentDay": "nextDay"})
		}
		title := "Shipments " + scheduleText(result.After)
		return colonyOutcome{after: result.State, label: title, entries: []plannedEntry{{
			day:        colony.state.Day,
			kind:       TimelineKindShipmentScheduleChanged,
			title:      title,
			body:       "Before: " + scheduleText(result.Before),
			visibility: TimelineVisibilityPublic,
		}}}, nil
	})
}

// ClearShipmentSchedule hides the countdown; it writes SHIPMENT_SCHEDULE_CHANGED, PUBLIC.
func (service *Service) ClearShipmentSchedule(requestContext context.Context, viewer Viewer, input ClearShipmentScheduleInput) (ColonyChange, error) {
	return service.mutateColony(requestContext, viewer, input.ExpectedVersion, func(colony colonySnapshot, _ *store.Queries) (colonyOutcome, error) {
		if colony.state.Schedule == nil {
			return colonyOutcome{}, Conflict("There is no shipment schedule to clear.")
		}
		result := game.ClearSchedule(colony.state)
		title := "Shipments stopped"
		return colonyOutcome{after: result.State, label: title, entries: []plannedEntry{{
			day:        colony.state.Day,
			kind:       TimelineKindShipmentScheduleChanged,
			title:      title,
			body:       "Before: " + scheduleText(result.Before),
			visibility: TimelineVisibilityPublic,
		}}}, nil
	})
}

// CorrectDay writes DAY_CORRECTED with DM visibility, e.g. "Day 10 → 30 (3 shipment dates skipped)".
func (service *Service) CorrectDay(requestContext context.Context, viewer Viewer, input CorrectDayInput) (ColonyChange, error) {
	return service.mutateColony(requestContext, viewer, input.ExpectedVersion, func(colony colonySnapshot, _ *store.Queries) (colonyOutcome, error) {
		if input.Day == colony.state.Day {
			message := "The colony is already on " + dayText(input.Day) + "."
			return colonyOutcome{}, Invalid(message, FieldError{Field: "day", Message: message})
		}
		result, err := game.CorrectDay(colony.state, input.Day)
		if err != nil {
			return colonyOutcome{}, gameError(err, nil)
		}
		title := dayText(result.FromDay) + " → " + formatNumber(result.ToDay)
		if result.SkippedShipments > 0 {
			title += " (" + formatNumber(result.SkippedShipments) + " shipment " +
				plural(result.SkippedShipments, "date", "dates") + " skipped)"
		}
		body := ""
		if result.After != nil && (result.Before == nil || result.Before.NextDay != result.After.NextDay) {
			body = "Next shipment: " + dayText(result.After.NextDay)
		}
		return colonyOutcome{after: result.State, label: title, entries: []plannedEntry{{
			day:        result.ToDay,
			kind:       TimelineKindDayCorrected,
			title:      title,
			body:       body,
			visibility: TimelineVisibilityDm,
		}}}, nil
	})
}

// UndoLastChange restores the before rows, marks the group's entries undone and writes UNDO (spec
// §5.1). There is one level only: it clears the undo slot.
func (service *Service) UndoLastChange(requestContext context.Context, viewer Viewer, input UndoInput) (ColonyChange, error) {
	if err := requireDm(viewer); err != nil {
		return ColonyChange{}, err
	}
	var change ColonyChange
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		colony, err := readColony(requestContext, queries, true)
		if err != nil {
			return err
		}
		record, err := usableLastChange(colony.row)
		if err != nil {
			return err
		}
		if record == nil {
			return Conflict("There is nothing to undo.")
		}
		if record.GroupID != input.GroupID {
			return Conflict("A newer change replaced that one. Undo offers only the newest change.")
		}
		restored, err := restoreSnapshot(colony, record.Before)
		if err != nil {
			return err
		}
		if err := saveState(requestContext, queries, colony.state, restored); err != nil {
			return err
		}
		undone, err := queries.MarkTimelineGroupUndone(requestContext, record.GroupID)
		if err != nil {
			return err
		}
		// PUBLIC only if a player saw something being undone, and then named by the newest public
		// entry's own title, so a public UNDO never names what players were never shown (spec §5.8).
		entry := plannedEntry{
			day:        restored.Day,
			kind:       TimelineKindUndo,
			title:      "The DM rewound: " + record.Label,
			visibility: TimelineVisibilityDm,
		}
		var newestPublic *store.TimelineEvent
		for index := range undone {
			if undone[index].Visibility == TimelineVisibilityPublic && (newestPublic == nil || undone[index].Sequence > newestPublic.Sequence) {
				newestPublic = &undone[index]
			}
		}
		if newestPublic != nil {
			entry.visibility = TimelineVisibilityPublic
			entry.title = "The DM rewound: " + newestPublic.Title
		}
		if restored.Day != colony.state.Day {
			entry.body = "Back to " + dayText(restored.Day)
		}
		if _, err := queries.SaveColonyChange(requestContext, saveColonyParameters(restored, nil)); err != nil {
			return err
		}
		groupID := uuid.New()
		entries, err := writeEntries(requestContext, queries, groupID, viewer, []plannedEntry{entry})
		if err != nil {
			return err
		}
		board, err := service.readBoard(requestContext, queries)
		if err != nil {
			return err
		}
		change = ColonyChange{GroupID: groupID, Label: entry.title, Entries: entries, Board: board}
		return nil
	})
	if err != nil {
		return ColonyChange{}, err
	}
	service.broker.Publish(TopicColony, TopicTimeline, TopicResources)
	return change, nil
}

// colonySnapshot is the colony as one operation reads it: the row, the game.State built from it and
// what text needs to name resources.
type colonySnapshot struct {
	row       store.Colony
	state     game.State
	resources map[string]store.Resource
	// order is every resource key in board order, so text lists resources the way the board does.
	order  []string
	labels resourceLabels
}

// readColony loads the colony, locking its row first when a mutation will follow (one lock order,
// spec §5.1). Unlocked, it reads the row before the stock, so a preview's version is never newer than
// the numbers it shows: Confirm then fails safe with a 409.
func readColony(requestContext context.Context, queries *store.Queries, lock bool) (colonySnapshot, error) {
	var row store.Colony
	var err error
	if lock {
		row, err = queries.GetColonyForUpdate(requestContext)
	} else {
		row, err = queries.GetColony(requestContext)
	}
	if err != nil {
		return colonySnapshot{}, err
	}
	resources, err := queries.ListResources(requestContext)
	if err != nil {
		return colonySnapshot{}, err
	}
	roles, err := queries.ListRoles(requestContext)
	if err != nil {
		return colonySnapshot{}, err
	}
	snapshot := colonySnapshot{
		row: row,
		state: game.State{
			Day:       int64(row.Day),
			Resources: make(map[string]game.Resource, len(resources)),
			Roles:     make(map[string]game.Role, len(roles)),
		},
		resources: make(map[string]store.Resource, len(resources)),
		order:     make([]string, 0, len(resources)),
		labels:    make(resourceLabels, len(resources)),
	}
	if row.ShipmentIntervalDays != nil && row.NextShipmentDay != nil {
		snapshot.state.Schedule = &game.Schedule{IntervalDays: int64(*row.ShipmentIntervalDays), NextDay: int64(*row.NextShipmentDay)}
	}
	for _, resource := range resources {
		snapshot.state.Resources[resource.Key] = game.Resource{
			Label:     resource.Label,
			Amount:    int64(resource.Amount),
			MinAmount: int64(resource.MinAmount),
			MaxAmount: int64Pointer(resource.MaxAmount),
			Archived:  resource.Archived,
		}
		snapshot.resources[resource.Key] = resource
		snapshot.order = append(snapshot.order, resource.Key)
		snapshot.labels[resource.Key] = resource.Label
	}
	// NamedCount stays 0 until named NPC colonists arrive (M2).
	for _, role := range roles {
		snapshot.state.Roles[role.Key] = game.Role{UnnamedCount: int64(role.UnnamedCount), Archived: role.Archived}
	}
	return snapshot, nil
}

// plannedEntry is one timeline entry a colony mutation writes, its text already rendered.
type plannedEntry struct {
	day        int64
	kind       TimelineKind
	title      string
	body       string
	visibility TimelineVisibility
	deltas     Effects
	// data is the kind-specific JSON (arrivalData, cargoData); nil writes {}.
	data any
}

// colonyOutcome is what one colony mutation decided: the State to save, the undo label and the
// entries to write.
type colonyOutcome struct {
	after   game.State
	label   string
	entries []plannedEntry
}

// mutateColony is the write path every form-started colony mutation shares (spec §5.1): lock the
// colony, check the version, call the rule (decide), save, write the grouped entries, record the undo
// slot, bump the version, commit and then publish.
func (service *Service) mutateColony(requestContext context.Context, viewer Viewer, expectedVersion int64,
	decide func(colony colonySnapshot, queries *store.Queries) (colonyOutcome, error),
) (ColonyChange, error) {
	if err := requireDm(viewer); err != nil {
		return ColonyChange{}, err
	}
	var change ColonyChange
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		colony, err := readColony(requestContext, queries, true)
		if err != nil {
			return err
		}
		if colony.row.Version != expectedVersion {
			return Conflict("The colony changed (now " + dayText(int64(colony.row.Day)) + "). Review and retry.")
		}
		outcome, err := decide(colony, queries)
		if err != nil {
			return err
		}
		if err := saveState(requestContext, queries, colony.state, outcome.after); err != nil {
			return err
		}
		groupID := uuid.New()
		record, err := json.Marshal(lastChangeRecord{
			GroupID:      groupID,
			Label:        outcome.label,
			Day:          outcome.after.Day,
			VersionAfter: colony.row.Version + 1,
			ChangedAt:    time.Now().UTC(),
			Before:       snapshotBefore(colony.state, outcome.after),
		})
		if err != nil {
			return fmt.Errorf("core: encode colony.last_change: %w", err)
		}
		if _, err := queries.SaveColonyChange(requestContext, saveColonyParameters(outcome.after, record)); err != nil {
			return err
		}
		entries, err := writeEntries(requestContext, queries, groupID, viewer, outcome.entries)
		if err != nil {
			return err
		}
		board, err := service.readBoard(requestContext, queries)
		if err != nil {
			return err
		}
		change = ColonyChange{GroupID: groupID, Label: outcome.label, Entries: entries, Board: board}
		return nil
	})
	if err != nil {
		return ColonyChange{}, err
	}
	// The DM's /settings list shows amounts too, so resources goes with colony and timeline.
	service.broker.Publish(TopicColony, TopicTimeline, TopicResources)
	return change, nil
}

// snapshotBefore records the day, the schedule and every amount and headcount the change touches, as
// they were before it.
func snapshotBefore(before game.State, after game.State) undoSnapshot {
	snapshot := undoSnapshot{
		Day:               before.Day,
		ResourceAmounts:   map[string]int64{},
		RoleUnnamedCounts: map[string]int64{},
	}
	if before.Schedule != nil {
		intervalDays := before.Schedule.IntervalDays
		nextDay := before.Schedule.NextDay
		snapshot.ShipmentIntervalDays = &intervalDays
		snapshot.NextShipmentDay = &nextDay
	}
	for key, resource := range after.Resources {
		if previous := before.Resources[key]; previous.Amount != resource.Amount {
			snapshot.ResourceAmounts[key] = previous.Amount
		}
	}
	for key, role := range after.Roles {
		if previous := before.Roles[key]; previous.UnnamedCount != role.UnnamedCount {
			snapshot.RoleUnnamedCounts[key] = previous.UnnamedCount
		}
	}
	return snapshot
}

// restoreSnapshot is the State undo saves. A restore the database would refuse (a resource's bounds
// changed on /settings since, or a role was archived) is a conflict the DM resolves by hand.
func restoreSnapshot(colony colonySnapshot, before undoSnapshot) (game.State, error) {
	restored := colony.state.Clone()
	restored.Day = before.Day
	restored.Schedule = nil
	if before.ShipmentIntervalDays != nil && before.NextShipmentDay != nil {
		restored.Schedule = &game.Schedule{IntervalDays: *before.ShipmentIntervalDays, NextDay: *before.NextShipmentDay}
	}
	for _, key := range slices.Sorted(maps.Keys(before.ResourceAmounts)) {
		amount := before.ResourceAmounts[key]
		resource, found := restored.Resources[key]
		if !found {
			return game.State{}, Conflict("Undo can't restore " + key + ": that resource no longer exists.")
		}
		if amount < resource.MinAmount || (resource.MaxAmount != nil && amount > *resource.MaxAmount) {
			return game.State{}, Conflict("Undo would put " + resource.Label + " back at " + formatNumber(amount) +
				", outside the bounds it has had since. Adjust it instead.")
		}
		resource.Amount = amount
		restored.Resources[key] = resource
	}
	for _, key := range slices.Sorted(maps.Keys(before.RoleUnnamedCounts)) {
		count := before.RoleUnnamedCounts[key]
		role, found := restored.Roles[key]
		if !found || (role.Archived && count > 0) {
			return game.State{}, Conflict("Undo can't restore the " + key + " headcount: that role was archived since.")
		}
		role.UnnamedCount = count
		restored.Roles[key] = role
	}
	return restored, nil
}

// saveState writes every amount and headcount that differs between before and after.
func saveState(requestContext context.Context, queries *store.Queries, before game.State, after game.State) error {
	for _, key := range slices.Sorted(maps.Keys(after.Resources)) {
		amount := after.Resources[key].Amount
		if previous, found := before.Resources[key]; found && previous.Amount == amount {
			continue
		}
		if err := queries.SetResourceAmount(requestContext, store.SetResourceAmountParams{Key: key, Amount: int32(amount)}); err != nil {
			return err
		}
	}
	for _, key := range slices.Sorted(maps.Keys(after.Roles)) {
		count := after.Roles[key].UnnamedCount
		if previous, found := before.Roles[key]; found && previous.UnnamedCount == count {
			continue
		}
		if err := queries.SetRoleUnnamedCount(requestContext, store.SetRoleUnnamedCountParams{Key: key, UnnamedCount: int32(count)}); err != nil {
			return err
		}
	}
	return nil
}

func saveColonyParameters(state game.State, lastChange []byte) store.SaveColonyChangeParams {
	parameters := store.SaveColonyChangeParams{Day: int32(state.Day), LastChange: lastChange}
	if state.Schedule != nil {
		intervalDays := int32(state.Schedule.IntervalDays)
		nextDay := int32(state.Schedule.NextDay)
		parameters.ShipmentIntervalDays = &intervalDays
		parameters.NextShipmentDay = &nextDay
	}
	return parameters
}

// writeEntries inserts the entries in order, all sharing groupID.
func writeEntries(requestContext context.Context, queries *store.Queries, groupID uuid.UUID, viewer Viewer, planned []plannedEntry) ([]TimelineEntry, error) {
	entries := make([]TimelineEntry, 0, len(planned))
	for _, entry := range planned {
		deltas, err := json.Marshal(entry.deltas)
		if err != nil {
			return nil, fmt.Errorf("core: encode timeline deltas: %w", err)
		}
		data := []byte("{}")
		if entry.data != nil {
			data, err = json.Marshal(entry.data)
			if err != nil {
				return nil, fmt.Errorf("core: encode timeline data: %w", err)
			}
		}
		row, err := queries.CreateTimelineEvent(requestContext, store.CreateTimelineEventParams{
			GroupID:    &groupID,
			Day:        int32(entry.day),
			Kind:       string(entry.kind),
			Title:      entry.title,
			Body:       entry.body,
			Visibility: entry.visibility,
			Deltas:     deltas,
			Data:       data,
			CreatedBy:  memberReference(viewer),
		})
		if err != nil {
			return nil, err
		}
		converted, err := toTimelineEntry(row)
		if err != nil {
			return nil, err
		}
		entries = append(entries, converted)
	}
	return entries, nil
}

// memberReference is the viewer's member ID for created_by, or nil for a caller with no member row.
func memberReference(viewer Viewer) *uuid.UUID {
	if viewer.MemberID == uuid.Nil {
		return nil
	}
	memberID := viewer.MemberID
	return &memberID
}

// gameError maps a rule's refusal to core's typed errors, renaming fields to the contract's JSON
// names. Any other error means the stored state was impossible, which is a 500.
func gameError(err error, renames map[string]string) error {
	var invalidError *game.InvalidError
	if errors.As(err, &invalidError) {
		field := invalidError.Field
		if renamed, found := renames[field]; found {
			field = renamed
		}
		return Invalid(invalidError.Message+".", FieldError{Field: field, Message: invalidError.Message + "."})
	}
	var shortfallError *game.ShortfallError
	if errors.As(err, &shortfallError) {
		return Invalid(shortfallError.Error() + ".")
	}
	return err
}

// advancePreview turns a rule result into the preview sheet: "Day 12 → 17 · Food 140 → 80 · 1
// shipment arrival (Day 14)".
func advancePreview(colony colonySnapshot, result game.AdvanceResult) AdvancePreview {
	preview := AdvancePreview{
		Days:         result.Days,
		Version:      colony.row.Version,
		FromDay:      result.Before.Day,
		ToDay:        result.After.Day,
		Colonists:    result.Colonists,
		Resources:    []ResourceChangePreview{},
		ArrivalCount: result.ArrivalCount,
		Arrivals:     make([]ArrivalRun, 0, len(result.Arrivals)),
	}
	if result.After.Schedule != nil {
		preview.ShipmentsAfter = &ShipmentSchedule{
			IntervalDays: result.After.Schedule.IntervalDays,
			NextDay:      result.After.Schedule.NextDay,
			DaysUntil:    result.After.Schedule.NextDay - result.After.Day,
		}
	}
	for _, run := range result.Arrivals {
		preview.Arrivals = append(preview.Arrivals, toArrivalRun(run.First, run.Every, run.Count))
	}
	parts := []string{dayText(result.Before.Day) + " → " + formatNumber(result.After.Day)}
	for _, key := range colony.order {
		before := result.Before.Resources[key].Amount
		after := result.After.Resources[key].Amount
		requested := after - before
		if key == game.FoodResourceKey {
			requested = -upkeepNeeded(result.Colonists, result.Days)
		}
		if requested == 0 && after == before {
			continue
		}
		clamped := after-before != requested
		preview.Resources = append(preview.Resources, ResourceChangePreview{
			Key:       key,
			Label:     colony.labels.of(key),
			Icon:      colony.resources[key].Icon,
			Before:    before,
			After:     after,
			Requested: requested,
			Clamped:   clamped,
		})
		parts = append(parts, colony.labels.of(key)+" "+formatNumber(before)+" → "+formatNumber(after))
	}
	switch {
	case result.Before.Schedule == nil:
		parts = append(parts, "No shipment schedule")
	case result.ArrivalCount == 0:
		parts = append(parts, "0 shipment arrivals")
	default:
		parts = append(parts, formatNumber(result.ArrivalCount)+" shipment "+plural(result.ArrivalCount, "arrival", "arrivals")+
			" ("+arrivalDaysText(result.Arrivals)+")")
	}
	preview.Summary = joinParts(parts...)
	return preview
}

// upkeepNeeded is the food N days of upkeep ask for, saturating rather than overflowing for a huge N.
func upkeepNeeded(colonists int64, days int64) int64 {
	perDay := colonists * game.FoodPerColonistPerDay
	if perDay <= 0 || days <= 0 {
		return 0
	}
	if days > math.MaxInt64/perDay {
		return math.MaxInt64
	}
	return perDay * days
}

// changeText is one preview line: "Food +30", or "Morale 98 → 100 (capped)" when a bound cut it.
func changeText(label string, before int64, after int64, requested int64, clamped bool) string {
	if !clamped {
		return label + " " + formatDelta(after-before)
	}
	bound := "capped"
	if requested < 0 {
		bound = "floored"
	}
	return label + " " + formatNumber(before) + " → " + formatNumber(after) + " (" + bound + ")"
}

// plannedAdjustment is an AdjustInput checked and applied to the colony, not yet saved.
type plannedAdjustment struct {
	result game.EffectsResult
	reason string
	// arrival is the SHIPMENT_ARRIVED entry the cargo came on; nil for an adjustment.
	arrival *store.TimelineEvent
}

const adjustReasonMaxLength = 200

// planAdjustment checks an adjustment or cargo and applies it in CLAMP mode (spec §5.4). "Set to"
// values become deltas against the colony as read here, which is under the lock for a mutation.
func planAdjustment(requestContext context.Context, queries *store.Queries, colony colonySnapshot, input AdjustInput) (plannedAdjustment, error) {
	var fieldErrors []FieldError
	var arrival *store.TimelineEvent
	switch input.Source {
	case AdjustSourceAdjustment:
		if input.ArrivalEntryID != nil {
			fieldErrors = append(fieldErrors, FieldError{Field: "arrivalEntryId", Message: "Only cargo names a shipment arrival."})
		}
	case AdjustSourceShipment:
		found, fieldError, err := arrivalFor(requestContext, queries, input.ArrivalEntryID)
		if err != nil {
			return plannedAdjustment{}, err
		}
		if fieldError != nil {
			fieldErrors = append(fieldErrors, *fieldError)
		}
		arrival = found
	default:
		fieldErrors = append(fieldErrors, FieldError{Field: "source", Message: "Choose ADJUSTMENT or SHIPMENT."})
	}
	reason, fieldError := cleanText("reason", input.Reason, adjustReasonMaxLength)
	if fieldError != nil {
		fieldErrors = append(fieldErrors, *fieldError)
	}
	fieldErrors = append(fieldErrors, adjustableKeys("deltas", input.Deltas, colony)...)
	fieldErrors = append(fieldErrors, adjustableKeys("setTo", input.SetTo, colony)...)
	for _, key := range slices.Sorted(maps.Keys(input.SetTo)) {
		if _, both := input.Deltas[key]; both {
			fieldErrors = append(fieldErrors, FieldError{Field: "setTo", Message: colony.labels.of(key) + " has both a change and a set-to value. Use one."})
		}
	}
	if input.Colonists < 0 {
		fieldErrors = append(fieldErrors, FieldError{Field: "colonists", Message: game.MessageNegativeColonist + "."})
	}
	if len(fieldErrors) > 0 {
		return plannedAdjustment{}, Invalid("The adjustment needs fixing.", fieldErrors...)
	}
	effects, err := game.AdjustmentFromTargets(colony.state, input.SetTo)
	if err != nil {
		return plannedAdjustment{}, gameError(err, map[string]string{"resources": "setTo"})
	}
	for key, delta := range input.Deltas {
		effects.Resources[key] = delta
	}
	effects.Colonists = input.Colonists
	// Set-to deltas always fit (both ends are storable amounts), so a storage refusal names a delta.
	result, err := game.ApplyEffects(colony.state, effects, game.Clamp)
	if err != nil {
		return plannedAdjustment{}, gameError(err, map[string]string{"resources": "deltas"})
	}
	return plannedAdjustment{result: result, reason: reason, arrival: arrival}, nil
}

// adjustableKeys refuses resource keys that don't exist or are archived, naming the field they came in.
func adjustableKeys(field string, values map[string]int64, colony colonySnapshot) []FieldError {
	var fieldErrors []FieldError
	for _, key := range slices.Sorted(maps.Keys(values)) {
		resource, found := colony.resources[key]
		switch {
		case !found:
			fieldErrors = append(fieldErrors, FieldError{Field: field, Message: `There is no resource "` + key + `".`})
		case resource.Archived:
			fieldErrors = append(fieldErrors, FieldError{Field: field, Message: resource.Label + " is archived."})
		}
	}
	return fieldErrors
}

// arrivalFor finds the SHIPMENT_ARRIVED entry cargo names; it must exist and not be undone.
func arrivalFor(requestContext context.Context, queries *store.Queries, arrivalEntryID *uuid.UUID) (*store.TimelineEvent, *FieldError, error) {
	if arrivalEntryID == nil {
		return nil, &FieldError{Field: "arrivalEntryId", Message: "Pick the shipment arrival this cargo came on."}, nil
	}
	row, err := queries.GetTimelineEvent(requestContext, *arrivalEntryID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, &FieldError{Field: "arrivalEntryId", Message: "That shipment arrival doesn't exist."}, nil
	case err != nil:
		return nil, nil, err
	case TimelineKind(row.Kind) != TimelineKindShipmentArrived:
		return nil, &FieldError{Field: "arrivalEntryId", Message: "That entry isn't a shipment arrival."}, nil
	case row.UndoneAt != nil:
		return nil, &FieldError{Field: "arrivalEntryId", Message: "That shipment arrival was undone."}, nil
	}
	return &row, nil, nil
}

// cargoTitle names the arrival cargo came on: "Cargo from the Day 7 shipment", or for a folded entry
// "Cargo from the shipments of Days 42 – 1,000,034".
func cargoTitle(arrival store.TimelineEvent) string {
	var data arrivalData
	if err := json.Unmarshal(arrival.Data, &data); err != nil || data.Count <= 1 {
		return "Cargo from the " + dayText(int64(arrival.Day)) + " shipment"
	}
	run := toArrivalRun(data.First, data.Every, data.Count)
	return "Cargo from the shipments of Days " + formatNumber(run.First) + " – " + formatNumber(run.Last)
}

// joinLines puts the reason on its own line under the summary.
func joinLines(summary string, reason string) string {
	if reason == "" {
		return summary
	}
	if summary == "" {
		return reason
	}
	return summary + "\n" + reason
}

// readBoard reads the board with the given queries, so a mutation can answer with the board its own
// transaction wrote.
func (service *Service) readBoard(requestContext context.Context, queries *store.Queries) (ColonyBoard, error) {
	colony, err := queries.GetColony(requestContext)
	if err != nil {
		return ColonyBoard{}, err
	}
	settings, err := queries.GetSettings(requestContext)
	if err != nil {
		return ColonyBoard{}, err
	}
	resources, err := queries.ListResources(requestContext)
	if err != nil {
		return ColonyBoard{}, err
	}
	roles, err := queries.ListRoles(requestContext)
	if err != nil {
		return ColonyBoard{}, err
	}
	return toColonyBoard(colony, settings, resources, roles)
}

// toColonyBoard fills Dm unconditionally; the Transformer decides who keeps it.
func toColonyBoard(colony store.Colony, settings store.Settings, resources []store.Resource, roles []store.Role) (ColonyBoard, error) {
	board := ColonyBoard{
		CampaignName: settings.CampaignName,
		ColonyName:   colony.Name,
		Day:          int64(colony.Day),
		Version:      colony.Version,
		Shipments:    toShipmentSchedule(int64(colony.Day), colony.ShipmentIntervalDays, colony.NextShipmentDay),
		Resources:    make([]ColonyResource, 0, len(resources)),
		Roles:        make([]RoleHeadcount, 0, len(roles)),
		Dm:           &ColonyBoardDm{},
	}
	for _, resource := range resources {
		if resource.Archived {
			continue
		}
		board.Resources = append(board.Resources, ColonyResource{
			Key:       resource.Key,
			Label:     resource.Label,
			Icon:      resource.Icon,
			Amount:    int64(resource.Amount),
			MinAmount: int64(resource.MinAmount),
			MaxAmount: int64Pointer(resource.MaxAmount),
			IsSystem:  resource.IsSystem,
		})
	}
	headcounts := make(map[string]game.Role, len(roles))
	for _, role := range roles {
		headcounts[role.Key] = game.Role{UnnamedCount: int64(role.UnnamedCount), Archived: role.Archived}
		if role.Key == game.UnassignedRoleKey {
			board.Unassigned = int64(role.UnnamedCount)
		}
		if role.Archived {
			continue
		}
		board.Roles = append(board.Roles, RoleHeadcount{
			Key:          role.Key,
			Label:        role.Label,
			Icon:         role.Icon,
			UnnamedCount: int64(role.UnnamedCount),
			Total:        int64(role.UnnamedCount),
			IsSystem:     role.IsSystem,
		})
	}
	board.Colonists = game.Colonists(headcounts)
	lastChange, err := availableLastChange(colony)
	if err != nil {
		return ColonyBoard{}, err
	}
	board.Dm.LastChange = lastChange
	return board, nil
}

// availableLastChange is the undo slot as the board shows it, while it can still be used.
func availableLastChange(colony store.Colony) (*LastChange, error) {
	record, err := usableLastChange(colony)
	if err != nil || record == nil {
		return nil, err
	}
	return &LastChange{GroupID: record.GroupID, Label: record.Label, Day: record.Day, ChangedAt: record.ChangedAt.UTC()}, nil
}

// usableLastChange is the undo slot while nothing has changed the colony since (spec §5.1), else nil.
func usableLastChange(colony store.Colony) (*lastChangeRecord, error) {
	if len(colony.LastChange) == 0 {
		return nil, nil
	}
	var record lastChangeRecord
	if err := json.Unmarshal(colony.LastChange, &record); err != nil {
		return nil, fmt.Errorf("core: decode colony.last_change: %w", err)
	}
	if record.VersionAfter != colony.Version {
		return nil, nil
	}
	return &record, nil
}

func toShipmentSchedule(day int64, intervalDays *int32, nextDay *int32) *ShipmentSchedule {
	if intervalDays == nil || nextDay == nil {
		return nil
	}
	return &ShipmentSchedule{IntervalDays: int64(*intervalDays), NextDay: int64(*nextDay), DaysUntil: int64(*nextDay) - day}
}

func int64Pointer(value *int32) *int64 {
	if value == nil {
		return nil
	}
	widened := int64(*value)
	return &widened
}
