// Package btctl connects a BLE controller that speaks HCI, such as the Espressif controller
// library, to the Go host packages. Received packets wait in a ring from the injected memory.
package btctl

import (
	"sync"

	"runtime/memory"
)

// Memory is where the receive ring comes from.
type Memory interface {
	memory.Allocator
	memory.Freer
}

// Driver sends H4 packets to the controller. On the ESP32-S3 it wraps esp_vhci_host_send_packet
// and esp_vhci_host_check_send_available (ESP-IDF esp_bt.h).
type Driver interface {
	CanSend() bool
	Send(packet []byte)
}

// Receiver takes H4 packets from the controller, for example peripheral.Peripheral.HandleHCI.
type Receiver interface {
	HandleHCI(packet []byte, now uint32) error
}

// Error is a string error type, so error values are constants in flash and not in RAM.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrNilDependency Error = "btctl: memory, driver and lock cannot be nil"
	ErrNoMemory      Error = "btctl: out of memory"
	ErrBusy          Error = "btctl: controller cannot take a packet now"
	ErrTooLong       Error = "btctl: packet is longer than the receive ring allows"
	ErrClosed        Error = "btctl: closed"
)

// lenBytes is the length in front of each packet in the ring. A packet must fit in a quarter of the ring.
const lenBytes = 2

// Controller is one HCI link to a controller.
type Controller struct {
	drv     Driver
	lock    sync.Locker
	mem     Memory
	ring    []byte
	head    int // next byte to read
	used    int
	dropped uint32
	pkt     []byte // one packet for Poll, from the injected memory
	closed  bool
}

// New returns a controller link with a receive ring of ringSize bytes. lock protects the ring,
// because the controller delivers packets from its own task. On bare metal it can disable interrupts.
func New(mem Memory, drv Driver, lock sync.Locker, ringSize int) (*Controller, error) {
	if mem == nil || drv == nil || lock == nil {
		return nil, ErrNilDependency
	}
	c := &Controller{drv: drv, lock: lock, mem: mem}
	if c.ring = memory.MakeSlice[byte](mem, ringSize); c.ring == nil {
		return nil, ErrNoMemory
	}
	if c.pkt = memory.MakeSlice[byte](mem, ringSize/4); c.pkt == nil {
		memory.FreeSlice(mem, c.ring)
		return nil, ErrNoMemory
	}
	return c, nil
}

// WriteHCI sends one H4 packet. It satisfies peripheral.Transport.
func (c *Controller) WriteHCI(packet []byte) error {
	if c.closed {
		return ErrClosed
	}
	if !c.drv.CanSend() {
		return ErrBusy
	}
	c.drv.Send(packet)
	return nil
}

// Deliver copies one packet from the controller into the ring. The controller callback calls it.
// It reports false and counts a drop when the ring is full or the packet is too long.
func (c *Controller) Deliver(packet []byte) bool {
	c.lock.Lock()
	n := len(packet)
	if c.closed || n == 0 || n > len(c.pkt) || c.used+lenBytes+n > len(c.ring) {
		c.dropped++
		c.lock.Unlock()
		return false
	}
	tail := (c.head + c.used) % len(c.ring)
	c.put(tail, byte(n))
	c.put(tail+1, byte(n>>8))
	for i := 0; i < n; i++ {
		c.put(tail+lenBytes+i, packet[i])
	}
	c.used += lenBytes + n
	c.lock.Unlock()
	return true
}

// Poll gives each waiting packet to r, oldest first, and returns the first error of r.
// The lock is not held while r runs.
func (c *Controller) Poll(r Receiver, now uint32) error {
	var first error
	for {
		n, ok := c.take()
		if !ok {
			return first
		}
		if err := r.HandleHCI(c.pkt[:n], now); err != nil && first == nil {
			first = err
		}
		clear(c.pkt[:n])
	}
}

// Dropped returns how many packets did not fit in the ring.
func (c *Controller) Dropped() uint32 {
	c.lock.Lock()
	d := c.dropped
	c.lock.Unlock()
	return d
}

// Close frees the ring. The controller must not call Deliver after Close returns.
func (c *Controller) Close() {
	c.lock.Lock()
	if !c.closed {
		clear(c.ring)
		memory.FreeSlice(c.mem, c.ring)
		memory.FreeSlice(c.mem, c.pkt)
		c.ring, c.pkt, c.used, c.closed = nil, nil, 0, true
	}
	c.lock.Unlock()
}

// take copies the oldest packet into c.pkt and returns its length.
func (c *Controller) take() (int, bool) {
	c.lock.Lock()
	if c.closed || c.used == 0 {
		c.lock.Unlock()
		return 0, false
	}
	n := int(c.get(c.head)) | int(c.get(c.head+1))<<8
	for i := 0; i < n; i++ {
		c.pkt[i] = c.get(c.head + lenBytes + i)
	}
	for i := 0; i < lenBytes+n; i++ {
		c.put(c.head+i, 0)
	}
	c.head = (c.head + lenBytes + n) % len(c.ring)
	c.used -= lenBytes + n
	c.lock.Unlock()
	return n, true
}

func (c *Controller) put(i int, b byte) { c.ring[i%len(c.ring)] = b }
func (c *Controller) get(i int) byte    { return c.ring[i%len(c.ring)] }
