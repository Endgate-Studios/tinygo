// Startup for the BCM2710 (Cortex-A53) as the firmware's armstub8 enters it: core 0
// only, at EL2, with the MMU off. Cores 1 to 3 wait in the armstub's spin table at
// 0xE0, 0xE8 and 0xF0 until the cores scheduler starts them (armstub8.S, spin_cpu1 to 3).

#define SCTLR_EL1_RES1  0x30d00800
#define SCTLR_M         (1 << 0)
#define SCTLR_C         (1 << 2)
#define SCTLR_I         (1 << 12)

// MAIR attributes: 0 Device-nGnRnE, 1 Normal write-back, 2 Normal non-cacheable.
#define MAIR_VALUE      0x44ff00

// TCR_EL1: T0SZ=32 (4 GB), walks write-back and inner shareable, 4 KB granule, TTBR1 off.
#define TCR_VALUE       ((32 << 0) | (1 << 8) | (1 << 10) | (3 << 12) | (1 << 23))

#define DESC_TABLE      0x3
#define DESC_BLOCK      0x1
#define DESC_AF         (1 << 10)
#define DESC_ISH        (3 << 8)
#define DESC_XN         ((1 << 53) | (1 << 54))
#define DESC_DEVICE     (DESC_BLOCK | DESC_AF | DESC_XN | (0 << 2))
#define DESC_NORMAL     (DESC_BLOCK | DESC_AF | DESC_ISH | (1 << 2))
#define DESC_UNCACHED   (DESC_BLOCK | DESC_AF | DESC_ISH | DESC_XN | (2 << 2))

#define PERIPHERAL_BASE 0x3f000000
#define DMA_START       0x10000000
#define DMA_END         0x10800000

// to_el1 leaves EL2 for EL1 at label, with the counters and FP/SIMD open to EL1.
.macro to_el1 label
    mrs     x1, CurrentEL
    lsr     x1, x1, #2
    cmp     x1, #2
    b.ne    \label
    mov     x1, #3
    msr     cnthctl_el2, x1
    msr     cntvoff_el2, xzr
    mov     x1, #(1 << 31)
    msr     hcr_el2, x1
    mov     x1, #0x33ff
    msr     cptr_el2, x1
    msr     hstr_el2, xzr
    ldr     x1, =SCTLR_EL1_RES1
    msr     sctlr_el1, x1
    mov     x1, #0x3c5
    msr     spsr_el2, x1
    adr     x1, \label
    msr     elr_el2, x1
    eret
.endm

// el1_setup turns on FP/SIMD and sets the vectors.
.macro el1_setup
    mov     x1, #(3 << 20)
    msr     cpacr_el1, x1
    ldr     x1, =exception_vectors
    msr     vbar_el1, x1
    isb
.endm

// mmu_on starts the MMU and caches with the tables core 0 built.
.macro mmu_on
    ldr     x1, =MAIR_VALUE
    msr     mair_el1, x1
    ldr     x1, =TCR_VALUE
    msr     tcr_el1, x1
    ldr     x1, =page_table_l1
    msr     ttbr0_el1, x1
    dsb     ish
    isb
    tlbi    vmalle1
    dsb     ish
    isb
    ldr     x1, =(SCTLR_EL1_RES1 | SCTLR_M | SCTLR_C | SCTLR_I)
    msr     sctlr_el1, x1
    isb
.endm

.section .text.boot, "ax"
.global _start
_start:
    mrs     x1, mpidr_el1
    and     x1, x1, #3
    cbnz    x1, park
    to_el1  el1

el1:
    el1_setup
    ldr     x1, =_stack_top
    mov     sp, x1

    ldr     x1, =_sbss
    ldr     x2, =_ebss
1:  cmp     x1, x2
    b.hs    2f
    stp     xzr, xzr, [x1], #16
    b       1b
