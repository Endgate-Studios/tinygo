//go:build esp32s3

package radio

import (
	"sync"
	"time"
	"unsafe"
)

// The FreeRTOS objects of the OS functions, in Go. A tick is 1 ms, see
// osiMsToTick.

const (
	pdTrue  = 1
	pdFalse = 0
	forever = 0xffffffff // portMAX_DELAY
)

// semaphore is a counting semaphore. Its tokens are in a buffered channel.
type semaphore struct {
	tokens chan struct{}
}

func newSemaphore(max, initial uint32) *semaphore {
	s := &semaphore{tokens: make(chan struct{}, max)}
	for i := uint32(0); i < initial; i++ {
		s.tokens <- struct{}{}
	}
	return s
}

func (s *semaphore) take(ticks uint32) bool {
	if ticks == forever {
		<-s.tokens
		return true
	}
	select {
	case <-s.tokens:
		return true
	default:
	}
	if ticks == 0 {
		return false
	}
	t := getWaitTimer(ticks)
	select {
	case <-s.tokens:
		putWaitTimer(t, false)
		return true
	case <-t.C:
		putWaitTimer(t, true)
		return false
	}
}

// give does not block, so it also works in an interrupt.
func (s *semaphore) give() bool {
	select {
	case s.tokens <- struct{}{}:
		return true
	default:
		return false
	}
}

//export tinygo_wifi_semphr_create
func osiSemphrCreate(max, initial uint32) unsafe.Pointer {
	return handles.add(newSemaphore(max, initial))
}

//export tinygo_wifi_semphr_delete
func osiSemphrDelete(h unsafe.Pointer) {
	handles.remove(h)
}

//export tinygo_wifi_semphr_take
func osiSemphrTake(h unsafe.Pointer, ticks uint32) int32 {
	if s, ok := handles.get(h).(*semaphore); ok && s.take(ticks) {
		return pdTrue
	}
	return pdFalse
}

//export tinygo_wifi_semphr_give
func osiSemphrGive(h unsafe.Pointer) int32 {
	if s, ok := handles.get(h).(*semaphore); ok && s.give() {
		return pdTrue
	}
	return pdFalse
}

// mutex is a FreeRTOS mutex. A recursive mutex can be taken again by its
// owner task.
type mutex struct {
	sem       *semaphore
	recursive bool
	owner     unsafe.Pointer
	depth     int
}

//export tinygo_wifi_mutex_create
func osiMutexCreate() unsafe.Pointer {
	return handles.add(&mutex{sem: newSemaphore(1, 1)})
}

//export tinygo_wifi_recursive_mutex_create
func osiRecursiveMutexCreate() unsafe.Pointer {
	return handles.add(&mutex{sem: newSemaphore(1, 1), recursive: true})
}

//export tinygo_wifi_mutex_delete
func osiMutexDelete(h unsafe.Pointer) {
	handles.remove(h)
}

//export tinygo_wifi_mutex_lock
func osiMutexLock(h unsafe.Pointer) int32 {
	m, ok := handles.get(h).(*mutex)
	if !ok {
		return pdFalse
	}
	self := currentTask()
	if m.recursive && m.owner == self {
		m.depth++
		return pdTrue
	}
	m.sem.take(forever)
	m.owner, m.depth = self, 1
	return pdTrue
}

//export tinygo_wifi_mutex_unlock
func osiMutexUnlock(h unsafe.Pointer) int32 {
	m, ok := handles.get(h).(*mutex)
	if !ok {
		return pdFalse
	}
	if m.depth--; m.depth > 0 {
		return pdTrue
	}
	m.owner = nil
	m.sem.give()
	return pdTrue
}

// threadSemaphores are the semaphores of _wifi_thread_semphr_get, one for
// each task that is in a call to the blobs. free has the released ones.
var threadSemaphores struct {
	sync.Mutex
	m    map[unsafe.Pointer]unsafe.Pointer
	free []unsafe.Pointer
}

//export tinygo_wifi_thread_semphr_get
func osiThreadSemphrGet() unsafe.Pointer {
	self := currentTask()
	threadSemaphores.Lock()
	defer threadSemaphores.Unlock()
	if threadSemaphores.m == nil {
		threadSemaphores.m = map[unsafe.Pointer]unsafe.Pointer{}
	}
	h, ok := threadSemaphores.m[self]
	if !ok {
		if n := len(threadSemaphores.free); n > 0 {
			h = threadSemaphores.free[n-1]
			threadSemaphores.free = threadSemaphores.free[:n-1]
		} else {
			h = handles.add(newSemaphore(1, 0))
		}
		threadSemaphores.m[self] = h
	}
	return h
}

