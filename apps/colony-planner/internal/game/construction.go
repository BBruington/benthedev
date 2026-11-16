package game

import (
	"maps"
	"slices"
	"strconv"

	"github.com/google/uuid"
)

// The construction rules (docs/design/construction.md §2 and §3): a project's labor lines each gain
// their crew in role-days every day, a line's crew goes back to idle on the day it finishes, and the
// project finishes on the day its last line does. Crews are counts: across every unfinished project a
// role's crews never exceed its headcount, and nothing shrinks a crew on its own.

// Project is one unfinished building as the rules see it.
type Project struct {
	BuildingID uuid.UUID
	// Lines are sorted by role key, one per role.
	Lines []LaborLine
	// CompletionEffects apply once, CLAMP, on the day the project finishes.
	CompletionEffects Effects
	// Output is its blueprint's live output, which it gives once it stands: its storage bonus from the
	// end of its finish day, its daily output from the next day.
	Output map[string]ResourceOutput
}

// LaborLine is one role's share of a project: Effort role-days, done at Progress, with Crew working.
type LaborLine struct {
	RoleKey string
	Effort  int64
	// MaxCrew is nil when the line's crew has no cap.
	MaxCrew  *int64
	Progress int64
	Crew     int64
}

func (line LaborLine) done() bool {
	return line.Progress >= line.Effort
}

// BuildingFinish is one project an advance finished.
type BuildingFinish struct {
	BuildingID uuid.UUID
	Day        int64
	// Applied is what the completion Effects added after CLAMP: the BUILDING_FINISHED deltas.
	Applied Effects
	// Skipped lists the completion Effects' resource keys archived since Start, which add nothing.
	Skipped []string
	// Lines are the project's labor lines as it finished: each at its effort, with no crew.
	Lines []LaborLine
}

// Construction is the daily step that builds: each unfinished line gains crew role-days, capped at
// its effort, and a project finishing today stands from now and applies its completion Effects after
// upkeep.
type Construction struct{}

func (construction Construction) Span(state *State, from int64, to int64, log *Log) {
	days := to - from
	if days <= 0 || len(state.Projects) == 0 {
		return
	}
	for index := range state.Projects {
		project := &state.Projects[index]
		finishDay := int64(0)
		for lineIndex := range project.Lines {
			line := &project.Lines[lineIndex]
			if line.Crew <= 0 || line.done() {
				continue
			}
			needed := daysToFinish(*line)
			if needed > days {
				line.Progress += line.Crew * days
				continue
			}
			line.Progress = line.Effort
			line.Crew = 0
			finishDay = max(finishDay, from+needed)
		}
		if finishDay == 0 || !allDone(project.Lines) {
			continue
		}
		// NextBoundary ends every span on a finish day, so finishDay is to and upkeep has already run. The
		// building stands first, so its completion Effects already fill its own storage.
		standUp(state, *project)
		applied, skipped := applyCompletion(state, project.CompletionEffects)
		for key, delta := range project.CompletionEffects.Resources {
			if !slices.Contains(skipped, key) {
				log.RecordRequested(key, delta)
			}
		}
		log.finishes = append(log.finishes, BuildingFinish{
			BuildingID: project.BuildingID,
			Day:        finishDay,
			Applied:    applied,
			Skipped:    skipped,
			Lines:      slices.Clone(project.Lines),
		})
	}
	state.Projects = slices.DeleteFunc(state.Projects, func(project Project) bool { return allDone(project.Lines) })
}

// A finish changes crews, which the next span's progress reads, the stock and headcounts upkeep reads
// and the standing buildings and caps output reads, so the next day any line finishes is a boundary.
// A line with no crew never finishes.
func (construction Construction) NextBoundary(state State, after int64) (int64, bool) {
	next, found := int64(0), false
	for _, project := range state.Projects {
		for _, line := range project.Lines {
			if line.Crew <= 0 || line.done() {
				continue
			}
			if day := after + daysToFinish(line); !found || day < next {
				next, found = day, true
			}
		}
	}
	return next, found
}

// daysToFinish is ceil(remaining / crew) for a line with a crew and work left.
func daysToFinish(line LaborLine) int64 {
	remaining := line.Effort - line.Progress
	return (remaining + line.Crew - 1) / line.Crew
}

