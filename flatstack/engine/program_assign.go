package engine

import (
	"fmt"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
)

// This file compiles the stores: assignment over every lvalue shape, the
// container a nested write vivifies first, `??=`, `list()` and the value an
// assignment used as an expression answers.
//
// php resolves the container, the key and the receiver of a target before the
// value, which `$m[$i++] = $i` reads. opSetIndex and opSetProperty take that
// order under `a: 1`; the destructuring path in storeTop holds a value already
// on the stack and takes the other.

func (c *compiler) ensureContainer(expr model.Expr, path string) error {
	switch node := modelutil.UnwrapParenthesized(expr).(type) {
	case *model.Var:
		if nodeConst, ok := c.staticSlot(node.Name); ok {
			c.emit(instruction{op: opStaticLoad, a: nodeConst, name: node.Name})
			c.emit(instruction{op: opEnsureArray})
			c.emit(instruction{op: opDup})
			c.emit(instruction{op: opStaticStore, a: nodeConst, name: node.Name})
			return nil
		}
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
		if node.NameExpr != nil {
			return unsupported(path, "dynamic property name")
		}
		if err := c.expr(node.Base, path+".base"); err != nil {
			return err
		}
		c.emit(instruction{op: opVivifyProperty, name: node.Name})
		return nil
	case *model.StaticProp:
		// `self::$entries[$k] = $v`: the emitted code makes the static property
		// an array and writes it back, then indexing vivifies below it.
		class, err := c.resolveClass(node.Class)
		if err != nil {
			return err
		}
		c.emit(instruction{op: opStaticProp, name: class, extra: node.Name})
		c.emit(instruction{op: opEnsureArray})
		c.emit(instruction{op: opDup})
		c.emit(instruction{op: opSetStaticProp, a: c.constant("="), name: class, extra: node.Name})
		return nil
	default:
		return c.expr(expr, path)
	}
}

