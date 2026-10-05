// The Bluetooth LE controller seam: the OS function table, the controller
// config and the VHCI callbacks of ESP-IDF v5.3.2
// components/bt/controller/esp32c3/bt.c, which the ESP32-S3 also uses.

#include "seam.h"

// The Go functions that the table calls, see the //export lines.
void *tinygo_wifi_semphr_create(uint32_t max, uint32_t init);
void tinygo_wifi_semphr_delete(void *semphr);
int32_t tinygo_wifi_semphr_take(void *semphr, uint32_t block_time_tick);
int32_t tinygo_wifi_semphr_give(void *semphr);
void *tinygo_wifi_mutex_create(void);
void tinygo_wifi_mutex_delete(void *mutex);
int32_t tinygo_wifi_mutex_lock(void *mutex);
int32_t tinygo_wifi_mutex_unlock(void *mutex);
void *tinygo_wifi_queue_create(uint32_t queue_len, uint32_t item_size);
void tinygo_wifi_queue_delete(void *queue);
int32_t tinygo_wifi_queue_send(void *queue, void *item, uint32_t block_time_tick);
int32_t tinygo_wifi_queue_send_from_isr(void *queue, void *item, void *hptw);
int32_t tinygo_wifi_queue_recv(void *queue, void *item, uint32_t block_time_tick);
int32_t tinygo_wifi_task_create_pinned_to_core(void *task_func, const char *name, uint32_t stack_depth, void *param, uint32_t prio, void *task_handle, uint32_t core_id);
void tinygo_wifi_task_delete(void *task_handle);
void tinygo_wifi_task_yield_from_isr(void);
bool tinygo_wifi_is_from_isr(void);
int32_t tinygo_wifi_read_mac(uint8_t *mac, unsigned int type);
uint32_t tinygo_wifi_rand(void);

int32_t tinygo_bt_interrupt_alloc(int32_t cpu_id, int32_t source, void *handler, void *arg, void **ret_handle);
int32_t tinygo_bt_interrupt_free(void *handle);
int32_t tinygo_bt_interrupt_enable(void *handle);
int32_t tinygo_bt_interrupt_disable(void *handle);
void tinygo_bt_global_intr_disable(void);
void tinygo_bt_global_intr_restore(void);
void tinygo_bt_task_yield(void);
void tinygo_bt_send_available(void);
void tinygo_bt_module_reset(void);
int32_t tinygo_bt_recv(uint8_t *data, uint16_t len);

// Library and ROM functions.
extern int btdm_osi_funcs_register(void *osi_funcs);
extern int btdm_controller_rom_data_init(void);
extern int btdm_controller_init(void *config_opts);
extern int btdm_controller_enable(int mode);
extern void btdm_controller_disable(void);
extern void btdm_controller_deinit(void);
extern void coex_disable(void);
extern bool btdm_lpclk_select_src(uint32_t sel);
extern bool btdm_lpclk_set_div(uint32_t div);
extern void sdk_config_extend_set_pll_track(bool enable);
extern void coex_pti_v2(void);
extern void ets_delay_us(uint32_t us);

typedef struct {
	void (*notify_host_send_available)(void);
	int (*notify_host_recv)(uint8_t *data, uint16_t len);
} vhci_host_callback_t;

extern bool API_vhci_host_check_send_available(void);
extern void API_vhci_host_send_packet(uint8_t *data, uint16_t len);
extern int API_vhci_host_register_callback(const vhci_host_callback_t *callback);

// The wrappers have the exact types of the table: int32_t is a long on Xtensa,
// so a Go function cannot go into an int field directly.

static int interrupt_alloc(int cpu_id, int source, void *handler, void *arg, void **ret_handle) {
	return tinygo_bt_interrupt_alloc(cpu_id, source, handler, arg, ret_handle);
}
static int interrupt_free(void *handle) { return tinygo_bt_interrupt_free(handle); }
static int interrupt_enable(void *handle) { return tinygo_bt_interrupt_enable(handle); }
static int interrupt_disable(void *handle) { return tinygo_bt_interrupt_disable(handle); }

