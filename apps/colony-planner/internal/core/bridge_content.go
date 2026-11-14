package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/BBruington/colony-planner/internal/store"
)

// The Foundry bridge's second phase (docs/design/foundry-bridge.md §13): revealed NPCs, places and
// quests as journal entries, paid shipments and polls as chat messages. Every payload is read from the
// player projection in the change's own transaction, so a projection read that finds nothing (a hidden
// row) writes no upsert.

const (
	bridgeNpcTargetHead            = "npc:"
	bridgePlaceTargetHead          = "place:"
	bridgeQuestTargetHead          = "quest:"
	bridgeDeliveryTargetHead       = "delivery:"
	bridgeDeliveryUndoneTargetHead = "delivery-undone:"
	bridgePollTargetHead           = "poll:"
	bridgePollPostedTargetTail     = ":posted"
	bridgePollClosedTargetTail     = ":closed"
)

// bridgeOneShotKinds are the kinds Foundry applies once by their target key; every other kind carries
// its target's whole state. Only these are trimmed past the outbox cap.
var bridgeOneShotKinds = []string{
	string(BridgeOutboxKindDaysAdvanced),
	string(BridgeOutboxKindDelivery),
	string(BridgeOutboxKindDeliveryUndone),
	string(BridgeOutboxKindPollPosted),
	string(BridgeOutboxKindPollClosed),
}

// bridgeBackfilledKinds are the state kinds pairing sends again whole, so an earlier link's waiting
// rows of them are dropped first: one queued for a row hidden since would show it again.
var bridgeBackfilledKinds = []string{
	string(BridgeOutboxKindColonySummary),
	string(BridgeOutboxKindNpcUpsert),
	string(BridgeOutboxKindNpcWithdraw),
	string(BridgeOutboxKindPlaceUpsert),
	string(BridgeOutboxKindPlaceWithdraw),
	string(BridgeOutboxKindQuestUpsert),
	string(BridgeOutboxKindQuestWithdraw),
}

// bridgeWithdrawal is a withdraw kind's payload: it doesn't say whether the row was hidden or deleted.
type bridgeWithdrawal struct{}

type bridgeNpcPayload struct {
	DisplayName    string      `json:"displayName"`
	Aliases        []string    `json:"aliases"`
	Labels         []string    `json:"labels"`
	Race           string      `json:"race"`
	CharacterClass string      `json:"characterClass"`
	Age            string      `json:"age"`
	Level          *int64      `json:"level"`
	Appearance     string      `json:"appearance"`
	Personality    string      `json:"personality"`
	Backstory      string      `json:"backstory"`
	PlayerNotes    string      `json:"playerNotes"`
	Status         NpcStatus   `json:"status"`
	Health         NpcHealth   `json:"health"`
	Relation       NpcRelation `json:"relation"`
	CurrentPlace   *string     `json:"currentPlace"`
	OriginPlace    *string     `json:"originPlace"`
	ColonyRole     *string     `json:"colonyRole"`
	SiteURL        string      `json:"siteUrl"`
}

type bridgePlacePayload struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	PlayerNotes string `json:"playerNotes"`
	SiteURL     string `json:"siteUrl"`
}

type bridgeQuestPayload struct {
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Kind        QuestKind   `json:"kind"`
	Status      QuestStatus `json:"status"`
	Giver       *string     `json:"giver"`
	Patron      *string     `json:"patron"`
	Reward      string      `json:"reward"`
	RewardFavor int64       `json:"rewardFavor"`
	Loot        []LootItem  `json:"loot"`
	ResolvedDay *int64      `json:"resolvedDay"`
	SiteURL     string      `json:"siteUrl"`
}

type bridgeDeliveryPayload struct {
	PaidDay   int64                `json:"paidDay"`
	TotalGold int64                `json:"totalGold"`
	Summary   string               `json:"summary"`
	Items     []bridgeDeliveryItem `json:"items"`
}

type bridgeDeliveryItem struct {
	Name           string  `json:"name"`
	Quantity       int64   `json:"quantity"`
	CompendiumUUID *string `json:"compendiumUuid"`
}

type bridgeDeliveryUndonePayload struct {
	Summary string                     `json:"summary"`
	Items   []bridgeDeliveryUndoneItem `json:"items"`
}

