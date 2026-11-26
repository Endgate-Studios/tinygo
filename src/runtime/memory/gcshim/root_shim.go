//go:build gc.custom && (tinygo.wasm || (baremetal && (esp32 || esp32s3 || esp32c3 || esp32p4 || scheduler.cores)) || (windows && amd64 && scheduler.tasks))

package gcshim

// roots is outside shim on purpose: markRoots skips shim, and the globals
// scan must see these pointers to keep the Roots themselves alive.
var roots struct {
	list [MaxRoots]Root
	n    int
}

// AddRoot has the collector scan r's live part on every collection, until
// RemoveRoot. It returns ErrTooManyRoots when MaxRoots are registered.
func AddRoot(r Root) error {
	shim.lock.Lock()
	defer shim.lock.Unlock()
	return addRootLocked(r)
}

// RemoveRoot stops the scan of r. It does nothing if r is not registered.
func RemoveRoot(r Root) {
	shim.lock.Lock()
	defer shim.lock.Unlock()
	for i, x := range roots.list[:roots.n] {
		if x == r {
			roots.n--
			roots.list[i] = roots.list[roots.n]
			roots.list[roots.n] = nil
			return
		}
	}
}

// addRootLocked is AddRoot for a caller that holds shim.lock.
func addRootLocked(r Root) error {
	if roots.n == MaxRoots {
		return ErrTooManyRoots
	}
	roots.list[roots.n] = r
	roots.n++
	return nil
}

// markRegisteredRoots marks what the Roots point at. The caller holds
// shim.lock, with the world stopped.
func markRegisteredRoots() {
	for _, r := range roots.list[:roots.n] {
		if start, end := r.Live(); end > start {
			shim.heap.MarkRange(start, end)
		}
	}
}