// releaseThreadSemaphore gives back the semaphore of the current goroutine
// after a call to the blobs. ESP-IDF frees it at thread exit with a pthread key
// destructor (esp_wifi/esp32s3/esp_adapter.c), but TinyGo has no exit hook.
// Without this, the map keeps each goroutine that called the blobs.
func releaseThreadSemaphore() {
	self := currentTask()
	threadSemaphores.Lock()
	defer threadSemaphores.Unlock()
	h, ok := threadSemaphores.m[self]
	if !ok {
		return
	}
	delete(threadSemaphores.m, self)
	// A late give after a timeout leaves a token, so do not use it again.
	if s, _ := handles.get(h).(*semaphore); s != nil && len(s.tokens) == 0 {
		threadSemaphores.free = append(threadSemaphores.free, h)
	} else {
		handles.remove(h)
	}
}

// queue is a FreeRTOS queue. The items are in a ring that is allocated once,
// so send and receive do not allocate, also in an interrupt. The channels
// count the free and the full slots, for the waits.
type queue struct {
	size        uintptr
	buf         []byte
	n           int
	lock        spinLock
	head, count int
	free, full  chan struct{}
}

func newQueue(n, size uint32) *queue {
	q := &queue{size: uintptr(size), buf: make([]byte, int(n)*int(size)), n: int(n),
		free: make(chan struct{}, n), full: make(chan struct{}, n)}
	for i := 0; i < q.n; i++ {
		q.free <- struct{}{}
	}
	return q
}

func (q *queue) slot(i int) []byte {
	return q.buf[uintptr(i)*q.size : uintptr(i+1)*q.size]
}

// send adds an item at the back, or at the front as xQueueSendToFront does.
func (q *queue) send(item unsafe.Pointer, ticks uint32, front bool) bool {
	if !takeToken(q.free, ticks) {
		return false
	}
	state := q.lock.acquire()
	var i int
	if front {
		q.head = (q.head + q.n - 1) % q.n
		i = q.head
	} else {
		i = (q.head + q.count) % q.n
	}
	q.count++
	copy(q.slot(i), unsafe.Slice((*byte)(item), q.size))
	q.lock.release(state)
	q.full <- struct{}{}
	return true
}

func (q *queue) recv(item unsafe.Pointer, ticks uint32) bool {
	if !takeToken(q.full, ticks) {
		return false
	}
	state := q.lock.acquire()
	copy(unsafe.Slice((*byte)(item), q.size), q.slot(q.head))
	q.head = (q.head + 1) % q.n
	q.count--
	q.lock.release(state)
	q.free <- struct{}{}
	return true
}

// takeToken receives from c, and waits for at most ticks.
func takeToken(c chan struct{}, ticks uint32) bool {
	switch ticks {
	case forever:
		<-c
		return true
	case 0:
		select {
		case <-c:
			return true
		default:
			return false
		}
	}
	t := getWaitTimer(ticks)
	select {
	case <-c:
		putWaitTimer(t, false)
		return true
	case <-t.C:
		putWaitTimer(t, true)
		return false
	}
}

//export tinygo_wifi_queue_create
func osiQueueCreate(n, size uint32) unsafe.Pointer {
	return handles.add(newQueue(n, size))
}

//export tinygo_wifi_queue_delete
func osiQueueDelete(h unsafe.Pointer) {
	handles.remove(h)
}

//export tinygo_wifi_queue_send
func osiQueueSend(h, item unsafe.Pointer, ticks uint32) int32 {
	if q, ok := handles.get(h).(*queue); ok && q.send(item, ticks, false) {
		return pdTrue
	}
	return pdFalse
}

// The blobs send urgent messages to the front.
//
//export tinygo_wifi_queue_send_to_front
func osiQueueSendToFront(h, item unsafe.Pointer, ticks uint32) int32 {
	if q, ok := handles.get(h).(*queue); ok && q.send(item, ticks, true) {
		return pdTrue
	}
	return pdFalse
}

//export tinygo_wifi_queue_send_from_isr
func osiQueueSendFromISR(h, item, woken unsafe.Pointer) int32 {
	if woken != nil {
		*(*int32)(woken) = 0
	}
	return osiQueueSend(h, item, 0)
}