type bridgeDeliveryUndoneItem struct {
	Name     string `json:"name"`
	Quantity int64  `json:"quantity"`
}

type bridgePollPostedPayload struct {
	Question      string     `json:"question"`
	Options       []string   `json:"options"`
	AllowMultiple bool       `json:"allowMultiple"`
	Voting        PollVoting `json:"voting"`
	ClosesAt      time.Time  `json:"closesAt"`
	PollsURL      string     `json:"pollsUrl"`
}

type bridgePollClosedPayload struct {
	Question string                   `json:"question"`
	ClosedAt time.Time                `json:"closedAt"`
	Options  []bridgePollClosedOption `json:"options"`
}

type bridgePollClosedOption struct {
	Label      string `json:"label"`
	Count      int64  `json:"count"`
	OtherVotes int64  `json:"otherVotes"`
}

// bridgeNextSession is COLONY_SUMMARY's next game night: a snapshot taken when the row is written.
type bridgeNextSession struct {
	Title     string    `json:"title"`
	StartsAt  time.Time `json:"startsAt"`
	EndsAt    time.Time `json:"endsAt"`
	LocalDate string    `json:"localDate"`
	Timezone  string    `json:"timezone"`
}

// bridgeOutbox writes one transaction's content rows: they wait in pending until finish inserts them
// together and trims the outbox once. Whether the campaign has an active link is read once, so an
// unlinked campaign makes no projection reads. The caller holds the colony lock, taken first as
// PairBridge takes it, so a pairing's backfill either sees this change or waits for it.
type bridgeOutbox struct {
	service        *Service
	requestContext context.Context
	queries        *store.Queries
	campaignID     uuid.UUID
	linked         bool
	pending        []bridgePendingRow
	// tookBack is set when a waiting row was deleted, which Foundry also hears about.
	tookBack     bool
	campaignSlug string
	colony       *colonySnapshot
}

type bridgePendingRow struct {
	kind      BridgeOutboxKind
	targetKey string
	payload   []byte
}

func (service *Service) openBridgeOutbox(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID) (*bridgeOutbox, error) {
	linked, err := queries.HasActiveBridgeLink(requestContext, campaignID)
	if err != nil {
		return nil, err
	}
	return &bridgeOutbox{service: service, requestContext: requestContext, queries: queries, campaignID: campaignID, linked: linked}, nil
}

// write queues a row; a later write of the same target replaces it, keeping one waiting row per target.
func (outbox *bridgeOutbox) write(kind BridgeOutboxKind, targetKey string, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("core: encode a bridge payload: %w", err)
	}
	outbox.pending = slices.DeleteFunc(outbox.pending, func(row bridgePendingRow) bool { return row.targetKey == targetKey })
	outbox.pending = append(outbox.pending, bridgePendingRow{kind: kind, targetKey: targetKey, payload: encoded})
	return nil
}

// finish inserts the queued rows with consecutive seqs in the order they were written, trims the
// outbox once, and reports whether Foundry has anything new to hear. Every use of an outbox ends here.
func (outbox *bridgeOutbox) finish() (bool, error) {
	rows := outbox.pending
	outbox.pending = nil
	if len(rows) == 0 {
		return outbox.tookBack, nil
	}
	wrote, err := outbox.service.insertBridgeRows(outbox.requestContext, outbox.queries, outbox.campaignID, rows)
	if err != nil || !wrote {
		return outbox.tookBack, err
	}
	return true, outbox.service.trimBridgeOutbox(outbox.requestContext, outbox.queries, outbox.campaignID)
}

func (outbox *bridgeOutbox) siteURL(page string) (string, error) {
	if outbox.campaignSlug == "" {
		header, err := outbox.queries.GetBoardHeader(outbox.requestContext, outbox.campaignID)
		if err != nil {
			return "", campaignGone(err)
		}
		outbox.campaignSlug = header.CampaignSlug
	}
	return outbox.service.publicURL + "/c/" + outbox.campaignSlug + page, nil
}

// colonyLabels is the colony's resource order and labels, read once, for a quest's reward text.
func (outbox *bridgeOutbox) colonyLabels() (colonySnapshot, error) {
	if outbox.colony == nil {
		colony, err := readColony(outbox.requestContext, outbox.queries, outbox.campaignID, false)
		if err != nil {
			return colonySnapshot{}, err
		}
		outbox.colony = &colony
	}
	return *outbox.colony, nil
}

