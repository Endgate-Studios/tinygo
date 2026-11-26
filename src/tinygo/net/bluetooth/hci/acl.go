package hci

import "encoding/binary"

// Packet boundary flags of an ACL packet (Core spec v5.4 Vol 4 Part E 5.4.2).
const (
	PBFirstNonFlushable = 0x0 // host to controller, first fragment
	PBContinuing        = 0x1
	PBFirstFlushable    = 0x2 // controller to host, first fragment
)

// ACL is a parsed ACL data packet. Data is a view into the packet.
type ACL struct {
	Handle uint16
	PB     uint8
	BC     uint8
	Data   []byte
}

// PutACL writes an H4 ACL data packet and returns its length.
func PutACL(dst []byte, handle uint16, pb uint8, data []byte) (int, error) {
	if len(data) > 0xffff {
		return 0, ErrParams
	}
	n := 5 + len(data)
	if len(dst) < n {
		return 0, ErrBuffer
	}
	dst[0] = TypeACL
	binary.LittleEndian.PutUint16(dst[1:], handle&0x0fff|uint16(pb&3)<<12)
	binary.LittleEndian.PutUint16(dst[3:], uint16(len(data)))
	copy(dst[5:], data)
	return n, nil
}

// ParseACL decodes an H4 ACL data packet.
func ParseACL(b []byte) (ACL, error) {
	if len(b) < 5 {
		return ACL{}, ErrShort
	}
	if b[0] != TypeACL {
		return ACL{}, ErrType
	}
	hf := binary.LittleEndian.Uint16(b[1:])
	n := int(binary.LittleEndian.Uint16(b[3:]))
	if len(b) < 5+n {
		return ACL{}, ErrShort
	}
	return ACL{Handle: hf & 0x0fff, PB: uint8(hf>>12) & 3, BC: uint8(hf >> 14), Data: b[5 : 5+n]}, nil
}

// Flow tracks the credits of the controller: how many commands it accepts now, and how many
// ACL packets fit in its buffers (Core spec v5.4 Vol 4 Part E 4.4 and 4.1.1).
type Flow struct {
	commands uint8
	acl      uint16
}

// NewFlow starts with one command credit, as after reset, and aclBuffers from LE Read Buffer Size.
func NewFlow(aclBuffers uint16) Flow { return Flow{commands: 1, acl: aclBuffers} }

// CanSendCommand reports whether the controller accepts a command now, and TookCommand counts one.
func (f *Flow) CanSendCommand() bool { return f.commands > 0 }
func (f *Flow) TookCommand()         { f.commands-- }

// CanSendACL reports whether an ACL buffer is free, and TookACL counts one.
func (f *Flow) CanSendACL() bool { return f.acl > 0 }
func (f *Flow) TookACL()         { f.acl-- }

// Update takes the credits from Command Complete, Command Status and Number Of Completed Packets events.
func (f *Flow) Update(e Event) {
	if cc, ok := e.CommandComplete(); ok {
		f.commands = cc.NumPackets
		return
	}
	if cs, ok := e.CommandStatus(); ok {
		f.commands = cs.NumPackets
		return
	}
	if it, ok := e.CompletedPackets(); ok {
		for {
			_, count, ok := it.Next()
			if !ok {
				return
			}
			f.acl += count
		}
	}
}
