// The coexistence adapter of ESP-IDF v5.3.2
// components/esp_coex/esp32s3/esp_coex_adapter.c for libcoexist.

#include "seam.h"

void tinygo_wifi_task_yield_from_isr(void);
void *tinygo_wifi_semphr_create(uint32_t max, uint32_t init);
void tinygo_wifi_semphr_delete(void *semphr);
int32_t tinygo_wifi_semphr_take(void *semphr, uint32_t block_time_tick);
int32_t tinygo_wifi_semphr_give(void *semphr);
bool tinygo_wifi_is_from_isr(void);
void tinygo_wifi_timer_disarm(void *timer);
void tinygo_wifi_timer_done(void *ptimer);
void tinygo_wifi_timer_setfn(void *ptimer, void *pfunction, void *parg);
void tinygo_wifi_timer_arm_us(void *ptimer, uint32_t us, bool repeat);
int64_t tinygo_wifi_esp_timer_get_time(void);

extern int esp_coex_adapter_register(void *funcs);
extern int coex_pre_init(void);

static int32_t semphr_take_from_isr(void *semphr, void *hptw) { return tinygo_wifi_semphr_take(semphr, 0); }
static int32_t semphr_give_from_isr(void *semphr, void *hptw) { return tinygo_wifi_semphr_give(semphr); }
static int is_in_isr(void) { return tinygo_wifi_is_from_isr(); }
static void *malloc_internal(size_t size) { return tinygo_wifi_malloc(size); }
static void free_internal(void *p) { tinygo_wifi_free(p); }
static int debug_matrix_init(int event, int signal, bool rev) { return 0x106; } // ESP_ERR_NOT_SUPPORTED

// coex_adapter_funcs_t of esp_coexist_adapter.h for the ESP32-S3. ESP-IDF
// leaves _env_is_chip and _xtal_freq_get NULL on this chip.
static struct {
	int32_t version;
	void (*task_yield_from_isr)(void);
	void *(*semphr_create)(uint32_t, uint32_t);
	void (*semphr_delete)(void *);
	int32_t (*semphr_take_from_isr)(void *, void *);
	int32_t (*semphr_give_from_isr)(void *, void *);
	int32_t (*semphr_take)(void *, uint32_t);
	int32_t (*semphr_give)(void *);
	int (*is_in_isr)(void);
	void *(*malloc_internal)(size_t);
	void (*free)(void *);
	int64_t (*esp_timer_get_time)(void);
	bool (*env_is_chip)(void);
	void (*timer_disarm)(void *);
	void (*timer_done)(void *);
	void (*timer_setfn)(void *, void *, void *);
	void (*timer_arm_us)(void *, uint32_t, bool);
	int (*debug_matrix_init)(int, int, bool);
	int (*xtal_freq_get)(void);
	int32_t magic;
} coex_adapter = {
	.version = 0x00000002, // COEX_ADAPTER_VERSION
	.task_yield_from_isr = tinygo_wifi_task_yield_from_isr,
	.semphr_create = tinygo_wifi_semphr_create,
	.semphr_delete = tinygo_wifi_semphr_delete,
	.semphr_take_from_isr = semphr_take_from_isr,
	.semphr_give_from_isr = semphr_give_from_isr,
	.semphr_take = tinygo_wifi_semphr_take,
	.semphr_give = tinygo_wifi_semphr_give,
	.is_in_isr = is_in_isr,
	.malloc_internal = malloc_internal,
	.free = free_internal,
	.esp_timer_get_time = tinygo_wifi_esp_timer_get_time,
	.timer_disarm = tinygo_wifi_timer_disarm,
	.timer_done = tinygo_wifi_timer_done,
	.timer_setfn = tinygo_wifi_timer_setfn,
	.timer_arm_us = tinygo_wifi_timer_arm_us,
	.debug_matrix_init = debug_matrix_init,
	.magic = 0xDEADBEAF, // COEX_ADAPTER_MAGIC
};

// seam_coex_pre_init is the coexistence part of the ESP-IDF startup
// (esp_system/startup_funcs.c). It runs once.
int seam_coex_pre_init(void) {
	static bool done;
	if (done) {
		return 0;
	}
	done = true;
	int err = esp_coex_adapter_register(&coex_adapter);
	if (err != 0) {
		return err;
	}
	return coex_pre_init();
}

extern int coex_init(void);

// seam_coex_init is the coexistence part of esp_wifi_init (ESP-IDF v5.3.2
// esp_wifi/src/wifi_init.c:399).
int seam_coex_init(void) {
	int err = seam_coex_pre_init();
	if (err != 0) {
		return err;
	}
	return coex_init();
}

extern uint32_t coex_status_get(uint8_t bitmap);
extern int coex_schm_register_callback(int type, void *callback);

// The two entries of the Wi-Fi OS table that do more than call libcoexist
// (esp_wifi/esp32s3/esp_adapter.c).
uint32_t seam_coex_status_get(void) {
	return coex_status_get(1 << 0); // COEX_STATUS_GET_WIFI_BITMAP
}

int seam_coex_schm_register_cb(int type, int (*cb)(int)) {
	return coex_schm_register_callback(type, cb);
}
