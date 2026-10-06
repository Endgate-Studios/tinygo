# device/esp/radio compared with espradio

This document compares the `device/esp/radio` package with the
`tinygo.org/x/espradio` module on the ESP32-S3. It gives features, memory,
throughput and power. It also tells which values are measured and which are
not.

## Summary

| Item | device/esp/radio | espradio |
|---|---|---|
| Dual core (`-scheduler=cores`) | Yes. Tested for 2 hours with Wi-Fi and BLE | No multicore code. Not tested with `-scheduler=cores` |
| Wi-Fi and BLE at the same time | Yes. Tested | BLE stops at start with `BLE assert emi.c 164` on our board |
| Static RAM of the radio | 9.3 KB less with Wi-Fi | Reference |
| Blob memory, Wi-Fi connected | 27,976 bytes | 31,296 bytes |
| AMPDU (frame aggregation) | On | Off |
| Wi-Fi power save | `PowerSaveMinModem` (ESP-IDF default). `SetPowerSave` can change it | `WIFI_PS_NONE`, fixed |
| PSRAM | Optional, for the TX cache and the frame pool | Not used |
| Memory statistics | In use, peak, failures, largest free block | In use and capacity |
| Station mode | Yes | Yes |
| HTTP requests in 2 min, 1 client | 4,326 to 4,796 | 3,143 to 3,169 |
| Access point mode | WPA2-PSK or open, up to 4 stations, DHCP server in `tinygo/net/ip`. Not tested with a station yet | Yes, with a DHCP server |
| Stop and restart | `Stop`, `Deinit`, `BLEDisable`. 20 cycles tested, no memory growth | Not compared |
| ESP-NOW | Yes, with encryption, peer rates and Long Range. Tested with one board, reception not tested yet | Yes |
| TCP/IP stack | Separate (`tinygo/net/ip`) | Included (lneto), and a `netdev` driver |

## Test setup

- Board: ESP32-S3 with 8 MB octal PSRAM, target `esp32s3-psram-octal`.
- The same TinyGo tree and linker script for the two packages.
- espradio version: `v0.3.1-0.20260918065743-8e5c9e19bca5`.
- device/esp/radio: ESP-IDF v5.3.2 blobs.
- espradio test programs call `Enable`, `Start` and `Connect` directly. The
  `http-hello` example of espradio built without radio code, so we did not
  use it.
- The espradio BLE build needs 41 old BT ROM symbols that our linker script
  does not have. We added them only for the test build.

## Features

### Dual core

device/esp/radio runs with `-scheduler=cores`:

- Goroutines that use the radio can run on the two cores. `Send` works from
  each core.
- The blob tasks and the service goroutine stay on core 0, as the Wi-Fi task
  and the BT controller task do in ESP-IDF.
- The radio interrupts go to core 0. Interrupt line changes from core 1 go
  through the service goroutine.
- Test: 205,583 HTTP requests and 36,547 BLE reports in 1 hour 55 minutes,
  with 0 failures.

espradio has no multicore code. Its README tells only that the chip has two
cores. The Wi-Fi interrupt work runs on the scheduler goroutine, the tasks are
not pinned to a core, and there is no relay for the interrupt lines of core 0.

### Examples

Each example has a comment at the top with the flash command. Give the Wi-Fi
SSID and password with `-ldflags`:

| Example | What it does |
|---|---|
| `src/examples/wifi` | Connects with WPA2, gets an address with DHCP, gets a page, answers ping, and runs UDP and TCP echo on port 7 |
| `src/examples/wifi-http` | HTTP client and server with `tinygo/net/web`, with no allocation for each request |
| `src/examples/wifi-ble` | HTTP server on Wi-Fi and BLE passive scan at the same time |
| `src/examples/wifi-ap` | WPA2 access point with a DHCP server and an HTTP server on 192.168.4.1 |
| `src/examples/espnow` | ESP-NOW broadcast and unicast messages between boards, with no access point |
| `src/examples/udp-nodes` | Finds the other nodes on the Wi-Fi network with UDP broadcast on port 4210, and answers mDNS for `tinygo-XXXXXX.local` |
| `src/examples/ble-scan` | BLE scan with raw HCI commands |

```
tinygo flash -target esp32s3-psram-octal -scheduler=cores \
    -ldflags='-X "main.ssid=..." -X "main.password=..."' ./src/examples/wifi-ble
```

