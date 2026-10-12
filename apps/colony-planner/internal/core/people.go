package core

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/game"
	"github.com/BBruington/colony-planner/internal/store"
)

// Workforce is /people (spec §5.5, §6.2): every unarchived role with its headcounts and named
// colonists. Players see every role's total; named colonists are listed only when revealed, and a
// hidden one is counted among the unnamed (roleHeadcount), so the numbers never hint at them.
type Workforce struct {
	Version    int64           `json:"version" doc:"The colony version this read saw; workforce forms send it as expectedVersion"`
	Colonists  int64           `json:"colonists"`
	Unassigned int64           `json:"unassigned" doc:"The Unassigned role's total"`
	Roles      []WorkforceRole `json:"roles" doc:"Unarchived roles in display order, Unassigned included"`
}

type WorkforceRole struct {
	Key            string         `json:"key"`
	Label          string         `json:"label"`
	Icon           ColonyIcon     `json:"icon"`
	UnnamedCount   int64          `json:"unnamedCount" doc:"For players, hidden named colonists are counted here"`
	NamedCount     int64          `json:"namedCount"`
	Total          int64          `json:"total"`
	IsSystem       bool           `json:"isSystem" doc:"Marks UNASSIGNED, where new colonists land"`
	NamedColonists []NpcReference `json:"namedColonists"`
}

// TransferInput moves unnamed colonists between roles; WORKFORCE_CHANGED is DM-only unless Public.
type TransferInput struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	FromRoleKey     string `json:"fromRoleKey"`
	ToRoleKey       string `json:"toRoleKey"`
	Count           int64  `json:"count" doc:"1 or more, and no more than the source role's unnamed colonists"`
	Public          bool   `json:"public"`
}

// SetRoleCountInput sets a role's unnamed headcount: a correction, and how deaths and departures are
// recorded. WORKFORCE_CHANGED is DM-only unless Public.
type SetRoleCountInput struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	RoleKey         string `json:"roleKey"`
	Count           int64  `json:"count" doc:"0 or more"`
	Public          bool   `json:"public"`
}

// JoinColonyInput makes an NPC a named colonist in a role.
type JoinColonyInput struct {
	ExpectedVersion int64    `json:"expectedVersion"`
	RoleKey         string   `json:"roleKey"`
	Mode            JoinMode `json:"mode" doc:"NEW_COLONIST (the default: Colonists +1) or PROMOTE_UNNAMED (names one of the role's unnamed colonists)"`
}

// ChangeColonyRoleInput moves a named colonist to another role. WORKFORCE_CHANGED is DM-only unless
// Public, and stays DM-only while the NPC is hidden.
type ChangeColonyRoleInput struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	RoleKey         string `json:"roleKey"`
	Public          bool   `json:"public"`
}

// LeaveColonyInput takes a named colonist out of the colony.
type LeaveColonyInput struct {
	ExpectedVersion int64     `json:"expectedVersion"`
	Mode            LeaveMode `json:"mode" doc:"REMOVE (the default: Colonists -1) or RETURN_TO_POOL (they stay as one of the role's unnamed colonists)"`
}

// GetWorkforce is /people for every active member.
func (service *Service) GetWorkforce(requestContext context.Context, viewer Viewer) (Workforce, error) {
	if err := requireActive(viewer); err != nil {
		return Workforce{}, err
	}
	colony, err := service.queries.GetColony(requestContext, viewer.campaignID)
	if err != nil {
		return Workforce{}, campaignGone(err)
	}
	roles, err := service.queries.ListRoles(requestContext, viewer.campaignID)
	if err != nil {
		return Workforce{}, err
	}
	named, err := service.queries.ListNamedColonists(requestContext, store.ListNamedColonistsParams{
		CampaignID:   viewer.campaignID,
		DmProjection: viewer.SeesDmFields(),
	})
	if err != nil {
		return Workforce{}, err
	}
	workforce := Workforce{Version: colony.Version, Roles: make([]WorkforceRole, 0, len(roles))}
	headcounts := make(map[string]game.Role, len(roles))
	for _, row := range roles {
		headcounts[row.Role.Key] = game.Role{UnnamedCount: int64(row.Role.UnnamedCount), NamedCount: row.NamedCount}
		headcount := roleHeadcount(row, viewer.SeesDmFields())
		if row.Role.Key == game.UnassignedRoleKey {
			workforce.Unassigned = headcount.Total
		}
		if row.Role.Archived {
			continue
		}
		role := WorkforceRole{
			Key:            headcount.Key,
			Label:          headcount.Label,
			Icon:           headcount.Icon,
			UnnamedCount:   headcount.UnnamedCount,
			NamedCount:     headcount.NamedCount,
			Total:          headcount.Total,
			IsSystem:       headcount.IsSystem,
			NamedColonists: []NpcReference{},
		}
		for _, npc := range named {
			if npc.ColonyRoleKey != nil && *npc.ColonyRoleKey == role.Key {
				role.NamedColonists = append(role.NamedColonists, toNpcReference(npc))
			}
		}
		workforce.Roles = append(workforce.Roles, role)
	}
	workforce.Colonists = game.Colonists(headcounts)
	return workforce, nil
}

