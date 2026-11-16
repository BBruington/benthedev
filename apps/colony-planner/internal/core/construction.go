package core

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/BBruington/colony-planner/internal/game"
	"github.com/BBruington/colony-planner/internal/store"
)

// BuildingStatus is the Postgres building_status enum: a project is CONSTRUCTING until Advance
// finishes it (FINISHED) or the DM cancels it (CANCELLED); the DM demolishes a finished one.
type BuildingStatus = store.BuildingStatus

const (
	BuildingStatusConstructing = store.BuildingStatusCONSTRUCTING
	BuildingStatusFinished     = store.BuildingStatusFINISHED
	BuildingStatusCancelled    = store.BuildingStatusCANCELLED
	BuildingStatusDemolished   = store.BuildingStatusDEMOLISHED
)

// AllBuildingStatusValues lists every status, for the API's enum schema.
func AllBuildingStatusValues() []BuildingStatus {
	return store.AllBuildingStatusValues()
}

// ConstructionPage is the Buildings page (docs/design/construction.md §6.2) in the viewer's
// projection. Players get only revealed, unarchived blueprints and the buildings of revealed ones, and
// a hidden project's crews count only as busy elsewhere in Roles.
type ConstructionPage struct {
	Version    int64               `json:"version" doc:"The colony version this read saw; Start and Set crews send it as expectedVersion"`
	Day        int64               `json:"day"`
	Blueprints []BuildingBlueprint `json:"blueprints" doc:"Unarchived first, then by position and name"`
	Buildings  []Building          `json:"buildings" doc:"Every status, oldest first"`
	Roles      []ConstructionRole  `json:"roles" doc:"The construction role and every role a listed blueprint or building needs, in display order"`
}

// BuildingBlueprint is one building type in the DM's catalog.
type BuildingBlueprint struct {
	ID                uuid.UUID                  `json:"id" format:"uuid"`
	Name              string                     `json:"name"`
	Description       string                     `json:"description"`
	Cost              map[string]int64           `json:"cost" doc:"Resource key to the amount Start takes"`
	Labor             []BlueprintLabor           `json:"labor" doc:"By role key"`
	CompletionEffects Effects                    `json:"completionEffects" doc:"Applied once, capped at each maximum, on the day a building of it finishes"`
	Output            map[string]BlueprintOutput `json:"output" doc:"Resource key to what each Active standing building of it does for that resource; live, so an edit changes standing buildings from the next day"`
	Archived          bool                       `json:"archived"`
	SortOrder         int64                      `json:"sortOrder"`
	// UpdatedAt goes back in StartBuildingInput, so a Start sheet that showed an older cost is refused.
	UpdatedAt time.Time            `json:"updatedAt"`
	Dm        *BuildingBlueprintDm `json:"dm,omitempty"`
}

// BuildingBlueprintDm is what only the DM sees of a blueprint.
type BuildingBlueprintDm struct {
	Revealed bool   `json:"revealed"`
	DmNotes  string `json:"dmNotes"`
}

// BlueprintLabor is one role's work on a blueprint: effort role-days, with at most maxCrew at once.
type BlueprintLabor struct {
	RoleKey string `json:"roleKey"`
	Effort  int64  `json:"effort"`
	MaxCrew *int64 `json:"maxCrew" doc:"null for no cap"`
}

// BlueprintOutput is what a building of a blueprint does for one resource while it stands Active.
type BlueprintOutput struct {
	DailyDelta   int64 `json:"dailyDelta" doc:"Added each day, capped at the effective max; negative is upkeep, floored at the minimum"`
	StorageBonus int64 `json:"storageBonus" doc:"0 or more, added to the resource's max; nothing for a resource with no max"`
}

// Building is one project or standing building. Cost, labor and completion Effects are what its
// blueprint said when it started (K18); its name is the blueprint's current one.
type Building struct {
	ID                 uuid.UUID                  `json:"id" format:"uuid"`
	BlueprintID        uuid.UUID                  `json:"blueprintId" format:"uuid"`
	Name               string                     `json:"name"`
	Status             BuildingStatus             `json:"status"`
	Active             bool                       `json:"active" doc:"Meaningful only while FINISHED: an Idle building (false) gives no output and no storage bonus"`
	Added              bool                       `json:"added" doc:"The DM placed it with Add building, for free, rather than building it"`
	Output             map[string]BlueprintOutput `json:"output" doc:"Its blueprint's live output, which it gives while it stands Active"`
	StartedDay         int64                      `json:"startedDay"`
	FinishedDay        *int64                     `json:"finishedDay" doc:"null until it finishes"`
	EndedDay           *int64                     `json:"endedDay" doc:"The day it was cancelled or demolished; null otherwise"`
	Cost               map[string]int64           `json:"cost" doc:"Resource key to the amount Start took"`
	CompletionEffects  Effects                    `json:"completionEffects"`
	Refund             map[string]int64           `json:"refund" nullable:"true" doc:"What cancelling or demolishing gave back; null otherwise"`
	Labor              []BuildingLabor            `json:"labor" doc:"By role key"`
	ProjectedFinishDay *int64                     `json:"projectedFinishDay" doc:"The finish day with the current crews; null when it isn't under construction or a line with work left has no crew"`
}

// BuildingLabor is one labor line of a building: progress of effort role-days, crew working now.
type BuildingLabor struct {
	RoleKey  string `json:"roleKey"`
	Effort   int64  `json:"effort"`
	MaxCrew  *int64 `json:"maxCrew" doc:"null for no cap"`
	Progress int64  `json:"progress"`
	Crew     int64  `json:"crew"`
}

// ConstructionRole is one role's colonists as the Buildings page counts them: crewed on the projects
// listed here, and idle (headcount minus every crew, hidden projects' included).
type ConstructionRole struct {
	RoleKey   string `json:"roleKey"`
	Label     string `json:"label"`
	Headcount int64  `json:"headcount"`
	Crewed    int64  `json:"crewed" doc:"On the projects this page lists"`
	Idle      int64  `json:"idle" doc:"Headcount minus every crew"`
}

// BlueprintInput replaces a blueprint's record (PUT semantics). Visibility and archiving each have
// their own operation, so a stale edit form can't undo them.
type BlueprintInput struct {
	Name              string                         `json:"name" doc:"1 to 80 characters, unique in the campaign"`
	Description       string                         `json:"description" doc:"At most 4000 characters"`
	DmNotes           string                         `json:"dmNotes" doc:"At most 4000 characters"`
	Cost              map[string]int64               `json:"cost" doc:"Resource key to an amount of 1 or more; {} for none"`
	Labor             map[string]BlueprintLaborInput `json:"labor" doc:"Role key to its work; 1 to 10 roles"`
	CompletionEffects Effects                        `json:"completionEffects"`
	Output            map[string]BlueprintOutput     `json:"output" doc:"Resource key to its daily change and storage bonus (0 or more), not both 0; {} for none. Required, since a full-record edit without it would clear the output"`
	SortOrder         int64                          `json:"sortOrder"`
}

// BlueprintLaborInput is one role's work: effort role-days, and an optional cap on its crew.
type BlueprintLaborInput struct {
	Effort  int64  `json:"effort" doc:"1 or more role-days"`
	MaxCrew *int64 `json:"maxCrew" nullable:"true" doc:"1 or more; null for no cap"`
}

type NewBlueprintInput struct {
	BlueprintInput
	Revealed bool `json:"revealed" doc:"Visible to players; the create form starts it off"`
}

// StartBuildingInput starts a project of a blueprint the viewer can see, with its first crews.
type StartBuildingInput struct {
	ExpectedVersion    int64            `json:"expectedVersion"`
	BlueprintID        uuid.UUID        `json:"blueprintId" format:"uuid"`
	BlueprintUpdatedAt time.Time        `json:"blueprintUpdatedAt" doc:"The blueprint's updatedAt as the Start sheet showed it; a newer blueprint is a 409"`
	Crews              map[string]int64 `json:"crews" doc:"Role key to crew; a labor role left out starts with none"`
}

