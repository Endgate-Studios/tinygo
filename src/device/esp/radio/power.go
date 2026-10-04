//go:build esp32s3

package radio

import (
	"device/esp"
	"time"
)

// Register bits of ESP-IDF v5.3.2 soc/esp32s3/include/soc (rtc_cntl_reg.h
// and syscon_reg.h).
const (
	rtcWifiForcePD  = 1 << 17 // RTC_CNTL_WIFI_FORCE_PD in RTC_CNTL_DIG_PWC_REG
	rtcWifiForceISO = 1 << 28 // RTC_CNTL_WIFI_FORCE_ISO in RTC_CNTL_DIG_ISO_REG

	// SYSTEM_WIFI_CLK_WIFI_BT_COMMON_M in SYSCON_WIFI_CLK_EN_REG.
	wifiBTCommonClock = 0x78078f

	// MODEM_RESET_FIELD_WHEN_PU in SYSCON_WIFI_RST_EN_REG: the Wi-Fi baseband,
	// front end, Wi-Fi MAC and Bluetooth resets.
	modemResetWhenPowerUp = 1<<0 | 1<<1 | 1<<2 | 1<<3 | 1<<4 | 1<<9 | 1<<11 | 1<<13
)

var powerOn bool

// powerDomainOn switches on the Wi-Fi and Bluetooth power domain once, as
// ESP-IDF esp_wifi_bt_power_domain_on (components/esp_phy/src/phy_init.c).
func powerDomainOn() {
	if powerOn {
		return
	}
	powerOn = true
	esp.RTC_CNTL.DIG_PWC.ClearBits(rtcWifiForcePD)
	time.Sleep(10 * time.Microsecond)
	esp.APB_CTRL.WIFI_CLK_EN.SetBits(wifiBTCommonClock)
	esp.APB_CTRL.WIFI_RST_EN.SetBits(modemResetWhenPowerUp)
	esp.APB_CTRL.WIFI_RST_EN.ClearBits(modemResetWhenPowerUp)
	esp.RTC_CNTL.DIG_ISO.ClearBits(rtcWifiForceISO)
	esp.APB_CTRL.WIFI_CLK_EN.ClearBits(wifiBTCommonClock)
}
