// Startup for the BCM2710 (Cortex-A53) as the firmware's armstub8 enters it: core 0
// only, at EL2, with the MMU off. Cores 1 to 3 wait in the armstub's spin table.

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

.section .text.boot, "ax"
.global _start
_start:
    mrs     x1, mpidr_el1
    and     x1, x1, #3
    cbnz    x1, park

    mrs     x1, CurrentEL
    lsr     x1, x1, #2
    cmp     x1, #2
    b.ne    el1

    // Leave EL2 for EL1, with the counters and FP/SIMD open to EL1.
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
    adr     x1, el1
    msr     elr_el2, x1
    eret

el1:
    mov     x1, #(3 << 20)
    msr     cpacr_el1, x1
    ldr     x1, =exception_vectors
    msr     vbar_el1, x1
    isb

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

    bl      main
park:
    wfe
    b       park

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
    vector 5
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
