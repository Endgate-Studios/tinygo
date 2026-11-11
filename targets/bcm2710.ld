/* The firmware loads kernel8.img at 0x80000 with arm_64bit=1.
 * RAM above 0x10000000 is for DMA buffers, and the GPU's share is at the top. */
MEMORY
{
    RAM (rwx) : ORIGIN = 0x80000, LENGTH = 0x10000000 - 0x80000
}

ENTRY(_start)

_stack_size = 64K;

SECTIONS
{
    .text :
    {
        KEEP(*(.text.boot))
        *(.text .text.*)
    } >RAM

    .rodata :
    {
        . = ALIGN(16);
        *(.rodata .rodata.*)
    } >RAM

    .data :
    {
        . = ALIGN(16);
        _globals_start = .;
        *(.data .data.*)
        . = ALIGN(16);
    } >RAM

    .bss (NOLOAD) :
    {
        . = ALIGN(16);
        _sbss = .;
        *(.bss .bss.*)
        *(COMMON)
        . = ALIGN(16);
        _ebss = .;
        _globals_end = .;
    } >RAM

    .stack (NOLOAD) :
    {
        . = ALIGN(16);
        . += _stack_size;
        _stack_top = .;
        . += _stack_size;
        _stack1_top = .;
        . += _stack_size;
        _stack2_top = .;
        . += _stack_size;
        _stack3_top = .;
    } >RAM

    _heap_start = ALIGN(16);
    /* 0x0F000000 up is the firmware bundle that config.txt loads, see device/bcm2710. */
    _heap_end = 0x0f000000;

    /DISCARD/ :
    {
        *(.eh_frame)
    }
}
