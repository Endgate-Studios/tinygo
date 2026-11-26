# Wasm threads: -scheduler=cores on wasm, Web Workers as the cores

Goal: TinyGo (minigo) wasm that runs goroutines on several threads in the
browser, for a graphics.gd game's web build. Worktree tinygo-wasm-cores, branch
wasm-cores, from rpi-zero2w's last commit; ~/tinygo and minigo untouched.

The cores scheduler already runs on esp32s3, esp32p4, rp2 and bcm2710: each
platform gives currentCPU, startSecondaryCores, schedulerWake(Core),
gcPauseCore, coreStackTop and its locks; gcshim has alloc, lock and roots
for cores. Wasm is one more platform: the cores are Web Workers (Node's
worker_threads in the tests) sharing one WebAssembly.Memory
(SharedArrayBuffer: the page cross-origin isolated, as webserve's is).

Limits to keep in view: the browser's main thread (core 0, where Godot
calls Go) can't block (Atomics.wait is refused there), so it spins or goes
back to the event loop; wasm locals aren't in memory, so stacks are found
by TinyGo's portable stack chain, one per core; a core in a loop that
neither allocates nor yields holds up the GC until it does (no signals).

## Steps (each test-first, checked under Node, then a headless browser)

- [x] **The target**: 2026-11-24, targets/wasm-cores.json (wasm's with
  +atomics, shared imported memory up to 1 GiB, a tinygo.wasm.cores tag;
  the asyncify scheduler until the runtime port). It took the compiler:
  libraries built for wasm with atomics get -matomics and their own cache
  name (Config.WasmAtomics), since shared memory links only atomic
  objects. tests/wasmcores/run.sh: hello builds, imports a shared
  env.memory of 2..16384 pages; plain -target=wasm still builds and runs.
  The worktree's compiler (build/tinygo.exe, GOFLAGS=-buildvcs=false)
  reads ~/tinygo's submodules, llvm-project and llvm-build by junctions.
  Found on the way: cores can't use native stack switching on wasm; the
  compiler applies asyncify only when the scheduler is asyncify, so the
  runtime port makes cores on wasm mean the cores run queue with asyncify
  switching (the compiler's asyncify checks taught about it).
- [x] **The glue**: 2026-11-24, targets/wasm_exec_cores.js (GoCores on
  wasm_exec.js's Go: the shared memory made from the module's own import
  limits; tinygo_cores.start_core(core, stackTop) answered by a Worker on
  the same module and memory, its __stack_pointer set, entered at the
  exported tinygo_core_entry; Node's worker_threads or a browser Worker)
  and wasm_exec_cores_node.js. The target exports its memory and
  __stack_pointer. tests/wasmcores twocores: core 0 starts core 1 on a stack
  of its own, both print, core 0 reads what core 1 wrote (42): two threads,
  one memory.
- [x] **Thread-locals per core**: 2026-11-24, the target exports
  __wasm_init_tls, __tls_size and __tls_align; start_core(core, stackTop,
  tls) gives each core a TLS block, initialized before its entry. Found:
  without it a worker's thread-locals sat at address 0 in the shared
  memory. tests/wasmcores tls: a fresh core sees a _Thread_local's initial
  value (7), sets its own, core 0's untouched.
- [x] **Per-core runtime state**: 2026-11-24. A //go:threadlocal pragma
  (compiler/symbol.go: the LLVM global thread-local; on wasm-cores, one per
  core by __tls_base). runtime.stackChainStart, runtime.savedStackPointer,
  task.currentTask and task.panicStackState declared thread-local on
  tinygo.wasm.cores (pairs of files, the plain wasm ones unchanged); the
  compiler's tinygo_panic_rewinding declaration thread-local with wasm
  atomics; task_asyncify_wasm.S reads and writes tinygo_rewinding and
  tinygo_panic_rewinding through __tls_base under __wasm_atomics__, in
  .tbss. Tests: gotls (a //go:threadlocal Go variable: core 1 sees the
  initial 7, sets its own, core 0 keeps 0, a plain global shared) and
  perstate (the three globals at a different address on each core); plain
  -target=wasm builds and runs. The asm's thread-local flags are proved by
  the next step (two cores rewinding at once).
- [x] **The cores scheduler on wasm**: 2026-11-25. -scheduler=cores on
  wasm-cores: the compiler builds its goroutines as asyncify's
  (Config.CompilerScheduler; the asyncify pass, panic unwinding), the
  runtime runs the cores scheduler (scheduler_cores.go, its run and NumCPU
  moved out for the ports) with asyncify switching (task_asyncify*.go
  for cores on wasm; Pause with the cores handshake, task_asyncify_cores.go).
  runtime_wasm_cores.go: core 0 (the page) starts the cores and goes back
  to the event loop; main and every goroutine run on cores 1 and up, idle
  on memory.atomic.wait32, woken by notify (asm_tinygowasm_cores.S); the
  page waits on an exit word (Atomics.waitAsync). Fixed on the way: each
  core's stack and TLS kept alive and given out before any core starts; a
  lock taken on a core's own stack spins instead of pausing nothing; the
  stack list locked; goroutines pinned to their first core (thread-local
  addresses kept across a pause); workers' syscall/js state; workers'
  output written at once. Measured (Node, 16 threads): 7 CPU-bound jobs
  244 ms one after another, 53 ms on 4 worker cores: 4.6x; goroutines,
  channels, mutexes, wait groups, defers on two cores, 10 runs each.
  Stopgaps, each a later step: panic-unwind none on wasm-cores (asyncify
  panic unwinding breaks across cores: recover doesn't catch yet), 64 MiB
  initial memory (a collection with other cores running isn't safe yet).
- [x] **Panics across cores**: 2026-11-25. Three single-thread
  assumptions in asyncify panic unwinding, fixed: each catch wrapper's
  result global (a value returned through it) thread-local; the panic
  replay's tinygo_rewinding declaration thread-local; the per-core unwind
  signal indexed so it can't panic (a function that can is instrumented by
  asyncify, and unwindPending runs while a replay rewinds). And a deadlock
  with them: an exited goroutine's stack is freed at the core's next
  Resume, not under the scheduler's lock (freeing takes the heap's). Panic
  unwinding is wasm-cores' default again. tests/wasmcores panics: 4
  goroutines recovering 200 panics each while 4 others return values
  through wrappers under a contended lock, 4 worker cores, 5 runs: 800 of
  800 recovered, no wrong values. Each run.sh run now has a timeout; a
  stuck program's scheduler state is printed with WASMCORES_DEBUG
  (tinygo_cores_debug).
- [x] **Locks**: 2026-11-25, tests/wasmcores stress: Mutex (8 goroutines
  x 5000), RWMutex (writers and readers, no torn pair), unbuffered and
  buffered channels, select with a timeout never taken, Cond hand-offs,
  Once, atomics, 2000 short goroutines, 64 timers; exact counts, on 2, 4
  and 7 worker cores, 3 runs each: all pass (they're the cores scheduler's
  own, sound on wasm since the earlier steps). A dying core now ends the
  program with exit 2 (it used to leave Node to quit with 0). On one worker
  core the 2000 goroutines outgrow the 64 MiB heap and a collection on a
  worker core scans the wrong stack range: the GC step's.
- [x] **The GC**: 2026-11-25. wasm cores is a port of gc_stack_cores.go,
  as esp32s3 is: gcPauseRequest/gcSignalWait, gcInterruptHandler,
  gcPauseCore/gcSignalCore, coreStackTop, and the system stack through
  runtime.systemStackPtr. With no interrupt between cores, a core takes a
  pause request in spinLoopWait (lock waits, the heap lock, idle waits woken
  by signalCore); the page and unstarted cores answer at once. Goroutine
  stacks are conservative objects as on native. Runtime locks (task.PMutex)
  wait on the core: the compiler takes allocation as never suspending. The
  64 MiB initial memory is gone. gcmin, gcstress (21 to 67 collections, 1 to
  7 MB heap, nothing broken), allocdefer on 1, 2, 4 and 8 cores: all pass.
- [x] **The browser**: 2026-11-25. tests/wasmcores/browser runs a wasm in
  headless Chrome on a cross-origin isolated local page (workers as the
  cores, output posted back in order); run_browser.sh is the suite there.
  Browsers fill and decode no view of a shared memory, unlike Node: the glue
  copies for crypto.getRandomValues and TextDecoder. All pass on wasm-cores
  (3.6x on 4 workers).
- [x] **wasm64**: 2026-11-25. Go's int is the pointer's width in TinyGo, 32
  bits on wasm32; targets/wasm64.json and wasm64-cores.json (memory64, a
  64-bit table) make it 64 as stock Go's. Pointer-width assembly
  (runtime/wasm_ptr.h; LLVM's assembler takes no i64 call_indirect index, so
  a C helper calls), the builder's archive index reads wasm64 objects, the
  wasmbuiltins libc (wasi-libc's musl is wasm32 only: Go's math, randomness
  from the host), memory intrinsics by width, and glue that takes BigInt
  pointers. Node 22's V8 has no 64-bit tables: Chrome runs it. 31 testdata
  programs match; finalizeridle keeps an object alive longer (a stale
  pointer). The whole suite passes in Chrome on wasm64-cores (4.1x on 4).
- [ ] **graphics.gd under minigo**: the reflect.Value.Call gap (graphics.gd
  patched to call through interfaces, web/gd_tinygo_patch.py); Go calls
  into Godot only from core 0.
  reflect.Value.Call, CallSlice and the func type's In/Out/NumIn/NumOut/
  IsVariadic now work (compiler: a call stub per func type descriptor;
  tests/reflectcall on wasm, wasm-cores and built for linux/amd64); Method
  and MethodByName are still unimplemented.

- [ ] **A game on cores**: a graphics.gd game's web build with cores, its
  pure work (zombie AI, paths) on goroutines across cores, fps measured.

## Rules

Nothing committed; ~/tinygo, its branch and minigo untouched. AGENTS.md:
brief comments, no tool attributions. Headless checks only.
