package parser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/twinmind/newo-tool/internal/nsl/ast"
	"github.com/twinmind/newo-tool/internal/nsl/lexer"
)

func TestTemplateStatements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		verify func(*testing.T, ast.Statement)
	}{
		{
			name:  "set",
			input: `{% set my_var = 5 %}`,
			verify: func(t *testing.T, stmt ast.Statement) {
				setStmt := requireSetStatement(t, stmt)
				requireIdentifierNode(t, setStmt.Name, "my_var")
				requireIntegerLiteral(t, setStmt.Value, 5)
			},
		},
		{
			name:  "if",
			input: `{% if x > y %} 10 {% endif %}`,
			verify: func(t *testing.T, stmt ast.Statement) {
				ifStmt := requireIfStatement(t, stmt)
				requireInfixExpression(t, ifStmt.Condition, "x", ">", "y")
				requireBlockWithInteger(t, ifStmt.Consequence, 10)
				if len(ifStmt.ElseIfs) != 0 {
					t.Fatalf("expected no elif clauses, got=%d", len(ifStmt.ElseIfs))
				}
				if ifStmt.Alternative != nil {
					t.Fatalf("expected alternative branch to be nil, got=%+v", ifStmt.Alternative)
				}
			},
		},
		{
			name:  "if_else",
			input: `{% if x > y %} 10 {% else %} 20 {% endif %}`,
			verify: func(t *testing.T, stmt ast.Statement) {
				ifStmt := requireIfStatement(t, stmt)
				requireInfixExpression(t, ifStmt.Condition, "x", ">", "y")
				requireBlockWithInteger(t, ifStmt.Consequence, 10)
				if len(ifStmt.ElseIfs) != 0 {
					t.Fatalf("expected no elif clauses, got=%d", len(ifStmt.ElseIfs))
				}
				requireBlockWithInteger(t, ifStmt.Alternative, 20)
			},
		},
		{
			name:  "if_elif_else",
			input: `{% if x > y %} 10 {% elif y > z %} 20 {% else %} 30 {% endif %}`,
			verify: func(t *testing.T, stmt ast.Statement) {
				ifStmt := requireIfStatement(t, stmt)
				requireInfixExpression(t, ifStmt.Condition, "x", ">", "y")
				requireBlockWithInteger(t, ifStmt.Consequence, 10)

				if len(ifStmt.ElseIfs) != 1 {
					t.Fatalf("expected 1 elif clause, got=%d", len(ifStmt.ElseIfs))
				}
				requireElseIfClause(t, ifStmt.ElseIfs[0], func(t *testing.T, clause *ast.ElseIfClause) {
					requireInfixExpression(t, clause.Condition, "y", ">", "z")
					requireBlockWithInteger(t, clause.Consequence, 20)
				})

				requireBlockWithInteger(t, ifStmt.Alternative, 30)
			},
		},
		{
			name:  "for",
			input: `{% for user in users %} {{ user.name | upper }} {% endfor %}`,
			verify: func(t *testing.T, stmt ast.Statement) {
				forStmt := requireForStatement(t, stmt)
				requireIdentifierNode(t, forStmt.Iterator, "user")
				requireIdentifierExpression(t, forStmt.Sequence, "users")

				if forStmt.Body == nil {
					t.Fatalf("for body is nil")
				}

				stmts := requireStatements(t, &ast.Program{Statements: forStmt.Body.Statements}, 1)
				output := requireOutputStatement(t, stmts[0])

				filter := requireFilterExpression(t, output.Expression)
				requireIdentifierNode(t, filter.Filter, "upper")

				attr := requireAttributeAccess(t, filter.Input)
				requireIdentifierExpression(t, attr.Object, "user")
				requireIdentifierNode(t, attr.Attribute, "name")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program := parseProgram(t, tt.input)
			statements := requireStatements(t, program, 1)
			tt.verify(t, statements[0])
		})
	}
}

func TestOutputStatement(t *testing.T) {
	t.Parallel()

	program := parseProgram(t, `{{ answer }}`)
	statements := requireStatements(t, program, 1)

	output := requireOutputStatement(t, statements[0])
	requireIdentifierExpression(t, output.Expression, "answer")
}

