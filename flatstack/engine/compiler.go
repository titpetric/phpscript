package engine

import (
	"fmt"

	"github.com/titpetric/phpscript/model"
)

type loopContext struct {
	breaks         []int
	continues      []int
	continueTarget int
}

type compiler struct {
	program  Program
	locals   map[string]int
	loops    []loopContext
	nextIter int
}

// Compile validates and lowers the entire AST before execution can cause side
// effects. Any unsupported nested node rejects the whole program.
func Compile(ast *model.Program) (program *Program, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			program = nil
			err = fmt.Errorf("flatstack: compiler panic: %v", recovered)
		}
	}()
	c := &compiler{locals: make(map[string]int)}
	if ast != nil {
		for i, stmt := range ast.Stmts {
			if err := c.stmt(stmt, fmt.Sprintf("stmt[%d]", i)); err != nil {
				return nil, err
			}
		}
	}
	return &c.program, nil
}

func (c *compiler) emit(inst instruction) int {
	index := len(c.program.code)
	c.program.code = append(c.program.code, inst)
	return index
}

func (c *compiler) constant(value any) int {
	index := len(c.program.constants)
	c.program.constants = append(c.program.constants, value)
	return index
}

func (c *compiler) slot(name string) int {
	if slot, ok := c.locals[name]; ok {
		return slot
	}
	slot := len(c.program.localNames)
	c.locals[name] = slot
	c.program.localNames = append(c.program.localNames, name)
	return slot
}

func (c *compiler) block(statements []model.Stmt, path string) error {
	for i, stmt := range statements {
		if err := c.stmt(stmt, fmt.Sprintf("%s[%d]", path, i)); err != nil {
			return err
		}
	}
	return nil
}

func (c *compiler) stmt(stmt model.Stmt, path string) error {
	switch node := stmt.(type) {
	case *model.InlineHTML:
		c.emit(instruction{op: opPushConst, a: c.constant(node.Text)})
		c.emit(instruction{op: opEcho})
	case *model.Echo:
		for i, argument := range node.Args {
			if err := c.expr(argument, fmt.Sprintf("%s.echo[%d]", path, i)); err != nil {
				return err
			}
			c.emit(instruction{op: opEcho})
		}
	case *model.ExprStmt:
		if err := c.expr(node.X, path+".expr"); err != nil {
			return err
		}
		c.emit(instruction{op: opPop})
	case *model.Assign:
		return c.assignment(node.Target, node.Op, node.Value, false, path)
	case *model.If:
		if err := c.expr(node.Cond, path+".cond"); err != nil {
			return err
		}
		falseJump := c.emit(instruction{op: opJumpFalse, target: -1})
		if err := c.block(node.Then, path+".then"); err != nil {
			return err
		}
		endJump := c.emit(instruction{op: opJump, target: -1})
		c.program.code[falseJump].target = len(c.program.code)
		if err := c.block(node.Else, path+".else"); err != nil {
			return err
		}
		c.program.code[endJump].target = len(c.program.code)
	case *model.For:
		return c.forStmt(node, path)
	case *model.Foreach:
		return c.foreachStmt(node, path)
	case *model.Switch:
		return c.switchStmt(node, path)
	case *model.Try:
		return c.tryStmt(node, path)
	case *model.Throw:
		if err := c.expr(node.X, path+".value"); err != nil {
			return err
		}
		c.emit(instruction{op: opThrow})
	case *model.FuncDecl:
		if node.Class != "" {
			return unsupported(path, "class method decl %s::%s", node.Class, node.Name)
		}
		return c.funcDecl(node, path)
	case *model.Return:
		if node.Value != nil {
			if err := c.expr(node.Value, path+".value"); err != nil {
				return err
			}
		} else {
			c.emit(instruction{op: opPushConst, a: c.constant(nil)})
		}
		c.emit(instruction{op: opReturn})
	case *model.Break:
		if len(c.loops) == 0 {
			return unsupported(path, "break outside loop")
		}
		jump := c.emit(instruction{op: opJump, target: -1})
		index := len(c.loops) - 1
		c.loops[index].breaks = append(c.loops[index].breaks, jump)
	case *model.Continue:
		loopIndex := -1
		for i := len(c.loops) - 1; i >= 0; i-- {
			if c.loops[i].continueTarget != -1 {
				loopIndex = i
				break
			}
		}
		if loopIndex < 0 {
			return unsupported(path, "continue outside loop")
		}
		jump := c.emit(instruction{op: opJump, target: -1})
		c.loops[loopIndex].continues = append(c.loops[loopIndex].continues, jump)
	default:
		return unsupported(path, "statement %T", stmt)
	}
	return nil
}

