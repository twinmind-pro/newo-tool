package parser

import "github.com/twinmind/newo-tool/internal/nsl/ast"

// Visitor defines callbacks for traversing NSL AST nodes.
type Visitor interface {
	VisitProgram(*ast.Program)
	VisitSet(*ast.SetStatement)
	VisitIf(*ast.IfStatement)
	VisitFor(*ast.ForStatement)
	VisitOutput(*ast.OutputStatement)
	VisitExpression(ast.Expression)
}

// Walk traverses the AST and invokes the provided visitor on each node.
func Walk(v Visitor, node ast.Node) {
	switch n := node.(type) {
	case *ast.Program:
		if v != nil {
			v.VisitProgram(n)
		}
		for _, stmt := range n.Statements {
			Walk(v, stmt)
		}
	case *ast.SetStatement:
		if v != nil {
			v.VisitSet(n)
		}
		Walk(v, n.Value)
	case *ast.IfStatement:
		if v != nil {
			v.VisitIf(n)
		}
		Walk(v, n.Condition)
		if n.Consequence != nil {
			Walk(v, n.Consequence)
		}
		for _, clause := range n.ElseIfs {
			Walk(v, clause.Condition)
			if clause.Consequence != nil {
				Walk(v, clause.Consequence)
			}
		}
		if n.Alternative != nil {
			Walk(v, n.Alternative)
		}
	case *ast.ForStatement:
		if v != nil {
			v.VisitFor(n)
		}
		Walk(v, n.Sequence)
		if n.Body != nil {
			Walk(v, n.Body)
		}
	case *ast.OutputStatement:
		if v != nil {
			v.VisitOutput(n)
		}
		Walk(v, n.Expression)
	case *ast.BlockStatement:
		for _, stmt := range n.Statements {
			Walk(v, stmt)
		}
	case *ast.ExpressionStatement:
		Walk(v, n.Expression)
	case *ast.AttributeAccess:
		if v != nil {
			v.VisitExpression(n)
		}
		Walk(v, n.Object)
	case *ast.FilterExpression:
		if v != nil {
			v.VisitExpression(n)
		}
		Walk(v, n.Input)
	case *ast.InfixExpression:
		if v != nil {
			v.VisitExpression(n)
		}
		Walk(v, n.Left)
		Walk(v, n.Right)
	case *ast.PrefixExpression:
		if v != nil {
			v.VisitExpression(n)
		}
		Walk(v, n.Right)
	case *ast.Identifier, *ast.IntegerLiteral, *ast.StringLiteral, *ast.Boolean:
		if v != nil {
			v.VisitExpression(n.(ast.Expression))
		}
	default:
		// For any other expression types that may be added later,
		// just invoke the visitor callback.
		if expr, ok := n.(ast.Expression); ok && v != nil {
			v.VisitExpression(expr)
		}
	}
}
