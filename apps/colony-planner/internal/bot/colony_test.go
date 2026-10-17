package bot

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"

	"github.com/BBruington/colony-planner/internal/core"
)

func embedField(embed *discordgo.MessageEmbed, name string) string {
	for _, field := range embed.Fields {
		if field.Name == name {
			return field.Value
		}
	}
	return ""
}

func TestNextSessionShowsTheNextScheduledDate(t *testing.T) {
	test := newWorld(t)
	test.tableReady(guildX, test.dmA, channelXGeneral, nil)
	if reply := test.run(slashCommand(guildX, activeUserID, "nextsession")); reply != noUpcomingSessionMessage {
		t.Errorf("/nextsession with no series = %q", reply)
	}

	series := test.createSeries(test.dmA, weekly(fridaysTitle, 5, "2026-10-16"))
	if _, err := test.service.CancelSessionDate(test.context, test.dmA, core.SessionDateInput{SeriesID: series.ID, LocalDate: "2026-10-16"}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	test.bot.posts.Wait()
	secondFridayStart := firstFridayStart.AddDate(0, 0, 7)
	expected := fmt.Sprintf("The next session, **%s**, starts <t:%d:F> (<t:%d:R>).", fridaysTitle, secondFridayStart.Unix(), secondFridayStart.Unix())
	for _, discordUserID := range []string{activeUserID, dmUserID} {
		if reply := test.run(slashCommand(guildX, discordUserID, "nextsession")); reply != expected {
			t.Errorf("/nextsession for %s = %q, want %q", discordUserID, reply, expected)
		}
	}
	// Ana is in both campaigns; B's server answers with B's schedule.
	test.tableReady(guildY, test.dmB, channelYDesk, nil)
	if reply := test.run(slashCommand(guildY, activeUserID, "nextsession")); reply != noUpcomingSessionMessage {
		t.Errorf("/nextsession in B's server = %q, want B's empty schedule", reply)
	}
	if reply := test.run(slashCommand(guildX, pendingUserID, "nextsession")); reply != core.ErrNotLetIn.Message {
		t.Errorf("/nextsession for a member not yet let in = %q", reply)
	}
}

func TestColonyShowsTheBoardPlayersSee(t *testing.T) {
	test := newWorld(t)
	test.tableReady(guildX, test.dmA, channelXGeneral, nil)
	test.createSeries(test.dmA, weekly(fridaysTitle, 5, "2026-10-16"))
	player := test.enter(test.signIn(activeUserID, "ana"), test.dmA.CampaignID())
	board, err := test.service.GetColonyBoard(test.context, player)
	if err != nil {
		t.Fatal(err)
	}

	for _, discordUserID := range []string{activeUserID, dmUserID} {
		response := test.runFull(slashCommand(guildX, discordUserID, "colony"))
		if len(response.Data.Embeds) != 1 {
			t.Fatalf("/colony for %s answered %d embeds, want 1", discordUserID, len(response.Data.Embeds))
		}
		embed := response.Data.Embeds[0]
		if embed.Title != escapeMarkdown(board.ColonyName) || embed.URL != testPublicURL+"/c/"+campaignASlug || embed.Description != fmt.Sprintf("Day %d", board.Day) {
			t.Errorf("/colony embed = %q, %q, %q", embed.Title, embed.URL, embed.Description)
		}
		for _, resource := range board.Resources {
			if line := fmt.Sprintf("%s: %d", escapeMarkdown(resource.Label), resource.Amount); !strings.Contains(embedField(embed, "Stockpile"), line) {
				t.Errorf("the stockpile lacks %q: %q", line, embedField(embed, "Stockpile"))
			}
		}
		if colonists := fmt.Sprintf("%d (%d unassigned)", board.Colonists, board.Unassigned); embedField(embed, "Colonists") != colonists {
			t.Errorf("colonists = %q, want %q", embedField(embed, "Colonists"), colonists)
		}
		if shipment := embedField(embed, "Next shipment"); shipment != shipmentText(board.Shipments) {
			t.Errorf("next shipment = %q", shipment)
		}
		if session := embedField(embed, "Next session"); session != fridaysTitle+"\n"+sessionWhen(firstFridayStart) {
			t.Errorf("next session = %q", session)
		}
	}
}

func TestShipmentTextReadsLikeTheBoard(t *testing.T) {
	for _, testCase := range []struct {
		shipments *core.ShipmentSchedule
		expected  string
	}{
		{nil, "None scheduled"},
		{&core.ShipmentSchedule{IntervalDays: 7, NextDay: 14, DaysUntil: 1}, "Day 14 (in 1 day)"},
		{&core.ShipmentSchedule{IntervalDays: 7, NextDay: 14, DaysUntil: 6}, "Day 14 (in 6 days)"},
	} {
		if got := shipmentText(testCase.shipments); got != testCase.expected {
			t.Errorf("shipmentText(%+v) = %q, want %q", testCase.shipments, got, testCase.expected)
		}
	}
}