func (c *compiler) tryStmt(node *model.Try, path string) error {
	if len(node.Catches) == 0 && len(node.Finally) == 0 {
		return unsupported(path, "try without catch or finally")
	}
	catchSlot := -1
	var catch *model.Catch
	if len(node.Catches) > 0 {
		catch = &node.Catches[0]
		if catch.Var != "" {
			catchSlot = c.slot(catch.Var)
		}
	}
	push := c.emit(instruction{op: opTryPush, a: catchSlot, target: -1})
	if err := c.block(node.Body, path+".body"); err != nil {
		return err
	}
	pop := c.emit(instruction{op: opTryPop})
	c.program.code[push].b = pop
	endJump := c.emit(instruction{op: opJump, target: -1})

	c.program.code[push].target = len(c.program.code)
	if catch != nil {
		if err := c.block(catch.Body, path+".catch"); err != nil {
			return err
		}
	}
	finallyTarget := len(c.program.code)
	c.program.code[endJump].target = finallyTarget
	if len(node.Finally) > 0 {
		if err := c.block(node.Finally, path+".finally"); err != nil {
			return err
		}
	}
	return nil
}

func (c *compiler) funcDecl(node *model.FuncDecl, path string) error {
	jumpAround := c.emit(instruction{op: opJump, target: -1})
	funcPC := len(c.program.code)

	for _, param := range node.Params {
		_ = c.slot(param.Name)
	}

	if err := c.block(node.Body, path+".body"); err != nil {
		return err
	}
	c.emit(instruction{op: opPushConst, a: c.constant(nil)})
	c.emit(instruction{op: opReturn})

	endPC := len(c.program.code)
	c.program.code[jumpAround].target = endPC

	if c.program.userFuncs == nil {
		c.program.userFuncs = make(map[string]userFuncDef)
	}
	params := make([]string, len(node.Params))
	for i, p := range node.Params {
		params[i] = p.Name
	}
	c.program.userFuncs[node.Name] = userFuncDef{
		entryPC: funcPC,
		params:  params,
	}
	return nil
}

func (c *compiler) switchStmt(node *model.Switch, path string) error {
	if err := c.expr(node.Cond, path+".cond"); err != nil {
		return err
	}
	conditionSlot := c.slot(fmt.Sprintf("\x00switch-%d", len(c.program.code)))
	c.emit(instruction{op: opStore, a: conditionSlot})
	caseJumps := make([]int, len(node.Cases))
	for i, switchCase := range node.Cases {
		c.emit(instruction{op: opLoad, a: conditionSlot})
		if err := c.expr(switchCase.Value, fmt.Sprintf("%s.case[%d].value", path, i)); err != nil {
			return err
		}
		c.emit(instruction{op: opBinary, name: "=="})
		caseJumps[i] = c.emit(instruction{op: opJumpTrue, target: -1})
	}
	defaultJump := c.emit(instruction{op: opJump, target: -1})
	c.loops = append(c.loops, loopContext{continueTarget: -1})
	for i, switchCase := range node.Cases {
		c.program.code[caseJumps[i]].target = len(c.program.code)
		if err := c.block(switchCase.Body, fmt.Sprintf("%s.case[%d].body", path, i)); err != nil {
			return err
		}
	}
	c.program.code[defaultJump].target = len(c.program.code)
	if err := c.block(node.Default, path+".default"); err != nil {
		return err
	}
	endPC := len(c.program.code)
	control := &c.loops[len(c.loops)-1]
	for _, patch := range control.breaks {
		c.program.code[patch].target = endPC
	}
	c.loops = c.loops[:len(c.loops)-1]
	return nil
}