// BuildingCrewsInput is one project's whole crew map (PUT semantics): a role left out gets none.
type BuildingCrewsInput struct {
	ExpectedVersion int64            `json:"expectedVersion"`
	Crews           map[string]int64 `json:"crews" doc:"Role key to crew"`
}

// AddBuildingInput adds a building of a blueprint at once, for free (K19).
type AddBuildingInput struct {
	ExpectedVersion int64 `json:"expectedVersion"`
}

// BuildingVersionInput is a colony change on one building that needs nothing but the version.
type BuildingVersionInput struct {
	ExpectedVersion int64 `json:"expectedVersion"`
}

// BuildingRefundInput is what the DM gives back when cancelling or demolishing; it applies capped.
type BuildingRefundInput struct {
	ExpectedVersion int64            `json:"expectedVersion"`
	Refund          map[string]int64 `json:"refund" doc:"Resource key to an amount of 0 or more; {} for nothing"`
}

// buildingBefore is a building as it was before a colony mutation changed it, for undo. A building a
// Start created did not exist before, so undo deletes it. Active is nil in a slot written before
// buildings had it, which leaves it as it is.
type buildingBefore struct {
	Created     bool                   `json:"created"`
	Status      BuildingStatus         `json:"status"`
	Active      *bool                  `json:"active,omitempty"`
	FinishedDay *int32                 `json:"finishedDay"`
	EndedDay    *int32                 `json:"endedDay"`
	Refund      *Effects               `json:"refund"`
	Lines       map[string]laborBefore `json:"lines"`
}

type laborBefore struct {
	Progress int64 `json:"progress"`
	Crew     int64 `json:"crew"`
}

const (
	blueprintNameMaxLength  = 80
	blueprintProseMaxLength = 4000
	blueprintLaborMaxLines  = 10
)

var (
	errBlueprintNotFound = NotFound("That blueprint doesn't exist.")
	errBuildingNotFound  = NotFound("That building doesn't exist.")
	errBlueprintLimit    = Conflict("This campaign has " + formatNumber(BlueprintLimit) + " blueprints, the most it can hold.")
	errBuildingLimit     = Conflict("This campaign has " + formatNumber(UnfinishedBuildingLimit) +
		" buildings under construction, the most it can hold. Finish or cancel one first.")
)

// ListConstruction is the Buildings page for every active member.
func (service *Service) ListConstruction(requestContext context.Context, viewer Viewer) (ConstructionPage, error) {
	if err := requireActive(viewer); err != nil {
		return ConstructionPage{}, err
	}
	return readConstructionPage(requestContext, service.queries, viewer)
}

func readConstructionPage(requestContext context.Context, queries *store.Queries, viewer Viewer) (ConstructionPage, error) {
	campaignID, dmProjection := viewer.campaignID, viewer.SeesDmFields()
	colony, err := queries.GetColony(requestContext, campaignID)
	if err != nil {
		return ConstructionPage{}, campaignGone(err)
	}
	settings, err := queries.GetSettings(requestContext, campaignID)
	if err != nil {
		return ConstructionPage{}, campaignGone(err)
	}
	blueprints, err := listBlueprints(requestContext, queries, campaignID, dmProjection)
	if err != nil {
		return ConstructionPage{}, err
	}
	buildings, err := listBuildings(requestContext, queries, campaignID, dmProjection, int64(colony.Day))
	if err != nil {
		return ConstructionPage{}, err
	}
	unfinished, err := queries.ListUnfinishedBuildingLabor(requestContext, campaignID)
	if err != nil {
		return ConstructionPage{}, err
	}
	roles, err := queries.ListRoles(requestContext, campaignID)
	if err != nil {
		return ConstructionPage{}, err
	}
	relevant := map[string]bool{}
	if settings.ConstructionRoleKey != nil {
		relevant[*settings.ConstructionRoleKey] = true
	}
	for _, blueprint := range blueprints {
		for _, line := range blueprint.Labor {
			relevant[line.RoleKey] = true
		}
	}
	crewed := map[string]int64{}
	for _, building := range buildings {
		for _, line := range building.Labor {
			relevant[line.RoleKey] = true
			if building.Status == BuildingStatusConstructing {
				crewed[line.RoleKey] += line.Crew
			}
		}
	}
	allCrews := map[string]int64{}
	for _, row := range unfinished {
		allCrews[row.BuildingLabor.RoleKey] += int64(row.BuildingLabor.Crew)
	}
	page := ConstructionPage{
		Version:    colony.Version,
		Day:        int64(colony.Day),
		Blueprints: blueprints,
		Buildings:  buildings,
		Roles:      []ConstructionRole{},
	}
	for _, row := range roles {
		if !relevant[row.Role.Key] {
			continue
		}
		headcount := int64(row.Role.UnnamedCount) + row.NamedCount
		page.Roles = append(page.Roles, ConstructionRole{
			RoleKey:   row.Role.Key,
			Label:     row.Role.Label,
			Headcount: headcount,
			Crewed:    crewed[row.Role.Key],
			Idle:      max(headcount-allCrews[row.Role.Key], 0),
		})
	}
	return page, nil
}

// listBlueprints reads the projection's blueprints with their cost and labor lines.
func listBlueprints(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, dmProjection bool) ([]BuildingBlueprint, error) {
	rows, err := queries.ListBlueprints(requestContext, store.ListBlueprintsParams{CampaignID: campaignID, DmProjection: dmProjection})
	if err != nil {
		return nil, err
	}
	lines, err := readBlueprintLines(requestContext, queries, campaignID)
	if err != nil {
		return nil, err
	}
	blueprints := make([]BuildingBlueprint, 0, len(rows))
	for _, row := range rows {
		blueprint, err := toBlueprint(row, lines)
		if err != nil {
			return nil, err
		}
		blueprints = append(blueprints, blueprint)
	}
	return blueprints, nil
}

// blueprintLines is every blueprint's cost, labor and output rows, keyed by blueprint ID.
type blueprintLines struct {
	costs   map[uuid.UUID][]store.BuildingBlueprintCost
	labor   map[uuid.UUID][]store.BuildingBlueprintLabor
	outputs map[uuid.UUID]map[string]game.ResourceOutput
}

func readBlueprintLines(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID) (blueprintLines, error) {
	costRows, err := queries.ListBlueprintCosts(requestContext, campaignID)
	if err != nil {
		return blueprintLines{}, err
	}
	laborRows, err := queries.ListBlueprintLabor(requestContext, campaignID)
	if err != nil {
		return blueprintLines{}, err
	}
	outputs, err := readBlueprintOutputs(requestContext, queries, campaignID)
	if err != nil {
		return blueprintLines{}, err
	}
	lines := blueprintLines{costs: map[uuid.UUID][]store.BuildingBlueprintCost{}, labor: map[uuid.UUID][]store.BuildingBlueprintLabor{}, outputs: outputs}
	for _, row := range costRows {
		lines.costs[row.BlueprintID] = append(lines.costs[row.BlueprintID], row)
	}
	for _, row := range laborRows {
		lines.labor[row.BlueprintID] = append(lines.labor[row.BlueprintID], row)
	}
	return lines, nil
}

// readBlueprintOutputs is every blueprint's live output, keyed by blueprint ID and resource key.
func readBlueprintOutputs(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID) (map[uuid.UUID]map[string]game.ResourceOutput, error) {
	rows, err := queries.ListBlueprintOutputs(requestContext, campaignID)
	if err != nil {
		return nil, err
	}
	outputs := map[uuid.UUID]map[string]game.ResourceOutput{}
	for _, row := range rows {
		if outputs[row.BlueprintID] == nil {
			outputs[row.BlueprintID] = map[string]game.ResourceOutput{}
		}
		outputs[row.BlueprintID][row.ResourceKey] = game.ResourceOutput{DailyDelta: int64(row.DailyDelta), StorageBonus: int64(row.StorageBonus)}
	}
	return outputs, nil
}

