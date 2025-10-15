package lexer

import (
	"testing"

	"github.com/twinmind/newo-tool/internal/nsl/token"
)

func TestNextToken(t *testing.T) {
	input := `
        {% set my_var = 10 %}
        {{ my_var + 5 }}

        {% if my_var > 5 %}
            "Hello, World!"
        {% else %}
            'Goodbye'
        {% endif %}

        {% for item in items %}
            {{ item.name }}
        {% endfor %}
    `

	tests := []struct {
		expectedType    token.TokenType
		expectedLiteral string
	}{
		{token.LPERCENT, "{%"},
		{token.SET, "set"},
		{token.IDENT, "my_var"},
		{token.ASSIGN, "="},
		{token.INT, "10"},
		{token.RPERCENT, "%}"},

		{token.LBRACE, "{{"},
		{token.IDENT, "my_var"},
		{token.PLUS, "+"},
		{token.INT, "5"},
		{token.RBRACE, "}}"},

		{token.LPERCENT, "{%"},
		{token.IF, "if"},
		{token.IDENT, "my_var"},
		{token.GT, ">"},
		{token.INT, "5"},
		{token.RPERCENT, "%}"},

		{token.STRING, "Hello, World!"},

		{token.LPERCENT, "{%"},
		{token.ELSE, "else"},
		{token.RPERCENT, "%}"},

		{token.STRING, "Goodbye"},

		{token.LPERCENT, "{%"},
		{token.ENDIF, "endif"},
		{token.RPERCENT, "%}"},

		{token.LPERCENT, "{%"},
		{token.FOR, "for"},
		{token.IDENT, "item"},
		{token.IN, "in"},
		{token.IDENT, "items"},
		{token.RPERCENT, "%}"},

		{token.LBRACE, "{{"},
		{token.IDENT, "item"},
		{token.DOT, "."},
		{token.IDENT, "name"},
		{token.RBRACE, "}}"},

		{token.LPERCENT, "{%"},
		{token.ENDFOR, "endfor"},
		{token.RPERCENT, "%}"},

		{token.EOF, ""},
	}

	l := New(input)

	for i, tt := range tests {
		tok := l.NextToken()

		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q",
				i, tt.expectedType, tok.Type)
		}

		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q",
				i, tt.expectedLiteral, tok.Literal)
		}
	}
}
