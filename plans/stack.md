# The fork as a git-spice stack

The fork's work is a stack of small branches, one subject each, on the trunk
`endgate-dev`. `endgate-dev` is v0.44.0 (`344590c7`). Each branch is one
commit, and its parent is the branch below it. The top of the stack,
before the fixes, has the same tree as `rpi-zero2w` at `f7beab1c` (the
branch `fork/reference`), which had all the fork's work.

The old branches (`rpi-zero2w`, `wasm-cores`, `esp32p4`, `tls-system-roots`,
`esp32s3-*`, `runtime-memory`, `linux-c-shared`, ...) are left as they were.
Delete them once the stack is on GitHub.

## Working on it

- Trunk: `endgate-dev`. git-spice is set up with `gs repo init --trunk endgate-dev --remote origin`.
- New work: a new branch on the top, `gs branch create fork/<name>`.
- A change to a lower branch: `gs branch checkout fork/<name>`, change it, `gs commit amend`. git-spice restacks the branches above.
- To GitHub: `gs stack submit`. It pushes each branch and opens a pull request into the branch below it, the bottom one into `endgate-dev`.
- Onto a newer TinyGo: `gs upstack onto` from the bottom branch, after `endgate-dev` moves to the new release.
- For an upstream PR: `gs branch onto` the branch to upstream `dev`. The bottom 16 hold the T items of [upstream.md](upstream.md).
- `minigo` builds from `~/tinygo`. Check out the top branch there.

## The stack, bottom to top

Runtime, interrupts and the cores scheduler (T items of upstream.md):