// toBlueprintOutput is a blueprint's output as the API shows it; never nil.
func toBlueprintOutput(outputs map[string]game.ResourceOutput) map[string]BlueprintOutput {
	shown := make(map[string]BlueprintOutput, len(outputs))
	for key, output := range outputs {
		shown[key] = BlueprintOutput{DailyDelta: output.DailyDelta, StorageBonus: output.StorageBonus}
	}
	return shown
}

// toBlueprint fills Dm unconditionally; the Transformer decides who keeps it.
func toBlueprint(row store.BuildingBlueprint, lines blueprintLines) (BuildingBlueprint, error) {
	costs, labor := lines.costs[row.ID], lines.labor[row.ID]
	completion, err := decodeEffects(row.CompletionEffects)
	if err != nil {
		return BuildingBlueprint{}, err
	}
	blueprint := BuildingBlueprint{
		ID:                row.ID,
		Name:              row.Name,
		Description:       row.Description,
		Cost:              make(map[string]int64, len(costs)),
		Labor:             make([]BlueprintLabor, 0, len(labor)),
		CompletionEffects: completion,
		Output:            toBlueprintOutput(lines.outputs[row.ID]),
		Archived:          row.Archived,
		SortOrder:         int64(row.SortOrder),
		UpdatedAt:         row.UpdatedAt.UTC(),
		Dm:                &BuildingBlueprintDm{Revealed: row.Revealed, DmNotes: row.DmNotes},
	}
	for _, cost := range costs {
		blueprint.Cost[cost.ResourceKey] = int64(cost.Amount)
	}
	for _, line := range labor {
		blueprint.Labor = append(blueprint.Labor, BlueprintLabor{RoleKey: line.RoleKey, Effort: int64(line.Effort), MaxCrew: int64Pointer(line.MaxCrew)})
	}
	return blueprint, nil
}

// listBuildings reads the projection's buildings with their labor lines and projected finish days.
func listBuildings(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, dmProjection bool, day int64) ([]Building, error) {
	rows, err := queries.ListBuildings(requestContext, store.ListBuildingsParams{CampaignID: campaignID, DmProjection: dmProjection})
	if err != nil {
		return nil, err
	}
	laborRows, err := queries.ListBuildingLabor(requestContext, campaignID)
	if err != nil {
		return nil, err
	}
	outputs, err := readBlueprintOutputs(requestContext, queries, campaignID)
	if err != nil {
		return nil, err
	}
	labor := map[uuid.UUID][]store.BuildingLabor{}
	for _, row := range laborRows {
		labor[row.BuildingID] = append(labor[row.BuildingID], row)
	}
	buildings := make([]Building, 0, len(rows))
	for _, row := range rows {
		building, err := toBuilding(row.Building, row.BlueprintName, labor[row.Building.ID], outputs[row.Building.BlueprintID], day)
		if err != nil {
			return nil, err
		}
		buildings = append(buildings, building)
	}
	return buildings, nil
}

func toBuilding(row store.Building, name string, labor []store.BuildingLabor, output map[string]game.ResourceOutput, day int64) (Building, error) {
	cost, err := decodeEffects(row.Cost)
	if err != nil {
		return Building{}, err
	}
	completion, err := decodeEffects(row.CompletionEffects)
	if err != nil {
		return Building{}, err
	}
	building := Building{
		ID:                row.ID,
		BlueprintID:       row.BlueprintID,
		Name:              name,
		Status:            row.Status,
		Active:            row.Active,
		Added:             row.Added,
		Output:            toBlueprintOutput(output),
		StartedDay:        int64(row.StartedDay),
		FinishedDay:       int64Pointer(row.FinishedDay),
		EndedDay:          int64Pointer(row.EndedDay),
		Cost:              make(map[string]int64, len(cost.Resources)),
		CompletionEffects: completion,
		Labor:             make([]BuildingLabor, 0, len(labor)),
	}
	for key, delta := range cost.Resources {
		building.Cost[key] = -delta
	}
	if row.Refund != nil {
		refund, err := decodeEffects(row.Refund)
		if err != nil {
			return Building{}, err
		}
		building.Refund = refund.Resources
	}
	for _, line := range labor {
		building.Labor = append(building.Labor, BuildingLabor{
			RoleKey: line.RoleKey, Effort: int64(line.Effort), MaxCrew: int64Pointer(line.MaxCrew), Progress: int64(line.Progress), Crew: int64(line.Crew),
		})
	}
	if row.Status == BuildingStatusConstructing {
		building.ProjectedFinishDay = game.ProjectedFinishDay(day, toProject(row.ID, labor, completion, nil))
	}
	return building, nil
}

func toProject(buildingID uuid.UUID, labor []store.BuildingLabor, completion Effects, output map[string]game.ResourceOutput) game.Project {
	project := game.Project{BuildingID: buildingID, Lines: make([]game.LaborLine, 0, len(labor)), CompletionEffects: completion, Output: output}
	for _, line := range labor {
		project.Lines = append(project.Lines, game.LaborLine{
			RoleKey: line.RoleKey, Effort: int64(line.Effort), MaxCrew: int64Pointer(line.MaxCrew), Progress: int64(line.Progress), Crew: int64(line.Crew),
		})
	}
	return project
}

// readProjects is the unfinished projects game.State carries, sorted by building ID, each with its
// blueprint's live output.
func readProjects(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, outputs map[uuid.UUID]map[string]game.ResourceOutput) ([]game.Project, error) {
	rows, err := queries.ListUnfinishedBuildingLabor(requestContext, campaignID)
	if err != nil {
		return nil, err
	}
	var projects []game.Project
	for index := 0; index < len(rows); {
		buildingID, blueprintID := rows[index].BuildingLabor.BuildingID, rows[index].BlueprintID
		completion, err := decodeEffects(rows[index].CompletionEffects)
		if err != nil {
			return nil, err
		}
		var labor []store.BuildingLabor
		for ; index < len(rows) && rows[index].BuildingLabor.BuildingID == buildingID; index++ {
			labor = append(labor, rows[index].BuildingLabor)
		}
		projects = append(projects, toProject(buildingID, labor, completion, maps.Clone(outputs[blueprintID])))
	}
	return projects, nil
}

// readStandingBuildings is the finished buildings game.State carries, sorted by building ID, each with
// its blueprint's live output.
func readStandingBuildings(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, outputs map[uuid.UUID]map[string]game.ResourceOutput) ([]game.StandingBuilding, error) {
	rows, err := queries.ListStandingBuildings(requestContext, campaignID)
	if err != nil {
		return nil, err
	}
	var buildings []game.StandingBuilding
	for _, row := range rows {
		buildings = append(buildings, game.StandingBuilding{BuildingID: row.ID, Active: row.Active, Output: maps.Clone(outputs[row.BlueprintID])})
	}
	return buildings, nil
}

// CreateBlueprint adds a blueprint to the DM's catalog, hidden unless asked otherwise. It is not a
// colony mutation and writes no entry.
func (service *Service) CreateBlueprint(requestContext context.Context, viewer Viewer, input NewBlueprintInput) (BuildingBlueprint, error) {
	if err := requireDm(viewer); err != nil {
		return BuildingBlueprint{}, err
	}
	return service.changeBlueprint(requestContext, viewer, func(queries *store.Queries) (uuid.UUID, error) {
		count, err := queries.CountBlueprints(requestContext, viewer.campaignID)
		if err != nil {
			return uuid.Nil, err
		}
		if count >= BlueprintLimit {
			return uuid.Nil, errBlueprintLimit
		}
		cleaned, err := cleanBlueprintInput(requestContext, queries, viewer.campaignID, uuid.Nil, input.BlueprintInput)
		if err != nil {
			return uuid.Nil, err
		}
		row, err := queries.CreateBlueprint(requestContext, store.CreateBlueprintParams{
			CampaignID:        viewer.campaignID,
			Name:              cleaned.name,
			Description:       cleaned.description,
			DmNotes:           cleaned.dmNotes,
			Revealed:          input.Revealed,
			CompletionEffects: cleaned.completionEffects,
			SortOrder:         cleaned.sortOrder,
		})
		if err != nil {
			return uuid.Nil, err
		}
		return row.ID, saveBlueprintLines(requestContext, queries, viewer.campaignID, row.ID, cleaned)
	})
}

