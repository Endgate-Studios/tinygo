package nn

// EnableDualCore lets the functions give half of a large operation to the
// other core, as esp-nn esp_nn_dual_core_enable. It needs the cores scheduler
// on the ESP32-S3, else it does nothing. The results do not change.
func EnableDualCore() {
	enableDualCore()
}
