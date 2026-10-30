// The C side of the seam between the ESP32-S3 Wi-Fi blobs and Go. It has only
// what Go cannot do: the function pointer table, variadic functions and the
// symbols that the blobs use by name.
#pragma once

#include <stdarg.h>
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include "esp_private/wifi_os_adapter.h"
#include "esp_wifi_crypto_types.h"

// wifi_init_config_t of ESP-IDF v5.3.2 esp_wifi.h.
typedef struct {
	wifi_osi_funcs_t *osi_funcs;
	wpa_crypto_funcs_t wpa_crypto_funcs;
	int static_rx_buf_num;
	int dynamic_rx_buf_num;
	int tx_buf_type;
	int static_tx_buf_num;
	int dynamic_tx_buf_num;
	int rx_mgmt_buf_type;
	int rx_mgmt_buf_num;
	int cache_tx_buf_num;
	int csi_enable;
	int ampdu_rx_enable;
	int ampdu_tx_enable;
	int amsdu_tx_enable;
	int nvs_enable;
	int nano_enable;
	int rx_ba_win;
	int wifi_task_core_id;
	int beacon_max_len;
	int mgmt_sbuf_num;
	uint64_t feature_caps;
	bool sta_disconnected_pm;
	int espnow_max_encrypt_num;
	int tx_hetb_queue_num;
	bool dump_hesigb_enable;
	int magic;
} wifi_init_config_t;

extern const wpa_crypto_funcs_t seam_crypto_funcs;

// wifi_ap_record_t of ESP-IDF v5.3.2 esp_wifi_types_generic.h. The enums are
// int, see -fno-short-enums.
typedef struct {
	char cc[3];
	uint8_t schan;
	uint8_t nchan;
	int8_t max_tx_power;
	int policy;
} wifi_country_t;

typedef struct {
	uint8_t bss_color : 6;
	uint8_t partial_bss_color : 1;
	uint8_t bss_color_disabled : 1;
	uint8_t bssid_index;
} wifi_he_ap_info_t;

typedef struct {
	uint8_t bssid[6];
	uint8_t ssid[33];
	uint8_t primary;
	int second;
	int8_t rssi;
	int authmode;
	int pairwise_cipher;
	int group_cipher;
	int ant;
	uint32_t phy_flags; // the phy_11b to reserved bit fields
	wifi_country_t country;
	wifi_he_ap_info_t he_ap;
	uint8_t bandwidth;
	uint8_t vht_ch_freq1;
	uint8_t vht_ch_freq2;
} wifi_ap_record_t;

// wifi_sta_config_t of ESP-IDF v5.3.2 esp_wifi_types_generic.h. The enums
// are int. flags and he_flags hold the bit fields.
typedef struct {
	uint8_t ssid[32];
	uint8_t password[64];
	int scan_method;
	bool bssid_set;
	uint8_t bssid[6];
	uint8_t channel;
	uint16_t listen_interval;
	int sort_method;
	struct {
		int8_t rssi;
		int authmode;
	} threshold;
	struct {
		bool capable;
		bool required;
	} pmf_cfg;
	uint32_t flags;
	int sae_pwe_h2e;
	int sae_pk_mode;
	uint8_t failure_retry_cnt;
	uint32_t he_flags;
	uint8_t sae_h2e_identifier[32];
} wifi_sta_config_t;

// wifi_ap_config_t of ESP-IDF v5.3.2 esp_wifi_types_generic.h. The enums are
// int.
typedef struct {
	uint8_t ssid[32];
	uint8_t password[64];
	uint8_t ssid_len;
	uint8_t channel;
	int authmode;
	uint8_t ssid_hidden;
	uint8_t max_connection;
	uint16_t beacon_interval;
	uint8_t csa_count;
	uint8_t dtim_period;
	int pairwise_cipher;
	bool ftm_responder;
	struct {
		bool capable;
		bool required;
	} pmf_cfg;
	int sae_pwe_h2e;
} wifi_ap_config_t;

// wifi_config_t is a union. The blobs copy all of it, so the size is larger
// than each member.
typedef union {
	wifi_sta_config_t sta;
	wifi_ap_config_t ap;
	uint8_t size[256];
} wifi_config_t;

