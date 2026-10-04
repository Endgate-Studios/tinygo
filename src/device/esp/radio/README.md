# device/esp/radio

Package radio runs the ESP32-S3 Wi-Fi and Bluetooth LE blobs of ESP-IDF v5.3.2.
You can use the two radios separately or at the same time.

For a comparison with the espradio module (features, memory, throughput and
power), see [COMPARISON.md](COMPARISON.md).

## Memory

All memory of the blobs is in one blob heap in internal RAM, because the MAC
uses DMA. Each radio adds its share to this heap:

| Radio | Default share | Option |
|---|---|---|
| Wi-Fi (`Init`) | 48 KB (`DefaultArenaSize`) | `WithArenaSize`, `WithArenaMemory` |
| BLE (`BLEEnable`) | 32 KB (`DefaultBLEArenaSize`) | `WithArenaSize`, `WithBLEActivities` |

With the two radios, the heap is 81,904 bytes. `MemoryStats` gives the
counters of the heap. `Failures` is the number of allocations that did not
get memory.

### Measured use with Wi-Fi and BLE

Conditions:

- ESP32-S3 with octal PSRAM, `-scheduler=cores`, default shares.
- The `wifi-ble` example: an HTTP server and a passive BLE scan (interval
  100 ms, window 30 ms).
- One access point near the board. Each test was 60 s.
- `PowerSaveNone`. The default was `PowerSaveNone` at the time of these
  tests.

| Load | HTTP | RX frames/s (max) | TX frames/s (max) | Payload in / out | Heap peak | Failed allocations |
|---|---|---|---|---|---|---|
| 1 client, small pages | 19.8 req/s | 106 (137) | 100 (120) | 8.8 / 8.5 KB/s | 74,232 B | 0 |
| 4 clients, small pages | 33.7 req/s | 180 (262) | 172 (259) | 14.2 / 14.6 KB/s | 79,800 B | 1.1 per second |
| 1 client, 16 KB pages | 5.1 req/s | 92 (114) | 187 (210) | 5.8 / 91.3 KB/s | 74,520 B | 0 |
| 4 clients, 16 KB pages | 6.1 req/s | 112 (135) | 228 (253) | 7.1 / 109.4 KB/s | 78,968 B | 0 |

There were no TX errors and no dropped RX frames in these tests.

A 2-hour test with one client gave 205,583 requests, 0 failed requests and
36,547 BLE reports. The heap peak stayed at 74,368 bytes.

### What to know

- The heap peak does not show how much memory the blobs need. The Wi-Fi blob
  allocates up to 32 dynamic RX buffers and 32 dynamic TX buffers
  (`dynamic_rx_buf_num` and `dynamic_tx_buf_num` in `seam.c`). It uses more
  buffers when memory is free, and fewer when the heap is small.
- Many small incoming frames at the same time can fill the heap. Each failed
  allocation is one dynamic RX buffer of 1,700 bytes. The blob then drops the
  frame, and the access point sends it again. At 180 RX frames/s, this was
  0.6% of the frames. It did not cause HTTP errors.
- Large outgoing data did not fill the heap. 110 KB/s out used less memory
  than 180 small frames/s in.
- With 4 clients and 16 KB pages, 44 requests failed with "connection
  refused". The cause is the one TCP listener of the example, not the radio
  or the memory.

### Recommendations

- The default shares are sufficient for one client at all the loads above.
- For bursts of many small incoming frames, add memory to the Wi-Fi share,
  for example `WithArenaSize(56 << 10)`. Each 1.7 KB gives one more RX buffer
  for a burst.
- Wi-Fi alone (`wifi-http`, 48 KB) had a heap peak of 47 KB at 57 req/s with
  no failed requests. The heap was almost full, but the blob worked with the
  buffers that it had.

## Power save

The station uses `PowerSaveMinModem`, the ESP-IDF default. The receiver
sleeps between DTIM beacons. `SetPowerSave(PowerSaveNone)` keeps the receiver
on. Measured with one HTTP client, `-scheduler=cores`:

| Example | Power save | Ping average | Requests in 2 min | Failures |
|---|---|---|---|---|
| `wifi-http` | `PowerSaveMinModem` | 75 ms | 7,045 | 0 |
| `wifi-http` | `PowerSaveNone` | 8 ms | 5,375 | 0 |
| `wifi-ble` | `PowerSaveMinModem` | 64 ms | 2,858 | 0 |
| `wifi-ble` | `PowerSaveNone` | 21 ms | about 3,700 | 0 |

With `PowerSaveMinModem`, 10 requests after 10 s of idle time each got an
answer. With BLE coexistence, `PowerSaveNone` gives more Wi-Fi throughput.

## Cores

With `-scheduler=cores`, goroutines that use the radio can run on the two
cores. The blob tasks and the service goroutine stay on core 0, as the Wi-Fi
task and the BT controller task do in ESP-IDF. The radio interrupts go to
core 0. Interrupt line changes from core 1 go through the service goroutine
(`core.go`).

The runtime sets the core voltage for 240 MHz from the eFuse values before it
changes the CPU clock (`src/runtime/runtime_esp32s3.go`). Without this, core 0
gets errors when the radio transmits.

## Next work

The items are in order of priority.

1. **Current measurement.** Measure the current with `PowerSaveMinModem` and
   with `PowerSaveNone`, with a USB power meter or a shunt resistor.
2. **Reconnect.** Test what occurs when the access point stops or the signal
   is lost. The station must connect again after a disconnect event.
3. **Throughput against espradio.** Add a TCP server to the espradio test
   program and measure the two packages with the same load.
4. **Access point mode.** Add the AP configuration and a DHCP server, so that
   the board can make its own network.
5. **BLE advertising and connections.** Only the passive scan is tested. Test
   advertising and a connection with Wi-Fi under load.
6. **Stop and restart.** Add functions to stop Wi-Fi and BLE, give back their
   memory, and start them again.
7. **IRAM option.** The Wi-Fi IRAM sections of the blobs use about 35 KB of
   SRAM. Add an option to put them in flash, and measure the throughput.
8. **RX bursts.** Many small incoming frames can fill the Wi-Fi share. Give a
   size option for the dynamic RX buffers, or a larger default share.
9. **ESP-NOW.** Add it if a user needs it.
10. **ESP32-C3 and ESP32-C6.** The runtime fix for `sleepTicks` builds, but
    it is not tested on these chips.
