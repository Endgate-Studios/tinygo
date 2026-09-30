//go:build scheduler.cores

package task

// Affinity holds core+1 of a pinned task, or 0 for any core.
type Affinity struct {
	pin     uint8 // set by machine.LockCore
	osPin   uint8 // set by runtime.LockOSThread
	osLocks uint8
}

// PinnedCore returns the core of a pinned task. machine.LockCore has priority
// over runtime.LockOSThread.
func (t *Task) PinnedCore() (core uint32, pinned bool) {
	c := t.Affinity.pin
	if c == 0 {
		c = t.Affinity.osPin
	}
	return uint32(c) - 1, c != 0
}

// RunsOn returns whether the task can run on the given core.
func (t *Task) RunsOn(core uint32) bool {
	c, pinned := t.PinnedCore()
	return !pinned || c == core
}

func (t *Task) SetPin(core uint32) {
	t.Affinity.pin = uint8(core + 1)
}

func (t *Task) ClearPin() {
	t.Affinity.pin = 0
}

// LockOSThread pins the task to the given core. Calls nest as in Go.
func (t *Task) LockOSThread(core uint32) {
	if t.Affinity.osLocks == 0 {
		t.Affinity.osPin = uint8(core + 1)
	}
	t.Affinity.osLocks++
}

func (t *Task) UnlockOSThread() {
	if t.Affinity.osLocks == 0 {
		return
	}
	t.Affinity.osLocks--
	if t.Affinity.osLocks == 0 {
		t.Affinity.osPin = 0
	}
}

// PopFor removes the first task that can run on the given core.
func (q *Queue) PopFor(core uint32) *Task {
	mask := lockAtomics()
	var prev *Task
	for t := q.head; t != nil; prev, t = t, t.Next {
		if !t.RunsOn(core) {
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