// UpdateBlueprint replaces a blueprint's record. Started buildings keep the snapshot they took, but its
// output is live (K18): standing buildings' caps move with it, so the board and /settings change too.
func (service *Service) UpdateBlueprint(requestContext context.Context, viewer Viewer, blueprintID uuid.UUID, input BlueprintInput) (BuildingBlueprint, error) {
	if err := requireDm(viewer); err != nil {
		return BuildingBlueprint{}, err
	}
	return service.changeBlueprint(requestContext, viewer, func(queries *store.Queries) (uuid.UUID, error) {
		current, err := queries.GetBlueprint(requestContext, store.GetBlueprintParams{CampaignID: viewer.campaignID, ID: blueprintID})
		if err != nil {
			return uuid.Nil, blueprintGone(err)
		}
		cleaned, err := cleanBlueprintInput(requestContext, queries, viewer.campaignID, blueprintID, input)
		if err != nil {
			return uuid.Nil, err
		}
		// As an NPC rename does: entries written with the old name stay away from players once revealed.
		if !current.Revealed && cleaned.name != current.Name {
			if err := queries.HideBlueprintTimelineEvents(requestContext, store.HideBlueprintTimelineEventsParams{CampaignID: viewer.campaignID, BlueprintID: blueprintID}); err != nil {
				return uuid.Nil, err
			}
		}
		if _, err := queries.UpdateBlueprint(requestContext, store.UpdateBlueprintParams{
			CampaignID:        viewer.campaignID,
			ID:                blueprintID,
			Name:              cleaned.name,
			Description:       cleaned.description,
			DmNotes:           cleaned.dmNotes,
			CompletionEffects: cleaned.completionEffects,
			SortOrder:         cleaned.sortOrder,
		}); err != nil {
			return uuid.Nil, err
		}
		if err := queries.DeleteBlueprintCosts(requestContext, store.DeleteBlueprintCostsParams{CampaignID: viewer.campaignID, BlueprintID: blueprintID}); err != nil {
			return uuid.Nil, err
		}
		if err := queries.DeleteBlueprintLabor(requestContext, store.DeleteBlueprintLaborParams{CampaignID: viewer.campaignID, BlueprintID: blueprintID}); err != nil {
			return uuid.Nil, err
		}
		if err := queries.DeleteBlueprintOutputs(requestContext, store.DeleteBlueprintOutputsParams{CampaignID: viewer.campaignID, BlueprintID: blueprintID}); err != nil {
			return uuid.Nil, err
		}
		return blueprintID, saveBlueprintLines(requestContext, queries, viewer.campaignID, blueprintID, cleaned)
	}, TopicTimeline, TopicColony, TopicResources)
}

// SetBlueprintRevealed is the DM's Visible to players switch. It reveals or hides the blueprint's
// buildings and their BUILDING_FINISHED entries with it.
func (service *Service) SetBlueprintRevealed(requestContext context.Context, viewer Viewer, blueprintID uuid.UUID, revealed bool) (BuildingBlueprint, error) {
	if err := requireDm(viewer); err != nil {
		return BuildingBlueprint{}, err
	}
	return service.changeBlueprint(requestContext, viewer, func(queries *store.Queries) (uuid.UUID, error) {
		_, err := queries.SetBlueprintRevealed(requestContext, store.SetBlueprintRevealedParams{CampaignID: viewer.campaignID, ID: blueprintID, Revealed: revealed})
		return blueprintID, blueprintGone(err)
	}, TopicTimeline)
}

// SetBlueprintArchived takes a blueprint out of players' Start list (or brings it back). Its
// buildings stand; blueprints are never deleted.
func (service *Service) SetBlueprintArchived(requestContext context.Context, viewer Viewer, blueprintID uuid.UUID, archived bool) (BuildingBlueprint, error) {
	if err := requireDm(viewer); err != nil {
		return BuildingBlueprint{}, err
	}
	return service.changeBlueprint(requestContext, viewer, func(queries *store.Queries) (uuid.UUID, error) {
		_, err := queries.SetBlueprintArchived(requestContext, store.SetBlueprintArchivedParams{CampaignID: viewer.campaignID, ID: blueprintID, Archived: archived})
		return blueprintID, blueprintGone(err)
	})
}

// changeBlueprint runs one blueprint change under the colony lock (one lock order, and two creates of
// one name can't race) and answers with the blueprint as the DM sees it.
func (service *Service) changeBlueprint(requestContext context.Context, viewer Viewer, change func(queries *store.Queries) (uuid.UUID, error), topics ...Topic) (BuildingBlueprint, error) {
	var blueprint BuildingBlueprint
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		blueprintID, err := change(queries)
		if err != nil {
			return err
		}
		row, err := queries.GetBlueprint(requestContext, store.GetBlueprintParams{CampaignID: viewer.campaignID, ID: blueprintID})
		if err != nil {
			return blueprintGone(err)
		}
		lines, err := readBlueprintLines(requestContext, queries, viewer.campaignID)
		if err != nil {
			return err
		}
		blueprint, err = toBlueprint(row, lines)
		return err
	})
	if err != nil {
		return BuildingBlueprint{}, err
	}
	service.broker.Publish(viewer.campaignID, append([]Topic{TopicConstruction}, topics...)...)
	return blueprint, nil
}

// cleanedBlueprint is a BlueprintInput checked against this campaign's resources and roles.
type cleanedBlueprint struct {
	name, description, dmNotes string
	cost                       map[string]int64
	labor                      []store.CreateBlueprintLaborParams
	outputs                    []store.CreateBlueprintOutputParams
	completionEffects          []byte
	sortOrder                  int32
}

