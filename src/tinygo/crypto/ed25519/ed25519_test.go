package ed25519

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"testing"
)

// vectors are from crypto/ed25519 of Go 1.27 with pseudorandom seeds and
// messages, after the seeds and messages of RFC 8032 section 7.1 TEST 1 to 3,
// whose public keys and signatures there match.
var vectors = []struct {
	name, seed, pub, msg, sig string
}{
	{"RFC 8032 7.1 TEST 1", "9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60", "d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a", "", "e5564300c360ac729086e2cc806e828a84877f1eb8e5d974d873e065224901555fb8821590a33bacc61e39701cf9b46bd25bf5f0595bbe24655141438e7a100b"},
	{"RFC 8032 7.1 TEST 2", "4ccd089b28ff96da9db6c346ec114e0f5b8a319f35aba624da8cf6ed4fb8a6fb", "3d4017c3e843895a92b70aa74d1b7ebc9c982ccf2ec4968cc0cd55f12af4660c", "72", "92a009a9f0d4cab8720e820b5f642540a2b27b5416503f8fb3762223ebdb69da085ac1e43e15996e458f3613d0f11d8c387b2eaeb4302aeeb00d291612bb0c00"},
	{"RFC 8032 7.1 TEST 3", "c5aa8df43f9f837bedb7442f31dcb7b166d38535076f094b85ce3a2e0b4458f7", "fc51cd8e6218a1a38da47ed00230f0580816ed13ba3303ac5deb911548908025", "af82", "6291d657deec24024827e69c3abe01a30ce548a284743a445e3680d7db5ac3ac18ff9b538d16f290ae67f760984dc6594a7c15e9716ed28dc027beceea1ec40a"},
	{"zero seed", "0000000000000000000000000000000000000000000000000000000000000000", "3b6a27bcceb6a42d62a3a8d02a6f0d73653215771de243a63ac048a18b59da29", "616263", "885dfb07cab2796eb960531a2f09b972ad59b97bb125bef5fdda0855d6bebebf24447e705fa11575639df396c201ccf52a1a16b014a7a2f0ce73a7a161757308"},
	{"ff seed", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "76a1592044a6e4f511265bca73a604d90b0529d1df602be30a19a9257660d1f5", "616263", "aec6ab6a9122aff0f7dcb9667ff613136894732b6e78c26f5b673101e267fe2e2b65fa4d53dad478a1ada64d50fd1dfdb7d94920dc3e1a564a647b1cba356001"},
	{"0 bytes", "67a0081b96a14cbabeea8ecf5747bfa2ebb48b68a6bb9b994662e5ee78d21397", "d81844641ee04edb5cc475e5988094536401297080c318b3126d105e71e210b9", "", "3a815e91899d6eba6a4fa7747c08e31bd516f90a3f2878a4f4d28ea47089e540dd8beb4e4e5de6fec9970bf5a24602524ba59fb06eb4ac032d42a0e1729a960b"},
	{"1 bytes", "821984a04b964f55a1d128b7e16a868aec95fb9a1954b557a240603721442a36", "55a929654b3d1c0f16e11238534ed10e44853bd12ec6ee7a0444cb00ac6e4d34", "de", "2a079d0d98c3cf17438dd5e179a3cb7ac97483a7fd802fa3c75388119086c2fa341ec6fd2eb422cadc3d63084d77139ffd92f6a0da3995a15c077759d6568e0c"},
	{"31 bytes", "0c00b54e2b9dc9b152ac6caa94c44f31aafd67ee0d1d4e4f7925696a32773f89", "f23e741d3f9a2cbf2c234c95661ebd703f7e5143c46137fd7c5c6c4c2a6927ea", "5d063778ce2df45685c557ed8e0f4bb9c2a6603f07324b4768975d0ed74c66", "bce319b1837834a0ea71ff29aebb16446bc45a949e4952df4ce8c3a02f5d91627368395d687c852d15c4f470c610e82457a26860193c992a7f762b181d427703"},
	{"32 bytes", "552e2ec17c17740483da7d09c7afbb20befb1a3f35409a87d79d39744edf8189", "9916d93f9866bcb8bfb85226fedeec48d374c101dc130a6029594f5dc15f2125", "2c636b327c5621045b4f68e674cda8cb34fed07aaa96dc2ef8708f01e58bc37e", "1bde9849374c1d7bc6b4baf3f0a0e0100773d3abf75e2df53776e5f0180d13dd833e5ab0d697fa95f057a87ae8222ec692fd2060aa3ded96e5cd29e311282406"},
	{"63 bytes", "15f88ace9d011599e19a1022eca451f4452e5982aac21eb4929cb674a8de5148", "7b67896095f20b12b310c207c888aca6a4b31be4188fd61351664834c780e6fa", "7a05b1bc9ac323744d90e8cbf6df66a5eb763cdde7becf0394cad575f9370f390a42bcc63cd350b8ce29bd9e7ee9fc873fa537f87c71a3377390116dca4576", "4aee1913b7e8da717c25c1edc018f8a6e098e02c0fa3d828a534a884578b640eac816f69ec2ba602cfed5be3024818873930e5cef51ba56e9588eca46546d907"},
	{"64 bytes", "e6efb1fd60f3ee7558876703127a50f2a153a8be4d1ef19cd114bbc35211f795", "b32fa9844a3134fe9d42363b7b63276c824a266c16ace74ba292bc038ed92159", "7805deaabfdd08b0568db0b8a8c7ab1947dd0c86ca2f300a71060bc3784f204ed69ef03878618adafbd67ec8d7d76d6ca7c7830777ded2fe4b5dba790591b41d", "4c8a7dad663e1537d236f7e351b96d655c97260e36ad69ae7c9f2770136c69d995cf053b8b24a32a11bdfb627bc471a1a833d2d287de4c76fd9c473cbe9e4203"},
	{"65 bytes", "b4f07f8edabcdf6c7dedda2a1b151c6dec73fb94c4738a8320519e0fc3554d4b", "b0481ec58d0ccfbe1fa64caabc392e43189dd6d59b23559e4c3f37d809419bce", "56822b60edaecd6e666045ff32896225b59b65481a9e14b9bfedd0dcdd58e1e58ec65804175a57b131d20c0ed8b7265946807a41297faeb928e5fca2afb57b42c5", "19ac88bbe3438eb91cfc50506391a5bcc8037bf250c483175d329c2615286041fb40264ed45f66efa9213e67fa21c1545716e0f40b55a5ffa7eed60ef4961b03"},
	{"95 bytes", "37642f91204ccf169784b830b8e5aa0db6246f5da84d1d4c1aeb9e7ad09713eb", "49c12a8dea5802634a1391ab73d85aec533487ab825f1cc872d72561c6869e44", "3040e5c7b15d921a3a5ab1cb17212b0c5c7ac7a246953d5fdf7f804fe1634b667c5c971b78850b4000c8f7c3deb7acec467c19505f9908fd7321f73010c61fe1ef1c716ea8d01bffda74ce4e3fc0ca453b0dc1326272a1b3e4789b7f2c068a", "7f923f6646618471a58f09ddaee4e878c9da3a0a381ce44d46af80854de454f1d77b22ec0a1961847e483162e228ff81b3df6533991f0e8ddf133f60cb7cb906"},
	{"96 bytes", "d1aba3f8583b414f94fe5ef42f89fbd9991eaef68974619832858b5d16ad9cef", "d414166a823395150b86457ea95e212fd338a59c8ccd8bbe3a7ac28011b42a9d", "8d8a9728a782b4586ce8537e925532810d4bac05768bb70e18400d62486a2d4c9a842f349c04250fa171e936abccce2136c9b8090b11f2e82072cc143945b85f6837cc253631be0c3f8514419c96ee978947c3802263ea1c013bcdcb2d2eedf0", "b138feb74c63b087643b222c79d518b05cef4dcf5f6d409c6ce150b08e61d26d29d38de9022bec9822b83830b559315f08ff02de63a72eda9d83a45ce0e9840b"},
	{"127 bytes", "f90db624b9de60e2875285a606dd71af0572656995b9bd158f558f31bee12e0c", "1428e7523fefb511024bee4eb9b3630c643baa8a9a74cfe941525acb213d30ef", "16912cf1b7760262d55759d2cbd7a7e5ee01340d791081484477b65e94b83a150773cc63911689c2b67dd52181c1ac53587646ed31574534635f8037f0b7b9b545b66d1622d49cfddbe45a7fed0d4598928aa4e8d22568a21433266ccae211e7dc24e170a90d48d470c6c0af9118854c0ef72b85badf6b7b0c2729c3742e0f", "88e8bbda702be5f3a1dbe17ac480782db8486fde1ad115205944d46e38ea8ed17ecc09338fab0c986501c60085d095bf840a3598941bd3fc223683f5a7a2800f"},
	{"128 bytes", "eacf182527b618314f7d6eed7ea5eb2d7b402b9f4fbc48bdf533f3ad476b43ae", "71c7381cd267d82996c2ab913b56f0f08adfff103606d710c552d66ff11590b5", "b774a149076036b7cf546d4bd2473e25bbbc492664d93e13e96fdac5bb6d0e0f951c7a05047f86b6593116e5cf3b718293e4c0989090367dd0f71cb8ebb8c94ccee1e3069755bbb8f6a4ef338b2a32eed5208790bb69e0891950ea10e8bac5033a8cd01a02ff5ae23a02bdb851fcb37931f955395c0814c97f940f76df419834", "567735ef39bed6c9a243cde15be8c042d795f0f27607b12d08c8688b61c299d451a321c00e1c46abe1a8f1b24d907f867715e25485987fc6bc49a73e75429709"},
	{"129 bytes", "0f3eb0ad27298ddb0fffa6422f03435e417040e08f1e362f7375c7bb95795cae", "f37aa9cf3e3587f08d6d641fc7c6c6d4929cfeb475fcf03162ce9804c312b87e", "28e67ec412584a1adf71fa3bddc807baf22a7d15bc9a19de7fbc36f30196c9825fe220dbd9bdc7284c5a5c8cbc9082728626e3e11684d469eae751f18ecf07fa48439e221802a35bf8acbb3820dfe2d12747d42acce13c12de1f2521e1cd78834e5136ad16e4df893d3affc2544ac443d91cf590b0b7eccc66ac1240628192a1e3", "e8dbf3ff11176056c6a2c23ac98a6a03bacff812ca7b8574d7b1afc1f960b3c13442b053bf7ba7f1a4929c4cea42d544d1b542532767692320782622db97b709"},
	{"1000 bytes", "46a3a48719db0593ab48137f9a48bcfed97f2032866bb4ddedf859f8ed27294d", "d562d03e765329d8a65cccfe7505656f8de5aa920f707eb93b3bc529a578b451", "4659d147256741321aa2f4e9cf3fcf79e4d30827c0d397eee857c56a772d964630471c0397341331a744a10d8c43410f31e68483b91bd22c956d8f854cdc45d5887c7bfdc11778aa076a2d09c1473b8b3e39245c813cfe24e1ee1c0dbaf4c251922b65715e9d04bcd68b22b94b7672ea49993fd231e43cbd83e580d2344a4287514c0c30d245cd02b78b6b99c7d3b8523261bbdf4eb624b7e04843ac5458355e32604b8cd2f24c505fa7e559748f60b017da1cf9c49826ca5f07a66642e2781ac4bd991c1957d8ad8bc90f8c9cb981ba7ee6293b45fe8e7c3b9e9c3ef25378f62732d149c94d54bc81e83b3f8cc4acdc31a0b582769d89cf42974759864325bbb6486619ecefea04ad5caf961c61d8a15402a60d7840144181635bfeed2800193ac09b5538902a45a58ee905cc5d19c299d20962c58a5d72df7f5dc95dbc63e425b6347f58c987bbf9d6033490d1a491e90955737c0c8a43c8bf985039f0fc601b0f2fbda903c4ae442746e704762307fb65a87e27195cd9d294ce036ad2c5411406a3e0757bc3e72f36f7e1384f2821a03f1fef01f85033c7d2874644af798a758ceec355b07c96186ed88b2f6a244b1893f8c25c6bbda81e416b8652508aa5ef74148da71753e5f5ebf53f2f34ccf45d2f469ee26d4a6bc13547b816798a9d494574b3e147c82da3b05e466584eef6eed884ccc2e7f214455af38ab0366dafce27f41116d3644fe046595bc05ea9145e57fd24c40e8cd00ea6ebd01f4d5648d561b05089d28e0d0d0023942cc28ad1c92381dfe0bc6bdb5351295ff421669563ee6269bc5b2d5ed4cb7f6fb1d5f6fff56df841324df43e6094fb8124b6ed3bea3e97613181957cdfde1a23e38d69a4de66c7d62f15e336ec46ba481c9d509081bea993f20293961cd4393e5f1ee14067f2b671560f74459ab32b35c04cfb130b6b15c1f50a38445c67efff53c8d799a261f0d785c05a3738c96f5c53a39ba0ef9a8b1eeafe89930a0c184b1808971d7baf1dc0b514e682ab36fc61940ed9bd078c2484da2c765b311d0ac9cd80e8c2349ac33951257b701c40cddcbf89a9f2f299f2c01ffd6e544ad3413eb0c920815e8e8e1964903e508e0503f4eb8b3a989b8a9e5162a0db41368e5f4394f549005f9d23401c684a1beefe237042b78d741c46974535020c9e73ec29ee221e7c425e76f66531dcfffd3b16a6139683790b2c120de06da95aeab00c0a32552f214269ab002f965a5b2b474905b0a4699640a27ca56e0768d61cb46c9c52259aac0a2e25ccc30d4341745af875a3590a22b8c4d2a74f5b95ce29d95fcc838de58cb4cb0dd032f0305741521529b2de5f64a15e2fbdfbb22d137f143082b21b3ef73d0e0a92e50f4ffaf60c3a789b71196cc8c63f4f731be97701", "8e04fabc38a86561e21d0b26c269c0a775b77adf4091321d3c15a741b9c91d99a06d51b2645e5d09ed1a55e0604ea370267e8379ca2f8f9154fcc3b1cdd27504"},
}