## Memory

### Static RAM

Measured on 2026-10-05 with the current linker script, which puts the sleep
and extra Wi-Fi sections of the blobs in flash (see README.md, section IRAM).
The builds use `-scheduler=tasks` and a dummy SSID with `-ldflags`, because a
build without an SSID drops the radio code. With `-scheduler=cores`, the heap
is 4,096 bytes smaller, for the stack of the second core. All values are in
bytes:

- IRAM, `.data` and `.bss` are the section sizes of the ELF file.
- The example buffers are the sizes of the symbols `socketMem`, `serverBuf`
  and `ringMem` of our examples. The espradio test program has no such
  buffers.
- The GC heap is `_heap_end` minus `_heap_start`.

| | radio, Wi-Fi (`wifi-http`) | espradio, Wi-Fi | radio, Wi-Fi and BLE (`wifi-ble`) |
|---|---|---|---|
| IRAM code | 22,180 | 22,300 | 37,988 |
| .data | 8,832 | 9,332 | 9,572 |
| .bss | 45,894 | 20,394 | 54,086 |
| Example buffers in .bss | 33,792 | 0 | 41,984 |
| GC heap | 325,728 | 350,192 | 300,704 |
| GC heap without example buffers | 359,520 | 350,192 | 342,688 |

Without the example buffers, device/esp/radio leaves 9,328 bytes more GC heap
than espradio with Wi-Fi. The espradio build with Wi-Fi and BLE does not link
with this linker script, because it needs 41 old BT ROM symbols, so it has no
column. The blob heap comes from the GC heap at run time and is not in these
values: 64 KB for Wi-Fi in device/esp/radio, and 48 KB in espradio.

With the old linker script, which put all Wi-Fi IRAM sections in IRAM, the
IRAM code of device/esp/radio with Wi-Fi was 38,800 bytes, and of espradio
39,780 bytes.

### Blob heap

The blobs allocate memory at run time from an arena in the GC heap.

| | device/esp/radio | espradio |
|---|---|---|
| Arena | One share for each radio: Wi-Fi 64 KB, BLE 32 KB | One arena of 48 KB for Wi-Fi and BLE |
| RX frame buffers | 8 × 1,536 = 12,288 bytes in the arena, or in PSRAM | 6 × 1,602 = 9,612 bytes in a static C ring |
| Wi-Fi buffers | 10 static RX, 32 dynamic RX, 32 dynamic TX, 5 management | The same |
| AMPDU RX and TX | On | Off |
| BLE activities | 6, from 1 to 10 with `WithBLEActivities` | 6 |
| Blob task stacks | 8 KB goroutine stack for each task | The same |

### Measured arena use

| State | device/esp/radio | espradio |
|---|---|---|
| Wi-Fi connected, no traffic | 40,264 bytes: 27,976 blobs and 12,288 frame pool | 31,296 bytes, plus 9,612 bytes in the static ring |
| Wi-Fi peak during the connection | 45,400 bytes | No peak counter |
| Wi-Fi under load | Peak 47,192 bytes at 57 requests/s with 1 client. Peak 56.0 KB with 4 clients and a 64 KB share | Not measured |
| Wi-Fi and BLE scan, no traffic | 67,040 bytes | Could not run |
| Wi-Fi and BLE under load | Peak 74,000 to 79,800 bytes | Could not run |

The RX path of the two packages uses almost the same memory when Wi-Fi is
connected: 40,264 bytes against 40,908 bytes. device/esp/radio has AMPDU on.

BLE adds about 26.8 KB in device/esp/radio. If BLE needs a similar quantity in
espradio, Wi-Fi and BLE need about 58 KB, which is more than its arena of
48 KB. This is an estimate. We could not measure it.

## Throughput

One HTTP client asks for `/` again and again for 2 minutes, with a timeout of
3 s. Two rounds for each build:

| Build | Requests in 2 min | Failures |
|---|---|---|
| espradio, lneto HTTP server, `-scheduler=tasks`, `WIFI_PS_NONE` | 3,169 and 3,143 | 0 |
| device/esp/radio, `wifi-http`, `-scheduler=tasks`, `PowerSaveMinModem` | 4,796 and 4,678 | 0 |
| device/esp/radio, `wifi-http`, `-scheduler=cores`, `PowerSaveMinModem` | 4,435 and 4,326 | 0 |

