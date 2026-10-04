//go:build esp32s3

package radio

/*
#include "seam.h"
*/
import "C"

import (
	"device/esp"
	"runtime/interrupt"
	"runtime/memory"
	"sync"
	"time"
	"unsafe"
)

// SYSTEM_WIFIMAC_RST in SYSCON_WIFI_RST_EN_REG, ESP-IDF v5.3.2
// soc/esp32s3/include/soc/syscon_reg.h.
const wifiMACReset = 1 << 2

// wifiClock is bit 6 of SYSCON_WIFI_CLK_EN_REG. Without it, hal_init of the
// blobs waits for the MAC forever.
const wifiClock = 1 << 6

// pllTrackPeriod is PHY_TRACK_PLL_PERIOD_IN_US of ESP-IDF v5.3.2
// components/esp_phy/src/phy_common.c.
const pllTrackPeriod = time.Second

// phy is the state of esp_phy_enable and esp_phy_disable of ESP-IDF v5.3.2
// components/esp_phy/src/phy_init.c, for Wi-Fi only.
var phy struct {
	mu         sync.Mutex
	modems     uint8 // the users of the PHY: modemWifi and modemBT
	calibrated bool
	lastTrack  time.Time
}

// The modems of esp_phy_modem_t in ESP-IDF v5.3.2 esp_phy_init.h. Wi-Fi and
// Bluetooth share the PHY: it starts with the first user and stops after the
// last one.
const (
	modemWifi = 1 << 0 // PHY_MODEM_WIFI
	modemBT   = 1 << 1 // PHY_MODEM_BT
)

// The comment of SYSTEM_WIFI_CLK_WIFI_EN in ESP-IDF v5.3.2 syscon_reg.h names
// bit 6 as the Wi-Fi clock, but the ESP-IDF mask is 0.
//
//export tinygo_wifi_clock_enable
func osiWifiClockEnable() {
	esp.APB_CTRL.WIFI_CLK_EN.SetBits(wifiClock)
}

// The blobs call it for modem sleep. ESP-IDF clears no bits, and a MAC
// without its clock cannot wake up, so the bit stays set.
//
//export tinygo_wifi_clock_disable
func osiWifiClockDisable() {}

// osiWifiResetMAC is periph_module_reset(PERIPH_WIFI_MODULE).
//
//export tinygo_wifi_reset_mac
func osiWifiResetMAC() {
	esp.APB_CTRL.WIFI_RST_EN.SetBits(wifiMACReset)
	esp.APB_CTRL.WIFI_RST_EN.ClearBits(wifiMACReset)
}

//export tinygo_wifi_phy_enable
func osiPhyEnable() { phyEnable(modemWifi) }

//export tinygo_wifi_phy_disable
func osiPhyDisable() { phyDisable(modemWifi) }

// phyEnable is esp_phy_enable of ESP-IDF v5.3.2 phy_init.c.
func phyEnable(modem uint8) {
	phy.mu.Lock()
	defer phy.mu.Unlock()
	if phy.modems == 0 {
		esp.APB_CTRL.WIFI_CLK_EN.SetBits(wifiBTCommonClock)
		if phy.calibrated {
			C.phy_wakeup_init()
		} else {
			calibratePhy()
			phy.calibrated = true
		}
	}
	phy.modems |= modem
	if time.Since(phy.lastTrack) > pllTrackPeriod {
		trackPLL()
	}
}

// phyDisable is esp_phy_disable: the PHY stops when the last user leaves.
func phyDisable(modem uint8) {
	phy.mu.Lock()
	defer phy.mu.Unlock()
	if phy.modems&modem == 0 {
		return
	}
	phy.modems &^= modem
	if phy.modems == 0 {
		C.phy_close_rf()
		C.phy_xpd_tsens()
		esp.APB_CTRL.WIFI_CLK_EN.ClearBits(wifiBTCommonClock)
	}
}

// ESP-IDF returns ESP_OK without CONFIG_ESP_PHY_MULTIPLE_INIT_DATA_BIN.
//
//export tinygo_wifi_phy_update_country_info
func osiPhyUpdateCountryInfo(country *byte) int32 {
	return 0
}

// calibratePhy does a full RF calibration, as esp_phy_load_cal_and_init
// without NVS. libphy keeps a copy of the calibration data.
func calibratePhy() {
	println("wifi: phy version", C.GoString(C.get_phy_version_str()))
	cal := memory.New[C.esp_phy_calibration_data_t](blobHeap)
	if cal == nil {
		panic("wifi: no memory for the PHY calibration")
	}
	mac := MAC()
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&cal.mac[0])), len(mac)), mac[:])
	C.register_chipv7_phy(&C.seam_phy_init_data, cal, C.PHY_RF_CAL_FULL)
	memory.Free(blobHeap, cal)
}

func trackPLL() {
	phy.lastTrack = time.Now()
	C.phy_param_track_tot(phy.modems&modemWifi != 0, phy.modems&modemBT != 0)
}

// runPLLTrack is phy_track_pll_timer_callback of phy_common.c. The service
// goroutine calls it often, and it tracks once per pllTrackPeriod.
func runPLLTrack() {
	phy.mu.Lock()
	if phy.modems != 0 && time.Since(phy.lastTrack) >= pllTrackPeriod {
		trackPLL()
	}
	phy.mu.Unlock()
}

// libphy calls these around its register access. They disable the
// interrupts of this core only, the same as portENTER_CRITICAL on one core.
//
//export phy_enter_critical
func phyEnterCritical() uint32 {
	return uint32(interrupt.Disable())
}

//export phy_exit_critical
func phyExitCritical(level uint32) {
	interrupt.Restore(interrupt.State(level))
}
