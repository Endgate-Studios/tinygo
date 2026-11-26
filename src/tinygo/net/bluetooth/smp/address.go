package smp

import "io"

// AH is the random address hash function ah (Core spec v5.4 Vol 3 Part H 2.2.2). k is in spec
// order, and r and the result are the low 24 bits.
func AH(mem Memory, k *[16]byte, r uint32) (uint32, error) {
	var in, out [16]byte
	in[13], in[14], in[15] = byte(r>>16), byte(r>>8), byte(r)
	if err := e(mem, k, &in, &out); err != nil {
		return 0, err
	}
	return uint32(out[13])<<16 | uint32(out[14])<<8 | uint32(out[15]), nil
}

// ResolveAddress reports whether addr is a resolvable private address made with irk (Core spec
// v5.4 Vol 6 Part B 1.3.2.3). irk and addr are in wire order, as in a Bond and in HCI events.
func ResolveAddress(mem Memory, irk *[16]byte, addr *[6]byte) bool {
	if addr[5]>>6 != 0b01 {
		return false
	}
	var k [16]byte
	reverse(k[:], irk[:])
	prand := uint32(addr[5])<<16 | uint32(addr[4])<<8 | uint32(addr[3])
	hash, err := AH(mem, &k, prand)
	clear(k[:])
	return err == nil && hash == uint32(addr[2])<<16|uint32(addr[1])<<8|uint32(addr[0])
}

// NewRPA makes a resolvable private address from irk in wire order (Core spec v5.4 Vol 6 Part B
// 1.3.2.2). The random part of prand is not all zeros and not all ones.
func NewRPA(mem Memory, irk *[16]byte, rng io.Reader, addr *[6]byte) error {
	var k [16]byte
	reverse(k[:], irk[:])
	err := makeRPA(mem, &k, rng, addr)
	clear(k[:])
	return err
}

func makeRPA(mem Memory, k *[16]byte, rng io.Reader, addr *[6]byte) error {
	for tries := 0; tries < 16; tries++ {
		if _, err := io.ReadFull(rng, addr[3:6]); err != nil {
			return ErrRandom
		}
		addr[5] = addr[5]&0x3f | 0x40
		random := uint32(addr[5]&0x3f)<<16 | uint32(addr[4])<<8 | uint32(addr[3])
		if random == 0 || random == 0x3fffff {
			continue
		}
		hash, err := AH(mem, k, 0x400000|random)
		if err != nil {
			return err
		}
		addr[0], addr[1], addr[2] = byte(hash), byte(hash>>8), byte(hash>>16)
		return nil
	}
	return ErrRandom
}

// BondMatches reports whether bond b belongs to a peer that now uses addr. pairedFrom is the
// address that the peer paired from. With an IRK the peer is known by its identity address or by
// a resolvable private address, and without one by pairedFrom.
func BondMatches(mem Memory, b *Bond, pairedFrom, addr Address) bool {
	if !b.HasPeerIRK {
		return pairedFrom == addr
	}
	return b.PeerIdentity == addr || addr.Type == 1 && ResolveAddress(mem, &b.PeerIRK, &addr.Addr)
}
