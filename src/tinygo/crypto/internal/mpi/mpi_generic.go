//go:build !esp32s3

package mpi

// Mul sets z = x * y mod m. z can be x or y.
func (k *Modulus) Mul(z, x, y []uint32) { k.mulSoft(z, x, y) }
