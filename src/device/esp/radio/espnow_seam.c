// The ESP-NOW seam: the callbacks and the peer struct of ESP-IDF v5.3.2
// components/esp_wifi/include/esp_now.h. libespnow.a has the protocol.

#include <string.h>

#include "seam.h"

typedef struct {
	uint8_t peer_addr[6];
	uint8_t lmk[16];
	uint8_t channel;
	int ifidx; // wifi_interface_t
	bool encrypt;
	void *priv;
} esp_now_peer_info_t;

typedef struct {
	uint8_t *src_addr;
	uint8_t *des_addr;
	int8_t *rx_ctrl; // wifi_pkt_rx_ctrl_t, the first byte is the RSSI
} esp_now_recv_info_t;

int esp_now_init(void);
int esp_now_deinit(void);
int esp_now_register_recv_cb(void (*cb)(const esp_now_recv_info_t *, const uint8_t *, int));
int esp_now_register_send_cb(void (*cb)(const uint8_t *, int));
int esp_now_add_peer(const esp_now_peer_info_t *peer);
int esp_now_mod_peer(const esp_now_peer_info_t *peer);

static void recv_cb(const esp_now_recv_info_t *info, const uint8_t *data, int len) {
	int8_t rssi = info->rx_ctrl != NULL ? info->rx_ctrl[0] : 0;
	tinygo_espnow_recv(info->src_addr, info->des_addr, rssi, (uint8_t *)data, len);
}

static void send_cb(const uint8_t *mac, int status) {
	tinygo_espnow_sent((uint8_t *)mac, status == 0); // ESP_NOW_SEND_SUCCESS
}

int seam_espnow_init(void) {
	int err = esp_now_init();
	if (err != 0) {
		return err;
	}
	err = esp_now_register_recv_cb(recv_cb);
	if (err == 0) {
		err = esp_now_register_send_cb(send_cb);
	}
	if (err != 0) {
		esp_now_deinit();
	}
	return err;
}

// seam_espnow_peer adds the peer, or changes it when modify is true. A NULL
// lmk is a peer without encryption.
int seam_espnow_peer(const uint8_t *mac, const uint8_t *lmk, uint8_t channel, int ifidx, bool modify) {
	esp_now_peer_info_t p;
	memset(&p, 0, sizeof(p));
	memcpy(p.peer_addr, mac, 6);
	if (lmk != NULL) {
		memcpy(p.lmk, lmk, 16);
		p.encrypt = true;
	}
	p.channel = channel;
	p.ifidx = ifidx;
	return modify ? esp_now_mod_peer(&p) : esp_now_add_peer(&p);
}