static int semphr_take_from_isr(void *semphr, void *hptw) { return tinygo_wifi_semphr_take(semphr, 0); }
static int semphr_give_from_isr(void *semphr, void *hptw) { return tinygo_wifi_semphr_give(semphr); }
static int semphr_take(void *semphr, uint32_t ms) { return tinygo_wifi_semphr_take(semphr, ms); }
static int semphr_give(void *semphr) { return tinygo_wifi_semphr_give(semphr); }
static int mutex_lock(void *mutex) { return tinygo_wifi_mutex_lock(mutex); }
static int mutex_unlock(void *mutex) { return tinygo_wifi_mutex_unlock(mutex); }
static int queue_send(void *queue, void *item, uint32_t ms) { return tinygo_wifi_queue_send(queue, item, ms); }
static int queue_send_from_isr(void *queue, void *item, void *hptw) { return tinygo_wifi_queue_send_from_isr(queue, item, hptw); }
static int queue_recv(void *queue, void *item, uint32_t ms) { return tinygo_wifi_queue_recv(queue, item, ms); }
static int queue_recv_from_isr(void *queue, void *item, void *hptw) { return tinygo_wifi_queue_recv(queue, item, 0); }

static int task_create(void *task_func, const char *name, uint32_t stack_depth, void *param, uint32_t prio, void *task_handle, uint32_t core_id) {
	return tinygo_wifi_task_create_pinned_to_core(task_func, name, stack_depth, param, prio, task_handle, core_id);
}

static int read_efuse_mac(uint8_t mac[6]) { return tinygo_wifi_read_mac(mac, 2); } // ESP_MAC_BT
static void srand_nop(unsigned int seed) {}
static int rand_hw(void) { return (int)tinygo_wifi_rand(); }

// The low power clock is the main XTAL with a divider of 40, so one cycle is
// 1 us: btdm_lpcycle_us is 1 << 19 with 19 fraction bits (bt.c:1338-1342).
static uint32_t btdm_lpcycle_us = 1 << 19;
static uint8_t btdm_lpcycle_us_frac = 19;

// btdm_lpcycles_2_hus of bt.c: low power cycles to half microseconds.
static uint32_t lpcycles_2_hus(uint32_t cycles, uint32_t *error_corr) {
	uint64_t local_error_corr = (error_corr == NULL) ? 0 : (uint64_t)(*error_corr);
	uint64_t res = (uint64_t)btdm_lpcycle_us * cycles * 2;
	local_error_corr += res;
	res = (local_error_corr >> btdm_lpcycle_us_frac);
	local_error_corr -= (res << btdm_lpcycle_us_frac);
	if (error_corr) {
		*error_corr = (uint32_t)local_error_corr;
	}
	return (uint32_t)res;
}

// btdm_hus_2_lpcycles of bt.c.
static uint32_t hus_2_lpcycles(uint32_t hus) {
	uint64_t cycles = ((uint64_t)(hus) << btdm_lpcycle_us_frac) / btdm_lpcycle_us;
	return (uint32_t)(cycles >> 1);
}

// btdm_sleep_check_duration of bt.c.
static bool sleep_check_duration(int32_t *half_slot_cnt) {
	if (*half_slot_cnt < 24) {
		return false;
	}
	*half_slot_cnt -= 8;
	return true;
}

// The sleep mode is 0 (no modem sleep), so the sleep entries do nothing, as
// in bt.c without a low power clock.
static void sleep_enter_phase1(uint32_t lpcycles) {}
static void sleep_enter_phase2(void) {}
static void sleep_exit_phase3(void) {}

// Coexistence entries with CONFIG_SW_COEXIST_ENABLE (bt.c:1885-1936). The
// wake requests do nothing without sleep mode 1 (async_wakeup_request).
extern int coex_init(void);
extern int coex_enable(void);
extern int coex_schm_register_callback(int type, void *callback);
extern void coex_schm_status_bit_set(uint32_t type, uint32_t status);
extern void coex_schm_status_bit_clear(uint32_t type, uint32_t status);
extern uint32_t coex_schm_interval_get(void);
extern uint8_t coex_schm_curr_period_get(void);
extern void *coex_schm_curr_phase_get(void);

static void coex_wifi_sleep_set(bool sleep) {}
static int coex_schm_register_btdm_callback(void *callback) {
	return coex_schm_register_callback(1, callback); // COEX_SCHM_CALLBACK_TYPE_BT
}
static bool coex_bt_wakeup_request(void) { return false; }
static void coex_bt_wakeup_request_end(void) {}