// edges are crafted keys and signatures with the verdict of crypto/ed25519
// of Go 1.27, which accepts keys of small order and non-canonical y.
var edges = []struct {
	name, pub, msg, sig string
	ok                  bool
}{
	{"identity key, identity R, S 0", "0100000000000000000000000000000000000000000000000000000000000000", "616263", "01000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000", true},
	{"x 0 with sign bit, identity R, S 0", "0100000000000000000000000000000000000000000000000000000000000080", "616263", "01000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000", true},
	{"y = p + 1 key, identity R, S 0", "eeffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f", "616263", "01000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000", true},
	{"identity key, R = y p + 1, S 0", "0100000000000000000000000000000000000000000000000000000000000000", "616263", "eeffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f0000000000000000000000000000000000000000000000000000000000000000", false},
	{"y = 2 not on curve", "0200000000000000000000000000000000000000000000000000000000000000", "616263", "01000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000", false},
	{"all ff key", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "616263", "01000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000", false},
}

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestVectors derives each key from its seed, signs, and verifies.
func TestVectors(t *testing.T) {
	for _, tt := range vectors {
		t.Run(tt.name, func(t *testing.T) {
			seed, pub, msg, sig := unhex(t, tt.seed), unhex(t, tt.pub), unhex(t, tt.msg), unhex(t, tt.sig)
			priv := NewKeyFromSeed(seed)
			if !bytes.Equal(priv.Public(), pub) || !bytes.Equal(priv.Seed(), seed) {
				t.Fatalf("public key %x, want %x", priv.Public(), pub)
			}
			if got := Sign(priv, msg); !bytes.Equal(got, sig) {
				t.Fatalf("Sign = %x, want %x", got, sig)
			}
			if !Verify(pub, msg, sig) {
				t.Fatal("Verify = false")
			}
		})
	}
}

