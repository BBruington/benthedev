package seed

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

// samplePlace and sampleNpc are the M2 sample cast (spec §9). Fictional on their face, reusing the
// spec's example names where it has them.
type samplePlace struct {
	input core.NewPlaceInput
}

type sampleNpc struct {
	input core.NewNpcInput
	// currentPlace and originPlace name a samplePlace by its name; blank for none.
	currentPlace string
	originPlace  string
	// colonyRole makes them a named colonist by naming one of that role's unnamed colonists, so the
	// colonist total, and with it every M1 number, stays as it was.
	colonyRole string
}

var sampleColonyPlaces = []samplePlace{
	{core.NewPlaceInput{Revealed: true, PlaceInput: core.PlaceInput{Name: "Port Eldari Docks", Description: "Two timber jetties and a crane that groans in the wind.", PlayerNotes: "Ships from the mainland tie up here."}}},
	{core.NewPlaceInput{Revealed: true, PlaceInput: core.PlaceInput{Name: "The Longhouse", Description: "The colony's hall, kitchen and sickroom under one turf roof."}}},
	{core.NewPlaceInput{Revealed: true, PlaceInput: core.PlaceInput{Name: "Saltmarsh Fields", Description: "Reclaimed marsh, drained by ditches the settlers cut by hand."}}},
	{core.NewPlaceInput{Revealed: true, PlaceInput: core.PlaceInput{Name: "Wolf Den Hollow", Description: "A wooded dell north of the fields. Howling most nights."}}},
	{core.NewPlaceInput{Revealed: false, PlaceInput: core.PlaceInput{Name: "The Drowned Chapel", Description: "A half-sunken chapel older than any map of this coast.", DmNotes: "The cult meets here at the new moon."}}},
}

var sampleColonyNpcs = []sampleNpc{
	{input: core.NewNpcInput{Revealed: true, NpcInput: core.NpcInput{Name: "Brannoc", Aliases: []string{"Bran"}, Labels: []string{"settler"}, Race: "Human", Age: "mid-40s",
		Appearance: "Broad, sunburnt, mud to the knees.", Personality: "Slow to speak, quick to help.",
		Status: core.NpcStatusAlive, Health: core.NpcHealthHealthy, Relation: core.NpcRelationAlly}},
		currentPlace: "Saltmarsh Fields", colonyRole: "FARMER"},
	{input: core.NewNpcInput{Revealed: true, NpcInput: core.NpcInput{Name: "Sister Maren", Labels: []string{"settler"}, Race: "Half-elf", CharacterClass: "Cleric", Level: level(3),
		Personality: "Brisk, kind, unimpressed by heroics.", Status: core.NpcStatusAlive, Health: core.NpcHealthHealthy, Relation: core.NpcRelationAlly}},
		currentPlace: "The Longhouse", colonyRole: "HEALER"},
	{input: core.NewNpcInput{Revealed: false, NpcInput: core.NpcInput{Name: "Edric Vael", KnownName: "The Quiet Mason", Labels: []string{"settler"}, Race: "Human",
		DmNotes: "Fled the mainland after the guild hall fire. The Crown still wants him.",
		Status:  core.NpcStatusAlive, Health: core.NpcHealthHealthy, Relation: core.NpcRelationNeutral}},
		colonyRole: "BUILDER"},
	{input: core.NewNpcInput{Revealed: true, NpcInput: core.NpcInput{Name: "Lady Vex", Labels: []string{"patron", "noble"}, Race: "Human",
		Backstory: "A mainland noble who paid for half the colony's charter.", DmNotes: "She wants the silver she believes lies under the ridge.",
		Status: core.NpcStatusAlive, Health: core.NpcHealthHealthy, Relation: core.NpcRelationAlly}}},
	{input: core.NewNpcInput{Revealed: true, NpcInput: core.NpcInput{Name: "Orsolya Dane", Aliases: []string{"the Captain"}, Labels: []string{"sailor"}, Race: "Human", CharacterClass: "Fighter", Level: level(5),
		Status: core.NpcStatusAlive, Health: core.NpcHealthInjured, Relation: core.NpcRelationNeutral, PlayerNotes: "Captain of the supply ship. Owes Thorin a drink."}},
		currentPlace: "Port Eldari Docks"},
	{input: core.NewNpcInput{Revealed: true, NpcInput: core.NpcInput{Name: "Grey Tam", Labels: []string{"settler"}, Race: "Human", Backstory: "Drowned in the first storm.",
		Status: core.NpcStatusDead, Health: core.NpcHealthDead, Relation: core.NpcRelationAlly}}},
	{input: core.NewNpcInput{Revealed: true, NpcInput: core.NpcInput{Name: "Selwyn Hart", KnownName: "The Hooded Stranger", Aliases: []string{"the stranger in grey"}, Race: "Elf",
		Appearance: "Grey cloak, hood always up.", DmNotes: "A spy for the Crown, sent to watch Edric Vael.",
		Status: core.NpcStatusAlive, Health: core.NpcHealthHealthy, Relation: core.NpcRelationSuspicious}},
		currentPlace: "Port Eldari Docks"},
	{input: core.NewNpcInput{Revealed: false, NpcInput: core.NpcInput{Name: "Mother Corrow", Labels: []string{"cult"}, Race: "Human", CharacterClass: "Warlock", Level: level(7),
		DmNotes: "Leads the drowned cult. Wants the chapel's bell rung again.",
		Status:  core.NpcStatusAlive, Health: core.NpcHealthHealthy, Relation: core.NpcRelationEnemy}},
		currentPlace: "The Drowned Chapel"},
	{input: core.NewNpcInput{Revealed: true, NpcInput: core.NpcInput{Name: "Pip Lark", Race: "Halfling", Age: "about 12", Personality: "Into everything.",
		Status: core.NpcStatusAlive, Health: core.NpcHealthHealthy, Relation: core.NpcRelationNeutral}},
		currentPlace: "The Longhouse"},
	{input: core.NewNpcInput{Revealed: false, NpcInput: core.NpcInput{Name: "Hrothgar", KnownName: "The Wolf-Father", Race: "Werewolf",
		DmNotes: "Was a settler on the first ship. Nobody has noticed he is missing.",
		Status:  core.NpcStatusAlive, Health: core.NpcHealthHealthy, Relation: core.NpcRelationEnemy}},
		currentPlace: "Wolf Den Hollow"},
}

