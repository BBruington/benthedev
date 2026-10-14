package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"

	"github.com/BBruington/colony-planner/internal/core"
)

const noUpcomingSessionMessage = "I don't see any sessions coming up."

// discordTimestamp is Discord's <t:…:style>, which every reader sees in their own time zone and language.
func discordTimestamp(instant time.Time, style string) string {
	return fmt.Sprintf("<t:%d:%s>", instant.Unix(), style)
}

// sessionWhen is "<t:…:F> (<t:…:R>)": the full date and time, then how far away it is.
func sessionWhen(startsAt time.Time) string {
	return discordTimestamp(startsAt, "F") + " (" + discordTimestamp(startsAt, "R") + ")"
}

// nextSession is /nextsession: the next SCHEDULED date from core's occurrences, the same one the board
// shows. Sessions have no secrets, so the player projection changes nothing here.
func nextSession(requestContext context.Context, service colonyService, viewer core.Viewer, _ discordgo.ApplicationCommandInteractionData) (*discordgo.InteractionResponseData, error) {
	session, err := service.NextSession(requestContext, viewer)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return &discordgo.InteractionResponseData{Content: noUpcomingSessionMessage}, nil
	}
	return &discordgo.InteractionResponseData{
		Content: fmt.Sprintf("The next session, **%s**, starts %s.", escapeMarkdown(session.Title), sessionWhen(session.StartsAt)),
	}, nil
}

// colonySummary is /colony: the board as players see it (day, stockpile, colonists with Unassigned,
// next shipment, next session) and a link to the site. The viewer is already the player projection.
func colonySummary(publicURL string) func(context.Context, colonyService, core.Viewer, discordgo.ApplicationCommandInteractionData) (*discordgo.InteractionResponseData, error) {
	return func(requestContext context.Context, service colonyService, viewer core.Viewer, _ discordgo.ApplicationCommandInteractionData) (*discordgo.InteractionResponseData, error) {
		board, err := service.GetColonyBoard(requestContext, viewer)
		if err != nil {
			return nil, err
		}
		session, err := service.NextSession(requestContext, viewer)
		if err != nil {
			return nil, err
		}
		target, linked, err := service.DiscordTargetFor(requestContext, viewer.CampaignID())
		if err != nil {
			return nil, err
		}
		siteLink := ""
		if linked && publicURL != "" {
			// The slug is a-z, 0-9 and dashes (core checks it), so it goes in the link unescaped.
			siteLink = publicURL + "/c/" + target.CampaignSlug
		}
		return &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{colonyEmbed(board, session, siteLink)}}, nil
	}
}

func colonyEmbed(board core.ColonyBoard, session *core.Session, siteLink string) *discordgo.MessageEmbed {
	title := fitText(board.ColonyName, embedTitleMaxLength, escapeMarkdown)
	description := "Day " + strconv.FormatInt(board.Day, 10)
	fields := embedFields{remaining: embedMaxLength - utf8.RuneCountInString(title) - utf8.RuneCountInString(description)}

	stockpile := make([]string, 0, len(board.Resources))
	for _, resource := range board.Resources {
		stockpile = append(stockpile, escapeMarkdown(resource.Label)+": "+strconv.FormatInt(resource.Amount, 10))
	}
	fields.add("Stockpile", strings.Join(stockpile, "\n"), false, keepText)
	fields.add("Colonists", fmt.Sprintf("%d (%d unassigned)", board.Colonists, board.Unassigned), true, keepText)
	fields.add("Next shipment", shipmentText(board.Shipments), true, keepText)
	nextSessionText := "None scheduled"
	if session != nil {
		nextSessionText = escapeMarkdown(session.Title) + "\n" + sessionWhen(session.StartsAt)
	}
	fields.add("Next session", nextSessionText, false, keepText)
	return &discordgo.MessageEmbed{Title: title, URL: siteLink, Description: description, Fields: fields.fields}
}

// shipmentText is the board's "Day D (in X days)".
func shipmentText(shipments *core.ShipmentSchedule) string {
	if shipments == nil {
		return "None scheduled"
	}
	unit := "days"
	if shipments.DaysUntil == 1 {
		unit = "day"
	}
	return fmt.Sprintf("Day %d (in %d %s)", shipments.NextDay, shipments.DaysUntil, unit)
}
