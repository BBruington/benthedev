package core_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

const testActorID = "abcdefghij123456"

func inboundEvent(kind core.BridgeEventKind, payload string) core.BridgeInboundEvent {
	return core.BridgeInboundEvent{ID: uuid.New(), Kind: kind, OccurredAt: time.Now().UTC(), Payload: json.RawMessage(payload)}
}

func coinDeposit(gold int64) core.BridgeInboundEvent {
	payload, _ := json.Marshal(map[string]any{"gold": gold, "actorId": testActorID, "actorName": "The Wayfarers"})
	return inboundEvent(core.BridgeEventKindCoinDeposit, string(payload))
}

// record sends events of distinct kinds through the bridge and returns the inbox item each made, by
// kind: items received at one instant list in ID order, not in the order they were sent.
func (test *colonyTest) record(caller core.BridgeCaller, events ...core.BridgeInboundEvent) map[core.BridgeEventKind]core.BridgeInboxItem {
	test.t.Helper()
	known := map[uuid.UUID]bool{}
	for _, item := range test.bridgeStatus().Inbox {
		known[item.ID] = true
	}
	if _, err := test.service.RecordBridgeEvents(test.context, caller, core.BridgeEventsInput{Events: events}); err != nil {
		test.t.Fatalf("record: %v", err)
	}
	made := map[core.BridgeEventKind]core.BridgeInboxItem{}
	for _, item := range test.bridgeStatus().Inbox {
		if !known[item.ID] {
			made[item.Kind] = item
		}
	}
	if len(made) != len(events) {
		test.t.Fatalf("recorded %d events, made inbox items %+v", len(events), made)
	}
	return made
}

func (test *colonyTest) setGoldMax(maxAmount int64) {
	test.t.Helper()
	if _, err := test.database.Pool.Exec(test.context, `UPDATE resource SET max_amount = $2 WHERE campaign_id = $1 AND key = 'gold'`, test.campaignID, maxAmount); err != nil {
		test.t.Fatal(err)
	}
}

func requireFieldError(t *testing.T, step string, err error, field string) {
	t.Helper()
	var operationError *core.Error
	if !errors.As(err, &operationError) || operationError.Kind != core.KindInvalid || len(operationError.Fields) == 0 || operationError.Fields[0].Field != field {
		t.Fatalf("%s: %v, want a %s field error", step, err, field)
	}
}

