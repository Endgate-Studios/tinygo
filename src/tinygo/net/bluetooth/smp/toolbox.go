package smp

import (
	"encoding/binary"

	"tinygo/crypto/aes"
)

// e is the security function e: AES-128 of one block (Core spec v5.4 Vol 3 Part H 2.2.1).
func e(mem Memory, key, in, out *[16]byte) error {
	if mem == nil {
		return ErrNilMemory
	}
	var c aes.Cipher
	if err := c.Init(mem, key[:]); err != nil {
		return ErrNoMemory
	}
	c.Encrypt(out, in)
	c.Close()
	return nil
}

// C1 is the LE legacy confirm value function c1 (2.2.3).
// preq and pres are the Pairing Request and Response PDUs, iat and rat the address types.
func C1(mem Memory, k, r *[16]byte, preq, pres *[7]byte, iat, rat uint8, ia, ra *[6]byte, out *[16]byte) error {
	var p [16]byte
	// p1 = pres || preq || rat' || iat', where rat' and iat' are 1 byte each.
	copy(p[0:7], pres[:])
	copy(p[7:14], preq[:])
	p[14], p[15] = rat&1, iat&1
	for i := range p {
		p[i] ^= r[i]
	}
	if err := e(mem, k, &p, &p); err != nil {
		return err
	}
	// p2 = padding || ia || ra
	var p2 [16]byte
	copy(p2[4:10], ia[:])
	copy(p2[10:16], ra[:])
	for i := range p {
		p[i] ^= p2[i]
	}
	err := e(mem, k, &p, out)
	clear(p[:])
	return err
}

// S1 is the LE legacy key generation function s1 (2.2.4). It takes the low 64 bits of r1 and r2.
func S1(mem Memory, k, r1, r2 *[16]byte, out *[16]byte) error {
	var r [16]byte
	copy(r[0:8], r1[8:16])
	copy(r[8:16], r2[8:16])
	err := e(mem, k, &r, out)
	clear(r[:])
	return err
}

// F4 is the LE Secure Connections confirm value function f4 (2.2.6): AES-CMAC_X(U || V || Z).
func F4(mem Memory, u, v *[32]byte, x *[16]byte, z uint8, out *[16]byte) error {
	var m cmac
	if err := m.init(mem, x); err != nil {
		return err
	}
	m.write(u[:])
	m.write(v[:])
	m.write([]byte{z})
	m.sum(out)
	return nil
}

// salt is SALT of f5 (2.2.7).
var salt = [16]byte{0x6c, 0x88, 0x83, 0x91, 0xaa, 0xf5, 0xa5, 0x38, 0x60, 0x37, 0x0b, 0xdb, 0x5a, 0x60, 0x83, 0xbe}

// F5 is the LE Secure Connections key generation function f5 (2.2.7). a1 and a2 are the address
// type followed by the 6-byte address. It returns the MacKey and the LTK.
func F5(mem Memory, w *[32]byte, n1, n2 *[16]byte, a1, a2 *[7]byte, macKey, ltk *[16]byte) error {
	var t [16]byte
	var m cmac
	if err := m.init(mem, &salt); err != nil {
		return err
	}
	m.write(w[:])
	m.sum(&t)
	// t is cleared on each return, not with defer: a deferred call moves t to the GC heap in TinyGo.
	for counter, out := range [2]*[16]byte{macKey, ltk} {
		if err := m.init(mem, &t); err != nil {
			clear(t[:])
			return err
		}
		m.write([]byte{byte(counter), 0x62, 0x74, 0x6c, 0x65}) // counter and keyID "btle"
		m.write(n1[:])
		m.write(n2[:])
		m.write(a1[:])
		m.write(a2[:])
		m.write([]byte{0x01, 0x00}) // Length = 256
		m.sum(out)
	}
	clear(t[:])
	return nil
}

// F6 is the LE Secure Connections check value function f6 (2.2.8).
func F6(mem Memory, w, n1, n2, r *[16]byte, ioCap *[3]byte, a1, a2 *[7]byte, out *[16]byte) error {
	var m cmac
	if err := m.init(mem, w); err != nil {
		return err
	}
	m.write(n1[:])
	m.write(n2[:])
	m.write(r[:])
	m.write(ioCap[:])
	m.write(a1[:])
	m.write(a2[:])
	m.sum(out)
	return nil
}

// G2 is the LE Secure Connections numeric comparison function g2 (2.2.9). It returns the 32-bit
// value. The 6-digit number to show is the value modulo 1000000.
func G2(mem Memory, u, v *[32]byte, x, y *[16]byte) (uint32, error) {
	var m cmac
	if err := m.init(mem, x); err != nil {
		return 0, err
	}
	m.write(u[:])
	m.write(v[:])
	m.write(y[:])
	var out [16]byte
	m.sum(&out)
	return binary.BigEndian.Uint32(out[12:]), nil
}
