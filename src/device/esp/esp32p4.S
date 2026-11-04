// The ROM bootloader has loaded the whole program into L2MEM and jumps here.

.section .init
.global call_start_cpu0
.type call_start_cpu0,@function
call_start_cpu0:
    j _start

// Traps and non-vectored interrupts jump to the MTVEC base, which must be
// aligned to 64 bytes. See ESP-IDF components/riscv/vectors_clic.S.
.section .text.exception_vectors
.global _vector_table
.type _vector_table,@function
.balign 64
_vector_table:
    j handleInterruptASM
.size _vector_table, .-_vector_table
