/* Linker script for ESP32-P4 chip revisions below 3.0.
 *
 * The ROM data starts at 0x4FF2BBD0 on these chips.
 * See ESP-IDF components/esp_system/ld/esp32p4/memory.ld.in.
 */

MEMORY
{
    SRAM (rwx) : ORIGIN = 0x4FF00000, LENGTH = 0x2BBD0
}

/* ROM functions of these chips. See ESP-IDF components/esp_rom/esp32p4/ld/esp32p4.rom.ld. */
PROVIDE(esp_rom_spi_cmd_config = 0x4fc00108);
PROVIDE(esp_rom_spi_cmd_start = 0x4fc0010c);
PROVIDE(esp_rom_spi_set_op_mode = 0x4fc00110);

INCLUDE "targets/esp32p4-sections.ld"