// TransferColonists moves n unnamed colonists from one role to another (spec §5.5). It refuses, with
// the numbers, when the source has fewer than n.
func (service *Service) TransferColonists(requestContext context.Context, viewer Viewer, input TransferInput) (ColonyChange, error) {
	return service.mutateColony(requestContext, viewer, input.ExpectedVersion, func(colony colonySnapshot, _ *store.Queries) (colonyOutcome, error) {
		after, err := game.Transfer(colony.state, input.FromRoleKey, input.ToRoleKey, input.Count)
		if err != nil {
			return colonyOutcome{}, gameError(err, nil)
		}
		title := colonistCount(input.Count) + ": " + colony.state.Roles[input.FromRoleKey].Label + " → " + colony.state.Roles[input.ToRoleKey].Label
		return colonyOutcome{after: after, label: title, entries: []plannedEntry{{
			day:        colony.state.Day,
			kind:       TimelineKindWorkforceChanged,
			title:      title,
			visibility: optInPublic(input.Public),
		}}}, nil
	})
}

// SetRoleCount sets a role's unnamed headcount directly (spec §5.5).
func (service *Service) SetRoleCount(requestContext context.Context, viewer Viewer, input SetRoleCountInput) (ColonyChange, error) {
	return service.mutateColony(requestContext, viewer, input.ExpectedVersion, func(colony colonySnapshot, _ *store.Queries) (colonyOutcome, error) {
		after, err := game.SetUnnamedCount(colony.state, input.RoleKey, input.Count)
		if err != nil {
			return colonyOutcome{}, gameError(err, nil)
		}
		// Role totals, not unnamed counts: players count a hidden named colonist as unnamed, so only
		// the totals read the same in both projections and can't hint that one exists (spec §5.5).
		role := colony.state.Roles[input.RoleKey]
		title := role.Label + ": " + formatNumber(role.UnnamedCount+role.NamedCount) + " → " + colonistCount(input.Count+role.NamedCount)
		return colonyOutcome{after: after, label: title, entries: []plannedEntry{{
			day:        colony.state.Day,
			kind:       TimelineKindWorkforceChanged,
			title:      title,
			visibility: optInPublic(input.Public),
		}}}, nil
	})
}

// JoinColony makes an NPC a named colonist in a role, as a new colonist (Colonists +1) or by naming
// one of the role's unnamed colonists (unchanged). COLONIST_JOINED is PUBLIC only when the NPC is
// revealed, and names them by display name.
func (service *Service) JoinColony(requestContext context.Context, viewer Viewer, npcID uuid.UUID, input JoinColonyInput) (ColonyChange, error) {
	return service.mutateColony(requestContext, viewer, input.ExpectedVersion, func(colony colonySnapshot, queries *store.Queries) (colonyOutcome, error) {
		npc, err := colonyNpc(requestContext, queries, colony.campaignID, npcID)
		if err != nil {
			return colonyOutcome{}, err
		}
		if npc.ColonyRoleKey != nil {
			return colonyOutcome{}, Conflict(npc.DisplayName + " is already a colonist (" + colony.state.Roles[*npc.ColonyRoleKey].Label + "). Change their role instead.")
		}
		mode := cmp.Or(input.Mode, JoinModeNewColonist)
		if !slices.Contains(AllJoinModes(), mode) {
			return colonyOutcome{}, Invalid("Pick how they join.", FieldError{Field: "mode", Message: "Pick NEW_COLONIST or PROMOTE_UNNAMED."})
		}
		after, err := game.JoinColony(colony.state, input.RoleKey, mode == JoinModePromoteUnnamed)
		if err != nil {
			return colonyOutcome{}, gameError(err, nil)
		}
		roleLabel := colony.state.Roles[input.RoleKey].Label
		entry := plannedEntry{
			day:        colony.state.Day,
			kind:       TimelineKindColonistJoined,
			title:      npc.DisplayName + " joined the colony as " + roleLabel,
			visibility: revealedPublic(npc),
			npcID:      &npc.ID,
			npcName:    npc.DisplayName,
		}
		if mode == JoinModePromoteUnnamed {
			entry.body = "Named from the colony's " + roleLabel + " colonists; the count is unchanged."
		}
		roleKey := input.RoleKey
		return colonyOutcome{
			after:    after,
			label:    entry.title,
			entries:  []plannedEntry{entry},
			npcRoles: map[uuid.UUID]npcRoleChange{npc.ID: {before: nil, after: &roleKey}},
		}, nil
	})
}