// TestEdges checks the verdict of crypto/ed25519 on crafted inputs.
func TestEdges(t *testing.T) {
	for _, tt := range edges {
		if got := Verify(unhex(t, tt.pub), unhex(t, tt.msg), unhex(t, tt.sig)); got != tt.ok {
			t.Errorf("%s: Verify = %v, want %v", tt.name, got, tt.ok)
		}
	}
}

func scalarBytes(s *scalar) []byte {
	var b [32]byte
	s.bytes(&b)
	return b[:]
}

func reverse(b []byte) []byte {
	r := make([]byte, len(b))
	for i, v := range b {
		r[len(b)-1-i] = v
	}
	return r
}

// leBig and bigLE convert between math/big and 32 little-endian bytes.
func leBig(b []byte) *big.Int { return new(big.Int).SetBytes(reverse(b)) }

func bigLE(v *big.Int) []byte {
	b := v.Bytes()
	return reverse(append(make([]byte, 32-len(b)), b...))
}

func cat(a, b []byte) []byte { return append(append([]byte(nil), a...), b...) }

// TestVerifyFailures changes one input of a valid signature at a time.
func TestVerifyFailures(t *testing.T) {
	v := vectors[len(vectors)-1] // 1000 bytes
	pub, msg, sig := unhex(t, v.pub), unhex(t, v.msg), unhex(t, v.sig)
	other := unhex(t, vectors[0].pub)
	flip := func(b []byte, i int, bits byte) []byte {
		c := append([]byte(nil), b...)
		c[i] ^= bits
		return c
	}
	// S + L is the same S modulo L, so only the check S < L rejects it.
	l := leBig(scalarBytes(&order))
	plusL := cat(sig[:32], bigLE(new(big.Int).Add(leBig(sig[32:]), l)))
	tests := []struct {
		name          string
		pub, msg, sig []byte
	}{
		{"wrong public key", other, msg, sig},
		{"public key not on the curve", flip(pub, 0, 1), msg, sig},
		{"tampered message", pub, flip(msg, 500, 1), sig},
		{"truncated message", pub, msg[:999], sig},
		{"tampered R", pub, msg, flip(sig, 0, 1)},
		{"tampered S", pub, msg, flip(sig, 32, 1)},
		{"S + L (malleable)", pub, msg, plusL},
		{"S = L", pub, msg, cat(sig[:32], scalarBytes(&order))},
		{"S top bits set", pub, msg, flip(sig, 63, 0xe0)},
		{"short signature", pub, msg, sig[:63]},
		{"long signature", pub, msg, cat(sig, []byte{0})},
		{"short public key", pub[:31], msg, sig},
		{"nil public key", nil, msg, sig},
		{"nil signature", pub, msg, nil},
	}
	for _, tt := range tests {
		if Verify(tt.pub, tt.msg, tt.sig) {
			t.Errorf("%s: Verify = true", tt.name)
		}
	}
}