func TestImportEventsAreCheckedAndStoredCleaned(t *testing.T) {
	test := newColonyTest(t)
	_, caller := test.pairBridge()
	items := test.record(caller,
		inboundEvent(core.BridgeEventKindCoinDeposit, `{"gold":40,"actorId":"abcdefghij123456","actorName":"  The Wayfarers  ","extra":true}`),
		inboundEvent(core.BridgeEventKindNpcImport, `{"foundryUuid":"Compendium.pf2e.npcs.Actor.abcdefghij123456","name":" Brother Aldo ","level":3,
			"race":"Human","characterClass":"Cleric","appearance":"Tall.\r\nGrey robes.","dmNotes":"Spy."}`),
		inboundEvent(core.BridgeEventKindItemImport, `{"foundryUuid":"Item.abcdefghij123456","name":"Boots of Striding","description":"Quick.",
			"priceGold":null,"compendiumUuid":""}`),
	)
	if len(items) != 3 {
		t.Fatalf("inbox = %+v", items)
	}
	deposit, npc, item := items[core.BridgeEventKindCoinDeposit], items[core.BridgeEventKindNpcImport], items[core.BridgeEventKindItemImport]
	if deposit.Kind != core.BridgeEventKindCoinDeposit || deposit.Deposit == nil || deposit.Deposit.Gold != 40 || deposit.Deposit.ActorName != "The Wayfarers" ||
		deposit.Npc != nil || deposit.Item != nil || deposit.Days != 0 {
		t.Fatalf("deposit = %+v", deposit)
	}
	if npc.Npc == nil || npc.Npc.Name != "Brother Aldo" || npc.Npc.Level == nil || *npc.Npc.Level != 3 || npc.Npc.Appearance != "Tall.\nGrey robes." ||
		npc.Npc.DmNotes != "Spy." || npc.Npc.FoundryUUID != "Compendium.pf2e.npcs.Actor.abcdefghij123456" {
		t.Fatalf("npc = %+v", npc.Npc)
	}
	if item.Item == nil || item.Item.PriceGold != nil || item.Item.CompendiumUUID != nil || item.Item.Name != "Boots of Striding" {
		t.Fatalf("item = %+v", item.Item)
	}
	if board := test.board(); len(board.Dm.BridgeInbox) != 3 {
		t.Fatalf("the DM's board inbox = %+v", board.Dm.BridgeInbox)
	}

	invalid := map[string]core.BridgeInboundEvent{
		"unknown kind":           inboundEvent("BOGUS", `{}`),
		"no gold":                inboundEvent(core.BridgeEventKindCoinDeposit, `{"actorId":"abcdefghij123456","actorName":"P"}`),
		"too much gold":          coinDeposit(core.BridgeDepositMaxGold + 1),
		"half a gold piece":      inboundEvent(core.BridgeEventKindCoinDeposit, `{"gold":1.5,"actorId":"abcdefghij123456","actorName":"P"}`),
		"short actor ID":         inboundEvent(core.BridgeEventKindCoinDeposit, `{"gold":1,"actorId":"abc","actorName":"P"}`),
		"blank actor name":       inboundEvent(core.BridgeEventKindCoinDeposit, `{"gold":1,"actorId":"abcdefghij123456","actorName":"  "}`),
		"long actor name":        inboundEvent(core.BridgeEventKindCoinDeposit, `{"gold":1,"actorId":"abcdefghij123456","actorName":"`+strings.Repeat("x", 101)+`"}`),
		"embedded actor":         inboundEvent(core.BridgeEventKindNpcImport, `{"foundryUuid":"Actor.abcdefghij123456.Item.abcdefghij123456","name":"A"}`),
		"npc without a name":     inboundEvent(core.BridgeEventKindNpcImport, `{"foundryUuid":"Actor.abcdefghij123456","name":""}`),
		"npc level too high":     inboundEvent(core.BridgeEventKindNpcImport, `{"foundryUuid":"Actor.abcdefghij123456","name":"A","level":101}`),
		"line break in a race":   inboundEvent(core.BridgeEventKindNpcImport, `{"foundryUuid":"Actor.abcdefghij123456","name":"A","race":"a\nb"}`),
		"long dm notes":          inboundEvent(core.BridgeEventKindNpcImport, `{"foundryUuid":"Actor.abcdefghij123456","name":"A","dmNotes":"`+strings.Repeat("x", 4001)+`"}`),
		"actor UUID on an item":  inboundEvent(core.BridgeEventKindItemImport, `{"foundryUuid":"Actor.abcdefghij123456","name":"A"}`),
		"negative price":         inboundEvent(core.BridgeEventKindItemImport, `{"foundryUuid":"Item.abcdefghij123456","name":"A","priceGold":-1}`),
		"world item compendium":  inboundEvent(core.BridgeEventKindItemImport, `{"foundryUuid":"Item.abcdefghij123456","name":"A","compendiumUuid":"Item.abcdefghij123456"}`),
		"a name that is numeric": inboundEvent(core.BridgeEventKindItemImport, `{"foundryUuid":"Item.abcdefghij123456","name":5}`),
	}
	for name, event := range invalid {
		if _, err := test.service.RecordBridgeEvents(test.context, caller, core.BridgeEventsInput{Events: []core.BridgeInboundEvent{event}}); !isKind(err, core.KindInvalid) {
			t.Errorf("%s: %v, want invalid", name, err)
		}
	}
	if inbox := test.bridgeStatus().Inbox; len(inbox) != 3 {
		t.Fatalf("a refused event reached the inbox: %+v", inbox)
	}
}