// cleanBlueprintInput checks a blueprint body: every key is loaded with the viewer's campaign, so
// another campaign's resource or role is simply not found (Invariant 11).
func cleanBlueprintInput(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, blueprintID uuid.UUID, input BlueprintInput) (cleanedBlueprint, error) {
	var fieldErrors []FieldError
	collect := func(fieldError *FieldError) {
		if fieldError != nil {
			fieldErrors = append(fieldErrors, *fieldError)
		}
	}
	var cleaned cleanedBlueprint
	var fieldError *FieldError
	cleaned.name, fieldError = cleanText("name", input.Name, blueprintNameMaxLength)
	collect(fieldError)
	if fieldError == nil && cleaned.name == "" {
		collect(&FieldError{Field: "name", Message: "Give the blueprint a name."})
	}
	if fieldError == nil && cleaned.name != "" {
		taken, err := queries.BlueprintNameTaken(requestContext, store.BlueprintNameTakenParams{CampaignID: campaignID, Name: cleaned.name, ExceptID: blueprintID})
		if err != nil {
			return cleanedBlueprint{}, err
		}
		if taken {
			collect(&FieldError{Field: "name", Message: "There is already a blueprint called " + cleaned.name + "."})
		}
	}
	cleaned.description, fieldError = cleanMultilineText("description", input.Description, blueprintProseMaxLength)
	collect(fieldError)
	cleaned.dmNotes, fieldError = cleanMultilineText("dmNotes", input.DmNotes, blueprintProseMaxLength)
	collect(fieldError)
	collect(checkSortOrder(input.SortOrder))

	resources, err := campaignResources(requestContext, queries, campaignID)
	if err != nil {
		return cleanedBlueprint{}, err
	}
	cleaned.cost = map[string]int64{}
	for _, key := range slices.Sorted(maps.Keys(input.Cost)) {
		amount := input.Cost[key]
		resource, found := resources[key]
		switch {
		case !found:
			collect(&FieldError{Field: "cost", Message: `There is no resource "` + key + `".`})
		case resource.Archived:
			collect(&FieldError{Field: "cost", Message: resource.Label + " is archived."})
		case amount < 1 || amount > game.MaxStored:
			collect(&FieldError{Field: "cost", Message: "A cost of " + resource.Label + " is 1 or more, up to " + formatNumber(game.MaxStored) + "."})
		default:
			cleaned.cost[key] = amount
		}
	}
	completion, completionErrors := cleanEffects("completionEffects", input.CompletionEffects, resources)
	fieldErrors = append(fieldErrors, completionErrors...)
	for _, key := range slices.Sorted(maps.Keys(input.Output)) {
		output := input.Output[key]
		resource, found := resources[key]
		switch {
		case !found:
			collect(&FieldError{Field: "output", Message: `There is no resource "` + key + `".`})
		case resource.Archived:
			collect(&FieldError{Field: "output", Message: resource.Label + " is archived."})
		case output.DailyDelta < game.MinStored || output.DailyDelta > game.MaxStored:
			collect(&FieldError{Field: "output", Message: "That daily change of " + resource.Label + " can't be stored."})
		case output.StorageBonus < 0 || output.StorageBonus > game.MaxStored:
			collect(&FieldError{Field: "output", Message: "A storage bonus for " + resource.Label + " is 0 or more, up to " + formatNumber(game.MaxStored) + "."})
		case output.DailyDelta == 0 && output.StorageBonus == 0:
			collect(&FieldError{Field: "output", Message: "Give " + resource.Label + " a daily change or a storage bonus, or remove it."})
		default:
			cleaned.outputs = append(cleaned.outputs, store.CreateBlueprintOutputParams{
				CampaignID: campaignID, ResourceKey: key, DailyDelta: int32(output.DailyDelta), StorageBonus: int32(output.StorageBonus),
			})
		}
	}

	roles, err := queries.ListRoles(requestContext, campaignID)
	if err != nil {
		return cleanedBlueprint{}, err
	}
	switch {
	case len(input.Labor) == 0:
		collect(&FieldError{Field: "labor", Message: "Give the blueprint at least one role's work. A building that needs none is a stock adjustment."})
	case len(input.Labor) > blueprintLaborMaxLines:
		collect(&FieldError{Field: "labor", Message: fmt.Sprintf("Use at most %d roles.", blueprintLaborMaxLines)})
	}
	for _, key := range slices.Sorted(maps.Keys(input.Labor)) {
		line := input.Labor[key]
		index := slices.IndexFunc(roles, func(row store.ListRolesRow) bool { return row.Role.Key == key })
		switch {
		case index < 0:
			collect(&FieldError{Field: "labor", Message: `There is no role "` + key + `".`})
		case roles[index].Role.Archived:
			collect(&FieldError{Field: "labor", Message: roles[index].Role.Label + " is archived."})
		case line.Effort < 1 || line.Effort > game.MaxStored:
			collect(&FieldError{Field: "labor", Message: roles[index].Role.Label + " work is 1 or more role-days, up to " + formatNumber(game.MaxStored) + "."})
		case line.MaxCrew != nil && (*line.MaxCrew < 1 || *line.MaxCrew > game.MaxStored):
			collect(&FieldError{Field: "labor", Message: "A crew cap for " + roles[index].Role.Label + " work is 1 or more; leave it blank for none."})
		default:
			cleaned.labor = append(cleaned.labor, store.CreateBlueprintLaborParams{
				CampaignID: campaignID, RoleKey: key, Effort: int32(line.Effort), MaxCrew: int32PointerOf(line.MaxCrew),
			})
		}
	}
	if len(fieldErrors) > 0 {
		return cleanedBlueprint{}, Invalid("Check the blueprint.", fieldErrors...)
	}
	cleaned.completionEffects, err = encodeEffects(completion)
	if err != nil {
		return cleanedBlueprint{}, err
	}
	cleaned.sortOrder = int32(input.SortOrder)
	return cleaned, nil
}

func saveBlueprintLines(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, blueprintID uuid.UUID, cleaned cleanedBlueprint) error {
	for _, key := range slices.Sorted(maps.Keys(cleaned.cost)) {
		if err := queries.CreateBlueprintCost(requestContext, store.CreateBlueprintCostParams{
			CampaignID: campaignID, BlueprintID: blueprintID, ResourceKey: key, Amount: int32(cleaned.cost[key]),
		}); err != nil {
			return err
		}
	}
	for _, line := range cleaned.labor {
		line.BlueprintID = blueprintID
		if err := queries.CreateBlueprintLabor(requestContext, line); err != nil {
			return err
		}
	}
	for _, output := range cleaned.outputs {
		output.BlueprintID = blueprintID
		if err := queries.CreateBlueprintOutput(requestContext, output); err != nil {
			return err
		}
	}
	return nil
}