func allDone(lines []LaborLine) bool {
	return !slices.ContainsFunc(lines, func(line LaborLine) bool { return !line.done() })
}

// applyCompletion adds a finished project's completion Effects, each resource clamped to its minimum
// and its effective max (never lowering, K13) or what the column can store, and colonists into
// UNASSIGNED. A finish happens inside an advance and can't be refused, so a resource archived since
// Start is skipped and named instead.
func applyCompletion(state *State, effects Effects) (Effects, []string) {
	applied := Effects{Resources: map[string]int64{}}
	skipped := []string{}
	for _, key := range slices.Sorted(maps.Keys(effects.Resources)) {
		resource, found := state.Resources[key]
		if !found || resource.Archived {
			skipped = append(skipped, key)
			continue
		}
		newAmount := clampGain(resource.Amount, effects.Resources[key], resource.MinAmount, ceiling(*state, key))
		if newAmount != resource.Amount {
			applied.Resources[key] = newAmount - resource.Amount
		}
		resource.Amount = newAmount
		state.Resources[key] = resource
	}
	if unassigned, found := state.Roles[UnassignedRoleKey]; found && effects.Colonists > 0 {
		added := min(effects.Colonists, MaxStored-unassigned.UnnamedCount)
		unassigned.UnnamedCount += added
		state.Roles[UnassignedRoleKey] = unassigned
		applied.Colonists = added
	}
	return applied, skipped
}

// ProjectedFinishDay is the day a project finishes with its current crews, counted from day; nil when
// a line with work left has no crew.
func ProjectedFinishDay(day int64, project Project) *int64 {
	finish := day
	for _, line := range project.Lines {
		if line.done() {
			continue
		}
		if line.Crew <= 0 {
			return nil
		}
		finish = max(finish, day+daysToFinish(line))
	}
	return &finish
}

// Crews sums each role's crews over every unfinished project.
func Crews(projects []Project) map[string]int64 {
	crews := map[string]int64{}
	for _, project := range projects {
		for _, line := range project.Lines {
			if line.Crew > 0 {
				crews[line.RoleKey] += line.Crew
			}
		}
	}
	return crews
}

// checkCrewsFit refuses a workforce change that leaves a role with fewer colonists than its crews
// (K8): "5 Builders are on projects; take 2 off a project first".
func checkCrewsFit(state State, field string) error {
	crews := Crews(state.Projects)
	for _, roleKey := range slices.Sorted(maps.Keys(crews)) {
		role := state.Roles[roleKey]
		headcount := role.UnnamedCount + role.NamedCount
		if crews[roleKey] <= headcount {
			continue
		}
		onProjects := " are on projects"
		if crews[roleKey] == 1 {
			onProjects = " is on a project"
		}
		return invalid(field, roleCountText(crews[roleKey], role, roleKey)+onProjects+"; take "+
			strconv.FormatInt(crews[roleKey]-headcount, 10)+" off a project first")
	}
	return nil
}

// StartProject takes a project's cost, STRICT, and puts it under construction with the starter's
// crews. It refuses a labor role that is archived and crews the colony can't spare.
func StartProject(state State, project Project, cost Effects, crews map[string]int64) (EffectsResult, error) {
	for _, line := range project.Lines {
		if _, err := activeRole(state, line.RoleKey, "blueprintId"); err != nil {
			return EffectsResult{}, err
		}
		if line.Effort < 1 || line.Effort > MaxStored || line.Progress != 0 || line.Crew != 0 {
			return EffectsResult{}, invalid("blueprintId", "That labor can't be stored")
		}
	}
	result, err := ApplyEffects(state, cost, Strict)
	if err != nil {
		return EffectsResult{}, err
	}
	withProject := result.After.Clone()
	withProject.Projects = append(withProject.Projects, project)
	slices.SortFunc(withProject.Projects, func(left, right Project) int {
		return compareUUIDs(left.BuildingID, right.BuildingID)
	})
	crewed, err := SetCrews(withProject, project.BuildingID, crews)
	if err != nil {
		return EffectsResult{}, err
	}
	result.After = crewed
	return result, nil
}

