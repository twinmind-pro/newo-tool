package jsonschema

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// SchemaGenerator generates JSON Schema from Go structs.
type SchemaGenerator struct {
	definitions map[string]map[string]interface{}
	processed   map[reflect.Type]string // Tracks types already processed to avoid infinite recursion
}

// New creates a new SchemaGenerator.
func New() *SchemaGenerator {
	return &SchemaGenerator{
		definitions: make(map[string]map[string]interface{}),
		processed:   make(map[reflect.Type]string),
	}
}

// Generate takes a root Go type (e.g., ast.Program) and returns its JSON Schema representation.
func (g *SchemaGenerator) Generate(rootType reflect.Type) (map[string]interface{}, error) {
	// Initialize the root schema
	rootSchema := map[string]interface{}{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type":    "object",
	}

	// Generate schema for the root type
	schema, err := g.generateSchemaForType(rootType)
	if err != nil {
		return nil, err
	}

	// If the root type is a struct, its definition will be in definitions,
	// so the root schema should reference it.
	if rootType.Kind() == reflect.Ptr {
		rootType = rootType.Elem()
	}
	if rootType.Kind() == reflect.Struct {
		rootSchema["$ref"] = "#/definitions/" + rootType.Name()
	} else {
		// If rootType is not a struct, just merge its schema directly
		for k, v := range schema {
			rootSchema[k] = v
		}
	}

	if len(g.definitions) > 0 {
		rootSchema["definitions"] = g.definitions
	}

	return rootSchema, nil
}

func (g *SchemaGenerator) generateSchemaForType(t reflect.Type) (map[string]interface{}, error) {
	// Handle pointers
	if t.Kind() == reflect.Ptr {
		return g.generateSchemaForType(t.Elem())
	}

	// If it's a struct, check if already processed or add to definitions
	if t.Kind() == reflect.Struct {
		if refName, ok := g.processed[t]; ok {
			return map[string]interface{}{ "$ref": "#/definitions/" + refName }, nil
		}
		// Mark as processed and add a placeholder to definitions to handle circular dependencies
		g.processed[t] = t.Name()
		g.definitions[t.Name()] = map[string]interface{}{"type": "object"} // Placeholder

		schema := make(map[string]interface{})
		schema["type"] = "object"
		properties := make(map[string]interface{})

		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			jsonTag := field.Tag.Get("json")
			if jsonTag == "-" { // Skip ignored fields
				continue
			}
			fieldName := field.Name
			if jsonTag != "" {
				parts := strings.Split(jsonTag, ",")
				if parts[0] != "" {
					fieldName = parts[0]
				}
			}

			fieldSchema, err := g.generateSchemaForType(field.Type)
			if err != nil {
				return nil, err
			}
			properties[fieldName] = fieldSchema
		}
		schema["properties"] = properties
		// Update the placeholder with the full schema
		g.definitions[t.Name()] = schema
		return map[string]interface{}{ "$ref": "#/definitions/" + t.Name() }, nil
	}

	// Handle other types directly
	schema := make(map[string]interface{})
	switch t.Kind() {
	case reflect.String:
		schema["type"] = "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		schema["type"] = "integer"
	case reflect.Float32, reflect.Float64:
		schema["type"] = "number"
	case reflect.Bool:
		schema["type"] = "boolean"
	case reflect.Slice, reflect.Array:
		schema["type"] = "array"
		itemsSchema, err := g.generateSchemaForType(t.Elem())
		if err != nil {
			return nil, err
		}
		schema["items"] = itemsSchema
	case reflect.Map:
		schema["type"] = "object" // Generic object for maps
	case reflect.Interface:
		schema["type"] = "object"
		schema["description"] = fmt.Sprintf("Interface %s (concrete type unknown at schema generation)", t.Name())
	default:
		return nil, fmt.Errorf("unsupported type kind: %s", t.Kind())
	}

	return schema, nil
}
// ToJSONString converts a schema map to a pretty-printed JSON string.
func ToJSONString(schema map[string]interface{}) (string, error) {
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal schema to JSON: %w", err)
	}
	return string(data), nil
}