func TestOutputStatementWithStringLiteral(t *testing.T) {
	t.Parallel()

	program := parseProgram(t, `{{ "hello" }}`)
	statements := requireStatements(t, program, 1)

	output := requireOutputStatement(t, statements[0])
	requireStringLiteral(t, output.Expression, "hello")
}

func TestExpressionStatement(t *testing.T) {
	t.Parallel()

	program := parseProgram(t, `5 + 5`)
	statements := requireStatements(t, program, 1)

	exprStmt := requireExpressionStatement(t, statements[0])
	requireInfixExpression(t, exprStmt.Expression, 5, "+", 5)
}

func TestPrefixExpressions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		operator string
		value    interface{}
	}{
		{input: `!5`, operator: "!", value: 5},
		{input: `-15`, operator: "-", value: 15},
		{input: `!true`, operator: "!", value: true},
		{input: `!false`, operator: "!", value: false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			program := parseProgram(t, tt.input)
			statements := requireStatements(t, program, 1)

			exprStmt := requireExpressionStatement(t, statements[0])
			requirePrefixExpression(t, exprStmt.Expression, tt.operator, tt.value)
		})
	}
}

func TestInfixExpressions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input      string
		leftValue  interface{}
		operator   string
		rightValue interface{}
	}{
		{input: `5 + 5`, leftValue: 5, operator: "+", rightValue: 5},
		{input: `5 - 5`, leftValue: 5, operator: "-", rightValue: 5},
		{input: `5 * 5`, leftValue: 5, operator: "*", rightValue: 5},
		{input: `5 / 5`, leftValue: 5, operator: "/", rightValue: 5},
		{input: `5 > 5`, leftValue: 5, operator: ">", rightValue: 5},
		{input: `5 < 5`, leftValue: 5, operator: "<", rightValue: 5},
		{input: `5 == 5`, leftValue: 5, operator: "==", rightValue: 5},
		{input: `5 != 5`, leftValue: 5, operator: "!=", rightValue: 5},
		{input: `true == true`, leftValue: true, operator: "==", rightValue: true},
		{input: `true != false`, leftValue: true, operator: "!=", rightValue: false},
		{input: `false == false`, leftValue: false, operator: "==", rightValue: false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			program := parseProgram(t, tt.input)
			statements := requireStatements(t, program, 1)

			exprStmt := requireExpressionStatement(t, statements[0])
			requireInfixExpression(t, exprStmt.Expression, tt.leftValue, tt.operator, tt.rightValue)
		})
	}
}

func TestOperatorPrecedenceParsing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{input: `-a * b`, expected: "((-a) * b)"},
		{input: `!-a`, expected: "(!(-a))"},
		{input: `a + b + c`, expected: "((a + b) + c)"},
		{input: `a + b - c`, expected: "((a + b) - c)"},
		{input: `a * b * c`, expected: "((a * b) * c)"},
		{input: `a * b / c`, expected: "((a * b) / c)"},
		{input: `a + b / c`, expected: "(a + (b / c))"},
		{input: `a + b * c + d / e - f`, expected: "(((a + (b * c)) + (d / e)) - f)"},
		{input: `5 > 4 == 3 < 4`, expected: "((5 > 4) == (3 < 4))"},
		{input: `5 < 4 != 3 > 4`, expected: "((5 < 4) != (3 > 4))"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			program := parseProgram(t, tt.input)
			if actual := program.String(); actual != tt.expected {
				t.Fatalf("expected=%q, got=%q", tt.expected, actual)
			}
		})
	}
}

