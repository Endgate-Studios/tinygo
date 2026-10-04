// The struct wpa_funcs table of ESP-IDF v5.3.2
// wpa_supplicant/esp_supplicant/src/esp_wifi_driver.h. supplicant.go has the
// station supplicant. The other entries that esp_supplicant_init sets report
// their name once, and the rest are NULL, as in ESP-IDF.

#include "seam.h"

#define REPORT_ONCE(name)                  \
	do {                                   \
		static bool done;                  \
		if (!done) {                       \
			done = true;                   \
			seam_unimplemented("wpa " name); \
		}                                  \
	} while (0)

// wpa_attach of esp_supplicant/src/esp_wpa_main.c registers the EAPOL TX
// done callback. The keys of the 4-way handshake are set there.
static bool wpa_sta_init(void) {
	return esp_wifi_register_eapol_txdonecb_internal(tinygo_wifi_eapol_txdone) == 0;
}

static bool wpa_sta_deinit(void) {
	esp_wifi_register_eapol_txdonecb_internal(NULL);
	tinygo_wifi_wpa_sta_deinit();
	return true;
}

// supplicant_sta_conn_handler does work only with 802.11k, v or r.
static void wpa_sta_connected_cb(uint8_t *bssid) {
}

static char *wpa_config_parse_string(const char *value, size_t *len) {
	REPORT_ONCE("wpa_config_parse_string");
	return NULL;
}

static int wpa_michael_mic_failure(uint16_t is_unicast) {
	REPORT_ONCE("wpa_michael_mic_failure");
	return -1;
}

// esp_set_scan_ie does work only with 802.11k and v.
static void wpa_config_done(void) {
}

// There is no PMKSA cache.
static void wpa_sta_clear_curr_pmksa(void) {
}

static seam_wpa_funcs_t seam_wpa_cb = {
	.wpa_sta_init = wpa_sta_init,
	.wpa_sta_deinit = wpa_sta_deinit,
	.wpa_sta_connect = tinygo_wifi_wpa_sta_connect,
	.wpa_sta_connected_cb = wpa_sta_connected_cb,
	.wpa_sta_disconnected_cb = tinygo_wifi_wpa_sta_disconnected,
	.wpa_sta_rx_eapol = tinygo_wifi_wpa_sta_rx_eapol,
	.wpa_sta_in_4way_handshake = tinygo_wifi_wpa_sta_in_4way_handshake,
	.wpa_config_parse_string = wpa_config_parse_string,
	.wpa_parse_wpa_ie = tinygo_wifi_parse_wpa_ie,
	.wpa_michael_mic_failure = wpa_michael_mic_failure,
	.wpa_config_done = wpa_config_done,
	.wpa_sta_clear_curr_pmksa = wpa_sta_clear_curr_pmksa,
};

int seam_register_wpa_cb(void) {
	return esp_wifi_register_wpa_cb_internal(&seam_wpa_cb);
}
