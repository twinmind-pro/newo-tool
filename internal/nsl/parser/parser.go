package parser

import (
	"fmt"
	"strconv"

	"github.com/twinmind/newo-tool/internal/nsl/ast"
	"github.com/twinmind/newo-tool/internal/nsl/lexer"
	"github.com/twinmind/newo-tool/internal/nsl/token"
)

const (
	_ int = iota
	LOWEST
	EQUALS
	LESSGREATER
	SUM
	PRODUCT
	FILTER
	ATTRIBUTE
	PREFIX
	CALL
	INDEX
)

var precedences = map[token.TokenType]int{
	token.EQ:       EQUALS,
	token.NOT_EQ:   EQUALS,
	token.LT:       LESSGREATER,
	token.GT:       LESSGREATER,
	token.PLUS:     SUM,
	token.MINUS:    SUM,
	token.SLASH:    PRODUCT,
	token.ASTERISK: PRODUCT,
	token.PIPE:     FILTER,
	token.DOT:      ATTRIBUTE,
}

type (
	prefixParseFn func() ast.Expression
	infixParseFn  func(ast.Expression) ast.Expression
)

type Parser struct {
	l      *lexer.Lexer
	errors []string

	curToken  token.Token
	peekToken token.Token

	prefixParseFns map[token.TokenType]prefixParseFn
	infixParseFns  map[token.TokenType]infixParseFn
}

func New(l *lexer.Lexer) *Parser {
	p := &Parser{l: l, errors: []string{}}

	p.prefixParseFns = make(map[token.TokenType]prefixParseFn)
	p.registerPrefix(token.IDENT, p.parseIdentifier)
	p.registerPrefix(token.INT, p.parseIntegerLiteral)
	p.registerPrefix(token.TRUE, p.parseBoolean)
	p.registerPrefix(token.FALSE, p.parseBoolean)
	p.registerPrefix(token.BANG, p.parsePrefixExpression)
	p.registerPrefix(token.MINUS, p.parsePrefixExpression)
	p.registerPrefix(token.STRING, p.parseStringLiteral)

	p.infixParseFns = make(map[token.TokenType]infixParseFn)
	p.registerInfix(token.PLUS, p.parseInfixExpression)
	p.registerInfix(token.MINUS, p.parseInfixExpression)
	p.registerInfix(token.SLASH, p.parseInfixExpression)
	p.registerInfix(token.ASTERISK, p.parseInfixExpression)
	p.registerInfix(token.EQ, p.parseInfixExpression)
	p.registerInfix(token.NOT_EQ, p.parseInfixExpression)
	p.registerInfix(token.LT, p.parseInfixExpression)
	p.registerInfix(token.GT, p.parseInfixExpression)
	p.registerInfix(token.DOT, p.parseAttributeAccess)
	p.registerInfix(token.PIPE, p.parseFilterExpression)

	p.nextToken()
	p.nextToken()

	return p
}

func (p *Parser) Errors() []string {
	return p.errors
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.NextToken()
}

func (p *Parser) ParseProgram() *ast.Program {
	program := &ast.Program{}
	program.Statements = []ast.Statement{}

	for !p.curTokenIs(token.EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			program.Statements = append(program.Statements, stmt)
		} else {
			p.synchronize()
		}
		p.nextToken()
	}

	return program
}

func (p *Parser) parseStatement() ast.Statement {
	switch p.curToken.Type {
	case token.LPERCENT:
		return p.parseTemplateStatement()
	case token.LBRACE:
		return p.parseOutputStatement()
	default:
		return p.parseExpressionStatement()
	}
}

func (p *Parser) parseTemplateStatement() ast.Statement {
	switch p.peekToken.Type {
	case token.SET:
		p.nextToken()
		return p.parseSetStatement()
	case token.IF:
		p.nextToken()
		return p.parseIfStatement()
	case token.FOR:
		p.nextToken()
		return p.parseForStatement()
	default:
		msg := fmt.Sprintf("unexpected template tag %q", p.peekToken.Literal)
		p.errors = append(p.errors, msg)
		return nil
	}
}

func (p *Parser) parseSetStatement() *ast.SetStatement {
	stmt := &ast.SetStatement{Token: p.curToken}

	if !p.expectPeek(token.IDENT) {
		return nil
	}

	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if !p.expectPeek(token.ASSIGN) {
		return nil
	}

	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)
	if stmt.Value == nil {
		return nil
	}

	if !p.peekTokenIs(token.RPERCENT) {
		return nil
	}
	p.nextToken()

	return stmt
}

