//go:build !esp32s3

package mpi

// HardwareExp tells that Exp is faster than a loop of Mul.
const HardwareExp = false

// Mul sets z = x * y mod m. z can be x or y.
func (k *Modulus) Mul(z, x, y []uint32) { k.mulSoft(z, x, y) }

// Exp sets z = x^e mod m. e has the length of m and is public. z can be x.
func (k *Modulus) Exp(z, x, e []uint32) { k.expSoft(z, x, e) }
