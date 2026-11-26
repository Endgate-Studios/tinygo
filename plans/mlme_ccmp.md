Yes. tiny_core's ieee80211 package is in src/net/wifi/ieee80211/, with all 7 files (data.go, frame.go, ie.go, mgmt.go, rsn.go and the two test files). Its host tests pass with tinygo test net/wifi/ieee80211.

Today the Wi-Fi code uses only one thing from it: the ieee80211.MAC type, in eapol and in our supplicant. The frame builders (ToDSData, the management headers, the element and RSN helpers) are not used. With the blobs, the blob builds the 802.11 frames itself, and we only send and receive Ethernet frames.

Two related tiny_core packages are not copied:

Package	What it is	Why it is not needed with the blobs
mlme	the station state machine: scan, authenticate, associate, WPA2 and the data path	the blob does all of this
ccmp	CCMP-128 encryption of 802.11 data frames in software	the MAC hardware does CCMP after we install the keys
mlme, ccmp and most of ieee80211 are for a Wi-Fi stack without the blobs, where Go builds the raw 802.11 frames. That goes with the espmac work that you told me to ignore. I can copy mlme and ccmp too if you want them here for later.