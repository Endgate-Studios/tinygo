#include "../../runtime/wasm_ptr.h"

.globaltype __stack_pointer, PTR
#if defined(__wasm64__)
.functype tinygo_call_entry (i64, i64) -> ()
#endif
#if defined(__wasm_atomics__)
.globaltype __tls_base, PTR
#endif

.functype start_unwind (PTR) -> ()
.import_module start_unwind, asyncify
.import_name start_unwind, start_unwind
.functype stop_unwind () -> ()
.import_module stop_unwind, asyncify
.import_name stop_unwind, stop_unwind
.functype start_rewind (PTR) -> ()
.import_module start_rewind, asyncify
.import_name start_rewind, start_rewind
.functype stop_rewind () -> ()
.import_module stop_rewind, asyncify
.import_name stop_rewind, stop_rewind

.global  tinygo_unwind
.hidden  tinygo_unwind
.type    tinygo_unwind,@function
tinygo_unwind: // func (state *stackState) unwind()
    .functype tinygo_unwind (PTR) -> ()
    // Check if we are rewinding.
#if defined(__wasm_atomics__)
    global.get __tls_base
    PTR_CONST tinygo_rewinding@TLSREL
    PTR_ADD
    i32.load8_u 0
#else
    PTR_CONST 0
    i32.load8_u tinygo_rewinding
#endif
    if // if tinygo_rewinding {
    // Stop rewinding.
    call stop_rewind
#if defined(__wasm_atomics__)
    global.get __tls_base
    PTR_CONST tinygo_panic_rewinding@TLSREL
    PTR_ADD
    i32.load8_u 0
#else
    PTR_CONST 0
    i32.load8_u tinygo_panic_rewinding
#endif
    if
    // Trap as soon as the original frames have been reconstructed.
    unreachable
    end_if
#if defined(__wasm_atomics__)
    global.get __tls_base
    PTR_CONST tinygo_rewinding@TLSREL
    PTR_ADD
    i32.const 0
    i32.store8 0
#else
    PTR_CONST 0
    i32.const 0
    i32.store8 tinygo_rewinding // tinygo_rewinding = false;
#endif
    else
    // Save the C stack pointer (destination structure pointer is in local 0).
    local.get 0
    global.get __stack_pointer
    PTR_STORE PTR_SIZE // state.csp = getCurrentStackPointer()
    // Ask asyncify to unwind.
    // When resuming, asyncify will return this function with tinygo_rewinding set to true.
    local.get 0
    call start_unwind // asyncify.start_unwind(state)
    end_if
    return
    end_function

.global tinygo_launch
.hidden tinygo_launch
.type tinygo_launch,@function
tinygo_launch: // func (state *state) launch()
    .functype tinygo_launch (PTR) -> ()
    // Switch to the goroutine's C stack.
    global.get __stack_pointer // prev := getCurrentStackPointer()
    local.get 0
    PTR_LOAD PTR_SIZE_3
    global.set __stack_pointer // setStackPointer(state.csp)
    // Get the argument pack and entry pointer.
    local.get 0
    PTR_LOAD PTR_SIZE // args := state.args
    local.get 0
    PTR_LOAD 0 // fn := state.entry
    // Launch the entry function.
    CALL_ENTRY // fn(args)
    // Stop unwinding.
    call stop_unwind
    // Restore the C stack.
    global.set __stack_pointer // setStackPointer(prev)
    return
    end_function

.global  tinygo_rewind
.hidden  tinygo_rewind
.type    tinygo_rewind,@function
tinygo_rewind: // func (state *state) rewind()
    .functype tinygo_rewind (PTR) -> ()
    // Switch to the goroutine's C stack.
    global.get __stack_pointer // prev := getCurrentStackPointer()
    local.get 0
    PTR_LOAD PTR_SIZE_3
    global.set __stack_pointer // setStackPointer(state.csp)
    // Get the argument pack and entry pointer.
    local.get 0
    PTR_LOAD PTR_SIZE // args := state.args
    local.get 0
    PTR_LOAD 0 // fn := state.entry
    // Prepare to rewind.
#if defined(__wasm_atomics__)
    global.get __tls_base
    PTR_CONST tinygo_rewinding@TLSREL
    PTR_ADD
    i32.const 1
    i32.store8 0
#else
    PTR_CONST 0
    i32.const 1
    i32.store8 tinygo_rewinding // tinygo_rewinding = true;
#endif
    local.get 0
    PTR_CONST PTR_SIZE_2
    PTR_ADD
    call start_rewind // asyncify.start_rewind(&state.stackState)
    // Launch the entry function.
    // This will actually rewind the call stack.
    CALL_ENTRY // fn(args)
    // Stop unwinding.
    call stop_unwind
    // Restore the C stack.
    global.set __stack_pointer // setStackPointer(prev)
    return
    end_function

.global tinygo_panic_replay
.hidden tinygo_panic_replay
.type tinygo_panic_replay,@function
tinygo_panic_replay:
    .functype tinygo_panic_replay (PTR, PTR, PTR) -> ()
    local.get 1
    local.get 0
    CALL_ENTRY
    return
    end_function

.global tinygo_set_panic_rewind_stack_pointer
.hidden tinygo_set_panic_rewind_stack_pointer
.type tinygo_set_panic_rewind_stack_pointer,@function
tinygo_set_panic_rewind_stack_pointer:
    .functype tinygo_set_panic_rewind_stack_pointer (PTR, PTR) -> ()
    local.get 0
    global.set __stack_pointer
    return
    end_function

        .hidden tinygo_rewinding                # @tinygo_rewinding
        .type   tinygo_rewinding,@object
#if defined(__wasm_atomics__)
        .section        .tbss.tinygo_rewinding,"T",@
#else
        .section        .bss.tinygo_rewinding,"",@
#endif
        .globl  tinygo_rewinding
tinygo_rewinding:
        .int8   0                               # 0x0
        .size   tinygo_rewinding, 1

        .hidden tinygo_panic_rewinding
        .type   tinygo_panic_rewinding,@object
#if defined(__wasm_atomics__)
        .section        .tbss.tinygo_panic_rewinding,"T",@
#else
        .section        .bss.tinygo_panic_rewinding,"",@
#endif
        .globl  tinygo_panic_rewinding
tinygo_panic_rewinding:
        .int8   0
        .size   tinygo_panic_rewinding, 1
