//go:build esp32s3

package radio

import (
	"runtime/interrupt"
	"unsafe"

	"runtime/memory/arena"
)

// The share of each radio in the blob heap without an arena option. The
// first radio makes the heap and the second one adds its share. Wi-Fi with 4
// HTTP clients had a peak of 56 KB, and with 48 KB it lost many RX frames.
// BLE with 6 activities uses about 29 KB, and about 25 KB with 2.
const (
	DefaultArenaSize    = 64 * 1024
	DefaultBLEArenaSize = 32 * 1024
)

// Address windows of ESP-IDF v5.3.2 soc/esp32s3/include/soc/soc.h.
const (
	dramLow       = 0x3FC88000 // SOC_DRAM_LOW
	dramHigh      = 0x3FD00000 // SOC_DRAM_HIGH
	extramDataLow = 0x3C000000 // SOC_EXTRAM_DATA_LOW
	extramDataHi  = 0x3E000000 // SOC_EXTRAM_DATA_HIGH
)

// blobHeap holds the memory of the blobs in internal RAM, which the MAC can
// reach with DMA. The blobs get memory in two ways:
//
//   - The OS table: _malloc, _free, _wifi_malloc, _malloc_internal and the
//     other memory entries point to tinygo_wifi_malloc and so on.
//   - Direct calls to malloc, calloc, realloc and free. The --wrap flags of
//     targets/esp32s3.json send them to __wrap_malloc and so on.
//
// psramHeap is nil by default. With WithPSRAMArena, the _wifi_malloc group,
// which ESP-IDF can put in SPIRAM (CONFIG_SPIRAM_TRY_ALLOCATE_WIFI_LWIP),
// tries it first, and so does the frame pool.
var blobHeap, psramHeap *arena.Heap

// Option configures Init and BLEEnable.
type Option func(*options)

type options struct {
	size          int
	mem           []byte
	psram         []byte
	bleActivities int
	rxBuffers     int
}

// WithArenaSize sets the share of the radio in the blob heap, which comes
// from the GC heap.
func WithArenaSize(n int) Option {
	return func(o *options) { o.size, o.mem = n, nil }
}

// WithArenaMemory gives the share of the radio in the blob heap, for example
// a static array. It must be internal RAM. The heap owns mem after that.
func WithArenaMemory(mem []byte) Option {
	return func(o *options) { o.mem, o.size = mem, 0 }
}

// Limits of the dynamic RX buffers of ESP-IDF v5.3.2 esp_wifi/Kconfig
// (ESP_WIFI_DYNAMIC_RX_BUFFER_NUM). The minimum is the static RX buffer count.
const (
	DefaultRXBuffers = 32
	minRXBuffers     = 10
	maxRXBuffers     = 128
)

// WithRXBuffers sets the largest number of dynamic RX buffers of Wi-Fi, from
// 10 to 128. The default is DefaultRXBuffers. Each buffer takes up to 1.7 KB of
// the blob heap. A smaller number keeps memory free for the other users of the
// heap. A larger number helps with bursts of small frames, with a larger share.
func WithRXBuffers(n int) Option {
	return func(o *options) { o.rxBuffers = n }
}

// WithPSRAMArena gives PSRAM for the Wi-Fi buffers that need no DMA, for
// example a //go:section .psram array. Without it, all memory is internal.
func WithPSRAMArena(mem []byte) Option {
	return func(o *options) { o.psram = mem }
}

// wifiShare and bleShare tell whether the share of a radio is in the blob
// heap. A restarted radio uses its share again.
var wifiShare, bleShare bool

// initMemory adds the share of one radio to the blob heap once. size is the
// share without an arena option.
func initMemory(o options, share *bool) error {
	if *share {
		return nil
	}
	mem := o.mem
	if mem == nil {
		mem = make([]byte, o.size)
	}
	// The GC heap can be in PSRAM with gcshim_psram, so check the address.
	if !inRange(mem, dramLow, dramHigh) {
		return ErrInvalidArg
	}
	if o.psram != nil && (psramHeap != nil || !inRange(o.psram, extramDataLow, extramDataHi)) {
		return ErrInvalidArg
	}
	if blobHeap != nil {
		if err := blobHeap.Grow(mem); err != nil {
			return err
		}
	} else {
		h, err := arena.NewHeap(mem, arena.WithLocker(&memoryLock{}))
		if err != nil {
			return err
		}
		blobHeap = h
	}
	if o.psram != nil {
		ph, err := arena.NewHeap(o.psram, arena.WithLocker(&memoryLock{}))
		if err != nil {
			return err
		}
		psramHeap = ph
	}
	*share = true
	return nil
}

// radioStopped stops the timers and the power domain when no radio runs. The
// blob heap stays: the deinit of the blobs frees their blocks, but blob
// variables keep pointers into the heap, as in ESP-IDF, where the heap does
// not go away. A restart uses the same heap.
func radioStopped() {
	if wifiOn || ble.enabled.Load() {
		return
	}
	disarmTimers()
	coexReset()
	powerDomainOff()
}

