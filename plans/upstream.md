# Plan: send the fork changes to upstream TinyGo

This plan lists the changes of this fork that can go to upstream TinyGo
(`tinygo-org/tinygo`). For each change, it gives the size, the dependencies
and the work that is necessary before a PR.

The line counts come from `git diff v0.44.0 tls-system-roots`. They include
the tests.

## Scope

| Part | Lines | Decision |
|---|---|---|
| Runtime, scheduler, interrupt and compiler changes | about 2,500 | Send to TinyGo in small PRs (this plan) |
| PSRAM | about 1,100 | Finish the open PR #5554 with its author |
| Radio seam (`device/esp/radio`) and its linker changes | about 7,300 | Keep in this fork. Do not send to `espradio` |
| Network, TLS, crypto, WPA/SAE and BLE host (`src/tinygo/...`) | about 32,000 | Keep in this fork |
| DSP and NN (`device/esp/dsp`, `device/esp/nn`) | about 12,600 | Keep in this fork, or make a module after T7 and T9 |
| Memory packages (`runtime/memory`) | about 4,900 | Keep in this fork. Send only the hooks (T5, T6) |

Upstream does not accept arena allocators. See issues #4434 and #5518.

## Totals

- 16 new PRs to `tinygo-org/tinygo`, and work on 1 open PR (#5554).
- About 2,500 lines in the new PRs, and about 1,100 lines in #5554.

## Before the first PR

- [ ] Set `git config user.name` to your real name. The commits show `unknown` now.
- [ ] Start each PR branch from upstream `dev`. Do not start from the gitea `release` branch.
- [ ] Keep one subject in each PR. In #5554, a reviewer objected to a combined PR.
- [ ] Add a `make/smoketest.mk` line for each new target or example.
- [ ] Write the boards and the results of the hardware test in each PR.
- [ ] Give a source for each hardware value: ESP-IDF file, TRM section or datasheet page.

## Phase 1: small fixes with no dependencies

These PRs are small and do not change a design. Start with them. The order
does not matter.

### T1. ESP32: sleepTicks must not lose a wakeup

- Files: `runtime_esp32sx.go`, `runtime_esp32c3.go`, `runtime_esp32xx_plic.go`.
- Size: about 40 lines.
- Change: an interrupt since the last sleep ends the sleep. Today the sleep
  clears the flag and waits again.
- Work before the PR:
  - [ ] Test on an ESP32-C3 and an ESP32-C6. Only the ESP32-S3 has a hardware test now.
  - [ ] Write a test that shows the lost wakeup, or describe how to see it.

### T2. ESP32-S3: accept only level 1 interrupt lines

- File: `runtime/interrupt/interrupt_esp32s3.go`.
- Size: about 15 lines.
- Change: `Enable` returns an error for a line that is not level 1, because
  other levels go to stub vectors.
- Work before the PR:
  - [ ] Cite ESP-IDF `components/esp_hw_support/port/esp32s3/esp_cpu_intr.c`.

### T3. ESP32-S3: core voltage for 240 MHz and the ROM CPU frequency

- File: `runtime_esp32s3.go`.
- Size: about 140 lines.
- Change: set the LDO voltage from the eFuse values before the clock goes to
  240 MHz, and call `ets_update_cpu_frequency`, as ESP-IDF v5.3.2 does.
- Work before the PR:
  - [ ] Separate this change from the radio. Today the radio needs it, but it is correct for all programs.
  - [ ] Test on more than one chip revision, because the eFuse fields change between revisions.

### T4. ESP32 I2C: internal pull-ups

- Files: `machine_esp32xx_i2c.go`, `machine_esp32c3.go`, `machine_esp32c6.go`, `machine_esp32s3.go`.
- Size: about 40 lines.
- Change: an option to enable the internal pull-ups on SDA and SCL.
- Work before the PR:
  - [ ] Make it an `I2CConfig` option, as `enable_internal_pullup` in ESP-IDF. Today it is always on.
  - [ ] Check it with the new GPIO STOP bus clear of upstream.

### T5. Let -gc=custom work with -scheduler=cores

- Files: `gc_custom.go`, `gc_stack_cores.go`.
- Size: about 30 lines.
- Change: `markCurrentGoroutineStack` and `markRoot` in the custom GC
  interface, and the build tag of `gc_stack_cores.go`.
- Work before the PR:
  - [ ] Explain that this completes the `gc.custom` interface. Do not depend on gcshim in the PR.

### T6. Read TZ without an allocation with -gc=none

- Files: `env_gc.go`, `env_gcnone.go`.
- Size: about 25 lines.

### T7. Assemble .S files in packages

- Files: `loader`, `builder`.
- Size: about 40 lines, and a test.
- Change: TinyGo compiles only the C files of a package now. With this change,
  the DSP and NN assembly can leave `extra-files` of `targets/esp32s3.json`.
  Today every ESP32-S3 build assembles it.
- Work before the PR:
  - [ ] Ask in an issue first. This changes how packages build.

## Phase 2: ESP32-S3 interrupts and the coprocessor

These PRs change the interrupt code. The review will be careful.

### T8. Interrupt handler: spill all windows at entry

- File: `targets/esp32s3-interrupts.S`.
- Size: about 80 lines of assembly.
- Change: spill the register windows at entry, as `_xt_context_save` in
  ESP-IDF, and save `LBEG`, `LEND` and `LCOUNT`. This replaces the upstream
  fix `f67060d5`.
- Work before the PR:
  - [ ] Write a case that fails with the upstream fix: for example, a deep handler that interrupts a loop.
  - [ ] Show that the two fixes cannot both be in the handler (see the v0.44.0 merge).

### T9. PIE coprocessor: enable it and save its state

- Files: `src/device/esp/esp32s3.S`, the task switch code.
- Size: about 60 lines, and new code for the save.
- Change: set `CPENABLE` for the FPU and the PIE on both cores.
- Work before the PR:
  - [ ] Write the save of the PIE registers. Today no code saves them at a goroutine switch or in an interrupt, so two goroutines that use DSP or NN can corrupt the state of each other.
  - [ ] Use a lazy save, as ESP-IDF does with the coprocessor exception.

## Phase 3: the cores scheduler

Upstream owns this design. Open an issue before each PR.

### T10. Wake tasks from interrupt handlers

- Files: `scheduler_cores.go`, `chan.go`, `time.go`, `runtime_esp32s3_cores.go`,
  `runtime_rp2.go`, `gc_blocks.go` and about 4 more.
- Size: about 200 lines.
- Change: an interrupt puts the task on a list of its core. The scheduler
  takes the list. The GC marks the tasks on the lists. Channel locks are spin
  locks with the cores scheduler.
- Work before the PR:
  - [ ] Show the problem: `scheduleTask` in an interrupt takes the scheduler lock, which the interrupted code can hold.
  - [ ] Add a test on `riscv-qemu` with `-scheduler=cores` if possible.
  - [ ] This is the most important and the most difficult PR of the plan.

### T11. Sleep with waiti

- File: `runtime_esp32s3_cores.go`.
- Size: about 60 lines.
- Depends on: T10.
- Change: the sleeping core waits with `waiti`, and the timer alarm goes to
  that core only. Upstream uses a busy loop now.
- Work before the PR:
  - [ ] Measure the current and the wake latency on a board. Upstream accepted the busy loop, so numbers are necessary.

### T12. LockOSThread keeps a goroutine on its core

- Files: `affinity_cores.go`, `scheduler_cores.go`, `pin_notcores.go`, the `arch_*.go` files.
- Size: about 90 lines.
- Work before the PR:
  - [ ] Propose it as an addition to `machine.AssignCore`, which upstream marks as experimental.

### T13. Peterson lock and -s32c1i for PSRAM targets

- Files: `runtime_esp32s3_cores.go`, `targets/esp32s3-psram-octal.json`.
- Size: about 80 lines.
- Depends on: #5554.
- Change: `S32C1I` is not atomic on PSRAM, so the PSRAM targets need locks
  without compare-and-swap.
- Work before the PR:
  - [ ] Move `-s32c1i` from `targets/esp32s3.json` to the PSRAM targets. On all ESP32-S3 builds, it makes each atomic a function call.
  - [ ] Cite `esp_cpu_compare_and_set` in ESP-IDF.

## Phase 4: PSRAM

### T14. Finish #5554 with its author

- Do not open a second PR. #5554 is by digitalentity and has the commit
  `esp32s3/psram: address review items 1-5` of this fork.
- Size: about 1,100 lines.
- Open review items from deadprogram:
  - [ ] Each ESP32-S3 image grows by about 124 KB of zero padding.
  - [ ] The `.psram` section is not a GC root. The arena tests of this fork show what that does.
  - [ ] `zeroInitPSRAM` does not check the chip density.
  - [ ] The `Cache_Dbus_MMU_Set` error panics while the DCache is off.
  - [ ] `initPSRAM` runs before the serial port.
  - [ ] The docs name a QSPI driver that is not in the PR.
- Other work:
  - [ ] Rebase on the v0.44.0 flash layout. Upstream took a different large-flash fix (`03503292`), and the merge of v0.44.0 into this fork shows the conflicts.
  - [ ] Keep the PSRAM GC spans of gcshim out of #5554.

## Phase 5: Linux shared libraries

### T15. Linux: syscall override and the musl network sources

- Files: `loader/goroot.go`, `builder/musl.go`, `src/syscall/goroot/rlimit.go`.
- Size: about 120 lines, with tests.

### T16. Linux: -buildmode=c-shared

- Files: `builder/build.go`, `builder/tools.go`, `compileopts/config.go`,
  `compiler`, `runtime_unix.go`, `asm_amd64.S`, `asm_arm64.S`,
  `task_stack_amd64.S`, `task_stack_arm64.S`.
- Size: about 200 lines, with tests.
- Depends on: T15.
- Work before the PR:
  - [ ] Today it allows only `-scheduler=none` and `-gc=none` or `-gc=leaking`. Upstream will ask if a GC can work. Write the reason or add GC support.

## What stays in this fork

- [ ] The radio seam and its linker changes in `targets/esp32s3.ld`: the BT ROM
  symbols and the Wi-Fi sleep sections. Each upstream merge can conflict here.
- [ ] The network, TLS, crypto, WPA/SAE and BLE host packages.
- [ ] The memory packages, the gcshim collector and `tools/arenacheck`.
- [ ] DSP and NN, until T7 and T9 are merged.

## Order

```
Phase 1 (T1 to T7): any order, now
T8  -> T9 -> DSP and NN as a module (with T7)
T10 -> T11
#5554 (T14) -> T13
T15 -> T16
T12: after an issue
```

## Sources

- [Adding Arena Allocator? (#4434)](https://github.com/tinygo-org/tinygo/issues/4434)
- [Support discontinuous memory (#5518)](https://github.com/tinygo-org/tinygo/issues/5518)
- [feat(esp32s3): add PSRAM support (#5554)](https://github.com/tinygo-org/tinygo/pull/5554)
- [esp32s3: fix booting large DROM images (#5567)](https://github.com/tinygo-org/tinygo/pull/5567)