// TestSizes checks the panics on a bad seed or private key, and the sizes.
func TestSizes(t *testing.T) {
	if PublicKeySize != 32 || PrivateKeySize != 64 || SignatureSize != 64 || SeedSize != 32 {
		t.Fatal("sizes")
	}
	tests := []struct {
		name string
		f    func()
		want Error
	}{
		{"31-byte seed", func() { NewKeyFromSeed(make([]byte, 31)) }, ErrSeedSize},
		{"64-byte seed", func() { NewKeyFromSeed(make([]byte, 64)) }, ErrSeedSize},
		{"32-byte private key", func() { Sign(make(PrivateKey, 32), nil) }, ErrPrivateKeySize},
		{"nil private key", func() { Sign(nil, nil) }, ErrPrivateKeySize},
	}
	for _, tt := range tests {
		func() {
			defer func() {
				if r := recover(); r != tt.want {
					t.Errorf("%s: panic %v, want %v", tt.name, r, tt.want)
				}
			}()
			tt.f()
		}()
	}
}

type countReader byte

func (c *countReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(*c)
		*c++
	}
	return len(p), nil
}

type failReader struct{}

func (failReader) Read(p []byte) (int, error) { return 0, ErrRandom }

// TestGenerateKey checks that GenerateKey uses the seed from rand, and
// fails with ErrRandom on a nil or failing reader.
func TestGenerateKey(t *testing.T) {
	var c countReader
	pub, priv, err := GenerateKey(&c)
	if err != nil {
		t.Fatal(err)
	}
	var seed [SeedSize]byte
	for i := range seed {
		seed[i] = byte(i)
	}
	want := NewKeyFromSeed(seed[:])
	if !priv.Equal(want) || !pub.Equal(want.Public()) || priv.Equal(want[:63]) || pub.Equal(PublicKey(seed[:])) {
		t.Fatalf("GenerateKey = %x", priv)
	}
	msg := []byte("pair-verify")
	if !Verify(pub, msg, Sign(priv, msg)) {
		t.Fatal("Verify = false")
	}
	if _, _, err := GenerateKey(nil); err != ErrRandom {
		t.Errorf("nil reader: %v", err)
	}
	if _, _, err := GenerateKey(failReader{}); err != ErrRandom {
		t.Errorf("failing reader: %v", err)
	}
}

