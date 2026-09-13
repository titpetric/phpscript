package engine

// Register-form fusion: a peephole pass over the compiled stream that
// rewrites the stack shapes the compiler emits most into single
// instructions reading their operands from slots or the constant pool and
// optionally writing the result straight back to a slot. The operand stack
// is the flat VM's dominant cost per executed instruction - every value
// crosses an append, a nil-clear and a pop - and the fused forms skip it.
//
// Fused shapes, matched left to right, longest first:
//
//	opLoad x; opLoad y; opBinary      -> opBinLL x, y
//	opLoad x; opPushConst k; opBinary -> opBinLC x, k
//	opPushConst k; opBinary           -> opBinTC top, k
//
// followed by an optional store fold: any of the above, or a plain
// opBinary, directly followed by a plain `=` store into a slot writes the
// result there instead of pushing it (instruction.target = slot+1; 0 keeps
// the push). The VM's fused case reproduces opLoad's extras/host fallback,
// opStore's reassignment check and the SetGlobal offer exactly.
//
// An instruction that is a jump target, a try boundary, a catch target or a
// function entry stays addressable: a run containing one anywhere but its
// head is not fused, and every recorded pc is renumbered afterwards.

// fusePrograms rewrites p.code in place and remaps every recorded pc.
func fuseProgram(p *Program) {
	if len(p.code) == 0 {
		return
	}
	protected := protectedPCs(p)

	fused := make([]instruction, 0, len(p.code))
	newPC := make([]int, len(p.code)+1)
	i := 0
	for i < len(p.code) {
		newPC[i] = len(fused)
		inst, width := fuseAt(p, protected, i)
		// Every consumed instruction maps to the fused head, which only
		// matters for opTryPush.b when the try body's last instruction was
		// consumed: the range bound lands on the run that absorbed it.
		for j := 1; j < width; j++ {
			newPC[i+j] = len(fused)
		}
		fused = append(fused, inst)
		i += width
	}
	newPC[len(p.code)] = len(fused)

	for i := range fused {
		switch fused[i].op {
		case opJump, opJumpFalse, opJumpTrue, opIterNext:
			fused[i].target = newPC[fused[i].target]
		case opTryPush:
			fused[i].target = newPC[fused[i].target]
			fused[i].b = newPC[fused[i].b]
		}
	}
	for g, group := range p.catchGroups {
		for c := range group {
			p.catchGroups[g][c].target = newPC[group[c].target]
		}
	}
	for c := range p.closures {
		p.closures[c].entryPC = newPC[p.closures[c].entryPC]
	}
	for name, def := range p.userFuncs {
		def.entryPC = newPC[def.entryPC]
		p.userFuncs[name] = def
	}
	p.code = fused
}

// fuseAt returns the instruction to emit for position i and how many source
// instructions it consumed.
func fuseAt(p *Program, protected map[int]bool, i int) (instruction, int) {
	code := p.code
	inst := code[i]

	fusable := func(j int) bool { return j < len(code) && !protected[j] }

	var fused instruction
	width := 0
	switch {
	case inst.op == opLoad && fusable(i+1) && fusable(i+2) &&
		code[i+1].op == opLoad && code[i+2].op == opBinary:
		b := code[i+2]
		fused = instruction{op: opBinLL, a: inst.a, c: code[i+1].a, b: b.b, name: b.name}
		width = 3
	case inst.op == opLoad && fusable(i+1) && fusable(i+2) &&
		code[i+1].op == opPushConst && code[i+2].op == opBinary:
		b := code[i+2]
		fused = instruction{op: opBinLC, a: inst.a, c: code[i+1].a, b: b.b, name: b.name}
		width = 3
	case inst.op == opPushConst && fusable(i+1) && code[i+1].op == opBinary:
		b := code[i+1]
		fused = instruction{op: opBinTC, c: inst.a, b: b.b, name: b.name}
		width = 2
	case inst.op == opBinary:
		fused = inst
		width = 1
	default:
		return inst, 1
	}

	// Store fold: the result goes straight to the slot when the next
	// instruction is a plain `=` store that keeps nothing on the stack.
	if j := i + width; fusable(j) && code[j].op == opStore &&
		(code[j].name == "" || code[j].name == "=") &&
		code[j].b == 0 && code[j].c == 0 && code[j].extra == "" {
		fused.target = code[j].a + 1
		width++
	}
	return fused, width
}

// protectedPCs collects every instruction index recorded anywhere as a pc:
// jump targets, try boundaries, catch clause targets and function entries.
// A protected index must survive as its own instruction so the reference
// still lands on it.
func protectedPCs(p *Program) map[int]bool {
	protected := map[int]bool{}
	for _, inst := range p.code {
		switch inst.op {
		case opJump, opJumpFalse, opJumpTrue, opIterNext:
			protected[inst.target] = true
		case opTryPush:
			protected[inst.target] = true
			protected[inst.b] = true
		}
	}
	for _, group := range p.catchGroups {
		for _, clause := range group {
			protected[clause.target] = true
		}
	}
	for _, def := range p.closures {
		protected[def.entryPC] = true
	}
	for _, def := range p.userFuncs {
		protected[def.entryPC] = true
	}
	return protected
}
