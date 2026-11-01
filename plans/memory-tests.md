# Plan: unit tests and benchmarks for the memory packages

The unit tests of `src/runtime/memory` must show two things, for each memory
strategy:

- **Safe:** the collector keeps every live object, and each misuse fails in a
  known way.
- **Fast:** the cost of each strategy against the GC heap, with numbers.

The tests must also run after each upstream merge, so that a change in the
runtime that breaks a strategy fails a test. Today, `gcsuite` is the only
check of gcshim, `AddRoot`, `Scope` and `free.Free`, and it is an example
program, not a unit test.

## What exists today

| Package | Tests | Gaps |
|---|---|---|
| `arena` | Heap and Region units, fuzz, misuse, GC visibility, 4 benchmarks | No rotation test, no reset-cycle benchmark |
| `collector` | 24 units, a model test, 2 benchmarks | None for this plan |
| `heap` | 3 units | No early-free test |
| `memory` | Only `TestTypedHelpersOverArena` in `arena` | No tests of its own |
| `free` | None | All |
| `gcshim` | None | All. `gcsuite` has `testRegisteredRoot`, `testFreeShim` and `testArenaOnReservedMemory` |

## Where the tests run

| Name | Command | Collector | Real free | gcshim |
|---|---|---|---|---|
| Host | `tinygo test -gc=conservative` and `-gc=precise` | built-in | No | No |
| Host, real free | `tinygo test -gc=precise -tags=memory_realfree` | built-in | Yes | No |
| Wasm | `tinygo test -target=wasip1 -gc=custom` | gcshim | Yes | Yes |
| Board | `tinygo test -c -target=esp32s3-psram-octal -scheduler=cores -gc=custom -tags=gcshim_psram,gcshim_precise_globals`, then flash and read serial | gcshim, internal RAM and PSRAM | Yes | Yes |
| Board, debug | Board with `-tags=...,memory_debug` | as above, freed memory poisoned | Yes | Yes |

A test that needs gcshim calls `gcshim.Installed()` and skips when it is
false. A test that needs a real free calls `free.IsReal()` and skips when it is
false. Then each package runs in each place, and each place tests what it can.

The Wasm row needs `wasmtime` on PATH. It is not installed on this PC.

## Shared test code

### `runtime/memory/internal/memtest`

- [ ] Move the victim pattern from `src/examples/gcsuite/arena_gc.go` here:
  `PlantVictims`, `ClobberStack`, `ReuseFreed` and `CorruptedVictims`.
  `gcsuite`, `arena`, `gcshim` and `free` use it.
- [ ] `Survives(slots, rounds)`: up to 8 rounds of `runtime.GC` and
  `ReuseFreed`. It returns the number of corrupted victims.
- [ ] `Strategies()`: the table below, with a skip reason for each strategy
  that the build cannot run.

| Name | Alloc | Release | Needs |
|---|---|---|---|
| `gc` | `new`, `make` | the collector | |
| `free` | `heap.New` | `memory.Free`, `memory.FreeSlice` | real free |
| `arena.Heap` | `arena.NewHeap` | `Free` | |
| `arena.Region` | `arena.NewRegion` | `Reset` | |
| `region-root` | `arena.NewRegion` with `gcshim.AddRoot` | `Reset` | gcshim |
| `scope` | `gcshim.NewScope`, `Enter` | `Reset` | gcshim |

Each test and benchmark that compares strategies runs a subtest for each row.

## Tests by package

### `free`

- [ ] `TestNilAndEmpty`: `Free(nil)`, `FreeSlice(nil)` and a slice with
  `cap` 0 do nothing.
- [ ] `TestIsRealMatchesBuild`: `IsReal` is true only with `gc.custom` or
  `memory_realfree`.
- [ ] `TestNoOpWithoutRealFree`: without a real free, `Free` of a GC object
  does not crash, and the object stays usable. This is the regression test for
  upstream `02021b58` and the fork's `edfd4d53`.
- [ ] `TestFreeReturnsMemory`: with a real free, `MemStats.Frees` grows by one
  for each `Free`, and the next allocation of the same size can use the block.
- [ ] `TestFreeSliceReturnsBackingArray`: as above, for `FreeSlice`.
- [ ] `TestEarlyFreeKeepsLiveData`: build a list, free 1000 other objects
  early, collect 3 times, and check the list. From `testHeapStrategyEarlyFree`.
- [ ] `TestFreeForeignPointer`: `Free` of a pointer into a global, a stack
  array or an arena. Write down what happens. The test fixes that behavior.

### `gcshim`

- [ ] `TestInstalled`: true with `-gc=custom`, false with the built-in
  collectors.
- [ ] `TestRegisteredRootKeepsObjects`: the only pointer to each victim is in
  a registered `Region`. All victims survive. On the board, once over a static
  array and once over `.psram`. From `testRegisteredRoot`.
- [ ] `TestRemoveRootLetsObjectsGo`: the same, then `RemoveRoot`. The victims
  are freed.
- [ ] `TestRootLiveRange`: a `Root` whose `Live` covers only half its memory.
  Victims in the live half survive, victims in the other half are freed.
- [ ] `TestMaxRoots`: `MaxRoots` roots register, the next returns
  `ErrTooManyRoots`, and `RemoveRoot` frees a place.
