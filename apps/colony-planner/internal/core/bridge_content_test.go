package core_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

const testCompendiumUUID = "Compendium.pf2e.equipment-srd.Item.abcdefghij123456"

// drain reads every waiting row, oldest first, and acknowledges them, so the next drain shows only
// what changed since.
func (test *colonyTest) drain(caller core.BridgeCaller) []core.BridgeOutboxRow {
	test.t.Helper()
	var rows []core.BridgeOutboxRow
	after := int64(0)
	for {
		page, err := test.service.ReadBridgeOutbox(test.context, caller, after)
		if err != nil {
			test.t.Fatalf("read the outbox: %v", err)
		}
		rows = append(rows, page.Rows...)
		if len(page.Rows) > 0 {
			after = page.Rows[len(page.Rows)-1].Seq
		}
		if !page.More {
			break
		}
	}
	if err := test.service.AckBridgeOutbox(test.context, caller, core.BridgeAckInput{Through: after}); err != nil {
		test.t.Fatalf("ack: %v", err)
	}
	return rows
}

// rowKinds is "KIND target" for each row, in order, leaving out the colony summary.
func rowKinds(rows []core.BridgeOutboxRow) []string {
	kinds := []string{}
	for _, row := range rows {
		if row.Kind != core.BridgeOutboxKindColonySummary {
			kinds = append(kinds, string(row.Kind)+" "+row.TargetKey)
		}
	}
	return kinds
}