// npcUpsert writes NPC_UPSERT for a revealed NPC and nothing for a hidden or missing one.
func (outbox *bridgeOutbox) npcUpsert(npcID uuid.UUID) error {
	return outbox.npcUpserts([]uuid.UUID{npcID})
}

// npcUpserts upserts each NPC in order with one projection read; any hidden or missing are skipped.
func (outbox *bridgeOutbox) npcUpserts(npcIDs []uuid.UUID) error {
	if !outbox.linked || len(npcIDs) == 0 {
		return nil
	}
	rows, err := outbox.queries.ListRevealedNpcViews(outbox.requestContext, store.ListRevealedNpcViewsParams{CampaignID: outbox.campaignID, Ids: npcIDs})
	if err != nil {
		return err
	}
	npcs := make(map[uuid.UUID]Npc, len(rows))
	for _, row := range rows {
		npcs[row.Npc.ID] = npcFromView(store.GetNpcViewRow(row))
	}
	for _, npcID := range npcIDs {
		if npc, found := npcs[npcID]; found {
			if err := outbox.writeNpc(npc); err != nil {
				return err
			}
		}
	}
	return nil
}

func (outbox *bridgeOutbox) writeNpc(npc Npc) error {
	// The payload reads only player fields; dropping Dm keeps the projection literal anyway.
	npc.Dm = nil
	siteURL, err := outbox.siteURL("/npcs/" + npc.ID.String())
	if err != nil {
		return err
	}
	return outbox.write(BridgeOutboxKindNpcUpsert, bridgeNpcTargetHead+npc.ID.String(), bridgeNpcFromProjection(npc, siteURL))
}

func bridgeNpcFromProjection(npc Npc, siteURL string) bridgeNpcPayload {
	payload := bridgeNpcPayload{
		DisplayName:    npc.DisplayName,
		Aliases:        npc.Aliases,
		Labels:         npc.Labels,
		Race:           npc.Race,
		CharacterClass: npc.CharacterClass,
		Age:            npc.Age,
		Level:          npc.Level,
		Appearance:     npc.Appearance,
		Personality:    npc.Personality,
		Backstory:      npc.Backstory,
		PlayerNotes:    npc.PlayerNotes,
		Status:         npc.Status,
		Health:         npc.Health,
		Relation:       npc.Relation,
		SiteURL:        siteURL,
	}
	if npc.CurrentPlace != nil {
		payload.CurrentPlace = &npc.CurrentPlace.Name
	}
	if npc.OriginPlace != nil {
		payload.OriginPlace = &npc.OriginPlace.Name
	}
	if npc.ColonyRole != nil {
		payload.ColonyRole = &npc.ColonyRole.Label
	}
	return payload
}

func (outbox *bridgeOutbox) npcWithdraw(npcID uuid.UUID) error {
	if !outbox.linked {
		return nil
	}
	return outbox.write(BridgeOutboxKindNpcWithdraw, bridgeNpcTargetHead+npcID.String(), bridgeWithdrawal{})
}

// revealedNpcsOfPlace are the NPCs whose Foundry entries name the place, read before a delete clears it.
func (outbox *bridgeOutbox) revealedNpcsOfPlace(placeID uuid.UUID) ([]uuid.UUID, error) {
	if !outbox.linked {
		return nil, nil
	}
	return outbox.queries.ListRevealedNpcIdsByPlace(outbox.requestContext, store.ListRevealedNpcIdsByPlaceParams{CampaignID: outbox.campaignID, PlaceID: &placeID})
}

func (outbox *bridgeOutbox) npcsOfPlace(placeID uuid.UUID) error {
	npcIDs, err := outbox.revealedNpcsOfPlace(placeID)
	if err != nil {
		return err
	}
	return outbox.npcUpserts(npcIDs)
}

func (outbox *bridgeOutbox) placeUpsert(placeID uuid.UUID) error {
	if !outbox.linked {
		return nil
	}
	row, err := outbox.queries.GetPlaceView(outbox.requestContext, store.GetPlaceViewParams{CampaignID: outbox.campaignID, ID: placeID, DmProjection: false})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return outbox.writePlace(toPlace(row))
}

