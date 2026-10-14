package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/BBruington/colony-planner/internal/game"
	"github.com/BBruington/colony-planner/internal/store"
)

// TimelineEntry is one timeline row in the viewer's projection. Players get only PUBLIC entries that
// have not been undone, filtered in SQL (spec §2.2, item 3); the DM gets every entry, with its marks
// in Dm. Title and body were rendered when the entry was written, in player-facing words.
type TimelineEntry struct {
	ID uuid.UUID `json:"id" format:"uuid"`
	// GroupID is shared by every entry one colony mutation wrote; null for CUSTOM entries.
	GroupID *uuid.UUID   `json:"groupId" format:"uuid" nullable:"true"`
	Day     int64        `json:"day"`
	Kind    TimelineKind `json:"kind"`
	Title   string       `json:"title"`
	Body    string       `json:"body"`
	// Deltas are the applied effects only (spec §5.4).
	Deltas Effects `json:"deltas"`
	// Arrivals is set on SHIPMENT_ARRIVED: the arrival days the entry stands for (count 1 unless
	// folded beyond 12 arrivals).
	Arrivals *ArrivalRun `json:"arrivals,omitempty" doc:"SHIPMENT_ARRIVED only; absent otherwise"`
	// ArrivalEntryID is set on CARGO_RECORDED: the arrival the cargo came on.
	ArrivalEntryID *uuid.UUID `json:"arrivalEntryId" format:"uuid" nullable:"true" doc:"CARGO_RECORDED only; null otherwise"`
	// NpcID links an entry about an NPC to their page. Players only get entries whose NPC is revealed
	// or unlinked, so it never names a hidden NPC to them. The name the entry was written with stays
	// out: it can be a name the party doesn't know (Invariant 15).
	NpcID *uuid.UUID `json:"npcId" format:"uuid" nullable:"true" doc:"The NPC this entry is about; null for none, or once that NPC is deleted"`
	// QuestID, PatronID and RequestID link an entry to what it is about. Players only get entries whose
	// linked rows they can see, so a link never names a hidden quest, patron or action to them.
	QuestID   *uuid.UUID `json:"questId" format:"uuid" nullable:"true" doc:"The quest this entry is about; null for none, or once that quest is deleted"`
	PatronID  *uuid.UUID `json:"patronId" format:"uuid" nullable:"true" doc:"The patron this entry names; null for none"`
	RequestID *uuid.UUID `json:"requestId" format:"uuid" nullable:"true" doc:"The request this entry granted; null for none"`
	// SessionID links SESSION_PLAYED to its session; sessions have no secrets.
	SessionID *uuid.UUID       `json:"sessionId" format:"uuid" nullable:"true" doc:"The session this entry is about; null for none"`
	CreatedAt time.Time        `json:"createdAt"`
	Dm        *TimelineEntryDm `json:"dm,omitempty"`
}

// TimelineEntryDm is what only the DM sees about an entry: its visibility (eye-slash when DM),
// whether it was undone (struck through) and what the DM may do with it (spec §6.2).
type TimelineEntryDm struct {
	Visibility TimelineVisibility `json:"visibility"`
	UndoneAt   *time.Time         `json:"undoneAt"`
	EditedAt   *time.Time         `json:"editedAt"`
	// CanDelete is true for CUSTOM entries only; auto-logged entries are never deleted.
	CanDelete bool `json:"canDelete"`
	// CanRecordCargo is true for a SHIPMENT_ARRIVED entry that has not been undone.
	CanRecordCargo bool `json:"canRecordCargo"`
	// Note is detail kept off the entry's text, such as a hidden patron's favor line (spec §5.6).
	Note string `json:"note" doc:"DM-only detail, e.g. a hidden patron's favor line; blank for none"`
}

// TimelineRangeInput is GET /api/campaigns/{campaignId}/timeline?fromDay=&toDay= (both inclusive).
type TimelineRangeInput struct {
	FromDay int64
	ToDay   int64
}

