package printer

import (
	"strings"
	"testing"

	"github.com/twinmind/newo-tool/internal/nsl/ast"
	"github.com/twinmind/newo-tool/internal/nsl/lexer"
	"github.com/twinmind/newo-tool/internal/nsl/parser"
)

func TestPrintSetStatement(t *testing.T) {
	input := `{% set myVar = 123 %}`
	expected := `{% set myVar = 123 %}` + "\n"

	program := parseInput(t, input)
	p := New()
	output := p.Print(program)

	if output != expected {
		t.Errorf("expected %q, got %q", expected, output)
	}
}

func TestPrintOutputStatement(t *testing.T) {
	input := `{{ myVar }}`
	expected := `{{ myVar }}` + "\n"

	program := parseInput(t, input)
	p := New()
	output := p.Print(program)

	if output != expected {
		t.Errorf("expected %q, got %q", expected, output)
	}
}

func TestPrintIfStatement(t *testing.T) {
	input := `{% if condition %}
    {{ trueBlock }}
{% else %}
    {{ falseBlock }}
{% endif %}`
	expected := `{% if condition %}
    {{ trueBlock }}
{% else %}
    {{ falseBlock }}
{% endif %}` + "\n"

	program := parseInput(t, input)
	p := New()
	output := p.Print(program)

	if output != expected {
		t.Errorf("expected %q, got %q", expected, output)
	}
}

func TestPrintForStatement(t *testing.T) {
	input := `{% for item in items %}
    {{ item }}
{% endfor %}`
	expected := `{% for item in items %}
    {{ item }}
{% endfor %}` + "\n"

	program := parseInput(t, input)
	p := New()
	output := p.Print(program)

	if output != expected {
		t.Errorf("expected %q, got %q", expected, output)
	}
}

func TestPrintComplexProgram(t *testing.T) {
	input := `{% set name = 'World' %}
{% if user.isAdmin %}
    {{ "Admin user" }}
{% elif user.isEditor %}
    {{ "Editor user" }}
{% else %}
    {{ "Regular user" }}
{% endif %}
{% for item in items | reverse %}
    {{ item.name }}
{% endfor %}
{{ greeting | upper }}`
	expected := `{% set name = "World" %}
{% if user.isAdmin %}
    {{ "Admin user" }}
{% elif user.isEditor %}
    {{ "Editor user" }}
{% else %}
    {{ "Regular user" }}
{% endif %}
{% for item in items | reverse %}
    {{ item.name }}
{% endfor %}
{{ greeting | upper }}` + "\n"

	program := parseInput(t, input)
	p := New()
	output := p.Print(program)

	if output != expected {
		t.Errorf("expected %q, got %q", expected, output)
	}
}

func parseInput(t *testing.T, input string) *ast.Program {
	l := lexer.New(input)
	p := parser.New(l)
	program := p.ParseProgram()

	if len(p.Errors()) != 0 {
		t.Fatalf("parser errors: %s", strings.Join(p.Errors(), "; "))
	}
	return program
}
