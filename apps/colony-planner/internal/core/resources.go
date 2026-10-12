package core

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/BBruington/colony-planner/internal/game"
	"github.com/BBruington/colony-planner/internal/store"
)

// ResourceSettings is one resource as the DM manages it on /settings (spec §4.2, §6.2). Resources are
// data: adding one needs no code. Amounts change only through colony mutations, never here.
type ResourceSettings struct {
	// Key is derived from the label when the resource is created and never changes, because Effects
	// JSON (timeline deltas, later rewards and actions) names resources by key.
	Key       string     `json:"key"`
	Label     string     `json:"label"`
	Icon      ColonyIcon `json:"icon"`
	Amount    int64      `json:"amount"`
	MinAmount int64      `json:"minAmount"`
	MaxAmount *int64     `json:"maxAmount" doc:"null when unbounded"`
	SortOrder int64      `json:"sortOrder"`
	// IsSystem marks a resource a rule reads (food); it can be renamed but not archived.
	IsSystem bool `json:"isSystem"`
	Archived bool `json:"archived"`
}

// ResourceList is every resource, archived ones included, in board order.
type ResourceList struct {
	Resources []ResourceSettings `json:"resources"`
}

// NewResourceInput adds a resource. It starts at the amount nearest 0 within its bounds; the DM
// sets a starting stock with Adjust, so the timeline records it.
type NewResourceInput struct {
	Label     string     `json:"label" doc:"1 to 40 characters after trimming"`
	Icon      ColonyIcon `json:"icon"`
	MinAmount int64      `json:"minAmount"`
	MaxAmount *int64     `json:"maxAmount" doc:"null for no maximum; at least minAmount"`
}

// ResourceInput replaces a resource's settings (PUT semantics). New bounds must hold the current
// amount: the DM adjusts the amount first.
type ResourceInput struct {
	Label     string     `json:"label" doc:"1 to 40 characters after trimming"`
	Icon      ColonyIcon `json:"icon"`
	MinAmount int64      `json:"minAmount"`
	MaxAmount *int64     `json:"maxAmount" doc:"null for no maximum; at least minAmount"`
	SortOrder int64      `json:"sortOrder"`
	Archived  bool       `json:"archived" doc:"A system resource (food) can't be archived"`
}

// ListResources is the DM's resource list on /settings.
func (service *Service) ListResources(requestContext context.Context, viewer Viewer) (ResourceList, error) {
	if err := requireDm(viewer); err != nil {
		return ResourceList{}, err
	}
	rows, err := service.queries.ListResources(requestContext, viewer.campaignID)
	if err != nil {
		return ResourceList{}, err
	}
	resources := make([]ResourceSettings, 0, len(rows))
	for _, row := range rows {
		resources = append(resources, toResourceSettings(row))
	}
	return ResourceList{Resources: resources}, nil
}

const (
	resourceLabelMaxLength = 40
	// resourceKeyMaxLength matches the resource.key CHECK.
	resourceKeyMaxLength = 40
)

// CreateResource adds a resource. It publishes resources and colony (the board lists resources).
func (service *Service) CreateResource(requestContext context.Context, viewer Viewer, input NewResourceInput) (ResourceSettings, error) {
	if err := requireDm(viewer); err != nil {
		return ResourceSettings{}, err
	}
	label, fieldErrors := checkResourceSettings(input.Label, input.Icon, input.MinAmount, input.MaxAmount)
	if len(fieldErrors) > 0 {
		return ResourceSettings{}, Invalid("The resource needs fixing.", fieldErrors...)
	}
	var row store.Resource
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		// Not a colony mutation, but the colony row is still locked first: one lock order (spec §5.1),
		// and two adds of the same label can't race for one key.
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		existing, err := queries.ListResources(requestContext, viewer.campaignID)
		if err != nil {
			return err
		}
		if fieldError := labelTaken(label, "", existing); fieldError != nil {
			return Invalid("The resource needs fixing.", *fieldError)
		}
		sortOrder, err := queries.NextResourceSortOrder(requestContext, viewer.campaignID)
		if err != nil {
			return err
		}
		row, err = queries.CreateResource(requestContext, store.CreateResourceParams{
			CampaignID: viewer.campaignID,
			Key:        resourceKeyFor(label, existing),
			Label:      label,
			Icon:       input.Icon,
			Amount:     int32(startingAmount(input.MinAmount, input.MaxAmount)),
			MinAmount:  int32(input.MinAmount),
			MaxAmount:  int32PointerOf(input.MaxAmount),
			SortOrder:  sortOrder,
		})
		return err
	})
	if err != nil {
		return ResourceSettings{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicResources, TopicColony)
	return toResourceSettings(row), nil
}

