package syntax

import (
	"fmt"
	"golox/loxerror"
	"golox/references"
	"golox/scanner"
)

var currentClass references.ClassType = references.NoneClass

type VariableData struct {
	variableType references.FunctionType
	defined      bool
}

type Resolver struct {
	interpreter     *Interpreter
	scopes          *Stack
	currentFunction references.FunctionType
}

func NewResolver(interpreter *Interpreter) *Resolver {
	return &Resolver{
		interpreter:     interpreter,
		scopes:          NewStack(),
		currentFunction: references.None,
	}
}

func (resolver *Resolver) Resolve(stmts []Stmt) {
	defer func() {
		if r := recover(); r != nil {
			if !loxerror.HadError() && !loxerror.HadRuntimeError() {
				if err, ok := r.(error); ok {
					fmt.Printf("Failed to resolve program: %s\n", err.Error())
				} else {
					fmt.Println("Failed to resolve program.")
				}
			}
		}
	}()

	resolver.beginScope()
	resolver.resolveStatements(stmts)
	resolver.endScope()
}

func (resolver *Resolver) visitBlockStmt(stmt *Block) interface{} {
	resolver.beginScope()
	resolver.resolveStatements(stmt.statements)
	resolver.endScope()
	return nil
}

func (resolver *Resolver) visitVarCmdStmt(stmt *VarCmd) interface{} {
	resolver.declare(stmt.name, references.None)
	if stmt.initializer != nil {
		resolver.resolveExpression(stmt.initializer)
	}

	resolver.define(stmt.name, references.None)
	return nil
}

func (resolver *Resolver) visitVariableExpr(expr *Variable) interface{} {
	if !resolver.scopes.IsEmpty() && !resolver.isDefined(expr.name.Lexeme, expr.t) {
		throwError(expr.name, fmt.Sprintf("Can't read local variable '%s' in its own initializer.", expr.name.Lexeme))
	}

	resolver.resolveLocal(expr, expr.name)
	return nil
}

func (resolver *Resolver) visitThisExpr(expr *This) interface{} {
	if currentClass == references.NoneClass {
		throwError(expr.keyword, "Can't use 'this' outside of a class.")
	}

	resolver.resolveLocal(expr, expr.keyword)
	return nil
}

func (resolver *Resolver) isDefined(lexeme string, t references.FunctionType) bool {
	for i := resolver.scopes.length - 1; i >= 0; i-- {
		data, ok := resolver.scopes.Get(i).(map[string]*VariableData)[buildKey(lexeme, t)]
		if ok {
			return data.defined
		}
	}

	return false
}

func (resolver *Resolver) visitAssignExpr(expr *Assign) interface{} {
	resolver.resolveExpression(expr.value)
	resolver.resolveLocal(expr, expr.name)
	return nil
}

func (resolver *Resolver) visitFunctionStmt(stmt *Function) interface{} {
	resolver.declare(stmt.name, references.Function)
	resolver.define(stmt.name, references.Function)

	resolver.resolveFunction(stmt, references.Function)
	return nil
}

func (resolver *Resolver) visitExpressionStmt(stmt *Expression) interface{} {
	resolver.resolveExpression(stmt.expression)
	return nil
}

func (resolver *Resolver) visitIfCmdStmt(stmt *IfCmd) interface{} {
	resolver.resolveExpression(stmt.condition)
	resolver.resolveStatement(stmt.thenBranch)
	if stmt.elseBranch != nil {
		resolver.resolveStatement(stmt.elseBranch)
	}

	return nil
}

func (resolver *Resolver) visitSetExpr(expr *Set) interface{} {
	resolver.resolveExpression(expr.value)
	resolver.resolveExpression(expr.object)
	return nil
}

func (resolver *Resolver) visitClassStmt(stmt *Class) interface{} {
	enclosingClassType := currentClass
	currentClass = references.KlassClass

	resolver.declare(stmt.name, references.Klass)
	resolver.define(stmt.name, references.Klass)

	if stmt.superclass != nil && stmt.name.Lexeme == stmt.superclass.name.Lexeme {
		throwError(stmt.superclass.name, "A class can't inherit from itself.")
	}

	if stmt.superclass != nil {
		//resolver.beginScope()
		resolver.scopes.Peek().(map[string]*VariableData)[buildKey("super", references.None)] = &VariableData{variableType: references.Method, defined: true}
	}

	if stmt.superclass != nil {
		currentClass = references.SubClass
		resolver.resolveExpression(stmt.superclass)
	}

	resolver.beginScope()
	resolver.scopes.Peek().(map[string]*VariableData)[buildKey("this", references.None)] = &VariableData{
		variableType: references.Property,
		defined:      true,
	}

	for _, method := range stmt.methods {
		declaration := references.Method
		if method.name.Lexeme == "init" {
			declaration = references.Initializer
		}
		resolver.resolveFunction(method, declaration)
	}

	resolver.endScope()

	if stmt.superclass != nil {
		//resolver.endScope()
	}

	currentClass = enclosingClassType

	return nil
}

func (resolver *Resolver) visitSuperExpr(expr *Super) interface{} {
	if currentClass == references.NoneClass {
		throwError(expr.keyword, "Can't use 'super' outside of a class.")
	} else if currentClass != references.SubClass {
		throwError(expr.keyword, "Can't use 'super' in a class with no superclass.")
	}

	resolver.resolveLocal(expr, expr.keyword)
	return nil
}