// TestConstants checks the curve constants, L and 2^504 mod L with math/big.
func TestConstants(t *testing.T) {
	p := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(19))
	fb := func(e *fe) *big.Int {
		var b [32]byte
		e.Bytes(&b)
		return leBig(b[:])
	}
	mod := func(x *big.Int) *big.Int { return x.Mod(x, p) }
	d := fb(&dConst)
	if mod(new(big.Int).Mul(d, big.NewInt(121666))).Cmp(mod(big.NewInt(-121665))) != 0 {
		t.Error("d")
	}
	if fb(&d2).Cmp(mod(new(big.Int).Lsh(d, 1))) != 0 {
		t.Error("2d")
	}
	i := fb(&sqrtM1)
	if mod(new(big.Int).Mul(i, i)).Cmp(new(big.Int).Sub(p, big.NewInt(1))) != 0 {
		t.Error("sqrt(-1)")
	}
	x, y := fb(&base.x), fb(&base.y)
	if mod(new(big.Int).Mul(y, big.NewInt(5))).Cmp(big.NewInt(4)) != 0 || x.Bit(0) != 0 {
		t.Error("B: y = 4/5 and x even")
	}
	xx, yy := mod(new(big.Int).Mul(x, x)), mod(new(big.Int).Mul(y, y))
	lhs := mod(new(big.Int).Sub(yy, xx)) // -x^2 + y^2
	rhs := mod(new(big.Int).Add(big.NewInt(1), new(big.Int).Mul(d, new(big.Int).Mul(xx, yy))))
	if lhs.Cmp(rhs) != 0 || fb(&base.t).Cmp(mod(new(big.Int).Mul(x, y))) != 0 {
		t.Error("B is not on the curve, or T != xy")
	}
	l, _ := new(big.Int).SetString("27742317777372353535851937790883648493", 10)
	l.Add(l, new(big.Int).Lsh(big.NewInt(1), 252))
	if leBig(scalarBytes(&order)).Cmp(l) != 0 {
		t.Error("L")
	}
	if leBig(scalarBytes(&c504)).Cmp(new(big.Int).Exp(big.NewInt(2), big.NewInt(504), l)) != 0 {
		t.Error("2^504 mod L")
	}
}