// Blob functions. wifi_mode_t is an int enum, see -fno-short-enums.
int esp_wifi_init_internal(const wifi_init_config_t *config);
int esp_wifi_set_mode(int mode);
int esp_wifi_start(void);
int esp_wifi_stop(void);
int esp_wifi_set_config(int interface, wifi_config_t *conf);
// esp_wifi_connect and esp_wifi_disconnect of esp_wifi/src/wifi_init.c call
// these without the roaming app.
int esp_wifi_connect_internal(void);
int esp_wifi_set_ps(int type);
int esp_wifi_disconnect_internal(void);
int esp_wifi_scan_start(const void *config, bool block);
int esp_wifi_scan_get_ap_num(uint16_t *number);
int esp_wifi_scan_get_ap_records(uint16_t *number, wifi_ap_record_t *ap_records);

// libphy types and functions of ESP-IDF v5.3.2 esp_phy_init.h and
// esp_private/phy.h.
typedef struct {
	uint8_t params[128];
} esp_phy_init_data_t;

typedef struct {
	uint8_t version[4];
	uint8_t mac[6];
	uint8_t opaque[1894];
} esp_phy_calibration_data_t;

enum {
	PHY_RF_CAL_PARTIAL = 0,
	PHY_RF_CAL_NONE = 1,
	PHY_RF_CAL_FULL = 2,
};

int register_chipv7_phy(const esp_phy_init_data_t *init_data, esp_phy_calibration_data_t *cal_data, int cal_mode);
void phy_wakeup_init(void);
void phy_close_rf(void);
void phy_xpd_tsens(void);
void phy_param_track_tot(bool en_wifi, bool en_ble_154);
char *get_phy_version_str(void);
void phy_bbpll_en_usb(bool en);

extern const esp_phy_init_data_t seam_phy_init_data;

// wifi_wpa_ie_t of ESP-IDF v5.3.2 esp_supplicant/src/esp_wifi_driver.h.
typedef struct {
	int proto;
	int pairwise_cipher;
	int group_cipher;
	int key_mgmt;
	int capabilities;
	size_t num_pmkid;
	const uint8_t *pmkid;
	int mgmt_group_cipher;
	uint8_t rsnxe_capa;
} wifi_wpa_ie_t;

int tinygo_wifi_parse_wpa_ie(const uint8_t *wpa_ie, size_t wpa_ie_len, wifi_wpa_ie_t *data);

// struct wpa_funcs of ESP-IDF v5.3.2 esp_supplicant/src/esp_wifi_driver.h,
// with void pointers for the supplicant types.
typedef struct {
	bool (*wpa_sta_init)(void);
	bool (*wpa_sta_deinit)(void);
	int (*wpa_sta_connect)(uint8_t *bssid);
	void (*wpa_sta_connected_cb)(uint8_t *bssid);
	void (*wpa_sta_disconnected_cb)(uint8_t reason_code);
	int (*wpa_sta_rx_eapol)(uint8_t *src_addr, uint8_t *buf, uint32_t len);
	bool (*wpa_sta_in_4way_handshake)(void);
	void *(*wpa_ap_init)(void);
	bool (*wpa_ap_deinit)(void *data);
	bool (*wpa_ap_join)(void **sm, uint8_t *bssid, uint8_t *wpa_ie, uint8_t wpa_ie_len, uint8_t *rsnxe, uint8_t rsnxe_len, bool *pmf_enable, int subtype, uint8_t *pairwise_cipher);
	bool (*wpa_ap_remove)(uint8_t *bssid);
	uint8_t *(*wpa_ap_get_wpa_ie)(uint8_t *len);
	bool (*wpa_ap_rx_eapol)(void *hapd_data, void *sm, uint8_t *data, size_t data_len);
	void (*wpa_ap_get_peer_spp_msg)(void *sm, bool *spp_cap, bool *spp_req);
	char *(*wpa_config_parse_string)(const char *value, size_t *len);
	int (*wpa_parse_wpa_ie)(const uint8_t *wpa_ie, size_t wpa_ie_len, wifi_wpa_ie_t *data);
	int (*wpa_config_bss)(uint8_t *bssid);
	int (*wpa_michael_mic_failure)(uint16_t is_unicast);
	void *wpa3_build_sae_msg;
	void *wpa3_parse_sae_msg;
	void *wpa3_hostap_handle_auth;
	void *wpa_sta_rx_mgmt;
	void (*wpa_config_done)(void);
	void *owe_build_dhie;
	void *owe_process_assoc_resp;
	void (*wpa_sta_clear_curr_pmksa)(void);
} seam_wpa_funcs_t;

