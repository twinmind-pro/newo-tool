package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/twinmind/newo-tool/internal/llm"
	"github.com/twinmind/newo-tool/internal/nsl/ast"
	"github.com/twinmind/newo-tool/internal/nsl/jsonschema"
	"github.com/twinmind/newo-tool/internal/nsl/printer"
)

// GenerateCommand generates NSL code using an LLM.
type GenerateCommand struct {
	stdout io.Writer
	stderr io.Writer
	prompt *string
}

// NewGenerateCommand constructs a generate command.
func NewGenerateCommand(stdout, stderr io.Writer) *GenerateCommand {
	return &GenerateCommand{stdout: stdout, stderr: stderr}
}

func (c *GenerateCommand) Name() string {
	return "generate"
}

func (c *GenerateCommand) Summary() string {
	return "Generate NSL code from a natural language prompt using an LLM"
}

func (c *GenerateCommand) RegisterFlags(fs *flag.FlagSet) {
	c.prompt = fs.String("prompt", "", "Natural language prompt for code generation")
}

func (c *GenerateCommand) Run(ctx context.Context, _ []string) error {
	if c.prompt == nil || strings.TrimSpace(*c.prompt) == "" {
		return fmt.Errorf("prompt is required")
	}

	// 1. Generate JSON Schema for NSL AST
	generator := jsonschema.New()
	programSchema, err := generator.Generate(reflect.TypeOf(&ast.Program{}))
	if err != nil {
		return fmt.Errorf("failed to generate NSL AST schema: %w", err)
	}

	programSchemaJSON, err := jsonschema.ToJSONString(programSchema)
	if err != nil {
		return fmt.Errorf("failed to marshal NSL AST schema to JSON: %w", err)
	}

	// 2. Construct LLM prompt with schema
	llmPrompt := fmt.Sprintf("Generate NSL code as a JSON object conforming to the following JSON Schema. The JSON object should represent the Abstract Syntax Tree (AST) of the NSL code. The NSL code should fulfill the following request: \"%s\".\n\nJSON Schema:\n%s\n\nJSON AST:", *c.prompt, programSchemaJSON)

	// 3. Call LLM
	llmClient := llm.NewClient()
	mockLLMResponse, err := llmClient.GenerateCode(llmPrompt)
	if err != nil {
		return fmt.Errorf("LLM code generation failed: %w", err)
	}

	// 4. Unmarshal LLM response into AST
	var generatedProgram ast.Program
	if err := unmarshalASTJSON([]byte(mockLLMResponse), &generatedProgram); err != nil {
		return fmt.Errorf("failed to unmarshal LLM response into AST: %w", err)
	}

	// 5. Convert AST to NSL code using Pretty-Printer
	prettyPrinter := printer.New()
	generatedNSL := prettyPrinter.Print(&generatedProgram)

	_, err = fmt.Fprintln(c.stdout, generatedNSL)
	return err
}

// unmarshalASTJSON unmarshals the LLM's JSON response into an ast.Program.
func unmarshalASTJSON(data []byte, program *ast.Program) error {
	var temp struct {
		Statements []json.RawMessage `json:"Statements"` // Unmarshal raw messages first
	}

	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	program.Statements = make([]ast.Statement, len(temp.Statements))
	for i, rawStmt := range temp.Statements {
		var sw struct {
			Type string `json:"_type"`
		}
		if err := json.Unmarshal(rawStmt, &sw); err != nil {
			return err
		}

		switch sw.Type {
		case "SetStatement":
			var stmt ast.SetStatement
			if err := json.Unmarshal(rawStmt, &stmt); err != nil {
				return err
			}
			program.Statements[i] = &stmt
		case "OutputStatement":
			var stmt ast.OutputStatement
			if err := json.Unmarshal(rawStmt, &stmt); err != nil {
				return err
			}
			program.Statements[i] = &stmt
		case "ExpressionStatement":
			var stmt ast.ExpressionStatement
			if err := json.Unmarshal(rawStmt, &stmt); err != nil {
				return err
			}
			program.Statements[i] = &stmt
		case "IfStatement":
			var stmt ast.IfStatement
			if err := json.Unmarshal(rawStmt, &stmt); err != nil {
				return err
			}
			program.Statements[i] = &stmt
		case "ForStatement":
			var stmt ast.ForStatement
			if err := json.Unmarshal(rawStmt, &stmt); err != nil {
				return err
			}
			program.Statements[i] = &stmt
		case "BlockStatement":
			var stmt ast.BlockStatement
			if err := json.Unmarshal(rawStmt, &stmt); err != nil {
				return err
			}
			program.Statements[i] = &stmt
		default:
			return fmt.Errorf("unknown statement type: %s", sw.Type)
		}
	}
	return nil
}