func (p *Parser) parseIfStatement() *ast.IfStatement {
	stmt := &ast.IfStatement{Token: p.curToken} // curToken is 'if'
	stmt.ElseIfs = []*ast.ElseIfClause{}

	p.nextToken() // consume 'if'
	stmt.Condition = p.parseExpression(LOWEST)
	if stmt.Condition == nil {
		return nil
	}

	if !p.expectPeek(token.RPERCENT) {
		return nil
	}

	stmt.Consequence = p.parseBlockStatement()

	// After parseBlockStatement, curToken is on the tag that terminated it (e.g., {% of else/elif/endif)
	for p.curTokenIs(token.LPERCENT) {
		switch p.peekToken.Type {
		case token.ELIF:
			clause := &ast.ElseIfClause{}
			p.nextToken() // move to ELIF
			clause.Token = p.curToken

			p.nextToken()
			clause.Condition = p.parseExpression(LOWEST)
			if clause.Condition == nil {
				return nil
			}

			if !p.expectPeek(token.RPERCENT) {
				return nil
			}

			clause.Consequence = p.parseBlockStatement()
			stmt.ElseIfs = append(stmt.ElseIfs, clause)
		case token.ELSE:
			p.nextToken() // move to ELSE

			if !p.expectPeek(token.RPERCENT) {
				return nil
			}
			stmt.Alternative = p.parseBlockStatement()
		default:
			// Not an elif/else tag, break out to consume endif
			goto endifCheck
		}
	}

	// Expect the endif tag at the end
endifCheck:
	if !p.curTokenIs(token.LPERCENT) || !p.peekTokenIs(token.ENDIF) {
		// This error is tricky. The block parsing might have just ended.
		// Let's advance and see.
		p.peekError(token.ENDIF)
		return nil
	}
	p.nextToken() // move to ENDIF
	if !p.expectPeek(token.RPERCENT) {
		return nil
	}

	return stmt
}

func (p *Parser) parseForStatement() *ast.ForStatement {
	stmt := &ast.ForStatement{Token: p.curToken} // curToken is 'for'

	if !p.expectPeek(token.IDENT) {
		return nil
	}
	stmt.Iterator = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if !p.expectPeek(token.IN) {
		return nil
	}

	p.nextToken()
	stmt.Sequence = p.parseExpression(LOWEST)
	if stmt.Sequence == nil {
		return nil
	}

	if !p.expectPeek(token.RPERCENT) {
		return nil
	}

	stmt.Body = p.parseBlockStatement()

	if !p.curTokenIs(token.LPERCENT) || !p.peekTokenIs(token.ENDFOR) {
		p.peekError(token.ENDFOR)
		return nil
	}

	p.nextToken() // move to ENDFOR

	if !p.expectPeek(token.RPERCENT) {
		return nil
	}

	return stmt
}

func (p *Parser) parseBlockStatement() *ast.BlockStatement {
	block := &ast.BlockStatement{}
	block.Token = p.curToken
	block.Statements = []ast.Statement{}

	p.nextToken()

	for !p.isBlockEnd() && !p.curTokenIs(token.EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		} else {
			p.synchronize()
		}
		p.nextToken()
	}

	if p.curTokenIs(token.EOF) {
		p.addError("unexpected EOF while parsing block starting with %q", block.Token.Literal)
	}

	return block
}

func (p *Parser) isBlockEnd() bool {
	if p.curToken.Type == token.LPERCENT {
		switch p.peekToken.Type {
		case token.ELSE, token.ELIF, token.ENDIF, token.ENDFOR, token.ENDBLOCK:
			return true
		}
	}
	return false
}

func (p *Parser) parseOutputStatement() *ast.OutputStatement {
	stmt := &ast.OutputStatement{Token: p.curToken}
	p.nextToken() // Consume {{
	stmt.Expression = p.parseExpression(LOWEST)
	if stmt.Expression == nil {
		return nil
	}
	if !p.expectPeek(token.RBRACE) {
		return nil
	}
	return stmt
}

func (p *Parser) parseExpressionStatement() *ast.ExpressionStatement {
	stmt := &ast.ExpressionStatement{Token: p.curToken}
	stmt.Expression = p.parseExpression(LOWEST)
	if stmt.Expression == nil {
		return nil
	}

	return stmt
}