func TestProgramMultipleStatements(t *testing.T) {
	t.Parallel()

	input := `{% set foo = 1 %}{% if foo %} {{ user.name | upper }} {% elif foo == 2 %} {{ user.email }} {% else %} {{ "fallback" }} {% endif %} {{ foo }}`

	program := parseProgram(t, input)
	statements := requireStatements(t, program, 3)

	setStmt := requireSetStatement(t, statements[0])
	requireIdentifierNode(t, setStmt.Name, "foo")
	requireIntegerLiteral(t, setStmt.Value, 1)

	ifStmt := requireIfStatement(t, statements[1])
	requireIdentifierExpression(t, ifStmt.Condition, "foo")

	requireBlockWithOutput(t, ifStmt.Consequence, func(expr ast.Expression) {
		filter := requireFilterExpression(t, expr)
		requireIdentifierNode(t, filter.Filter, "upper")

		attr := requireAttributeAccess(t, filter.Input)
		requireIdentifierNode(t, attr.Attribute, "name")
		requireIdentifierExpression(t, attr.Object, "user")
	})

	if len(ifStmt.ElseIfs) != 1 {
		t.Fatalf("expected 1 elif clause, got=%d", len(ifStmt.ElseIfs))
	}
	requireElseIfClause(t, ifStmt.ElseIfs[0], func(t *testing.T, clause *ast.ElseIfClause) {
		requireInfixExpression(t, clause.Condition, "foo", "==", 2)
		requireBlockWithOutput(t, clause.Consequence, func(expr ast.Expression) {
			requireAttributeExpression(t, expr, []string{"user", "email"})
		})
	})

	requireBlockWithOutput(t, ifStmt.Alternative, func(expr ast.Expression) {
		requireStringLiteral(t, expr, "fallback")
	})

	output := requireOutputStatement(t, statements[2])
	requireIdentifierExpression(t, output.Expression, "foo")
}

func TestNestedBlocks(t *testing.T) {
	t.Parallel()

	input := `{% for user in users %}{% if user.active %}{{ user.name }}{% endif %}{% endfor %}`

	program := parseProgram(t, input)
	statements := requireStatements(t, program, 1)

	forStmt := requireForStatement(t, statements[0])
	requireIdentifierNode(t, forStmt.Iterator, "user")
	requireIdentifierExpression(t, forStmt.Sequence, "users")

	if len(forStmt.Body.Statements) != 1 {
		t.Fatalf("expected for-body to contain 1 statement, got=%d", len(forStmt.Body.Statements))
	}

	ifStmt := requireIfStatement(t, forStmt.Body.Statements[0])
	requireAttributeExpression(t, ifStmt.Condition, []string{"user", "active"})
	requireBlockWithOutput(t, ifStmt.Consequence, func(expr ast.Expression) {
		requireAttributeExpression(t, expr, []string{"user", "name"})
	})
}

func TestAttributeAccessFilterChain(t *testing.T) {
	t.Parallel()

	input := `{{ user.profile.address.city | upper }}`
	program := parseProgram(t, input)
	statements := requireStatements(t, program, 1)

	output := requireOutputStatement(t, statements[0])
	filter := requireFilterExpression(t, output.Expression)
	requireIdentifierNode(t, filter.Filter, "upper")

	city := requireAttributeAccess(t, filter.Input)
	requireIdentifierNode(t, city.Attribute, "city")

	address := requireAttributeAccess(t, city.Object)
	requireIdentifierNode(t, address.Attribute, "address")

	profile := requireAttributeAccess(t, address.Object)
	requireIdentifierNode(t, profile.Attribute, "profile")

	requireIdentifierExpression(t, profile.Object, "user")
}

func TestParserReportsErrors(t *testing.T) {
	t.Parallel()

	input := `{% if x > y %} 10 `

	l := lexer.New(input)
	p := New(l)
	p.ParseProgram()

	if len(p.Errors()) == 0 {
		t.Fatalf("expected parser to report an error for unterminated if-block")
	}
}

func TestParserRecoveryAfterStatementError(t *testing.T) {
	t.Parallel()

	input := `{% set foo = %} {% if true %} {{ 1 }} {% endif %}`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) == 0 {
		t.Fatalf("expected parser to report an error for invalid set statement")
	}

	foundIf := false
	for _, stmt := range program.Statements {
		if _, ok := stmt.(*ast.IfStatement); ok {
			foundIf = true
			break
		}
	}

	if !foundIf {
		t.Fatalf("expected parser to recover and include an if statement")
	}
}

