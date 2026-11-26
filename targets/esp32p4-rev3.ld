/* Linker script for ESP32-P4 chip revision 3.0 and higher.
 *
 * The L2 cache is at the bottom of L2MEM on these chips, and the ROM data
 * starts at 0x4FFADFC0. The 128 KB skipped here is the smallest cache size.
 * See ESP-IDF master components/esp_system/ld/esp32p4/memory.ld.in.
 */

MEMORY
{
    SRAM (rwx) : ORIGIN = 0x4FF20000, LENGTH = 0x40000
    CODE (rwx) : ORIGIN = 0x4FF60000, LENGTH = 0x4DFC0
}

INCLUDE "targets/esp32p4-sections.ld"
