package coverage

import (
	"unsafe"

	"github.com/titpetric/phpscript/model"
)

// stmtTable maps a statement to its coverage entry by the pointer it holds.
type stmtTable struct {
	slots []stmtSlot
	mask  uintptr
	count int
}

// stmtSlot holds one registered statement, its pointer and its entry.
type stmtSlot struct {
	stmt  model.Stmt
	key   uintptr
	entry *coverStmt
}

// stmtKey returns the pointer word of a statement interface.
func stmtKey(s model.Stmt) uintptr {
	return (*[2]uintptr)(unsafe.Pointer(&s))[1]
}

// tableHash spreads the aligned pointer over the table's low bits.
func tableHash(key, mask uintptr) uintptr {
	return ((key >> 3) * 0x9e3779b97f4a7c15) & mask
}

// get returns the entry a statement's pointer holds, or nil.
func (t *stmtTable) get(key uintptr) *coverStmt {
	if t.mask == 0 {
		return nil
	}
	for i := tableHash(key, t.mask); ; i = (i + 1) & t.mask {
		slot := &t.slots[i]
		if slot.entry == nil {
			return nil
		}
		if slot.key == key {
			return slot.entry
		}
	}
}

// put records an entry under a statement's pointer and keeps the entry there.
func (t *stmtTable) put(stmt model.Stmt, entry *coverStmt) {
	key := stmtKey(stmt)
	if t.mask == 0 || (t.count+1)*2 > len(t.slots) {
		t.grow()
	}
	for i := tableHash(key, t.mask); ; i = (i + 1) & t.mask {
		slot := &t.slots[i]
		if slot.entry == nil {
			*slot = stmtSlot{stmt: stmt, key: key, entry: entry}
			t.count++
			return
		}
		if slot.key == key {
			return
		}
	}
}

// grow doubles the table and rehashes the entries it holds.
func (t *stmtTable) grow() {
	size := len(t.slots) * 2
	if size < 16 {
		size = 16
	}
	slots := t.slots
	t.slots = make([]stmtSlot, size)
	t.mask = uintptr(size - 1)
	t.count = 0
	for _, slot := range slots {
		if slot.entry != nil {
			t.put(slot.stmt, slot.entry)
		}
	}
}

// each calls fn for every entry in the table.
func (t *stmtTable) each(fn func(*coverStmt)) {
	for i := range t.slots {
		if entry := t.slots[i].entry; entry != nil {
			fn(entry)
		}
	}
}