func TestParserReportsMultipleErrors(t *testing.T) {
	t.Parallel()

	input := `{% if %} {% elif %} {% endif %}`

	l := lexer.New(input)
	p := New(l)
	_ = p.ParseProgram()

	if len(p.Errors()) < 2 {
		t.Fatalf("expected at least 2 parser errors, got=%d", len(p.Errors()))
	}
}

func TestSetStatementMissingValueProducesError(t *testing.T) {
	t.Parallel()

	input := `{% set foo = %}`
	l := lexer.New(input)
	p := New(l)
	p.ParseProgram()

	requireErrorContains(t, p.Errors(), "%}")
}

func TestIfStatementMissingConditionProducesError(t *testing.T) {
	t.Parallel()

	input := `{% if %} {% endif %}`
	l := lexer.New(input)
	p := New(l)
	p.ParseProgram()

	requireErrorContains(t, p.Errors(), "no prefix")
}

func TestIfStatementMissingEndTagProducesError(t *testing.T) {
	t.Parallel()

	input := `{% if true %} {{ 1 }}`
	l := lexer.New(input)
	p := New(l)
	p.ParseProgram()

	requireErrorContains(t, p.Errors(), "ENDIF")
}

func TestForStatementMissingInProducesError(t *testing.T) {
	t.Parallel()

	input := `{% for user users %}{% endfor %}`
	l := lexer.New(input)
	p := New(l)
	p.ParseProgram()

	requireErrorContains(t, p.Errors(), "IN")
}

func TestForStatementMissingSequenceProducesError(t *testing.T) {
	t.Parallel()

	input := `{% for user in %}{% endfor %}`
	l := lexer.New(input)
	p := New(l)
	p.ParseProgram()

	requireErrorContains(t, p.Errors(), "no prefix")
}

func TestOutputStatementMissingExpressionProducesError(t *testing.T) {
	t.Parallel()

	input := `{{ }}`
	l := lexer.New(input)
	p := New(l)
	p.ParseProgram()

	requireErrorContains(t, p.Errors(), "no prefix")
}

func TestMismatchedEndTagProducesError(t *testing.T) {
	t.Parallel()

	input := `{% if true %}{% endfor %}`
	l := lexer.New(input)
	p := New(l)
	p.ParseProgram()

	requireErrorContains(t, p.Errors(), "ENDIF")
}

func TestBlockRecoveryWithinIf(t *testing.T) {
	t.Parallel()

	input := `{% if true %} {{ user. }} {{ user.name }} {% endif %}`
	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	requireErrorContains(t, p.Errors(), "expected next token to be IDENT")

	statements := program.Statements
	if len(statements) != 1 {
		t.Fatalf("expected 1 top-level statement after recovery, got=%d", len(statements))
	}

	ifStmt := requireIfStatement(t, statements[0])
	if ifStmt.Consequence == nil {
		t.Fatalf("expected consequence block after recovery")
	}

	found := false
	for _, stmt := range ifStmt.Consequence.Statements {
		if out, ok := stmt.(*ast.OutputStatement); ok && out != nil {
			if out.Expression != nil {
				found = true
				break
			}
		}
	}

	if !found {
		t.Fatalf("expected to find valid output statement after recovery")
	}
}

func parseProgram(t *testing.T, input string) *ast.Program {
	t.Helper()

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if errs := p.Errors(); len(errs) != 0 {
		t.Fatalf("parser returned %d errors:\n%s", len(errs), strings.Join(errs, "\n"))
	}

	return program
}

func requireStatements(t *testing.T, program *ast.Program, expected int) []ast.Statement {
	t.Helper()

	if len(program.Statements) != expected {
		t.Fatalf("program.Statements expected %d elements, got=%d", expected, len(program.Statements))
	}

	return program.Statements
}

func requireSetStatement(t *testing.T, stmt ast.Statement) *ast.SetStatement {
	t.Helper()

	setStmt, ok := stmt.(*ast.SetStatement)
	if !ok {
		t.Fatalf("statement is not *ast.SetStatement. got=%T", stmt)
	}

	return setStmt
}

func requireIfStatement(t *testing.T, stmt ast.Statement) *ast.IfStatement {
	t.Helper()

	ifStmt, ok := stmt.(*ast.IfStatement)
	if !ok {
		t.Fatalf("statement is not *ast.IfStatement. got=%T", stmt)
	}

	return ifStmt
}