func (c *compiler) forStmt(node *model.For, path string) error {
	if node.Init != nil {
		if err := c.stmt(node.Init, path+".init"); err != nil {
			return err
		}
	}
	conditionPC := len(c.program.code)
	var endCondition int = -1
	if node.Cond != nil {
		if err := c.expr(node.Cond, path+".cond"); err != nil {
			return err
		}
		endCondition = c.emit(instruction{op: opJumpFalse, target: -1})
	}
	c.loops = append(c.loops, loopContext{continueTarget: -2})
	if err := c.block(node.Body, path+".body"); err != nil {
		return err
	}
	postPC := len(c.program.code)
	loop := &c.loops[len(c.loops)-1]
	loop.continueTarget = postPC
	for _, patch := range loop.continues {
		c.program.code[patch].target = postPC
	}
	if node.Post != nil {
		if err := c.stmt(node.Post, path+".post"); err != nil {
			return err
		}
	}
	c.emit(instruction{op: opJump, target: conditionPC})
	endPC := len(c.program.code)
	if endCondition >= 0 {
		c.program.code[endCondition].target = endPC
	}
	for _, patch := range loop.breaks {
		c.program.code[patch].target = endPC
	}
	c.loops = c.loops[:len(c.loops)-1]
	return nil
}

func (c *compiler) foreachStmt(node *model.Foreach, path string) error {
	keyTarget, valueTarget := node.KeyTarget, node.ValTarget
	if keyTarget == nil && node.KeyVar != "" {
		keyTarget = &model.Var{Name: node.KeyVar}
	}
	if valueTarget == nil && node.ValVar != "" {
		valueTarget = &model.Var{Name: node.ValVar}
	}
	if valueTarget == nil {
		return unsupported(path+".value", "missing foreach target")
	}
	if err := c.expr(node.Source, path+".source"); err != nil {
		return err
	}
	iterator := c.nextIter
	c.nextIter++
	keySlot := -1
	if keyTarget != nil {
		keySlot = c.slot(fmt.Sprintf("\x00foreach-key-%d", iterator))
	}
	valueSlot := c.slot(fmt.Sprintf("\x00foreach-value-%d", iterator))
	c.emit(instruction{op: opIterInit, a: iterator})
	nextPC := len(c.program.code)
	next := c.emit(instruction{op: opIterNext, a: iterator, b: keySlot, c: valueSlot, target: -1})
	if keyTarget != nil {
		c.emit(instruction{op: opLoad, a: keySlot})
		if err := c.storeTop(keyTarget, path+".key"); err != nil {
			return err
		}
	}
	c.emit(instruction{op: opLoad, a: valueSlot})
	if err := c.storeTop(valueTarget, path+".value"); err != nil {
		return err
	}
	c.loops = append(c.loops, loopContext{continueTarget: nextPC})
	if err := c.block(node.Body, path+".body"); err != nil {
		return err
	}
	c.emit(instruction{op: opJump, target: nextPC})
	closePC := len(c.program.code)
	c.emit(instruction{op: opIterClose, a: iterator})
	endPC := len(c.program.code)
	c.program.code[next].target = closePC
	loop := &c.loops[len(c.loops)-1]
	for _, patch := range loop.breaks {
		c.program.code[patch].target = closePC
	}
	for _, patch := range loop.continues {
		c.program.code[patch].target = nextPC
	}
	c.loops = c.loops[:len(c.loops)-1]
	_ = endPC
	return nil
}

// storeTop consumes a value already on the operand stack and writes it to a
// foreach target. Index targets are evaluated once per iteration.
func (c *compiler) storeTop(target model.Expr, path string) error {
	switch target := target.(type) {
	case *model.Var:
		c.emit(instruction{op: opStore, a: c.slot(target.Name)})
	case *model.Index:
		if target.Index == nil {
			return unsupported(path, "append foreach target")
		}
		if err := c.expr(target.Base, path+".base"); err != nil {
			return err
		}
		if err := c.expr(target.Index, path+".index"); err != nil {
			return err
		}
		c.emit(instruction{op: opSetIndex, name: "="})
	default:
		return unsupported(path, "foreach target %T", target)
	}
	return nil
}

