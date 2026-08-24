package engine

import (
	"fmt"

	"github.com/titpetric/phpscript/model"
)

type loopContext struct {
	breaks         []int
	continues      []int
	continueTarget int
	// writeBack emits the instructions a by-reference foreach needs before
	// control leaves its body: the loop variable is written back into the
	// element it came from. It is nil for every other loop. `break` and
	// `return` leave without reaching the end of the body, so they emit it
	// too: a reference makes the element live, and PHP keeps a write made
	// before the jump.
	writeBack func() error
}

type compiler struct {
	program Program
	locals  map[string]int
	loops   []loopContext
	// class is the name of the class whose method body is being compiled, or
	// "" at top level. It is what `self`, `static` and `parent` resolve to,
	// the same collapse the interpreter's resolveClassName makes at run time.
	class    string
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
		c.collectClasses(ast.Stmts)
		for _, class := range c.program.classes {
			for _, method := range class.Methods {
				if method == nil {
					continue
				}
				if method.Name == "__construct" || method.Name == class.Name {
					continue
				}
				if err := c.classMethod(class.Name, method, "class."+class.Name+"."+method.Name); err != nil {
					return nil, err
				}
			}
		}
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
	case *model.Use:
		// Imports are resolved while parsing and emit no code.
	case *model.Declare:
		// No directive changes how the engine behaves; the block form still
		// wraps ordinary code.
		return c.block(node.Body, path+".body")
	case *model.Unset:
		return c.unsetStmt(node, path)
	case *model.Switch:
		return c.switchStmt(node, path)
	case *model.Try:
		return c.tryStmt(node, path)
	case *model.Throw:
		if err := c.expr(node.X, path+".value"); err != nil {
			return err
		}
		c.emit(instruction{op: opThrow})
	case *model.ClassDecl:
		// Top-level classes are hoisted before execution. Nested declarations
		// are a no-op here, matching the interpreter.
	case *model.Include:
		if err := c.include(node, path); err != nil {
			return err
		}
		c.emit(instruction{op: opPop})
	case *model.FuncDecl:
		if node.Class != "" {
			for _, class := range c.program.classes {
				if class.Name == node.Class {
					class.Methods[node.Name] = node
					break
				}
			}
			return c.classMethod(node.Class, node, path)
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
		// Returning leaves every enclosing loop at once, so each by-reference
		// foreach on the way out writes its loop variable back first. The
		// return value is already on the stack and each write-back is stack
		// balanced, so it stays on top.
		for i := len(c.loops) - 1; i >= 0; i-- {
			if c.loops[i].writeBack == nil {
				continue
			}
			if err := c.loops[i].writeBack(); err != nil {
				return err
			}
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

func (c *compiler) collectClasses(stmts []model.Stmt) {
	for _, stmt := range stmts {
		decl, ok := stmt.(*model.ClassDecl)
		if !ok {
			continue
		}
		class := &model.Class{
			Name:    decl.Name,
			Fields:  decl.Fields,
			Statics: decl.Statics,
			Consts:  decl.Consts,
			Methods: make(map[string]*model.FuncDecl, len(decl.Methods)),
		}
		for _, method := range decl.Methods {
			class.Methods[method.Name] = method
		}
		c.program.classes = append(c.program.classes, class)
	}
}

func (c *compiler) classMethod(className string, node *model.FuncDecl, path string) error {
	jumpAround := c.emit(instruction{op: opJump, target: -1})
	funcPC := len(c.program.code)
	enclosing := c.loops
	c.loops = nil
	enclosingClass := c.class
	c.class = className
	defer func() {
		c.loops = enclosing
		c.class = enclosingClass
	}()
	params := make([]string, 0, 1+len(node.Params))
	params = append(params, "this")
	_ = c.slot("this")
	for _, param := range node.Params {
		params = append(params, param.Name)
		_ = c.slot(param.Name)
	}
	if err := c.block(node.Body, path+".body"); err != nil {
		return err
	}
	c.emit(instruction{op: opPushConst, a: c.constant(nil)})
	c.emit(instruction{op: opReturn})
	c.program.code[jumpAround].target = len(c.program.code)
	if c.program.userFuncs == nil {
		c.program.userFuncs = make(map[string]userFuncDef)
	}
	c.program.userFuncs[className+"::"+node.Name] = userFuncDef{entryPC: funcPC, params: params}
	return nil
}

func (c *compiler) ensureContainer(expr model.Expr, path string) error {
	switch node := model.UnwrapParenthesized(expr).(type) {
	case *model.Var:
		c.emit(instruction{op: opLoad, a: c.slot(node.Name)})
		c.emit(instruction{op: opEnsureArray})
		c.emit(instruction{op: opDup})
		c.emit(instruction{op: opStore, a: c.slot(node.Name)})
		return nil
	case *model.Index:
		if node.Index == nil {
			return unsupported(path, "append container")
		}
		if err := c.ensureContainer(node.Base, path+".base"); err != nil {
			return err
		}
		if err := c.expr(node.Index, path+".index"); err != nil {
			return err
		}
		c.emit(instruction{op: opVivifyIndex})
		return nil
	case *model.PropAccess:
		if err := c.expr(node.Base, path+".base"); err != nil {
			return err
		}
		c.emit(instruction{op: opVivifyProperty, name: node.Name})
		return nil
	default:
		return c.expr(expr, path)
	}
}

func (c *compiler) include(node *model.Include, path string) error {
	if err := c.expr(node.Path, path+".path"); err != nil {
		return err
	}
	once := 0
	if node.Once {
		once = 1
	}
	c.emit(instruction{op: opInclude, a: once, name: node.Keyword})
	return nil
}

// tryStmt lays out a try as: body, then one block per catch clause, then the
// finally block. Every clause is compiled with its own declared type, variable
// slot and body, and the VM enters the first one whose type matches; the clause
// list, not the instruction stream, decides that.
//
// A clause that ran, and a body that completed, jump to the finally block. An
// error no clause matched goes there too, after the VM parks it in a hidden
// local, so the finally block runs before opRethrow resumes the search in the
// enclosing try. That local is an ordinary slot, so it is per call frame and a
// recursive call cannot see an outer frame's pending error.
func (c *compiler) tryStmt(node *model.Try, path string) error {
	if len(node.Catches) == 0 {
		return unsupported(path, "try without catch")
	}
	group := len(c.program.catchGroups)
	pendingSlot := c.slot(fmt.Sprintf("\x00try-pending-%d", group))
	clauses := make([]catchClause, len(node.Catches))
	for i, catch := range node.Catches {
		clauses[i] = catchClause{declaredType: catch.Type, local: -1, target: -1}
		if catch.Var != "" {
			clauses[i].local = c.slot(catch.Var)
		}
	}
	c.program.catchGroups = append(c.program.catchGroups, clauses)

	push := c.emit(instruction{op: opTryPush, a: group, c: pendingSlot, target: -1})
	if err := c.block(node.Body, path+".body"); err != nil {
		return err
	}
	pop := c.emit(instruction{op: opTryPop})
	c.program.code[push].b = pop
	// Every completed path leaves through one of these jumps, patched to the
	// finally block once its address is known.
	exits := []int{c.emit(instruction{op: opJump, target: -1})}

	for i := range node.Catches {
		clauses[i].target = len(c.program.code)
		if err := c.block(node.Catches[i].Body, fmt.Sprintf("%s.catch[%d]", path, i)); err != nil {
			return err
		}
		exits = append(exits, c.emit(instruction{op: opJump, target: -1}))
	}

	finallyTarget := len(c.program.code)
	for _, jump := range exits {
		c.program.code[jump].target = finallyTarget
	}
	c.program.code[push].target = finallyTarget
	if len(node.Finally) > 0 {
		if err := c.block(node.Finally, path+".finally"); err != nil {
			return err
		}
	}
	// Reached on every path out of the try, and a no-op unless the VM parked
	// an unmatched error in pendingSlot.
	c.emit(instruction{op: opRethrow, a: pendingSlot})
	return nil
}

func (c *compiler) funcDecl(node *model.FuncDecl, path string) error {
	jumpAround := c.emit(instruction{op: opJump, target: -1})
	funcPC := len(c.program.code)

	// A function body is its own control-flow scope: a loop around the
	// declaration is not a loop this body can break out of or write back to.
	// It is also outside any class, so `self::` in one resolves to nothing.
	enclosing := c.loops
	c.loops = nil
	enclosingClass := c.class
	c.class = ""
	defer func() {
		c.loops = enclosing
		c.class = enclosingClass
	}()

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

// unsetStmt lowers `unset($a, $b[$k])`. A local is cleared in place; an array
// entry is removed through the host. Property and static-property targets have
// no instruction yet, so a program using one falls back to the interpreter.
func (c *compiler) unsetStmt(node *model.Unset, path string) error {
	for i, target := range node.Targets {
		targetPath := fmt.Sprintf("%s.target[%d]", path, i)
		switch t := model.UnwrapParenthesized(target).(type) {
		case *model.Var:
			c.emit(instruction{op: opUnsetLocal, a: c.slot(t.Name)})
		case *model.Index:
			if t.Index == nil {
				return unsupported(targetPath, "unset of an append target")
			}
			if err := c.expr(t.Base, targetPath+".base"); err != nil {
				return err
			}
			if err := c.expr(t.Index, targetPath+".index"); err != nil {
				return err
			}
			c.emit(instruction{op: opUnsetIndex})
		default:
			return unsupported(targetPath, "unset target %T", target)
		}
	}
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
		if err := c.storeTop(keyTarget, "foreach", path+".key"); err != nil {
			return err
		}
	}
	c.emit(instruction{op: opLoad, a: valueSlot})
	// `foreach ($a as $v)` binds a copy of the element and `foreach ($a as &$v)`
	// binds the element itself. The copy is only made when the body assigns
	// through the target, since that is the only way the difference can be
	// observed, and copying every element of every loop would charge the common
	// read-only loop for a semantic it does not use.
	if !node.ByRef && model.AssignsTo(node.Body, valueTarget) {
		c.emit(instruction{op: opCopyValue})
	}
	if err := c.storeTop(valueTarget, "foreach", path+".value"); err != nil {
		return err
	}

	// writeBack pushes the loop variable and stores it into the element it came
	// from. It is emitted at each point control leaves the body of a
	// by-reference loop, and never for a by-value one.
	var writeBack func() error
	if node.ByRef {
		writeBack = func() error {
			if err := c.expr(valueTarget, path+".writeback"); err != nil {
				return err
			}
			c.emit(instruction{op: opIterSet, a: iterator})
			return nil
		}
	}

	c.loops = append(c.loops, loopContext{continueTarget: -2, writeBack: writeBack})
	if err := c.block(node.Body, path+".body"); err != nil {
		return err
	}

	// Falling off the end of the body and `continue` both resume the loop, so
	// they share one write-back.
	continuePC := len(c.program.code)
	if writeBack != nil {
		if err := writeBack(); err != nil {
			return err
		}
	}
	c.emit(instruction{op: opJump, target: nextPC})

	// `break` leaves the loop, and needs its own write-back because it skips
	// the one above. A by-value loop has none, so its breaks jump straight to
	// the close and no instruction is emitted here.
	breakPC := len(c.program.code)
	if writeBack != nil {
		if err := writeBack(); err != nil {
			return err
		}
		c.emit(instruction{op: opJump, target: -1})
	}

	closePC := len(c.program.code)
	c.emit(instruction{op: opIterClose, a: iterator})
	c.program.code[next].target = closePC
	if writeBack != nil {
		// Patch the jump the break write-back ends with. Without one, breakPC
		// already is closePC and nothing was emitted.
		c.program.code[closePC-1].target = closePC
	}
	loop := &c.loops[len(c.loops)-1]
	loop.continueTarget = continuePC
	for _, patch := range loop.breaks {
		c.program.code[patch].target = breakPC
	}
	for _, patch := range loop.continues {
		c.program.code[patch].target = continuePC
	}
	c.loops = c.loops[:len(c.loops)-1]
	return nil
}

// storeTop consumes a value already on the operand stack and writes it to a
// destructuring target. Index targets are evaluated once per write. kind names
// the construct the target belongs to, "foreach" or "list()", so a rejection
// reports the one the source actually contains.
func (c *compiler) storeTop(target model.Expr, kind, path string) error {
	switch target := target.(type) {
	case *model.Var:
		c.emit(instruction{op: opStore, a: c.slot(target.Name)})
	case *model.Index:
		if target.Index == nil {
			return unsupported(path, "append %s target", kind)
		}
		if err := c.expr(target.Base, path+".base"); err != nil {
			return err
		}
		if err := c.expr(target.Index, path+".index"); err != nil {
			return err
		}
		c.emit(instruction{op: opSetIndex, name: "="})
	case *model.PropAccess:
		// opSetProperty pops the receiver and then the value, the order the
		// *model.PropAccess branch of assignment emits.
		if err := c.expr(target.Base, path+".base"); err != nil {
			return err
		}
		c.emit(instruction{op: opSetProperty, name: target.Name, extra: "="})
	default:
		return unsupported(path, "%s target %T", kind, target)
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
		if err := c.ensureContainer(target.Base, path+".target.base"); err != nil {
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
		if err := c.storeTop(elem, "list()", fmt.Sprintf("%s.elem[%d]", path, i)); err != nil {
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
		switch node.Op {
		case "!", "+", "-", "~":
		default:
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
		switch node.Name {
		case "compact":
			return unsupported(path, "compact() requires scope reflection")
		case "defer":
			// defer() registers its callback on the frame that called it, and
			// the frame runs it on the way out. The bytecode engine hands a
			// binding a throwaway scope per call, so the registration would be
			// dropped rather than run late. It only became reachable once
			// closures compiled, since the callback is nearly always one.
			return unsupported(path, "defer() registers on the calling frame")
		}
		for i, argument := range node.Args {
			// An output parameter is handed to the binding as a setter for the
			// caller's variable, the way the interpreter emits __ref.
			if variable, ok := model.UnwrapParenthesized(argument).(*model.Var); ok &&
				model.ByRefArg(node.Name, node.Fallback, i) {
				c.emit(instruction{op: opRef, a: c.slot(variable.Name)})
				continue
			}
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
	case *model.ClassConst:
		c.emit(instruction{op: opClassConst, name: c.resolveClass(node.Class), extra: node.Name})
	case *model.Cast:
		if err := c.expr(node.X, path+".operand"); err != nil {
			return err
		}
		c.emit(instruction{op: opCast, name: node.Type})
	case *model.AssignExpr:
		if node.Op != "" && node.Op != "=" {
			return unsupported(path, "assignment expression operator %q", node.Op)
		}
		return c.assignment(node.Target, node.Op, node.Value, true, path)
	case *model.Include:
		return c.include(node, path)
	case *model.Closure:
		return c.closure(node, path)
	default:
		return unsupported(path, "expression %T", expr)
	}
	return nil
}

// closure compiles an anonymous function. The body is laid out inline and
// jumped over, the way funcDecl lays out a named function, and the opClosure
// left behind builds the callable value when control reaches it.
//
// The value is a func(...any) (any, error), the one shape Runtime.Callable
// reduces every PHP callable to, so usort() and array_map() invoke a compiled
// closure without knowing which backend produced it.
func (c *compiler) closure(node *model.Closure, path string) error {
	def := closureDef{thisSlot: -1}
	for i, param := range node.Params {
		paramPath := fmt.Sprintf("%s.param[%d]", path, i)
		switch {
		case param.Default != nil:
			// A default is an expression evaluated at call time in the scope of
			// the declaration, and no instruction runs on the path where the
			// argument is missing.
			return unsupported(paramPath, "closure parameter default")
		case param.ByRef:
			return unsupported(paramPath, "by-reference closure parameter")
		case param.Variadic:
			return unsupported(paramPath, "variadic closure parameter")
		}
		def.paramSlots = append(def.paramSlots, c.slot(param.Name))
	}
	for _, use := range node.Uses {
		if use.ByRef {
			// `use (&$x)` binds the enclosing variable itself, and a compiled
			// closure frame holds copies. Rejecting it sends the program to the
			// interpreter rather than silently compiling it as by-value.
			return unsupported(path, "by-reference capture use (&$%s)", use.Name)
		}
		def.captures = append(def.captures, c.slot(use.Name))
	}
	// A closure written in a method carries `$this` away with it, and a
	// `static function` is declared not to. The interpreter drops the class
	// with the receiver, so `self::` in a static closure resolves the same way
	// on both backends.
	if !node.Static && c.class != "" {
		def.thisSlot = c.slot("this")
	}

	jumpAround := c.emit(instruction{op: opJump, target: -1})
	def.entryPC = len(c.program.code)

	// The body is its own control-flow scope: a loop around the closure is not
	// one its `break` can leave, and a `return` in it returns from the closure.
	enclosingLoops, enclosingClass := c.loops, c.class
	c.loops = nil
	if node.Static {
		c.class = ""
	}
	err := c.block(node.Body, path+".body")
	c.loops, c.class = enclosingLoops, enclosingClass
	if err != nil {
		return err
	}
	c.emit(instruction{op: opPushConst, a: c.constant(nil)})
	c.emit(instruction{op: opReturn})
	c.program.code[jumpAround].target = len(c.program.code)

	c.program.closures = append(c.program.closures, def)
	c.emit(instruction{op: opClosure, a: len(c.program.closures) - 1})
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
	case ".", "+", "-", "*", "/", "%", "**", "==", "!=", "===", "!==", "<", "<=", ">", ">=",
		"&", "|", "^", "<<", ">>":
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

// resolveClass collapses the contextual class names onto the class being
// compiled. There is no inheritance in this tree, so `self`, `static` and
// `parent` all name the enclosing class, exactly as the interpreter's
// resolveClassName decides at run time. Outside a class body the name is left
// alone, so the host reports it the way the interpreter would.
func (c *compiler) resolveClass(name string) string {
	switch name {
	case "self", "static", "parent":
		if c.class != "" {
			return c.class
		}
	}
	return name
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
