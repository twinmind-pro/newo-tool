package ast

import (
	"bytes"
	"github.com/twinmind/newo-tool/internal/nsl/token"
)

// Node is the base interface for every node in our AST.
type Node interface {
	TokenLiteral() string // Used for debugging and testing
	String() string
}

// Statement nodes execute actions but do not produce values.
type Statement interface {
	Node
	statementNode()
}

// Expression nodes produce values.
type Expression interface {
	Node
	expressionNode()
}

// Program is the root node of every AST our parser produces.
type Program struct {
	Statements []Statement
}

func (p *Program) TokenLiteral() string {
	if len(p.Statements) > 0 {
		return p.Statements[0].TokenLiteral()
	}
	return ""
}

func (p *Program) String() string {
	var out bytes.Buffer
	for _, s := range p.Statements {
		out.WriteString(s.String())
	}
	return out.String()
}

// --- Expressions ---

// Identifier represents a variable or function name.
type Identifier struct {
	Token token.Token // the token.IDENT token
	Value string
}

func (i *Identifier) expressionNode()      {}
func (i *Identifier) TokenLiteral() string { return i.Token.Literal }
func (i *Identifier) String() string       { return i.Value }

// Literals
type IntegerLiteral struct {
	Token token.Token
	Value int64
}

func (il *IntegerLiteral) expressionNode()      {}
func (il *IntegerLiteral) TokenLiteral() string { return il.Token.Literal }
func (il *IntegerLiteral) String() string       { return il.Token.Literal }

type StringLiteral struct {
	Token token.Token
	Value string
}

func (sl *StringLiteral) expressionNode()      {}
func (sl *StringLiteral) TokenLiteral() string { return sl.Token.Literal }
func (sl *StringLiteral) String() string       { return sl.Token.Literal }

type Boolean struct {
	Token token.Token
	Value bool
}

func (b *Boolean) expressionNode()      {}
func (b *Boolean) TokenLiteral() string { return b.Token.Literal }
func (b *Boolean) String() string       { return b.Token.Literal }

// Complex Expressions
type PrefixExpression struct {
	Token    token.Token // The prefix token, e.g. !
	Operator string
	Right    Expression
}

func (pe *PrefixExpression) expressionNode()      {}
func (pe *PrefixExpression) TokenLiteral() string { return pe.Token.Literal }
func (pe *PrefixExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	out.WriteString(pe.Operator)
	out.WriteString(pe.Right.String())
	out.WriteString(")")
	return out.String()
}

type InfixExpression struct {
	Token    token.Token // The operator token, e.g. +
	Left     Expression
	Operator string
	Right    Expression
}

func (ie *InfixExpression) expressionNode()      {}
func (ie *InfixExpression) TokenLiteral() string { return ie.Token.Literal }
func (ie *InfixExpression) String() string {
	var out bytes.Buffer
	out.WriteString("(")
	out.WriteString(ie.Left.String())
	out.WriteString(" " + ie.Operator + " ")
	out.WriteString(ie.Right.String())
	out.WriteString(")")
	return out.String()
}

type AttributeAccess struct {
	Token     token.Token // The . token
	Object    Expression
	Attribute *Identifier
}

func (aa *AttributeAccess) expressionNode()      {}
func (aa *AttributeAccess) TokenLiteral() string { return aa.Token.Literal }
func (aa *AttributeAccess) String() string {
	return aa.Object.String() + "." + aa.Attribute.String()
}

type FilterExpression struct {
	Token  token.Token // The | token
	Input  Expression
	Filter *Identifier
}

func (fe *FilterExpression) expressionNode()      {}
func (fe *FilterExpression) TokenLiteral() string { return fe.Token.Literal }
func (fe *FilterExpression) String() string {
	return fe.Input.String() + " | " + fe.Filter.String()
}

// --- Statements ---

// ExpressionStatement is a statement that consists of a single expression.
type ExpressionStatement struct {
	Token      token.Token // the first token of the expression
	Expression Expression
}

func (es *ExpressionStatement) statementNode()       {}
func (es *ExpressionStatement) TokenLiteral() string { return es.Token.Literal }
func (es *ExpressionStatement) String() string {
	if es.Expression != nil {
		return es.Expression.String()
	}
	return ""
}

// SetStatement represents a `{% set my_var = ... %}` statement.
type SetStatement struct {
	Token token.Token // the {% token
	Name  *Identifier
	Value Expression
}

func (ss *SetStatement) statementNode()       {}
func (ss *SetStatement) TokenLiteral() string { return ss.Token.Literal }
func (ss *SetStatement) String() string {
	var out bytes.Buffer
	out.WriteString(ss.TokenLiteral() + " ")
	out.WriteString(ss.Name.String())
	out.WriteString(" = ")
	if ss.Value != nil {
		out.WriteString(ss.Value.String())
	}
	return out.String()
}

// OutputStatement represents a `{{ ... }}` block.
type OutputStatement struct {
	Token      token.Token // the {{ token
	Expression Expression
}

func (os *OutputStatement) statementNode()       {}
func (os *OutputStatement) TokenLiteral() string { return os.Token.Literal }
func (os *OutputStatement) String() string {
	return "{{" + os.Expression.String() + "}}"
}

// BlockStatement is a sequence of statements.
type BlockStatement struct {
	Token      token.Token // the {% or {{ token that starts the block
	Statements []Statement
}

func (bs *BlockStatement) statementNode()       {}
func (bs *BlockStatement) TokenLiteral() string { return bs.Token.Literal }
func (bs *BlockStatement) String() string {
	var out bytes.Buffer
	for _, s := range bs.Statements {
		out.WriteString(s.String())
	}
	return out.String()
}

// IfStatement represents an `if-else` expression.
type IfStatement struct {
	Token       token.Token // The {% token
	Condition   Expression
	Consequence *BlockStatement
	ElseIfs     []*ElseIfClause
	Alternative *BlockStatement // Can be nil
}

func (is *IfStatement) statementNode()       {}
func (is *IfStatement) TokenLiteral() string { return is.Token.Literal }
func (is *IfStatement) String() string {
	var out bytes.Buffer
	out.WriteString("if " + is.Condition.String() + " " + is.Consequence.String())
	for _, elif := range is.ElseIfs {
		out.WriteString("elif " + elif.Condition.String() + " " + elif.Consequence.String())
	}
	if is.Alternative != nil {
		out.WriteString("else " + is.Alternative.String())
	}
	return out.String()
}

type ElseIfClause struct {
	Token       token.Token
	Condition   Expression
	Consequence *BlockStatement
}

func (eic *ElseIfClause) String() string {
	var out bytes.Buffer
	out.WriteString(eic.Condition.String() + " ")
	out.WriteString(eic.Consequence.String())
	return out.String()
}

// ForStatement represents a `for` loop.
type ForStatement struct {
	Token    token.Token // The {% token
	Iterator *Identifier
	Sequence Expression
	Body     *BlockStatement
}

func (fs *ForStatement) statementNode()       {}
func (fs *ForStatement) TokenLiteral() string { return fs.Token.Literal }
func (fs *ForStatement) String() string {
	var out bytes.Buffer
	out.WriteString("for " + fs.Iterator.String() + " in " + fs.Sequence.String() + " ")
	out.WriteString(fs.Body.String())
	return out.String()
}
