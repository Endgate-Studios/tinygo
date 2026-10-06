// Package mpi multiplies numbers modulo an odd modulus. On the ESP32-S3 the
// RSA accelerator does it, and on other targets Montgomery multiplication in
// software. Its time does not depend on the values. The numbers are 32-bit
// words, the least significant word first, and always below the modulus.
package mpi

// MaxWords is the largest modulus, 4096 bits (ESP-IDF soc_caps.h,
// SOC_RSA_MAX_BIT_LEN).
const MaxWords = 128

// Modulus is an odd modulus. Use it as a value after Init.
type Modulus struct {
	m   []uint32
	rr  []uint32 // R^2 mod m, with R = 2^(32 len(m))
	inv uint32   // -m^-1 mod 2^32
	id  uint32   // tells the accelerator that it must load another modulus
}

var lastID uint32

// Init sets the modulus to m, which is odd, above 1 and has a top word that
// is not 0. rr is storage for len(m) words. m and rr must stay.
func (k *Modulus) Init(m, rr []uint32) bool {
	n := len(m)
	if n == 0 || n > MaxWords || len(rr) != n || m[0]&1 == 0 || m[n-1] == 0 || (n == 1 && m[0] == 1) {
		return false
	}
	y := m[0]
	for i := 0; i < 5; i++ {
		y *= 2 - m[0]*y
	}
	lastID++
	*k = Modulus{m: m, rr: rr, inv: -y, id: lastID}
	// R^2 mod m by doubling. m is public, so this need not be constant time.
	clear(rr)
	rr[0] = 1
	for i := 0; i < 64*n; i++ {
		top := rr[n-1] >> 31
		for j := n - 1; j > 0; j-- {
			rr[j] = rr[j]<<1 | rr[j-1]>>31
		}
		rr[0] <<= 1
		if top == 1 || !less(rr, m) {
			sub(rr, rr, m)
		}
	}
	return true
}

// Words gives the length of the modulus in words.
func (k *Modulus) Words() int { return len(k.m) }

// mulSoft sets z = x * y mod m with two Montgomery multiplications: x*y/R,
// then times R^2/R.
func (k *Modulus) mulSoft(z, x, y []uint32) {
	var t [MaxWords]uint32
	n := len(k.m)
	k.mont(t[:n], x, y)
	k.mont(z, t[:n], k.rr)
	clear(t[:n])
}

// mont sets z = x * y / R mod m (Handbook of Applied Cryptography, algorithm
// 14.36). z can be x or y.
func (k *Modulus) mont(z, x, y []uint32) {
	var tb [MaxWords + 2]uint32
	m := k.m
	l := len(m)
	t := tb[:l+2]
	for i := 0; i < l; i++ {
		var c uint64
		for j := 0; j < l; j++ {
			v := uint64(t[j]) + uint64(x[i])*uint64(y[j]) + c
			t[j], c = uint32(v), v>>32
		}
		v := uint64(t[l]) + c
		t[l], t[l+1] = uint32(v), uint32(v>>32)
		u := t[0] * k.inv
		c = (uint64(t[0]) + uint64(u)*uint64(m[0])) >> 32
		for j := 1; j < l; j++ {
			v := uint64(t[j]) + uint64(u)*uint64(m[j]) + c
			t[j-1], c = uint32(v), v>>32
		}
		v = uint64(t[l]) + c
		t[l-1] = uint32(v)
		t[l] = t[l+1] + uint32(v>>32)
	}
	// t < 2m: subtract m when t[l] is set or t >= m, with a mask.
	var borrow uint64
	for j := 0; j < l; j++ {
		v := uint64(t[j]) - uint64(m[j]) - borrow
		z[j], borrow = uint32(v), (v>>32)&1
	}
	keep := -(uint32(borrow) &^ t[l]) // all ones when t < m
	for j := 0; j < l; j++ {
		z[j] = t[j]&keep | z[j]&^keep
	}
	clear(t)
}

func less(a, b []uint32) bool {
	for i := len(a) - 1; i >= 0; i-- {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

func sub(z, a, b []uint32) {
	var borrow uint64
	for i := range z {
		v := uint64(a[i]) - uint64(b[i]) - borrow
		z[i], borrow = uint32(v), (v>>32)&1
	}
}
