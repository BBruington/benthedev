package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/game"
	"github.com/BBruington/colony-planner/internal/store"
)

// The Foundry bridge's third phase (docs/design/foundry-bridge.md §14): the GM sends coin deposits, NPCs
// and items to the DM's inbox. Each payload is checked as a request body is and stored cleaned; only the
// DM's Accept, or a save of the sheet an item opened, changes Colony Planner.

const (
	// BridgeDepositMaxGold is the largest COIN_DEPOSIT, in gold pieces.
	BridgeDepositMaxGold = 1000000

	bridgeActorNameMaxLength      = 100
	bridgeFoundryUUIDMaxLength    = 200
	bridgeDepositRefundTargetHead = "deposit-refund:"
	bridgeDepositUndoneTargetHead = "deposit-undone:"
)

var (
	bridgeActorIDPattern      = regexp.MustCompile(`^[A-Za-z0-9]{16}$`)
	bridgeFoundryActorPattern = regexp.MustCompile(`^(Compendium\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.)?Actor\.[A-Za-z0-9]{16}$`)
	bridgeFoundryItemPattern  = regexp.MustCompile(`^(Compendium\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.)?Item\.[A-Za-z0-9]{16}$`)
	// bridgeInboxKinds are the kinds that wait in the DM's inbox; Dismiss answers any of them.
	bridgeInboxKinds = []BridgeEventKind{BridgeEventKindTimeAdvanced, BridgeEventKindCoinDeposit, BridgeEventKindNpcImport, BridgeEventKindItemImport}
	// bridgeUndoReopenedKinds are the kinds a colony change accepts, so its undo puts them back.
	bridgeUndoReopenedKinds = []string{string(BridgeEventKindTimeAdvanced), string(BridgeEventKindCoinDeposit)}
)

// BridgeInboxDeposit is a COIN_DEPOSIT inbox item: coins the GM took from a PF2e party or character,
// held in Foundry until the DM accepts or dismisses them.
type BridgeInboxDeposit struct {
	_         struct{} `nullable:"true"`
	Gold      int64    `json:"gold"`
	ActorName string   `json:"actorName" doc:"Foundry's name for the actor that paid: the party or a character"`
}

// BridgeInboxNpc is an NPC_IMPORT inbox item, in NpcInput's field names so the NPC sheet fills from it.
// The inbox is the DM's alone, which is what lets it carry the true name and DM notes outside a dm object.
type BridgeInboxNpc struct {
	_              struct{} `nullable:"true"`
	FoundryUUID    string   `json:"foundryUuid" doc:"The Foundry actor's UUID"`
	Name           string   `json:"name" doc:"The true name"`
	Level          *int64   `json:"level" doc:"null when unknown"`
	Race           string   `json:"race"`
	CharacterClass string   `json:"characterClass"`
	Appearance     string   `json:"appearance"`
	DmNotes        string   `json:"dmNotes"`
}