func (outbox *bridgeOutbox) writePlace(place Place) error {
	// The payload reads only player fields; dropping Dm keeps the projection literal anyway.
	place.Dm = nil
	siteURL, err := outbox.siteURL("/places/" + place.ID.String())
	if err != nil {
		return err
	}
	return outbox.write(BridgeOutboxKindPlaceUpsert, bridgePlaceTargetHead+place.ID.String(), bridgePlacePayload{
		Name: place.Name, Description: place.Description, PlayerNotes: place.PlayerNotes, SiteURL: siteURL,
	})
}

func (outbox *bridgeOutbox) placeWithdraw(placeID uuid.UUID) error {
	if !outbox.linked {
		return nil
	}
	return outbox.write(BridgeOutboxKindPlaceWithdraw, bridgePlaceTargetHead+placeID.String(), bridgeWithdrawal{})
}

func (outbox *bridgeOutbox) questUpsert(questID uuid.UUID) error {
	return outbox.questUpserts([]uuid.UUID{questID})
}

// questUpserts upserts each quest in order with one projection read; any hidden or missing are skipped.
func (outbox *bridgeOutbox) questUpserts(questIDs []uuid.UUID) error {
	if !outbox.linked || len(questIDs) == 0 {
		return nil
	}
	rows, err := outbox.queries.ListRevealedQuestViews(outbox.requestContext, store.ListRevealedQuestViewsParams{CampaignID: outbox.campaignID, Ids: questIDs})
	if err != nil {
		return err
	}
	quests := make(map[uuid.UUID]Quest, len(rows))
	for _, row := range rows {
		quest, err := questFromView(store.GetQuestViewRow(row), false)
		if err != nil {
			return err
		}
		quests[row.Quest.ID] = quest
	}
	for _, questID := range questIDs {
		if quest, found := quests[questID]; found {
			if err := outbox.writeQuest(quest); err != nil {
				return err
			}
		}
	}
	return nil
}

func (outbox *bridgeOutbox) writeQuest(quest Quest) error {
	siteURL, err := outbox.siteURL("/quests/" + quest.ID.String())
	if err != nil {
		return err
	}
	colony, err := outbox.colonyLabels()
	if err != nil {
		return err
	}
	payload := bridgeQuestPayload{
		Title:       quest.Title,
		Description: quest.Description,
		Kind:        quest.Kind,
		Status:      quest.Status,
		Reward:      appliedEffectsText(quest.RewardEffects, colony.order, colony.labels),
		RewardFavor: quest.RewardFavor,
		Loot:        quest.Loot,
		ResolvedDay: quest.ResolvedDay,
		SiteURL:     siteURL,
	}
	if quest.GiverNpc != nil {
		payload.Giver = &quest.GiverNpc.DisplayName
	}
	if quest.Patron != nil {
		payload.Patron = &quest.Patron.Name
	}
	return outbox.write(BridgeOutboxKindQuestUpsert, bridgeQuestTargetHead+quest.ID.String(), payload)
}

func (outbox *bridgeOutbox) questWithdraw(questID uuid.UUID) error {
	if !outbox.linked {
		return nil
	}
	return outbox.write(BridgeOutboxKindQuestWithdraw, bridgeQuestTargetHead+questID.String(), bridgeWithdrawal{})
}

// revealedQuestsOfNpc are the quests whose Foundry entries name the NPC as their giver, read before a
// delete clears the giver.
func (outbox *bridgeOutbox) revealedQuestsOfNpc(npcID uuid.UUID) ([]uuid.UUID, error) {
	if !outbox.linked {
		return nil, nil
	}
	return outbox.queries.ListRevealedQuestIdsByGiver(outbox.requestContext, store.ListRevealedQuestIdsByGiverParams{CampaignID: outbox.campaignID, NpcID: &npcID})
}

func (outbox *bridgeOutbox) questsOfNpc(npcID uuid.UUID) error {
	questIDs, err := outbox.revealedQuestsOfNpc(npcID)
	if err != nil {
		return err
	}
	return outbox.questUpserts(questIDs)
}