func (c *compiler) assignment(target model.Expr, operator string, value model.Expr, keep bool, path string) error {
	if err := c.expr(value, path+".value"); err != nil {
		return err
	}
	switch target := target.(type) {
	case *model.Var:
		constructorID := ""
		if operator == "" || operator == "=" {
			if _, ok := model.UnwrapParenthesized(value).(*model.New); ok {
				constructorID = target.Name
			}
		}
		c.emit(instruction{op: opStore, a: c.slot(target.Name), b: boolInt(keep), name: operator, extra: constructorID})
	case *model.Index:
		if err := c.expr(target.Base, path+".target.base"); err != nil {
			return err
		}
		appendValue := target.Index == nil || operator == "[]="
		if !appendValue {
			if err := c.expr(target.Index, path+".target.index"); err != nil {
				return err
			}
		}
		c.emit(instruction{op: opSetIndex, b: boolInt(appendValue), c: boolInt(keep), name: operator})
	case *model.PropAccess:
		if err := c.expr(target.Base, path+".target.base"); err != nil {
			return err
		}
		c.emit(instruction{op: opSetProperty, b: boolInt(keep), name: target.Name, extra: operator})
	case *model.ListExpr:
		return c.listAssignment(target.Elems, keep, path)
	case *model.ArrayLit:
		elems := make([]model.Expr, len(target.Items))
		for i, item := range target.Items {
			elems[i] = item.Val
		}
		return c.listAssignment(elems, keep, path)
	default:
		return unsupported(path+".target", "assignment target %T", target)
	}
	return nil
}

func (c *compiler) listAssignment(elems []model.Expr, keep bool, path string) error {
	sourceSlot := c.slot(fmt.Sprintf("\x00list-%d", len(c.program.code)))
	c.emit(instruction{op: opStore, a: sourceSlot})
	for i, elem := range elems {
		if elem == nil {
			continue
		}
		c.emit(instruction{op: opLoad, a: sourceSlot})
		c.emit(instruction{op: opPushConst, a: c.constant(int64(i))})
		c.emit(instruction{op: opIndex})
		if err := c.storeTop(elem, fmt.Sprintf("%s.elem[%d]", path, i)); err != nil {
			return err
		}
	}
	if keep {
		c.emit(instruction{op: opLoad, a: sourceSlot})
	}
	return nil
}

