#include <stdio.h>
#include <string.h>

#include "seam.h"

// The event base name of the Wi-Fi events, as ESP_EVENT_DEFINE_BASE(WIFI_EVENT).
const char *WIFI_EVENT = "WIFI_EVENT";

// seam_wifi_init calls the blob with the ESP-IDF v5.3.2 defaults for the
// ESP32-S3 (WIFI_INIT_CONFIG_DEFAULT), but without NVS. With PSRAM, TX
// buffers can be there, so the blob copies them to internal cache buffers
// for the DMA, as with CONFIG_SPIRAM in esp_wifi.h.
int seam_wifi_init(bool psram) {
	static wifi_init_config_t config = {
		.osi_funcs = &g_wifi_osi_funcs,
		.static_rx_buf_num = 10,
		.dynamic_rx_buf_num = 32,
		.tx_buf_type = 1,
		.static_tx_buf_num = 0,
		.dynamic_tx_buf_num = 32,
		.rx_mgmt_buf_type = 0,
		.rx_mgmt_buf_num = 5,
		.cache_tx_buf_num = 0,
		.csi_enable = 0,
		.ampdu_rx_enable = 1,
		.ampdu_tx_enable = 1,
		.amsdu_tx_enable = 0,
		.nvs_enable = 0,
		.nano_enable = 0,
		.rx_ba_win = 6,
		.wifi_task_core_id = 0,
		.beacon_max_len = 752,
		.mgmt_sbuf_num = 32,
		.feature_caps = 1, // CONFIG_FEATURE_WPA3_SAE_BIT
		.sta_disconnected_pm = false,
		.espnow_max_encrypt_num = 7,
		.tx_hetb_queue_num = 1,
		.dump_hesigb_enable = false,
		.magic = 0x1F2F3F4F, // WIFI_INIT_CONFIG_MAGIC
	};
	config.wpa_crypto_funcs = seam_crypto_funcs;
	if (psram) {
		config.cache_tx_buf_num = 32;  // CONFIG_ESP_WIFI_CACHE_TX_BUFFER_NUM
		config.feature_caps |= 1 << 1; // CONFIG_FEATURE_CACHE_TX_BUF_BIT
	}
	int err = esp_wifi_init_internal(&config);
	if (err != 0) {
		return err;
	}
	// esp_wifi_init of ESP-IDF v5.3.2 esp_wifi/src/wifi_init.c registers the
	// supplicant table next, in esp_supplicant_init.
	return seam_register_wpa_cb();
}

// seam_phy_init_data is phy_init_data of ESP-IDF v5.3.2
// components/esp_phy/esp32s3/phy_init_data.c with CONFIG_ESP_PHY_MAX_TX_POWER 20.
const esp_phy_init_data_t seam_phy_init_data = {
	.params = {
		0x00, 0x00, 0x50, 0x50, 0x50, 0x4c, 0x4c, 0x48, 0x4c, 0x48, 0x48, 0x44,
		0x4a, 0x46, 0x46, 0x42, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x74,
	},
};

bool seam_env_is_chip(void) {
	return true;
}

// The queues of _wifi_create_queue are a struct with the queue handle, as
// wifi_static_queue_t of ESP-IDF esp_wifi/esp32s3/esp_adapter.c. The blobs
// read the handle from it.
typedef struct {
	void *handle;
	void *storage;
} wifi_static_queue_t;

void *seam_wifi_create_queue(int n, int size) {
	wifi_static_queue_t *q = tinygo_wifi_calloc(1, sizeof(*q));
	if (q != NULL) {
		q->handle = tinygo_wifi_queue_create((uint32_t)n, (uint32_t)size);
	}
	return q;
}

void seam_wifi_delete_queue(void *p) {
	wifi_static_queue_t *q = p;
	if (q != NULL) {
		tinygo_wifi_queue_delete(q->handle);
		tinygo_wifi_free(q);
	}
}

void *seam_zalloc(size_t size) {
	return tinygo_wifi_calloc(1, size);
}

// seam_wifi_zalloc is wifi_zalloc_wrapper: PSRAM first, if there is a PSRAM
// heap.
void *seam_wifi_zalloc(size_t size) {
	return tinygo_wifi_ext_calloc(1, size);
}

void seam_unimplemented(const char *name) {
	char text[64];
	int n = snprintf(text, sizeof(text), "wifi osi: %s is not implemented\n", name);
	tinygo_wifi_log(text, n < (int)sizeof(text) ? n : (int)sizeof(text) - 1);
}

// seam_call calls a function of the blobs with one argument. Go cannot call a
// C function pointer.
void seam_call(void *fn, void *arg) {
	((void (*)(void *))fn)(arg);
}

void seam_log_writev(unsigned int level, const char *tag, const char *format, va_list args) {
	char text[240];
	int n = snprintf(text, sizeof(text), "wifi %u %s: [%s] ", level, tag, format);
	if (n > 0 && n < (int)sizeof(text)) {
		int m = vsnprintf(text + n, sizeof(text) - n, format, args);
		n = m < 0 ? n : n + m;
	}
	if (n >= (int)sizeof(text) - 1) {
		n = (int)sizeof(text) - 2;
	}
	text[n++] = '\n';
	tinygo_wifi_log(text, n);
}

void seam_log_write(unsigned int level, const char *tag, const char *format, ...) {
	va_list args;
	va_start(args, format);
	seam_log_writev(level, tag, format, args);
	va_end(args);
}

// The log functions of the blobs. Go cannot define variadic functions.
static int seam_printf(const char *tag, const char *format, va_list args) {
	seam_log_writev(3, tag, format, args);
	return 0;
}

int net80211_printf(const char *format, ...) {
	va_list args;
	va_start(args, format);
	int n = seam_printf("net80211", format, args);
	va_end(args);
	return n;
}

int pp_printf(const char *format, ...) {
	va_list args;
	va_start(args, format);
	int n = seam_printf("pp", format, args);
	va_end(args);
	return n;
}

int phy_printf(const char *format, ...) {
	va_list args;
	va_start(args, format);
	int n = seam_printf("phy", format, args);
	va_end(args);
	return n;
}

int coexist_printf(const char *format, ...) {
	va_list args;
	va_start(args, format);
	int n = seam_printf("coexist", format, args);
	va_end(args);
	return n;
}

// targets/esp32s3.json wraps this blob function. The wrapper keeps the
// original behavior.
bool __real_ppCheckTxConnTrafficIdle(void);

bool __wrap_ppCheckTxConnTrafficIdle(void) {
	return __real_ppCheckTxConnTrafficIdle();
}

// seam_intenable_set and seam_intenable_clear change the Xtensa INTENABLE
// special register. The caller disables the interrupts around them.
void seam_intenable_set(uint32_t mask) {
	uint32_t v;
	__asm__ volatile("rsr %0, intenable" : "=r"(v));
	v |= mask;
	__asm__ volatile("wsr %0, intenable\n\trsync" ::"r"(v));
}

void seam_intenable_clear(uint32_t mask) {
	uint32_t v;
	__asm__ volatile("rsr %0, intenable" : "=r"(v));
	v &= ~mask;
	__asm__ volatile("wsr %0, intenable\n\trsync" ::"r"(v));
}

// esp_wifi_internal_reg_rxcb of esp_private/wifi.h. C gives the address of
// the Go function.
int esp_wifi_internal_reg_rxcb(int ifx, int (*fn)(void *buffer, uint16_t len, void *eb));

int seam_wifi_reg_rxcb(void) {
	return esp_wifi_internal_reg_rxcb(0, tinygo_wifi_rx);
}