// TimelinePage is the entries of a day range, oldest first. The web pages seven days at a time
// ("Days 36–42"); the server only bounds the range.
type TimelinePage struct {
	FromDay int64           `json:"fromDay"`
	ToDay   int64           `json:"toDay"`
	Today   int64           `json:"today" doc:"The colony's current day, for Jump to today"`
	Entries []TimelineEntry `json:"entries" doc:"Ordered by day, then write order"`
	// EarlierDay and LaterDay let paging skip empty weeks (after advancing a million days, say).
	EarlierDay *int64 `json:"earlierDay" doc:"The nearest day before fromDay with an entry this viewer sees; null when none"`
	LaterDay   *int64 `json:"laterDay" doc:"The nearest day after toDay with an entry this viewer sees; null when none"`
}

// RecentTimeline is the newest entries in write order, for the board and the live toasts.
type RecentTimeline struct {
	Entries []TimelineEntry `json:"entries" doc:"Newest first, at most 10"`
}

// TimelineEntryInput creates a CUSTOM entry or replaces an entry's text and visibility (PUT
// semantics). Only a CUSTOM entry may move to another day (spec §5.8).
type TimelineEntryInput struct {
	Day        int64              `json:"day" doc:"0 or more, past or future; must stay the same unless the entry is CUSTOM"`
	Title      string             `json:"title" doc:"1 to 120 characters after trimming"`
	Body       string             `json:"body" doc:"At most 2000 characters; line breaks allowed"`
	Visibility TimelineVisibility `json:"visibility"`
}

const (
	// maxTimelineRangeDays bounds one timeline read; entries per day are few, so this bounds the answer.
	maxTimelineRangeDays = 366
	recentTimelineLimit  = 10
)

// arrivalData is a SHIPMENT_ARRIVED entry's data: {first, every, count}.
type arrivalData struct {
	First int64 `json:"first"`
	Every int64 `json:"every"`
	Count int64 `json:"count"`
}

// dmNoteData is the data of an entry with DM-only detail: {dmNote}.
type dmNoteData struct {
	DmNote string `json:"dmNote"`
}

// cargoData is a CARGO_RECORDED entry's data: {arrivalEntryId}.
type cargoData struct {
	ArrivalEntryID uuid.UUID `json:"arrivalEntryId"`
}

// validateTimelineRange checks a page of days, for the timeline and its session bands alike.
func validateTimelineRange(input TimelineRangeInput) error {
	var fieldErrors []FieldError
	if input.FromDay < 0 || input.FromDay > game.MaxStored {
		fieldErrors = append(fieldErrors, FieldError{Field: "fromDay", Message: "Pick a day from 0 up."})
	}
	switch {
	case input.ToDay < input.FromDay:
		fieldErrors = append(fieldErrors, FieldError{Field: "toDay", Message: "The last day can't be before the first."})
	case input.ToDay-input.FromDay >= maxTimelineRangeDays:
		fieldErrors = append(fieldErrors, FieldError{Field: "toDay", Message: fmt.Sprintf("Show at most %d days at once.", maxTimelineRangeDays)})
	}
	if len(fieldErrors) > 0 {
		return Invalid("That range of days can't be shown.", fieldErrors...)
	}
	return nil
}

