package linter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/twinmind/newo-tool/internal/nsl/ast"
	"gopkg.in/yaml.v3"
)

// SkillMetadata defines the structure for skill metadata YAML files.
type SkillMetadata struct {
	Parameters []struct {
		Name string `yaml:"name"`
	} `yaml:"parameters"`
}

func checkUndefinedVariables(filePath string, program *ast.Program) ([]LintError, error) {
	declaredParams, err := getDeclaredParameters(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get declared parameters: %w", err)
	}

	analyzer := newASTAnalyzer(filePath, declaredParams)
	analyzer.analyzeProgram(program)
	return analyzer.errors, nil
}

func getDeclaredParameters(nslFilePath string) ([]string, error) {
	metaPath := strings.TrimSuffix(nslFilePath, ".nsl") + ".meta.yaml"
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		metaPath = strings.TrimSuffix(nslFilePath, ".nsl") + ".meta.yml"
	}

	yamlFile, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}

	var metadata SkillMetadata
	if err := yaml.Unmarshal(yamlFile, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %s: %w", filepath.Base(metaPath), err)
	}

	var params []string
	for _, p := range metadata.Parameters {
		if p.Name != "" {
			params = append(params, p.Name)
		}
	}
	return params, nil
}

type astAnalyzer struct {
	filePath string
	scope    *scope
	errors   []LintError
}

type scope struct {
	vars   map[string]struct{}
	parent *scope
}

func newScope(parent *scope) *scope {
	return &scope{vars: make(map[string]struct{}), parent: parent}
}

func (s *scope) declare(name string) {
	if s == nil || name == "" {
		return
	}
	s.vars[name] = struct{}{}
}

func (s *scope) contains(name string) bool {
	if s == nil {
		return false
	}
	if _, ok := s.vars[name]; ok {
		return true
	}
	return s.parent.contains(name)
}

func newASTAnalyzer(filePath string, params []string) *astAnalyzer {
	root := newScope(nil)
	for _, p := range params {
		root.declare(p)
	}
	for name := range globalVars {
		root.declare(name)
	}
	return &astAnalyzer{filePath: filePath, scope: root}
}

func (a *astAnalyzer) analyzeProgram(program *ast.Program) {
	if program == nil {
		return
	}
	for _, stmt := range program.Statements {
		a.analyzeStatement(stmt)
	}
}

func (a *astAnalyzer) analyzeStatement(stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.SetStatement:
		if s.Value != nil {
			a.analyzeExpression(s.Value)
		}
		if s.Name != nil {
			a.scope.declare(s.Name.Value)
		}
	case *ast.ForStatement:
		if s.Sequence != nil {
			a.analyzeExpression(s.Sequence)
		}
		a.pushScope()
		if s.Iterator != nil {
			a.scope.declare(s.Iterator.Value)
		}
		if s.Body != nil {
			a.analyzeBlock(s.Body)
		}
		a.popScope()
	case *ast.IfStatement:
		if s.Condition != nil {
			a.analyzeExpression(s.Condition)
		}
		if s.Consequence != nil {
			a.analyzeBlock(s.Consequence)
		}
		for _, clause := range s.ElseIfs {
			if clause.Condition != nil {
				a.analyzeExpression(clause.Condition)
			}
			if clause.Consequence != nil {
				a.analyzeBlock(clause.Consequence)
			}
		}
		if s.Alternative != nil {
			a.analyzeBlock(s.Alternative)
		}
	case *ast.OutputStatement:
		a.analyzeExpression(s.Expression)
	case *ast.BlockStatement:
		a.analyzeBlock(s)
	case *ast.ExpressionStatement:
		a.analyzeExpression(s.Expression)
	}
}

func (a *astAnalyzer) analyzeBlock(block *ast.BlockStatement) {
	if block == nil {
		return
	}
	for _, stmt := range block.Statements {
		a.analyzeStatement(stmt)
	}
}

func (a *astAnalyzer) analyzeExpression(expr ast.Expression) {
	switch e := expr.(type) {
	case *ast.Identifier:
		a.checkIdentifier(e)
	case *ast.AttributeAccess:
		if e.Object != nil {
			a.analyzeExpression(e.Object)
		}
	case *ast.FilterExpression:
		if e.Input != nil {
			a.analyzeExpression(e.Input)
		}
	case *ast.InfixExpression:
		a.analyzeExpression(e.Left)
		a.analyzeExpression(e.Right)
	case *ast.PrefixExpression:
		a.analyzeExpression(e.Right)
	case *ast.Boolean, *ast.IntegerLiteral, *ast.StringLiteral:
		// literals: nothing to do
	}
}

func (a *astAnalyzer) checkIdentifier(ident *ast.Identifier) {
	if ident == nil {
		return
	}
	name := ident.Value
	if name == "" {
		return
	}
	if a.scope.contains(name) {
		return
	}

	line := ident.Token.Line
	if line == 0 {
		line = 1
	}

	a.errors = append(a.errors, LintError{
		FilePath: a.filePath,
		Line:     line,
		Message:  fmt.Sprintf("undefined variable: '%s' is used but not defined in parameters or in the skill", name),
	})
}

func (a *astAnalyzer) pushScope() {
	a.scope = newScope(a.scope)
}

func (a *astAnalyzer) popScope() {
	if a.scope != nil {
		a.scope = a.scope.parent
	}
}

// globalVars holds built-in variables, functions, and keywords that are always available.
var globalVars = map[string]bool{
	"true":      true,
	"false":     true,
	"null":      true,
	"None":      true,
	"range":     true,
	"dict":      true,
	"lipsum":    true,
	"cycler":    true,
	"joiner":    true,
	"namespace": true,
	"in":        true,
	"is":        true,
	"not":       true,
	"and":       true,
	"or":        true,
	"defined":   true,
	"undefined": true,
	"callable":  true,
	"divisible": true,
	"by":        true,
	"eq":        true,
	"equalto":   true,
	"even":      true,
	"ne":        true,
	"odd":       true,
}