| # | Branch | Subject |
|---|---|---|
| 1 | `fork/esp32-sleep-wakeup` | T1: an interrupt since the last sleep ends the sleep |
| 2 | `fork/esp32s3-intr-level1` | T2: accept only level 1 lines on the ESP32-S3 |
| 3 | `fork/esp32s3-240mhz` | T3: core voltage for 240 MHz, and the ROM's CPU frequency |
| 4 | `fork/esp32-i2c-pullup` | T4: the internal pull-ups of the I2C pins |
| 5 | `fork/gc-custom-cores` | T5: -gc=custom with -scheduler=cores |
| 6 | `fork/env-tz-gcnone` | T6: the environment without an allocation with -gc=none |
| 7 | `fork/esp32s3-intr-spill` | T8: spill all register windows when an interrupt comes |
| 8 | `fork/esp32s3-pie` | T9: the PIE coprocessor on both cores |
| 9 | `fork/esp32s3-core1-start` | leave core 1 running if a debugger started it |
| 10 | `fork/cores-intr-wakes` | T10: wake tasks from interrupt handlers |
| 11 | `fork/cores-waiti` | T11: a sleeping core waits with waiti |
| 12 | `fork/cores-lockosthread` | T12: LockOSThread keeps a goroutine on its core |
| 13 | `fork/iface-equal-noalloc` | compare interface strings without an allocation |
| 14 | `fork/esp32s3-pin-intr-core0` | route the pin interrupt to core 0 |
| 15 | `fork/esp32s3-psram` | T14: PSRAM for Octal chips (#5554) |
| 16 | `fork/psram-cores-lock` | T13: a Peterson lock for the cores, and no S32C1I |

T7 (assembling .S files in packages) was never written.

Linux and the compiler:

| # | Branch | Subject |
|---|---|---|
| 17 | `fork/linux-syscall-musl` | T15: single GOROOT file overrides, musl's network sources |
| 18 | `fork/llvmutil-captures` | only the provenance bits of captures let a pointer escape |
| 19 | `fork/linux-c-shared` | T16: -buildmode=c-shared on Linux |
| 20 | `fork/docker-image` | the compiler image on gitea |

Memory:

| # | Branch | Subject |
|---|---|---|
| 21 | `fork/memory-collector` | a mark-sweep collector with size classes |
| 22 | `fork/memory-gcshim` | the collector as -gc=custom, with PSRAM spans |
| 23 | `fork/memory-api` | the interfaces of the memory strategies |
| 24 | `fork/memory-arena` | allocators over memory the GC doesn't scan |
| 25 | `fork/memory-free-heap` | give GC objects back early |
| 26 | `fork/memory-gcsuite` | gcsuite, memtest and make memory-test |

Crypto, Wi-Fi, the radio, the network and Bluetooth:

| # | Branch | Subject |
|---|---|---|
| 27 | `fork/crypto-hash-aes` | SHA-1, SHA-256, SHA-512 and AES |
| 28 | `fork/crypto-ec` | P-256, P-384, X25519 and HKDF |
| 29 | `fork/crypto-rsa-x509` | RSA verify and X.509 |
| 30 | `fork/crypto-ed25519-srp` | Ed25519, ChaCha20-Poly1305 and SRP-6a |
| 31 | `fork/wifi-80211-wpa` | 802.11 frames and the WPA keys |
| 32 | `fork/wifi-eapol-sae` | EAPOL and WPA3's SAE |
| 33 | `fork/radio` | Wi-Fi, ESP-NOW and BLE on the ESP32-S3 |
| 34 | `fork/net-ip` | the IPv4 stack |
| 35 | `fork/net-netdev` | netdev, sockets, web and MQTT |
| 36 | `fork/net-tls` | TLS and HTTPS |
| 37 | `fork/arenacheck` | arena memory holds no GC pointers |
| 38 | `fork/bt-hci-l2cap` | HCI, L2CAP and the ACL queue |
| 39 | `fork/bt-att-smp` | ATT, GATT and SMP |
| 40 | `fork/bt-roles` | central and peripheral |

DSP, NN, flash, USB and the boards:

| # | Branch | Subject |
|---|---|---|
| 41 | `fork/esp-dsp` | the esp-dsp functions |
| 42 | `fork/esp-nn` | the esp-nn functions |
| 43 | `fork/esp32-flash-size` | ask the flash chip its size |
| 44 | `fork/esp32s3-boot-slots` | start from a firmware slot, and write flash by chip address |
| 45 | `fork/usb-cdc-wait` | CDC leaves the host waiting rather than drop what it sends |
| 46 | `fork/usb-rawhid` | raw HID beside the keyboard |
| 47 | `fork/esp-heap-align` | align the ESP8266 heap start to 16 bytes |
| 48 | `fork/esp32p4` | the ESP32-P4 and the CrowPanel Advanced 9 inch board |
| 49 | `fork/esp32p4-usb` | USB 2.0 on the ESP32-P4 |
| 50 | `fork/sdmmc` | the SD/MMC host of the ESP32-S3 and ESP32-P4 |
| 51 | `fork/esp32p4-radio` | Wi-Fi on the ESP32-P4 through its ESP32-C6 |
| 52 | `fork/rpi-zero2w` | the Raspberry Pi Zero 2 W |
| 53 | `fork/rpi-cores` | the cores scheduler on its 4 cores |
| 54 | `fork/rpi-cyw43` | its SDIO host and CYW43436 Wi-Fi |
| 55 | `fork/examples` | the Wi-Fi, TLS, MQTT and UDP examples |
| 56 | `fork/plans` | the plans |

Fixes after the rebuild:

| # | Branch | Subject |
|---|---|---|
| 57 | `fork/fix-p4-usb-link` | the ROM's software reset for the plain ESP32-P4 target |
| 58 | `fork/stack-doc` | this file |

## How each branch was checked

- Built with the compiler of that branch: stock v0.44.0 below `fork/linux-syscall-musl`, and the branch's own `make tinygo` from there on.
- The targets each branch touches: `esp32s3-generic` with and without `-scheduler=cores`, the PSRAM targets, `xiao-esp32c3`, `xiao-esp32c6`, `pico`, `pico2`, `riscv-qemu`, Linux amd64 and arm64, `wasip1`, Windows, the CrowPanel targets and `rpi-zero2w`.
- The new packages' tests on Windows with `tinygo test`, and compiled for `esp32s3-generic`. The memory packages with the conservative, precise and gcshim collectors.
- `tinygo/net/tls` and `https` with stock Go on a GOROOT with the fork's packages, as their tests need the real `crypto/tls`.
- The compiler packages' `go test` where a branch changes them: `loader`, `compileopts`, `compiler`, `builder`, `transform`, `tools/arenacheck`, `tools/rpiboot`.
- Not on a board.

## What fails, as it did before the rebuild

Each of these fails the same way on `fork/reference`:

- `builder` `TestESP32S3ImageLayout`: "unexpected padding before IROM". The test came with the large flash segment fix, and the v0.44.0 merge took upstream's image builder instead.
- `builder` `TestClangAttributes/esp32s3`: the target has `-s32c1i` (T13) and Clang has `+s32c1i`. upstream.md T13 says to move `-s32c1i` to the PSRAM targets.
- `TestFixTinyGoSource`: Go 1.27's `go fix` wants changes in the runtime's sources.
- `gcsuite quick` with `-gc=custom` on Windows: "GC reads the make arena, so its victims survive" fails. The conservative and precise collectors pass.
- The x509 tests compiled for `esp32s3-generic`: LLVM's "Incomplete scavenging after 2nd pass". The package builds into the TLS examples.
- USB on `crowpanel-advanced-9-p4-rev3`: `esp32p4-rev3.ld` has none of the ROM addresses. `fork/fix-p4-usb-link` fixed the other P4 targets.
- `device/esp/radio` `hosted_wire_test.go` isn't run: the package's C files stop `tinygo test` and `go test` on the host.