// ChangeColonyRole moves a named colonist to another role; the total is unchanged.
func (service *Service) ChangeColonyRole(requestContext context.Context, viewer Viewer, npcID uuid.UUID, input ChangeColonyRoleInput) (ColonyChange, error) {
	return service.mutateColony(requestContext, viewer, input.ExpectedVersion, func(colony colonySnapshot, queries *store.Queries) (colonyOutcome, error) {
		npc, err := colonyNpc(requestContext, queries, colony.campaignID, npcID)
		if err != nil {
			return colonyOutcome{}, err
		}
		if npc.ColonyRoleKey == nil {
			return colonyOutcome{}, Conflict(npc.DisplayName + " isn't a colonist. Add them to the colony first.")
		}
		after, err := game.ChangeRole(colony.state, *npc.ColonyRoleKey, input.RoleKey)
		if err != nil {
			return colonyOutcome{}, gameError(err, nil)
		}
		title := npc.DisplayName + ": " + colony.state.Roles[*npc.ColonyRoleKey].Label + " → " + colony.state.Roles[input.RoleKey].Label
		visibility := TimelineVisibilityDm
		if input.Public && npc.Revealed {
			visibility = TimelineVisibilityPublic
		}
		roleKey := input.RoleKey
		return colonyOutcome{
			after: after,
			label: title,
			entries: []plannedEntry{{
				day:        colony.state.Day,
				kind:       TimelineKindWorkforceChanged,
				title:      title,
				visibility: visibility,
				npcID:      &npc.ID,
				npcName:    npc.DisplayName,
			}},
			npcRoles: map[uuid.UUID]npcRoleChange{npc.ID: {before: npc.ColonyRoleKey, after: &roleKey}},
		}, nil
	})
}

// LeaveColony takes a named colonist out of the colony: removed (Colonists -1) or returned to their
// role's unnamed pool (unchanged). COLONIST_LEFT is PUBLIC only when the NPC is revealed.
func (service *Service) LeaveColony(requestContext context.Context, viewer Viewer, npcID uuid.UUID, input LeaveColonyInput) (ColonyChange, error) {
	return service.mutateColony(requestContext, viewer, input.ExpectedVersion, func(colony colonySnapshot, queries *store.Queries) (colonyOutcome, error) {
		npc, err := colonyNpc(requestContext, queries, colony.campaignID, npcID)
		if err != nil {
			return colonyOutcome{}, err
		}
		if npc.ColonyRoleKey == nil {
			return colonyOutcome{}, Conflict(npc.DisplayName + " isn't a colonist.")
		}
		mode := cmp.Or(input.Mode, LeaveModeRemove)
		if !slices.Contains(AllLeaveModes(), mode) {
			return colonyOutcome{}, Invalid("Pick how they leave.", FieldError{Field: "mode", Message: "Pick REMOVE or RETURN_TO_POOL."})
		}
		after, err := game.LeaveColony(colony.state, *npc.ColonyRoleKey, mode == LeaveModeReturnToPool)
		if err != nil {
			return colonyOutcome{}, gameError(err, nil)
		}
		roleLabel := colony.state.Roles[*npc.ColonyRoleKey].Label
		entry := plannedEntry{
			day:        colony.state.Day,
			kind:       TimelineKindColonistLeft,
			title:      npc.DisplayName + " left the colony",
			visibility: revealedPublic(npc),
			npcID:      &npc.ID,
			npcName:    npc.DisplayName,
		}
		if mode == LeaveModeReturnToPool {
			entry.title = npc.DisplayName + " is no longer named among the " + roleLabel + " colonists"
			entry.body = "Still counted as one of them; the count is unchanged."
		}
		return colonyOutcome{
			after:    after,
			label:    entry.title,
			entries:  []plannedEntry{entry},
			npcRoles: map[uuid.UUID]npcRoleChange{npc.ID: {before: npc.ColonyRoleKey, after: nil}},
		}, nil
	})
}