// UpdateResource replaces a resource's settings. It is not a colony mutation: no version, no timeline.
func (service *Service) UpdateResource(requestContext context.Context, viewer Viewer, resourceKey string, input ResourceInput) (ResourceSettings, error) {
	if err := requireDm(viewer); err != nil {
		return ResourceSettings{}, err
	}
	label, fieldErrors := checkResourceSettings(input.Label, input.Icon, input.MinAmount, input.MaxAmount)
	if input.SortOrder < game.MinStored || input.SortOrder > game.MaxStored {
		fieldErrors = append(fieldErrors, FieldError{Field: "sortOrder", Message: "That position can't be stored."})
	}
	if len(fieldErrors) > 0 {
		return ResourceSettings{}, Invalid("The resource needs fixing.", fieldErrors...)
	}
	var row store.Resource
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		// Locked first so no colony mutation changes the amount between the bounds check and the save.
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		// The key comes from the path: only this campaign's resources are looked at.
		existing, err := queries.ListResources(requestContext, viewer.campaignID)
		if err != nil {
			return err
		}
		index := slices.IndexFunc(existing, func(resource store.Resource) bool { return resource.Key == resourceKey })
		if index < 0 {
			return NotFound("That resource doesn't exist.")
		}
		current := existing[index]
		var fieldErrors []FieldError
		if fieldError := labelTaken(label, resourceKey, existing); fieldError != nil {
			fieldErrors = append(fieldErrors, *fieldError)
		}
		amount := int64(current.Amount)
		if amount < input.MinAmount {
			fieldErrors = append(fieldErrors, FieldError{Field: "minAmount", Message: fmt.Sprintf(
				"%s is at %s now, below that minimum. Adjust it first.", current.Label, formatNumber(amount))})
		}
		if input.MaxAmount != nil && amount > *input.MaxAmount {
			fieldErrors = append(fieldErrors, FieldError{Field: "maxAmount", Message: fmt.Sprintf(
				"%s is at %s now, above that maximum. Adjust it first.", current.Label, formatNumber(amount))})
		}
		if current.IsSystem && input.Archived {
			fieldErrors = append(fieldErrors, FieldError{Field: "archived", Message: current.Label + " is used by upkeep, so it can't be archived."})
		}
		if len(fieldErrors) > 0 {
			return Invalid("The resource needs fixing.", fieldErrors...)
		}
		row, err = queries.UpdateResource(requestContext, store.UpdateResourceParams{
			CampaignID: viewer.campaignID,
			Key:        resourceKey,
			Label:      label,
			Icon:       input.Icon,
			MinAmount:  int32(input.MinAmount),
			MaxAmount:  int32PointerOf(input.MaxAmount),
			SortOrder:  int32(input.SortOrder),
			Archived:   input.Archived,
		})
		return err
	})
	if err != nil {
		return ResourceSettings{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicResources, TopicColony)
	return toResourceSettings(row), nil
}

// checkResourceSettings checks what adding and editing share: the label, the icon and the bounds.
func checkResourceSettings(rawLabel string, icon ColonyIcon, minAmount int64, maxAmount *int64) (string, []FieldError) {
	var fieldErrors []FieldError
	label, fieldError := cleanText("label", rawLabel, resourceLabelMaxLength)
	switch {
	case fieldError != nil:
		fieldErrors = append(fieldErrors, *fieldError)
	case label == "":
		fieldErrors = append(fieldErrors, FieldError{Field: "label", Message: "Give the resource a name."})
	}
	if !slices.Contains(AllColonyIconValues(), icon) {
		fieldErrors = append(fieldErrors, FieldError{Field: "icon", Message: "Pick one of the listed icons."})
	}
	if minAmount < game.MinStored || minAmount > game.MaxStored {
		fieldErrors = append(fieldErrors, FieldError{Field: "minAmount", Message: "That minimum can't be stored."})
	}
	if maxAmount != nil {
		switch {
		case *maxAmount < game.MinStored || *maxAmount > game.MaxStored:
			fieldErrors = append(fieldErrors, FieldError{Field: "maxAmount", Message: "That maximum can't be stored."})
		case *maxAmount < minAmount:
			fieldErrors = append(fieldErrors, FieldError{Field: "maxAmount", Message: "The maximum can't be below the minimum."})
		}
	}
	return label, fieldErrors
}

// labelTaken refuses a label another resource already has, ignoring case, so the board never shows
// two tiles with one name.
func labelTaken(label string, ownKey string, existing []store.Resource) *FieldError {
	for _, resource := range existing {
		if resource.Key != ownKey && strings.EqualFold(resource.Label, label) {
			return &FieldError{Field: "label", Message: "There is already a resource called " + resource.Label + "."}
		}
	}
	return nil
}

// resourceKeyFor derives a new resource's key from its label ("Dragon Scales" → dragon_scales), with
// a numeric suffix when it is taken. Keys never change afterwards: Effects JSON names resources by key.
func resourceKeyFor(label string, existing []store.Resource) string {
	var builder strings.Builder
	separator := false
	for _, character := range strings.ToLower(label) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			if separator && builder.Len() > 0 {
				builder.WriteByte('_')
			}
			separator = false
			builder.WriteRune(character)
			continue
		}
		separator = true
	}
	base := builder.String()
	if base == "" || (base[0] >= '0' && base[0] <= '9') {
		base = "resource_" + base
	}
	base = strings.TrimSuffix(base, "_")
	// Room for a suffix such as _12 within the key CHECK.
	if len(base) > resourceKeyMaxLength-4 {
		base = strings.TrimRight(base[:resourceKeyMaxLength-4], "_")
	}
	taken := make(map[string]bool, len(existing))
	for _, resource := range existing {
		taken[resource.Key] = true
	}
	key := base
	for suffix := 2; taken[key]; suffix++ {
		key = base + "_" + strconv.Itoa(suffix)
	}
	return key
}

// startingAmount is the amount nearest 0 within the bounds; the DM sets real stock with Adjust, so
// the timeline records it.
func startingAmount(minAmount int64, maxAmount *int64) int64 {
	switch {
	case minAmount > 0:
		return minAmount
	case maxAmount != nil && *maxAmount < 0:
		return *maxAmount
	default:
		return 0
	}
}

func int32PointerOf(value *int64) *int32 {
	if value == nil {
		return nil
	}
	narrowed := int32(*value)
	return &narrowed
}

func toResourceSettings(row store.Resource) ResourceSettings {
	return ResourceSettings{
		Key:       row.Key,
		Label:     row.Label,
		Icon:      row.Icon,
		Amount:    int64(row.Amount),
		MinAmount: int64(row.MinAmount),
		MaxAmount: int64Pointer(row.MaxAmount),
		SortOrder: int64(row.SortOrder),
		IsSystem:  row.IsSystem,
		Archived:  row.Archived,
	}
}
