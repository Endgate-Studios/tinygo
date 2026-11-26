// Pointers are i32 on wasm32, i64 on wasm64 (memory64, with a 64-bit table:
// a function index is a pointer too).
#if defined(__wasm64__)
#define PTR i64
#define PTR_LOAD i64.load
#define PTR_STORE i64.store
#define PTR_CONST i64.const
#define PTR_ADD i64.add
#define PTR_SIZE 8
#define PTR_SIZE_2 16
#define PTR_SIZE_3 24
// LLVM's wasm assembler takes a call_indirect index as i32 even with a
// 64-bit table; a C helper (targets/task_asyncify_wasm64.c) makes the call instead.
#define CALL_ENTRY call tinygo_call_entry
#else
#define PTR i32
#define PTR_LOAD i32.load
#define PTR_STORE i32.store
#define PTR_CONST i32.const
#define PTR_ADD i32.add
#define PTR_SIZE 4
#define PTR_SIZE_2 8
#define PTR_SIZE_3 12
#define CALL_ENTRY call_indirect (i32) -> ()
#endif
