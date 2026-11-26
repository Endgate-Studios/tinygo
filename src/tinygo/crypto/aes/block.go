package aes

// The state is column major, byte 4*c+r is row r of column c (FIPS 197 section 3.4).

var sbox, invSbox [256]byte

// init builds the S-box from the multiplicative inverse in GF(2^8) and the affine transform
// (FIPS 197 section 5.1.1). p walks all units as powers of 3, and q walks their inverses.
func init() {
	p, q := byte(1), byte(1)
	for {
		p ^= p<<1 ^ reduce(p)
		q ^= q << 1
		q ^= q << 2
		q ^= q << 4
		if q&0x80 != 0 {
			q ^= 0x09
		}
		sbox[p] = q ^ rotl(q, 1) ^ rotl(q, 2) ^ rotl(q, 3) ^ rotl(q, 4) ^ 0x63
		if p == 1 {
			break
		}
	}
	sbox[0] = 0x63
	for i := range sbox {
		invSbox[sbox[i]] = byte(i)
	}
}

func reduce(b byte) byte {
	if b&0x80 != 0 {
		return 0x1b
	}
	return 0
}

func rotl(b byte, n uint) byte { return b<<n | b>>(8-n) }

func xtime(b byte) byte { return b<<1 ^ reduce(b) }

// gmul multiplies a and b in GF(2^8) (FIPS 197 section 4.2).
func gmul(a, b byte) byte {
	var p byte
	for b != 0 {
		if b&1 != 0 {
			p ^= a
		}
		a = xtime(a)
		b >>= 1
	}
	return p
}

// expandKey fills rk with the key schedule (FIPS 197 section 5.2).
func expandKey(key, rk []byte) {
	nk := len(key) / 4
	copy(rk, key)
	rcon := byte(1)
	for i := nk; i < len(rk)/4; i++ {
		var t [4]byte
		copy(t[:], rk[4*(i-1):4*i])
		switch {
		case i%nk == 0:
			t = [4]byte{sbox[t[1]] ^ rcon, sbox[t[2]], sbox[t[3]], sbox[t[0]]}
			rcon = xtime(rcon)
		case nk > 6 && i%nk == 4:
			for j := range t {
				t[j] = sbox[t[j]]
			}
		}
		for j := 0; j < 4; j++ {
			rk[4*i+j] = rk[4*(i-nk)+j] ^ t[j]
		}
	}
}

func addRoundKey(s *[BlockSize]byte, k []byte) {
	for i := range s {
		s[i] ^= k[i]
	}
}

func subBytes(s *[BlockSize]byte) {
	for i := range s {
		s[i] = sbox[s[i]]
	}
}

func invSubBytes(s *[BlockSize]byte) {
	for i := range s {
		s[i] = invSbox[s[i]]
	}
}

// shiftRows rotates row r left by r columns (FIPS 197 section 5.1.2).
func shiftRows(s *[BlockSize]byte) {
	t := *s
	for c := 0; c < 4; c++ {
		for r := 1; r < 4; r++ {
			s[4*c+r] = t[4*((c+r)%4)+r]
		}
	}
}

func invShiftRows(s *[BlockSize]byte) {
	t := *s
	for c := 0; c < 4; c++ {
		for r := 1; r < 4; r++ {
			s[4*((c+r)%4)+r] = t[4*c+r]
		}
	}
}

// mixColumns multiplies each column by {03}x^3 + {01}x^2 + {01}x + {02} (FIPS 197 section 5.1.3).
func mixColumns(s *[BlockSize]byte) {
	for c := 0; c < 16; c += 4 {
		a0, a1, a2, a3 := s[c], s[c+1], s[c+2], s[c+3]
		all := a0 ^ a1 ^ a2 ^ a3
		s[c] ^= all ^ xtime(a0^a1)
		s[c+1] ^= all ^ xtime(a1^a2)
		s[c+2] ^= all ^ xtime(a2^a3)
		s[c+3] ^= all ^ xtime(a3^a0)
	}
}

// invMixColumns multiplies each column by {0b}x^3 + {0d}x^2 + {09}x + {0e} (FIPS 197 section 5.3.3).
func invMixColumns(s *[BlockSize]byte) {
	for c := 0; c < 16; c += 4 {
		a0, a1, a2, a3 := s[c], s[c+1], s[c+2], s[c+3]
		s[c] = gmul(a0, 0x0e) ^ gmul(a1, 0x0b) ^ gmul(a2, 0x0d) ^ gmul(a3, 0x09)
		s[c+1] = gmul(a0, 0x09) ^ gmul(a1, 0x0e) ^ gmul(a2, 0x0b) ^ gmul(a3, 0x0d)
		s[c+2] = gmul(a0, 0x0d) ^ gmul(a1, 0x09) ^ gmul(a2, 0x0e) ^ gmul(a3, 0x0b)
		s[c+3] = gmul(a0, 0x0b) ^ gmul(a1, 0x0d) ^ gmul(a2, 0x09) ^ gmul(a3, 0x0e)
	}
}
