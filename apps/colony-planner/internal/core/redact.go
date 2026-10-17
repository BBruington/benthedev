package core

import (
	"reflect"
	"strings"
)

// dmKey is the reserved JSON name for DM-only data. Every DM-only field lives in a `dm` object, so
// redaction is one walk instead of a decision in every handler (spec §2.2, item 4).
const dmKey = "dm"

// RedactDm returns a deep copy of value with every `dm` object removed at any depth: struct fields
// whose JSON name is "dm" are zeroed (nil for the usual pointer sub-struct) and map entries keyed "dm"
// are dropped. The original value is never modified, so cached or shared data stays intact.
func RedactDm(value any) any {
	if value == nil {
		return nil
	}
	return redactValue(reflect.ValueOf(value)).Interface()
}

func redactValue(value reflect.Value) reflect.Value {
	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return value
		}
		copied := reflect.New(value.Type().Elem())
		copied.Elem().Set(redactValue(value.Elem()))
		return copied
	case reflect.Interface:
		if value.IsNil() {
			return value
		}
		wrapped := reflect.New(value.Type()).Elem()
		wrapped.Set(redactValue(value.Elem()))
		return wrapped
	case reflect.Struct:
		return redactStruct(value)
	case reflect.Slice:
		if value.IsNil() {
			return value
		}
		copied := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for index := range value.Len() {
			copied.Index(index).Set(redactValue(value.Index(index)))
		}
		return copied
	case reflect.Array:
		copied := reflect.New(value.Type()).Elem()
		for index := range value.Len() {
			copied.Index(index).Set(redactValue(value.Index(index)))
		}
		return copied
	case reflect.Map:
		return redactMap(value)
	default:
		return value
	}
}

func redactStruct(value reflect.Value) reflect.Value {
	copied := reflect.New(value.Type()).Elem()
	// Copying the whole struct first keeps unexported fields (time.Time, for example) intact.
	copied.Set(value)
	structType := value.Type()
	for index := range structType.NumField() {
		field := structType.Field(index)
		if !field.IsExported() {
			continue
		}
		if isDmField(field) {
			copied.Field(index).SetZero()
			continue
		}
		copied.Field(index).Set(redactValue(value.Field(index)))
	}
	return copied
}

func redactMap(value reflect.Value) reflect.Value {
	if value.IsNil() {
		return value
	}
	copied := reflect.MakeMapWithSize(value.Type(), value.Len())
	entries := value.MapRange()
	for entries.Next() {
		key := entries.Key()
		if key.Kind() == reflect.String && key.String() == dmKey {
			continue
		}
		copied.SetMapIndex(key, redactValue(entries.Value()))
	}
	return copied
}

func isDmField(field reflect.StructField) bool {
	jsonName, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	if jsonName == "" {
		jsonName = field.Name
	}
	return strings.EqualFold(jsonName, dmKey)
}