// StartBuilding starts a project: any active member, of a blueprint they can see. It snapshots the
// blueprint, takes its cost STRICT and sets the first crews (docs/design/construction.md §1, §2).
func (service *Service) StartBuilding(requestContext context.Context, viewer Viewer, input StartBuildingInput) (ColonyChange, error) {
	return service.runColonyMutation(requestContext, viewer, requireActive, &input.ExpectedVersion, func(colony colonySnapshot, queries *store.Queries) (colonyOutcome, error) {
		blueprint, err := queries.GetBlueprint(requestContext, store.GetBlueprintParams{CampaignID: viewer.campaignID, ID: input.BlueprintID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !blueprint.Revealed && !viewer.SeesDmFields()) {
			return colonyOutcome{}, Invalid("That blueprint doesn't exist.", FieldError{Field: "blueprintId", Message: "That blueprint doesn't exist."})
		}
		if err != nil {
			return colonyOutcome{}, err
		}
		if blueprint.Archived {
			return colonyOutcome{}, Conflict("The " + blueprint.Name + " blueprint is archived.")
		}
		if !blueprint.UpdatedAt.Equal(input.BlueprintUpdatedAt) {
			return colonyOutcome{}, Conflict("The blueprint changed. Review it and start again.")
		}
		unfinished, err := queries.CountUnfinishedBuildings(requestContext, viewer.campaignID)
		if err != nil {
			return colonyOutcome{}, err
		}
		if unfinished >= UnfinishedBuildingLimit {
			return colonyOutcome{}, errBuildingLimit
		}
		lines, err := readBlueprintLines(requestContext, queries, viewer.campaignID)
		if err != nil {
			return colonyOutcome{}, err
		}
		completion, err := decodeEffects(blueprint.CompletionEffects)
		if err != nil {
			return colonyOutcome{}, err
		}
		cost := Effects{Resources: map[string]int64{}}
		for _, line := range lines.costs[blueprint.ID] {
			cost.Resources[line.ResourceKey] = -int64(line.Amount)
		}
		buildingID := uuid.New()
		project := game.Project{BuildingID: buildingID, Lines: []game.LaborLine{}, CompletionEffects: completion, Output: lines.outputs[blueprint.ID]}
		for _, line := range lines.labor[blueprint.ID] {
			project.Lines = append(project.Lines, game.LaborLine{RoleKey: line.RoleKey, Effort: int64(line.Effort), MaxCrew: int64Pointer(line.MaxCrew)})
		}
		result, err := game.StartProject(colony.state, project, cost, input.Crews)
		var shortfallError *game.ShortfallError
		if errors.As(err, &shortfallError) {
			return colonyOutcome{}, Conflict(shortfallError.Error() + ".")
		}
		if err != nil {
			return colonyOutcome{}, gameError(err, map[string]string{"resources": "blueprintId"})
		}
		encodedCost, err := encodeEffects(cost)
		if err != nil {
			return colonyOutcome{}, err
		}
		if _, err := queries.CreateBuilding(requestContext, store.CreateBuildingParams{
			CampaignID:        viewer.campaignID,
			ID:                buildingID,
			BlueprintID:       blueprint.ID,
			Cost:              encodedCost,
			CompletionEffects: blueprint.CompletionEffects,
			StartedDay:        int32(colony.state.Day),
			StartedBy:         memberReference(viewer),
		}); err != nil {
			return colonyOutcome{}, err
		}
		started := result.After.Projects[slices.IndexFunc(result.After.Projects, func(project game.Project) bool { return project.BuildingID == buildingID })]
		for _, line := range started.Lines {
			if err := queries.CreateBuildingLabor(requestContext, store.CreateBuildingLaborParams{
				CampaignID: viewer.campaignID, BuildingID: buildingID, RoleKey: line.RoleKey, Effort: int32(line.Effort),
				MaxCrew: int32PointerOf(line.MaxCrew), Crew: int32(line.Crew),
			}); err != nil {
				return colonyOutcome{}, err
			}
		}
		title := "Construction started: " + blueprint.Name
		return colonyOutcome{
			after: result.After,
			label: title,
			entries: []plannedEntry{{
				day:        colony.state.Day,
				kind:       TimelineKindBuildingStarted,
				title:      title,
				body:       joinParts(appliedEffectsText(result.Applied, colony.order, colony.labels), crewsText(colony, started.Lines)),
				visibility: TimelineVisibilityDm,
				deltas:     result.Applied,
				buildingID: &buildingID,
			}},
			buildings: map[uuid.UUID]buildingBefore{buildingID: {Created: true}},
		}, nil
	})
}

// SetBuildingCrews replaces one project's crews: any active member, under the crew guards.
func (service *Service) SetBuildingCrews(requestContext context.Context, viewer Viewer, buildingID uuid.UUID, input BuildingCrewsInput) (ColonyChange, error) {
	return service.runColonyMutation(requestContext, viewer, requireActive, &input.ExpectedVersion, func(colony colonySnapshot, queries *store.Queries) (colonyOutcome, error) {
		building, err := visibleBuilding(requestContext, queries, viewer, buildingID)
		if err != nil {
			return colonyOutcome{}, err
		}
		if building.Building.Status != BuildingStatusConstructing {
			return colonyOutcome{}, Conflict("The " + building.BlueprintName + " isn't under construction, so it takes no crews.")
		}
		after, err := game.SetCrews(colony.state, buildingID, input.Crews)
		if err != nil {
			return colonyOutcome{}, gameError(err, nil)
		}
		before := projectOf(colony.state, buildingID)
		changed := projectOf(after, buildingID)
		parts := []string{}
		for index, line := range changed.Lines {
			previous := before.Lines[index]
			if line.Crew == previous.Crew {
				continue
			}
			if err := queries.SetBuildingLabor(requestContext, store.SetBuildingLaborParams{
				CampaignID: viewer.campaignID, BuildingID: buildingID, RoleKey: line.RoleKey, Progress: int32(line.Progress), Crew: int32(line.Crew),
			}); err != nil {
				return colonyOutcome{}, err
			}
			parts = append(parts, pluralRoleLabel(colony, line.RoleKey)+" "+formatNumber(previous.Crew)+" → "+formatNumber(line.Crew))
		}
		if len(parts) == 0 {
			return colonyOutcome{}, Invalid("Nothing would change: those are the crews it has.")
		}
		title := building.BlueprintName + " crews: " + strings.Join(parts, ", ")
		snapshot, err := snapshotBuilding(building.Building, before.Lines)
		if err != nil {
			return colonyOutcome{}, err
		}
		return colonyOutcome{
			after: after,
			label: title,
			entries: []plannedEntry{{
				day:        colony.state.Day,
				kind:       TimelineKindBuildingCrewsChanged,
				title:      title,
				visibility: TimelineVisibilityDm,
				buildingID: &buildingID,
			}},
			buildings: map[uuid.UUID]buildingBefore{buildingID: snapshot},
		}, nil
	})
}

// CancelBuilding stops a project (DM). The refund the DM typed applies capped; its crews go idle.
func (service *Service) CancelBuilding(requestContext context.Context, viewer Viewer, buildingID uuid.UUID, input BuildingRefundInput) (ColonyChange, error) {
	return service.endBuilding(requestContext, viewer, buildingID, input, BuildingStatusConstructing, BuildingStatusCancelled)
}

// DemolishBuilding takes a finished building down (DM). The refund the DM typed applies capped.
func (service *Service) DemolishBuilding(requestContext context.Context, viewer Viewer, buildingID uuid.UUID, input BuildingRefundInput) (ColonyChange, error) {
	return service.endBuilding(requestContext, viewer, buildingID, input, BuildingStatusFinished, BuildingStatusDemolished)
}

func (service *Service) endBuilding(requestContext context.Context, viewer Viewer, buildingID uuid.UUID, input BuildingRefundInput, from BuildingStatus, to BuildingStatus) (ColonyChange, error) {
	return service.mutateColony(requestContext, viewer, input.ExpectedVersion, func(colony colonySnapshot, queries *store.Queries) (colonyOutcome, error) {
		building, err := visibleBuilding(requestContext, queries, viewer, buildingID)
		if err != nil {
			return colonyOutcome{}, err
		}
		name := building.BlueprintName
		if building.Building.Status != from {
			if to == BuildingStatusCancelled {
				return colonyOutcome{}, Conflict("Only a building under construction can be cancelled.")
			}
			return colonyOutcome{}, Conflict("Only a finished building can be demolished.")
		}
		fieldErrors := adjustableKeys("refund", input.Refund, colony)
		for _, key := range slices.Sorted(maps.Keys(input.Refund)) {
			if amount := input.Refund[key]; amount < 0 || amount > game.MaxStored {
				fieldErrors = append(fieldErrors, FieldError{Field: "refund", Message: "A refund of " + colony.labels.of(key) + " is 0 or more."})
			}
		}
		if len(fieldErrors) > 0 {
			return colonyOutcome{}, Invalid("Check the refund.", fieldErrors...)
		}
		result, err := game.ApplyEffects(game.WithoutBuilding(colony.state, buildingID), Effects{Resources: input.Refund}, game.Clamp)
		if err != nil {
			return colonyOutcome{}, gameError(err, map[string]string{"resources": "refund"})
		}
		refund, err := encodeEffects(result.Applied)
		if err != nil {
			return colonyOutcome{}, err
		}
		labor, err := queries.ListLaborOfBuilding(requestContext, store.ListLaborOfBuildingParams{CampaignID: viewer.campaignID, BuildingID: buildingID})
		if err != nil {
			return colonyOutcome{}, err
		}
		before, err := snapshotBuilding(building.Building, toProject(buildingID, labor, Effects{}, nil).Lines)
		if err != nil {
			return colonyOutcome{}, err
		}
		endedDay := int32(colony.state.Day)
		if err := queries.SetBuildingStatus(requestContext, store.SetBuildingStatusParams{
			CampaignID: viewer.campaignID, ID: buildingID, Status: to, FinishedDay: building.Building.FinishedDay, EndedDay: &endedDay, Refund: refund,
		}); err != nil {
			return colonyOutcome{}, err
		}
		for _, line := range labor {
			if line.Crew == 0 {
				continue
			}
			if err := queries.SetBuildingLabor(requestContext, store.SetBuildingLaborParams{
				CampaignID: viewer.campaignID, BuildingID: buildingID, RoleKey: line.RoleKey, Progress: line.Progress, Crew: 0,
			}); err != nil {
				return colonyOutcome{}, err
			}
		}
		kind, title := TimelineKindBuildingCancelled, "Construction cancelled: "+name
		if to == BuildingStatusDemolished {
			kind, title = TimelineKindBuildingDemolished, name+" demolished"
		}
		body := appliedEffectsText(result.Applied, colony.order, colony.labels)
		if body == "" {
			body = "Nothing given back"
		}
		return colonyOutcome{
			after: result.After,
			label: title,
			entries: []plannedEntry{{
				day:        colony.state.Day,
				kind:       kind,
				title:      title,
				body:       body,
				visibility: TimelineVisibilityDm,
				deltas:     result.Applied,
				buildingID: &buildingID,
			}},
			buildings: map[uuid.UUID]buildingBefore{buildingID: before},
		}, nil
	})
}

// AddBuilding places a finished, Active building of a blueprint on today's day (DM, K19): no cost, no
// labor and no completion Effects. Its storage bonus counts at once and its output runs from the next
// day. A hidden blueprint's building is a hidden row like any other of it, and its BUILDING_ADDED entry
// is PUBLIC the way BUILDING_FINISHED is: players see it only while the blueprint is revealed.
func (service *Service) AddBuilding(requestContext context.Context, viewer Viewer, blueprintID uuid.UUID, input AddBuildingInput) (ColonyChange, error) {
	return service.mutateColony(requestContext, viewer, input.ExpectedVersion, func(colony colonySnapshot, queries *store.Queries) (colonyOutcome, error) {
		blueprint, err := queries.GetBlueprint(requestContext, store.GetBlueprintParams{CampaignID: viewer.campaignID, ID: blueprintID})
		if err != nil {
			return colonyOutcome{}, blueprintGone(err)
		}
		if blueprint.Archived {
			return colonyOutcome{}, Conflict("The " + blueprint.Name + " blueprint is archived.")
		}
		lines, err := readBlueprintLines(requestContext, queries, viewer.campaignID)
		if err != nil {
			return colonyOutcome{}, err
		}
		buildingID := uuid.New()
		if _, err := queries.CreateAddedBuilding(requestContext, store.CreateAddedBuildingParams{
			CampaignID: viewer.campaignID, ID: buildingID, BlueprintID: blueprint.ID, Day: int32(colony.state.Day), StartedBy: memberReference(viewer),
		}); err != nil {
			return colonyOutcome{}, err
		}
		for _, line := range lines.labor[blueprint.ID] {
			if err := queries.CreateBuildingLabor(requestContext, store.CreateBuildingLaborParams{
				CampaignID: viewer.campaignID, BuildingID: buildingID, RoleKey: line.RoleKey, Effort: line.Effort, MaxCrew: line.MaxCrew, Progress: line.Effort,
			}); err != nil {
				return colonyOutcome{}, err
			}
		}
		title := blueprint.Name + " added by the DM"
		return colonyOutcome{
			after: game.PlaceBuilding(colony.state, buildingID, lines.outputs[blueprint.ID]),
			label: title,
			entries: []plannedEntry{{
				day:        colony.state.Day,
				kind:       TimelineKindBuildingAdded,
				title:      title,
				visibility: TimelineVisibilityPublic,
				buildingID: &buildingID,
			}},
			buildings: map[uuid.UUID]buildingBefore{buildingID: {Created: true}},
		}, nil
	})
}

// IdleBuilding switches a finished building Idle (DM): its daily output and storage bonus stop at once,
// and stock above a cap that falls stays (K12, K13).
func (service *Service) IdleBuilding(requestContext context.Context, viewer Viewer, buildingID uuid.UUID, input BuildingVersionInput) (ColonyChange, error) {
	return service.switchBuilding(requestContext, viewer, buildingID, input, false)
}

// ActivateBuilding switches an Idle building Active again (DM): its output runs from the next day and
// its storage bonus counts at once.
func (service *Service) ActivateBuilding(requestContext context.Context, viewer Viewer, buildingID uuid.UUID, input BuildingVersionInput) (ColonyChange, error) {
	return service.switchBuilding(requestContext, viewer, buildingID, input, true)
}

func (service *Service) switchBuilding(requestContext context.Context, viewer Viewer, buildingID uuid.UUID, input BuildingVersionInput, active bool) (ColonyChange, error) {
	return service.mutateColony(requestContext, viewer, input.ExpectedVersion, func(colony colonySnapshot, queries *store.Queries) (colonyOutcome, error) {
		building, err := visibleBuilding(requestContext, queries, viewer, buildingID)
		if err != nil {
			return colonyOutcome{}, err
		}
		name := building.BlueprintName
		kind, title := TimelineKindBuildingIdled, name+" set Idle"
		if active {
			kind, title = TimelineKindBuildingActivated, name+" set Active"
		}
		switch {
		case building.Building.Status != BuildingStatusFinished:
			return colonyOutcome{}, Conflict("Only a finished building can be set Idle or Active.")
		case building.Building.Active == active && active:
			return colonyOutcome{}, Conflict("The " + name + " is already Active.")
		case building.Building.Active == active:
			return colonyOutcome{}, Conflict("The " + name + " is already Idle.")
		}
		after, err := game.SetBuildingActive(colony.state, buildingID, active)
		if err != nil {
			return colonyOutcome{}, gameError(err, nil)
		}
		before, err := snapshotBuilding(building.Building, nil)
		if err != nil {
			return colonyOutcome{}, err
		}
		if err := queries.SetBuildingActive(requestContext, store.SetBuildingActiveParams{CampaignID: viewer.campaignID, ID: buildingID, Active: active}); err != nil {
			return colonyOutcome{}, err
		}
		return colonyOutcome{
			after: after,
			label: title,
			entries: []plannedEntry{{
				day:        colony.state.Day,
				kind:       kind,
				title:      title,
				visibility: TimelineVisibilityDm,
				buildingID: &buildingID,
			}},
			buildings: map[uuid.UUID]buildingBefore{buildingID: before},
		}, nil
	})
}

// visibleBuilding loads a building from the path with the viewer's campaign; a building of a hidden
// blueprint is not found for players, exactly like a missing one.
func visibleBuilding(requestContext context.Context, queries *store.Queries, viewer Viewer, buildingID uuid.UUID) (store.GetBuildingRow, error) {
	building, err := queries.GetBuilding(requestContext, store.GetBuildingParams{CampaignID: viewer.campaignID, ID: buildingID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !building.BlueprintRevealed && !viewer.SeesDmFields()) {
		return store.GetBuildingRow{}, errBuildingNotFound
	}
	return building, err
}

func projectOf(state game.State, buildingID uuid.UUID) game.Project {
	return state.Projects[slices.IndexFunc(state.Projects, func(project game.Project) bool { return project.BuildingID == buildingID })]
}

func snapshotBuilding(row store.Building, lines []game.LaborLine) (buildingBefore, error) {
	active := row.Active
	before := buildingBefore{Status: row.Status, Active: &active, FinishedDay: row.FinishedDay, EndedDay: row.EndedDay, Lines: map[string]laborBefore{}}
	if row.Refund != nil {
		refund, err := decodeEffects(row.Refund)
		if err != nil {
			return buildingBefore{}, err
		}
		before.Refund = &refund
	}
	for _, line := range lines {
		before.Lines[line.RoleKey] = laborBefore{Progress: line.Progress, Crew: line.Crew}
	}
	return before, nil
}

// advanceBuildings saves what an advance did to the projects: every line's progress and crew, and each
// finish's status. It returns the undo snapshot of every building it changed and a BUILDING_FINISHED
// entry per finish, PUBLIC (players see it only while the blueprint is revealed), with the completion
// Effects that applied as its deltas and any skipped resource in its DM-only note.
func advanceBuildings(requestContext context.Context, queries *store.Queries, colony colonySnapshot, result game.AdvanceResult) (map[uuid.UUID]buildingBefore, []plannedEntry, error) {
	befores := map[uuid.UUID]buildingBefore{}
	entries := []plannedEntry{}
	finishes := map[uuid.UUID]game.BuildingFinish{}
	for _, finish := range result.Finishes {
		finishes[finish.BuildingID] = finish
	}
	for _, project := range result.Before.Projects {
		var lines []game.LaborLine
		finish, finished := finishes[project.BuildingID]
		if finished {
			lines = finish.Lines
		} else {
			lines = projectOf(result.After, project.BuildingID).Lines
		}
		if slices.EqualFunc(lines, project.Lines, func(left, right game.LaborLine) bool {
			return left.Progress == right.Progress && left.Crew == right.Crew
		}) {
			continue
		}
		building, err := queries.GetBuilding(requestContext, store.GetBuildingParams{CampaignID: colony.campaignID, ID: project.BuildingID})
		if err != nil {
			return nil, nil, err
		}
		befores[project.BuildingID], err = snapshotBuilding(building.Building, project.Lines)
		if err != nil {
			return nil, nil, err
		}
		for index, line := range lines {
			if line.Progress == project.Lines[index].Progress && line.Crew == project.Lines[index].Crew {
				continue
			}
			if err := queries.SetBuildingLabor(requestContext, store.SetBuildingLaborParams{
				CampaignID: colony.campaignID, BuildingID: project.BuildingID, RoleKey: line.RoleKey, Progress: int32(line.Progress), Crew: int32(line.Crew),
			}); err != nil {
				return nil, nil, err
			}
		}
		if !finished {
			continue
		}
		finishedDay := int32(finish.Day)
		if err := queries.SetBuildingStatus(requestContext, store.SetBuildingStatusParams{
			CampaignID: colony.campaignID, ID: project.BuildingID, Status: BuildingStatusFinished, FinishedDay: &finishedDay,
		}); err != nil {
			return nil, nil, err
		}
		buildingID := project.BuildingID
		entries = append(entries, plannedEntry{
			day:        finish.Day,
			kind:       TimelineKindBuildingFinished,
			title:      "The " + building.BlueprintName + " is finished",
			body:       appliedEffectsText(finish.Applied, colony.order, colony.labels),
			visibility: TimelineVisibilityPublic,
			deltas:     finish.Applied,
			buildingID: &buildingID,
			dmNote:     skippedCompletionNote(colony, project.CompletionEffects, finish.Skipped),
		})
	}
	slices.SortStableFunc(entries, func(left, right plannedEntry) int { return int(left.day - right.day) })
	return befores, entries, nil
}

// skippedCompletionNote names the completion Effects a finish couldn't apply: "Not applied, archived
// since the start: Timber +10".
func skippedCompletionNote(colony colonySnapshot, completion Effects, skipped []string) string {
	if len(skipped) == 0 {
		return ""
	}
	parts := make([]string, 0, len(skipped))
	for _, key := range skipped {
		parts = append(parts, colony.labels.of(key)+" "+formatDelta(completion.Resources[key]))
	}
	return "Not applied, archived since the start: " + strings.Join(parts, ", ")
}

// AdvanceFinish is one building an advance would finish, for its preview.
type AdvanceFinish struct {
	BuildingID uuid.UUID `json:"buildingId" format:"uuid"`
	Name       string    `json:"name"`
	Day        int64     `json:"day"`
}

// advanceFinishes names the preview's finishes. Only the DM previews an advance, so it names hidden
// blueprints too.
func advanceFinishes(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, result game.AdvanceResult) ([]AdvanceFinish, error) {
	finishes := make([]AdvanceFinish, 0, len(result.Finishes))
	for _, finish := range result.Finishes {
		building, err := queries.GetBuilding(requestContext, store.GetBuildingParams{CampaignID: campaignID, ID: finish.BuildingID})
		if err != nil {
			return nil, err
		}
		finishes = append(finishes, AdvanceFinish{BuildingID: finish.BuildingID, Name: building.BlueprintName, Day: finish.Day})
	}
	return finishes, nil
}

// restoreBuildings puts back every building a colony mutation changed (spec §5.1). Every ID comes from
// this campaign's own undo slot and is loaded with the campaign. A building that would be under
// construction again with a labor role archived since is a conflict the DM resolves by hand, as
// restoreSnapshot's headcounts are, so Advance never meets an archived role.
func restoreBuildings(requestContext context.Context, queries *store.Queries, colony colonySnapshot, befores map[uuid.UUID]buildingBefore) error {
	campaignID := colony.campaignID
	for _, buildingID := range slices.SortedFunc(maps.Keys(befores), compareIDs) {
		building, err := queries.GetBuilding(requestContext, store.GetBuildingParams{CampaignID: campaignID, ID: buildingID})
		if err != nil {
			return err
		}
		before := befores[buildingID]
		if before.Created {
			if err := queries.DeleteBuilding(requestContext, store.DeleteBuildingParams{CampaignID: campaignID, ID: buildingID}); err != nil {
				return err
			}
			continue
		}
		if before.Status == BuildingStatusConstructing {
			for _, roleKey := range slices.Sorted(maps.Keys(before.Lines)) {
				if role := colony.roles[roleKey]; role.Archived {
					return Conflict("Undo can't put the " + building.BlueprintName + " back under construction: its " +
						role.Label + " role was archived since.")
				}
			}
		}
		var refund []byte
		if before.Refund != nil {
			refund, err = encodeEffects(*before.Refund)
			if err != nil {
				return err
			}
		}
		if err := queries.SetBuildingStatus(requestContext, store.SetBuildingStatusParams{
			CampaignID: campaignID, ID: buildingID, Status: before.Status, FinishedDay: before.FinishedDay, EndedDay: before.EndedDay, Refund: refund,
		}); err != nil {
			return err
		}
		if before.Active != nil && *before.Active != building.Building.Active {
			if err := queries.SetBuildingActive(requestContext, store.SetBuildingActiveParams{CampaignID: campaignID, ID: buildingID, Active: *before.Active}); err != nil {
				return err
			}
		}
		for _, roleKey := range slices.Sorted(maps.Keys(before.Lines)) {
			line := before.Lines[roleKey]
			if err := queries.SetBuildingLabor(requestContext, store.SetBuildingLaborParams{
				CampaignID: campaignID, BuildingID: buildingID, RoleKey: roleKey, Progress: int32(line.Progress), Crew: int32(line.Crew),
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// hiddenCreatedBuildings is the buildings an undo will delete (a Start's or an Add's) whose blueprint is
// hidden, read before the delete takes the link that says so.
func hiddenCreatedBuildings(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, befores map[uuid.UUID]buildingBefore) (map[uuid.UUID]bool, error) {
	hidden := map[uuid.UUID]bool{}
	for _, buildingID := range slices.SortedFunc(maps.Keys(befores), compareIDs) {
		if !befores[buildingID].Created {
			continue
		}
		building, err := queries.GetBuilding(requestContext, store.GetBuildingParams{CampaignID: campaignID, ID: buildingID})
		if err != nil {
			return nil, err
		}
		if !building.BlueprintRevealed {
			hidden[buildingID] = true
		}
	}
	return hidden, nil
}

// unlinkDeletedBuildings brings the undone rows in line with the delete, whose foreign key cleared
// their link to a created building. With the link gone nothing keeps a hidden blueprint's PUBLIC row
// (an Add's BUILDING_ADDED) from players, so it moves to DM, and the UNDO can't take it as the newest
// public entry.
func unlinkDeletedBuildings(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, undone []store.TimelineEvent,
	befores map[uuid.UUID]buildingBefore, hiddenCreated map[uuid.UUID]bool,
) error {
	for index := range undone {
		buildingID := undone[index].BuildingID
		if buildingID == nil || !befores[*buildingID].Created {
			continue
		}
		undone[index].BuildingID = nil
		if !hiddenCreated[*buildingID] || undone[index].Visibility != TimelineVisibilityPublic {
			continue
		}
		if err := queries.HideTimelineEvent(requestContext, store.HideTimelineEventParams{CampaignID: campaignID, ID: undone[index].ID}); err != nil {
			return err
		}
		undone[index].Visibility = TimelineVisibilityDm
	}
	return nil
}

// deletesCreatedBuilding reports an undo of a Start, which deletes the building: the delete clears every
// entry's building_id, so the UNDO could no longer be hidden with the blueprint and must not name it.
func deletesCreatedBuilding(before undoSnapshot) bool {
	for _, building := range before.Buildings {
		if building.Created {
			return true
		}
	}
	return false
}

// crewsText is a Start entry's crew line: "Crews: 2 Builders, 1 Mage", or "No crew yet".
func crewsText(colony colonySnapshot, lines []game.LaborLine) string {
	parts := []string{}
	for _, line := range lines {
		if line.Crew > 0 {
			parts = append(parts, roleColonistsText(colony, line.Crew, line.RoleKey))
		}
	}
	if len(parts) == 0 {
		return "No crew yet"
	}
	return "Crews: " + strings.Join(parts, ", ")
}

// pluralRoleLabel is "Builders", as the workforce text names a role's colonists.
func pluralRoleLabel(colony colonySnapshot, roleKey string) string {
	if role, found := colony.roles[roleKey]; found {
		return role.Label + "s"
	}
	return roleKey
}

func blueprintGone(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return errBlueprintNotFound
	}
	return err
}
