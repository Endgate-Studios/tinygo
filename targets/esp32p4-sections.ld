/* Sections for the ESP32-P4. The ROM bootloader loads everything into L2MEM,
 * so this does not need the flash cache or its MMU.
 */

ENTRY(call_start_cpu0)

SECTIONS
{
    /* The stack is at the bottom, so that a stack overflow causes a fault. */
    .stack (NOLOAD) :
    {
        . = ALIGN(16);
        . += _stack_size;
        _stack_top = .;
    } >SRAM

    .bss (NOLOAD) : ALIGN(4)
    {
        . = ALIGN (4);
        _sbss = ABSOLUTE(.);
        *(.sbss)
        *(.bss .bss.*)
        . = ALIGN (4);
        _ebss = ABSOLUTE(.);
    } >SRAM

    .data : ALIGN(4)
    {
        . = ALIGN (4);
        _sdata = ABSOLUTE(.);
        *(.sdata)
        *(.data .data.*)
        . = ALIGN (4);
        _edata = ABSOLUTE(.);
    } >SRAM

    .rodata : ALIGN(4)
    {
        *(.rodata*)
        *(.srodata*)
        . = ALIGN (4);
        _erodata = ABSOLUTE(.);
    } >SRAM

    .text : ALIGN(4)
    {
        *(.iram.startup)
        *(.init)
        *(.text .text.*)
        . = ALIGN (4);
    } >CODE

    /* Goroutine stacks come from the heap and need 16 byte alignment
     * (RISC-V ELF psABI, integer calling convention).
     */
    PROVIDE(_heap_start = ALIGN(_erodata, 16));
    PROVIDE(_heap_end = ORIGIN(SRAM) + LENGTH(SRAM));

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

/* For the GC and scheduler. */
_flash_map_pages = 0;
_flash_map_addr = 0;

_globals_start = _sbss;
_globals_end = _edata;

_stack_size = 4K;
