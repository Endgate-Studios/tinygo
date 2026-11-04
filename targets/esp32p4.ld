/* Linker script for ESP32-P4 chip revisions below 3.0.
 *
 * The ROM data starts at 0x4FF2BBD0 on these chips.
 * See ESP-IDF components/esp_system/ld/esp32p4/memory.ld.in.
 */

MEMORY
{
    SRAM (rwx) : ORIGIN = 0x4FF00000, LENGTH = 0x2BBD0
}

INCLUDE "targets/esp32p4-sections.ld"
