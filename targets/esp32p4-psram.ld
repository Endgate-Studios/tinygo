/* Linker script for ESP32-P4 chips below revision 3.0, running from flash with the heap in PSRAM.
 *
 * The ROM bootloader loads the RAM part. The startup code maps the flash part at 0x40010000,
 * from _flash_map_addr, which the image builder sets (builder/esp.go). The first 4 MB of PSRAM
 * stay out of the heap, for frame buffers and other DMA memory.
 * See ESP-IDF components/soc/esp32p4/include/soc/ext_mem_defs.h.
 */

MEMORY
{
    SRAM (rwx)  : ORIGIN = 0x4FF00000, LENGTH = 0x2BBD0
    SRAM2 (rw)  : ORIGIN = 0x4FF40000, LENGTH = 0x40000
    PSRAM (rw)  : ORIGIN = 0x48400000, LENGTH = 28M
    FLASH (rx)  : ORIGIN = 0x40010000, LENGTH = 16M - 64K
}

/* ROM functions of these chips. See ESP-IDF components/esp_rom/esp32p4/ld/esp32p4.rom.ld. */
PROVIDE(ets_set_appcpu_boot_addr = 0x4fc000a8);
PROVIDE(rtc_get_reset_reason = 0x4fc00018);
PROVIDE(Cache_WriteBack_Addr = 0x4fc003f4);
PROVIDE(Cache_Invalidate_Addr = 0x4fc003e4);
PROVIDE(rom_spiflash_legacy_data = 0x4ff3ffe8);
PROVIDE(Cache_Enable_L1_CORE1_ICache = 0x4fc004e4);
PROVIDE(esp_rom_spi_cmd_config = 0x4fc00108);
PROVIDE(esp_rom_spi_cmd_start = 0x4fc0010c);
PROVIDE(esp_rom_spi_set_op_mode = 0x4fc00110);
PROVIDE(esp_rom_software_reset_system = 0x4fc00094);

ENTRY(call_start_cpu0)

SECTIONS
{
    /* The stack is at the bottom, so that a stack overflow causes a fault. */
    .stack (NOLOAD) :
    {
        . = ALIGN(16);
        . += _stack_size;
        _stack_top = .;
        . += _stack_size;
        _stack1_top = .;
    } >SRAM

    /* Large buffers with no pointers: the GC doesn't scan these, and nothing clears them. */
    .bss_large (NOLOAD) : ALIGN(16)
    {
        *(.bss.main.stripMem .bss.main.wifiArena)
    } >SRAM

    .bss (NOLOAD) : ALIGN(4)
    {
        . = ALIGN (4);
        _sbss = ABSOLUTE(.);
        *(.sbss)
        *(.bss .bss.*)
        . = ALIGN (4);
        _ebss = ABSOLUTE(.);
    } >SRAM2

    .data : ALIGN(4)
    {
        . = ALIGN (4);
        _sdata = ABSOLUTE(.);
        *(.sdata)
        *(.data .data.*)
        . = ALIGN (4);
        _flash_map_addr = ABSOLUTE(.);
        LONG(0);
        _edata = ABSOLUTE(.);
    } >SRAM2

    .psram (NOLOAD) : ALIGN(16)
    {
        _psram_start = ABSOLUTE(.);
        *(.psram .psram.*)
        . = ALIGN (16);
        _psram_end = ABSOLUTE(.);
    } >PSRAM

    .iram : ALIGN(4)
    {
        *(.iram .iram.*)
        . = ALIGN (4);
    } >SRAM

    .text : ALIGN(4)
    {
        *(.init)
        *(.text .text.*)
        . = ALIGN (4);
    } >FLASH

    .rodata : ALIGN(4)
    {
        *(.rodata*)
        *(.srodata*)
        . = ALIGN (4);
        _flash_end = ABSOLUTE(.);
    } >FLASH

    /DISCARD/ :
    {
        *(.eh_frame)
    }

    /* For the garbage collector. */
    .tinygo_stacksizes (INFO) :
    {
        *(.tinygo_stacksizes)
    }
    .tinygo_arm_entries (INFO) :
    {
        *(.tinygo_arm_entries)
    }
}

_flash_map_pages = (_flash_end - ORIGIN(FLASH) + 0xFFFF) / 0x10000;

_heap_start = _psram_end;
_heap_end = _psram_end + 8M;

/* machine.Flash starts past the program, which starts at 0x10000, or further if the RAM part is over 60 KB.
 * This allows for 120 KB. pkg/wifinet/flashstore's sectors are just under 2 MB, and it needs them past this.
 * The whole chip is mapped at 0x41000000 for reads. */
__flash_data_phys = ALIGN(_flash_end - ORIGIN(FLASH) + 0x20000, 0x1000);
__flash_data_start = 0x41000000 + __flash_data_phys;
__flash_data_end = 0x42000000;

/* For the GC and scheduler. */
_globals_start = _sbss;
_globals_end = _edata;

_stack_size = 4K;