func (p *Parser) parseExpression(precedence int) ast.Expression {
	prefix := p.prefixParseFns[p.curToken.Type]
	if prefix == nil {
		p.noPrefixParseFnError(p.curToken.Type)
		return nil
	}
	leftExp := prefix()

	for !p.peekTokenIs(token.RPERCENT) && !p.peekTokenIs(token.RBRACE) && precedence < p.peekPrecedence() {
		infix := p.infixParseFns[p.peekToken.Type]
		if infix == nil {
			return leftExp
		}
		p.nextToken()
		leftExp = infix(leftExp)
	}

	return leftExp
}

func (p *Parser) parseIdentifier() ast.Expression {
	return &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *Parser) parseIntegerLiteral() ast.Expression {
	lit := &ast.IntegerLiteral{Token: p.curToken}
	value, err := strconv.ParseInt(p.curToken.Literal, 0, 64)
	if err != nil {
		msg := fmt.Sprintf("could not parse %q as integer", p.curToken.Literal)
		p.errors = append(p.errors, msg)
		return nil
	}
	lit.Value = value
	return lit
}

func (p *Parser) parseBoolean() ast.Expression {
	return &ast.Boolean{Token: p.curToken, Value: p.curToken.Type == token.TRUE}
}

func (p *Parser) parseStringLiteral() ast.Expression {
	return &ast.StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *Parser) parsePrefixExpression() ast.Expression {
	expression := &ast.PrefixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
	}
	p.nextToken()
	expression.Right = p.parseExpression(PREFIX)
	return expression
}

func (p *Parser) parseInfixExpression(left ast.Expression) ast.Expression {
	expression := &ast.InfixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
		Left:     left,
	}
	precedence := p.curPrecedence()
	p.nextToken()
	expression.Right = p.parseExpression(precedence)
	return expression
}

func (p *Parser) parseAttributeAccess(left ast.Expression) ast.Expression {
	expression := &ast.AttributeAccess{
		Token:  p.curToken,
		Object: left,
	}

	if !p.expectPeek(token.IDENT) {
		return nil
	}

	expression.Attribute = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	return expression
}

func (p *Parser) parseFilterExpression(left ast.Expression) ast.Expression {
	expression := &ast.FilterExpression{
		Token: p.curToken,
		Input: left,
	}

	if !p.expectPeek(token.IDENT) {
		return nil
	}

	expression.Filter = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	return expression
}

func (p *Parser) expectPeek(t token.TokenType) bool {
	if p.peekTokenIs(t) {
		p.nextToken()
		return true
	}
	p.peekError(t)
	return false
}

func (p *Parser) curTokenIs(t token.TokenType) bool {
	return p.curToken.Type == t
}

func (p *Parser) peekTokenIs(t token.TokenType) bool {
	return p.peekToken.Type == t
}

func (p *Parser) peekError(t token.TokenType) {
	msg := fmt.Sprintf("expected next token to be %s, got %s instead", t, p.peekToken.Type)
	p.errors = append(p.errors, msg)
}

func (p *Parser) noPrefixParseFnError(t token.TokenType) {
	msg := fmt.Sprintf("no prefix parse function for %s found", t)
	p.errors = append(p.errors, msg)
}

func (p *Parser) registerPrefix(tokenType token.TokenType, fn prefixParseFn) {
	p.prefixParseFns[tokenType] = fn
}

func (p *Parser) registerInfix(tokenType token.TokenType, fn infixParseFn) {
	p.infixParseFns[tokenType] = fn
}

func (p *Parser) peekPrecedence() int {
	if p, ok := precedences[p.peekToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) curPrecedence() int {
	if p, ok := precedences[p.curToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) addError(format string, args ...interface{}) {
	p.errors = append(p.errors, fmt.Sprintf(format, args...))
}

func (p *Parser) synchronize() {
	for !p.curTokenIs(token.EOF) {
		switch p.curToken.Type {
		case token.RPERCENT, token.RBRACE:
			return
		case token.LPERCENT, token.LBRACE:
			return
		}

		if p.peekTokenIs(token.LPERCENT) || p.peekTokenIs(token.LBRACE) {
			p.nextToken()
			return
		}

		p.nextToken()
	}
}