func (c *compiler) expr(expr model.Expr, path string) error {
	switch node := expr.(type) {
	case *model.Lit:
		c.emit(instruction{op: opPushConst, a: c.constant(node.Value)})
	case *model.Parenthesized:
		return c.expr(node.X, path+".expression")
	case *model.Var:
		c.emit(instruction{op: opLoad, a: c.slot(node.Name)})
	case *model.ArrayLit:
		for i, item := range node.Items {
			if item.Key == nil {
				c.emit(instruction{op: opPushConst, a: c.constant(nil)})
			} else if err := c.expr(item.Key, fmt.Sprintf("%s.key[%d]", path, i)); err != nil {
				return err
			}
			if err := c.expr(item.Val, fmt.Sprintf("%s.value[%d]", path, i)); err != nil {
				return err
			}
		}
		c.emit(instruction{op: opArray, a: len(node.Items)})
	case *model.Index:
		if node.Index == nil {
			return unsupported(path, "append index read")
		}
		if err := c.expr(node.Base, path+".base"); err != nil {
			return err
		}
		if err := c.expr(node.Index, path+".index"); err != nil {
			return err
		}
		c.emit(instruction{op: opIndex})
	case *model.Binary:
		return c.binary(node, path)
	case *model.Unary:
		if node.Op == "++" || node.Op == "--" {
			return c.incDec(node, path)
		}
		if node.Op != "!" && node.Op != "+" && node.Op != "-" {
			return unsupported(path, "unary operator %q", node.Op)
		}
		if err := c.expr(node.X, path+".operand"); err != nil {
			return err
		}
		c.emit(instruction{op: opUnary, name: node.Op})
	case *model.Ternary:
		if err := c.expr(node.Cond, path+".cond"); err != nil {
			return err
		}
		c.emit(instruction{op: opDup})
		falseJump := c.emit(instruction{op: opJumpFalse, target: -1})
		if node.Then != nil {
			c.emit(instruction{op: opPop})
			if err := c.expr(node.Then, path+".then"); err != nil {
				return err
			}
		}
		endJump := c.emit(instruction{op: opJump, target: -1})
		c.program.code[falseJump].target = len(c.program.code)
		c.emit(instruction{op: opPop})
		if err := c.expr(node.Else, path+".else"); err != nil {
			return err
		}
		c.program.code[endJump].target = len(c.program.code)
	case *model.Call:
		if node.Name == "compact" {
			return unsupported(path, "compact() requires scope reflection")
		}
		for i, argument := range node.Args {
			if err := c.expr(argument, fmt.Sprintf("%s.arg[%d]", path, i)); err != nil {
				return err
			}
		}
		c.emit(instruction{op: opCall, a: len(node.Args), name: node.Name, extra: node.Fallback})
	case *model.New:
		for i, argument := range node.Args {
			if err := c.expr(argument, fmt.Sprintf("%s.arg[%d]", path, i)); err != nil {
				return err
			}
		}
		c.emit(instruction{op: opConstruct, a: len(node.Args), name: node.Class})
	case *model.MethodCall:
		if err := c.expr(node.Base, path+".base"); err != nil {
			return err
		}
		for i, argument := range node.Args {
			if err := c.expr(argument, fmt.Sprintf("%s.arg[%d]", path, i)); err != nil {
				return err
			}
		}
		c.emit(instruction{op: opCallMethod, a: len(node.Args), name: node.Method})
	case *model.PropAccess:
		if err := c.expr(node.Base, path+".base"); err != nil {
			return err
		}
		c.emit(instruction{op: opGetProperty, name: node.Name})
	case *model.AssignExpr:
		if node.Op != "" && node.Op != "=" {
			return unsupported(path, "assignment expression operator %q", node.Op)
		}
		return c.assignment(node.Target, node.Op, node.Value, true, path)
	default:
		return unsupported(path, "expression %T", expr)
	}
	return nil
}

func (c *compiler) incDec(node *model.Unary, path string) error {
	switch target := node.X.(type) {
	case *model.Var:
		c.emit(instruction{op: opIncDecLocal, a: c.slot(target.Name), b: boolInt(node.Postfix), name: node.Op})
	case *model.Index:
		if target.Index == nil {
			return unsupported(path, "increment append target")
		}
		if err := c.expr(target.Base, path+".base"); err != nil {
			return err
		}
		if err := c.expr(target.Index, path+".index"); err != nil {
			return err
		}
		c.emit(instruction{op: opIncDecIndex, b: boolInt(node.Postfix), name: node.Op})
	default:
		return unsupported(path, "increment target %T", node.X)
	}
	return nil
}

func (c *compiler) binary(node *model.Binary, path string) error {
	switch node.Op {
	case "&&", "||":
		if err := c.expr(node.Left, path+".left"); err != nil {
			return err
		}
		jumpOp, constant := opJumpFalse, false
		if node.Op == "||" {
			jumpOp, constant = opJumpTrue, true
		}
		branch := c.emit(instruction{op: jumpOp, target: -1})
		if err := c.expr(node.Right, path+".right"); err != nil {
			return err
		}
		c.emit(instruction{op: opTruthy})
		end := c.emit(instruction{op: opJump, target: -1})
		c.program.code[branch].target = len(c.program.code)
		c.emit(instruction{op: opPushConst, a: c.constant(constant)})
		c.program.code[end].target = len(c.program.code)
	case ".", "+", "-", "*", "/", "%", "==", "!=", "===", "!==", "<", "<=", ">", ">=":
		if err := c.expr(node.Left, path+".left"); err != nil {
			return err
		}
		if err := c.expr(node.Right, path+".right"); err != nil {
			return err
		}
		c.emit(instruction{op: opBinary, name: node.Op})
	default:
		return unsupported(path, "binary operator %q", node.Op)
	}
	return nil
}

func unsupported(path, format string, args ...any) error {
	return fmt.Errorf("flatstack: %s: unsupported %s", path, fmt.Sprintf(format, args...))
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
