package ast

import (
	"testing"

	"github.com/twinmind/newo-tool/internal/nsl/token"
)

// TestString confirms that the AST nodes can be instantiated and produce a string representation.
func TestString(t *testing.T) {
	program := &Program{
		Statements: []Statement{
			&SetStatement{
				Token: token.Token{Type: token.SET, Literal: "set"},
				Name: &Identifier{
					Token: token.Token{Type: token.IDENT, Literal: "my_var"},
					Value: "my_var",
				},
				Value: &InfixExpression{
					Token:    token.Token{Type: token.PLUS, Literal: "+"},
					Left:     &IntegerLiteral{Token: token.Token{Type: token.INT, Literal: "5"}, Value: 5},
					Operator: "+",
					Right:    &IntegerLiteral{Token: token.Token{Type: token.INT, Literal: "5"}, Value: 5},
				},
			},
		},
	}

	expected := "set my_var = (5 + 5)"

	if program.String() != expected {
		t.Errorf("program.String() wrong. expected=%q, got=%q", expected, program.String())
	}
}
