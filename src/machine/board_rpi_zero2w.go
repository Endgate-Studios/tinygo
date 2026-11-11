//go:build rpi_zero2w

package machine

// The ACT LED is on GPIO29, wired active low (bcm2710-rpi-zero-2-w.dts, led-act).
const (
	LED = GPIO29

	UART_TX_PIN = GPIO14
	UART_RX_PIN = GPIO15

	SDA_PIN = GPIO2
	SCL_PIN = GPIO3
)

const (
	GPIO0  Pin = 0
	GPIO1  Pin = 1
	GPIO2  Pin = 2
	GPIO3  Pin = 3
	GPIO4  Pin = 4
	GPIO5  Pin = 5
	GPIO6  Pin = 6
	GPIO7  Pin = 7
	GPIO8  Pin = 8
	GPIO9  Pin = 9
	GPIO10 Pin = 10
	GPIO11 Pin = 11
	GPIO12 Pin = 12
	GPIO13 Pin = 13
	GPIO14 Pin = 14
	GPIO15 Pin = 15
	GPIO16 Pin = 16
	GPIO17 Pin = 17
	GPIO18 Pin = 18
	GPIO19 Pin = 19
	GPIO20 Pin = 20
	GPIO21 Pin = 21
	GPIO22 Pin = 22
	GPIO23 Pin = 23
	GPIO24 Pin = 24
	GPIO25 Pin = 25
	GPIO26 Pin = 26
	GPIO27 Pin = 27
	GPIO29 Pin = 29
)