// SetCrews replaces one project's crews: every line the map leaves out gets none. Each crew stays
// within its line's max crew, a finished line takes none, and a role's crews across every project stay
// within its headcount ("Only 2 Mages are free").
func SetCrews(state State, buildingID uuid.UUID, crews map[string]int64) (State, error) {
	index := slices.IndexFunc(state.Projects, func(project Project) bool { return project.BuildingID == buildingID })
	if index < 0 {
		return State{}, invalid("crews", "That project isn't under construction")
	}
	project := state.Projects[index]
	for _, roleKey := range slices.Sorted(maps.Keys(crews)) {
		if !slices.ContainsFunc(project.Lines, func(line LaborLine) bool { return line.RoleKey == roleKey }) {
			return State{}, invalid("crews", "This project needs no "+roleLabel(state, roleKey)+" work")
		}
	}
	after := state.Clone()
	lines := after.Projects[index].Lines
	others := Crews(slices.Delete(slices.Clone(after.Projects), index, index+1))
	for lineIndex, line := range lines {
		crew := crews[line.RoleKey]
		role := state.Roles[line.RoleKey]
		switch {
		case crew < 0:
			return State{}, invalid("crews", "A crew can't be negative")
		case crew > MaxStored:
			return State{}, invalid("crews", "That crew can't be stored")
		case crew > 0 && line.done():
			return State{}, invalid("crews", roleLabel(state, line.RoleKey)+" work on this project is done")
		case line.MaxCrew != nil && crew > *line.MaxCrew:
			return State{}, invalid("crews", "At most "+roleCountText(*line.MaxCrew, role, line.RoleKey)+" can work on this")
		case crew > line.Crew && crew > role.UnnamedCount+role.NamedCount-others[line.RoleKey]:
			return State{}, invalid("crews", freeText(role.UnnamedCount+role.NamedCount-others[line.RoleKey], role, line.RoleKey))
		}
		lines[lineIndex].Crew = crew
	}
	return after, nil
}

// freeText is "Only 2 Mages are free", "Only 1 Mage is free" or "No Mages are free".
func freeText(free int64, role Role, roleKey string) string {
	switch {
	case free <= 0:
		return "No " + pluralRole(role, roleKey) + " are free"
	case free == 1:
		return "Only 1 " + roleLabelOf(role, roleKey) + " is free"
	default:
		return "Only " + roleCountText(free, role, roleKey) + " are free"
	}
}

// roleCountText is "1 Builder" or "5 Builders".
func roleCountText(count int64, role Role, roleKey string) string {
	if count == 1 {
		return "1 " + roleLabelOf(role, roleKey)
	}
	return strconv.FormatInt(count, 10) + " " + pluralRole(role, roleKey)
}

func pluralRole(role Role, roleKey string) string {
	return roleLabelOf(role, roleKey) + "s"
}

func roleLabel(state State, roleKey string) string {
	return roleLabelOf(state.Roles[roleKey], roleKey)
}

// roleLabelOf is the role's label, or its key when it has none.
func roleLabelOf(role Role, roleKey string) string {
	if role.Label == "" {
		return roleKey
	}
	return role.Label
}

func compareUUIDs(left uuid.UUID, right uuid.UUID) int {
	return slices.Compare(left[:], right[:])
}

// cloneProjects deep-copies projects, keeping nil as nil so a clone compares equal to its source.
func cloneProjects(projects []Project) []Project {
	if projects == nil {
		return nil
	}
	clones := make([]Project, len(projects))
	for index, project := range projects {
		clones[index] = Project{
			BuildingID:        project.BuildingID,
			Lines:             slices.Clone(project.Lines),
			CompletionEffects: Effects{Resources: maps.Clone(project.CompletionEffects.Resources), Colonists: project.CompletionEffects.Colonists},
			Output:            maps.Clone(project.Output),
		}
		for lineIndex, line := range clones[index].Lines {
			if line.MaxCrew != nil {
				maxCrew := *line.MaxCrew
				clones[index].Lines[lineIndex].MaxCrew = &maxCrew
			}
		}
	}
	return clones
}

// checkStoredProjects reports labor lines and output the database could never hold.
func checkStoredProjects(projects []Project) bool {
	for _, project := range projects {
		if !checkStoredOutputs(project.Output) {
			return false
		}
		for _, line := range project.Lines {
			if line.Effort < 1 || line.Effort > MaxStored || line.Progress < 0 || line.Progress > line.Effort ||
				line.Crew < 0 || line.Crew > MaxStored || (line.MaxCrew != nil && line.Crew > *line.MaxCrew) {
				return false
			}
		}
	}
	return true
}
