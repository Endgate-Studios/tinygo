//go:build esp32s3

package mpi

import (
	"device/esp"
	"runtime/volatile"
	"sync"
	"unsafe"
)

// The RSA accelerator, as mpi_ll.h, mpi_hal.c and bignum_alt.c of ESP-IDF
// v5.3.2 for the ESP32-S3.

var (
	mu      sync.Mutex
	on      bool
	current uint32 // the id of the modulus in M_MEM
	mMem    = (*[MaxWords]volatile.Register32)(unsafe.Pointer(&esp.RSA.M_MEM[0]))
	zMem    = (*[MaxWords]volatile.Register32)(unsafe.Pointer(&esp.RSA.Z_MEM[0]))
	yMem    = (*[MaxWords]volatile.Register32)(unsafe.Pointer(&esp.RSA.Y_MEM[0]))
	xMem    = (*[MaxWords]volatile.Register32)(unsafe.Pointer(&esp.RSA.X_MEM[0]))
)

// HardwareExp tells that Exp is faster than a loop of Mul.
const HardwareExp = true

// Exp sets z = x^e mod m. e has the length of m and is public. z can be x.
// MODEXP gives X^Y mod M with Z = R^2 mod M at the start. With
// CONSTANT_TIME set, it does not skip the zero bits of Y.
func (k *Modulus) Exp(z, x, e []uint32) {
	n := len(k.m)
	k.start()
	for i := 0; i < n; i++ {
		xMem[i].Set(x[i])
		yMem[i].Set(e[i])
		zMem[i].Set(k.rr[i])
	}
	esp.RSA.CONSTANT_TIME.Set(1)
	esp.RSA.SEARCH_ENABLE.Set(0)
	run(&esp.RSA.MODEXP_START)
	for i := 0; i < n; i++ {
		z[i] = zMem[i].Get()
	}
	mu.Unlock()
}

// Mul sets z = x * y mod m. z can be x or y. MODMULT gives X * Y * Z / R^2
// mod M, so Z starts as R^2 mod M.
func (k *Modulus) Mul(z, x, y []uint32) {
	n := len(k.m)
	k.start()
	for i := 0; i < n; i++ {
		xMem[i].Set(x[i])
		yMem[i].Set(y[i])
		zMem[i].Set(k.rr[i])
	}
	run(&esp.RSA.MODMULT_START)
	for i := 0; i < n; i++ {
		z[i] = zMem[i].Get()
	}
	mu.Unlock()
}

// start takes the lock, powers the accelerator on and loads the modulus.
func (k *Modulus) start() {
	mu.Lock()
	if !on {
		esp.SYSTEM.PERIP_CLK_EN1.SetBits(esp.SYSTEM_PERIP_CLK_EN1_CRYPTO_RSA_CLK_EN)
		esp.SYSTEM.PERIP_RST_EN1.SetBits(esp.SYSTEM_PERIP_RST_EN1_CRYPTO_RSA_RST)
		// mpi_ll_reset_register: DS in reset also holds RSA in reset.
		esp.SYSTEM.PERIP_RST_EN1.ClearBits(esp.SYSTEM_PERIP_RST_EN1_CRYPTO_RSA_RST |
			esp.SYSTEM_PERIP_RST_EN1_CRYPTO_DS_RST)
		esp.SYSTEM.RSA_PD_CTRL.ClearBits(1) // RSA_MEM_PD
		for esp.RSA.CLEAN.Get() == 0 {
		}
		esp.RSA.INTERRUPT_ENA.Set(0)
		on = true
	}
	if current != k.id {
		esp.RSA.MODE.Set(uint32(len(k.m) - 1))
		for i, w := range k.m {
			mMem[i].Set(w)
		}
		esp.RSA.M_PRIME.Set(k.inv)
		current = k.id
	}
}

// run starts an operation and waits until it ends.
func run(startReg *volatile.Register32) {
	esp.RSA.CLEAR_INTERRUPT.Set(1)
	startReg.Set(1)
	for esp.RSA.IDLE.Get() == 0 {
	}
	esp.RSA.CLEAR_INTERRUPT.Set(1)
}