device/esp/radio gave 39% to 50% more requests. These runs used the old Wi-Fi
share of 48 KB. With one client, this share was sufficient. This compares the full
stacks: the TCP/IP stack and the HTTP server are different in the two builds.
So the result does not show the speed of the radio code alone. The espradio
build did not answer ping.

Other measurements of device/esp/radio, one HTTP client, `PowerSaveNone`:

| Test | Result |
|---|---|
| `wifi-http`, small pages, `-scheduler=cores` | About 45 requests/s |
| `wifi-ble`, small pages, BLE scan on | About 30 requests/s |
| `wifi-ble`, 16 KB pages, 1 client | 91.3 KB/s out |
| `wifi-ble`, 16 KB pages, 4 clients | 109.4 KB/s out |

## Power

We did not measure current. The values below are from the ESP32-S3 Series
Datasheet v2.2, section 5.6. Espressif measured them at 3.3 V and 25 °C.

### Supply

The recommended supply voltage is 3.0 V to 3.6 V, typical 3.3 V. With one
power supply, the recommended output current is 500 mA or more. See table 5-2
of the datasheet.

### Radio current (peak)

| Mode | Current |
|---|---|
| Wi-Fi TX, 802.11b, 1 Mbps, 21 dBm | 340 mA |
| Wi-Fi TX, 802.11g, 54 Mbps, 19 dBm | 291 mA |
| Wi-Fi TX, 802.11n HT20, MCS7, 18.5 dBm | 283 mA |
| Wi-Fi RX, 802.11b/g/n HT20 | 88 mA |
| BLE TX, 9 dBm | 193 mA |
| BLE TX, 0 dBm | 176 mA |
| BLE RX | 93 mA |

TX values are for a 100% duty cycle. See tables 5-7 and 5-8 of the datasheet.

### CPU current at 240 MHz, radio clock gated

| CPU state | Peripheral clocks off | Peripheral clocks on |
|---|---|---|
| Two cores in `waiti` | 32.9 mA | 47.6 mA |
| One core runs, one core in `waiti` | 51.2 mA | 65.9 mA |
| Two cores run | 66.2 mA | 81.3 mA |

These values are for 32-bit data access instructions. See table 5-9 of the
datasheet. The datasheet tells that chips with PSRAM can use more current.

### What the software changes

- **Core voltage:** before the switch to 240 MHz, the runtime sets the digital
  and RTC LDO values from the eFuse, as ESP-IDF does. On our board, the values
  are digital 31 and RTC 28, about 1.235 V for the digital supply. Without this
  step, core 0 gets errors when the radio transmits. This is in the runtime,
  so it applies to the two packages.
- **Second core:** with `-scheduler=cores`, an idle core runs `waiti`. The
  second core uses more current only when it has work.
- **Wi-Fi power save:**
  - device/esp/radio uses `PowerSaveMinModem`, the ESP-IDF default. The
    receiver sleeps between DTIM beacons. Tests with this mode had no lost
    frames (see README.md). `SetPowerSave(PowerSaveNone)` keeps the receiver
    on.
  - espradio sets `WIFI_PS_NONE`, fixed (`radio.go:640` and `radio.go:918`),
    because the power save callbacks crashed with its scheduler. The receiver
    stays on, and the Wi-Fi RX current is 88 mA.
  - ESP-IDF tells that `WIFI_PS_NONE` "increases power consumption, but
    minimizes the delay in receiving Wi-Fi data". See ESP-IDF v5.3.2
    `docs/en/api-guides/wifi.rst`. With BLE coexistence, Wi-Fi sleeps outside
    its time slice also with `WIFI_PS_NONE`.

To measure the current of a board, put a USB power meter or a shunt resistor
in the 3.3 V supply. Compare the average current with Wi-Fi connected and no
traffic, and the peak current during a transfer.

## Limits of this comparison

- One board and one access point.
- The espradio BLE build used 41 extra ROM symbols, and its BLE stopped with
  an assert. So there are no run-time values for espradio with BLE.
- The throughput test used a different TCP/IP stack and HTTP server for each
  package, and only one client.
- No current measurements. The power values are from the datasheet.