func (resolver *Resolver) visitGetMethodExpr(expr *GetMethod) interface{} {
	resolver.resolveExpression(expr.object)
	return nil
}

func (resolver *Resolver) visitGetFieldExpr(expr *GetField) interface{} {
	resolver.resolveExpression(expr.object)
	return nil
}

func (resolver *Resolver) visitPrintStmt(stmt *Print) interface{} {
	resolver.resolveExpression(stmt.expression)
	return nil
}

func (resolver *Resolver) visitBreakCmdStmt(stmt *BreakCmd) interface{} {
	if resolver.currentFunction == references.None {
		throwError(stmt.keyword, "Can't break from top-level code.")
	}

	return nil
}

func (resolver *Resolver) visitContinueCmdStmt(stmt *ContinueCmd) interface{} {
	if resolver.currentFunction == references.None {
		throwError(stmt.keyword, "Can't continue from top-level code.")
	}

	return nil
}

func (resolver *Resolver) visitReturnCmdStmt(stmt *ReturnCmd) interface{} {
	if resolver.currentFunction == references.None {
		throwError(stmt.keyword, "Can't return from top-level code.")
	}

	if resolver.currentFunction == references.Initializer {
		throwError(stmt.keyword, "Can't return a value from an initializer.")
	}

	if stmt.value != nil {
		resolver.resolveExpression(stmt.value)
	}

	return nil
}

func (resolver *Resolver) visitWhileLoopStmt(stmt *WhileLoop) interface{} {
	resolver.resolveExpression(stmt.condition)
	resolver.resolveStatement(stmt.body)
	return nil
}

func (resolver *Resolver) visitBinaryExpr(expr *Binary) interface{} {
	resolver.resolveExpression(expr.left)
	resolver.resolveExpression(expr.right)
	return nil
}

func (resolver *Resolver) visitCallExpr(expr *Call) interface{} {
	resolver.resolveExpression(expr.callee)

	for _, arg := range expr.arguments {
		resolver.resolveExpression(arg)
	}

	return nil
}

func (resolver *Resolver) visitGroupingExpr(expr *Grouping) interface{} {
	resolver.resolveExpression(expr.expression)
	return nil
}

func (resolver *Resolver) visitLiteralExpr(expr *Literal) interface{} {
	return nil
}

func (resolver *Resolver) visitUnaryExpr(expr *Unary) interface{} {
	resolver.resolveExpression(expr.right)
	return nil
}

func (resolver *Resolver) visitLogicalExpr(expr *Logical) interface{} {
	resolver.resolveExpression(expr.left)
	resolver.resolveExpression(expr.right)
	return nil
}

func (resolver *Resolver) resolveFunction(stmt *Function, functionType references.FunctionType) {
	enclosingFunction := resolver.currentFunction
	resolver.currentFunction = functionType

	resolver.beginScope()
	for _, token := range stmt.params {
		resolver.declare(token, references.None)
		resolver.define(token, references.None)
	}

	resolver.resolveStatements(stmt.body)
	resolver.endScope()
	resolver.currentFunction = enclosingFunction
}

func (resolver *Resolver) resolveLocal(expr Expr, name *scanner.Token) {
	t := references.None
	if variable, ok := expr.(*Variable); ok {
		t = variable.t
	}

	for i := resolver.scopes.Len() - 1; i >= 0; i-- {
		if _, ok := resolver.scopes.Get(i).(map[string]*VariableData)[buildKey(name.Lexeme, t)]; ok {
			index := resolver.scopes.Len() - 1 - i
			resolver.interpreter.resolve(expr, &index)
			return
		}
	}

	throwError(name, fmt.Sprintf("Couldn't resolve variable '%s'.", name.Lexeme))
}

func (resolver *Resolver) resolveStatements(statements []Stmt) {
	for _, stmt := range statements {
		resolver.resolveStatement(stmt)
	}
}

func (resolver *Resolver) resolveStatement(stmt Stmt) {
	stmt.accept(resolver)
}

func (resolver *Resolver) resolveExpression(expr Expr) {
	expr.accept(resolver)
}

func (resolver *Resolver) declare(name *scanner.Token, t references.FunctionType) {
	if resolver.scopes.IsEmpty() {
		return
	}

	scope := resolver.scopes.Peek().(map[string]*VariableData)
	if v, ok := scope[buildKey(name.Lexeme, t)]; ok {
		throwError(name, fmt.Sprintf("%s already exists with name %s", references.GetFunctionTypeName(v.variableType), name.Lexeme))
	}

	scope[buildKey(name.Lexeme, t)] = &VariableData{
		variableType: t,
		defined:      false,
	}
}

func (resolver *Resolver) define(name *scanner.Token, t references.FunctionType) {
	if resolver.scopes.IsEmpty() {
		return
	}

	resolver.scopes.Peek().(map[string]*VariableData)[buildKey(name.Lexeme, t)].defined = true
}

func (resolver *Resolver) beginScope() {
	resolver.scopes.Push(map[string]*VariableData{})
}

func (resolver *Resolver) endScope() {
	resolver.scopes.Pop()
}

func buildKey(name string, t references.FunctionType) string {
	return fmt.Sprintf("%s - %s", name, references.GetFunctionTypeName(t))
}