// storeTop consumes a value already on the operand stack and writes it to a
// destructuring target. An index target runs its base and index once per
// write. kind names the construct the target belongs to, "foreach" or
// "list()" and a rejection reports the one the source actually contains.
func (c *compiler) storeTop(target model.Expr, kind, path string) error {
	switch target := target.(type) {
	case *model.Var:
		if nodeConst, ok := c.staticSlot(target.Name); ok {
			c.emit(instruction{op: opStaticStore, a: nodeConst, name: target.Name})
			return nil
		}
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
		if target.NameExpr != nil {
			return unsupported(path, "dynamic property name")
		}
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
	// `$a ??= b` reads the target first and stores while it answers null. A
	// store of the value already there would write a string offset php leaves
	// untouched. See parser.assignExpr.
	if co, ok := value.(*model.Coalesce); ok && co.Assign {
		return c.coalesceAssignment(target, operator, co, keep, path)
	}
	// A target that holds an expression resolves ahead of the value; one that
	// names storage runs nothing and takes the value first.
	switch target := target.(type) {
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
		if err := c.expr(value, path+".value"); err != nil {
			return err
		}
		c.emit(instruction{op: opSetIndex, a: 1, b: boolInt(appendValue), c: boolInt(keep), name: operator})
		return nil
	case *model.PropAccess:
		if target.NameExpr != nil {
			return unsupported(path, "dynamic property name")
		}
		if err := c.expr(target.Base, path+".target.base"); err != nil {
			return err
		}
		if err := c.expr(value, path+".value"); err != nil {
			return err
		}
		c.emit(instruction{op: opSetProperty, a: 1, b: boolInt(keep), name: target.Name, extra: operator})
		return nil
	}
	if err := c.expr(value, path+".value"); err != nil {
		return err
	}
	switch target := target.(type) {
	case *model.Var:
		if nodeConst, ok := c.staticSlot(target.Name); ok {
			c.emit(instruction{op: opStaticStore, a: nodeConst, b: boolInt(keep), name: target.Name, extra: operator})
			return nil
		}
		constructorID := ""
		if operator == "" || operator == "=" {
			if _, ok := modelutil.UnwrapParenthesized(value).(*model.New); ok {
				constructorID = target.Name
			}
		}
		c.emit(instruction{op: opStore, a: c.slot(target.Name), b: boolInt(keep), name: operator, extra: constructorID})
	case *model.StaticProp:
		// The operator travels through the constant pool: name and extra
		// already hold the class and the property.
		class, err := c.resolveClass(target.Class)
		if err != nil {
			return err
		}
		c.emit(instruction{op: opSetStaticProp, a: c.constant(operator), b: boolInt(keep), name: class, extra: target.Name})
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

// coalesceAssignment compiles `$a ??= b`: the target's value stays on the
// stack when it holds something; the store runs over the right side alone.
func (c *compiler) coalesceAssignment(target model.Expr, operator string, co *model.Coalesce, keep bool, path string) error {
	switch node := modelutil.UnwrapParenthesized(target).(type) {
	case *model.Index:
		if node.Index != nil {
			return c.coalesceIndex(node, co, keep, path)
		}
	case *model.PropAccess:
		if node.NameExpr == nil {
			return c.coalesceProp(node, co, keep, path)
		}
	}
	if err := c.expr(co.Left, path+".target"); err != nil {
		return err
	}
	c.emit(instruction{op: opDup})
	setJump := c.emit(instruction{op: opJumpNotNil, target: -1})
	c.emit(instruction{op: opPop})
	if err := c.assignment(target, operator, co.Right, true, path+".coalesce"); err != nil {
		return err
	}
	c.program.code[setJump].target = len(c.program.code)
	if !keep {
		c.emit(instruction{op: opPop})
	}
	return nil
}

// coalesceIndex compiles `$a[k] ??= b` over two hidden slots: the container and
// the key run once, and the read and the store both load them back.
func (c *compiler) coalesceIndex(target *model.Index, co *model.Coalesce, keep bool, path string) error {
	baseSlot, keySlot := c.indexSlots()
	if err := c.resolveIndex(target, baseSlot, keySlot, path); err != nil {
		return err
	}
	c.emit(instruction{op: opLoad, a: baseSlot})
	c.emit(instruction{op: opLoad, a: keySlot})
	c.emit(instruction{op: opIndex})
	c.emit(instruction{op: opDup})
	setJump := c.emit(instruction{op: opJumpNotNil, target: -1})
	c.emit(instruction{op: opPop})
	if err := c.expr(co.Right, path+".value"); err != nil {
		return err
	}
	c.emit(instruction{op: opLoad, a: baseSlot})
	c.emit(instruction{op: opLoad, a: keySlot})
	c.emit(instruction{op: opSetIndex, c: boolInt(true), name: "="})
	c.program.code[setJump].target = len(c.program.code)
	if !keep {
		c.emit(instruction{op: opPop})
	}
	return nil
}

// coalesceProp compiles `$o->p ??= b`: the receiver runs once into a hidden
// slot, which the read and the store both load back.
func (c *compiler) coalesceProp(target *model.PropAccess, co *model.Coalesce, keep bool, path string) error {
	recvSlot := c.slot(fmt.Sprintf("\x00assign-recv-%d", len(c.program.code)))
	if err := c.expr(target.Base, path+".target.base"); err != nil {
		return err
	}
	c.emit(instruction{op: opStore, a: recvSlot})
	c.emit(instruction{op: opLoad, a: recvSlot})
	c.emit(instruction{op: opGetProperty, name: target.Name})
	c.emit(instruction{op: opDup})
	setJump := c.emit(instruction{op: opJumpNotNil, target: -1})
	c.emit(instruction{op: opPop})
	if err := c.expr(co.Right, path+".value"); err != nil {
		return err
	}
	c.emit(instruction{op: opLoad, a: recvSlot})
	c.emit(instruction{op: opSetProperty, b: boolInt(true), name: target.Name, extra: "="})
	c.program.code[setJump].target = len(c.program.code)
	if !keep {
		c.emit(instruction{op: opPop})
	}
	return nil
}

// assignExprValue compiles an assignment used as a value. A plain store pushes
// what it wrote; a compound operator resolves inside the host, which reports
// no result. The slots that held the target read it again.
func (c *compiler) assignExprValue(node *model.AssignExpr, path string) error {
	if node.Op == "" || node.Op == "=" {
		return c.assignment(node.Target, node.Op, node.Value, true, path)
	}
	switch target := modelutil.UnwrapParenthesized(node.Target).(type) {
	case *model.Index:
		if target.Index == nil {
			return unsupported(path, "append target of %q", node.Op)
		}
		baseSlot, keySlot := c.indexSlots()
		if err := c.resolveIndex(target, baseSlot, keySlot, path); err != nil {
			return err
		}
		c.emit(instruction{op: opLoad, a: baseSlot})
		c.emit(instruction{op: opLoad, a: keySlot})
		if err := c.expr(node.Value, path+".value"); err != nil {
			return err
		}
		c.emit(instruction{op: opSetIndex, a: 1, name: node.Op})
		c.emit(instruction{op: opLoad, a: baseSlot})
		c.emit(instruction{op: opLoad, a: keySlot})
		c.emit(instruction{op: opIndex})
		return nil
	case *model.PropAccess:
		if target.NameExpr != nil {
			return unsupported(path, "dynamic property name")
		}
		recvSlot := c.slot(fmt.Sprintf("\x00assign-recv-%d", len(c.program.code)))
		if err := c.expr(target.Base, path+".target.base"); err != nil {
			return err
		}
		c.emit(instruction{op: opStore, a: recvSlot})
		c.emit(instruction{op: opLoad, a: recvSlot})
		if err := c.expr(node.Value, path+".value"); err != nil {
			return err
		}
		c.emit(instruction{op: opSetProperty, a: 1, name: target.Name, extra: node.Op})
		c.emit(instruction{op: opLoad, a: recvSlot})
		c.emit(instruction{op: opGetProperty, name: target.Name})
		return nil
	case *model.StaticProp:
		class, err := c.resolveClass(target.Class)
		if err != nil {
			return err
		}
		if err := c.expr(node.Value, path+".value"); err != nil {
			return err
		}
		c.emit(instruction{op: opSetStaticProp, a: c.constant(node.Op), name: class, extra: target.Name})
		c.emit(instruction{op: opStaticProp, name: class, extra: target.Name})
		return nil
	}
	return c.assignment(node.Target, node.Op, node.Value, true, path)
}

// indexSlots names the two hidden slots an index target resolves into.
func (c *compiler) indexSlots() (int, int) {
	at := len(c.program.code)
	return c.slot(fmt.Sprintf("\x00assign-base-%d", at)), c.slot(fmt.Sprintf("\x00assign-key-%d", at))
}

// resolveIndex runs the container and the key of target once and leaves each in
// its slot, where a read and a store both reach them.
func (c *compiler) resolveIndex(target *model.Index, baseSlot, keySlot int, path string) error {
	if err := c.ensureContainer(target.Base, path+".target.base"); err != nil {
		return err
	}
	c.emit(instruction{op: opStore, a: baseSlot})
	if err := c.expr(target.Index, path+".target.index"); err != nil {
		return err
	}
	c.emit(instruction{op: opStore, a: keySlot})
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
