package sha512

import (
	"encoding/hex"
	"testing"
)

// Vectors from Python's hmac and hashlib; the second is RFC 4231 test case 1.
var hmacVectors = []struct{ key, msg, sum string }{
	{"6b6579", "The quick brown fox jumps over the lazy dog", "b42af09057bac1e2d41708e48a902e09b5ff7f12ab428a4fe86653c73dd248fb82f948a549f7b791a5b41915ee4d1ec3935357e4e2317250d0372afa2ebeeb3a"},
	{"0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b", "Hi There", "87aa7cdea5ef619d4ff0b4241a1d6cb02379f4e2ce4ec2787ad0b30545e17cdedaa833b7d6b8a702038b274eaea3f4e4be9d914eeb61f1702e696c203a126854"},
	{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "Test Using Larger Than Block-Size Key - Hash Key First", "80b24263c7c1a3ebb71493c1dd7be8b49b46d1f41b4aeec1121b013783f8f3526b56d037e05f2598bd0fd2215d6a1e5295e64f73f63f0aec8b915a985d786598"},
	{"", "", "b936cee86c9f87aa5d3c6f2e84cb5a4239a5fe50480a6ec66b70ab5b1f4ac6730c6c515421b327ec1d69402e53dfb49ad7381eb067b338fd7b0cb22247225d47"},
}

func TestHMAC(t *testing.T) {
	for _, v := range hmacVectors {
		key, _ := hex.DecodeString(v.key)
		var h HMAC
		h.Init(key)
		var out [Size]byte
		// The message split across Sum's four parts gives the same MAC.
		m := []byte(v.msg)
		h.Sum(&out, m[:len(m)/3], m[len(m)/3:len(m)/2], nil, m[len(m)/2:])
		if got := hex.EncodeToString(out[:]); got != v.sum {
			t.Errorf("HMAC(%q, %q) = %s, want %s", v.key, v.msg, got, v.sum)
		}
	}
}