func (outbox *bridgeOutbox) questsOfPatron(patronID uuid.UUID) error {
	if !outbox.linked {
		return nil
	}
	questIDs, err := outbox.queries.ListRevealedQuestIdsByPatron(outbox.requestContext, store.ListRevealedQuestIdsByPatronParams{CampaignID: outbox.campaignID, PatronID: &patronID})
	if err != nil {
		return err
	}
	return outbox.questUpserts(questIDs)
}

// backfill shows a newly linked world every revealed place, NPC and quest, in their lists' order;
// finish inserts them in one statement.
func (outbox *bridgeOutbox) backfill() error {
	if !outbox.linked {
		return nil
	}
	places, err := outbox.queries.ListPlaces(outbox.requestContext, store.ListPlacesParams{CampaignID: outbox.campaignID, DmProjection: false})
	if err != nil {
		return err
	}
	for _, row := range places {
		if err := outbox.writePlace(toPlace(store.GetPlaceViewRow(row))); err != nil {
			return err
		}
	}
	npcs, err := outbox.queries.ListNpcs(outbox.requestContext, store.ListNpcsParams{CampaignID: outbox.campaignID, DmProjection: false, ResultLimit: NpcLimit})
	if err != nil {
		return err
	}
	for _, row := range npcs {
		if err := outbox.writeNpc(npcFromView(store.GetNpcViewRow(row))); err != nil {
			return err
		}
	}
	quests, err := outbox.queries.ListQuests(outbox.requestContext, store.ListQuestsParams{CampaignID: outbox.campaignID, DmProjection: false})
	if err != nil {
		return err
	}
	for _, row := range quests {
		quest, err := questFromView(store.GetQuestViewRow(row), false)
		if err != nil {
			return err
		}
		if err := outbox.writeQuest(quest); err != nil {
			return err
		}
	}
	return nil
}

// specialItemLines are an order's lines that land in Foundry: neither supplies nor colonists, which
// the colony's own books receive.
func specialItemLines(lines []MarketOrderLine) []MarketOrderLine {
	special := []MarketOrderLine{}
	for _, line := range lines {
		if len(line.Resources) == 0 && line.Colonists == 0 {
			special = append(special, line)
		}
	}
	return special
}

// delivery writes a paid order's DELIVERY, keyed by the payment's timeline group, so an order paid,
// undone and paid again is two deliveries. Each line's Foundry item is its catalog item's UUID now.
func (outbox *bridgeOutbox) delivery(colony colonySnapshot, orderID uuid.UUID) error {
	if !outbox.linked {
		return nil
	}
	order, err := outbox.queries.GetMarketOrder(outbox.requestContext, store.GetMarketOrderParams{CampaignID: outbox.campaignID, ID: orderID})
	if err != nil {
		return err
	}
	if order.PaidGroupID == nil || order.PaidDay == nil {
		return errors.New("core: a delivery needs a paid order")
	}
	lines, err := listOrderLines(outbox.requestContext, outbox.queries, outbox.campaignID, []uuid.UUID{orderID})
	if err != nil {
		return err
	}
	special := specialItemLines(lines[orderID])
	itemIDs := make([]uuid.UUID, 0, len(special))
	for _, line := range special {
		itemIDs = append(itemIDs, line.ItemID)
	}
	compendiumUUIDs := map[uuid.UUID]string{}
	if len(itemIDs) > 0 {
		rows, err := outbox.queries.ListMarketItemCompendiumUuids(outbox.requestContext, store.ListMarketItemCompendiumUuidsParams{CampaignID: outbox.campaignID, ItemIds: itemIDs})
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row.CompendiumUuid != nil {
				compendiumUUIDs[row.ID] = *row.CompendiumUuid
			}
		}
	}
	payload := bridgeDeliveryPayload{
		PaidDay:   int64(*order.PaidDay),
		TotalGold: int64(order.TotalGold),
		Summary:   orderSummaryText(colony, lines[orderID]),
		Items:     make([]bridgeDeliveryItem, 0, len(special)),
	}
	for _, line := range special {
		item := bridgeDeliveryItem{Name: line.Name, Quantity: line.Quantity}
		if compendiumUUID, found := compendiumUUIDs[line.ItemID]; found {
			item.CompendiumUUID = &compendiumUUID
		}
		payload.Items = append(payload.Items, item)
	}
	return outbox.write(BridgeOutboxKindDelivery, bridgeDeliveryTargetHead+order.PaidGroupID.String(), payload)
}