2:

    // Level 1: the first GB through the level 2 table, the second GB (local peripherals) as device.
    ldr     x1, =page_table_l1
    ldr     x2, =page_table_l2
    orr     x3, x2, #DESC_TABLE
    str     x3, [x1]
    ldr     x3, =(0x40000000 | DESC_DEVICE)
    str     x3, [x1, #8]

    // Level 2: 512 blocks of 2 MB.
    mov     x3, #0
    ldr     x4, =PERIPHERAL_BASE
    ldr     x5, =DMA_START
    ldr     x6, =DMA_END
3:  lsl     x7, x3, #21
    cmp     x7, x4
    b.hs    4f
    cmp     x7, x5
    b.lo    5f
    cmp     x7, x6
    b.hs    5f
    ldr     x8, =DESC_UNCACHED
    b       6f
4:  ldr     x8, =DESC_DEVICE
    b       6f
5:  ldr     x8, =DESC_NORMAL
6:  orr     x7, x7, x8
    str     x7, [x2, x3, lsl #3]
    add     x3, x3, #1
    cmp     x3, #512
    b.lo    3b

    mmu_on
    bl      main
park:
    wfe
    b       park

// Cores 1 to 3 start here when the runtime writes this address to the armstub's spin table.
.global tinygo_secondaryStart
tinygo_secondaryStart:
    to_el1  secondary_el1
secondary_el1:
    el1_setup
    mrs     x0, mpidr_el1
    and     x0, x0, #3
    ldr     x1, =secondary_stacks
    ldr     x1, [x1, x0, lsl #3]
    mov     sp, x1
    mmu_on
    mrs     x0, mpidr_el1
    and     x0, x0, #3
    bl      tinygo_runSecondary
    b       park

.section .rodata.secondary_stacks, "a"
.balign 8
secondary_stacks:
    .quad _stack_top
    .quad _stack1_top
    .quad _stack2_top
    .quad _stack3_top

// Every exception goes to handleException(kind, esr, elr, far), which doesn't return.
.macro vector kind
    .balign 0x80
    mov     x0, #\kind
    b       exception_common
.endm

.section .text.exception_vectors, "ax"
.balign 2048
exception_vectors:
    vector 0
    vector 1
    vector 2
    vector 3
    vector 4
    .balign 0x80
    b       irq_entry
    vector 6
    vector 7
    vector 8
    vector 9
    vector 10
    vector 11
    vector 12
    vector 13
    vector 14
    vector 15

// irq_entry saves what a call may change, runs handleInterrupt and returns to where the IRQ came.
irq_entry:
    sub     sp, sp, #608
    stp     x0, x1, [sp, #0]
    stp     x2, x3, [sp, #16]
    stp     x4, x5, [sp, #32]
    stp     x6, x7, [sp, #48]
    stp     x8, x9, [sp, #64]
    stp     x10, x11, [sp, #80]
    stp     x12, x13, [sp, #96]
    stp     x14, x15, [sp, #112]
    stp     x16, x17, [sp, #128]
    stp     x18, x29, [sp, #144]
    mrs     x0, elr_el1
    mrs     x1, spsr_el1
    stp     x30, x0, [sp, #160]
    mrs     x0, fpcr
    mrs     x2, fpsr
    stp     x1, x0, [sp, #176]
    str     x2, [sp, #192]
    stp     q0, q1, [sp, #208]
    stp     q2, q3, [sp, #240]
    stp     q4, q5, [sp, #272]
    stp     q6, q7, [sp, #304]
    stp     q16, q17, [sp, #336]
    stp     q18, q19, [sp, #368]
    stp     q20, q21, [sp, #400]
    stp     q22, q23, [sp, #432]
    stp     q24, q25, [sp, #464]
    stp     q26, q27, [sp, #496]
    stp     q28, q29, [sp, #528]
    stp     q30, q31, [sp, #560]
    bl      handleInterrupt
    ldp     q30, q31, [sp, #560]
    ldp     q28, q29, [sp, #528]
    ldp     q26, q27, [sp, #496]
    ldp     q24, q25, [sp, #464]
    ldp     q22, q23, [sp, #432]
    ldp     q20, q21, [sp, #400]
    ldp     q18, q19, [sp, #368]
    ldp     q16, q17, [sp, #336]
    ldp     q6, q7, [sp, #304]
    ldp     q4, q5, [sp, #272]
    ldp     q2, q3, [sp, #240]
    ldp     q0, q1, [sp, #208]
    ldr     x2, [sp, #192]
    ldp     x1, x0, [sp, #176]
    msr     fpsr, x2
    msr     fpcr, x0
    msr     spsr_el1, x1
    ldp     x30, x0, [sp, #160]
    msr     elr_el1, x0
    ldp     x18, x29, [sp, #144]
    ldp     x16, x17, [sp, #128]
    ldp     x14, x15, [sp, #112]
    ldp     x12, x13, [sp, #96]
    ldp     x10, x11, [sp, #80]
    ldp     x8, x9, [sp, #64]
    ldp     x6, x7, [sp, #48]
    ldp     x4, x5, [sp, #32]
    ldp     x2, x3, [sp, #16]
    ldp     x0, x1, [sp, #0]
    add     sp, sp, #608
    eret

exception_common:
    mrs     x1, esr_el1
    mrs     x2, elr_el1
    mrs     x3, far_el1
    bl      handleException
    b       park

.section .bss.page_tables, "aw", @nobits
.balign 4096
page_table_l1:
    .space 4096
page_table_l2:
    .space 4096
