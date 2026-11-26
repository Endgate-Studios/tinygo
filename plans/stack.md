# Plan: rebuild the fork as a git-spice stack

The fork's work is 109 commits on `tls-system-roots`, with 3 merges and a WIP
commit (`375c1a49`), over v0.44.0: 434 files, about 59,000 lines. This plan
rebuilds it as a stack of small branches, one subject each, in dependency
order. Each branch builds and passes its tests. The top of the stack has the
same tree as today's work.

## Rules

- **Trunk:** `release`, which is v0.44.0, the version the fork is tested on.
  Later, `gs upstack onto` moves the stack onto a newer upstream release.
- **Reference:** `fork/reference` is `tls-system-roots` plus the uncommitted
  work, committed. Nothing changes it. Every branch takes its files from it.
- **Invariant:** `git diff fork/reference fork/<top>` is empty. A branch only
  moves code. Behaviour changes and cleanups go on new branches on top,
  after the rebuild.
- **One subject per branch,** with a commit message in the TinyGo style
  (`runtime/esp32: ...`). Bottom branches match the T items of
  [upstream.md](upstream.md), so each one can become an upstream PR.
- **Build and test each branch** before the next one starts (see Checks).
- **Leave the old branches alone** (`tls-system-roots`, `esp32s3-*`,
  `runtime-memory`, `linux-c-shared`, ...). Delete them only after the stack
  is reviewed.
- **Push nothing** until the whole stack is reviewed locally.

## How to split

- **New packages:** whole directories, taken with
  `git checkout fork/reference -- <paths>`. That's about 55,000 of the
  59,000 lines.
- **Shared files:** about 60 runtime, compiler, target and machine files
  (`runtime_esp32s3.go`, `scheduler_cores.go`, `targets/esp32s3.ld`,
  `targets/esp32s3.json`, ...) carry several subjects. These are split hunk
  by hunk. The diff against the reference shows what is left.

## The stack

Bottom to top. Each item is one branch, named `fork/<name>`.

### 1. Runtime, interrupts, scheduler (from upstream.md)

| # | Branch | Upstream item |
|---|---|---|
| 1 | `esp32-sleep-wakeup` | T1 |
| 2 | `esp32s3-intr-level1` | T2 |
| 3 | `esp32s3-240mhz` | T3 |
| 4 | `esp32-i2c-pullup` | T4 |
| 5 | `gc-custom-cores` | T5 |
| 6 | `env-tz-gcnone` | T6 |
| 7 | `loader-asm-files` | T7 |
| 8 | `esp32s3-intr-spill` | T8 |
| 9 | `esp32s3-pie` | T9 |
| 10 | `cores-intr-wakes` | T10 |
| 11 | `cores-waiti` | T11 |
| 12 | `cores-lockosthread` | T12 |
| 13 | `esp32s3-psram` | T14 (#5554) |
| 14 | `psram-cores-lock` | T13 |

On its own branch from trunk: `linux-syscall-musl` (T15), then
`linux-c-shared` (T16).

### 2. Memory

| # | Branch | Contents |
|---|---|---|
| 15 | `memory-api` | `runtime/memory`: the interfaces and the typed helpers |
| 16 | `memory-arena` | `arena`: Heap and Region, with tests |
| 17 | `memory-free-heap` | `free`, `heap` |
| 18 | `memory-collector` | `collector` |
| 19 | `memory-gcshim` | `gcshim`, PSRAM spans, Scope |
| 20 | `memory-roots` | `gcshim.AddRoot` and `Region.Live`, with the `gcsuite` test |
| 21 | `memory-gcsuite` | `examples/gcsuite`, `make/test.mk` memory targets |

### 3. Crypto, Wi-Fi security, radio

| # | Branch | Contents |
|---|---|---|
| 22 | `crypto-hash-aes` | `internal/shahw`, `sha1`, `sha256`, `sha512`, `aes` |
| 23 | `crypto-ec` | `p256`, `p384`, `x25519`, `hkdf` |
| 24 | `crypto-rsa-x509` | `internal/mpi`, `rsa`, `x509` |
| 25 | `wifi-80211-wpa` | `ieee80211`, `wpa` |
| 26 | `wifi-eapol-sae` | `eapol`, `sae` |
| 27 | `radio` | `device/esp/radio`, `ie`, blobs, the `esp32s3.ld` symbols and sections |
| 28 | `radio-arenacheck` | `tools/arenacheck` (it loads the radio stack) |

### 4. Network

| # | Branch | Contents |
|---|---|---|
| 29 | `net-ip` | `ip` |
| 30 | `net-netdev` | `netdev`, `socket`, `web`, `mqtt` |
| 31 | `net-tls` | `tls`, `tls/sysroots`, `https` |

### 5. Bluetooth

| # | Branch | Contents |
|---|---|---|
| 32 | `bt-hci-l2cap` | `hci`, `l2cap`, `aclq` |
| 33 | `bt-att-smp` | `att`, `smp` |
| 34 | `bt-roles` | `central`, `peripheral`, `btctl` |

### 6. DSP, NN, examples, docs

| # | Branch | Contents |
|---|---|---|
| 35 | `esp-dsp` | `device/esp/dsp`, `esp32s3_dsp.S` |
| 36 | `esp-nn` | `device/esp/nn`, `esp32s3_nn.S` |
| 37 | `examples` | the remaining `src/examples` |
| 38 | `plans` | `plans/` |

## Checks for each branch

- `go test` of the compiler packages that the branch touches.
- `minigo build` of the targets that the branch touches. For any ESP32-S3
  change, build both the player configuration
  (`-target=esp32s3-psram-octal-jtag -scheduler=cores -gc=custom -tags=gcshim_psram,gcshim_precise_globals`)
  and `esp32s3-generic`.
- From branch 15 on, `make memory-test`.
- The package tests of each new package, with `minigo test`.
- On the top branch, `git diff fork/reference` is empty.
- At the end of each phase, run on a board: `gcsuite`, and the mp3 player
  from `imp`.

## After the rebuild

- Push the stack to `origin` (GitHub `Endgate-Studios/tinygo`) with
  `gs stack submit`.
- Make the cleanups as new branches on top. For example, the 9 sentinel
  errors in `src/tinygo/...` that are still `var Err = errors.New(...)`
  become constants.
- For an upstream PR, move its branch onto upstream `dev` with
  `gs branch onto`.