func TestAcceptingADepositAddsGoldAndUndoReopensIt(t *testing.T) {
	test := newColonyTest(t)
	_, caller := test.pairBridge()
	deposit := coinDeposit(40)
	itemID := test.record(caller, deposit)[core.BridgeEventKindCoinDeposit].ID
	test.drain(caller)

	if _, err := test.service.AcceptBridgeDeposit(test.context, test.player, itemID); !isKind(err, core.KindForbidden) {
		t.Fatalf("a player accepted: %v", err)
	}
	accepted := test.must(test.service.AcceptBridgeDeposit(test.context, test.dm, itemID))
	if test.amount("gold") != 40 {
		t.Fatalf("gold = %d after the deposit, want 40", test.amount("gold"))
	}
	if len(accepted.Entries) != 1 || accepted.Entries[0].Kind != core.TimelineKindCoinDeposited || accepted.Entries[0].Title != "Coins deposited · 40 gold" ||
		accepted.Entries[0].Dm.Visibility != core.TimelineVisibilityPublic || strings.Contains(accepted.Entries[0].Title+accepted.Entries[0].Body, "Wayfarers") {
		t.Fatalf("entries = %+v", accepted.Entries)
	}
	if len(accepted.Board.Dm.BridgeInbox) != 0 {
		t.Fatalf("inbox after accepting = %+v", accepted.Board.Dm.BridgeInbox)
	}
	requireRows(t, "accept", test.drain(caller))
	if _, err := test.service.AcceptBridgeDeposit(test.context, test.dm, itemID); !isKind(err, core.KindConflict) {
		t.Fatalf("accepting twice: %v", err)
	}
	if _, err := test.service.AcceptBridgeDeposit(test.context, test.dm, uuid.New()); !isKind(err, core.KindNotFound) {
		t.Fatalf("accepting a missing item: %v", err)
	}

	undone := test.undo(accepted)
	if test.amount("gold") != 0 {
		t.Fatalf("gold = %d after the undo, want 0", test.amount("gold"))
	}
	if inbox := undone.Board.Dm.BridgeInbox; len(inbox) != 1 || inbox[0].ID != itemID {
		t.Fatalf("the undo's inbox = %+v, want the deposit back", inbox)
	}
	rows := test.drain(caller)
	requireRows(t, "undo", rows, "DEPOSIT_UNDONE deposit-undone:"+accepted.GroupID.String())
	if rows[0].Kind != core.BridgeOutboxKindColonySummary {
		t.Fatalf("the undo's first row is %s, want the summary", rows[0].Kind)
	}
	var whisper map[string]any
	if err := json.Unmarshal(rows[len(rows)-1].Payload, &whisper); err != nil {
		t.Fatal(err)
	}
	if whisper["depositId"] != deposit.ID.String() || whisper["gold"] != float64(40) || whisper["actorName"] != "The Wayfarers" || len(whisper) != 3 {
		t.Fatalf("DEPOSIT_UNDONE payload = %v", whisper)
	}

	again := test.must(test.service.AcceptBridgeDeposit(test.context, test.dm, itemID))
	test.undo(again)
	requireRows(t, "a second undo", test.drain(caller), "DEPOSIT_UNDONE deposit-undone:"+again.GroupID.String())
}

func TestADepositPastGoldsMaxIsRefusedWhole(t *testing.T) {
	test := newColonyTest(t)
	_, caller := test.pairBridge()
	test.setGoldMax(500)
	test.setTo(map[string]int64{"gold": 470})
	itemID := test.record(caller, coinDeposit(40))[core.BridgeEventKindCoinDeposit].ID
	version := test.board().Version
	_, err := test.service.AcceptBridgeDeposit(test.context, test.dm, itemID)
	var operationError *core.Error
	if !errors.As(err, &operationError) || operationError.Kind != core.KindConflict ||
		operationError.Message != "Colony gold would pass its max of 500. Raise the max on Settings, or dismiss the deposit to return the coins." {
		t.Fatalf("a deposit past the max: %v", err)
	}
	if board := test.board(); board.Version != version || test.amount("gold") != 470 || len(board.Dm.BridgeInbox) != 1 {
		t.Fatalf("a refused deposit changed something: version %d (was %d), gold %d, inbox %+v", board.Version, version, test.amount("gold"), board.Dm.BridgeInbox)
	}
	test.setGoldMax(510)
	test.must(test.service.AcceptBridgeDeposit(test.context, test.dm, itemID))
	if test.amount("gold") != 510 {
		t.Fatalf("gold = %d, want 510", test.amount("gold"))
	}
}

