/* Linker script for ESP32-P4 chips below revision 3.0, with the heap in PSRAM.
 *
 * The first 4 MB of PSRAM stay out of the heap, for frame buffers and other DMA memory.
 * The heap uses the cached address. See SOC_DRAM_PSRAM_ADDRESS_LOW in soc/ext_mem_defs.h.
 */

_heap_start = 0x48400000;
_heap_end = 0x48C00000;

INCLUDE "targets/esp32p4.ld"
