package app

import (
	"reflect"

	"github.com/danielgtaylor/huma/v2"

	"github.com/BBruington/colony-planner/internal/core"
)

// enumType describes one closed set. The values come from generated code (sqlc's AllXValues for
// Postgres enums), so a migration stays the only place a value is written.
type enumType struct {
	name         string
	values       []any
	goType       reflect.Type
	providerType reflect.Type
}

func newEnumType[Value ~string](name string, allValues func() []Value) enumType {
	values := make([]any, 0)
	for _, value := range allValues() {
		values = append(values, string(value))
	}
	return enumType{
		name:         name,
		values:       values,
		goType:       reflect.TypeFor[Value](),
		providerType: reflect.TypeFor[enumSchema[Value]](),
	}
}

// enumTypes lists every closed set the API exposes. Add one line per new Postgres enum.
var enumTypes = []enumType{
	newEnumType("MemberStatus", core.AllMemberStatusValues),
	newEnumType("CampaignRole", core.AllCampaignRoleValues),
	newEnumType("CampaignStatus", core.AllCampaignStatusValues),
	newEnumType("CampaignRequestStatus", core.AllCampaignRequestStatusValues),
	newEnumType("Topic", core.AllTopics),
	newEnumType("ColonyIcon", core.AllColonyIconValues),
	newEnumType("TimelineVisibility", core.AllTimelineVisibilityValues),
	newEnumType("TimelineKind", core.AllTimelineKinds),
	newEnumType("AdjustSource", core.AllAdjustSources),
	newEnumType("RemovalKind", core.AllRemovalKinds),
	newEnumType("NpcStatus", core.AllNpcStatusValues),
	newEnumType("NpcHealth", core.AllNpcHealthValues),
	newEnumType("NpcRelation", core.AllNpcRelationValues),
	newEnumType("JoinMode", core.AllJoinModes),
	newEnumType("LeaveMode", core.AllLeaveModes),
	newEnumType("QuestKind", core.AllQuestKindValues),
	newEnumType("QuestStatus", core.AllQuestStatusValues),
	newEnumType("RequestStatus", core.AllRequestStatusValues),
	newEnumType("RequestKind", core.AllRequestKinds),
	newEnumType("Terrain", core.AllTerrainValues),
}

var enumNamesByType = func() map[reflect.Type]string {
	names := map[reflect.Type]string{}
	for _, enum := range enumTypes {
		names[enum.goType] = enum.name
	}
	return names
}()

// enumSchema stands in for an enum type during schema generation. huma can't attach a schema to a
// type it doesn't own (sqlc's output), so each enum is aliased to this provider, which points at the
// named component and so becomes a named string union in TypeScript.
type enumSchema[Value ~string] string

func (enumSchema[Value]) Schema(huma.Registry) *huma.Schema {
	return &huma.Schema{Ref: "#/components/schemas/" + enumNamesByType[reflect.TypeFor[Value]()]}
}

// registerSchemas adds the enum components, and the types that only travel outside huma operations
// (the SSE message), so the generated TypeScript has a name for each.
func registerSchemas(api huma.API) {
	document := api.OpenAPI()
	registry := document.Components.Schemas
	for _, enum := range enumTypes {
		registry.Map()[enum.name] = &huma.Schema{Type: huma.TypeString, Enum: enum.values}
		registry.RegisterTypeAlias(enum.goType, enum.providerType)
	}
	// huma prunes components nothing references; this extension documents the stream's messages and
	// keeps their schemas in the document.
	if document.Extensions == nil {
		document.Extensions = map[string]any{}
	}
	document.Extensions["x-stream-events"] = map[string]*huma.Schema{
		"invalidate": registry.Schema(reflect.TypeFor[core.InvalidateMessage](), true, "InvalidateMessage"),
	}
}

// nullableEnumFields rewrites each nullable enum field (a pointer to an enum type, tagged
// nullable:"true") as anyOf the enum and null. huma would set a null type beside the $ref, which isn't
// valid JSON Schema, and TypeScript would read the field as never null. Run it once every operation
// is registered.
func nullableEnumFields(document *huma.OpenAPI) {
	for _, schema := range document.Components.Schemas.Map() {
		for name, property := range schema.Properties {
			if property == nil || property.Ref == "" || !property.Nullable {
				continue
			}
			nullable := &huma.Schema{
				Description: property.Description,
				AnyOf:       []*huma.Schema{{Ref: property.Ref}, {Type: "null"}},
			}
			nullable.PrecomputeMessages()
			schema.Properties[name] = nullable
		}
	}
}