var secondSamplePlaces = []samplePlace{
	{core.NewPlaceInput{Revealed: true, PlaceInput: core.PlaceInput{Name: "Ridgehold Gate", Description: "A palisade gate on the only path up."}}},
	{core.NewPlaceInput{Revealed: true, PlaceInput: core.PlaceInput{Name: "The Old Mine", Description: "Dug out long before the settlers came."}}},
	{core.NewPlaceInput{Revealed: false, PlaceInput: core.PlaceInput{Name: "Collapsed Shaft", Description: "Rubble, and a draught from below.", DmNotes: "Leads to the under-ridge."}}},
}

var secondSampleNpcs = []sampleNpc{
	{input: core.NewNpcInput{Revealed: true, NpcInput: core.NpcInput{Name: "Foreman Dagny", Race: "Dwarf", Status: core.NpcStatusAlive, Health: core.NpcHealthHealthy, Relation: core.NpcRelationAlly}},
		currentPlace: "The Old Mine", colonyRole: "MINER"},
	{input: core.NewNpcInput{Revealed: true, NpcInput: core.NpcInput{Name: "Old Brisk", Race: "Human", Age: "seventy if a day", Status: core.NpcStatusAlive, Health: core.NpcHealthSick, Relation: core.NpcRelationNeutral}},
		currentPlace: "Ridgehold Gate"},
	{input: core.NewNpcInput{Revealed: true, NpcInput: core.NpcInput{Name: "Aldo Pike", KnownName: "The Lantern Man", Race: "Human", DmNotes: "Knows the way through the collapsed shaft.",
		Status: core.NpcStatusUnknown, Health: core.NpcHealthUnknown, Relation: core.NpcRelationSuspicious}},
		originPlace: "Collapsed Shaft"},
	{input: core.NewNpcInput{Revealed: false, NpcInput: core.NpcInput{Name: "The Thing Below", DmNotes: "Not yet. Not for a long while.", Status: core.NpcStatusUnknown, Health: core.NpcHealthUnknown, Relation: core.NpcRelationEnemy}},
		currentPlace: "Collapsed Shaft"},
}

// seedPeople adds the places and NPCs through core, then makes the named colonists join their roles
// as colony mutations, so their entries and the undo slot are what real play writes.
func seedPeople(requestContext context.Context, service *core.Service, dm core.Viewer, places []samplePlace, npcs []sampleNpc) error {
	placeIDs := map[string]uuid.UUID{}
	for _, sample := range places {
		place, err := service.CreatePlace(requestContext, dm, sample.input)
		if err != nil {
			return fmt.Errorf("place %s: %w", sample.input.Name, err)
		}
		placeIDs[sample.input.Name] = place.ID
	}
	placeReference := func(name string) *uuid.UUID {
		if name == "" {
			return nil
		}
		placeID := placeIDs[name]
		return &placeID
	}
	for _, sample := range npcs {
		input := sample.input
		input.CurrentPlaceID = placeReference(sample.currentPlace)
		input.OriginPlaceID = placeReference(sample.originPlace)
		npc, err := service.CreateNpc(requestContext, dm, input)
		if err != nil {
			return fmt.Errorf("NPC %s: %w", input.Name, err)
		}
		if sample.colonyRole == "" {
			continue
		}
		board, err := service.GetColonyBoard(requestContext, dm)
		if err != nil {
			return err
		}
		if _, err := service.JoinColony(requestContext, dm, npc.ID, core.JoinColonyInput{
			ExpectedVersion: board.Version,
			RoleKey:         sample.colonyRole,
			Mode:            core.JoinModePromoteUnnamed,
		}); err != nil {
			return fmt.Errorf("%s joins the colony: %w", input.Name, err)
		}
	}
	return nil
}

func level(value int64) *int64 {
	return &value
}