// Without CONFIG_MAC_BB_PD the power and DMA backup entries do nothing.
static void hw_power_down(void) {}
static void hw_power_up(void) {}
static void backup_dma_copy(uint32_t reg, uint32_t mem_addr, uint32_t num, bool to_mem) {}
static void rom_table_ready(void) {}

static void *malloc_any(size_t size) { return tinygo_wifi_malloc(size); }
static void free_any(void *p) { tinygo_wifi_free(p); }

// struct osi_funcs_t of bt.c:158-219, 240 bytes. The library keeps a pointer
// to it, so it is static.
static struct {
	uint32_t magic;
	uint32_t version;
	int (*interrupt_alloc)(int, int, void *, void *, void **);
	int (*interrupt_free)(void *);
	void (*interrupt_handler_set_rsv)(int, void *, void *);
	void (*global_intr_disable)(void);
	void (*global_intr_restore)(void);
	void (*task_yield)(void);
	void (*task_yield_from_isr)(void);
	void *(*semphr_create)(uint32_t, uint32_t);
	void (*semphr_delete)(void *);
	int (*semphr_take_from_isr)(void *, void *);
	int (*semphr_give_from_isr)(void *, void *);
	int (*semphr_take)(void *, uint32_t);
	int (*semphr_give)(void *);
	void *(*mutex_create)(void);
	void (*mutex_delete)(void *);
	int (*mutex_lock)(void *);
	int (*mutex_unlock)(void *);
	void *(*queue_create)(uint32_t, uint32_t);
	void (*queue_delete)(void *);
	int (*queue_send)(void *, void *, uint32_t);
	int (*queue_send_from_isr)(void *, void *, void *);
	int (*queue_recv)(void *, void *, uint32_t);
	int (*queue_recv_from_isr)(void *, void *, void *);
	int (*task_create)(void *, const char *, uint32_t, void *, uint32_t, void *, uint32_t);
	void (*task_delete)(void *);
	bool (*is_in_isr)(void);
	int (*cause_sw_intr_to_core)(int, int);
	void *(*malloc)(size_t);
	void *(*malloc_internal)(size_t);
	void (*free)(void *);
	int (*read_efuse_mac)(uint8_t *);
	void (*srand)(unsigned int);
	int (*rand)(void);
	uint32_t (*btdm_lpcycles_2_hus)(uint32_t, uint32_t *);
	uint32_t (*btdm_hus_2_lpcycles)(uint32_t);
	bool (*btdm_sleep_check_duration)(int32_t *);
	void (*btdm_sleep_enter_phase1)(uint32_t);
	void (*btdm_sleep_enter_phase2)(void);
	void (*btdm_sleep_exit_phase1)(void);
	void (*btdm_sleep_exit_phase2)(void);
	void (*btdm_sleep_exit_phase3)(void);
	void (*coex_wifi_sleep_set)(bool);
	int (*coex_core_ble_conn_dyn_prio_get)(bool *, bool *);
	int (*coex_schm_register_btdm_callback)(void *);
	void (*coex_schm_status_bit_set)(uint32_t, uint32_t);
	void (*coex_schm_status_bit_clear)(uint32_t, uint32_t);
	uint32_t (*coex_schm_interval_get)(void);
	uint8_t (*coex_schm_curr_period_get)(void);
	void *(*coex_schm_curr_phase_get)(void);
	int (*interrupt_enable)(void *);
	int (*interrupt_disable)(void *);
	void (*esp_hw_power_down)(void);
	void (*esp_hw_power_up)(void);
	void (*ets_backup_dma_copy)(uint32_t, uint32_t, uint32_t, bool);
	void (*ets_delay_us)(uint32_t);
	void (*btdm_rom_table_ready)(void);
	bool (*coex_bt_wakeup_request)(void);
	void (*coex_bt_wakeup_request_end)(void);
} bt_osi = {
	.magic = 0xFADEBEAD,  // OSI_MAGIC_VALUE
	.version = 0x00010009, // OSI_VERSION
	.interrupt_alloc = interrupt_alloc,
	.interrupt_free = interrupt_free,
	.global_intr_disable = tinygo_bt_global_intr_disable,
	.global_intr_restore = tinygo_bt_global_intr_restore,
	.task_yield = tinygo_bt_task_yield,
	.task_yield_from_isr = tinygo_wifi_task_yield_from_isr,
	.semphr_create = tinygo_wifi_semphr_create,
	.semphr_delete = tinygo_wifi_semphr_delete,
	.semphr_take_from_isr = semphr_take_from_isr,
	.semphr_give_from_isr = semphr_give_from_isr,
	.semphr_take = semphr_take,
	.semphr_give = semphr_give,
	.mutex_create = tinygo_wifi_mutex_create,
	.mutex_delete = tinygo_wifi_mutex_delete,
	.mutex_lock = mutex_lock,
	.mutex_unlock = mutex_unlock,
	.queue_create = tinygo_wifi_queue_create,
	.queue_delete = tinygo_wifi_queue_delete,
	.queue_send = queue_send,
	.queue_send_from_isr = queue_send_from_isr,
	.queue_recv = queue_recv,
	.queue_recv_from_isr = queue_recv_from_isr,
	.task_create = task_create,
	.task_delete = tinygo_wifi_task_delete,
	.is_in_isr = tinygo_wifi_is_from_isr,
	.malloc = malloc_any,
	.malloc_internal = malloc_any,
	.free = free_any,
	.read_efuse_mac = read_efuse_mac,
	.srand = srand_nop,
	.rand = rand_hw,
	.btdm_lpcycles_2_hus = lpcycles_2_hus,
	.btdm_hus_2_lpcycles = hus_2_lpcycles,
	.btdm_sleep_check_duration = sleep_check_duration,
	.btdm_sleep_enter_phase1 = sleep_enter_phase1,
	.btdm_sleep_enter_phase2 = sleep_enter_phase2,
	.btdm_sleep_exit_phase3 = sleep_exit_phase3,
	.coex_wifi_sleep_set = coex_wifi_sleep_set,
	.coex_schm_register_btdm_callback = coex_schm_register_btdm_callback,
	.coex_schm_status_bit_set = coex_schm_status_bit_set,
	.coex_schm_status_bit_clear = coex_schm_status_bit_clear,
	.coex_schm_interval_get = coex_schm_interval_get,
	.coex_schm_curr_period_get = coex_schm_curr_period_get,
	.coex_schm_curr_phase_get = coex_schm_curr_phase_get,
	.interrupt_enable = interrupt_enable,
	.interrupt_disable = interrupt_disable,
	.esp_hw_power_down = hw_power_down,
	.esp_hw_power_up = hw_power_up,
	.ets_backup_dma_copy = backup_dma_copy,
	.ets_delay_us = ets_delay_us,
	.btdm_rom_table_ready = rom_table_ready,
	.coex_bt_wakeup_request = coex_bt_wakeup_request,
	.coex_bt_wakeup_request_end = coex_bt_wakeup_request_end,
};

