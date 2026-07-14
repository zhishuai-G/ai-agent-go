// Package schema generates the JSON Schema subset needed for structured LLM
// output and tool arguments.
package schema

import (
	"reflect"
	"strings"
)

// Schema is a portable JSON Schema subset suitable for LLM tool definitions.
type Schema struct {
	Type                 string             `json:"type,omitempty"`
	Description          string             `json:"description,omitempty"`
	Properties           map[string]*Schema `json:"properties,omitempty"`
	Items                *Schema            `json:"items,omitempty"`
	Required             []string           `json:"required,omitempty"`
	AdditionalProperties *Schema            `json:"additionalProperties,omitempty"`
}

// Generate creates a schema for v's type. It supports common Go primitives,
// structs, slices, arrays, maps, pointers, and json/desc field tags.
func Generate(v any) *Schema {
	if v == nil {
		return nil
	}
	return schemaForType(reflect.TypeOf(v), make(map[reflect.Type]bool))
}

func schemaForType(typ reflect.Type, visiting map[reflect.Type]bool) *Schema {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	switch typ.Kind() {
	case reflect.Bool:
		return &Schema{Type: "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &Schema{Type: "integer"}
	case reflect.Float32, reflect.Float64:
		return &Schema{Type: "number"}
	case reflect.String:
		return &Schema{Type: "string"}
	case reflect.Slice, reflect.Array:
		return &Schema{Type: "array", Items: schemaForType(typ.Elem(), visiting)}
	case reflect.Map:
		return &Schema{Type: "object", AdditionalProperties: schemaForType(typ.Elem(), visiting)}
	case reflect.Interface:
		return &Schema{Type: "object"}
	case reflect.Struct:
		if visiting[typ] {
			return &Schema{Type: "object"}
		}
		visiting[typ] = true
		defer delete(visiting, typ)

		schema := &Schema{Type: "object", Properties: make(map[string]*Schema)}
		for index := range typ.NumField() {
			field := typ.Field(index)
			if field.PkgPath != "" {
				continue
			}
			name, omitEmpty, include := jsonFieldName(field)
			if !include {
				continue
			}
			property := schemaForType(field.Type, visiting)
			if description := field.Tag.Get("desc"); description != "" {
				property.Description = description
			}
			schema.Properties[name] = property
			if !omitEmpty {
				schema.Required = append(schema.Required, name)
			}
		}
		return schema
	default:
		return &Schema{Type: "object"}
	}
}

func jsonFieldName(field reflect.StructField) (name string, omitEmpty bool, include bool) {
	tag := field.Tag.Get("json")
	parts := strings.Split(tag, ",")
	if parts[0] == "-" {
		return "", false, false
	}
	name = parts[0]
	if name == "" {
		name = field.Name
	}
	for _, option := range parts[1:] {
		if option == "omitempty" {
			omitEmpty = true
		}
	}
	return name, omitEmpty, true
}