func (service *Service) ListTimeline(requestContext context.Context, viewer Viewer, input TimelineRangeInput) (TimelinePage, error) {
	if err := requireActive(viewer); err != nil {
		return TimelinePage{}, err
	}
	if err := validateTimelineRange(input); err != nil {
		return TimelinePage{}, err
	}
	// Clamped after validation so a range ending past the last storable day still reads.
	toDay := min(input.ToDay, int64(game.MaxStored))
	dmProjection := viewer.SeesDmFields()
	colony, err := service.queries.GetColony(requestContext, viewer.campaignID)
	if err != nil {
		return TimelinePage{}, campaignGone(err)
	}
	rows, err := service.queries.ListTimelineEvents(requestContext, store.ListTimelineEventsParams{
		CampaignID:   viewer.campaignID,
		FromDay:      int32(input.FromDay),
		ToDay:        int32(toDay),
		DmProjection: dmProjection,
	})
	if err != nil {
		return TimelinePage{}, err
	}
	entries, err := toTimelineEntries(rows)
	if err != nil {
		return TimelinePage{}, err
	}
	earlierDay, err := optionalDay(service.queries.PreviousTimelineDay(requestContext, store.PreviousTimelineDayParams{
		CampaignID:   viewer.campaignID,
		BeforeDay:    int32(input.FromDay),
		DmProjection: dmProjection,
	}))
	if err != nil {
		return TimelinePage{}, err
	}
	laterDay, err := optionalDay(service.queries.NextTimelineDay(requestContext, store.NextTimelineDayParams{
		CampaignID:   viewer.campaignID,
		AfterDay:     int32(toDay),
		DmProjection: dmProjection,
	}))
	if err != nil {
		return TimelinePage{}, err
	}
	return TimelinePage{
		FromDay:    input.FromDay,
		ToDay:      input.ToDay,
		Today:      int64(colony.Day),
		Entries:    entries,
		EarlierDay: earlierDay,
		LaterDay:   laterDay,
	}, nil
}

func (service *Service) RecentTimeline(requestContext context.Context, viewer Viewer) (RecentTimeline, error) {
	if err := requireActive(viewer); err != nil {
		return RecentTimeline{}, err
	}
	rows, err := service.queries.ListRecentTimelineEvents(requestContext, store.ListRecentTimelineEventsParams{
		CampaignID:   viewer.campaignID,
		DmProjection: viewer.SeesDmFields(),
		EntryLimit:   recentTimelineLimit,
	})
	if err != nil {
		return RecentTimeline{}, err
	}
	entries, err := toTimelineEntries(rows)
	if err != nil {
		return RecentTimeline{}, err
	}
	return RecentTimeline{Entries: entries}, nil
}

const (
	timelineTitleMaxLength = 120
	timelineBodyMaxLength  = 2000
)