_Static_assert(sizeof(bt_osi) == 240, "struct osi_funcs_t of bt.c is 240 bytes");

// esp_bt_controller_config_t of esp_bt.h:367-420 with the BLE-only defaults
// of BT_CONTROLLER_INIT_CONFIG_DEFAULT for the ESP32-S3.
typedef struct {
	uint32_t magic;
	uint32_t version;
	uint16_t controller_task_stack_size;
	uint8_t controller_task_prio;
	uint8_t controller_task_run_cpu;
	uint8_t bluetooth_mode;
	uint8_t ble_max_act;
	uint8_t sleep_mode;
	uint8_t sleep_clock;
	uint8_t ble_st_acl_tx_buf_nb;
	uint8_t ble_hw_cca_check;
	uint16_t ble_adv_dup_filt_max;
	bool coex_param_en;
	uint8_t ce_len_type;
	bool coex_use_hooks;
	uint8_t hci_tl_type;
	void *hci_tl_funcs;
	uint8_t txant_dft;
	uint8_t rxant_dft;
	uint8_t txpwr_dft;
	uint32_t cfg_mask;
	uint8_t scan_duplicate_mode;
	uint8_t scan_duplicate_type;
	uint16_t normal_adv_size;
	uint16_t mesh_adv_size;
	uint8_t coex_phy_coded_tx_rx_time_limit;
	uint32_t hw_target_code;
	uint8_t slave_ce_len_min;
	uint8_t hw_recorrect_en;
	uint8_t cca_thresh;
	uint16_t scan_backoff_upperlimitmax;
	uint16_t dup_list_refresh_period;
	bool ble_50_feat_supp;
	uint8_t ble_cca_mode;
	uint8_t ble_data_lenth_zero_aux;
	uint8_t ble_chan_ass_en;
	uint8_t ble_ping_en;
	uint8_t ble_llcp_disc_flag;
	bool run_in_flash;
	bool dtm_en;
	bool enc_en;
	bool qa_test;
	bool master_en;
	bool scan_en;
} bt_config_t;

