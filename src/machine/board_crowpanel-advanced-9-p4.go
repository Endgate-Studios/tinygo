//go:build crowpanel_advanced_9_p4

// This file contains the pin mappings for the Elecrow CrowPanel Advanced 9 inch
// ESP32-P4 HMI display, board version 1.2. The pins are from the schematic in the
// Elecrow repository, Eagle_SCH&PCB/1.2/ESP32-P4 Display 9.0 inch V1.2.sch.
//
// Board version 1.0 has the SDIO data lines in the reverse order.
//
// - https://elecrow.com/wiki/CrowPanel_Advanced_9inch_ESP32-P4_HMI_AI_Display_1024x600_IPS.html

package machine

// VO4 is the only supply of the GPIO39 to GPIO48 pads, through R109 in the
// schematic. See the ESP32-P4 datasheet, table 2-1, for the pads on VDD_IO_5.
func init() {
	enableVO4()
}

// BOOT button K3. It is also the strapping pin for download mode at reset.
const BUTTON = GPIO35

// UART0 goes to the CH340K on the download port J1.
const (
	UART_TX_PIN = GPIO37
	UART_RX_PIN = GPIO38
)

// Expansion UART on connector J2.
const (
	UART1_TX_PIN = GPIO47
	UART1_RX_PIN = GPIO48
)

// UART on connector J10, through level shifters.
const (
	UART3_TX_PIN = GPIO34
	UART3_RX_PIN = GPIO33
)

// I2C bus for the GT911 touch controller and connector J13. Level shifters
// connect it to the audio chips and to the STC8H1K08 power management MCU.
const (
	SDA_PIN = GPIO45
	SCL_PIN = GPIO46
)

// GT911 touch controller.
const (
	TOUCH_RST_PIN = GPIO40
	TOUCH_INT_PIN = GPIO42
)

// LCD control. The panel itself uses the dedicated MIPI DSI pins.
const (
	LCD_BACKLIGHT_EN_PIN = GPIO31
	LCD_RESET_PIN        = GPIO41
)

// ESP32-C6 Wi-Fi coprocessor, on a 4 bit SDIO bus.
const (
	C6_EN_PIN       = GPIO32
	C6_SDIO_D3_PIN  = GPIO14
	C6_SDIO_D2_PIN  = GPIO15
	C6_SDIO_D1_PIN  = GPIO16
	C6_SDIO_D0_PIN  = GPIO17
	C6_SDIO_CLK_PIN = GPIO18
	C6_SDIO_CMD_PIN = GPIO19
)

// MicroSD card, in 1 bit mode.
const (
	SD_D0_PIN  = GPIO39
	SD_CLK_PIN = GPIO43
	SD_CMD_PIN = GPIO44
)

// Audio.
const (
	I2S_WS_PIN       = GPIO21
	I2S_SCK_PIN      = GPIO22
	I2S_SDO_PIN      = GPIO23
	AMP_CONTROL_PIN  = GPIO30
	PDM_MIC_CLK_PIN  = GPIO24
	PDM_MIC_DATA_PIN = GPIO26
)

// Wireless module socket, on connectors J9 and J11.
const (
	WIRELESS_MOSI_PIN = GPIO6
	WIRELESS_MISO_PIN = GPIO7
	WIRELESS_SCK_PIN  = GPIO8
)
