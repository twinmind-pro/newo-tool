package printer

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/twinmind/newo-tool/internal/nsl/ast"
)

// Printer converts an AST program back into a formatted NSL string.
type Printer struct {
	buffer            *bytes.Buffer
	indentationLevel  int
	indentationString string
}

// New creates a new Printer with default settings.
func New() *Printer {
	return &Printer{
		buffer:            new(bytes.Buffer),
		indentationLevel:  0,
		indentationString: "    ", // 4 spaces
	}
}

// Print takes an AST program and returns its formatted string representation.
func (p *Printer) Print(program *ast.Program) string {
	p.printProgram(program)
	return p.buffer.String()
}

func (p *Printer) printProgram(program *ast.Program) {
	for _, stmt := range program.Statements {
		p.printStatement(stmt)
	}
}

func (p *Printer) printStatement(stmt ast.Statement) {
	// Indentation is handled by writeNewline before printing the next statement
	// or explicitly by printBlockStatement for its children.
	switch s := stmt.(type) {
	case *ast.SetStatement:
		p.writeIndent()
		p.printSetStatement(s)
	case *ast.OutputStatement:
		p.writeIndent()
		p.printOutputStatement(s)
	case *ast.IfStatement:
		p.writeIndent()
		p.printIfStatement(s)
	case *ast.ForStatement:
		p.writeIndent()
		p.printForStatement(s)
	case *ast.ExpressionStatement:
		p.writeIndent()
		p.printExpressionStatement(s)
	case *ast.BlockStatement:
		// BlockStatement handles its own indentation for its children
		p.printBlockStatement(s)
	default:
		p.writeIndent()
		p.writeString(fmt.Sprintf("/* UNKNOWN STATEMENT: %T */", s))
	}
	p.writeNewline() // Ensure each statement ends with a newline
}

func (p *Printer) printSetStatement(stmt *ast.SetStatement) {
	p.writeString("{% set ")
	p.printExpression(stmt.Name)
	p.writeString(" = ")
	p.printExpression(stmt.Value)
	p.writeString(" %}")
}

func (p *Printer) printOutputStatement(stmt *ast.OutputStatement) {
	p.writeString("{{ ")
	p.printExpression(stmt.Expression)
	p.writeString(" }}")
}

func (p *Printer) printIfStatement(stmt *ast.IfStatement) {
	p.writeString("{% if ")
	p.printExpression(stmt.Condition)
	p.writeString(" %}")
	p.writeNewline() // Newline after {% if ... %}
	p.indent()
	p.printBlockStatement(stmt.Consequence)
	p.dedent()

	for _, clause := range stmt.ElseIfs {
		p.writeIndent()
		p.writeString("{% elif ")
		p.printExpression(clause.Condition)
		p.writeString(" %}")
		p.writeNewline() // Newline after {% elif ... %}
		p.indent()
		p.printBlockStatement(clause.Consequence)
		p.dedent()
	}

	if stmt.Alternative != nil {
		p.writeIndent()
		p.writeString("{% else %}")
		p.writeNewline() // Newline after {% else %}
		p.indent()
		p.printBlockStatement(stmt.Alternative)
		p.dedent()
	}
	p.writeIndent()
	p.writeString("{% endif %}")
}

func (p *Printer) printForStatement(stmt *ast.ForStatement) {
	p.writeString("{% for ")
	p.printExpression(stmt.Iterator)
	p.writeString(" in ")
	p.printExpression(stmt.Sequence)
	p.writeString(" %}")
	p.writeNewline() // Newline after {% for ... %}
	p.indent()
	p.printBlockStatement(stmt.Body)
	p.dedent()
	p.writeIndent()
	p.writeString("{% endfor %}")
}

func (p *Printer) printExpressionStatement(stmt *ast.ExpressionStatement) {
	p.printExpression(stmt.Expression)
}

func (p *Printer) printBlockStatement(block *ast.BlockStatement) {
	for _, stmt := range block.Statements {
		p.printStatement(stmt)
	}
}

func (p *Printer) printExpression(expr ast.Expression) {
	switch e := expr.(type) {
	case *ast.Identifier:
		p.writeString(e.Value)
	case *ast.IntegerLiteral:
		p.writeString(fmt.Sprintf("%d", e.Value))
	case *ast.StringLiteral:
		p.writeString(fmt.Sprintf("\"%s\"", e.Value)) // Use double quotes for consistency
	case *ast.Boolean:
		p.writeString(fmt.Sprintf("%t", e.Value))
	case *ast.InfixExpression:
		p.printExpression(e.Left)
		p.writeString(" " + e.Operator + " ")
		p.printExpression(e.Right)
	case *ast.PrefixExpression:
		p.writeString(e.Operator)
		p.printExpression(e.Right)
	case *ast.AttributeAccess:
		p.printExpression(e.Object)
		p.writeString(".")
		p.writeString(e.Attribute.Value)
	case *ast.FilterExpression:
		p.printExpression(e.Input)
		p.writeString(" | ")
		p.writeString(e.Filter.Value)
	default:
		p.writeString(fmt.Sprintf("/* UNKNOWN EXPRESSION: %T */", e))
	}
}

func (p *Printer) indent() {
	p.indentationLevel++
}

func (p *Printer) dedent() {
	p.indentationLevel--
	if p.indentationLevel < 0 {
		p.indentationLevel = 0 // Prevent negative indentation
	}
}

func (p *Printer) writeIndent() {
	p.buffer.WriteString(strings.Repeat(p.indentationString, p.indentationLevel))
}

func (p *Printer) writeString(s string) {
	p.buffer.WriteString(s)
}

func (p *Printer) writeNewline() {
	p.buffer.WriteByte('\n')
}