_Static_assert(sizeof(bt_config_t) == 68, "esp_bt_controller_config_t is 68 bytes");

// seam_bt_init does esp_bt_controller_init of bt.c:1369-1466 after the power
// domain is on: the ROM data, the table, the low power clock, coexistence,
// the module reset and the controller, in this order.
int seam_bt_init(uint8_t max_act) {
	bt_config_t cfg = {
		.magic = 0x5A5AA5A5,   // ESP_BT_CTRL_CONFIG_MAGIC_VAL
		.version = 0x02410230, // ESP_BT_CTRL_CONFIG_VERSION
		.controller_task_stack_size = 4096,
		.controller_task_prio = 23,
		.controller_task_run_cpu = 0,
		.bluetooth_mode = 1, // ESP_BT_MODE_BLE
		.ble_max_act = max_act,
		.ble_adv_dup_filt_max = 30,
		.hci_tl_type = 1, // VHCI
		.txpwr_dft = 11,  // +9 dBm
		.cfg_mask = 1,    // CFG_MASK_BIT_SCAN_DUPLICATE_OPTION
		.normal_adv_size = 100,
		.hw_target_code = 0x02010000, // BLE_HW_TARGET_CODE_CHIP_ECO0
		.slave_ce_len_min = 5,
		.cca_thresh = 20,
		.ble_50_feat_supp = true,
		.ble_chan_ass_en = 1,
		.ble_ping_en = 1,
		.dtm_en = true,
		.enc_en = true,
		.qa_test = true,
		.master_en = true,
		.scan_en = true,
	};
	btdm_controller_rom_data_init(); // before the table: r_osi_funcs_p is ROM data
	int err = btdm_osi_funcs_register(&bt_osi);
	if (err != 0) {
		return 0x100 | (err & 0xff);
	}
	if (!btdm_lpclk_select_src(0) || !btdm_lpclk_set_div(40)) { // XTAL, 40 MHz
		return 0x200;
	}
	if (coex_init() != 0) {
		return 0x300;
	}
	tinygo_bt_module_reset();
	return btdm_controller_init(&cfg);
}

// seam_bt_enable is esp_bt_controller_enable(ESP_BT_MODE_BLE) after the PHY
// is on. btdm_controller_enable waits for the controller task.
int seam_bt_enable(void) {
	coex_enable();
	sdk_config_extend_set_pll_track(false);
	int err = btdm_controller_enable(1);
	if (err != 0) {
		return err;
	}
	coex_pti_v2();
	return 0;
}

// seam_bt_disable is esp_bt_controller_disable of bt.c:1655-1676 without the
// sleep and power management parts. The caller turns off the PHY.
void seam_bt_disable(void) {
	btdm_controller_disable();
	coex_disable();
}

// seam_bt_deinit is esp_bt_controller_deinit of bt.c:1468-1478. The controller
// task deletes itself. The caller does bt_controller_deinit_internal.
void seam_bt_deinit(void) {
	btdm_controller_deinit();
}

static int vhci_recv(uint8_t *data, uint16_t len) { return tinygo_bt_recv(data, len); }

// The library keeps a pointer to the callbacks, so they are static.
static const vhci_host_callback_t vhci_callbacks = {
	.notify_host_send_available = tinygo_bt_send_available,
	.notify_host_recv = vhci_recv,
};

int seam_bt_register_vhci(void) {
	return API_vhci_host_register_callback(&vhci_callbacks);
}

bool seam_bt_can_send(void) {
	return API_vhci_host_check_send_available();
}

void seam_bt_send(uint8_t *data, uint16_t len) {
	API_vhci_host_send_packet(data, len);
}