func requireForStatement(t *testing.T, stmt ast.Statement) *ast.ForStatement {
	t.Helper()

	forStmt, ok := stmt.(*ast.ForStatement)
	if !ok {
		t.Fatalf("statement is not *ast.ForStatement. got=%T", stmt)
	}

	return forStmt
}

func requireOutputStatement(t *testing.T, stmt ast.Statement) *ast.OutputStatement {
	t.Helper()

	outputStmt, ok := stmt.(*ast.OutputStatement)
	if !ok {
		t.Fatalf("statement is not *ast.OutputStatement. got=%T", stmt)
	}

	return outputStmt
}

func requireExpressionStatement(t *testing.T, stmt ast.Statement) *ast.ExpressionStatement {
	t.Helper()

	exprStmt, ok := stmt.(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("statement is not *ast.ExpressionStatement. got=%T", stmt)
	}

	return exprStmt
}

func requireBlockWithInteger(t *testing.T, block *ast.BlockStatement, expected int64) {
	t.Helper()

	if block == nil {
		t.Fatalf("expected block statement, got=nil")
	}

	if len(block.Statements) != 1 {
		t.Fatalf("block should contain 1 statement, got=%d", len(block.Statements))
	}

	exprStmt := requireExpressionStatement(t, block.Statements[0])
	requireIntegerLiteral(t, exprStmt.Expression, expected)
}

func requireIdentifierNode(t *testing.T, ident *ast.Identifier, expected string) {
	t.Helper()

	if ident == nil {
		t.Fatalf("identifier is nil, expected %q", expected)
	}

	if ident.Value != expected {
		t.Fatalf("identifier.Value expected %q, got=%q", expected, ident.Value)
	}

	if ident.TokenLiteral() != expected {
		t.Fatalf("identifier.TokenLiteral expected %q, got=%q", expected, ident.TokenLiteral())
	}
}

func requireIntegerLiteral(t *testing.T, expr ast.Expression, value int64) {
	t.Helper()

	integ, ok := expr.(*ast.IntegerLiteral)
	if !ok {
		t.Fatalf("expression is not *ast.IntegerLiteral. got=%T", expr)
	}

	if integ.Value != value {
		t.Fatalf("integer literal value expected %d, got=%d", value, integ.Value)
	}

	if integ.TokenLiteral() != fmt.Sprintf("%d", value) {
		t.Fatalf("integer literal token expected %d, got=%q", value, integ.TokenLiteral())
	}
}

func requireBooleanLiteral(t *testing.T, expr ast.Expression, value bool) {
	t.Helper()

	boolean, ok := expr.(*ast.Boolean)
	if !ok {
		t.Fatalf("expression is not *ast.Boolean. got=%T", expr)
	}

	if boolean.Value != value {
		t.Fatalf("boolean literal value expected %t, got=%t", value, boolean.Value)
	}

	if boolean.TokenLiteral() != fmt.Sprintf("%t", value) {
		t.Fatalf("boolean literal token expected %t, got=%q", value, boolean.TokenLiteral())
	}
}

func requireIdentifierExpression(t *testing.T, expr ast.Expression, value string) {
	t.Helper()

	ident, ok := expr.(*ast.Identifier)
	if !ok {
		t.Fatalf("expression is not *ast.Identifier. got=%T", expr)
	}

	if ident.Value != value {
		t.Fatalf("identifier value expected %q, got=%q", value, ident.Value)
	}

	if ident.TokenLiteral() != value {
		t.Fatalf("identifier token literal expected %q, got=%q", value, ident.TokenLiteral())
	}
}

func requireStringLiteral(t *testing.T, expr ast.Expression, expected string) {
	t.Helper()

	str, ok := expr.(*ast.StringLiteral)
	if !ok {
		t.Fatalf("expression is not *ast.StringLiteral. got=%T", expr)
	}

	if str.Value != expected {
		t.Fatalf("string literal value expected %q, got=%q", expected, str.Value)
	}
}

