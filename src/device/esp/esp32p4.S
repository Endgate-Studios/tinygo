// The ROM bootloader has loaded the RAM part of the program and jumps here. The startup
// code runs from RAM, because the rest of the program can be in flash that isn't mapped yet.
// _flash_map_pages is 0 when the whole program is in RAM.

.section .iram.startup, "ax"
.global call_start_cpu0
.type call_start_cpu0,@function
call_start_cpu0:
    lui  s2, %hi(_flash_map_pages)
    addi s2, s2, %lo(_flash_map_pages)
    beqz s2, 2f
    mv   t0, s2

    // The first page is entry 1 of the flash MMU, at 0x40010000. An entry is the flash page
    // and SOC_MMU_FLASH_VALID. See ESP-IDF soc/esp32p4/include/soc/ext_mem_defs.h.
    lui  t1, %hi(_flash_map_addr)
    lw   t1, %lo(_flash_map_addr)(t1)
    srli t1, t1, 16
    li   t2, 0x1000
    or   t1, t1, t2
    li   t2, 1
    li   t3, 0x5008C380 // SPI_MEM_C_MMU_ITEM_INDEX_REG
    li   t4, 0x5008C37C // SPI_MEM_C_MMU_ITEM_CONTENT_REG
1:
    sw   t2, 0(t3)
    sw   t1, 0(t4)
    addi t1, t1, 1
    addi t2, t2, 1
    addi t0, t0, -1
    bnez t0, 1b

    // The ROM's loader can leave other flash in the caches at these addresses, so drop that, on its stack.
    // Cache_Invalidate_Addr is in ESP-IDF esp_rom/esp32p4/ld/esp32p4.rom.ld, its order in hal/cache_ll.h.
    slli s2, s2, 16
    li   a0, 0x01 // CACHE_MAP_L1_ICACHE_0
    li   a1, 0x40010000
    mv   a2, s2
    li   t0, 0x4fc003e4
    jalr t0
    li   a0, 0x10 // CACHE_MAP_L1_DCACHE
    li   a1, 0x40010000
    mv   a2, s2
    li   t0, 0x4fc003e4
    jalr t0
    li   a0, 0x20 // CACHE_MAP_L2_CACHE
    li   a1, 0x40010000
    mv   a2, s2
    li   t0, 0x4fc003e4
    jalr t0
2:
    lui  t0, %hi(_start)
    addi t0, t0, %lo(_start)
    jr   t0

// The ROM starts core 1 here, when ets_set_appcpu_boot_addr has been given this address.
.section .iram.startup, "ax"
.global call_start_cpu1
.type call_start_cpu1,@function
call_start_cpu1:
    .option push
    .option norelax
    lui  gp, %hi(__global_pointer$)
    addi gp, gp, %lo(__global_pointer$)
    .option pop
    lui  sp, %hi(_stack1_top)
    addi sp, sp, %lo(_stack1_top)
    .weak tinygo_runCore1
    lui  t0, %hi(tinygo_runCore1)
    addi t0, t0, %lo(tinygo_runCore1)
    jr   t0

// tinygo_flashPark holds a core in RAM during another core's flash write. It takes the address of the
// state: 1 says it's parked, 2 lets it go, and 0 says it's gone. Its stack is its own, in RAM, as the
// caller's can be in PSRAM. While it waits it calls tinygo_flashParkPoll, if a program has one: what has
// to go on during the write that an interrupt would otherwise do, such as starting a display's DMA on
// its next frame. That has to be in RAM, call nothing that isn't, and touch no PSRAM or flash.
.global tinygo_flashPark
.type tinygo_flashPark,@function
tinygo_flashPark:
    mv   t2, sp
    lui  sp, %hi(tinygo_flashParkStackTop)
    addi sp, sp, %lo(tinygo_flashParkStackTop)
    addi sp, sp, -16
    sw   t2, 0(sp)
    sw   ra, 4(sp)
    sw   s0, 8(sp)
    mv   s0, a0
    li   t0, 1
    fence
    sw   t0, 0(s0)
    fence
1:
    .weak tinygo_flashParkPoll
    lui  t0, %hi(tinygo_flashParkPoll)
    addi t0, t0, %lo(tinygo_flashParkPoll)
    beqz t0, 2f
    jalr t0
2:
    lw   t0, 0(s0)
    li   t1, 2
    bne  t0, t1, 1b
    fence
    sw   zero, 0(s0)
    fence
    lw   s0, 8(sp)
    lw   ra, 4(sp)
    lw   sp, 0(sp)
    ret

// The parked core's stack, in RAM.
.section .bss.tinygo_flashParkStack, "aw", @nobits
.balign 16
tinygo_flashParkStack:
    .space 1024
tinygo_flashParkStackTop:
.section .iram.startup, "ax"

// tinygo_flashCall(fn, a, b, c, stackTop) calls the ROM function fn(a, b, c) with the L2 cache suspended and
// branch prediction off, which would otherwise read flash. See spi_flash_disable_cache in ESP-IDF
// spi_flash/cache_utils.c. It runs on stackTop, as the caller's stack can be in PSRAM.
.global tinygo_flashCall
.type tinygo_flashCall,@function
tinygo_flashCall:
    mv   t0, sp
    andi sp, a4, -16
    addi sp, sp, -32
    sw   t0, 0(sp)
    sw   ra, 28(sp)
    sw   s0, 24(sp)
    sw   s1, 20(sp)
    sw   s2, 16(sp)
    sw   s3, 12(sp)
    sw   s4, 8(sp)
    mv   s0, a0
    mv   s1, a1
    mv   s2, a2
    mv   s3, a3
    csrr s4, 0x7c1 // MHCR in ESP-IDF riscv/include/riscv/rv_utils.h
    li   t0, 0x1030 // MHCR_RS, MHCR_BFE and MHCR_BTB
    csrc 0x7c1, t0
    li   t0, 0x4fc00508 // Cache_Suspend_L2_Cache
    jalr t0
    sw   a0, 4(sp)
    mv   a0, s1
    mv   a1, s2
    mv   a2, s3
    jalr s0
    mv   s1, a0
    lw   a0, 4(sp)
    li   t0, 0x4fc0050c // Cache_Resume_L2_Cache
    jalr t0
    csrw 0x7c1, s4
    mv   a0, s1
    lw   ra, 28(sp)
    lw   s0, 24(sp)
    lw   s1, 20(sp)
    lw   s2, 16(sp)
    lw   s3, 12(sp)
    lw   s4, 8(sp)
    lw   sp, 0(sp)
    ret

// Traps and non-vectored interrupts jump to the MTVEC base, which must be
// aligned to 64 bytes. See ESP-IDF components/riscv/vectors_clic.S.
.section .text.exception_vectors
.global _vector_table
.type _vector_table,@function
.balign 64
_vector_table:
    j handleInterruptASM
.size _vector_table, .-_vector_table
