package token

// TokenType is a string representing the type of a token.
type TokenType string

// Token represents a single token in the source code.
type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Column  int
}

const (
	// Special tokens
	ILLEGAL = "ILLEGAL" // An unknown token
	EOF     = "EOF"     // End of file

	// Identifiers & Literals
	IDENT  = "IDENT"  // my_variable, user
	INT    = "INT"    // 12345
	STRING = "STRING" // "hello world"

	// Delimiters
	LBRACE   = "{{" // Left brace for output
	RBRACE   = "}}" // Right brace for output
	LPERCENT = "{%" // Left percent for statements
	RPERCENT = "%}" // Right percent for statements

	// Operators
	ASSIGN   = "="
	PLUS     = "+"
	MINUS    = "-"
	BANG     = "!"
	ASTERISK = "*"
	SLASH    = "/"
	DOT      = "."
	PIPE     = "|"

	LT = "<"
	GT = ">"

	EQ     = "=="
	NOT_EQ = "!="
	LTE    = "<="
	GTE    = ">="

	// Keywords
	TRUE     = "TRUE"
	FALSE    = "FALSE"
	NULL     = "NULL"
	IF       = "IF"
	ELSE     = "ELSE"
	ELIF     = "ELIF"
	ENDIF    = "ENDIF"
	FOR      = "FOR"
	IN       = "IN"
	ENDFOR   = "ENDFOR"
	SET      = "SET"
	BLOCK    = "BLOCK"
	ENDBLOCK = "ENDBLOCK"
)

var keywords = map[string]TokenType{
	"true":     TRUE,
	"false":    FALSE,
	"null":     NULL,
	"if":       IF,
	"else":     ELSE,
	"elif":     ELIF,
	"endif":    ENDIF,
	"for":      FOR,
	"in":       IN,
	"endfor":   ENDFOR,
	"set":      SET,
	"block":    BLOCK,
	"endblock": ENDBLOCK,
}

// LookupIdent checks the keywords table to see whether the given identifier is a keyword.
func LookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return IDENT
}