int esp_wifi_register_wpa_cb_internal(seam_wpa_funcs_t *cb);

// Blob functions for the supplicant, ESP-IDF v5.3.2
// esp_supplicant/src/esp_wifi_driver.h and esp_private/wifi.h.
struct wifi_ssid {
	int len;
	uint8_t ssid[32];
};

bool esp_wifi_sta_prof_is_rsn_internal(void);
uint8_t esp_wifi_sta_get_prof_authmode_internal(void);
uint8_t esp_wifi_sta_get_pairwise_cipher_internal(void);
uint8_t esp_wifi_sta_get_group_cipher_internal(void);
int esp_wifi_get_macaddr_internal(uint8_t if_index, uint8_t *macaddr);
bool esp_wifi_sta_is_ap_notify_completed_rsne_internal(void);
uint16_t esp_wifi_get_spp_attrubute_internal(uint8_t ifx);
int esp_wifi_set_appie_internal(uint8_t type, uint8_t *ie, uint16_t len, uint8_t flag);
int esp_wifi_sta_connect_internal(const uint8_t *bssid);
int esp_wifi_internal_tx(int wifi_if, void *buffer, uint16_t len);
int esp_wifi_register_eapol_txdonecb_internal(void (*fn)(uint8_t *payload, size_t len, bool tx_failure));
int esp_wifi_set_sta_key_internal(int alg, uint8_t *addr, int key_idx, int set_tx, uint8_t *seq, size_t seq_len, uint8_t *key, size_t key_len, int key_flag);
bool esp_wifi_auth_done_internal(void);

// The soft-AP functions of esp_supplicant/src/esp_wifi_driver.h.
int esp_wifi_set_ap_key_internal(int alg, const uint8_t *addr, int idx, uint8_t *key, size_t key_len);
uint8_t *esp_wifi_ap_get_prof_pmk_internal(void);
bool esp_wifi_wpa_ptk_init_done_internal(uint8_t *mac);
int esp_wifi_ap_deauth_internal(uint8_t *mac, uint32_t reason);
int esp_wifi_unset_appie_internal(uint8_t type);

// The AP part of the seam (wpa_cb.c and seam.c).
extern const uint8_t seam_ap_rsn[22];
int seam_wifi_set_ap(const uint8_t *ssid, uint8_t ssid_len, const uint8_t *password, uint8_t password_len, uint8_t channel, uint8_t max_connection);
int seam_ap_send_assoc_resp(uint8_t *da, uint16_t status, int subtype);
bool tinygo_wifi_wpa_ap_init(void);
void tinygo_wifi_wpa_ap_deinit(void);
bool tinygo_wifi_wpa_ap_join(uint8_t *bssid, uint8_t *ie, uint8_t ie_len, int subtype, int32_t *slot);
bool tinygo_wifi_wpa_ap_remove(uint8_t *bssid);
void tinygo_wifi_wpa_ap_rx_eapol(int32_t slot, uint8_t *data, size_t len);
int tinygo_wifi_rx_ap(void *buffer, uint16_t len, void *eb);
int seam_wifi_reg_rxcb_ap(void);
void esp_wifi_deauthenticate_internal(uint8_t reason_code);

// Go functions of the supplicant in supplicant.go.
void tinygo_wifi_wpa_sta_deinit(void);
int tinygo_wifi_wpa_sta_connect(uint8_t *bssid);
void tinygo_wifi_wpa_sta_disconnected(uint8_t reason_code);
int tinygo_wifi_wpa_sta_rx_eapol(uint8_t *src_addr, uint8_t *buf, uint32_t len);
bool tinygo_wifi_wpa_sta_in_4way_handshake(void);
void tinygo_wifi_eapol_txdone(uint8_t *payload, size_t len, bool tx_failure);
int seam_register_wpa_cb(void);