// CreateCustomEntry writes a CUSTOM entry on any day, past or future. It is not a colony mutation.
func (service *Service) CreateCustomEntry(requestContext context.Context, viewer Viewer, input TimelineEntryInput) (TimelineEntry, error) {
	if err := requireDm(viewer); err != nil {
		return TimelineEntry{}, err
	}
	cleaned, err := cleanTimelineEntryInput(input)
	if err != nil {
		return TimelineEntry{}, err
	}
	var row store.TimelineEvent
	err = service.inTransaction(requestContext, func(queries *store.Queries) error {
		// Not a colony mutation, but every timeline insert waits for the colony lock, so a note can't
		// commit ahead of an advance that began before it and leave that advance's entries looking
		// older than news the players have already seen (their toasts would be skipped).
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		row, err = queries.CreateTimelineEvent(requestContext, store.CreateTimelineEventParams{
			CampaignID: viewer.campaignID,
			Day:        int32(cleaned.Day),
			Kind:       string(TimelineKindCustom),
			Title:      cleaned.Title,
			Body:       cleaned.Body,
			Visibility: cleaned.Visibility,
			Deltas:     []byte("{}"),
			Data:       []byte("{}"),
			CreatedBy:  memberReference(viewer),
		})
		return err
	})
	if err != nil {
		return TimelineEntry{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicTimeline)
	return toTimelineEntry(row)
}

// UpdateTimelineEntry replaces any entry's title, body and visibility (and a CUSTOM entry's day).
// Editing never reverses effects (spec §5.8).
func (service *Service) UpdateTimelineEntry(requestContext context.Context, viewer Viewer, timelineEntryID uuid.UUID, input TimelineEntryInput) (TimelineEntry, error) {
	if err := requireDm(viewer); err != nil {
		return TimelineEntry{}, err
	}
	cleaned, err := cleanTimelineEntryInput(input)
	if err != nil {
		return TimelineEntry{}, err
	}
	var row store.TimelineEvent
	err = service.inTransaction(requestContext, func(queries *store.Queries) error {
		existing, err := queries.GetTimelineEvent(requestContext, store.GetTimelineEventParams{CampaignID: viewer.campaignID, ID: timelineEntryID})
		if errors.Is(err, pgx.ErrNoRows) {
			return NotFound("That timeline entry doesn't exist.")
		}
		if err != nil {
			return err
		}
		if TimelineKind(existing.Kind) != TimelineKindCustom && int64(existing.Day) != cleaned.Day {
			message := "Only a custom entry can move to another day."
			return Invalid(message, FieldError{Field: "day", Message: message})
		}
		// Checked whenever PUBLIC text changes, not only when an entry turns PUBLIC, so the rule holds
		// for everything players can read.
		goingPublic := existing.Visibility != TimelineVisibilityPublic
		textChanged := existing.Title != cleaned.Title || existing.Body != cleaned.Body
		if cleaned.Visibility == TimelineVisibilityPublic && (goingPublic || textChanged) {
			entry := npcEntry{npcID: existing.NpcID, npcName: existing.NpcName, title: cleaned.Title, body: cleaned.Body}
			if err := checkNpcEntryCanGoPublic(requestContext, queries, viewer.campaignID, entry, goingPublic); err != nil {
				return err
			}
		}
		row, err = queries.UpdateTimelineEvent(requestContext, store.UpdateTimelineEventParams{
			CampaignID: viewer.campaignID,
			ID:         timelineEntryID,
			Day:        int32(cleaned.Day),
			Title:      cleaned.Title,
			Body:       cleaned.Body,
			Visibility: cleaned.Visibility,
		})
		return err
	})
	if err != nil {
		return TimelineEntry{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicTimeline)
	return toTimelineEntry(row)
}

// DeleteCustomEntry deletes a CUSTOM entry; any other kind gets a conflict (spec §5.8).
func (service *Service) DeleteCustomEntry(requestContext context.Context, viewer Viewer, timelineEntryID uuid.UUID) error {
	if err := requireDm(viewer); err != nil {
		return err
	}
	existing, err := service.queries.GetTimelineEvent(requestContext, store.GetTimelineEventParams{CampaignID: viewer.campaignID, ID: timelineEntryID})
	if errors.Is(err, pgx.ErrNoRows) {
		return NotFound("That timeline entry doesn't exist.")
	}
	if err != nil {
		return err
	}
	if TimelineKind(existing.Kind) != TimelineKindCustom {
		return Conflict("Only custom entries can be deleted. Edit this one, or hide it from players, instead.")
	}
	if _, err := service.queries.DeleteCustomTimelineEvent(requestContext, store.DeleteCustomTimelineEventParams{CampaignID: viewer.campaignID, ID: timelineEntryID}); err != nil {
		return err
	}
	service.broker.Publish(viewer.campaignID, TopicTimeline)
	return nil
}

// npcEntry is what decides whether an entry about an NPC may be PUBLIC: its link, the display name
// the app wrote it with, and its text.
type npcEntry struct {
	npcID   *uuid.UUID
	npcName string
	title   string
	body    string
}

// npcEntryRefusal says why players can't be shown an entry, or "" when they can (Invariant 15): its
// NPC is hidden (checked only when requireRevealed), or its text names the NPC by a name the party
// doesn't know. Those are the name the entry was written with once it is no longer the display name,
// and the true name while a known name is set. Text the DM typed in some other form is theirs to
// judge. Once the NPC is deleted, the name it was written with stays, unless players knew it by that
// name (PrepareNpcTimelineEventsForDelete).
func npcEntryRefusal(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, entry npcEntry, requireRevealed bool) (string, error) {
	if entry.npcID == nil {
		if entry.npcName != "" && namesUnknownName(entry.title+"\n"+entry.body, "", entry.npcName) {
			return "This entry names an NPC who was deleted, by a name the party may not know; edit the text first.", nil
		}
		return "", nil
	}
	npc, err := queries.GetNpc(requestContext, store.GetNpcParams{CampaignID: campaignID, ID: *entry.npcID})
	if err != nil {
		return "", err
	}
	if requireRevealed && !npc.Revealed {
		return npc.DisplayName + " is hidden from players. Reveal them first.", nil
	}
	var unknownNames []string
	if entry.npcName != "" && entry.npcName != npc.DisplayName {
		unknownNames = append(unknownNames, entry.npcName)
	}
	if npc.Name != npc.DisplayName {
		unknownNames = append(unknownNames, npc.Name)
	}
	if namesUnknownName(entry.title+"\n"+entry.body, npc.DisplayName, unknownNames...) {
		return "This entry names them by a name the party doesn't know; edit the text first.", nil
	}
	return "", nil
}

// checkNpcEntryCanGoPublic is npcEntryRefusal as a field error on visibility.
func checkNpcEntryCanGoPublic(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, entry npcEntry, requireRevealed bool) error {
	message, err := npcEntryRefusal(requestContext, queries, campaignID, entry, requireRevealed)
	if err != nil || message == "" {
		return err
	}
	return Invalid(message, FieldError{Field: "visibility", Message: message})
}

// namesUnknownName reports whether text holds one of unknownNames as whole words, ignoring case and
// reading any run of whitespace or invisible format characters as one space, except where the match
// is part of the display name itself ("Tam" inside "Old Tam").
func namesUnknownName(text string, displayName string, unknownNames ...string) bool {
	text = normalizeName(text)
	var knownSpans [][2]int
	if known := normalizeName(displayName); known != "" {
		knownSpans = wholeWordSpans(text, known)
	}
	for _, unknownName := range unknownNames {
		unknown := normalizeName(unknownName)
		if unknown == "" {
			continue
		}
		for _, span := range wholeWordSpans(text, unknown) {
			insideKnown := slices.ContainsFunc(knownSpans, func(known [2]int) bool { return known[0] <= span[0] && span[1] <= known[1] })
			if !insideKnown {
				return true
			}
		}
	}
	return false
}

func normalizeName(text string) string {
	visible := strings.Map(func(character rune) rune {
		if unicode.Is(unicode.Cf, character) {
			return ' '
		}
		return character
	}, text)
	return strings.Join(strings.Fields(strings.ToLower(visible)), " ")
}

// wholeWordSpans is every [start, end) where name occurs in text not joined to a letter or digit.
func wholeWordSpans(text string, name string) [][2]int {
	var spans [][2]int
	for offset := 0; offset < len(text); {
		index := strings.Index(text[offset:], name)
		if index < 0 {
			break
		}
		start := offset + index
		end := start + len(name)
		before, _ := utf8.DecodeLastRuneInString(text[:start])
		after, _ := utf8.DecodeRuneInString(text[end:])
		if !isWordRune(before) && !isWordRune(after) {
			spans = append(spans, [2]int{start, end})
		}
		_, width := utf8.DecodeRuneInString(text[start:])
		offset = start + width
	}
	return spans
}

func isWordRune(character rune) bool {
	return character != utf8.RuneError && (unicode.IsLetter(character) || unicode.IsDigit(character))
}

// cleanTimelineEntryInput checks a custom entry or an edit, and returns it trimmed.
func cleanTimelineEntryInput(input TimelineEntryInput) (TimelineEntryInput, error) {
	var fieldErrors []FieldError
	if input.Day < 0 || input.Day > game.MaxStored {
		fieldErrors = append(fieldErrors, FieldError{Field: "day", Message: "Pick a day from 0 up."})
	}
	title, fieldError := cleanText("title", input.Title, timelineTitleMaxLength)
	switch {
	case fieldError != nil:
		fieldErrors = append(fieldErrors, *fieldError)
	case title == "":
		fieldErrors = append(fieldErrors, FieldError{Field: "title", Message: "Give the entry a title."})
	}
	body, fieldError := cleanMultilineText("body", input.Body, timelineBodyMaxLength)
	if fieldError != nil {
		fieldErrors = append(fieldErrors, *fieldError)
	}
	if !slices.Contains(AllTimelineVisibilityValues(), input.Visibility) {
		fieldErrors = append(fieldErrors, FieldError{Field: "visibility", Message: "Choose PUBLIC or DM."})
	}
	if len(fieldErrors) > 0 {
		return TimelineEntryInput{}, Invalid("The entry needs fixing.", fieldErrors...)
	}
	return TimelineEntryInput{Day: input.Day, Title: title, Body: body, Visibility: input.Visibility}, nil
}

func toTimelineEntries(rows []store.TimelineEvent) ([]TimelineEntry, error) {
	entries := make([]TimelineEntry, 0, len(rows))
	for _, row := range rows {
		entry, err := toTimelineEntry(row)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// toTimelineEntry fills Dm unconditionally; the SQL projection decides which rows a player gets and
// the Transformer decides who keeps Dm.
func toTimelineEntry(row store.TimelineEvent) (TimelineEntry, error) {
	kind := TimelineKind(row.Kind)
	entry := TimelineEntry{
		ID:        row.ID,
		GroupID:   row.GroupID,
		Day:       int64(row.Day),
		Kind:      kind,
		Title:     row.Title,
		Body:      row.Body,
		NpcID:     row.NpcID,
		QuestID:   row.QuestID,
		PatronID:  row.PatronID,
		RequestID: row.RequestID,
		SessionID: row.SessionID,
		CreatedAt: row.CreatedAt.UTC(),
		Dm: &TimelineEntryDm{
			Visibility:     row.Visibility,
			UndoneAt:       utcPointer(row.UndoneAt),
			EditedAt:       utcPointer(row.EditedAt),
			CanDelete:      kind == TimelineKindCustom,
			CanRecordCargo: kind == TimelineKindShipmentArrived && row.UndoneAt == nil,
		},
	}
	if err := json.Unmarshal(row.Deltas, &entry.Deltas); err != nil {
		return TimelineEntry{}, fmt.Errorf("core: decode timeline_event %s deltas: %w", row.ID, err)
	}
	switch kind {
	case TimelineKindShipmentArrived:
		var data arrivalData
		if err := json.Unmarshal(row.Data, &data); err != nil {
			return TimelineEntry{}, fmt.Errorf("core: decode timeline_event %s data: %w", row.ID, err)
		}
		run := toArrivalRun(data.First, data.Every, data.Count)
		entry.Arrivals = &run
	case TimelineKindCargoRecorded:
		var data cargoData
		if err := json.Unmarshal(row.Data, &data); err != nil {
			return TimelineEntry{}, fmt.Errorf("core: decode timeline_event %s data: %w", row.ID, err)
		}
		if data.ArrivalEntryID != uuid.Nil {
			arrivalEntryID := data.ArrivalEntryID
			entry.ArrivalEntryID = &arrivalEntryID
		}
	case TimelineKindQuestCompleted, TimelineKindQuestFailed:
		var data dmNoteData
		if err := json.Unmarshal(row.Data, &data); err != nil {
			return TimelineEntry{}, fmt.Errorf("core: decode timeline_event %s data: %w", row.ID, err)
		}
		entry.Dm.Note = data.DmNote
	}
	return entry, nil
}

func toArrivalRun(first int64, every int64, count int64) ArrivalRun {
	return ArrivalRun{First: first, Every: every, Count: count, Last: first + every*(count-1)}
}

// optionalDay turns a single-day lookup that may find nothing into a nullable day.
func optionalDay(day int32, err error) (*int64, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	widened := int64(day)
	return &widened, nil
}
