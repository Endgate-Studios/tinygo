//go:build esp32s3

package radio

// The Bluetooth LE controller of ESP-IDF v5.3.2 (libbtdm_app.a) with an HCI
// host over VHCI. bt_seam.c has the OS table and the controller config.

/*
#cgo LDFLAGS: -Lblobs/esp32c3-bt-lib/esp32s3 -lbtdm_app -lbtbb
#include "seam.h"
*/
import "C"

import (
	"device/esp"
	"runtime"
	"runtime/interrupt"
	"strconv"
	"sync/atomic"
	"unsafe"
)

// The resets of PERIPH_BT_MODULE in SYSCON_WIFI_RST_EN_REG: BTBB, RW_BTMAC,
// RW_BTLP, RW_BTMAC_REG, RW_BTLP_REG and BTBB_REG (ESP-IDF v5.3.2
// hal/esp32s3/include/hal/clk_gate_ll.h and soc/syscon_reg.h).
const btReset = 1<<3 | 1<<9 | 1<<10 | 1<<11 | 1<<12 | 1<<13

// btClock is DPORT_WIFI_CLK_BT_EN_M of the ESP32 (0x61 << 11) in
// SYSCON_WIFI_CLK_EN_REG. The ESP-IDF v5.3.2 mask of the ESP32-S3 is 0, but
// ESP-IDF runs BLE with these bits set (the register reads 0xffffdfdf).
const btClock = 0x61 << 11

// ErrBLE is a failed controller call. The value is the result of the call.
type ErrBLE int32

func (e ErrBLE) Error() string { return "radio: BLE controller error " + strconv.Itoa(int(e)) }

var ble struct {
	enabled atomic.Bool
	recv    func(packet []byte)
	sendBuf [260]byte // an HCI packet: H4 type, header and up to 255 bytes
}

// BLEEnable starts the Bluetooth LE controller. recv gets each H4 packet from
// the controller in the controller task. It must copy the packet, for
// example with btctl.Controller.Deliver. BLE adds DefaultBLEArenaSize to the
// blob heap, or the share of an arena option.
func BLEEnable(recv func(packet []byte), opts ...Option) error {
	if ble.enabled.Load() {
		return nil
	}
	o := applyOptions(DefaultBLEArenaSize, opts)
	if err := initMemory(o, &bleShare); err != nil {
		return err
	}
	startService()
	powerDomainOn()
	ble.recv = recv

	// The Bluetooth MAC registers keep no value without these clock bits.
	esp.APB_CTRL.WIFI_CLK_EN.SetBits(btClock)
	if r := C.seam_coex_pre_init(); r != 0 {
		return ErrBLE(r)
	}
	act := o.bleActivities
	if act < 1 || act > 10 {
		act = 6
	}
	if r := C.seam_bt_init(C.uint8_t(act)); r != 0 {
		return ErrBLE(r)
	}
	phyEnable(modemBT)
	if r := C.seam_bt_enable(); r != 0 {
		phyDisable(modemBT)
		return ErrBLE(r)
	}
	if r := C.seam_bt_register_vhci(); r != 0 {
		return ErrBLE(r)
	}
	ble.enabled.Store(true)
	wakeService()
	return nil
}

// BLEDisable stops the Bluetooth LE controller, as esp_bt_controller_disable
// and esp_bt_controller_deinit. BLEEnable starts it again. When Wi-Fi is off
// too, the radio power domain goes off. The blob heap stays for the next start.
func BLEDisable() error {
	if !ble.enabled.Load() {
		return nil
	}
	ble.enabled.Store(false)
	C.seam_bt_disable()
	phyDisable(modemBT)
	C.seam_bt_deinit()
	// periph_module_disable(PERIPH_BT_MODULE) of bt_controller_deinit_internal.
	esp.APB_CTRL.WIFI_CLK_EN.ClearBits(btClock)
	esp.APB_CTRL.WIFI_RST_EN.SetBits(btReset)
	ble.recv = nil
	radioStopped()
	return nil
}

// WithBLEActivities sets the number of BLE activities, such as advertising
// sets, scans and connections, from 1 to 10 (ESP-IDF BT_CTRL_BLE_MAX_ACT). The
// default is 6. Each activity takes about 1 KB of the blob heap.
func WithBLEActivities(n int) Option {
	return func(o *options) { o.bleActivities = n }
}

// BLEDriver sends HCI packets to the controller. It has the methods of the
// Driver of tinygo/net/bluetooth/btctl.
type BLEDriver struct{}

// CanSend reports whether the controller takes a packet now.
func (BLEDriver) CanSend() bool {
	return ble.enabled.Load() && bool(C.seam_bt_can_send())
}

// Send sends one H4 packet. The controller copies it. Do not call it in an
// interrupt: it can wait for the controller.
func (BLEDriver) Send(packet []byte) {
	if len(packet) == 0 || len(packet) > len(ble.sendBuf) {
		return
	}
	// The library takes a pointer that is not const, so it gets a copy.
	n := copy(ble.sendBuf[:], packet)
	C.seam_bt_send((*C.uint8_t)(unsafe.Pointer(&ble.sendBuf[0])), C.uint16_t(n))
}

// btModuleReset is periph_module_reset(PERIPH_BT_MODULE). seam_bt_init calls
// it just before btdm_controller_init, as bt.c does.
//
//export tinygo_bt_module_reset
func btModuleReset() {
	esp.APB_CTRL.WIFI_RST_EN.SetBits(btReset)
	esp.APB_CTRL.WIFI_RST_EN.ClearBits(btReset)
}

// CanSend asks the controller each time, so this notice needs no action.
//
//export tinygo_bt_send_available
func btSendAvailable() {}

//export tinygo_bt_recv
func btRecv(data *byte, n uint16) int32 {
	if ble.recv != nil && n > 0 {
		ble.recv(unsafe.Slice(data, n))
	}
	return 0
}

//export tinygo_bt_task_yield
func btTaskYield() { runtime.Gosched() }

// The global interrupt disable of the controller is portENTER_CRITICAL on a
// spin lock in bt.c, so it also keeps out the other core. It can nest on one
// core. Only the outer call takes the lock and saves the state.
var btGlobal struct {
	lock  spinLock
	owner atomic.Uint32 // core + 1 of the holder, or 0
	depth int
	state interrupt.State
}

//export tinygo_bt_global_intr_disable
func btGlobalIntrDisable() {
	me := coreID() + 1
	if btGlobal.owner.Load() == me {
		btGlobal.depth++
		return
	}
	s := btGlobal.lock.acquire()
	btGlobal.owner.Store(me)
	btGlobal.state = s
	btGlobal.depth = 1
}

//export tinygo_bt_global_intr_restore
func btGlobalIntrRestore() {
	if btGlobal.owner.Load() != coreID()+1 {
		return
	}
	if btGlobal.depth--; btGlobal.depth == 0 {
		btGlobal.owner.Store(0)
		btGlobal.lock.release(btGlobal.state)
	}
}
