//go:build !esp32p4

// The struct wpa_funcs table of ESP-IDF v5.3.2
// wpa_supplicant/esp_supplicant/src/esp_wifi_driver.h. supplicant.go has the
// station supplicant and ap.go the soft-AP authenticator. The other entries that esp_supplicant_init sets report
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

// The soft-AP authenticator of ESP-IDF v5.3.2 esp_supplicant/src/esp_hostap.c
// and src/ap/wpa_auth.c. ap.go does the 4-way handshake.

// seam_ap_rsn is the RSN element of the AP: WPA2-PSK with CCMP for the group
// and the pairwise cipher, and no capabilities (src/ap/wpa_auth_ie.c).
const uint8_t seam_ap_rsn[22] = {
	0x30, 0x14, 0x01, 0x00, 0x00, 0x0f, 0xac, 0x04, 0x01, 0x00, 0x00,
	0x0f, 0xac, 0x04, 0x01, 0x00, 0x00, 0x0f, 0xac, 0x02, 0x00, 0x00,
};

// The blobs only test the hostapd pointer for NULL.
static uint8_t seam_hapd;

static void *wpa_ap_init(void) {
	return tinygo_wifi_wpa_ap_init() ? &seam_hapd : NULL;
}

static bool wpa_ap_deinit(void *data) {
	tinygo_wifi_wpa_ap_deinit();
	return true;
}

// wpa_ap_join is hostap_sta_join of esp_wpa_main.c. *sm is the station slot
// plus 1. pairwise_cipher is the bit index of WPA_CIPHER_CCMP.
static bool wpa_ap_join(void **sm, uint8_t *bssid, uint8_t *wpa_ie, uint8_t wpa_ie_len, uint8_t *rsnxe, uint8_t rsnxe_len, bool *pmf_enable, int subtype, uint8_t *pairwise_cipher) {
	int32_t slot;
	if (!tinygo_wifi_wpa_ap_join(bssid, wpa_ie, wpa_ie_len, subtype, &slot)) {
		return false;
	}
	*sm = (void *)(uintptr_t)(slot + 1);
	*pmf_enable = false;
	*pairwise_cipher = 3;
	return true;
}

static bool wpa_ap_remove(uint8_t *bssid) {
	return tinygo_wifi_wpa_ap_remove(bssid);
}

static uint8_t *wpa_ap_get_wpa_ie(uint8_t *len) {
	*len = sizeof(seam_ap_rsn);
	return (uint8_t *)seam_ap_rsn;
}

// data starts at the 802.1X header.
static bool wpa_ap_rx_eapol(void *hapd_data, void *sm, uint8_t *data, size_t data_len) {
	if (sm != NULL) {
		tinygo_wifi_wpa_ap_rx_eapol((int32_t)((uintptr_t)sm - 1), data, data_len);
	}
	return true;
}

// The AP does not use SPP A-MSDU.
static void wpa_ap_get_peer_spp_msg(void *sm, bool *spp_cap, bool *spp_req) {
	*spp_cap = false;
	*spp_req = false;
}

static const seam_wpa_funcs_t seam_wpa_cb = {
	.wpa_sta_init = wpa_sta_init,
	.wpa_sta_deinit = wpa_sta_deinit,
	.wpa_sta_connect = tinygo_wifi_wpa_sta_connect,
	.wpa_sta_connected_cb = wpa_sta_connected_cb,
	.wpa_sta_disconnected_cb = tinygo_wifi_wpa_sta_disconnected,
	.wpa_sta_rx_eapol = tinygo_wifi_wpa_sta_rx_eapol,
	.wpa_sta_in_4way_handshake = tinygo_wifi_wpa_sta_in_4way_handshake,
	.wpa_ap_init = wpa_ap_init,
	.wpa_ap_deinit = wpa_ap_deinit,
	.wpa_ap_join = wpa_ap_join,
	.wpa_ap_remove = wpa_ap_remove,
	.wpa_ap_get_wpa_ie = wpa_ap_get_wpa_ie,
	.wpa_ap_rx_eapol = wpa_ap_rx_eapol,
	.wpa_ap_get_peer_spp_msg = wpa_ap_get_peer_spp_msg,
	.wpa_config_parse_string = wpa_config_parse_string,
	.wpa_parse_wpa_ie = tinygo_wifi_parse_wpa_ie,
	.wpa_michael_mic_failure = wpa_michael_mic_failure,
	.wpa_config_done = wpa_config_done,
	.wpa_sta_clear_curr_pmksa = wpa_sta_clear_curr_pmksa,
};

// The blobs free the table in esp_wifi_unregister_wpa_cb_internal, so it is a
// copy in the blob heap, as the os_zalloc of esp_supplicant_init.
int seam_register_wpa_cb(void) {
	seam_wpa_funcs_t *cb = tinygo_wifi_malloc(sizeof(seam_wpa_cb));
	if (cb == NULL) {
		return 0x101; // ESP_ERR_NO_MEM
	}
	*cb = seam_wpa_cb;
	return esp_wifi_register_wpa_cb_internal(cb);
}
