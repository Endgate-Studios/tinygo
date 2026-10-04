// Package att is the ATT protocol with a GATT server database (Core spec v5.4 Vol 3 Parts F and G).
// Declaration values, connection state and response buffers come from an injected allocator that can free.
package att

import (
	"encoding/binary"

	"runtime/memory"
)

// Memory is where the database and servers allocate their buffers.
type Memory interface {
	memory.Allocator
	memory.Freer
}

// ValueHandler serves the value of a characteristic that is not static.
// Read returns the number of bytes written into dst from offset, or an ATT error code.
type ValueHandler interface {
	ReadValue(handle, offset uint16, dst []byte) (int, uint8)
	WriteValue(handle uint16, value []byte) uint8
}

// Perm is what a client may do with an attribute.
type Perm uint8

const (
	PermRead  Perm = 1 << 0
	PermWrite Perm = 1 << 1
	// PermReadEncrypted and PermWriteEncrypted also need an encrypted link (Core spec v5.4 Vol 3 Part C 10.3).
	PermReadEncrypted  Perm = 1 << 2
	PermWriteEncrypted Perm = 1 << 3

	permEncrypted = PermReadEncrypted | PermWriteEncrypted
)

// Characteristic properties (Core spec v5.4 Vol 3 Part G 3.3.1.1).
const (
	PropRead        = 0x02
	PropWriteNoResp = 0x04
	PropWrite       = 0x08
	PropNotify      = 0x10
	PropIndicate    = 0x20
)

// Attribute is one entry of the database. The caller owns the table, so its Go pointers stay in
// memory that the GC scans.
type Attribute struct {
	Handle  uint16
	Type    UUID
	Perm    Perm
	Value   []byte       // static value, or a declaration value from the database memory
	Handler ValueHandler // serves the value when it is not nil
	cccd    int          // index into the CCCD state of each server, or -1
	owned   bool         // Value came from the database memory
}

// Error is a string error type, so error values are constants in flash and not in RAM.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrNilDependency Error = "att: memory, table and sender cannot be nil or empty"
	ErrFull          Error = "att: attribute table is full"
	ErrNoMemory      Error = "att: out of memory"
	ErrOrder         Error = "att: add a service before a characteristic, and a characteristic before a CCCD"
	ErrClosed        Error = "att: closed"
	ErrNotifyOff     Error = "att: the client did not enable notifications"
	ErrHandle        Error = "att: no characteristic value at this handle"
	ErrMTU           Error = "att: value does not fit in the MTU"
	ErrNotEncrypted  Error = "att: the value needs an encrypted link"
)

// DB is a GATT database over a caller table.
type DB struct {
	mem    Memory
	attrs  []Attribute
	n      int
	cccds  int
	closed bool
}

// NewDB returns an empty database that fills table.
func NewDB(mem Memory, table []Attribute) (*DB, error) {
	if mem == nil || len(table) == 0 {
		return nil, ErrNilDependency
	}
	return &DB{mem: mem, attrs: table}, nil
}

// Len returns the number of attributes.
func (db *DB) Len() int { return db.n }

// AddService adds a primary service declaration and returns its handle.
func (db *DB) AddService(uuid UUID) (uint16, error) {
	var v [16]byte
	n := uuid.put(v[:])
	return db.add(TypePrimaryService, PermRead, v[:n], nil)
}

// AddCharacteristic adds a characteristic declaration and its value, and returns the value handle.
// A nil handler serves the static value.
func (db *DB) AddCharacteristic(uuid UUID, props uint8, perm Perm, value []byte, handler ValueHandler) (uint16, error) {
	if db.n == 0 {
		return 0, ErrOrder
	}
	var decl [19]byte
	decl[0] = props
	binary.LittleEndian.PutUint16(decl[1:], uint16(db.n+2))
	n := 3 + uuid.put(decl[3:])
	if _, err := db.add(TypeCharacteristic, PermRead, decl[:n], nil); err != nil {
		return 0, err
	}
	h, err := db.add(uuid, perm, nil, handler)
	if err != nil {
		return 0, err
	}
	db.attrs[db.n-1].Value = value
	return h, nil
}

// AddCCCD adds a Client Characteristic Configuration descriptor for the last characteristic.
// It needs encryption when the characteristic value needs it.
func (db *DB) AddCCCD() (uint16, error) {
	if db.n < 2 || db.attrs[db.n-2].Type != TypeCharacteristic {
		return 0, ErrOrder
	}
	h, err := db.add(TypeCCCD, PermRead|PermWrite|db.attrs[db.n-1].Perm&permEncrypted, nil, nil)
	if err != nil {
		return 0, err
	}
	db.attrs[db.n-1].cccd = db.cccds
	db.cccds++
	return h, nil
}

// Close frees the declaration values.
func (db *DB) Close() {
	for i := 0; i < db.n; i++ {
		if a := &db.attrs[i]; a.owned {
			clear(a.Value)
			memory.FreeSlice(db.mem, a.Value)
			a.Value, a.owned = nil, false
		}
	}
	db.closed = true
}

// add appends an attribute. A non-nil copyValue is copied into database memory.
func (db *DB) add(typ UUID, perm Perm, copyValue []byte, handler ValueHandler) (uint16, error) {
	if db.closed {
		return 0, ErrClosed
	}
	if db.n == len(db.attrs) {
		return 0, ErrFull
	}
	a := Attribute{Handle: uint16(db.n + 1), Type: typ, Perm: perm, Handler: handler, cccd: -1}
	if copyValue != nil {
		v := memory.MakeSlice[byte](db.mem, len(copyValue))
		if v == nil {
			return 0, ErrNoMemory
		}
		copy(v, copyValue)
		a.Value, a.owned = v, true
	}
	db.attrs[db.n] = a
	db.n++
	return a.Handle, nil
}

// get returns the attribute with handle h, or nil.
func (db *DB) get(h uint16) *Attribute {
	if h == 0 || int(h) > db.n {
		return nil
	}
	return &db.attrs[h-1]
}

// groupEnd returns the last handle of the service that starts at handle h.
func (db *DB) groupEnd(h uint16) uint16 {
	for i := int(h); i < db.n; i++ {
		if db.attrs[i].Type == TypePrimaryService {
			return uint16(i)
		}
	}
	return uint16(db.n)
}
