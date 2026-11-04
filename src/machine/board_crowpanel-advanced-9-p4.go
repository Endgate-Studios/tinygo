//go:build crowpanel_advanced_9_p4

// This file contains the pin mappings for the Elecrow CrowPanel Advanced 9 inch
// ESP32-P4 HMI display, board version 1.2. The pins are from the hardware manual
// in the Elecrow repository, in Eagle_SCH&PCB/1.2/readme.md, section 4.2.
//
// - https://elecrow.com/wiki/CrowPanel_Advanced_9inch_ESP32-P4_HMI_AI_Display_1024x600_IPS.html

package machine

// UART0 goes to the CH340K on the download port.
const (
	UART_TX_PIN = GPIO37
	UART_RX_PIN = GPIO38
)

// Expansion UART on connector J2.
const (
	UART1_TX_PIN = GPIO47
	UART1_RX_PIN = GPIO48
)

// I2C bus for the GT911 touch controller, the DHT20 sensor and connector J13.
const (
	SDA_PIN = GPIO45
	SCL_PIN = GPIO46
)

// GT911 touch controller.
const (
	TOUCH_RST_PIN = GPIO40
	TOUCH_INT_PIN = GPIO42
)

// LCD backlight. The panel itself uses the dedicated MIPI DSI pins.
const (
	LCD_BACKLIGHT_POWER_PIN = GPIO29
	LCD_BACKLIGHT_EN_PIN    = GPIO31
	LCD_RESET_PIN           = GPIO41
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