// RX of ESP-IDF v5.3.2 esp_wifi/include/esp_private/wifi.h.
void esp_wifi_internal_free_rx_buffer(void *buffer);
int tinygo_wifi_rx(void *buffer, uint16_t len, void *eb);
int seam_wifi_reg_rxcb(void);

// Bluetooth LE controller seam in bt_seam.c.
int seam_coex_pre_init(void);
int seam_bt_init(uint8_t max_act);
int seam_coex_init(void);
int seam_bt_enable(void);
void seam_bt_disable(void);
void seam_bt_deinit(void);
int seam_wifi_deinit(void);
int seam_bt_register_vhci(void);
bool seam_bt_can_send(void);
void seam_bt_send(uint8_t *data, uint16_t len);

// ESP-NOW, see espnow_seam.c and espnow.go.
int seam_espnow_init(void);
int seam_espnow_peer(const uint8_t *mac, const uint8_t *lmk, uint8_t channel, int ifidx, bool modify);
int seam_espnow_fetch_peer(int from_head, uint8_t *mac, uint8_t *channel, uint8_t *encrypt, uint8_t *ap);
int seam_espnow_rate(const uint8_t *mac, int phymode, int rate);
int esp_now_set_wake_window(uint16_t window);
int esp_wifi_connectionless_module_set_wake_interval(uint16_t wake_interval);
int esp_wifi_set_protocol(int ifx, uint8_t protocol_bitmap);
int esp_now_deinit(void);
int esp_now_send(const uint8_t *peer_addr, const uint8_t *data, size_t len);
int esp_now_del_peer(const uint8_t *peer_addr);
int esp_now_set_pmk(const uint8_t *pmk);
int esp_wifi_set_channel(uint8_t primary, int second);
int esp_wifi_get_channel(uint8_t *primary, int *second);
void tinygo_espnow_recv(uint8_t *src, uint8_t *dst, int8_t rssi, uint8_t *data, int len);
void tinygo_espnow_sent(uint8_t *mac, bool ok);

// Seam functions in seam.c.
int seam_wifi_init(bool psram, int dynamic_rx_buf_num);
void esp_wifi_set_sleep_min_active_time(uint32_t min_active_time);
void esp_wifi_set_keep_alive_time(uint32_t keep_alive_time);
void esp_wifi_set_sleep_wait_broadcast_data_time(uint32_t time);
bool seam_env_is_chip(void);
void seam_log_write(unsigned int level, const char *tag, const char *format, ...);
void seam_log_writev(unsigned int level, const char *tag, const char *format, va_list args);
void *seam_zalloc(size_t size);
void *seam_wifi_zalloc(size_t size);
void *seam_wifi_create_queue(int n, int size);
void seam_wifi_delete_queue(void *queue);
void seam_unimplemented(const char *name);
int32_t tinygo_crypto_pbkdf2_sha1(const uint8_t *passphrase, const uint8_t *ssid, uint32_t ssid_len, int32_t iterations, uint8_t *buf, uint32_t buflen);
void *tinygo_crypto_aes_init(const uint8_t *key, uint32_t len);
void tinygo_crypto_aes_deinit(void *ctx);
void tinygo_crypto_aes_encrypt(void *ctx, uint8_t *plain, uint8_t *crypt);
void tinygo_crypto_aes_decrypt(void *ctx, uint8_t *crypt, uint8_t *plain);
void seam_call(void *fn, void *arg);
void seam_intenable_set(uint32_t mask);
void seam_intenable_clear(uint32_t mask);

// Go functions, see the //export lines.
void *tinygo_wifi_malloc(size_t size);
void tinygo_wifi_free(void *p);
void *tinygo_wifi_calloc(size_t n, size_t size);
void *tinygo_wifi_realloc(void *p, size_t size);
void *tinygo_wifi_ext_calloc(size_t n, size_t size);
uint32_t tinygo_wifi_free_heap(void);
void tinygo_wifi_log(const char *text, int n);
void *tinygo_wifi_queue_create(uint32_t n, uint32_t size);
void tinygo_wifi_queue_delete(void *queue);
