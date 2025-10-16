package ast

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// UnmarshalJSON customizes how Program is unmarshaled from JSON.
func (p *Program) UnmarshalJSON(data []byte) error {
	var temp struct {
		Statements []json.RawMessage `json:"Statements"`
	}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	p.Statements = make([]Statement, len(temp.Statements))
	for i, rawStmt := range temp.Statements {
		node, err := unmarshalNode(rawStmt)
		if err != nil {
			return err
		}
		stmt, ok := node.(Statement)
		if !ok {
			return fmt.Errorf("expected statement, got %T", node)
		}
		p.Statements[i] = stmt
	}
	return nil
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

// UnmarshalJSON customizes how PrefixExpression is unmarshaled from JSON.
func (pe *PrefixExpression) UnmarshalJSON(data []byte) error {
	var temp struct {
		Token    json.RawMessage `json:"Token"`
		Operator string          `json:"Operator"`
		Right    json.RawMessage `json:"Right"`
	}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	if err := json.Unmarshal(temp.Token, &pe.Token); err != nil {
		return err
	}
	pe.Operator = temp.Operator

	node, err := unmarshalNode(temp.Right)
	if err != nil {
		return err
	}
	expr, ok := node.(Expression)
	if !ok {
		return fmt.Errorf("expected expression, got %T", node)
	}
	pe.Right = expr
	return nil
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

// UnmarshalJSON customizes how InfixExpression is unmarshaled from JSON.
func (ie *InfixExpression) UnmarshalJSON(data []byte) error {
	var temp struct {
		Token    json.RawMessage `json:"Token"`
		Left     json.RawMessage `json:"Left"`
		Operator string          `json:"Operator"`
		Right    json.RawMessage `json:"Right"`
	}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	if err := json.Unmarshal(temp.Token, &ie.Token); err != nil {
		return err
	}
	ie.Operator = temp.Operator

	node, err := unmarshalNode(temp.Left)
	if err != nil {
		return err
	}
	expr, ok := node.(Expression)
	if !ok {
		return fmt.Errorf("expected expression, got %T", node)
	}
	ie.Left = expr

	node, err = unmarshalNode(temp.Right)
	if err != nil {
		return err
	}
	expr, ok = node.(Expression)
	if !ok {
		return fmt.Errorf("expected expression, got %T", node)
	}
	ie.Right = expr
	return nil
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

// UnmarshalJSON customizes how AttributeAccess is unmarshaled from JSON.
func (aa *AttributeAccess) UnmarshalJSON(data []byte) error {
	var temp struct {
		Token     json.RawMessage `json:"Token"`
		Object    json.RawMessage `json:"Object"`
		Attribute json.RawMessage `json:"Attribute"`
	}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	if err := json.Unmarshal(temp.Token, &aa.Token); err != nil {
		return err
	}

	node, err := unmarshalNode(temp.Object)
	if err != nil {
		return err
	}
	expr, ok := node.(Expression)
	if !ok {
		return fmt.Errorf("expected expression, got %T", node)
	}
	aa.Object = expr

	var attr Identifier
	if err := json.Unmarshal(temp.Attribute, &attr); err != nil {
		return err
	}
	aa.Attribute = &attr
	return nil
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

// UnmarshalJSON customizes how FilterExpression is unmarshaled from JSON.
func (fe *FilterExpression) UnmarshalJSON(data []byte) error {
	var temp struct {
		Token  json.RawMessage `json:"Token"`
		Input  json.RawMessage `json:"Input"`
		Filter json.RawMessage `json:"Filter"`
	}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	if err := json.Unmarshal(temp.Token, &fe.Token); err != nil {
		return err
	}

	node, err := unmarshalNode(temp.Input)
	if err != nil {
		return err
	}
	expr, ok := node.(Expression)
	if !ok {
		return fmt.Errorf("expected expression, got %T", node)
	}
	fe.Input = expr

	var filter Identifier
	if err := json.Unmarshal(temp.Filter, &filter); err != nil {
		return err
	}
	fe.Filter = &filter
	return nil
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

// UnmarshalJSON customizes how ExpressionStatement is unmarshaled from JSON.
func (es *ExpressionStatement) UnmarshalJSON(data []byte) error {
	var temp struct {
		Token      json.RawMessage `json:"Token"`
		Expression json.RawMessage `json:"Expression"`
	}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	if err := json.Unmarshal(temp.Token, &es.Token); err != nil {
		return err
	}

	node, err := unmarshalNode(temp.Expression)
	if err != nil {
		return err
	}
	expr, ok := node.(Expression)
	if !ok {
		return fmt.Errorf("expected expression, got %T", node)
	}
	es.Expression = expr
	return nil
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

// UnmarshalJSON customizes how SetStatement is unmarshaled from JSON.
func (ss *SetStatement) UnmarshalJSON(data []byte) error {
	var temp struct {
		Token json.RawMessage `json:"Token"`
		Name  json.RawMessage `json:"Name"`
		Value json.RawMessage `json:"Value"`
	}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	// Unmarshal Token
	if err := json.Unmarshal(temp.Token, &ss.Token); err != nil {
		return err
	}

	// Unmarshal Name (Identifier)
	var name Identifier
	if err := json.Unmarshal(temp.Name, &name); err != nil {
		return err
	}
	ss.Name = &name

	// Unmarshal Value (Expression)
	node, err := unmarshalNode(temp.Value)
	if err != nil {
		return err
	}
	expr, ok := node.(Expression)
	if !ok {
		return fmt.Errorf("expected expression, got %T", node)
	}
	ss.Value = expr
	return nil
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

// UnmarshalJSON customizes how OutputStatement is unmarshaled from JSON.
func (os *OutputStatement) UnmarshalJSON(data []byte) error {
	var temp struct {
		Token      json.RawMessage `json:"Token"`
		Expression json.RawMessage `json:"Expression"`
	}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	if err := json.Unmarshal(temp.Token, &os.Token); err != nil {
		return err
	}

	node, err := unmarshalNode(temp.Expression)
	if err != nil {
		return err
	}
	expr, ok := node.(Expression)
	if !ok {
		return fmt.Errorf("expected expression, got %T", node)
	}
	os.Expression = expr
	return nil
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

// UnmarshalJSON customizes how BlockStatement is unmarshaled from JSON.
func (bs *BlockStatement) UnmarshalJSON(data []byte) error {
	var temp struct {
		Token      json.RawMessage `json:"Token"`
		Statements []json.RawMessage `json:"Statements"`
	}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	if err := json.Unmarshal(temp.Token, &bs.Token); err != nil {
		return err
	}

	bs.Statements = make([]Statement, len(temp.Statements))
	for i, rawStmt := range temp.Statements {
		node, err := unmarshalNode(rawStmt)
		if err != nil {
			return err
		}
		stmt, ok := node.(Statement)
		if !ok {
			return fmt.Errorf("expected statement, got %T", node)
		}
		bs.Statements[i] = stmt
	}
	return nil
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

// UnmarshalJSON customizes how IfStatement is unmarshaled from JSON.
func (is *IfStatement) UnmarshalJSON(data []byte) error {
	var temp struct {
		Token       json.RawMessage `json:"Token"`
		Condition   json.RawMessage `json:"Condition"`
		Consequence json.RawMessage `json:"Consequence"`
		ElseIfs     []json.RawMessage `json:"ElseIfs"`
		Alternative json.RawMessage `json:"Alternative"`
	}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	if err := json.Unmarshal(temp.Token, &is.Token); err != nil {
		return err
	}

	node, err := unmarshalNode(temp.Condition)
	if err != nil {
		return err
	}
	expr, ok := node.(Expression)
	if !ok {
		return fmt.Errorf("expected expression, got %T", node)
	}
	is.Condition = expr

	var consequence BlockStatement
	if err := json.Unmarshal(temp.Consequence, &consequence); err != nil {
		return err
	}
	is.Consequence = &consequence

	is.ElseIfs = make([]*ElseIfClause, len(temp.ElseIfs))
	for i, rawElseIf := range temp.ElseIfs {
		var elif ElseIfClause
		if err := json.Unmarshal(rawElseIf, &elif); err != nil {
			return err
		}
		is.ElseIfs[i] = &elif
	}

	if len(temp.Alternative) > 0 {
		var alternative BlockStatement
		if err := json.Unmarshal(temp.Alternative, &alternative); err != nil {
			return err
		}
		is.Alternative = &alternative
	}
	return nil
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

// UnmarshalJSON customizes how ElseIfClause is unmarshaled from JSON.
func (eic *ElseIfClause) UnmarshalJSON(data []byte) error {
	var temp struct {
		Token       json.RawMessage `json:"Token"`
		Condition   json.RawMessage `json:"Condition"`
		Consequence json.RawMessage `json:"Consequence"`
	}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	if err := json.Unmarshal(temp.Token, &eic.Token); err != nil {
		return err
	}

	node, err := unmarshalNode(temp.Condition)
	if err != nil {
		return err
	}
	expr, ok := node.(Expression)
	if !ok {
		return fmt.Errorf("expected expression, got %T", node)
	}
	eic.Condition = expr

	var consequence BlockStatement
	if err := json.Unmarshal(temp.Consequence, &consequence); err != nil {
		return err
	}
	eic.Consequence = &consequence
	return nil
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

// UnmarshalJSON customizes how ForStatement is unmarshaled from JSON.
func (fs *ForStatement) UnmarshalJSON(data []byte) error {
	var temp struct {
		Token    json.RawMessage `json:"Token"`
		Iterator json.RawMessage `json:"Iterator"`
		Sequence json.RawMessage `json:"Sequence"`
		Body     json.RawMessage `json:"Body"`
	}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	if err := json.Unmarshal(temp.Token, &fs.Token); err != nil {
		return err
	}

	var iterator Identifier
	if err := json.Unmarshal(temp.Iterator, &iterator); err != nil {
		return err
	}
	fs.Iterator = &iterator

	node, err := unmarshalNode(temp.Sequence)
	if err != nil {
		return err
	}
	expr, ok := node.(Expression)
	if !ok {
		return fmt.Errorf("expected expression, got %T", node)
	}
	fs.Sequence = expr

	var body BlockStatement
	if err := json.Unmarshal(temp.Body, &body); err != nil {
		return err
	}
	fs.Body = &body
	return nil
}

// unmarshalNode is a helper function to unmarshal raw JSON into the correct ast.Node type.
func unmarshalNode(raw json.RawMessage) (Node, error) {
	var nw struct {
		Type string `json:"_type"`
	}
	if err := json.Unmarshal(raw, &nw); err != nil {
		return nil, err
	}

	switch nw.Type {
	// Statements
	case "Program": // Program is the root, not a child node to be unmarshaled this way
		return nil, fmt.Errorf("program should not be unmarshaled via unmarshalNode")
	case "SetStatement":
		var stmt SetStatement
		if err := json.Unmarshal(raw, &stmt); err != nil {
			return nil, err
		}
		return &stmt, nil
	case "OutputStatement":
		var stmt OutputStatement
		if err := json.Unmarshal(raw, &stmt); err != nil {
			return nil, err
		}
		return &stmt, nil
	case "ExpressionStatement":
		var stmt ExpressionStatement
		if err := json.Unmarshal(raw, &stmt); err != nil {
			return nil, err
		}
		return &stmt, nil
	case "IfStatement":
		var stmt IfStatement
		if err := json.Unmarshal(raw, &stmt); err != nil {
			return nil, err
		}
		return &stmt, nil
	case "ForStatement":
		var stmt ForStatement
		if err := json.Unmarshal(raw, &stmt); err != nil {
			return nil, err
		}
		return &stmt, nil
	case "BlockStatement":
		var stmt BlockStatement
		if err := json.Unmarshal(raw, &stmt); err != nil {
			return nil, err
		}
		return &stmt, nil
	// Expressions
	case "Identifier":
		var expr Identifier
		if err := json.Unmarshal(raw, &expr); err != nil {
			return nil, err
		}
		return &expr, nil
	case "IntegerLiteral":
		var expr IntegerLiteral
		if err := json.Unmarshal(raw, &expr); err != nil {
			return nil, err
		}
		return &expr, nil
	case "StringLiteral":
		var expr StringLiteral
		if err := json.Unmarshal(raw, &expr); err != nil {
			return nil, err
		}
		return &expr, nil
	case "Boolean":
		var expr Boolean
		if err := json.Unmarshal(raw, &expr); err != nil {
			return nil, err
		}
		return &expr, nil
	case "InfixExpression":
		var expr InfixExpression
		if err := json.Unmarshal(raw, &expr); err != nil {
			return nil, err
		}
		return &expr, nil
	case "PrefixExpression":
		var expr PrefixExpression
		if err := json.Unmarshal(raw, &expr); err != nil {
			return nil, err
		}
		return &expr, nil
	case "AttributeAccess":
		var expr AttributeAccess
		if err := json.Unmarshal(raw, &expr); err != nil {
			return nil, err
		}
		return &expr, nil
	case "FilterExpression":
		var expr FilterExpression
		if err := json.Unmarshal(raw, &expr); err != nil {
			return nil, err
		}
		return &expr, nil
	case "Token": // Token is a struct, not an interface, so it should be unmarshaled directly
		return nil, fmt.Errorf("token should not be unmarshaled via unmarshalNode")
	default:
		return nil, fmt.Errorf("unknown AST node type: %s", nw.Type)
	}
}