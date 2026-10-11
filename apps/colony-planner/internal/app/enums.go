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
	newEnumType("Topic", core.AllTopics),
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
