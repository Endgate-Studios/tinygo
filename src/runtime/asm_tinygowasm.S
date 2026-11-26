#include "wasm_ptr.h"

.globaltype __stack_pointer, PTR

.global  tinygo_getCurrentStackPointer
.hidden  tinygo_getCurrentStackPointer
.type    tinygo_getCurrentStackPointer,@function
tinygo_getCurrentStackPointer: // func getCurrentStackPointer() uintptr
    .functype tinygo_getCurrentStackPointer() -> (PTR)
    global.get __stack_pointer
    return
    end_function
