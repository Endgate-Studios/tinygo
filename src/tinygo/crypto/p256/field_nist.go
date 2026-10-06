//go:build !esp32s3 && !p256mpi

package p256

// feMul sets r = a * b mod p.
func feMul(r, a, b *element) {
	var c [16]uint32
	for i := 0; i < 8; i++ {
		var carry uint64
		for j := 0; j < 8; j++ {
			v := uint64(a[i])*uint64(b[j]) + uint64(c[i+j]) + carry
			c[i+j], carry = uint32(v), v>>32
		}
		c[i+8] = uint32(carry)
	}
	reduce(r, &c)
	clear(c[:])
}

// reduce sets r = c mod p with the NIST fast reduction (FIPS 186-4 appendix D.2.3):
// s1 + 2s2 + 2s3 + s4 + s5 - s6 - s7 - s8 - s9, with c0 the least significant word.
func reduce(r *element, c *[16]uint32) {
	var w [16]int64
	for i := range w {
		w[i] = int64(c[i])
	}
	acc := [8]int64{
		w[0] + w[8] + w[9] - w[11] - w[12] - w[13] - w[14],
		w[1] + w[9] + w[10] - w[12] - w[13] - w[14] - w[15],
		w[2] + w[10] + w[11] - w[13] - w[14] - w[15],
		w[3] + 2*w[11] + 2*w[12] + w[13] - w[15] - w[8] - w[9],
		w[4] + 2*w[12] + 2*w[13] + w[14] - w[9] - w[10],
		w[5] + 2*w[13] + 2*w[14] + w[15] - w[10] - w[11],
		w[6] + 2*w[14] + 2*w[15] + w[14] + w[13] - w[8] - w[9],
		w[7] + 3*w[15] + w[8] - w[10] - w[11] - w[12] - w[13],
	}
	carry := propagate(r, &acc)
	// The top carry t stands for t*2^256 = t*(2^224 - 2^192 - 2^96 + 1) mod p. A fixed number of
	// folds keeps the timing independent of the value.
	for k := 0; k < 4; k++ {
		for i := range acc {
			acc[i] = int64(r[i])
		}
		acc[0] += carry
		acc[3] -= carry
		acc[6] -= carry
		acc[7] += carry
		carry = propagate(r, &acc)
	}
	subtractP(r, r, 0)
}

// propagate carries the signed limbs of acc into r and returns the signed carry out of the top.
func propagate(r *element, acc *[8]int64) int64 {
	var carry int64
	for i := range acc {
		v := acc[i] + carry
		r[i] = uint32(v)
		carry = v >> 32
	}
	return carry
}