// BridgeInboxMarketItem is an ITEM_IMPORT inbox item, in MarketItemInput's field names.
type BridgeInboxMarketItem struct {
	_              struct{} `nullable:"true"`
	FoundryUUID    string   `json:"foundryUuid" doc:"The Foundry item's UUID"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	PriceGold      *int64   `json:"priceGold" doc:"null when Foundry's price isn't a whole number of gold"`
	CompendiumUUID *string  `json:"compendiumUuid" doc:"The compendium item a paid shipment would hand the party, or null"`
}

// bridgeCoinDepositPayload is a COIN_DEPOSIT event's payload as Foundry sends it.
type bridgeCoinDepositPayload struct {
	Gold      *int64 `json:"gold"`
	ActorID   string `json:"actorId"`
	ActorName string `json:"actorName"`
}

// bridgeDepositContent is what a COIN_DEPOSIT keeps in bridge_event.content; gold has its own column.
type bridgeDepositContent struct {
	ActorID   string `json:"actorId"`
	ActorName string `json:"actorName"`
}

// bridgeDepositRefund is the DEPOSIT_REFUND payload. Like DEPOSIT_UNDONE's, it repeats only what Foundry
// sent and core checked, and names nothing of Colony Planner's.
type bridgeDepositRefund struct {
	DepositID string `json:"depositId"`
	Gold      int64  `json:"gold"`
	ActorID   string `json:"actorId"`
	ActorName string `json:"actorName"`
}

type bridgeDepositUndone struct {
	DepositID string `json:"depositId"`
	Gold      int64  `json:"gold"`
	ActorName string `json:"actorName"`
}

// coinDepositedData is a COIN_DEPOSITED entry's data: the inbox item it accepted.
type coinDepositedData struct {
	BridgeEventID uuid.UUID `json:"bridgeEventId"`
}

// validateBridgeImport checks one of the three inbox kinds Phase 3 adds, field by field.
func validateBridgeImport(field string, event BridgeInboundEvent) (validatedBridgeEvent, error) {
	payloadField := field + ".payload"
	var fieldErrors []FieldError
	collect := func(fieldError *FieldError) {
		if fieldError != nil {
			fieldErrors = append(fieldErrors, *fieldError)
		}
	}
	required := func(name string, value string, fieldError *FieldError, message string) {
		collect(fieldError)
		if fieldError == nil && value == "" {
			collect(&FieldError{Field: payloadField + "." + name, Message: message})
		}
	}
	foundryUUID := func(value string, pattern *regexp.Regexp, message string) string {
		trimmed := strings.TrimSpace(value)
		if len(trimmed) > bridgeFoundryUUIDMaxLength || !pattern.MatchString(trimmed) {
			collect(&FieldError{Field: payloadField + ".foundryUuid", Message: message})
		}
		return trimmed
	}
	checked := validatedBridgeEvent{event: event}
	var content any
	switch event.Kind {
	case BridgeEventKindCoinDeposit:
		var payload bridgeCoinDepositPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return validatedBridgeEvent{}, Invalid("A COIN_DEPOSIT payload holds gold, actorId and actorName.", FieldError{Field: payloadField, Message: "Send gold, actorId and actorName."})
		}
		if payload.Gold == nil || *payload.Gold < 1 || *payload.Gold > BridgeDepositMaxGold {
			collect(&FieldError{Field: payloadField + ".gold", Message: "Use a whole number of gold pieces from 1 to 1,000,000."})
		} else {
			gold := int32(*payload.Gold)
			checked.gold = &gold
		}
		if !bridgeActorIDPattern.MatchString(payload.ActorID) {
			collect(&FieldError{Field: payloadField + ".actorId", Message: "Send the paying actor's 16-character ID."})
		}
		actorName, fieldError := cleanText(payloadField+".actorName", payload.ActorName, bridgeActorNameMaxLength)
		required("actorName", actorName, fieldError, "Send the paying actor's name.")
		content = bridgeDepositContent{ActorID: payload.ActorID, ActorName: actorName}
	case BridgeEventKindNpcImport:
		var payload BridgeInboxNpc
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return validatedBridgeEvent{}, Invalid("An NPC_IMPORT payload holds the NPC's text fields.", FieldError{Field: payloadField, Message: "Send the NPC's fields."})
		}
		cleaned := BridgeInboxNpc{FoundryUUID: foundryUUID(payload.FoundryUUID, bridgeFoundryActorPattern, "Send a world or compendium actor's UUID.")}
		var fieldError *FieldError
		cleaned.Name, fieldError = cleanText(payloadField+".name", payload.Name, npcNameMaxLength)
		required("name", cleaned.Name, fieldError, "Send the NPC's name.")
		if payload.Level != nil && (*payload.Level < 0 || *payload.Level > npcLevelMax) {
			collect(&FieldError{Field: payloadField + ".level", Message: "Use a level from 0 to 100, or null."})
		}
		cleaned.Level = payload.Level
		cleaned.Race, fieldError = cleanText(payloadField+".race", payload.Race, npcShortMaxLength)
		collect(fieldError)
		cleaned.CharacterClass, fieldError = cleanText(payloadField+".characterClass", payload.CharacterClass, npcShortMaxLength)
		collect(fieldError)
		cleaned.Appearance, fieldError = cleanMultilineText(payloadField+".appearance", payload.Appearance, npcProseMaxLength)
		collect(fieldError)
		cleaned.DmNotes, fieldError = cleanMultilineText(payloadField+".dmNotes", payload.DmNotes, npcProseMaxLength)
		collect(fieldError)
		content = cleaned
	case BridgeEventKindItemImport:
		var payload BridgeInboxMarketItem
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return validatedBridgeEvent{}, Invalid("An ITEM_IMPORT payload holds the item's fields.", FieldError{Field: payloadField, Message: "Send the item's fields."})
		}
		cleaned := BridgeInboxMarketItem{FoundryUUID: foundryUUID(payload.FoundryUUID, bridgeFoundryItemPattern, "Send a world or compendium item's UUID.")}
		var fieldError *FieldError
		cleaned.Name, fieldError = cleanText(payloadField+".name", payload.Name, marketItemNameMaxLength)
		required("name", cleaned.Name, fieldError, "Send the item's name.")
		cleaned.Description, fieldError = cleanMultilineText(payloadField+".description", payload.Description, marketItemProseMaxLength)
		collect(fieldError)
		if payload.PriceGold != nil && (*payload.PriceGold < 0 || *payload.PriceGold > game.MaxStored) {
			collect(&FieldError{Field: payloadField + ".priceGold", Message: "Use a whole number of gold from 0 up, or null."})
		}
		cleaned.PriceGold = payload.PriceGold
		cleaned.CompendiumUUID, fieldError = cleanCompendiumUUID(payload.CompendiumUUID)
		if fieldError != nil {
			fieldError.Field = payloadField + ".compendiumUuid"
		}
		collect(fieldError)
		content = cleaned
	default:
		return validatedBridgeEvent{}, Invalid("That event kind isn't known.", FieldError{Field: field + ".kind", Message: "That event kind isn't known."})
	}
	if len(fieldErrors) > 0 {
		return validatedBridgeEvent{}, Invalid("A "+string(event.Kind)+" event needs fixing.", fieldErrors...)
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return validatedBridgeEvent{}, fmt.Errorf("core: encode a bridge event's content: %w", err)
	}
	checked.content = encoded
	return checked, nil
}

// toBridgeInboxItem decodes a PENDING row's stored content into its kind's own object.
func toBridgeInboxItem(row store.ListPendingBridgeEventsRow) (BridgeInboxItem, error) {
	item := BridgeInboxItem{ID: row.ID, Kind: row.Kind, ReceivedAt: row.ReceivedAt.UTC()}
	if row.Days != nil {
		item.Days = int64(*row.Days)
	}
	var err error
	switch row.Kind {
	case BridgeEventKindCoinDeposit:
		var content bridgeDepositContent
		err = json.Unmarshal(row.Content, &content)
		if row.Gold == nil {
			return BridgeInboxItem{}, errors.New("core: a deposit without gold")
		}
		item.Deposit = &BridgeInboxDeposit{Gold: int64(*row.Gold), ActorName: content.ActorName}
	case BridgeEventKindNpcImport:
		var npc BridgeInboxNpc
		err = json.Unmarshal(row.Content, &npc)
		item.Npc = &npc
	case BridgeEventKindItemImport:
		var marketItem BridgeInboxMarketItem
		err = json.Unmarshal(row.Content, &marketItem)
		item.Item = &marketItem
	}
	if err != nil {
		return BridgeInboxItem{}, fmt.Errorf("core: decode a bridge inbox item: %w", err)
	}
	return item, nil
}

// AcceptBridgeDeposit adds a PENDING COIN_DEPOSIT's gold to the colony: a colony change whose PENDING
// status is its guard, so the button needs no version. Past gold's max it refuses rather than caps, and
// the item waits (docs/design/foundry-bridge.md §14.5).
func (service *Service) AcceptBridgeDeposit(requestContext context.Context, viewer Viewer, eventID uuid.UUID) (ColonyChange, error) {
	return service.mutateColonyGuardedByStatus(requestContext, viewer, func(colony colonySnapshot, queries *store.Queries) (colonyOutcome, error) {
		decided, err := decideBridgeEvent(requestContext, queries, viewer.campaignID, eventID, []BridgeEventKind{BridgeEventKindCoinDeposit}, BridgeEventStatusAccepted, service.clock().UTC(), "")
		if err != nil {
			return colonyOutcome{}, err
		}
		if decided.Gold == nil {
			return colonyOutcome{}, errors.New("core: a deposit without gold")
		}
		result, err := game.Deposit(colony.state, int64(*decided.Gold))
		var invalidError *game.InvalidError
		if errors.As(err, &invalidError) {
			return colonyOutcome{}, Conflict(invalidError.Message + ".")
		}
		if err != nil {
			return colonyOutcome{}, err
		}
		// No actor name: it is Foundry's text and stays in the DM's inbox.
		title := "Coins deposited · " + formatNumber(int64(*decided.Gold)) + " " + strings.ToLower(colony.labels.of(game.GoldResourceKey))
		return colonyOutcome{
			after: result.After,
			label: title,
			entries: []plannedEntry{{
				day:        colony.state.Day,
				kind:       TimelineKindCoinDeposited,
				title:      title,
				visibility: TimelineVisibilityPublic,
				deltas:     result.Applied,
				data:       coinDepositedData{BridgeEventID: eventID},
			}},
			topics:        []Topic{TopicBridge},
			bridgeEventID: &eventID,
		}, nil
	})
}

// DismissBridgeInboxItem sets a PENDING item DISMISSED; anything else is 409. A dismissed deposit sends
// its coins back through DEPOSIT_REFUND in the same transaction, so it needs an active link.
func (service *Service) DismissBridgeInboxItem(requestContext context.Context, viewer Viewer, eventID uuid.UUID) error {
	if err := requireDm(viewer); err != nil {
		return err
	}
	bridged := false
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		// The colony lock first, as every operation that writes outbox rows takes it (Invariant 21).
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		decided, err := decideBridgeEvent(requestContext, queries, viewer.campaignID, eventID, bridgeInboxKinds, BridgeEventStatusDismissed, service.clock().UTC(), "")
		if err != nil {
			return err
		}
		if decided.Kind != BridgeEventKindCoinDeposit {
			return nil
		}
		outbox, err := service.openBridgeOutbox(requestContext, queries, viewer.campaignID)
		if err != nil {
			return err
		}
		if !outbox.linked {
			return Conflict("Connect Foundry first, so the coins can go back to the actor that paid.")
		}
		content, err := depositContent(decided.Gold, decided.Content)
		if err != nil {
			return err
		}
		if err := outbox.write(BridgeOutboxKindDepositRefund, bridgeDepositRefundTargetHead+decided.ModuleEventID.String(), bridgeDepositRefund{
			DepositID: decided.ModuleEventID.String(), Gold: int64(*decided.Gold), ActorID: content.ActorID, ActorName: content.ActorName,
		}); err != nil {
			return err
		}
		bridged, err = outbox.finish()
		return err
	})
	if err != nil {
		return err
	}
	service.broker.Publish(viewer.campaignID, withBridgeTopic(bridged, TopicColony, TopicBridge)...)
	return nil
}

// depositUndone tells the GMs an accepted deposit was undone and waits in the inbox again, keyed by the
// undone Accept's group, so each undo is its own whisper.
func (outbox *bridgeOutbox) depositUndone(undoneGroupID uuid.UUID, reopened store.ReopenAcceptedBridgeEventRow) error {
	if !outbox.linked {
		return nil
	}
	content, err := depositContent(reopened.Gold, reopened.Content)
	if err != nil {
		return err
	}
	return outbox.write(BridgeOutboxKindDepositUndone, bridgeDepositUndoneTargetHead+undoneGroupID.String(), bridgeDepositUndone{
		DepositID: reopened.ModuleEventID.String(), Gold: int64(*reopened.Gold), ActorName: content.ActorName,
	})
}

func depositContent(gold *int32, stored []byte) (bridgeDepositContent, error) {
	if gold == nil {
		return bridgeDepositContent{}, errors.New("core: a deposit without gold")
	}
	var content bridgeDepositContent
	if err := json.Unmarshal(stored, &content); err != nil {
		return bridgeDepositContent{}, fmt.Errorf("core: decode a deposit: %w", err)
	}
	return content, nil
}