func applyOptions(size int, opts []Option) options {
	o := options{size: size, rxBuffers: DefaultRXBuffers}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

func inRange(b []byte, low, high uintptr) bool {
	if len(b) == 0 {
		return false
	}
	start := uintptr(unsafe.Pointer(unsafe.SliceData(b)))
	return start >= low && start+uintptr(len(b)) <= high
}

// GrowArena adds n bytes from the GC heap to the internal blob heap. The heap
// cannot shrink, because the blobs keep pointers into it.
func GrowArena(n int) error {
	if blobHeap == nil {
		return ErrNotInit
	}
	mem := make([]byte, n)
	if !inRange(mem, dramLow, dramHigh) {
		return ErrInvalidArg
	}
	return blobHeap.Grow(mem)
}

// MemoryStats gives the counters of the internal blob heap.
func MemoryStats() arena.HeapStats {
	if blobHeap == nil {
		return arena.HeapStats{}
	}
	return blobHeap.Stats()
}

// PSRAMStats gives the counters of the PSRAM heap, or zeros without
// WithPSRAMArena.
func PSRAMStats() arena.HeapStats {
	if psramHeap == nil {
		return arena.HeapStats{}
	}
	return psramHeap.Stats()
}

// memoryLock is a sync.Locker for a blob heap. The blobs can allocate in an
// interrupt and on both cores.
type memoryLock struct {
	lock  spinLock
	state interrupt.State
}

func (l *memoryLock) Lock()   { l.state = l.lock.acquire() }
func (l *memoryLock) Unlock() { l.lock.release(l.state) }

// heapOf gives the heap that holds p.
func heapOf(p unsafe.Pointer) *arena.Heap {
	if psramHeap != nil && psramHeap.Contains(p) {
		return psramHeap
	}
	return blobHeap
}

// moveTo reallocates p into h, which is not the heap of p.
func moveTo(h *arena.Heap, p unsafe.Pointer, size uintptr) unsafe.Pointer {
	q := h.Alloc(size, 0)
	if q == nil {
		return nil
	}
	from := heapOf(p)
	n := min(size, from.Size(p))
	copy(unsafe.Slice((*byte)(q), n), unsafe.Slice((*byte)(p), n))
	from.Free(p)
	return q
}

// The C allocator functions give NULL before Init, as malloc does when it has
// no memory.

// Internal RAM: the *_internal entries, _malloc and the wrapped malloc.

//export tinygo_wifi_malloc
func blobMalloc(size uintptr) unsafe.Pointer {
	if blobHeap == nil {
		return nil
	}
	return blobHeap.Alloc(size, 0)
}

//export tinygo_wifi_calloc
func blobCalloc(n, size uintptr) unsafe.Pointer {
	if blobHeap == nil {
		return nil
	}
	return blobHeap.Calloc(n, size)
}

// blobRealloc keeps the block in internal RAM. A PSRAM block moves there,
// as heap_caps_realloc does for MALLOC_CAP_INTERNAL.
//
//export tinygo_wifi_realloc
func blobRealloc(p unsafe.Pointer, size uintptr) unsafe.Pointer {
	if blobHeap == nil {
		return nil
	}
	if p != nil && heapOf(p) != blobHeap {
		if size == 0 {
			blobFree(p)
			return nil
		}
		return moveTo(blobHeap, p, size)
	}
	return blobHeap.Realloc(p, size)
}

//export tinygo_wifi_free
func blobFree(p unsafe.Pointer) {
	if p != nil && blobHeap != nil {
		heapOf(p).Free(p)
	}
}

// PSRAM first, then internal RAM: the _wifi_malloc group, as
// heap_caps_malloc_prefer of wifi_malloc in esp_wifi/esp32s3/esp_adapter.c.

//export tinygo_wifi_ext_malloc
func extMalloc(size uintptr) unsafe.Pointer {
	if psramHeap != nil {
		if p := psramHeap.Alloc(size, 0); p != nil {
			return p
		}
	}
	return blobMalloc(size)
}

//export tinygo_wifi_ext_calloc
func extCalloc(n, size uintptr) unsafe.Pointer {
	if psramHeap != nil {
		if p := psramHeap.Calloc(n, size); p != nil {
			return p
		}
	}
	return blobCalloc(n, size)
}

//export tinygo_wifi_ext_realloc
func extRealloc(p unsafe.Pointer, size uintptr) unsafe.Pointer {
	if psramHeap == nil || blobHeap == nil {
		return blobRealloc(p, size)
	}
	if p == nil {
		return extMalloc(size)
	}
	if size == 0 {
		blobFree(p)
		return nil
	}
	h := heapOf(p)
	if q := h.Realloc(p, size); q != nil {
		return q
	}
	// The heap of p is full: move the block to the other heap.
	if h == psramHeap {
		return moveTo(blobHeap, p, size)
	}
	return moveTo(psramHeap, p, size)
}

//export tinygo_wifi_free_heap
func blobFreeHeap() uint32 { return uint32(MemoryStats().Free + PSRAMStats().Free) }

//export __wrap_malloc
func wrapMalloc(size uintptr) unsafe.Pointer { return blobMalloc(size) }

//export __wrap_calloc
func wrapCalloc(n, size uintptr) unsafe.Pointer { return blobCalloc(n, size) }

//export __wrap_realloc
func wrapRealloc(p unsafe.Pointer, size uintptr) unsafe.Pointer { return blobRealloc(p, size) }

//export __wrap_free
func wrapFree(p unsafe.Pointer) { blobFree(p) }
