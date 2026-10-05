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
| Wi-Fi (`Init`) | 64 KB (`DefaultArenaSize`) | `WithArenaSize`, `WithArenaMemory`, `WithRXBuffers` |
| BLE (`BLEEnable`) | 32 KB (`DefaultBLEArenaSize`) | `WithArenaSize`, `WithBLEActivities` |

With the two radios, the heap is 98,288 bytes. `MemoryStats` gives the
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
  `WithRXBuffers` changes the RX limit, from 10 to 128.
- Many small incoming frames at the same time can fill the heap. Each failed
  allocation is one dynamic RX buffer of 1,700 bytes (ESF buffer type 7 of
  the blob). The blob then drops the frame, and the access point sends it
  again.
- The RX buffers fail because the heap is full, not because of the limit.
  The number of RX buffers in use stayed far below the limit. Thus
  `WithRXBuffers(16)` did not change the failures.
- Large outgoing data did not fill the heap. 110 KB/s out used less memory
  than 180 small frames/s in.
- With 4 clients and 16 KB pages, 44 requests failed with "connection
  refused". The cause was the socket memory of the example, see "Socket
  memory" below.

### Size of the Wi-Fi share

The tests above used a Wi-Fi share of 48 KB, the old default. A test with 4
HTTP clients, small pages, `PowerSaveNone` and `-scheduler=cores`, 60 s for
each run:

| Radios | Wi-Fi share | Requests/s | Failed allocations | Heap peak |
|---|---|---|---|---|
| Wi-Fi | 48 KB | 12.9 to 15.7 | 6.3 to 7.2 per second | 47.8 KB (full) |
| Wi-Fi | 56 KB | 107.1 | 3 in total | 54.5 KB |
| Wi-Fi | 64 KB | 107.6 | 0 | 56.0 KB |
| Wi-Fi | 80 KB | 105.5 | 0 | 59.2 KB |
| Wi-Fi and BLE scan | 48 KB | 27.9 to 43.6 | 1.2 to 1.6 per second | 80.1 KB (full) |
| Wi-Fi and BLE scan | 56 KB | 48.2 | 0 | 82.9 KB |
| Wi-Fi and BLE scan | 64 KB | 56.8 and 57.8 | 0 | 82.8 to 84.5 KB |

With a full heap, the blob drops RX frames and TCP sends them again. For Wi-Fi
alone, this made the server 7 times slower. Thus the default share is now
64 KB. The HTTP errors in these tests were 1 to 22 "connection refused" for
each run, from the socket memory of the example.

- To keep 16 KB more GC heap, use `WithArenaSize(48 << 10)` for one client
  at a time.
- `WithRXBuffers` sets the RX limit of ESP-IDF. A lower limit did not reduce
  the failures in these tests.

### Socket memory

Each TCP connection of `tinygo/net/ip` takes a send ring and a receive ring of
the buffer size of `ListenTCP` from the socket memory, from the SYN to
TIME-WAIT. 16 KB holds about 3 connections with 2 KB rings. The HTTP
examples have 32 KB. A server that handles one connection at a time needs one connection
for each client that waits, and one more.

When the memory is full, the stack drops the SYN, and the client sends it
again later, as lwIP does. Before, the stack refused the connection.

Test with 4 clients, a Wi-Fi share of 64 KB and the `wifi-ble` code:

| Radios | Socket memory | Requests/s | HTTP errors in 60 s |
|---|---|---|---|
| Wi-Fi | 16 KB, SYN refused (old) | 107.6 | 22 refused |
| Wi-Fi | 16 KB, SYN dropped | 90.8 and 108.4 | 10 and 11 timeouts |
| Wi-Fi | 32 KB | 121.2 | 0 |
| Wi-Fi and BLE scan | 16 KB, SYN dropped | 59.9 | 10 timeouts |
| Wi-Fi and BLE scan | 32 KB | 73.6 | 0 |

With 4 clients at the same time, 32 KB of socket memory was sufficient.
In the Wi-Fi run with 32 KB, the frame pool of 8 buffers dropped 20 frames
(`FramesDropped`), and TCP sent them again.

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

Group frames (broadcast and multicast) are less reliable with
`PowerSaveMinModem`. The access point keeps them until a DTIM beacon. In a
test of 60 s with mDNS queries to the `udp-nodes` example, the board answered
90% of the queries with `PowerSaveMinModem`, with gaps of 1 to 5 s, and 100%
with `PowerSaveNone`.

ESP-IDF loses group frames in modem sleep too. In a test that sent 10 UDP
broadcasts per second, an ESP-IDF v5.3.2 program on the same board received
from 0% to 87% of them, at different times of the day with the same access
point. `seam_wifi_init` sets the sleep times of `esp_wifi_init` (minimum
active time, keep alive time and the wait for broadcast data). Without them,
the board received 2.5 to 3 times fewer broadcasts in each round of an
interleaved test. For a protocol that depends on group frames, use
`PowerSaveNone`.

## Cores

With `-scheduler=cores`, goroutines that use the radio can run on the two
cores. The blob tasks and the service goroutine stay on core 0, as the Wi-Fi
task and the BT controller task do in ESP-IDF. The radio interrupts go to
core 0. Interrupt line changes from core 1 go through the service goroutine
(`core.go`).

The runtime sets the core voltage for 240 MHz from the eFuse values before it
changes the CPU clock (`src/runtime/runtime_esp32s3.go`). Without this, core 0
gets errors when the radio transmits.

## Access point

