#include "wasm_ptr.h"

// memory.atomic.wait32 and memory.atomic.notify for the wasm cores.
// WebAssembly threads proposal, atomic memory instructions.

.globl  tinygo_atomic_wait32
.type   tinygo_atomic_wait32,@function
tinygo_atomic_wait32: // func atomicWait32(addr *uint32, expected uint32, timeout int64) int32
    .functype tinygo_atomic_wait32 (PTR, i32, i64) -> (i32)
    local.get 0
    local.get 1
    local.get 2
    memory.atomic.wait32 0
    end_function

.globl  tinygo_atomic_notify
.type   tinygo_atomic_notify,@function
tinygo_atomic_notify: // func atomicNotify(addr *uint32, count uint32) uint32
    .functype tinygo_atomic_notify (PTR, i32) -> (i32)
    local.get 0
    local.get 1
    memory.atomic.notify 0
    end_function

// scanCurrentStack for gc_stack_cores.go: wasm keeps no pointers in
// registers, so the stack is scanned from where it is.
.functype tinygo_scanstack (PTR) -> ()
.globaltype __stack_pointer, PTR

.globl  tinygo_scanCurrentStack
.type   tinygo_scanCurrentStack,@function
tinygo_scanCurrentStack: // func scanCurrentStack()
    .functype tinygo_scanCurrentStack () -> ()
    global.get __stack_pointer
    call tinygo_scanstack
    end_function
