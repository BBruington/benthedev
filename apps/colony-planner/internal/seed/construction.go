package seed

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

// sampleConstruction is Sample Colony's catalog and its one project, played through core with the
// advances: the DM drafts a revealed Granary and Palisade and a hidden Warded Tower; Thorin starts the
// Palisade on Day 14 with 2 Builders, who put in 10 Builder-days by Day 19 and then join the
// saltmarsh survey; on Day 22 the DM puts all 3 Builders back on it.
type sampleConstruction struct {
	service           *core.Service
	dm                core.Viewer
	enter             enterAs
	palisadeBlueprint core.BuildingBlueprint
	palisadeID        uuid.UUID
}

func newSampleConstruction(requestContext context.Context, service *core.Service, dm core.Viewer, enter enterAs) (*sampleConstruction, error) {
	maxCrew := int64(4)
	blueprints := []core.NewBlueprintInput{
		{Revealed: true, BlueprintInput: core.BlueprintInput{
			Name:              "Granary",
			Description:       "A raised timber store that keeps the harvest dry and the rats out.",
			Cost:              map[string]int64{"materials": 30, "gold": 20},
			Labor:             map[string]core.BlueprintLaborInput{"BUILDER": {Effort: 40, MaxCrew: &maxCrew}},
			CompletionEffects: core.Effects{Resources: map[string]int64{"morale": 5}},
			SortOrder:         10,
		}},
		{Revealed: true, BlueprintInput: core.BlueprintInput{
			Name:        "Palisade",
			Description: "A ring of sharpened stakes around the longhouse and the fields.",
			Cost:        map[string]int64{"materials": 40},
			Labor:       map[string]core.BlueprintLaborInput{"BUILDER": {Effort: 60}},
			SortOrder:   20,
		}},
		{BlueprintInput: core.BlueprintInput{
			Name:        "Warded Tower",
			Description: "A stone tower carved with old sigils.",
			DmNotes:     "The wards keep the Drowned Chapel's cult out of the colony. Reveal it once the party learns of them.",
			Cost:        map[string]int64{"materials": 80, "gold": 60},
			Labor:       map[string]core.BlueprintLaborInput{"BUILDER": {Effort: 30}, "MAGE": {Effort: 10}},
			SortOrder:   30,
		}},
	}
	construction := &sampleConstruction{service: service, dm: dm, enter: enter}
	for _, input := range blueprints {
		blueprint, err := service.CreateBlueprint(requestContext, dm, input)
		if err != nil {
			return nil, fmt.Errorf("the %s blueprint: %w", input.Name, err)
		}
		if input.Name == "Palisade" {
			construction.palisadeBlueprint = blueprint
		}
	}
	return construction, nil
}

func (construction *sampleConstruction) startPalisade(requestContext context.Context) func(int64) (core.ColonyChange, error) {
	return func(expectedVersion int64) (core.ColonyChange, error) {
		thorin, err := construction.enter("seed:ana")
		if err != nil {
			return core.ColonyChange{}, err
		}
		change, err := construction.service.StartBuilding(requestContext, thorin, core.StartBuildingInput{
			ExpectedVersion:    expectedVersion,
			BlueprintID:        construction.palisadeBlueprint.ID,
			BlueprintUpdatedAt: construction.palisadeBlueprint.UpdatedAt,
			Crews:              map[string]int64{"BUILDER": 2},
		})
		if err == nil {
			construction.palisadeID = *change.Entries[0].BuildingID
		}
		return change, err
	}
}

// crewPalisade sets the started Palisade's Builders.
func (construction *sampleConstruction) crewPalisade(requestContext context.Context, builders int64) func(int64) (core.ColonyChange, error) {
	return func(expectedVersion int64) (core.ColonyChange, error) {
		return construction.service.SetBuildingCrews(requestContext, construction.dm, construction.palisadeID, core.BuildingCrewsInput{
			ExpectedVersion: expectedVersion, Crews: map[string]int64{"BUILDER": builders},
		})
	}
}

// seedSecondConstruction gives Ridgehold one revealed blueprint for its miners and nothing under way.
func seedSecondConstruction(requestContext context.Context, service *core.Service, dm core.Viewer) error {
	maxCrew := int64(3)
	_, err := service.CreateBlueprint(requestContext, dm, core.NewBlueprintInput{Revealed: true, BlueprintInput: core.BlueprintInput{
		Name:        "Mine Hoist",
		Description: "A timber headframe and winch over the old shaft.",
		Cost:        map[string]int64{"timber": 20, "materials": 10},
		Labor:       map[string]core.BlueprintLaborInput{"MINER": {Effort: 30, MaxCrew: &maxCrew}},
	}})
	if err != nil {
		return fmt.Errorf("the mine hoist blueprint: %w", err)
	}
	return nil
}