`SetMode(ModeAccessPoint)` and `ConfigureAP` make a soft-AP with WPA2-PSK and
CCMP, or an open AP when the password is empty. It accepts up to `MaxStations`
(4) stations. `ap.go` does the 4-way handshake with `tinygo/net/wifi/eapol`,
as hostapd does in ESP-IDF.

- `AccessPoint` is the Ethernet link of the AP for `tinygo/net/ip`, and
  `HandleAPFrames` gives it the received frames.
- `Stack.ServeDHCP` of `tinygo/net/ip` gives addresses to the stations.
- `EventAPStaConnected` and `EventAPStaDisconnected` tell when a station joins
  or leaves.
- The blobs derive the PMK in `ConfigureAP` with `crypto.go`. PBKDF2 uses the
  SHA accelerator through `tinygo/crypto/sha1` and takes about 100 ms at
  240 MHz. With software SHA-1 it took 754 ms.

The `wifi-ap` example makes the network `tinygo-ap` and serves HTTP on
192.168.4.1. The board sends beacons and the unit tests do the handshake and
DHCP, but a test with a station is not done yet.

## ESP-NOW

ESP-NOW sends messages of up to 250 bytes between Espressif chips, in Wi-Fi
vendor action frames. It needs no access point and no IP. A PC or a phone
cannot receive it. The protocol is in `libespnow.a` of the blobs.

- `ESPNowInit` after `Start`. Without a connection, `SetChannel` sets the
  channel. All devices must use the same channel.
- `ESPNowAddPeer` before a send. `ESPNowBroadcast` sends to all devices on
  the channel. A peer with a `Key` of 16 bytes uses encryption, up to 7
  peers. Broadcast messages are not encrypted. `ESPNowPeers` lists the
  unicast peers.
- `ESPNowSend` waits for the send status. A unicast message gives
  `ErrESPNowNoAck` when the peer did not acknowledge it.
- `HandleESPNow` gets the received messages, with the source MAC and the
  RSSI. Call it once.
- `ESPNowSetPeerRate` sets the PHY rate for a peer. With `SetLongRange` on
  the devices, `PHYModeLR` sends at 250 or 500 kbps over a longer distance.
- `ESPNowSetWakeWindow` is for power save. As in ESP-IDF, it works only
  when the station is connected.
- `crypto.go` gives the blobs the AES block functions for encrypted peers.
  `tinygo/crypto/aes` uses the AES accelerator for 128 and 256 bit keys:
  3 µs for each block, against 43 µs in software.

The `espnow` example sends a broadcast each second and answers each new
board with a unicast message.

Tested with one board:

- The broadcasts are sent, also at 1 Mbps and at Long Range 500 kbps.
- A unicast message to a MAC that does not exist gives `ErrESPNowNoAck`,
  also for an encrypted peer.
- The 8th encrypted peer gives `ErrESPNowFull`.
- 5 cycles with `Deinit` left no memory in the blob heap.

Reception, the acknowledgement of a real peer and decryption need two
boards. See "Integration tests" below.

## Stop and restart

- `Stop` stops Wi-Fi, as `esp_wifi_stop`. `Start` starts it again.
- `Deinit` stops the Wi-Fi blobs and frees the frame pools. `Init` starts them
  again.
- `BLEDisable` stops the BT controller. `BLEEnable` starts it again.

When no radio runs, the package stops the blob timers and
turns off the power domain of the radio. The blob heap stays, because blob
variables keep pointers into it, as in ESP-IDF. A restart uses the same heap.

Test: 20 cycles of Wi-Fi `Init`, connect and DHCP, `BLEEnable` and a BLE scan,
then `BLEDisable`, `Stop` and `Deinit`, with `-scheduler=cores`. Each cycle
took about 4.1 s. After each cycle, the blob heap had 56 bytes in use, which
the coexist blob allocates one time. The GC heap stayed from 142.7 KB to
152.2 KB.

`Deinit` frees the IEs that the blobs copy for `esp_wifi_set_appie_internal`,
as `esp_supplicant_deinit` of ESP-IDF does. Before this, each connection
left 16 bytes in the blob heap.

`Deinit` while BLE runs also works. After it, a passive BLE scan got 22 to 30
reports for each cycle, and Wi-Fi started again while BLE stayed on.

## IRAM

The linker script uses the placement of ESP-IDF. `.wifi0iram` and
`.wifirxiram` stay in IRAM. The sleep and extra Wi-Fi sections go to flash.
This gives 16,608 bytes more GC heap (`wifi-http`: 338,016 bytes).

In an A/B test, all Wi-Fi IRAM sections in flash gave 2% to 6% fewer HTTP
requests than all of them in IRAM.

## BLE connections

A GATT peripheral test program (not in the examples) ran with the `wifi-http`
server under load for 4 minutes. One connection stayed up. 295 GATT write and
read cycles had 0 errors and 0 data mismatches. 8,086 HTTP requests had 0
failures. The LE ACL size was 251 bytes, with 12 buffers.

## Next work

1. **Current measurement.** Measure the current with `PowerSaveMinModem` and
   with `PowerSaveNone`, with a USB power meter or a shunt resistor.
2. **ESP32-C3 and ESP32-C6.** The runtime fix for `sleepTicks` builds, but it
   is not tested on these chips.

## Integration tests

These tests need other devices. They are not done yet.

- **Access point with a station.** Connect a phone to the `wifi-ap` example
  and check the 4-way handshake, DHCP and HTTP.
- **ESP-NOW with two boards.** Flash the `espnow` example to two boards and
  check reception, the unicast acknowledgement and an encrypted peer.
