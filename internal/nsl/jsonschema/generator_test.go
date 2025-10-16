package jsonschema

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/twinmind/newo-tool/internal/nsl/ast"
	"github.com/twinmind/newo-tool/internal/nsl/token"
)

func TestGenerateProgramSchema(t *testing.T) {
	generator := New()
	schema, err := generator.Generate(reflect.TypeOf(&ast.Program{}))
	if err != nil {
		t.Fatalf("Failed to generate schema: %v", err)
	}

	// Explicitly generate schemas for types that are expected to be in definitions
	// This is a temporary measure until a proper oneOf implementation for interfaces is done.
	_, err = generator.Generate(reflect.TypeOf(&ast.Identifier{}))
	if err != nil {
		t.Fatalf("Failed to generate schema for Identifier: %v", err)
	}
	_, err = generator.Generate(reflect.TypeOf(&ast.SetStatement{}))
	if err != nil {
		t.Fatalf("Failed to generate schema for SetStatement: %v", err)
	}
	_, err = generator.Generate(reflect.TypeOf(&ast.IntegerLiteral{}))
	if err != nil {
		t.Fatalf("Failed to generate schema for IntegerLiteral: %v", err)
	}
	_, err = generator.Generate(reflect.TypeOf(&ast.StringLiteral{}))
	if err != nil {
		t.Fatalf("Failed to generate schema for StringLiteral: %v", err)
	}
	_, err = generator.Generate(reflect.TypeOf(&ast.Boolean{}))
	if err != nil {
		t.Fatalf("Failed to generate schema for Boolean: %v", err)
	}
	_, err = generator.Generate(reflect.TypeOf(&ast.InfixExpression{}))
	if err != nil {
		t.Fatalf("Failed to generate schema for InfixExpression: %v", err)
	}
	_, err = generator.Generate(reflect.TypeOf(&ast.PrefixExpression{}))
	if err != nil {
		t.Fatalf("Failed to generate schema for PrefixExpression: %v", err)
	}
	_, err = generator.Generate(reflect.TypeOf(&ast.AttributeAccess{}))
	if err != nil {
		t.Fatalf("Failed to generate schema for AttributeAccess: %v", err)
	}
	_, err = generator.Generate(reflect.TypeOf(&ast.FilterExpression{}))
	if err != nil {
		t.Fatalf("Failed to generate schema for FilterExpression: %v", err)
	}
	_, err = generator.Generate(reflect.TypeOf(&token.Token{}))
	if err != nil {
		t.Fatalf("Failed to generate schema for Token: %v", err)
	}

	jsonString, err := ToJSONString(schema)
	if err != nil {
		t.Fatalf("Failed to convert schema to JSON string: %v", err)
	}

	var result map[string]interface{}
	err = json.Unmarshal([]byte(jsonString), &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON string: %v", err)
	}

	definitions, ok := result["definitions"].(map[string]interface{})
	if !ok {
		t.Fatalf("Schema missing definitions map")
	}

	// Basic checks for the generated schema
	if !strings.Contains(jsonString, "\"$schema\": \"http://json-schema.org/draft-07/schema#\"") {
		t.Errorf("Schema missing $schema definition")
	}
	if !strings.Contains(jsonString, "\"$ref\": \"#/definitions/Program\"") {
		t.Errorf("Root schema missing $ref to Program definition")
	}

	// Check for Program definition
	if _, ok := definitions["Program"]; !ok {
		t.Errorf("Schema missing Program definition in definitions")
	}
	programDef := definitions["Program"].(map[string]interface{})
	props, ok := programDef["properties"].(map[string]interface{})
	if !ok {
		t.Errorf("Program definition missing properties")
	}
	statementsProp, ok := props["Statements"].(map[string]interface{})
	if !ok {
		t.Errorf("Program definition missing Statements property")
	}
	if statementsProp["type"] != "array" {
		t.Errorf("Statements property not of type array")
	}
	items, ok := statementsProp["items"].(map[string]interface{})
	if !ok {
		t.Errorf("Statements array missing items definition")
	}
	if items["description"] != "Interface Statement (concrete type unknown at schema generation)" {
		t.Errorf("Statements items not referencing Statement interface correctly")
	}

	// Check for Identifier definition
	if _, ok := definitions["Identifier"]; !ok {
		t.Errorf("Schema missing Identifier definition in definitions")
	}
	identifierDef := definitions["Identifier"].(map[string]interface{})
	identProps, ok := identifierDef["properties"].(map[string]interface{})
	if !ok {
		t.Errorf("Identifier definition missing properties")
	}
	if identProps["Value"].(map[string]interface{})["type"] != "string" {
		t.Errorf("Identifier Value not of type string")
	}

	// Check for SetStatement definition
	if _, ok := definitions["SetStatement"]; !ok {
		t.Errorf("Schema missing SetStatement definition in definitions")
	}
	setStmtDef := definitions["SetStatement"].(map[string]interface{})
	setStmtProps, ok := setStmtDef["properties"].(map[string]interface{})
	if !ok {
		t.Errorf("SetStatement definition missing properties")
	}
	if setStmtProps["Name"].(map[string]interface{})["$ref"] != "#/definitions/Identifier" {
		t.Errorf("SetStatement Name not referencing Identifier")
	}

	// Check for IntegerLiteral definition
	if _, ok := definitions["IntegerLiteral"]; !ok {
		t.Errorf("Schema missing IntegerLiteral definition in definitions")
	}
	intLitDef := definitions["IntegerLiteral"].(map[string]interface{})
	intLitProps, ok := intLitDef["properties"].(map[string]interface{})
	if !ok {
		t.Errorf("IntegerLiteral definition missing properties")
	}
	if intLitProps["Value"].(map[string]interface{})["type"] != "integer" {
		t.Errorf("IntegerLiteral Value not of type integer")
	}

	// Check for Token definition (nested struct)
	if _, ok := definitions["Token"]; !ok {
		t.Errorf("Schema missing Token definition in definitions")
	}

	// t.Logf("Generated Schema:\n%s", jsonString) // Uncomment to see the full schema
}

