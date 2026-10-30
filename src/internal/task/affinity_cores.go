//go:build scheduler.cores

package task

// Affinity holds the assigned core of a task.
type Affinity struct {
	// Core is the assigned core plus one, or 0 when the task may run on any core.
	Core uint8

	osCore  uint8 // set by runtime.LockOSThread, the core plus one
	osLocks uint8
}

// AssignedCore returns the core the task must run on. machine.AssignCore has
// priority over runtime.LockOSThread.
func (t *Task) AssignedCore() (core uint32, assigned bool) {
	c := t.Affinity.Core
	if c == 0 {
		c = t.Affinity.osCore
	}
	return uint32(c) - 1, c != 0
}

// CanRunOn reports whether the task may run on the given core.
func (t *Task) CanRunOn(core uint32) bool {
	c, assigned := t.AssignedCore()
	return !assigned || c == core
}

// LockOSThread keeps the task on the given core. Calls nest as in Go.
func (t *Task) LockOSThread(core uint32) {
	if t.Affinity.osLocks == 0 {
		t.Affinity.osCore = uint8(core + 1)
	}
	t.Affinity.osLocks++
}

func (t *Task) UnlockOSThread() {
	if t.Affinity.osLocks == 0 {
		return
	}
	t.Affinity.osLocks--
	if t.Affinity.osLocks == 0 {
		t.Affinity.osCore = 0
	}
}

// PopCore removes and returns the first task that may run on the given core.
func (q *Queue) PopCore(core uint32) *Task {
	mask := lockAtomics()
	var prev *Task
	for t := q.head; t != nil; prev, t = t, t.Next {
		if !t.CanRunOn(core) {
			continue
		}
		if prev == nil {
			q.head = t.Next
		} else {
			prev.Next = t.Next
		}
		if q.tail == t {
			q.tail = prev
		}
		t.Next = nil
		unlockAtomics(mask)
		return t
	}
	unlockAtomics(mask)
	return nil
}