func TestDismissingADepositRefundsItThroughALinkedWorld(t *testing.T) {
	test := newColonyTest(t)
	_, caller := test.pairBridge()
	deposit := coinDeposit(25)
	items := test.record(caller, deposit, inboundEvent(core.BridgeEventKindNpcImport, `{"foundryUuid":"Actor.abcdefghij123456","name":"Aldo"}`))
	depositID, npcID := items[core.BridgeEventKindCoinDeposit].ID, items[core.BridgeEventKindNpcImport].ID
	test.drain(caller)

	if err := test.service.RevokeBridge(test.context, test.dm); err != nil {
		t.Fatal(err)
	}
	if err := test.service.DismissBridgeInboxItem(test.context, test.dm, depositID); !isKind(err, core.KindConflict) {
		t.Fatalf("dismissing a deposit while unlinked: %v", err)
	}
	if err := test.service.DismissBridgeInboxItem(test.context, test.dm, npcID); err != nil {
		t.Fatalf("dismissing an import while unlinked: %v", err)
	}
	if inbox := test.bridgeStatus().Inbox; len(inbox) != 1 || inbox[0].ID != depositID {
		t.Fatalf("inbox = %+v, want only the deposit waiting", inbox)
	}

	_, caller = test.pairBridge()
	test.drain(caller)
	if err := test.service.DismissBridgeInboxItem(test.context, test.dm, depositID); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	rows := test.drain(caller)
	requireRows(t, "dismiss", rows, "DEPOSIT_REFUND deposit-refund:"+deposit.ID.String())
	var refund map[string]any
	if err := json.Unmarshal(rows[0].Payload, &refund); err != nil {
		t.Fatal(err)
	}
	if refund["depositId"] != deposit.ID.String() || refund["gold"] != float64(25) || refund["actorId"] != testActorID || refund["actorName"] != "The Wayfarers" {
		t.Fatalf("DEPOSIT_REFUND payload = %v", refund)
	}
	if err := test.service.DismissBridgeInboxItem(test.context, test.dm, depositID); !isKind(err, core.KindConflict) {
		t.Fatalf("dismissing twice: %v", err)
	}
	if test.amount("gold") != 0 {
		t.Fatal("a dismissed deposit changed gold")
	}
}

