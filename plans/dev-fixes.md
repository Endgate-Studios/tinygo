# Plan: make endgate-dev pass its tests

`endgate-dev` has all the fork's work (61 commits over v0.44.0). One test
pass on it on 2026-11-25 built every board and passed every package test,
but for the failures below. Each one failed the same way before the
rebuild, on `rpi-zero2w` at `f7beab1c`. None is from the split.

Each fix is a branch on `endgate-dev` (`gs branch create fork/<name>`), one
subject each, merged when its test passes.

## Order

```
F1, F2, F3: any order, now
F4: after F1 to F3
F5, F6, F7: when there's time; F5 and F6 need a board or upstream
```

## F1. gcsuite: gcshim doesn't read pointer-free buffers

- Fails: `minigo run -gc=custom ./src/examples/gcsuite quick` on Windows:
  "GC reads the make arena, so its victims survive", 16 of 16 victims freed.
- Cause: the test is out of date, not the collector. `6a6acbd1` ("gcshim:
  don't scan large memory with no pointers") gives large pointer-free
  objects pages that are marked but never scanned. The arena over
  `make([]byte, 4<<10)` is such an object, so its victims are freed, as they
  should be. `gcmode_custom.go` and `gcmode_custom_precise.go` still say
  `heapBytesRead = true`.
- Fix: `heapBytesRead = false` in both, with the comment saying why, as
  `gcmode_precise.go` has. Add a case that puts the pointers in a heap
  object that has pointers (`make([]unsafe.Pointer, n)` as the arena's
  memory), to keep checking that gcshim reads those.
- Done when: `gcsuite quick` passes with `-gc=conservative`, `precise` and
  `custom` on Windows, and it builds for `esp32s3-psram-octal-jtag` with
  `-gc=custom -tags=gcshim_psram,gcshim_precise_globals`.

## F2. builder: TestClangAttributes/esp32s3 and S32C1I in C code

- Fails: `go test ./builder -run TestClangAttributes`: the target has
  `-s32c1i` but Clang has `+s32c1i`.
- Cause: `targets/esp32s3.json` turns S32C1I off for Go code
  (`fork/psram-cores-lock`, T13), because it isn't atomic on PSRAM. Its
  `cflags` don't, so Clang still uses S32C1I for C atomics: picolibc and the
  radio's seam C files. The test is right: C atomics on PSRAM are as broken
  as Go's were.
- Fix: give Clang the same feature in the ESP32-S3 `cflags`
  (`"-Xclang", "-target-feature", "-Xclang", "-s32c1i"`), and check with
  `llvm-objdump -d` that a C atomic in a seam file becomes a call, not
  `s32c1i`. The radio blobs are prebuilt by Espressif: check them with
  `llvm-objdump -d` for `s32c1i`, and if they use it, their data must stay in
  internal RAM. Write what you find in `device/esp/radio/README.md`.
- Then, as upstream.md T13 says: move `-s32c1i` and the `cflags` from
  `esp32s3.json` to the PSRAM targets, so builds without PSRAM keep native
  atomics. The Peterson lock works either way.
- Done when: `TestClangAttributes` passes, and the S3 examples build with
  and without PSRAM.

## F3. builder: TestESP32S3ImageLayout

- Fails: `go test ./builder -run ESP32S3`: "unexpected padding before
  IROM: image ends at 0x160, IROM starts at 0x10000", in all three cases.
- Cause: the test came with the fork's large flash segment fix
  (`ac70d2f9`). The v0.44.0 merge kept upstream's image builder (`03503292`)
  instead, which wants IROM straight after the RAM image, with the low 16
  bits of its address equal to the image's end, as `targets/esp32s3.ld`
  lays it out. The test's ELF puts `.text` at `0x42000000`.
- Fix: make `esp32s3TestImage` place `.text` and `.rodata` as the linker
  script does: `.text` at `0x42000000` plus the RAM image's end, and
  `.rodata` one 64 KB page past the end of `.text`, with the same low bits as
  its offset. Keep the checks that still hold for upstream's layout: two
  ROM segments, patched offsets inside the image, the contents at those
  offsets, and the large DROM case. Drop the 64 KB alignment check, which
  upstream's layout doesn't have.
- Done when: `TestESP32S3ImageLayout` passes, and a built
  `esp32s3-psram-octal` image of `examples/wifi` has the same layout.

## F4. One test pass on endgate-dev

- `make tinygo`, then `go test` of `loader`, `compileopts`, `compiler`,
  `compiler/llvmutil`, `transform`, `interp`, `builder`, `tools/arenacheck`
  and `tools/rpiboot`.
- The board builds and package tests of the 2026-11-25 pass: the targets and
  tests listed in [stack.md](stack.md) under "How each branch was checked",
  and `gcsuite quick` with each collector.
- `tinygo/net/tls` and `https` with stock Go on a GOROOT with the fork's
  packages.
- Done when: nothing fails but F5 to F7.

## F5. TestFixTinyGoSource under Go 1.27

- Fails: `go test . -run TestFixTinyGoSource`: Go 1.27's `go fix` wants
  changes in about ten upstream runtime files (`gc_boehm.go`,
  `hashmap.go`, `print.go`, `runtime.go`, ...).
- Cause: upstream's test with a Go newer than upstream tests with. The files
  are upstream's.
- Fix: none in the fork. Take upstream's fix when `endgate-dev` moves to the
  next release. Until then, run the root package's tests with
  `-skip TestFixTinyGoSource`.

## F6. USB on the CrowPanel rev3 target

- Fails: `minigo build -target=crowpanel-advanced-9-p4-rev3 -tags=usbotg`:
  `esp_rom_software_reset_system` is undefined.
- Cause: `targets/esp32p4-rev3.ld` has none of the ROM addresses that
  `esp32p4.ld` has. Chips of revision 3 have another ROM.
- Fix: the addresses from ESP-IDF's `esp32p4.rom.ld` for revision 3
  (`components/esp_rom/esp32p4/ld/`), in `esp32p4-rev3.ld`. Check each one
  against that file, not against the revision 1 addresses.
- Done when: the rev3 target builds with USB and with
  `-scheduler=cores`, and one short check on a rev3 board, if there is one.

## F7. Tests that don't run

- `tinygo/crypto/x509`'s tests compiled for `esp32s3-generic` stop LLVM:
  "Incomplete scavenging after 2nd pass", in the Xtensa backend. The package
  builds into the TLS examples. Find the test function that does it with
  `-run` on each, make a small reproducer, and report it to Espressif's
  LLVM fork. In the fork, skip that test on Xtensa.
- `device/esp/radio`'s `hosted_wire_test.go` (tag `hostedwire`) runs
  nowhere: the package's C files stop `tinygo test` and `go test` on the
  host. Move the hosted wire code to its own package, `device/esp/radio/hosted`,
  so its test runs on the host.