// TestReduce compares reduce and mulAdd with math/big at the edges and on
// pseudorandom values.
func TestReduce(t *testing.T) {
	l := leBig(scalarBytes(&order))
	var inputs [][64]byte
	var h [64]byte
	inputs = append(inputs, h) // 0
	for i := range h {
		h[i] = 0xff
	}
	inputs = append(inputs, h) // 2^512 - 1
	h = [64]byte{}
	copy(h[:], scalarBytes(&order))
	inputs = append(inputs, h) // L
	h[0]--
	inputs = append(inputs, h) // L - 1
	for i := 0; i < 100; i++ {
		for j := range h {
			h[j] = byte(i*131 + j*j*7 + j)
		}
		inputs = append(inputs, h)
	}
	for _, in := range inputs {
		var s scalar
		s.reduce(&in)
		want := new(big.Int).Mod(leBig(in[:]), l)
		if got := leBig(scalarBytes(&s)); got.Cmp(want) != 0 {
			t.Fatalf("reduce(%x) = %x, want %x", in, got, want)
		}
		var m scalar
		m.mulAdd(&s, &s, &s)
		w := new(big.Int).Mul(want, want)
		w.Add(w, want).Mod(w, l)
		if g := leBig(scalarBytes(&m)); g.Cmp(w) != 0 {
			t.Fatalf("mulAdd = %x, want %x", g, w)
		}
	}
}

// TestAllocs requires NewKeyFromSeedInto, SignInto and Verify to allocate
// nothing, and checks that the count works with Sign.
func TestAllocs(t *testing.T) {
	v := vectors[len(vectors)-1]
	seed, pub, msg, sig := unhex(t, v.seed), unhex(t, v.pub), unhex(t, v.msg), unhex(t, v.sig)
	var priv [PrivateKeySize]byte
	var out [SignatureSize]byte
	tests := []struct {
		name string
		f    func()
	}{
		{"NewKeyFromSeedInto", func() { NewKeyFromSeedInto(&priv, (*[SeedSize]byte)(seed)) }},
		{"SignInto", func() { SignInto(&out, priv[:], msg) }},
		{"Verify", func() {
			if !Verify(pub, msg, sig) {
				t.Fatal("Verify = false")
			}
		}},
		{"Verify failure", func() { Verify(pub, msg[1:], sig) }},
	}
	for _, tt := range tests {
		if n := testing.AllocsPerRun(5, tt.f); n != 0 {
			t.Errorf("%s: %v allocations, want 0", tt.name, n)
		}
	}
	if !bytes.Equal(out[:], sig) {
		t.Errorf("SignInto = %x, want %x", out, sig)
	}
	if n := testing.AllocsPerRun(5, func() { Sign(priv[:], msg) }); n < 1 {
		t.Errorf("Sign: %v allocations, want 1", n)
	}
}