// colonyNpc loads the NPC a workforce change names with the viewer's campaign, under the colony lock.
func colonyNpc(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, npcID uuid.UUID) (store.Npc, error) {
	npc, err := queries.GetNpc(requestContext, store.GetNpcParams{CampaignID: campaignID, ID: npcID})
	if err != nil {
		return store.Npc{}, npcGone(err)
	}
	return npc, nil
}

// revealedPublic is the visibility of COLONIST_JOINED and COLONIST_LEFT (spec §5.8): players may read
// about an NPC only once the DM has revealed them.
func revealedPublic(npc store.Npc) TimelineVisibility {
	if npc.Revealed {
		return TimelineVisibilityPublic
	}
	return TimelineVisibilityDm
}

func optInPublic(public bool) TimelineVisibility {
	if public {
		return TimelineVisibilityPublic
	}
	return TimelineVisibilityDm
}

// colonistCount is "1 colonist" or "3 colonists".
func colonistCount(count int64) string {
	return formatNumber(count) + " " + plural(count, "colonist", "colonists")
}

// RoleSettings is one role as the DM manages it on /settings (spec §4.2). Roles are data: adding one
// needs no code. Headcounts change only through colony mutations, never here.
type RoleSettings struct {
	// Key is derived from the label when the role is created and never changes.
	Key          string     `json:"key"`
	Label        string     `json:"label"`
	Icon         ColonyIcon `json:"icon"`
	SortOrder    int64      `json:"sortOrder"`
	UnnamedCount int64      `json:"unnamedCount"`
	NamedCount   int64      `json:"namedCount"`
	Headcount    int64      `json:"headcount" doc:"unnamedCount + namedCount; a role can be archived only at 0"`
	// IsSystem marks UNASSIGNED, where new colonists land: it keeps its name and can't be archived.
	IsSystem bool `json:"isSystem"`
	Archived bool `json:"archived"`
}

type RoleList struct {
	Roles []RoleSettings `json:"roles"`
}

type NewRoleInput struct {
	Label string     `json:"label" doc:"1 to 40 characters after trimming"`
	Icon  ColonyIcon `json:"icon"`
}

// RoleInput replaces a role's settings (PUT semantics).
type RoleInput struct {
	Label     string     `json:"label" doc:"1 to 40 characters after trimming; Unassigned keeps its name"`
	Icon      ColonyIcon `json:"icon"`
	SortOrder int64      `json:"sortOrder"`
	Archived  bool       `json:"archived" doc:"Only when nobody is in the role; Unassigned can't be archived"`
}

const roleLabelMaxLength = 40

// ListRoles is the DM's role list on /settings, archived ones included.
func (service *Service) ListRoles(requestContext context.Context, viewer Viewer) (RoleList, error) {
	if err := requireDm(viewer); err != nil {
		return RoleList{}, err
	}
	rows, err := service.queries.ListRoles(requestContext, viewer.campaignID)
	if err != nil {
		return RoleList{}, err
	}
	roles := make([]RoleSettings, 0, len(rows))
	for _, row := range rows {
		roles = append(roles, toRoleSettings(row.Role, row.NamedCount))
	}
	return RoleList{Roles: roles}, nil
}

// CreateRole adds a role with nobody in it; the DM moves people in with Transfer.
func (service *Service) CreateRole(requestContext context.Context, viewer Viewer, input NewRoleInput) (RoleSettings, error) {
	if err := requireDm(viewer); err != nil {
		return RoleSettings{}, err
	}
	label, fieldErrors := checkRoleSettings(input.Label, input.Icon)
	if len(fieldErrors) > 0 {
		return RoleSettings{}, Invalid("The role needs fixing.", fieldErrors...)
	}
	var row store.Role
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		// Not a colony mutation, but the colony row is still locked first: one lock order (spec §5.1),
		// and two adds of the same label can't race for one key.
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		existing, err := queries.ListRoles(requestContext, viewer.campaignID)
		if err != nil {
			return err
		}
		if fieldError := roleLabelTaken(label, "", existing); fieldError != nil {
			return Invalid("The role needs fixing.", *fieldError)
		}
		sortOrder, err := queries.NextRoleSortOrder(requestContext, viewer.campaignID)
		if err != nil {
			return err
		}
		taken := make(map[string]bool, len(existing))
		for _, role := range existing {
			taken[role.Role.Key] = true
		}
		row, err = queries.CreateRole(requestContext, store.CreateRoleParams{
			CampaignID: viewer.campaignID,
			Key:        uniqueKey(strings.ToUpper(keyBase(label, "role")), taken),
			Label:      label,
			Icon:       input.Icon,
			SortOrder:  sortOrder,
		})
		return err
	})
	if err != nil {
		return RoleSettings{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicPeople, TopicColony)
	return toRoleSettings(row, 0), nil
}

