package core_test

import (
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

const bridgeTestOrigin = "https://sampleworld.forge-vtt.com"

func (test *colonyTest) pairingCode() string {
	test.t.Helper()
	code, err := test.service.CreateBridgePairingCode(test.context, test.dm)
	if err != nil {
		test.t.Fatalf("make a pairing code: %v", err)
	}
	return code.Code
}

// pairBridge links the campaign to a Foundry world and returns the BridgeCaller its token enters as.
func (test *colonyTest) pairBridge() (string, core.BridgeCaller) {
	test.t.Helper()
	pairing, err := test.service.PairBridge(test.context, bridgeTestOrigin, core.BridgePairInput{Code: test.pairingCode()})
	if err != nil {
		test.t.Fatalf("pair: %v", err)
	}
	caller, err := test.service.EnterBridge(test.context, pairing.Token)
	if err != nil {
		test.t.Fatalf("enter the bridge: %v", err)
	}
	return pairing.Token, caller
}

func (test *colonyTest) outbox(caller core.BridgeCaller, after int64) []core.BridgeOutboxRow {
	test.t.Helper()
	page, err := test.service.ReadBridgeOutbox(test.context, caller, after)
	if err != nil {
		test.t.Fatalf("read the outbox: %v", err)
	}
	return page.Rows
}

func (test *colonyTest) bridgeStatus() core.BridgeStatus {
	test.t.Helper()
	status, err := test.service.GetBridgeStatus(test.context, test.dm)
	if err != nil {
		test.t.Fatalf("read the bridge status: %v", err)
	}
	return status
}

func (test *colonyTest) advanceBy(days int64, bridgeEventID *uuid.UUID) (core.ColonyChange, error) {
	return test.service.Advance(test.context, test.dm, core.AdvanceInput{Days: days, ExpectedVersion: test.board().Version, BridgeEventID: bridgeEventID})
}

func timeAdvanced(days int64) core.BridgeInboundEvent {
	payload, _ := json.Marshal(map[string]int64{"days": days})
	return core.BridgeInboundEvent{ID: uuid.New(), Kind: core.BridgeEventKindTimeAdvanced, OccurredAt: time.Now().UTC(), Payload: payload}
}

func TestPairingCodesAreSingleUseShortLivedAndLimited(t *testing.T) {
	test := newColonyTest(t)
	first := test.pairingCode()
	if len(first) != 7 || first[3] != '-' {
		t.Fatalf("code = %q, want XXX-XXX", first)
	}
	if status := test.bridgeStatus(); status.Code == nil || status.Link != nil || status.ModuleManifestURL != testPublicURL+"/foundry/module.json" {
		t.Fatalf("status with an open code = %+v", status)
	}
	second := test.pairingCode()
	if _, err := test.service.PairBridge(test.context, bridgeTestOrigin, core.BridgePairInput{Code: first}); !errors.Is(err, core.ErrBridgeCodeRejected) {
		t.Fatalf("a replaced code paired: %v", err)
	}
	// Typed in lower case and without the dash, it still works once.
	if _, err := test.service.PairBridge(test.context, bridgeTestOrigin, core.BridgePairInput{Code: strings.ToLower(strings.ReplaceAll(second, "-", ""))}); err != nil {
		t.Fatalf("pair with the open code: %v", err)
	}
	if _, err := test.service.PairBridge(test.context, bridgeTestOrigin, core.BridgePairInput{Code: second}); !errors.Is(err, core.ErrBridgeCodeRejected) {
		t.Fatalf("a used code paired again: %v", err)
	}
	expiring := test.pairingCode()
	test.clock.advance(core.BridgePairingCodeLifetime)
	if _, err := test.service.PairBridge(test.context, bridgeTestOrigin, core.BridgePairInput{Code: expiring}); !errors.Is(err, core.ErrBridgeCodeRejected) {
		t.Fatalf("an expired code paired: %v", err)
	}
	if status := test.bridgeStatus(); status.Code != nil {
		t.Fatalf("an expired code still shows: %+v", status.Code)
	}
	test.pairingCode()
	test.pairingCode()
	if _, err := test.service.CreateBridgePairingCode(test.context, test.dm); !isKind(err, core.KindConflict) {
		t.Fatalf("a sixth code in an hour: %v", err)
	}
	test.clock.advance(time.Hour)
	test.pairingCode()
	if _, err := test.service.CreateBridgePairingCode(test.context, test.player); !isKind(err, core.KindForbidden) {
		t.Fatalf("a player made a code: %v", err)
	}
}

func TestPairingChecksTheOrigin(t *testing.T) {
	test := newColonyTest(t)
	code := test.pairingCode()
	for _, origin := range []string{"", "http://sampleworld.forge-vtt.com", "https://forge-vtt.com", "https://sampleworld.forge-vtt.com.evil.example",
		"https://a.b.forge-vtt.com", "https://SampleWorld.forge-vtt.com", "http://localhost:30000", "null"} {
		if _, err := test.service.PairBridge(test.context, origin, core.BridgePairInput{Code: code}); !errors.Is(err, core.ErrBridgeOrigin) {
			t.Errorf("origin %q: %v, want ErrBridgeOrigin", origin, err)
		}
	}
	// A refused origin leaves the code working; a local Foundry pairs only where the option allows it.
	local := core.New(core.Options{Pool: test.database.Pool, Logger: slog.New(slog.DiscardHandler), Now: test.clock.Now, PublicURL: testPublicURL, AllowLocalFoundryOrigin: true})
	if !local.BridgeOriginAllowed("http://localhost:30000") || test.service.BridgeOriginAllowed("http://localhost:30000") {
		t.Fatal("http://localhost:30000 must be allowed only with AllowLocalFoundryOrigin")
	}
	pairing, err := local.PairBridge(test.context, "http://localhost:30000", core.BridgePairInput{Code: code})
	if err != nil || pairing.CampaignName != "Sample" {
		t.Fatalf("pair from a local Foundry: %+v, %v", pairing, err)
	}
}

// Disconnect closes the open code before revoking the link, the order a pairing claims them in, so the
// two can't deadlock (40P01) when the DM disconnects while Foundry pairs.
func TestDisconnectWaitsForAPairingWithoutDeadlock(t *testing.T) {
	test := newColonyTest(t)
	test.pairBridge()
	test.pairingCode()
	pairing, err := test.database.Pool.Begin(test.context)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pairing.Rollback(test.context) }()
	if _, err := pairing.Exec(test.context, `UPDATE bridge_pairing_code SET closed_at = now() WHERE campaign_id = $1 AND closed_at IS NULL`, test.campaignID); err != nil {
		t.Fatalf("claim the code: %v", err)
	}
	revoked := make(chan error, 1)
	go func() {
		revoked <- test.service.RevokeBridge(test.context, test.dm)
	}()
	select {
	case err := <-revoked:
		t.Fatalf("the disconnect didn't wait for the pairing: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if _, err := pairing.Exec(test.context, `UPDATE bridge_link SET revoked_at = now() WHERE campaign_id = $1 AND revoked_at IS NULL`, test.campaignID); err != nil {
		t.Fatalf("the pairing's revoke of the old link: %v", err)
	}
	if err := pairing.Commit(test.context); err != nil {
		t.Fatal(err)
	}
	if err := <-revoked; err != nil {
		t.Fatalf("disconnect: %v", err)
	}
}

func TestEnteringTheBridgeNeedsALiveToken(t *testing.T) {
	test := newColonyTest(t)
	token, caller := test.pairBridge()
	if caller.CampaignID() != test.campaignID || caller.Origin() != bridgeTestOrigin {
		t.Fatalf("caller = %+v", caller)
	}
	status := test.bridgeStatus()
	if status.Link == nil || status.Link.LastContactOn == nil || *status.Link.LastContactOn != "2026-10-11" || status.Link.Origin != bridgeTestOrigin {
		t.Fatalf("link after the first contact = %+v", status.Link)
	}
	test.clock.advance(24 * time.Hour)
	if _, err := test.service.EnterBridge(test.context, token); err != nil {
		t.Fatalf("enter the next day: %v", err)
	}
	if lastContact := test.bridgeStatus().Link.LastContactOn; lastContact == nil || *lastContact != "2026-10-12" {
		t.Fatalf("last contact the next day = %v", lastContact)
	}
	for _, bad := range []string{"", "short", token + "x", strings.Repeat("A", 43), " " + token} {
		if _, err := test.service.EnterBridge(test.context, bad); !errors.Is(err, core.ErrBridgeUnauthorized) {
			t.Errorf("token %q: %v", bad, err)
		}
	}
	if _, err := test.service.ReadBridgeOutbox(test.context, core.BridgeCaller{}, 0); !errors.Is(err, core.ErrBridgeUnauthorized) {
		t.Errorf("the zero BridgeCaller read the outbox: %v", err)
	}

	if _, err := test.service.SuspendCampaign(test.context, test.owner, test.campaignID); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if _, err := test.service.EnterBridge(test.context, token); !errors.Is(err, core.ErrBridgeUnauthorized) {
		t.Fatalf("a suspended campaign's token: %v", err)
	}
	if _, err := test.service.UnsuspendCampaign(test.context, test.owner, test.campaignID); err != nil {
		t.Fatalf("unsuspend: %v", err)
	}
	newToken, _ := test.pairBridge()
	if _, err := test.service.EnterBridge(test.context, token); !errors.Is(err, core.ErrBridgeUnauthorized) {
		t.Fatalf("a replaced link's token: %v", err)
	}
	if err := test.service.RevokeBridge(test.context, test.dm); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := test.service.EnterBridge(test.context, newToken); !errors.Is(err, core.ErrBridgeUnauthorized) {
		t.Fatalf("a revoked token: %v", err)
	}
	if status := test.bridgeStatus(); status.Link != nil || status.Code != nil {
		t.Fatalf("status after revoking = %+v", status)
	}
}

func TestTheOutboxFollowsColonyChanges(t *testing.T) {
	test := newColonyTest(t)
	// Without a link nothing is queued.
	test.must(test.advanceBy(1, nil))
	_, caller := test.pairBridge()
	rows := test.outbox(caller, 0)
	if len(rows) != 1 || rows[0].Kind != core.BridgeOutboxKindColonySummary || rows[0].TargetKey != "colony" {
		t.Fatalf("rows after pairing = %+v", rows)
	}
	var summary map[string]any
	if err := json.Unmarshal(rows[0].Payload, &summary); err != nil {
		t.Fatalf("decode the summary: %v", err)
	}
	if summary["campaignName"] != "Sample" || summary["day"] != float64(1) || summary["siteUrl"] != testPublicURL+"/c/sample" {
		t.Fatalf("summary = %v", summary)
	}
	for _, key := range []string{"resources", "workforce", "nextShipmentDay", "nextGoodsShipDay"} {
		if _, found := summary[key]; !found {
			t.Errorf("the summary lacks %s: %v", key, summary)
		}
	}
	// No order is at sea, so there is no goods ship to wait for.
	if summary["nextGoodsShipDay"] != nil {
		t.Errorf("nextGoodsShipDay = %v with no open order, want null", summary["nextGoodsShipDay"])
	}
	pairSeq := rows[0].Seq

	advanced := test.must(test.advanceBy(3, nil))
	rows = test.outbox(caller, 0)
	if len(rows) != 2 || rows[0].Kind != core.BridgeOutboxKindDaysAdvanced || rows[1].Kind != core.BridgeOutboxKindColonySummary {
		t.Fatalf("rows after an advance = %+v", rows)
	}
	var daysAdvanced map[string]int64
	_ = json.Unmarshal(rows[0].Payload, &daysAdvanced)
	if rows[0].Seq <= pairSeq || rows[0].TargetKey != "advance:"+jsonNumber(rows[0].Seq) ||
		daysAdvanced["fromDay"] != 1 || daysAdvanced["toDay"] != 4 || daysAdvanced["days"] != 3 {
		t.Fatalf("the DAYS_ADVANCED row = %+v %s", rows[0], rows[0].Payload)
	}
	if !strings.Contains(string(rows[1].Payload), `"day": 4`) {
		t.Fatalf("the summary didn't follow the advance: %s", rows[1].Payload)
	}

	// The undo queues the state again, never a second DAYS_ADVANCED.
	if _, err := test.service.UndoLastChange(test.context, test.dm, core.UndoInput{GroupID: advanced.GroupID}); err != nil {
		t.Fatalf("undo: %v", err)
	}
	rows = test.outbox(caller, 0)
	if len(rows) != 2 || rows[0].Kind != core.BridgeOutboxKindDaysAdvanced || !strings.Contains(string(rows[1].Payload), `"day": 1`) {
		t.Fatalf("rows after the undo = %+v", rows)
	}
	if after := test.outbox(caller, rows[0].Seq); len(after) != 1 || after[0].Seq != rows[1].Seq {
		t.Fatalf("rows after seq %d = %+v", rows[0].Seq, after)
	}
	if test.bridgeStatus().Link.UnappliedRows != 2 {
		t.Fatalf("unapplied rows = %d", test.bridgeStatus().Link.UnappliedRows)
	}

	// The DAYS_ADVANCED row fails; the summary is applied.
	if err := test.service.AckBridgeOutbox(test.context, caller, core.BridgeAckInput{
		Through: rows[1].Seq, Failed: []core.BridgeAckFailure{{Seq: rows[0].Seq, Reason: "No chat permission"}},
	}); err != nil {
		t.Fatalf("ack: %v", err)
	}
	if left := test.outbox(caller, 0); len(left) != 0 {
		t.Fatalf("rows after the ack = %+v", left)
	}
	link := test.bridgeStatus().Link
	if link.UnappliedRows != 0 || len(link.FailedRows) != 1 || link.FailedRows[0].Reason != "No chat permission" || link.FailedRows[0].Kind != core.BridgeOutboxKindDaysAdvanced {
		t.Fatalf("link after the ack = %+v", link)
	}
	for _, bad := range []core.BridgeAckInput{
		{Through: -1},
		{Through: 1, Failed: []core.BridgeAckFailure{{Seq: 0, Reason: "x"}}},
		{Through: 1, Failed: []core.BridgeAckFailure{{Seq: 1, Reason: strings.Repeat("x", 301)}}},
		{Through: 1, Failed: []core.BridgeAckFailure{{Seq: 1, Reason: "two\nlines"}}},
	} {
		if err := test.service.AckBridgeOutbox(test.context, caller, bad); !isKind(err, core.KindInvalid) {
			t.Errorf("ack %+v: %v, want invalid", bad, err)
		}
	}

	// Only the DM clears the failed rows, and clearing leaves the waiting count alone.
	if err := test.service.ClearBridgeFailedRows(test.context, test.player); !isKind(err, core.KindForbidden) {
		t.Fatalf("a player cleared the failed rows: %v", err)
	}
	if err := test.service.ClearBridgeFailedRows(test.context, test.dm); err != nil {
		t.Fatalf("clear the failed rows: %v", err)
	}
	if link := test.bridgeStatus().Link; len(link.FailedRows) != 0 || link.UnappliedRows != 0 {
		t.Fatalf("link after clearing = %+v", link)
	}

	// Applied and failed rows go 30 days later, on the next acknowledgement.
	test.clock.advance(31 * 24 * time.Hour)
	if err := test.service.AckBridgeOutbox(test.context, caller, core.BridgeAckInput{Through: 0}); err != nil {
		t.Fatalf("ack: %v", err)
	}
	var kept int
	if err := test.database.Pool.QueryRow(test.context, "SELECT count(*) FROM bridge_outbox WHERE campaign_id = $1", test.campaignID).Scan(&kept); err != nil || kept != 0 {
		t.Fatalf("outbox rows after the prune = %d, %v", kept, err)
	}
}

func jsonNumber(value int64) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func TestTheOutboxKeepsOneRowPerTargetAndACap(t *testing.T) {
	test := newColonyTest(t)
	_, caller := test.pairBridge()
	for range 3 {
		test.must(test.service.AdjustStock(test.context, test.dm, core.AdjustInput{
			ExpectedVersion: test.board().Version, Source: core.AdjustSourceAdjustment, Deltas: map[string]int64{"food": 1},
			SetTo: map[string]int64{},
		}))
	}
	rows := test.outbox(caller, 0)
	if len(rows) != 1 || rows[0].Kind != core.BridgeOutboxKindColonySummary {
		t.Fatalf("rows after three adjustments = %+v", rows)
	}
	// Past 10,000 unapplied rows the oldest go.
	if _, err := test.database.Pool.Exec(test.context, `INSERT INTO bridge_outbox (campaign_id, seq, kind, target_key, payload)
		SELECT $1, 100000 + n, 'DAYS_ADVANCED', 'advance:' || (100000 + n), '{}' FROM generate_series(1, 10000) AS n`, test.campaignID); err != nil {
		t.Fatalf("fill the outbox: %v", err)
	}
	if _, err := test.database.Pool.Exec(test.context, "UPDATE bridge_link SET outbox_seq = 200000 WHERE campaign_id = $1", test.campaignID); err != nil {
		t.Fatalf("move the sequence: %v", err)
	}
	test.must(test.advanceBy(1, nil))
	var unapplied, oldest int64
	if err := test.database.Pool.QueryRow(test.context, `SELECT count(*), min(seq) FROM bridge_outbox
		WHERE campaign_id = $1 AND applied_at IS NULL AND failed_at IS NULL`, test.campaignID).Scan(&unapplied, &oldest); err != nil {
		t.Fatal(err)
	}
	if unapplied != 10000 || oldest != 100003 {
		t.Fatalf("after the cap: %d unapplied, oldest %d; want 10000 and 100003", unapplied, oldest)
	}
}

type decodedSummaryResource struct {
	Name string `json:"name"`
}

type decodedSummaryRole struct {
	Role string `json:"role"`
}

// decodedSummary is the part of a COLONY_SUMMARY payload these tests read.
type decodedSummary struct {
	CampaignName     string                   `json:"campaignName"`
	Resources        []decodedSummaryResource `json:"resources"`
	Workforce        []decodedSummaryRole     `json:"workforce"`
	NextGoodsShipDay *int64                   `json:"nextGoodsShipDay"`
}

func (summary decodedSummary) hasResource(name string) bool {
	return slices.Contains(summary.Resources, decodedSummaryResource{Name: name})
}

func (summary decodedSummary) hasRole(role string) bool {
	return slices.Contains(summary.Workforce, decodedSummaryRole{Role: role})
}

func TestSettingsAndMarketChangesQueueAFreshSummary(t *testing.T) {
	test := newColonyTest(t)
	_, caller := test.pairBridge()
	subscription, err := test.service.SubscribeBridge(test.context, caller)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer subscription.Close()
	// Publishing is synchronous, so whatever an operation published is waiting once it returns.
	drainBridgeMessages := func() {
		for {
			select {
			case <-subscription.Messages():
			default:
				return
			}
		}
	}
	acknowledgeAll := func() {
		t.Helper()
		rows := test.outbox(caller, 0)
		if len(rows) > 0 {
			if err := test.service.AckBridgeOutbox(test.context, caller, core.BridgeAckInput{Through: rows[len(rows)-1].Seq}); err != nil {
				t.Fatalf("ack: %v", err)
			}
		}
		drainBridgeMessages()
	}
	requireSummary := func(change string) decodedSummary {
		t.Helper()
		select {
		case message := <-subscription.Messages():
			if !slices.Contains(message.Data.Topics, core.TopicBridge) {
				t.Fatalf("the bridge stream heard %v after %s", message.Data.Topics, change)
			}
		default:
			t.Fatalf("the bridge stream heard nothing after %s", change)
		}
		rows := test.outbox(caller, 0)
		if len(rows) != 1 || rows[0].Kind != core.BridgeOutboxKindColonySummary {
			t.Fatalf("rows after %s = %+v, want one summary", change, rows)
		}
		var summary decodedSummary
		if err := json.Unmarshal(rows[0].Payload, &summary); err != nil {
			t.Fatalf("decode the summary after %s: %v", change, err)
		}
		acknowledgeAll()
		return summary
	}
	acknowledgeAll()

	settings, err := test.service.GetSettings(test.context, test.dm)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if _, err := test.service.UpdateSettings(test.context, test.dm, core.SettingsInput{
		CampaignName: "Renamed Campaign", ColonyName: settings.ColonyName, Timezone: settings.Timezone,
		DiscordEventLocation: settings.DiscordEventLocation,
	}); err != nil {
		t.Fatalf("rename the campaign: %v", err)
	}
	if summary := requireSummary("a campaign rename"); summary.CampaignName != "Renamed Campaign" {
		t.Fatalf("campaignName = %q after a rename", summary.CampaignName)
	}

	resources, err := test.service.ListResources(test.context, test.dm)
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	food := resources.Resources[slices.IndexFunc(resources.Resources, func(resource core.ResourceSettings) bool { return resource.Key == "food" })]
	if _, err := test.service.UpdateResource(test.context, test.dm, food.Key, core.ResourceInput{
		Label: "Rations", Icon: food.Icon, MinAmount: food.MinAmount, MaxAmount: food.MaxAmount, SortOrder: food.SortOrder,
	}); err != nil {
		t.Fatalf("rename a resource: %v", err)
	}
	if summary := requireSummary("a resource rename"); !summary.hasResource("Rations") {
		t.Fatalf("resources after a rename = %+v", summary.Resources)
	}

	if _, err := test.service.CreateResource(test.context, test.dm, core.NewResourceInput{Label: "Timber", Icon: food.Icon}); err != nil {
		t.Fatalf("create a resource: %v", err)
	}
	if summary := requireSummary("a new resource"); !summary.hasResource("Timber") {
		t.Fatalf("resources after a create = %+v", summary.Resources)
	}

	role, err := test.service.CreateRole(test.context, test.dm, core.NewRoleInput{Label: "Scouts", Icon: food.Icon})
	if err != nil {
		t.Fatalf("create a role: %v", err)
	}
	if summary := requireSummary("a new role"); !summary.hasRole("Scouts") {
		t.Fatalf("workforce after a create = %+v", summary.Workforce)
	}

	if _, err := test.service.UpdateRole(test.context, test.dm, role.Key, core.RoleInput{Label: "Pathfinders", Icon: role.Icon, SortOrder: role.SortOrder}); err != nil {
		t.Fatalf("rename a role: %v", err)
	}
	if summary := requireSummary("a role rename"); !summary.hasRole("Pathfinders") || summary.hasRole("Scouts") {
		t.Fatalf("workforce after a rename = %+v", summary.Workforce)
	}

	foodOffer, found := test.offerID(test.dm, "food", "")
	if !found {
		t.Fatal("no food offer")
	}
	test.setCart(test.player, foodOffer, 1)
	drainBridgeMessages()
	order := test.placeOrder(test.player)
	if summary := requireSummary("placing an order"); summary.NextGoodsShipDay == nil || *summary.NextGoodsShipDay != order.ArrivesDay {
		t.Fatalf("nextGoodsShipDay = %v after placing, want %d", summary.NextGoodsShipDay, order.ArrivesDay)
	}

	if _, err := test.service.CancelMarketOrder(test.context, test.dm, order.ID); err != nil {
		t.Fatalf("cancel the order: %v", err)
	}
	if summary := requireSummary("cancelling the order"); summary.NextGoodsShipDay != nil {
		t.Fatalf("nextGoodsShipDay = %d after cancelling, want null", *summary.NextGoodsShipDay)
	}
}

func TestInboundEventsAreCheckedAndDeduplicated(t *testing.T) {
	test := newColonyTest(t)
	_, caller := test.pairBridge()
	status := core.BridgeInboundEvent{ID: uuid.New(), Kind: core.BridgeEventKindStatus, OccurredAt: time.Now().UTC(),
		Payload: json.RawMessage(`{"foundryVersion":"13.351","systemId":"pf2e","systemVersion":"7.12.2","moduleVersion":"0.1.0","worldTitle":"  Port Eldari  "}`)}
	advance := timeAdvanced(5)
	result, err := test.service.RecordBridgeEvents(test.context, caller, core.BridgeEventsInput{Events: []core.BridgeInboundEvent{status, advance, advance}})
	if err != nil || result.Accepted != 2 || result.Duplicates != 1 {
		t.Fatalf("record = %+v, %v; want 2 accepted and 1 duplicate", result, err)
	}
	again, err := test.service.RecordBridgeEvents(test.context, caller, core.BridgeEventsInput{Events: []core.BridgeInboundEvent{status}})
	if err != nil || again.Accepted != 0 || again.Duplicates != 1 {
		t.Fatalf("a repeat = %+v, %v", again, err)
	}
	bridge := test.bridgeStatus()
	if bridge.Link.WorldTitle != "Port Eldari" || bridge.Link.SystemID != "pf2e" || bridge.Link.ModuleVersion != "0.1.0" {
		t.Fatalf("link after a status event = %+v", bridge.Link)
	}
	if len(bridge.Inbox) != 1 || bridge.Inbox[0].Days != 5 || bridge.Inbox[0].Kind != core.BridgeEventKindTimeAdvanced {
		t.Fatalf("inbox = %+v", bridge.Inbox)
	}
	if board := test.board(); len(board.Dm.BridgeInbox) != 1 || board.Dm.BridgeInbox[0].ID != bridge.Inbox[0].ID {
		t.Fatalf("the DM's board inbox = %+v", board.Dm.BridgeInbox)
	}
	playerBoard, err := test.service.GetColonyBoard(test.context, test.player)
	if err != nil || len(playerBoard.Dm.BridgeInbox) != 0 {
		t.Fatalf("a player's board read the inbox: %+v, %v", playerBoard.Dm, err)
	}

	tooMany := make([]core.BridgeInboundEvent, core.BridgeEventsPerBatch+1)
	for index := range tooMany {
		tooMany[index] = timeAdvanced(1)
	}
	invalid := map[string][]core.BridgeInboundEvent{
		"no events":        {},
		"too many events":  tooMany,
		"no ID":            {{Kind: core.BridgeEventKindTimeAdvanced, OccurredAt: time.Now(), Payload: json.RawMessage(`{"days":1}`)}},
		"no time":          {{ID: uuid.New(), Kind: core.BridgeEventKindTimeAdvanced, Payload: json.RawMessage(`{"days":1}`)}},
		"unknown kind":     {{ID: uuid.New(), Kind: "NPC_IMPORT", OccurredAt: time.Now(), Payload: json.RawMessage(`{}`)}},
		"no days":          {{ID: uuid.New(), Kind: core.BridgeEventKindTimeAdvanced, OccurredAt: time.Now(), Payload: json.RawMessage(`{}`)}},
		"zero days":        {timeAdvanced(0)},
		"too many days":    {timeAdvanced(core.BridgeTimeAdvancedMaxDays + 1)},
		"half a day":       {{ID: uuid.New(), Kind: core.BridgeEventKindTimeAdvanced, OccurredAt: time.Now(), Payload: json.RawMessage(`{"days":1.5}`)}},
		"long title":       {{ID: uuid.New(), Kind: core.BridgeEventKindStatus, OccurredAt: time.Now(), Payload: json.RawMessage(`{"worldTitle":"` + strings.Repeat("x", 101) + `"}`)}},
		"control in title": {{ID: uuid.New(), Kind: core.BridgeEventKindStatus, OccurredAt: time.Now(), Payload: json.RawMessage(`{"worldTitle":"a\u0000b"}`)}},
		"numeric version":  {{ID: uuid.New(), Kind: core.BridgeEventKindStatus, OccurredAt: time.Now(), Payload: json.RawMessage(`{"foundryVersion":13}`)}},
		"a good one first": {timeAdvanced(2), timeAdvanced(0)},
	}
	for name, events := range invalid {
		if _, err := test.service.RecordBridgeEvents(test.context, caller, core.BridgeEventsInput{Events: events}); !isKind(err, core.KindInvalid) {
			t.Errorf("%s: %v, want invalid", name, err)
		}
	}
	if inbox := test.bridgeStatus().Inbox; len(inbox) != 1 {
		t.Fatalf("a refused batch wrote to the inbox: %+v", inbox)
	}

	// The inbox has a ceiling; decided events go after 30 days, pending ones never do.
	full := make([]core.BridgeInboundEvent, core.BridgeInboxLimit)
	for index := range full {
		full[index] = timeAdvanced(1)
	}
	if _, err := test.service.RecordBridgeEvents(test.context, caller, core.BridgeEventsInput{Events: full}); !isKind(err, core.KindConflict) {
		t.Fatalf("filling the inbox past its limit: %v", err)
	}
	test.clock.advance(31 * 24 * time.Hour)
	if _, err := test.service.RecordBridgeEvents(test.context, caller, core.BridgeEventsInput{Events: []core.BridgeInboundEvent{timeAdvanced(1)}}); err != nil {
		t.Fatalf("record: %v", err)
	}
	var statusRows, pendingRows int
	if err := test.database.Pool.QueryRow(test.context, `SELECT count(*) FILTER (WHERE kind = 'STATUS'), count(*) FILTER (WHERE status = 'PENDING')
		FROM bridge_event WHERE campaign_id = $1`, test.campaignID).Scan(&statusRows, &pendingRows); err != nil {
		t.Fatal(err)
	}
	if statusRows != 0 || pendingRows != 2 {
		t.Fatalf("after 30 days: %d status rows, %d pending; want 0 and 2", statusRows, pendingRows)
	}
	// At the limit, a retried event is still a duplicate, and only a new one is refused.
	upToLimit := make([]core.BridgeInboundEvent, core.BridgeInboxLimit-pendingRows)
	for index := range upToLimit {
		upToLimit[index] = timeAdvanced(1)
	}
	if _, err := test.service.RecordBridgeEvents(test.context, caller, core.BridgeEventsInput{Events: upToLimit}); err != nil {
		t.Fatalf("fill the inbox to its limit: %v", err)
	}
	retried, err := test.service.RecordBridgeEvents(test.context, caller, core.BridgeEventsInput{Events: upToLimit[:1]})
	if err != nil || retried.Duplicates != 1 || retried.Accepted != 0 {
		t.Fatalf("a retry at the limit = %+v, %v; want one duplicate", retried, err)
	}
	if _, err := test.service.RecordBridgeEvents(test.context, caller, core.BridgeEventsInput{Events: []core.BridgeInboundEvent{timeAdvanced(1)}}); !isKind(err, core.KindConflict) {
		t.Fatalf("a new item past the limit: %v", err)
	}
}

func TestAdvanceAcceptsAFoundryInboxItem(t *testing.T) {
	test := newColonyTest(t)
	_, caller := test.pairBridge()
	if _, err := test.service.RecordBridgeEvents(test.context, caller, core.BridgeEventsInput{Events: []core.BridgeInboundEvent{timeAdvanced(2), timeAdvanced(4)}}); err != nil {
		t.Fatalf("record: %v", err)
	}
	inbox := test.bridgeStatus().Inbox
	if len(inbox) != 2 {
		t.Fatalf("inbox = %+v", inbox)
	}
	first, second := inbox[0].ID, inbox[1].ID

	// A refused advance leaves the item waiting.
	if _, err := test.service.Advance(test.context, test.dm, core.AdvanceInput{Days: 2, ExpectedVersion: test.board().Version + 1, BridgeEventID: &first}); !isKind(err, core.KindConflict) {
		t.Fatalf("an advance at a stale version: %v", err)
	}
	if len(test.bridgeStatus().Inbox) != 2 {
		t.Fatal("a refused advance accepted its item")
	}
	test.must(test.advanceBy(2, &first))
	if inbox := test.bridgeStatus().Inbox; len(inbox) != 1 || inbox[0].ID != second {
		t.Fatalf("inbox after accepting = %+v", inbox)
	}
	if _, err := test.advanceBy(2, &first); !isKind(err, core.KindConflict) {
		t.Fatalf("accepting an item twice: %v", err)
	}
	missing := uuid.New()
	_, err := test.advanceBy(2, &missing)
	var operationError *core.Error
	if !errors.As(err, &operationError) || operationError.Kind != core.KindInvalid || len(operationError.Fields) != 1 || operationError.Fields[0].Field != "bridgeEventId" {
		t.Fatalf("an unknown item: %v, want a bridgeEventId field error", err)
	}
	if board := test.board(); board.Day != 2 {
		t.Fatalf("day = %d after one accepted advance; refused ones changed it", board.Day)
	}

	if err := test.service.DismissBridgeInboxItem(test.context, test.dm, second); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	if err := test.service.DismissBridgeInboxItem(test.context, test.dm, second); !isKind(err, core.KindConflict) {
		t.Fatalf("dismissing twice: %v", err)
	}
	if err := test.service.DismissBridgeInboxItem(test.context, test.dm, uuid.New()); !isKind(err, core.KindNotFound) {
		t.Fatalf("dismissing a missing item: %v", err)
	}
	if err := test.service.DismissBridgeInboxItem(test.context, test.player, second); !isKind(err, core.KindForbidden) {
		t.Fatalf("a player dismissed: %v", err)
	}
	if inbox := test.bridgeStatus().Inbox; len(inbox) != 0 {
		t.Fatalf("inbox after dismissing = %+v", inbox)
	}
}

func TestUndoingAnAcceptedAdvancePutsItsInboxItemBack(t *testing.T) {
	test := newColonyTest(t)
	_, caller := test.pairBridge()
	if _, err := test.service.RecordBridgeEvents(test.context, caller, core.BridgeEventsInput{Events: []core.BridgeInboundEvent{timeAdvanced(3)}}); err != nil {
		t.Fatalf("record: %v", err)
	}
	item := test.bridgeStatus().Inbox[0].ID
	advanced := test.must(test.advanceBy(3, &item))
	if inbox := test.bridgeStatus().Inbox; len(inbox) != 0 {
		t.Fatalf("inbox after accepting = %+v", inbox)
	}

	undo, err := test.service.UndoLastChange(test.context, test.dm, core.UndoInput{GroupID: advanced.GroupID})
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if inbox := undo.Board.Dm.BridgeInbox; len(inbox) != 1 || inbox[0].ID != item || inbox[0].Days != 3 {
		t.Fatalf("the undo's board inbox = %+v, want the accepted item back", inbox)
	}
	if inbox := test.bridgeStatus().Inbox; len(inbox) != 1 || inbox[0].ID != item {
		t.Fatalf("inbox after the undo = %+v", inbox)
	}

	test.must(test.advanceBy(3, &item))
	if board := test.board(); board.Day != 3 || len(board.Dm.BridgeInbox) != 0 {
		t.Fatalf("after accepting again: day %d, inbox %+v; want Day 3 and an empty inbox", board.Day, board.Dm.BridgeInbox)
	}
}

func TestBridgeStreamsHearOnlyTheBridgeTopic(t *testing.T) {
	test := newColonyTest(t)
	_, caller := test.pairBridge()
	subscription, err := test.service.SubscribeBridge(test.context, caller)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer subscription.Close()
	test.service.Broker().Publish(test.campaignID, core.TopicColony)
	test.must(test.advanceBy(1, nil))
	select {
	case message := <-subscription.Messages():
		if len(message.Data.Topics) != 1 || message.Data.Topics[0] != core.TopicBridge {
			t.Fatalf("the bridge stream heard %v", message.Data.Topics)
		}
	case <-time.After(time.Second):
		t.Fatal("no bridge message after an advance")
	}
	select {
	case message := <-subscription.Messages():
		t.Fatalf("an extra message %v", message.Data.Topics)
	default:
	}
	if err := test.service.RevokeBridge(test.context, test.dm); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	select {
	case <-subscription.Closed():
	case <-time.After(time.Second):
		t.Fatal("revoking left the bridge stream open")
	}
}