//export tinygo_wifi_queue_recv
func osiQueueRecv(h, item unsafe.Pointer, ticks uint32) int32 {
	if q, ok := handles.get(h).(*queue); ok && q.recv(item, ticks) {
		return pdTrue
	}
	return pdFalse
}

//export tinygo_wifi_queue_msg_waiting
func osiQueueMsgWaiting(h unsafe.Pointer) uint32 {
	if q, ok := handles.get(h).(*queue); ok {
		state := q.lock.acquire()
		n := q.count
		q.lock.release(state)
		return uint32(n)
	}
	return 0
}

// eventGroup is a FreeRTOS event group.
type eventGroup struct {
	mu      sync.Mutex
	bits    uint32
	waiters []chan struct{}
}

//export tinygo_wifi_event_group_create
func osiEventGroupCreate() unsafe.Pointer {
	return handles.add(&eventGroup{})
}

//export tinygo_wifi_event_group_delete
func osiEventGroupDelete(h unsafe.Pointer) {
	handles.remove(h)
}

//export tinygo_wifi_event_group_set_bits
func osiEventGroupSetBits(h unsafe.Pointer, bits uint32) uint32 {
	g, ok := handles.get(h).(*eventGroup)
	if !ok {
		return 0
	}
	g.mu.Lock()
	g.bits |= bits
	for _, w := range g.waiters {
		select {
		case w <- struct{}{}:
		default:
		}
	}
	g.waiters = g.waiters[:0]
	r := g.bits
	g.mu.Unlock()
	return r
}

//export tinygo_wifi_event_group_clear_bits
func osiEventGroupClearBits(h unsafe.Pointer, bits uint32) uint32 {
	g, ok := handles.get(h).(*eventGroup)
	if !ok {
		return 0
	}
	g.mu.Lock()
	r := g.bits
	g.bits &^= bits
	g.mu.Unlock()
	return r
}

//export tinygo_wifi_event_group_wait_bits
func osiEventGroupWaitBits(h unsafe.Pointer, bits uint32, clearOnExit, waitAll int32, ticks uint32) uint32 {
	g, ok := handles.get(h).(*eventGroup)
	if !ok {
		return 0
	}
	var deadline <-chan time.Time
	fired := false
	if ticks != forever && ticks != 0 {
		t := getWaitTimer(ticks)
		defer func() { putWaitTimer(t, fired) }()
		deadline = t.C
	}
	w := make(chan struct{}, 1)
	for {
		g.mu.Lock()
		have := g.bits & bits
		if (waitAll != 0 && have == bits) || (waitAll == 0 && have != 0) {
			r := g.bits
			if clearOnExit != 0 {
				g.bits &^= bits
			}
			g.mu.Unlock()
			return r
		}
		if ticks == 0 {
			r := g.bits
			g.mu.Unlock()
			return r
		}
		select {
		case <-w: // a wake of an earlier pass
		default:
		}
		g.waiters = append(g.waiters, w)
		g.mu.Unlock()
		select {
		case <-w:
		case <-deadline:
			fired = true
			g.mu.Lock()
			r := g.bits
			g.mu.Unlock()
			return r
		}
	}
}

// waitTimers keeps the stopped timers of the timed waits. The blobs wait with
// a timeout tens of times per scan, and a new timer each time is garbage.
var waitTimers struct {
	sync.Mutex
	free []*time.Timer
}

func getWaitTimer(ticks uint32) *time.Timer {
	d := time.Duration(ticks) * time.Millisecond
	waitTimers.Lock()
	if n := len(waitTimers.free); n > 0 {
		t := waitTimers.free[n-1]
		waitTimers.free = waitTimers.free[:n-1]
		waitTimers.Unlock()
		t.Reset(d)
		return t
	}
	waitTimers.Unlock()
	return time.NewTimer(d)
}

// putWaitTimer gives t back to the pool. fired tells whether the wait got
// the value of t.C. If Stop fails otherwise, the timer is firing, maybe on the
// other core, and its value comes soon. Wait for it, or the next wait that
// reuses t gets it and times out at once.
func putWaitTimer(t *time.Timer, fired bool) {
	if !fired && !t.Stop() {
		<-t.C
	}
	waitTimers.Lock()
	waitTimers.free = append(waitTimers.free, t)
	waitTimers.Unlock()
}
