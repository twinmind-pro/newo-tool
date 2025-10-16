package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/twinmind/newo-tool/internal/config"
	"github.com/twinmind/newo-tool/internal/llm"
	"github.com/twinmind/newo-tool/internal/nsl/lexer"
	"github.com/twinmind/newo-tool/internal/nsl/parser"
	"github.com/twinmind/newo-tool/internal/nsl/printer"
)

// GenerateCommand generates NSL code using an LLM.
type GenerateCommand struct {
	stdout    io.Writer
	stderr    io.Writer
	prompt    *string
	llmClient llm.LLMClient // Allow overriding for tests
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

	// Use the pre-configured llmClient if it exists (for testing).
	// Otherwise, load the configuration and create a new client.
	if c.llmClient == nil {
		env, err := config.LoadEnv()
		if err != nil {
			return fmt.Errorf("failed to load environment: %w", err)
		}
		if len(env.FileLLMs) == 0 {
			return fmt.Errorf("no LLM configurations found in newo.toml")
		}
		llmConfig := env.FileLLMs[0]

		client, err := llm.NewClient(llmConfig)
		if err != nil {
			return fmt.Errorf("failed to create LLM client: %w", err)
		}
		c.llmClient = client
	}

	// 1. Construct a few-shot prompt for direct NSL generation
	llmPrompt := fmt.Sprintf(`You are an expert in the NSL templating language. Your task is to generate NSL code based on the user\'s request.\n\nHere are some examples of how to write NSL:\n\n---\nRequest: "display the value of the \'username\' variable, but in all lowercase"\nNSL Code: {{ username | lower }}\n---\nRequest: "set the \'price\' variable to the value of \'base_price\' plus 10"\nNSL Code: {%% set price = base_price + 10 %%}\n---\nRequest: "if the user is an admin, show \'Admin Panel\'"\nNSL Code: {%% if user.is_admin %%}Admin Panel{%% endif %%}\n---\nRequest: "for each item in the \'products\' list, display its name"\nNSL Code: {%% for item in products %%}{{ item.name }}{%% endfor %%}\n---\n\nNow, generate the NSL code for the following request. Output ONLY the NSL code and nothing else.\n\nRequest: \"%s\"\nNSL Code:`, *c.prompt)

	// 2. Call LLM
	generatedCode, err := c.llmClient.GenerateCode(llmPrompt)
	if err != nil {
		return fmt.Errorf("LLM code generation failed: %w", err)
	}

	// 3. Parse the generated code to validate it and build an AST
	l := lexer.New(generatedCode)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		return fmt.Errorf("failed to parse generated NSL code: %v\n--- Generated Code ---\n%s", p.Errors(), generatedCode)
	}

	// 4. Convert the validated AST back to clean, formatted NSL code
	prettyPrinter := printer.New()
	finalNSL := prettyPrinter.Print(program)

	_, err = fmt.Fprintln(c.stdout, finalNSL)
	return err
}