- [ ] `TestScopeAllocatesInside`: between `Enter` and `Exit`, `new` returns
  memory inside the Scope. After `Exit`, it returns heap memory.
- [ ] `TestScopeIsRoot`: the only pointer to a heap object is in a Scope
  object. The heap object survives.
- [ ] `TestScopeReset`: after `Reset`, `Used` is 0, and the next allocation
  starts at the base.
- [ ] `TestScopeOverflowGoesToHeap`: a Scope too small for the allocations
  sends the rest to the heap and counts each one in `Overflows`.
- [ ] `TestScopeOtherGoroutine`: a goroutine that did not call `Enter`
  allocates on the heap.
- [ ] `TestScopePerCore`: with `-scheduler=cores`, a Scope entered on one core
  does not take the allocations of the other core.
- [ ] `TestScopeResetPoison`: with `memory_debug`, the memory of a Scope shows
  the poison pattern after `Reset`.
- [ ] `TestReserveIsNotScanned`: the only pointer to each victim is in
  `Reserve` memory. The victims are freed. This test records the unsafe case,
  so that a change shows.
- [ ] `TestReleaseReturnsMemory`: `Release` gives the memory back, and a later
  `Reserve` of the same size succeeds.
- [ ] `TestReadStatsMatchesMemStats`: `ReadStats` and
  `runtime.ReadMemStats` agree on allocations, frees and collections.
- [ ] `TestInternalRAMFirst`: on the board with `gcshim_psram`, small objects
  go to internal RAM until it is full, then to PSRAM.

### `arena`

- [ ] `TestRotationNoUseAfterReset`: 2 Regions in turn, as `imp/ui` uses
  them. A pointer from rebuild k is used only until rebuild k+1 is done. With
  `memory_debug`, a use after that shows the poison.
- [ ] `TestRegionNeverResetDoesNotGrow`: a Region that is never reset, with the
  same allocations each cycle (as `imp`'s `plainMemory`), must not grow after
  the first cycle when the program allocates only once. The test shows what
  the program must do.

### `memory`

- [ ] `TestNewZeroes` and `TestMakeSliceZeroes` over each strategy.
- [ ] `TestNewNilWhenFull` and `TestMakeSliceNilWhenFull`.
- [ ] `TestMakeSliceOverflow`: `n` times the element size overflows. The
  result is nil, not a short slice.
- [ ] `TestFreeUsesFreer`: `memory.Free` calls the `Freer` of the strategy, and
  does nothing for a strategy without one.

### `heap`

- [ ] `TestEarlyFreeKeepsLiveData`: as in `free`, through `heap.New`.

## Benchmarks

Each benchmark runs a subtest for each strategy of `memtest.Strategies()`. It
reports `ns/op`, `B/op`, `allocs/op`, and with `b.ReportMetric`: collections
per op (`gcs/op`), heap in use after (`heap-B`) and highest heap in use
(`peak-B`).

- [ ] `BenchmarkAllocRelease`: one 64-byte object, allocated and released.
  Extends `arena.BenchmarkAllocFree` with `free`, `region-root` and `scope`.
- [ ] `BenchmarkResetCycle`: one cycle allocates 200 objects of 16 to 700
  bytes with pointers between them, then releases all. This is the shape of a
  decoder song or a UI rebuild.
- [ ] `BenchmarkChurn`: extends `arena.BenchmarkChurn` with `free`.
- [ ] `BenchmarkGCPause`: 256 KiB of live list nodes in each strategy, then one
  forced collection. Extends `arena.BenchmarkGCWithLiveData` with
  `region-root` and `scope`, which the collector scans.
- [ ] `BenchmarkRootScan`: the cost of a collection for each 16 KiB of
  registered root, to show what `AddRoot` costs.
- [ ] Put the benchmarks in a package that imports all strategies, for example
  `runtime/memory/internal/memtest/bench_test.go`.

## Make targets

- [ ] `memory-test`: add `free`, `memory` and `gcshim` to
  `MEMORY_TEST_PACKAGES`, and add the `memory_realfree` run.
- [ ] `memory-test-wasm`: the packages with `-target=wasip1 -gc=custom`. It
  skips with a message when `wasmtime` is not on PATH.
- [ ] `memory-test-board`: build each package's test binary for the board,
  flash it with `esptool`, read serial until `PASS` or `FAIL`, then the same
  with `memory_debug`. `PORT` selects the port.
- [ ] `memory-bench` and `memory-bench-board`: the benchmarks in the same
  places. Write the results in Go benchmark format, so that `benchstat` can
  compare two runs, for example before and after an upstream merge.

## Order

1. `memtest` with the victim pattern and the strategy table.
2. `free` and `memory` tests. They run on the host now.
3. `gcshim` tests, on the board first. Wasm when `wasmtime` is installed.
4. `arena` additions.
5. Benchmarks.
6. Make targets.
7. `gcsuite` uses `memtest`, and keeps only what a unit test cannot do.

## Done when

- `make memory-test` and `make memory-test-board` pass, with and without
  `memory_debug`.
- `make memory-bench-board` gives a number for each strategy.
- The "Fork position" section of `src/runtime/memory/README.md` has these
  numbers.