func TestGenerateComplexSchema(t *testing.T) {
	generator := New()
	schema, err := generator.Generate(reflect.TypeOf(&ast.IfStatement{}))
	if err != nil {
		t.Fatalf("Failed to generate schema for IfStatement: %v", err)
	}

	jsonString, err := ToJSONString(schema)
	if err != nil {
		t.Fatalf("Failed to convert schema to JSON string: %v", err)
	}

	var result map[string]interface{}
	err = json.Unmarshal([]byte(jsonString), &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON string: %v", err)
	}

	definitions, ok := result["definitions"].(map[string]interface{})
	if !ok {
		t.Fatalf("Schema missing definitions map")
	}

	// Check for IfStatement specific properties
	if _, ok := definitions["IfStatement"]; !ok {
		t.Errorf("Schema missing IfStatement definition")
	}
	ifStmtDef := definitions["IfStatement"].(map[string]interface{})
	props, ok := ifStmtDef["properties"].(map[string]interface{})
	if !ok {
		t.Errorf("IfStatement definition missing properties")
	}
	if _, ok := props["Condition"]; !ok {
		t.Errorf("IfStatement missing Condition property")
	}
	if _, ok := props["Consequence"]; !ok {
		t.Errorf("IfStatement missing Consequence property")
	}
	if _, ok := props["ElseIfs"]; !ok {
		t.Errorf("IfStatement missing ElseIfs property")
	}
	if _, ok := props["Alternative"]; !ok {
		t.Errorf("IfStatement missing Alternative property")
	}

	// Check for recursive definition of BlockStatement
	if _, ok := definitions["BlockStatement"]; !ok {
		t.Errorf("Schema missing BlockStatement definition")
	}
	blockStmtDef := definitions["BlockStatement"].(map[string]interface{})
	blockProps, ok := blockStmtDef["properties"].(map[string]interface{})
	if !ok {
		t.Errorf("BlockStatement definition missing properties")
	}
	statementsPropLocal, ok := blockProps["Statements"].(map[string]interface{})
	if !ok {
		t.Errorf("BlockStatement missing Statements property")
	}
	if statementsPropLocal["type"] != "array" {
		t.Errorf("BlockStatement Statements property not of type array")
	}
	itemsLocal, ok := statementsPropLocal["items"].(map[string]interface{})
	if !ok {
		t.Errorf("BlockStatement Statements array missing items definition")
	}
	if itemsLocal["description"] != "Interface Statement (concrete type unknown at schema generation)" {
		t.Errorf("BlockStatement Statements items not referencing Statement interface correctly")
	}

	// t.Logf("Generated IfStatement Schema:\n%s", jsonString) // Uncomment to see the full schema
}