func requireLiteralExpression(t *testing.T, expr ast.Expression, expected interface{}) {
	t.Helper()

	switch v := expected.(type) {
	case int:
		requireIntegerLiteral(t, expr, int64(v))
	case int64:
		requireIntegerLiteral(t, expr, v)
	case string:
		requireIdentifierExpression(t, expr, v)
	case bool:
		requireBooleanLiteral(t, expr, v)
	default:
		t.Fatalf("literal type %T not supported", expected)
	}
}

func requireInfixExpression(t *testing.T, expr ast.Expression, left interface{}, operator string, right interface{}) {
	t.Helper()

	infix, ok := expr.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expression is not *ast.InfixExpression. got=%T", expr)
	}

	requireLiteralExpression(t, infix.Left, left)

	if infix.Operator != operator {
		t.Fatalf("operator expected %q, got=%q", operator, infix.Operator)
	}

	requireLiteralExpression(t, infix.Right, right)
}

func requirePrefixExpression(t *testing.T, expr ast.Expression, operator string, right interface{}) {
	t.Helper()

	prefix, ok := expr.(*ast.PrefixExpression)
	if !ok {
		t.Fatalf("expression is not *ast.PrefixExpression. got=%T", expr)
	}

	if prefix.Operator != operator {
		t.Fatalf("prefix operator expected %q, got=%q", operator, prefix.Operator)
	}

	requireLiteralExpression(t, prefix.Right, right)
}

func requireFilterExpression(t *testing.T, expr ast.Expression) *ast.FilterExpression {
	t.Helper()

	filter, ok := expr.(*ast.FilterExpression)
	if !ok {
		t.Fatalf("expression is not *ast.FilterExpression. got=%T", expr)
	}

	return filter
}

func requireAttributeAccess(t *testing.T, expr ast.Expression) *ast.AttributeAccess {
	t.Helper()

	attr, ok := expr.(*ast.AttributeAccess)
	if !ok {
		t.Fatalf("expression is not *ast.AttributeAccess. got=%T", expr)
	}

	return attr
}

func requireElseIfClause(t *testing.T, clause *ast.ElseIfClause, verify func(*testing.T, *ast.ElseIfClause)) {
	t.Helper()

	if clause == nil {
		t.Fatalf("expected elif clause to be non-nil")
	}

	verify(t, clause)
}

func requireBlockWithOutput(t *testing.T, block *ast.BlockStatement, check func(ast.Expression)) {
	t.Helper()

	if block == nil {
		t.Fatalf("expected block statement, got=nil")
	}

	if len(block.Statements) != 1 {
		t.Fatalf("block should contain 1 statement, got=%d", len(block.Statements))
	}

	output := requireOutputStatement(t, block.Statements[0])
	check(output.Expression)
}

func requireAttributeExpression(t *testing.T, expr ast.Expression, path []string) {
	t.Helper()

	actual, ok := attributePath(expr)
	if !ok {
		t.Fatalf("expression is not attribute access chain: %T", expr)
	}

	if len(actual) != len(path) {
		t.Fatalf("attribute path length mismatch: expected %v, got %v", path, actual)
	}

	for i := range actual {
		if actual[i] != path[i] {
			t.Fatalf("attribute path expected %v, got %v", path, actual)
		}
	}
}

func attributePath(expr ast.Expression) ([]string, bool) {
	var parts []string
	cur := expr

	for {
		switch node := cur.(type) {
		case *ast.AttributeAccess:
			parts = append(parts, node.Attribute.Value)
			cur = node.Object
		case *ast.Identifier:
			parts = append(parts, node.Value)
			return reverse(parts), true
		default:
			return nil, false
		}
	}
}

func reverse(in []string) []string {
	out := make([]string, len(in))
	for i := range in {
		out[i] = in[len(in)-1-i]
	}
	return out
}

func requireErrorContains(t *testing.T, errs []string, substr string) {
	t.Helper()

	if len(errs) == 0 {
		t.Fatalf("expected parser to report errors containing %q, got none", substr)
	}

	for _, err := range errs {
		if strings.Contains(err, substr) {
			return
		}
	}

	t.Fatalf("expected parser errors to contain %q, got=%v", substr, errs)
}