func TestCreatesAcceptImportsInTheirTransaction(t *testing.T) {
	test := newColonyTest(t)
	_, caller := test.pairBridge()
	items := test.record(caller,
		inboundEvent(core.BridgeEventKindNpcImport, `{"foundryUuid":"Actor.abcdefghij123456","name":"Aldo"}`),
		inboundEvent(core.BridgeEventKindItemImport, `{"foundryUuid":"Item.abcdefghij123456","name":"Boots","priceGold":12}`),
		timeAdvanced(2),
	)
	npcItem, marketItem, advanceItem := items[core.BridgeEventKindNpcImport].ID, items[core.BridgeEventKindItemImport].ID, items[core.BridgeEventKindTimeAdvanced].ID

	// The form's own validation runs first, and a refused save leaves the item waiting.
	if _, err := test.service.CreateNpc(test.context, test.dm, core.NewNpcInput{BridgeEventID: &npcItem}); !isKind(err, core.KindInvalid) {
		t.Fatalf("a nameless NPC: %v", err)
	}
	_, err := test.service.CreateNpc(test.context, test.dm, core.NewNpcInput{NpcInput: core.NpcInput{Name: "Aldo"}, BridgeEventID: &marketItem})
	requireFieldError(t, "an NPC answering an item import", err, "bridgeEventId")
	missing := uuid.New()
	_, err = test.service.CreateMarketItem(test.context, test.dm, core.NewMarketItemInput{MarketItemInput: core.MarketItemInput{Name: "Boots"}, BridgeEventID: &missing})
	requireFieldError(t, "an item answering a missing import", err, "bridgeEventId")
	_, err = test.advanceBy(2, &npcItem)
	requireFieldError(t, "an advance answering an NPC import", err, "bridgeEventId")
	if _, err := test.service.AcceptBridgeDeposit(test.context, test.dm, advanceItem); !isKind(err, core.KindConflict) {
		t.Fatalf("accepting a time advance as a deposit: %v", err)
	}
	if inbox := test.bridgeStatus().Inbox; len(inbox) != 3 {
		t.Fatalf("a refused save accepted its item: %+v", inbox)
	}
	if npcs, err := test.service.ListNpcs(test.context, test.dm, core.NpcFilter{}); err != nil || len(npcs.Npcs) != 0 {
		t.Fatalf("a refused create wrote an NPC: %+v, %v", npcs.Npcs, err)
	}

	test.drain(caller)
	npc, err := test.service.CreateNpc(test.context, test.dm, core.NewNpcInput{NpcInput: core.NpcInput{Name: "Brother Aldo"}, BridgeEventID: &npcItem})
	if err != nil || npc.Dm == nil || npc.Dm.Revealed {
		t.Fatalf("import as NPC: %+v, %v; want a hidden NPC", npc, err)
	}
	requireRows(t, "a hidden import", test.drain(caller))
	if _, err := test.service.CreateNpc(test.context, test.dm, core.NewNpcInput{NpcInput: core.NpcInput{Name: "Aldo again"}, BridgeEventID: &npcItem}); !isKind(err, core.KindConflict) {
		t.Fatalf("importing twice: %v", err)
	}
	if _, err := test.service.CreateMarketItem(test.context, test.dm, core.NewMarketItemInput{MarketItemInput: core.MarketItemInput{Name: "Boots", PriceGold: 12}, BridgeEventID: &marketItem}); err != nil {
		t.Fatalf("add to shipments: %v", err)
	}
	requireRows(t, "a catalog import", test.drain(caller))
	if inbox := test.bridgeStatus().Inbox; len(inbox) != 1 || inbox[0].ID != advanceItem {
		t.Fatalf("inbox = %+v, want only the time advance", inbox)
	}
}

func TestPruningKeepsTheItemTheUndoSlotNames(t *testing.T) {
	test := newColonyTest(t)
	_, caller := test.pairBridge()
	itemID := test.record(caller, coinDeposit(10))[core.BridgeEventKindCoinDeposit].ID
	accepted := test.must(test.service.AcceptBridgeDeposit(test.context, test.dm, itemID))
	test.clock.advance(31 * 24 * time.Hour)
	test.record(caller, timeAdvanced(1))

	undone := test.undo(accepted)
	if inbox := undone.Board.Dm.BridgeInbox; len(inbox) != 2 || !slices.ContainsFunc(inbox, func(item core.BridgeInboxItem) bool { return item.ID == itemID }) {
		t.Fatalf("inbox after an undo 31 days on = %+v, want the deposit back", inbox)
	}
}

// fillDecidedStatusEvents records count decided STATUS rows received now, enough to make the trim drop the
// oldest decided items.
func (test *colonyTest) fillDecidedStatusEvents(count int) {
	test.t.Helper()
	if _, err := test.database.Pool.Exec(test.context, `INSERT INTO bridge_event (campaign_id, module_event_id, kind, status, occurred_at, received_at)
		SELECT $1, gen_random_uuid(), 'STATUS', 'ACCEPTED', $2, $2 FROM generate_series(1, $3::integer)`,
		test.campaignID, test.clock.Now(), count); err != nil {
		test.t.Fatal(err)
	}
}

func (test *colonyTest) countBridgeEvents(kind core.BridgeEventKind) int {
	test.t.Helper()
	var count int
	if err := test.database.Pool.QueryRow(test.context, `SELECT count(*) FROM bridge_event WHERE campaign_id = $1 AND kind = $2`,
		test.campaignID, kind).Scan(&count); err != nil {
		test.t.Fatal(err)
	}
	return count
}

