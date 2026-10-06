//go:build esp32s3 || p256mpi

package p256

import "tinygo/crypto/internal/mpi"

// On the ESP32-S3 the RSA accelerator multiplies modulo p.

var (
	modP mpi.Modulus
	rrP  element
)

func init() { modP.Init(pElem[:], rrP[:]) }

// feMul sets r = a * b mod p.
func feMul(r, a, b *element) { modP.Mul(r[:], a[:], b[:]) }

// fePow sets r = a^e mod p. The exponent is public.
func fePow(r, a, e *element) { modP.Exp(r[:], a[:], e[:]) }
