// The ROM bootloader has loaded the RAM part of the program and jumps here. The startup
// code runs from RAM, because the rest of the program can be in flash that isn't mapped yet.
// _flash_map_pages is 0 when the whole program is in RAM.

.section .iram.startup, "ax"
.global call_start_cpu0
.type call_start_cpu0,@function
call_start_cpu0:
    lui  t0, %hi(_flash_map_pages)
    addi t0, t0, %lo(_flash_map_pages)
    beqz t0, 2f

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
2:
    lui  t0, %hi(_start)
    addi t0, t0, %lo(_start)
    jr   t0

// Traps and non-vectored interrupts jump to the MTVEC base, which must be
// aligned to 64 bytes. See ESP-IDF components/riscv/vectors_clic.S.
.section .text.exception_vectors
.global _vector_table
.type _vector_table,@function
.balign 64
_vector_table:
    j handleInterruptASM
.size _vector_table, .-_vector_table