// undoDelivery answers an undone payment: a DELIVERY Foundry hasn't acknowledged is taken back, linked
// or not, so a later link never delivers it, and the GMs always get DELIVERY_UNDONE, since a module may
// have fetched the DELIVERY before the undo and still apply it.
func (outbox *bridgeOutbox) undoDelivery(colony colonySnapshot, paidGroupID uuid.UUID, orderID uuid.UUID) error {
	withdrawn, err := outbox.queries.DeleteUnappliedBridgeOutboxTarget(outbox.requestContext, store.DeleteUnappliedBridgeOutboxTargetParams{
		CampaignID: outbox.campaignID, TargetKey: bridgeDeliveryTargetHead + paidGroupID.String(),
	})
	if err != nil {
		return err
	}
	outbox.tookBack = outbox.tookBack || withdrawn > 0
	if !outbox.linked {
		return nil
	}
	lines, err := listOrderLines(outbox.requestContext, outbox.queries, outbox.campaignID, []uuid.UUID{orderID})
	if err != nil {
		return err
	}
	special := specialItemLines(lines[orderID])
	payload := bridgeDeliveryUndonePayload{Summary: orderSummaryText(colony, lines[orderID]), Items: make([]bridgeDeliveryUndoneItem, 0, len(special))}
	for _, line := range special {
		payload.Items = append(payload.Items, bridgeDeliveryUndoneItem{Name: line.Name, Quantity: line.Quantity})
	}
	return outbox.write(BridgeOutboxKindDeliveryUndone, bridgeDeliveryUndoneTargetHead+paidGroupID.String(), payload)
}

func (outbox *bridgeOutbox) pollPosted(poll Poll) error {
	if !outbox.linked {
		return nil
	}
	pollsURL, err := outbox.siteURL("/polls")
	if err != nil {
		return err
	}
	payload := bridgePollPostedPayload{
		Question:      poll.Question,
		Options:       make([]string, 0, len(poll.Options)),
		AllowMultiple: poll.AllowMultiple,
		Voting:        poll.Voting,
		ClosesAt:      poll.ClosesAt.UTC(),
		PollsURL:      pollsURL,
	}
	for _, option := range poll.Options {
		payload.Options = append(payload.Options, option.Label)
	}
	return outbox.write(BridgeOutboxKindPollPosted, bridgePollTargetHead+poll.ID.String()+bridgePollPostedTargetTail, payload)
}

// pollClosed carries the counts the POLL_CLOSED entry wrote, other votes included, and names no winner.
func (outbox *bridgeOutbox) pollClosed(pollID uuid.UUID, question string, closedAt time.Time, counts []store.CountPollVotesByOptionRow) error {
	if !outbox.linked {
		return nil
	}
	payload := bridgePollClosedPayload{Question: question, ClosedAt: closedAt.UTC(), Options: make([]bridgePollClosedOption, 0, len(counts))}
	for _, count := range counts {
		payload.Options = append(payload.Options, bridgePollClosedOption{Label: count.Label, Count: count.Votes, OtherVotes: int64(count.DiscordOtherVotes)})
	}
	return outbox.write(BridgeOutboxKindPollClosed, bridgePollTargetHead+pollID.String()+bridgePollClosedTargetTail, payload)
}

// withdrawPollRows takes back a deleted poll's POLL_POSTED and POLL_CLOSED that Foundry hasn't read
// yet, so it never posts a poll that no longer exists, or its close.
func (outbox *bridgeOutbox) withdrawPollRows(pollID uuid.UUID) error {
	withdrawn, err := outbox.queries.DeleteUnappliedBridgeOutboxTargets(outbox.requestContext, store.DeleteUnappliedBridgeOutboxTargetsParams{
		CampaignID: outbox.campaignID,
		TargetKeys: []string{
			bridgePollTargetHead + pollID.String() + bridgePollPostedTargetTail,
			bridgePollTargetHead + pollID.String() + bridgePollClosedTargetTail,
		},
	})
	outbox.tookBack = outbox.tookBack || withdrawn > 0
	return err
}
