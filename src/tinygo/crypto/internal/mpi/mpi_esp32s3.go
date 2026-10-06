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

// Mul sets z = x * y mod m. z can be x or y. MODMULT gives X * Y * Z / R^2
// mod M, so Z starts as R^2 mod M.
func (k *Modulus) Mul(z, x, y []uint32) {
	n := len(k.m)
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
		esp.RSA.MODE.Set(uint32(n - 1))
		for i, w := range k.m {
			mMem[i].Set(w)
		}
		esp.RSA.M_PRIME.Set(k.inv)
		current = k.id
	}
	for i := 0; i < n; i++ {
		xMem[i].Set(x[i])
		yMem[i].Set(y[i])
		zMem[i].Set(k.rr[i])
	}
	esp.RSA.CLEAR_INTERRUPT.Set(1)
	esp.RSA.MODMULT_START.Set(1)
	for esp.RSA.IDLE.Get() == 0 {
	}
	esp.RSA.CLEAR_INTERRUPT.Set(1)
	for i := 0; i < n; i++ {
		z[i] = zMem[i].Get()
	}
	mu.Unlock()
}