// UpdateRole renames, re-icons, reorders or archives a role. Archiving needs a headcount of 0
// (unnamed and named, spec §4.2); Unassigned keeps its name and is never archived.
func (service *Service) UpdateRole(requestContext context.Context, viewer Viewer, roleKey string, input RoleInput) (RoleSettings, error) {
	if err := requireDm(viewer); err != nil {
		return RoleSettings{}, err
	}
	label, fieldErrors := checkRoleSettings(input.Label, input.Icon)
	if input.SortOrder < game.MinStored || input.SortOrder > game.MaxStored {
		fieldErrors = append(fieldErrors, FieldError{Field: "sortOrder", Message: "That position can't be stored."})
	}
	if len(fieldErrors) > 0 {
		return RoleSettings{}, Invalid("The role needs fixing.", fieldErrors...)
	}
	var updated RoleSettings
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		// Locked first so no workforce change moves anyone in between the headcount check and the save.
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		// The key comes from the path: only this campaign's roles are looked at.
		existing, err := queries.ListRoles(requestContext, viewer.campaignID)
		if err != nil {
			return err
		}
		index := slices.IndexFunc(existing, func(row store.ListRolesRow) bool { return row.Role.Key == roleKey })
		if index < 0 {
			return NotFound("That role doesn't exist.")
		}
		current := existing[index]
		var fieldErrors []FieldError
		if fieldError := roleLabelTaken(label, roleKey, existing); fieldError != nil {
			fieldErrors = append(fieldErrors, *fieldError)
		}
		if current.Role.IsSystem && label != current.Role.Label {
			fieldErrors = append(fieldErrors, FieldError{Field: "label", Message: current.Role.Label + " is where new colonists land, so it keeps its name."})
		}
		if current.Role.IsSystem && input.Archived {
			fieldErrors = append(fieldErrors, FieldError{Field: "archived", Message: current.Role.Label + " is where new colonists land, so it can't be archived."})
		}
		if headcount := int64(current.Role.UnnamedCount) + current.NamedCount; input.Archived && !current.Role.Archived && headcount > 0 {
			fieldErrors = append(fieldErrors, FieldError{Field: "archived", Message: current.Role.Label + " still has " + colonistCount(headcount) + ". Move them to another role first."})
		}
		if len(fieldErrors) > 0 {
			return Invalid("The role needs fixing.", fieldErrors...)
		}
		row, err := queries.UpdateRole(requestContext, store.UpdateRoleParams{
			CampaignID: viewer.campaignID,
			Key:        roleKey,
			Label:      label,
			Icon:       input.Icon,
			SortOrder:  int32(input.SortOrder),
			Archived:   input.Archived,
		})
		if err != nil {
			return err
		}
		updated = toRoleSettings(row, current.NamedCount)
		return nil
	})
	if err != nil {
		return RoleSettings{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicPeople, TopicColony, TopicNpcs)
	return updated, nil
}

func checkRoleSettings(rawLabel string, icon ColonyIcon) (string, []FieldError) {
	var fieldErrors []FieldError
	label, fieldError := cleanText("label", rawLabel, roleLabelMaxLength)
	switch {
	case fieldError != nil:
		fieldErrors = append(fieldErrors, *fieldError)
	case label == "":
		fieldErrors = append(fieldErrors, FieldError{Field: "label", Message: "Give the role a name."})
	}
	if !slices.Contains(AllColonyIconValues(), icon) {
		fieldErrors = append(fieldErrors, FieldError{Field: "icon", Message: "Pick one of the listed icons."})
	}
	return label, fieldErrors
}

// roleLabelTaken refuses a label another role already has, ignoring case.
func roleLabelTaken(label string, ownKey string, existing []store.ListRolesRow) *FieldError {
	for _, row := range existing {
		if row.Role.Key != ownKey && strings.EqualFold(row.Role.Label, label) {
			return &FieldError{Field: "label", Message: "There is already a role called " + row.Role.Label + "."}
		}
	}
	return nil
}

func toRoleSettings(row store.Role, namedCount int64) RoleSettings {
	return RoleSettings{
		Key:          row.Key,
		Label:        row.Label,
		Icon:         row.Icon,
		SortOrder:    int64(row.SortOrder),
		UnnamedCount: int64(row.UnnamedCount),
		NamedCount:   namedCount,
		Headcount:    int64(row.UnnamedCount) + namedCount,
		IsSystem:     row.IsSystem,
		Archived:     row.Archived,
	}
}