func requireRows(t *testing.T, step string, rows []core.BridgeOutboxRow, want ...string) {
	t.Helper()
	got := rowKinds(rows)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("%s wrote\n  %s\nwant\n  %s", step, strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// payloadOf decodes the payload of the row with this kind and target.
func payloadOf(t *testing.T, rows []core.BridgeOutboxRow, kind core.BridgeOutboxKind, targetKey string) map[string]any {
	t.Helper()
	for _, row := range rows {
		if row.Kind == kind && row.TargetKey == targetKey {
			var payload map[string]any
			if err := json.Unmarshal(row.Payload, &payload); err != nil {
				t.Fatalf("decode %s %s: %v", kind, targetKey, err)
			}
			return payload
		}
	}
	t.Fatalf("no %s %s row in %v", kind, targetKey, rowKinds(rows))
	return nil
}

func requireNoText(t *testing.T, rows []core.BridgeOutboxRow, secrets ...string) {
	t.Helper()
	for _, row := range rows {
		for _, secret := range secrets {
			if strings.Contains(string(row.Payload), secret) || strings.Contains(row.TargetKey, secret) {
				t.Errorf("%s %s carries %q: %s", row.Kind, row.TargetKey, secret, row.Payload)
			}
		}
	}
}

func npcKey(id uuid.UUID) string   { return "npc:" + id.String() }
func placeKey(id uuid.UUID) string { return "place:" + id.String() }
func questKey(id uuid.UUID) string { return "quest:" + id.String() }

func (test *colonyTest) place(name string, revealed bool) core.Place {
	test.t.Helper()
	place, err := test.service.CreatePlace(test.context, test.dm, core.NewPlaceInput{Revealed: revealed, PlaceInput: core.PlaceInput{
		Name: name, Description: "A " + name, PlayerNotes: "Seen by the party", DmNotes: "DM NOTE " + name,
	}})
	if err != nil {
		test.t.Fatalf("create place %s: %v", name, err)
	}
	return place
}

func (test *colonyTest) patronNamed(name string, revealed bool) core.Patron {
	test.t.Helper()
	page, err := test.service.CreatePatron(test.context, test.dm, core.NewPatronInput{Revealed: revealed, PatronInput: core.PatronInput{Name: name}})
	if err != nil {
		test.t.Fatalf("create patron %s: %v", name, err)
	}
	return page.Patron
}

func (test *colonyTest) questGiven(title string, revealed bool, giverNpcID *uuid.UUID, patronID *uuid.UUID) core.Quest {
	test.t.Helper()
	favor := int64(0)
	if patronID != nil {
		favor = 2
	}
	quest, err := test.service.CreateQuest(test.context, test.dm, core.NewQuestInput{Revealed: revealed, QuestInput: core.QuestInput{
		Title: title, Description: "Do " + title, GiverNpcID: giverNpcID, PatronID: patronID, RewardFavor: favor,
		RewardEffects:  core.Effects{Resources: map[string]int64{"food": 20}, Colonists: 0},
		FailureEffects: core.Effects{Resources: map[string]int64{"morale": -7}},
		Loot:           []core.LootItem{{Name: "Silver ring", Quantity: 1, Description: "Engraved"}},
		DmNotes:        "DM NOTE " + title,
	}})
	if err != nil {
		test.t.Fatalf("create quest %s: %v", title, err)
	}
	return quest
}

func npcInput(name string, currentPlaceID *uuid.UUID) core.NpcInput {
	return core.NpcInput{Name: name, Race: "Human", Personality: "Gruff\nkind", DmNotes: "DM NOTE " + name, CurrentPlaceID: currentPlaceID}
}

func TestNpcsPlacesAndQuestsReachFoundryFromThePlayerProjection(t *testing.T) {
	test := newColonyTest(t)
	_, caller := test.pairBridge()
	test.drain(caller)

	harbor := test.place("Harbor", true)
	requireRows(t, "a revealed place", test.drain(caller), "PLACE_UPSERT "+placeKey(harbor.ID))
	vault := test.place("Vault", false)
	requireRows(t, "a hidden place", test.drain(caller))

	input := npcInput("Brannoc Vale", &harbor.ID)
	input.OriginPlaceID = &vault.ID
	brannoc := test.npc(core.NewNpcInput{Revealed: true, NpcInput: input})
	rows := test.drain(caller)
	requireRows(t, "a revealed NPC", rows, "NPC_UPSERT "+npcKey(brannoc.ID))
	npc := payloadOf(t, rows, core.BridgeOutboxKindNpcUpsert, npcKey(brannoc.ID))
	if npc["displayName"] != "Brannoc Vale" || npc["currentPlace"] != "Harbor" || npc["originPlace"] != nil || npc["colonyRole"] != nil ||
		npc["personality"] != "Gruff\nkind" || npc["siteUrl"] != testPublicURL+"/c/sample/npcs/"+brannoc.ID.String() || npc["status"] != "UNKNOWN" {
		t.Fatalf("NPC payload = %v", npc)
	}
	requireNoText(t, rows, "DM NOTE", "Vault", vault.ID.String(), `"dm"`)

	vex := test.patronNamed("Lady Vex", true)
	wolfDen := test.questGiven("Wolf Den", true, &brannoc.ID, &vex.ID)
	rows = test.drain(caller)
	requireRows(t, "a revealed quest", rows, "QUEST_UPSERT "+questKey(wolfDen.ID))
	quest := payloadOf(t, rows, core.BridgeOutboxKindQuestUpsert, questKey(wolfDen.ID))
	if quest["title"] != "Wolf Den" || quest["giver"] != "Brannoc Vale" || quest["patron"] != "Lady Vex" || quest["rewardFavor"] != float64(2) ||
		quest["reward"] != "Food +20" || quest["status"] != "ACTIVE" || quest["kind"] != "COLONY" || quest["resolvedDay"] != nil ||
		quest["siteUrl"] != testPublicURL+"/c/sample/quests/"+wolfDen.ID.String() {
		t.Fatalf("quest payload = %v", quest)
	}
	if loot, _ := quest["loot"].([]any); len(loot) != 1 {
		t.Fatalf("quest loot = %v", quest["loot"])
	}
	requireNoText(t, rows, "DM NOTE", "Morale", "failure", `"revealed"`)

	// A known name makes the true name a secret: the NPC and the quest they give both re-send.
	input.KnownName = "The Ferryman"
	if _, err := test.service.UpdateNpc(test.context, test.dm, brannoc.ID, input); err != nil {
		t.Fatal(err)
	}
	rows = test.drain(caller)
	requireRows(t, "a known name", rows, "NPC_UPSERT "+npcKey(brannoc.ID), "QUEST_UPSERT "+questKey(wolfDen.ID))
	if giver := payloadOf(t, rows, core.BridgeOutboxKindQuestUpsert, questKey(wolfDen.ID))["giver"]; giver != "The Ferryman" {
		t.Fatalf("the quest's giver = %v", giver)
	}
	requireNoText(t, rows, "Brannoc")
	// An edit that keeps the display name re-sends only the NPC.
	input.Race = "Elf"
	if _, err := test.service.UpdateNpc(test.context, test.dm, brannoc.ID, input); err != nil {
		t.Fatal(err)
	}
	requireRows(t, "an NPC edit", test.drain(caller), "NPC_UPSERT "+npcKey(brannoc.ID))

	// Renaming or hiding a place re-sends the NPCs whose entries name it.
	if _, err := test.service.UpdatePlace(test.context, test.dm, harbor.ID, core.PlaceInput{Name: "Old Harbor"}); err != nil {
		t.Fatal(err)
	}
	rows = test.drain(caller)
	requireRows(t, "a place rename", rows, "PLACE_UPSERT "+placeKey(harbor.ID), "NPC_UPSERT "+npcKey(brannoc.ID))
	if place := payloadOf(t, rows, core.BridgeOutboxKindNpcUpsert, npcKey(brannoc.ID))["currentPlace"]; place != "Old Harbor" {
		t.Fatalf("currentPlace after the rename = %v", place)
	}
	if _, err := test.service.UpdatePlace(test.context, test.dm, vault.ID, core.PlaceInput{Name: "Deep Vault"}); err != nil {
		t.Fatal(err)
	}
	requireRows(t, "a hidden place's rename", test.drain(caller))
	if _, err := test.service.SetPlaceRevealed(test.context, test.dm, harbor.ID, false); err != nil {
		t.Fatal(err)
	}
	rows = test.drain(caller)
	requireRows(t, "hiding a place", rows, "PLACE_WITHDRAW "+placeKey(harbor.ID), "NPC_UPSERT "+npcKey(brannoc.ID))
	if withdraw := payloadOf(t, rows, core.BridgeOutboxKindPlaceWithdraw, placeKey(harbor.ID)); len(withdraw) != 0 {
		t.Fatalf("a withdraw carries %v", withdraw)
	}
	if place := payloadOf(t, rows, core.BridgeOutboxKindNpcUpsert, npcKey(brannoc.ID))["currentPlace"]; place != nil {
		t.Fatalf("a hidden place still names the NPC's place: %v", place)
	}
	if _, err := test.service.SetPlaceRevealed(test.context, test.dm, harbor.ID, false); err != nil {
		t.Fatal(err)
	}
	requireRows(t, "hiding a hidden place", test.drain(caller))
	if _, err := test.service.SetPlaceRevealed(test.context, test.dm, harbor.ID, true); err != nil {
		t.Fatal(err)
	}
	requireRows(t, "revealing a place", test.drain(caller), "PLACE_UPSERT "+placeKey(harbor.ID), "NPC_UPSERT "+npcKey(brannoc.ID))

	// A hidden patron leaves the quest's patron and favor out.
	if _, err := test.service.SetPatronRevealed(test.context, test.dm, vex.ID, false); err != nil {
		t.Fatal(err)
	}
	rows = test.drain(caller)
	requireRows(t, "hiding a patron", rows, "QUEST_UPSERT "+questKey(wolfDen.ID))
	if quest := payloadOf(t, rows, core.BridgeOutboxKindQuestUpsert, questKey(wolfDen.ID)); quest["patron"] != nil || quest["rewardFavor"] != float64(0) {
		t.Fatalf("a hidden patron's quest = %v", quest)
	}
	requireNoText(t, rows, "Vex")
	if _, err := test.service.UpdatePatron(test.context, test.dm, vex.ID, core.PatronInput{Name: "Lady Vex the Second"}); err != nil {
		t.Fatal(err)
	}
	requireRows(t, "renaming a hidden patron", test.drain(caller))
	if _, err := test.service.SetPatronRevealed(test.context, test.dm, vex.ID, true); err != nil {
		t.Fatal(err)
	}
	test.drain(caller)
	if _, err := test.service.UpdatePatron(test.context, test.dm, vex.ID, core.PatronInput{Name: "Lady Vex"}); err != nil {
		t.Fatal(err)
	}
	requireRows(t, "renaming a revealed patron", test.drain(caller), "QUEST_UPSERT "+questKey(wolfDen.ID))

	// Hiding the NPC withdraws them and drops them from their quest.
	if _, err := test.service.SetNpcRevealed(test.context, test.dm, brannoc.ID, false); err != nil {
		t.Fatal(err)
	}
	rows = test.drain(caller)
	requireRows(t, "hiding an NPC", rows, "NPC_WITHDRAW "+npcKey(brannoc.ID), "QUEST_UPSERT "+questKey(wolfDen.ID))
	if giver := payloadOf(t, rows, core.BridgeOutboxKindQuestUpsert, questKey(wolfDen.ID))["giver"]; giver != nil {
		t.Fatalf("a hidden NPC still gives the quest: %v", giver)
	}
	if _, err := test.service.SetNpcRevealed(test.context, test.dm, brannoc.ID, false); err != nil {
		t.Fatal(err)
	}
	requireRows(t, "hiding a hidden NPC", test.drain(caller))
	if _, err := test.service.SetNpcRevealed(test.context, test.dm, brannoc.ID, true); err != nil {
		t.Fatal(err)
	}
	requireRows(t, "revealing an NPC", test.drain(caller), "NPC_UPSERT "+npcKey(brannoc.ID), "QUEST_UPSERT "+questKey(wolfDen.ID))

	// Deletes withdraw, and their foreign keys' cleared links re-send what named them.
	if err := test.service.DeletePlace(test.context, test.dm, harbor.ID); err != nil {
		t.Fatal(err)
	}
	requireRows(t, "deleting a place", test.drain(caller), "PLACE_WITHDRAW "+placeKey(harbor.ID), "NPC_UPSERT "+npcKey(brannoc.ID))
	if err := test.service.DeleteNpc(test.context, test.dm, brannoc.ID); err != nil {
		t.Fatal(err)
	}
	rows = test.drain(caller)
	requireRows(t, "deleting an NPC", rows, "NPC_WITHDRAW "+npcKey(brannoc.ID), "QUEST_UPSERT "+questKey(wolfDen.ID))
	if giver := payloadOf(t, rows, core.BridgeOutboxKindQuestUpsert, questKey(wolfDen.ID))["giver"]; giver != nil {
		t.Fatalf("a deleted NPC still gives the quest: %v", giver)
	}
	if _, err := test.service.SetQuestRevealed(test.context, test.dm, wolfDen.ID, core.QuestRevealInput{Revealed: false}); err != nil {
		t.Fatal(err)
	}
	requireRows(t, "hiding a quest", test.drain(caller), "QUEST_WITHDRAW "+questKey(wolfDen.ID))
	if _, err := test.service.SetQuestRevealed(test.context, test.dm, wolfDen.ID, core.QuestRevealInput{Revealed: true}); err != nil {
		t.Fatal(err)
	}
	requireRows(t, "revealing a quest", test.drain(caller), "QUEST_UPSERT "+questKey(wolfDen.ID))
	if err := test.service.DeleteQuest(test.context, test.dm, wolfDen.ID); err != nil {
		t.Fatal(err)
	}
	requireRows(t, "deleting a quest", test.drain(caller), "QUEST_WITHDRAW "+questKey(wolfDen.ID))

	// Hiding then revealing before Foundry reads leaves only the upsert: they share one key.
	ford := test.place("Ford", true)
	test.drain(caller)
	for _, revealed := range []bool{false, true} {
		if _, err := test.service.SetPlaceRevealed(test.context, test.dm, ford.ID, revealed); err != nil {
			t.Fatal(err)
		}
	}
	requireRows(t, "hiding and revealing", test.drain(caller), "PLACE_UPSERT "+placeKey(ford.ID))
}

// Hidden rows write nothing but the summaries a colony mutation already writes (Invariant 15).
func TestHiddenRowsNeverReachTheOutbox(t *testing.T) {
	test := newColonyTest(t)
	_, caller := test.pairBridge()
	test.drain(caller)
	vault := test.place("Vault", false)
	if _, err := test.service.UpdatePlace(test.context, test.dm, vault.ID, core.PlaceInput{Name: "Deep Vault", DmNotes: "secret"}); err != nil {
		t.Fatal(err)
	}
	if _, err := test.service.SetPlaceRevealed(test.context, test.dm, vault.ID, false); err != nil {
		t.Fatal(err)
	}
	input := npcInput("Mira Thorn", &vault.ID)
	mira := test.npc(core.NewNpcInput{NpcInput: input})
	input.KnownName = "The Veiled One"
	if _, err := test.service.UpdateNpc(test.context, test.dm, mira.ID, input); err != nil {
		t.Fatal(err)
	}
	if _, err := test.service.SetNpcRevealed(test.context, test.dm, mira.ID, false); err != nil {
		t.Fatal(err)
	}
	shadow := test.patronNamed("Shadow Court", false)
	if _, err := test.service.UpdatePatron(test.context, test.dm, shadow.ID, core.PatronInput{Name: "Shadow Court of Ash"}); err != nil {
		t.Fatal(err)
	}
	heist := test.questGiven("Heist", false, &mira.ID, &shadow.ID)
	if _, err := test.service.UpdateQuest(test.context, test.dm, heist.ID, core.QuestInput{Title: "The Heist", GiverNpcID: &mira.ID, PatronID: &shadow.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := test.service.SetQuestRevealed(test.context, test.dm, heist.ID, core.QuestRevealInput{Revealed: false}); err != nil {
		t.Fatal(err)
	}
	joined := test.join(mira.ID, "FARMER", core.JoinModeNewColonist)
	rows := test.drain(caller)
	requireRows(t, "hidden rows changing", rows)
	if len(rows) == 0 {
		t.Fatal("joining the colony wrote no summary")
	}
	test.undo(joined)
	if err := test.service.DeleteQuest(test.context, test.dm, heist.ID); err != nil {
		t.Fatal(err)
	}
	if err := test.service.DeleteNpc(test.context, test.dm, mira.ID); err != nil {
		t.Fatal(err)
	}
	if err := test.service.DeletePlace(test.context, test.dm, vault.ID); err != nil {
		t.Fatal(err)
	}
	rows = test.drain(caller)
	requireRows(t, "hidden rows deleted", rows)
	requireNoText(t, rows, "Mira", "Veiled", "Vault", "Shadow", "Heist", mira.ID.String(), vault.ID.String(), heist.ID.String())
}

// Join, change role, leave, complete, fail, reopen and their undo re-send what they changed.
func TestColonyMutationsResendTheirNpcsAndQuests(t *testing.T) {
	test := newColonyTest(t)
	brannoc := test.npc(core.NewNpcInput{Revealed: true, NpcInput: npcInput("Brannoc Vale", nil)})
	wolfDen := test.questGiven("Wolf Den", true, nil, nil)
	_, caller := test.pairBridge()
	test.drain(caller)

	joined := test.join(brannoc.ID, "FARMER", core.JoinModeNewColonist)
	rows := test.drain(caller)
	requireRows(t, "joining", rows, "NPC_UPSERT "+npcKey(brannoc.ID))
	if role := payloadOf(t, rows, core.BridgeOutboxKindNpcUpsert, npcKey(brannoc.ID))["colonyRole"]; role != "Farmer" {
		t.Fatalf("colonyRole after joining = %v", role)
	}
	test.undo(joined)
	rows = test.drain(caller)
	requireRows(t, "undoing the join", rows, "NPC_UPSERT "+npcKey(brannoc.ID))
	if role := payloadOf(t, rows, core.BridgeOutboxKindNpcUpsert, npcKey(brannoc.ID))["colonyRole"]; role != nil {
		t.Fatalf("colonyRole after the undo = %v", role)
	}
	test.join(brannoc.ID, "FARMER", core.JoinModeNewColonist)
	test.drain(caller)
	test.leave(brannoc.ID, core.LeaveModeRemove)
	requireRows(t, "leaving", test.drain(caller), "NPC_UPSERT "+npcKey(brannoc.ID))

	completed := test.must(test.service.CompleteQuest(test.context, test.dm, wolfDen.ID))
	rows = test.drain(caller)
	requireRows(t, "completing", rows, "QUEST_UPSERT "+questKey(wolfDen.ID))
	if quest := payloadOf(t, rows, core.BridgeOutboxKindQuestUpsert, questKey(wolfDen.ID)); quest["status"] != "COMPLETED" || quest["resolvedDay"] != float64(0) {
		t.Fatalf("a completed quest = %v", quest)
	}
	test.undo(completed)
	rows = test.drain(caller)
	requireRows(t, "undoing the completion", rows, "QUEST_UPSERT "+questKey(wolfDen.ID))
	if status := payloadOf(t, rows, core.BridgeOutboxKindQuestUpsert, questKey(wolfDen.ID))["status"]; status != "ACTIVE" {
		t.Fatalf("status after the undo = %v", status)
	}
	test.must(test.service.FailQuest(test.context, test.dm, wolfDen.ID))
	rows = test.drain(caller)
	requireRows(t, "failing", rows, "QUEST_UPSERT "+questKey(wolfDen.ID))
	// Players read a failure's cost only on its timeline entry (Invariant 16).
	if failed := payloadOf(t, rows, core.BridgeOutboxKindQuestUpsert, questKey(wolfDen.ID)); failed["status"] != "FAILED" || failed["reward"] != "Food +20" || len(failed) != 11 {
		t.Fatalf("a failed quest = %v", failed)
	}
	test.must(test.service.ReopenQuest(test.context, test.dm, wolfDen.ID))
	requireRows(t, "reopening", test.drain(caller), "QUEST_UPSERT "+questKey(wolfDen.ID))
}

func TestPairingBackfillsEveryRevealedRow(t *testing.T) {
	test := newColonyTest(t)
	harbor := test.place("Harbor", true)
	vault := test.place("Vault", false)
	brannoc := test.npc(core.NewNpcInput{Revealed: true, NpcInput: npcInput("Brannoc Vale", &vault.ID)})
	mira := test.npc(core.NewNpcInput{NpcInput: npcInput("Mira Thorn", &harbor.ID)})
	wolfDen := test.questGiven("Wolf Den", true, &mira.ID, nil)
	heist := test.questGiven("Heist", false, &brannoc.ID, nil)
	var waiting int
	if err := test.database.Pool.QueryRow(test.context, "SELECT count(*) FROM bridge_outbox WHERE campaign_id = $1", test.campaignID).Scan(&waiting); err != nil || waiting != 0 {
		t.Fatalf("an unlinked campaign queued %d rows, %v", waiting, err)
	}

	_, caller := test.pairBridge()
	rows := test.drain(caller)
	if rows[0].Kind != core.BridgeOutboxKindColonySummary {
		t.Fatalf("the first row is %s", rows[0].Kind)
	}
	requireRows(t, "pairing", rows, "PLACE_UPSERT "+placeKey(harbor.ID), "NPC_UPSERT "+npcKey(brannoc.ID), "QUEST_UPSERT "+questKey(wolfDen.ID))
	if place := payloadOf(t, rows, core.BridgeOutboxKindNpcUpsert, npcKey(brannoc.ID))["currentPlace"]; place != nil {
		t.Fatalf("the backfill names a hidden place: %v", place)
	}
	if giver := payloadOf(t, rows, core.BridgeOutboxKindQuestUpsert, questKey(wolfDen.ID))["giver"]; giver != nil {
		t.Fatalf("the backfill names a hidden giver: %v", giver)
	}
	requireNoText(t, rows, "Vault", "Mira", "Heist", "DM NOTE", vault.ID.String(), mira.ID.String(), heist.ID.String())
	for index := 1; index < len(rows); index++ {
		if rows[index].Seq != rows[index-1].Seq+1 {
			t.Fatalf("backfill seqs jump: %d then %d", rows[index-1].Seq, rows[index].Seq)
		}
	}

	// Pairing again backfills again, replacing anything the old link left waiting.
	test.place("Ford", true)
	_, caller = test.pairBridge()
	if kinds := rowKinds(test.drain(caller)); len(kinds) != 4 {
		t.Fatalf("a second pairing wrote %v", kinds)
	}
}

// Past the cap only one-shot rows go: a trimmed state row would lose its target until it next changed.
func TestTheOutboxCapTrimsOnlyOneShotRows(t *testing.T) {
	test := newColonyTest(t)
	harbor := test.place("Harbor", true)
	_, caller := test.pairBridge()
	if _, err := test.database.Pool.Exec(test.context, `INSERT INTO bridge_outbox (campaign_id, seq, kind, target_key, payload)
		SELECT $1, 100000 + n, 'POLL_POSTED', 'poll:' || n || ':posted', '{}' FROM generate_series(1, 10000) AS n`, test.campaignID); err != nil {
		t.Fatalf("fill the outbox: %v", err)
	}
	if _, err := test.database.Pool.Exec(test.context, "UPDATE bridge_link SET outbox_seq = 200000 WHERE campaign_id = $1", test.campaignID); err != nil {
		t.Fatal(err)
	}
	test.must(test.advanceBy(1, nil))
	var unapplied, oneShots int64
	var placeKept bool
	if err := test.database.Pool.QueryRow(test.context, `SELECT count(*), count(*) FILTER (WHERE kind = 'POLL_POSTED'),
		bool_or(target_key = $2) FROM bridge_outbox WHERE campaign_id = $1 AND applied_at IS NULL AND failed_at IS NULL`,
		test.campaignID, placeKey(harbor.ID)).Scan(&unapplied, &oneShots, &placeKept); err != nil {
		t.Fatal(err)
	}
	// The summary, the place and the advance's DAYS_ADVANCED leave room for 9,997 of the filler.
	if unapplied != 10000 || oneShots != 9997 || !placeKept {
		t.Fatalf("after the cap: %d unapplied, %d filler, place kept %v; want 10000, 9997, true", unapplied, oneShots, placeKept)
	}
	if rows := test.outbox(caller, 0); rows[0].TargetKey != placeKey(harbor.ID) {
		t.Fatalf("the oldest waiting row is %s %s, want the backfilled place", rows[0].Kind, rows[0].TargetKey)
	}
}

// Past 10,000 waiting state rows the newest 1,000 one-shot rows still wait, so a new delivery or poll
// row is never trimmed in its own transaction.
func TestTheOutboxCapKeepsTheNewestOneShotsPastManyStateRows(t *testing.T) {
	test := newColonyTest(t)
	_, caller := test.pairBridge()
	test.drain(caller)
	if _, err := test.database.Pool.Exec(test.context, `INSERT INTO bridge_outbox (campaign_id, seq, kind, target_key, payload)
		SELECT $1::uuid, 100000 + n, 'NPC_UPSERT'::bridge_outbox_kind, 'npc:filler-' || n, '{}'::jsonb FROM generate_series(1, 10500) AS n
		UNION ALL
		SELECT $1, 200000 + n, 'POLL_POSTED', 'poll:filler-' || n || ':posted', '{}' FROM generate_series(1, 1500) AS n`, test.campaignID); err != nil {
		t.Fatalf("fill the outbox: %v", err)
	}
	if _, err := test.database.Pool.Exec(test.context, "UPDATE bridge_link SET outbox_seq = 300000 WHERE campaign_id = $1", test.campaignID); err != nil {
		t.Fatal(err)
	}
	test.must(test.advanceBy(1, nil))
	var stateRows, oneShots, oldestOneShot int64
	var advanceKept bool
	if err := test.database.Pool.QueryRow(test.context, `SELECT count(*) FILTER (WHERE kind IN ('NPC_UPSERT', 'COLONY_SUMMARY')),
		count(*) FILTER (WHERE kind IN ('POLL_POSTED', 'DAYS_ADVANCED')), min(seq) FILTER (WHERE kind = 'POLL_POSTED'),
		bool_or(kind = 'DAYS_ADVANCED') FROM bridge_outbox WHERE campaign_id = $1 AND applied_at IS NULL AND failed_at IS NULL`,
		test.campaignID).Scan(&stateRows, &oneShots, &oldestOneShot, &advanceKept); err != nil {
		t.Fatal(err)
	}
	// The advance's DAYS_ADVANCED and the newest 999 filler polls are the 1,000 one-shot rows kept.
	if stateRows != 10501 || oneShots != 1000 || oldestOneShot != 200502 || !advanceKept {
		t.Fatalf("after the cap: %d state rows, %d one-shot rows from seq %d, advance kept %v; want 10501, 1000 from 200502, true",
			stateRows, oneShots, oldestOneShot, advanceKept)
	}
}

// Pairing's backfill and a cascade cost the same queries however many NPCs they write.
func TestBackfillAndCascadesTakeAFixedNumberOfQueries(t *testing.T) {
	test := newColonyTest(t)
	harbor := test.place("Harbor", true)
	addNpcs := func(count int) {
		for index := range count {
			test.npc(core.NewNpcInput{Revealed: true, NpcInput: npcInput("Sailor "+strings.Repeat("I", index+1), &harbor.ID)})
		}
	}
	measure := func(newName string) (pairing int64, renaming int64) {
		test.database.Queries.Reset()
		_, caller := test.pairBridge()
		pairing = test.database.Queries.Count()
		test.drain(caller)
		test.database.Queries.Reset()
		if _, err := test.service.UpdatePlace(test.context, test.dm, harbor.ID, core.PlaceInput{Name: newName}); err != nil {
			t.Fatal(err)
		}
		renaming = test.database.Queries.Count()
		if rows := test.drain(caller); len(rowKinds(rows)) < 2 {
			t.Fatalf("the rename wrote %v", rowKinds(rows))
		}
		return pairing, renaming
	}
	addNpcs(2)
	fewPairing, fewRenaming := measure("Old Harbor")
	addNpcs(10)
	manyPairing, manyRenaming := measure("New Harbor")
	if fewPairing != manyPairing || fewRenaming != manyRenaming {
		t.Fatalf("2 NPCs: pairing %d, renaming %d queries; 12 NPCs: %d and %d", fewPairing, fewRenaming, manyPairing, manyRenaming)
	}
}

func TestPaidShipmentsAndTheirUndoReachFoundry(t *testing.T) {
	test := newColonyTest(t)
	test.setShipping(1, 1)
	boots := test.specialItem("Boots of Striding", 5, nil)
	test.setCart(test.player, test.marketItemID("Food"), 10)
	test.setCart(test.player, boots, 2)
	order := test.placeOrder(test.player)
	// Set while the ship sails: Pay reads the item's UUID then.
	if _, err := test.service.UpdateMarketItem(test.context, test.dm, boots, core.MarketItemInput{Name: "Boots of Striding", PriceGold: 5, CompendiumUUID: new(" " + testCompendiumUUID + " ")}); err != nil {
		t.Fatalf("set the UUID: %v", err)
	}
	test.advance(1)
	test.setTo(map[string]int64{"gold": 100})
	_, caller := test.pairBridge()
	test.drain(caller)

	paid, err := test.payOrder(test.player, order.ID)
	if err != nil {
		t.Fatalf("pay: %v", err)
	}
	rows := test.outbox(caller, 0)
	deliveryKey := "delivery:" + paid.GroupID.String()
	requireRows(t, "paying", rows, "DELIVERY "+deliveryKey)
	delivery := payloadOf(t, rows, core.BridgeOutboxKindDelivery, deliveryKey)
	items, _ := delivery["items"].([]any)
	if delivery["paidDay"] != float64(1) || delivery["totalGold"] != float64(20) || delivery["summary"] != "20 food, Boots of Striding × 2" || len(items) != 1 {
		t.Fatalf("delivery = %v", delivery)
	}
	if item, _ := items[0].(map[string]any); item["name"] != "Boots of Striding" || item["quantity"] != float64(2) || item["compendiumUuid"] != testCompendiumUUID {
		t.Fatalf("delivery item = %v", items[0])
	}
	// Undone after Foundry fetched the DELIVERY but before it acknowledged it: the row is taken back, so
	// a module that hasn't fetched it never delivers it, and DELIVERY_UNDONE still goes, since one that
	// has may apply it anyway.
	test.undo(paid)
	requireRows(t, "undoing an unacknowledged payment", test.drain(caller), "DELIVERY_UNDONE delivery-undone:"+paid.GroupID.String())

	paidAgain, err := test.payOrder(test.player, order.ID)
	if err != nil {
		t.Fatalf("pay again: %v", err)
	}
	if paidAgain.GroupID == paid.GroupID {
		t.Fatal("a second payment reused the first one's group")
	}
	requireRows(t, "paying again", test.drain(caller), "DELIVERY delivery:"+paidAgain.GroupID.String())
	test.undo(paidAgain)
	rows = test.drain(caller)
	undoneKey := "delivery-undone:" + paidAgain.GroupID.String()
	requireRows(t, "undoing a delivered payment", rows, "DELIVERY_UNDONE "+undoneKey)
	undone := payloadOf(t, rows, core.BridgeOutboxKindDeliveryUndone, undoneKey)
	if undoneItems, _ := undone["items"].([]any); undone["summary"] != "20 food, Boots of Striding × 2" || len(undoneItems) != 1 ||
		undoneItems[0].(map[string]any)["name"] != "Boots of Striding" || undoneItems[0].(map[string]any)["compendiumUuid"] != nil {
		t.Fatalf("DELIVERY_UNDONE = %v", undone)
	}
}

func TestOnlySpecialItemsTakeACompendiumUUID(t *testing.T) {
	test := newColonyTest(t)
	item, err := test.service.CreateMarketItem(test.context, test.dm, core.MarketItemInput{Name: "Lantern", PriceGold: 1, CompendiumUUID: new(testCompendiumUUID)})
	if err != nil || item.Dm == nil || item.Dm.CompendiumUUID == nil || *item.Dm.CompendiumUUID != testCompendiumUUID {
		t.Fatalf("create with a UUID = %+v, %v", item, err)
	}
	if page := test.market(test.dm); !strings.Contains(string(mustJSON(t, page.Items)), testCompendiumUUID) {
		t.Fatal("the DM doesn't see the item's UUID")
	}
	if page := test.market(test.player); strings.Contains(string(mustJSON(t, page.Items)), testCompendiumUUID) {
		t.Fatal("players see the item's DM-only UUID")
	}
	// A full-record PUT: blank and omitted both clear it.
	for _, cleared := range []*string{new("  "), nil} {
		item, err = test.service.UpdateMarketItem(test.context, test.dm, item.ID, core.MarketItemInput{Name: "Lantern", PriceGold: 1, CompendiumUUID: cleared})
		if err != nil || item.Dm == nil || item.Dm.CompendiumUUID != nil {
			t.Fatalf("clear the UUID with %v = %+v, %v", cleared, item, err)
		}
	}
	for _, bad := range []string{
		"Compendium.pf2e.equipment-srd.abcdefghij123456",
		"Compendium.pf2e.equipment-srd.Item.abcdefghij12345",
		"Compendium.pf2e.equipment-srd.Actor.abcdefghij123456",
		"Item.abcdefghij123456",
		"Compendium.pf2e.equip ment.Item.abcdefghij123456",
		"Compendium." + strings.Repeat("p", 170) + ".equipment-srd.Item.abcdefghij123456",
	} {
		_, err := test.service.UpdateMarketItem(test.context, test.dm, item.ID, core.MarketItemInput{Name: "Lantern", PriceGold: 1, CompendiumUUID: &bad})
		requireKind(t, err, core.KindInvalid, "compendiumUuid")
		if operationError, _ := core.AsError(err); !hasFieldMessage(operationError, "Paste the item's UUID from Foundry, like Compendium.pf2e.equipment-srd.Item.abcdefghij123456.") {
			t.Errorf("%q: %v", bad, err)
		}
	}
	var refused error
	_, refused = test.database.Pool.Exec(test.context, "UPDATE market_item SET compendium_uuid = $2 WHERE campaign_id = $1 AND resource_key IS NOT NULL", test.campaignID, testCompendiumUUID)
	if refused == nil {
		t.Fatal("a supply offer took a compendium UUID")
	}
}

func hasFieldMessage(operationError *core.Error, message string) bool {
	if operationError == nil {
		return false
	}
	for _, field := range operationError.Fields {
		if field.Field == "compendiumUuid" && field.Message == message {
			return true
		}
	}
	return false
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestPollsPostAndCloseInFoundry(t *testing.T) {
	test := newColonyTest(t)
	_, caller := test.pairBridge()
	test.drain(caller)

	// Deleted before Foundry read it: the post is taken back.
	early := test.poll(false, 24, "North field", "River bend")
	if err := test.service.DeletePoll(test.context, test.dm, early.ID); err != nil {
		t.Fatal(err)
	}
	requireRows(t, "deleting an unread poll", test.drain(caller))

	// Closed and deleted before Foundry read either: the close is taken back too.
	closedEarly := test.poll(false, 24, "North field", "River bend")
	if _, err := test.service.ClosePoll(test.context, test.dm, closedEarly.ID); err != nil {
		t.Fatal(err)
	}
	if err := test.service.DeletePoll(test.context, test.dm, closedEarly.ID); err != nil {
		t.Fatal(err)
	}
	requireRows(t, "deleting an unread closed poll", test.drain(caller))

	poll := test.poll(true, 24, "North field", "River bend")
	rows := test.drain(caller)
	postedKey := "poll:" + poll.ID.String() + ":posted"
	requireRows(t, "creating a poll", rows, "POLL_POSTED "+postedKey)
	posted := payloadOf(t, rows, core.BridgeOutboxKindPollPosted, postedKey)
	if options, _ := posted["options"].([]any); posted["question"] != "Where do we build the mill?" || len(options) != 2 || options[0] != "North field" ||
		posted["allowMultiple"] != true || posted["voting"] != "SITE" || posted["pollsUrl"] != testPublicURL+"/c/sample/polls" ||
		posted["closesAt"] != test.clock.Now().Add(24*time.Hour).UTC().Format(time.RFC3339) {
		t.Fatalf("POLL_POSTED = %v", posted)
	}
	test.vote(test.player, poll.ID, poll.Options[1].ID)
	requireRows(t, "a vote", test.drain(caller))
	if _, err := test.service.ClosePoll(test.context, test.dm, poll.ID); err != nil {
		t.Fatal(err)
	}
	rows = test.drain(caller)
	closedKey := "poll:" + poll.ID.String() + ":closed"
	requireRows(t, "closing a poll", rows, "POLL_CLOSED "+closedKey)
	closed := payloadOf(t, rows, core.BridgeOutboxKindPollClosed, closedKey)
	options, _ := closed["options"].([]any)
	if closed["question"] != "Where do we build the mill?" || closed["closedAt"] != test.clock.Now().UTC().Format(time.RFC3339) || len(options) != 2 {
		t.Fatalf("POLL_CLOSED = %v", closed)
	}
	if second, _ := options[1].(map[string]any); second["label"] != "River bend" || second["count"] != float64(1) || second["otherVotes"] != float64(0) {
		t.Fatalf("POLL_CLOSED's second option = %v", options[1])
	}
	requireNoText(t, rows, "ana", "winner")
	if _, err := test.service.ClosePoll(test.context, test.dm, poll.ID); err != nil {
		t.Fatal(err)
	}
	requireRows(t, "closing a closed poll", test.drain(caller))
	if err := test.service.DeletePoll(test.context, test.dm, poll.ID); err != nil {
		t.Fatal(err)
	}
	requireRows(t, "deleting a closed poll", test.drain(caller))
}

func TestTheSummaryCarriesTheNextGameNight(t *testing.T) {
	test := newColonyTest(t)
	_, caller := test.pairBridge()
	rows := test.drain(caller)
	if summary := payloadOf(t, rows, core.BridgeOutboxKindColonySummary, "colony"); summary["nextSession"] != nil {
		t.Fatalf("nextSession with no series = %v", summary["nextSession"])
	}
	everyWeeks := int64(1)
	series, err := test.service.CreateSessionSeries(test.context, test.dm, core.SessionSeriesInput{
		Title: "Colony night", Weekdays: []int64{1}, EveryWeeks: &everyWeeks, FirstDate: "2026-10-12", StartLocal: "19:00", DurationMinutes: 240,
	})
	if err != nil {
		t.Fatal(err)
	}
	next := payloadOf(t, test.drain(caller), core.BridgeOutboxKindColonySummary, "colony")["nextSession"]
	session, _ := next.(map[string]any)
	if session["title"] != "Colony night" || session["localDate"] != "2026-10-12" || session["timezone"] != test.board().Timezone || session["startsAt"] == nil || session["endsAt"] == nil {
		t.Fatalf("nextSession = %v", next)
	}
	if _, err := test.service.MarkSessionPlayed(test.context, test.dm, core.SessionDateInput{SeriesID: series.ID, LocalDate: "2026-10-12"}); err != nil {
		t.Fatal(err)
	}
	next = payloadOf(t, test.drain(caller), core.BridgeOutboxKindColonySummary, "colony")["nextSession"]
	if session, _ := next.(map[string]any); session["localDate"] != "2026-10-19" {
		t.Fatalf("nextSession after Mark played = %v", next)
	}
	if _, err := test.service.CancelSessionDate(test.context, test.dm, core.SessionDateInput{SeriesID: series.ID, LocalDate: "2026-10-19"}); err != nil {
		t.Fatal(err)
	}
	next = payloadOf(t, test.drain(caller), core.BridgeOutboxKindColonySummary, "colony")["nextSession"]
	if session, _ := next.(map[string]any); session["localDate"] != "2026-10-26" {
		t.Fatalf("nextSession after a cancellation = %v", next)
	}
	if err := test.service.DeleteSessionSeries(test.context, test.dm, series.ID); err != nil {
		t.Fatal(err)
	}
	if next := payloadOf(t, test.drain(caller), core.BridgeOutboxKindColonySummary, "colony")["nextSession"]; next != nil {
		t.Fatalf("nextSession after deleting the series = %v", next)
	}
}