func TestADecidedDepositIsNeverForgotten(t *testing.T) {
	test := newColonyTest(t)
	_, caller := test.pairBridge()
	accepted, dismissed := coinDeposit(10), coinDeposit(20)
	acceptedID := test.record(caller, accepted)[core.BridgeEventKindCoinDeposit].ID
	dismissedID := test.record(caller, dismissed)[core.BridgeEventKindCoinDeposit].ID
	test.must(test.service.AcceptBridgeDeposit(test.context, test.dm, acceptedID))
	test.advance(1)
	if err := test.service.DismissBridgeInboxItem(test.context, test.dm, dismissedID); err != nil {
		t.Fatalf("dismiss: %v", err)
	}

	test.clock.advance(31 * 24 * time.Hour)
	test.fillDecidedStatusEvents(10001)
	test.record(caller, timeAdvanced(1))
	if kept := test.countBridgeEvents(core.BridgeEventKindStatus); kept != 10000 {
		t.Fatalf("the trim kept %d STATUS rows, want 10000", kept)
	}

	replayed, err := test.service.RecordBridgeEvents(test.context, caller, core.BridgeEventsInput{Events: []core.BridgeInboundEvent{accepted, dismissed}})
	if err != nil || replayed.Duplicates != 2 || replayed.Accepted != 0 {
		t.Fatalf("replaying two decided deposits after the prune and the trim = %+v, %v; want two duplicates", replayed, err)
	}
	if inbox := test.bridgeStatus().Inbox; len(inbox) != 1 || inbox[0].Kind != core.BridgeEventKindTimeAdvanced {
		t.Fatalf("inbox = %+v, want only the time advance", inbox)
	}
}

func TestTrimmingKeepsTheItemTheUndoSlotNames(t *testing.T) {
	test := newColonyTest(t)
	_, caller := test.pairBridge()
	itemID := test.record(caller, timeAdvanced(2))[core.BridgeEventKindTimeAdvanced].ID
	accepted := test.must(test.advanceBy(2, &itemID))
	test.clock.advance(time.Hour)
	test.fillDecidedStatusEvents(10001)
	test.record(caller, timeAdvanced(1))
	if kept := test.countBridgeEvents(core.BridgeEventKindStatus); kept != 10000 {
		t.Fatalf("the trim kept %d STATUS rows, want 10000", kept)
	}

	undone := test.undo(accepted)
	if inbox := undone.Board.Dm.BridgeInbox; len(inbox) != 2 || !slices.ContainsFunc(inbox, func(item core.BridgeInboxItem) bool { return item.ID == itemID }) {
		t.Fatalf("inbox after an undo past the trim = %+v, want the time advance back", inbox)
	}
}

func TestDismissingAnUndoneDepositRefundsIt(t *testing.T) {
	test := newColonyTest(t)
	_, caller := test.pairBridge()
	deposit := coinDeposit(30)
	itemID := test.record(caller, deposit)[core.BridgeEventKindCoinDeposit].ID
	test.undo(test.must(test.service.AcceptBridgeDeposit(test.context, test.dm, itemID)))
	test.drain(caller)

	if err := test.service.DismissBridgeInboxItem(test.context, test.dm, itemID); err != nil {
		t.Fatalf("dismissing an undone deposit: %v", err)
	}
	requireRows(t, "dismiss after undo", test.drain(caller), "DEPOSIT_REFUND deposit-refund:"+deposit.ID.String())
	var status string
	if err := test.database.Pool.QueryRow(test.context, `SELECT status FROM bridge_event WHERE campaign_id = $1 AND id = $2`,
		test.campaignID, itemID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "DISMISSED" || test.amount("gold") != 0 || len(test.bridgeStatus().Inbox) != 0 {
		t.Fatalf("after dismiss: status %s, gold %d, inbox %+v; want DISMISSED, 0 and empty", status, test.amount("gold"), test.bridgeStatus().Inbox)
	}
}
