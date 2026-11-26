package chacha20poly1305

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// vectors are from golang.org/x/crypto/chacha20poly1305 v0.58.0 with
// pseudorandom keys, nonces and messages. The last one is the input of RFC
// 8439 section 2.8.2, whose output there matches.
var vectors = []struct {
	name, key, nonce, plain, ad, sealed string
}{
	{"empty", "4f6f5741b47e9db76d4deebc711aba44501ad50e4e0018968d852947a952e94a", "f6e59f20c465af7b091d29cc", "", "", "b269573c1e46df5d52cac28a08ce2987"},
	{"empty with AD", "94c16f50d5e937916f82c7a4d5831938e6ea7c96f38f9fb3c89c8c3d521b54d1", "6e420b1bee5b9a39598efda9", "", "6843133a22647d1ce5f1c2d2", "0e986c42de2f858537f561e1bbc50dc6"},
	{"1 byte", "cc64d7a0f7587a20496aedb4788c661523aaf7d8ed3a676db235a5e079cc94c9", "54f64ccf7d4037992059964b", "9e", "", "b5d2b79585a2498c9c4511e121b0ac4c69"},
	{"1 byte with AD", "46c097a02c1355e24b52c71c41ec44a1c01e77db691f2c1d2179f014b18d384d", "52f4a1181ea059e0c8c0d641", "99", "c8f8bd1efec5cafad69330af", "1ed9aff5a7510f5a3320c0a48c1bbabf71"},
	{"15 bytes", "8a48d48b4e1cdd6676766e588a5027a8c2e9a5c54d1cf1bf1b8fedfe398752da", "ebf6a1a39fcbfb770a8d0316", "d09896a0d8be5515b72f9432bdc071", "a2622f0f77", "1c9a9c4e364dfccb36923c636722d0db0c4232960d890f36ffc180a490aebd"},
	{"16 bytes", "9f1faa7c5143abc3408d2903ed97c257cb5c5e239e0126d04e1d573f586a825c", "cb51465e447289a29278697a", "0a071d2e27e91cb4b80a6f8cd80bbcfe", "473d0b9e1193300bf347f7dbc44c3a5e", "118d361e476d92c5b79b295d4664bd8f1ef8a934bd25fb4f387c36bfb1ff667a"},
	{"17 bytes", "29e66dc0e716e93871c7e1bb2f08dad7f92902a4f145aa7cc8f2b513cd18b736", "761e215ad8af574b464587f4", "60bebc975e96d7ee8d8036eaaa074edf7b", "", "837a91428a91fcd32897ca25bbb6fed216a8989b8896339e53ab4e0e4de90d9905"},
	{"63 bytes", "aeb7a41688d92f0d184a2faf4777e0f9101d424047c63c59ad970445bd987b51", "b8cbc6ad4c4084b3e46c9dc5", "94d27d85cd0da58c1c790d45b21246bdd96ce12c9ff2c5df839900aa903f357ba9c46810e792acfbfa3859d834e13ab90bf94f8ce3984e007d86462123813f", "700950c7fc4b5358ffb0ce96c1", "714b9ba29e4df1e0435f50d9fb51b8089655aac76edd622fd36e271091fee0bf17f1d13d306568205c7a7787dcae352b44608d86d48ac73f07058bf15e0875fe96942c2e8a986fc64e08b7e412f9e0"},
	{"64 bytes", "353eab96df73b5ef5391e7b5910941593213239116cc5f52730e89753b4d768d", "0ec93eaeea6d3b8017c9e718", "80ff7e3f1b762d71aeca7e1182eaf7b9c5d6831158c6271cc0eb655188759c33634702ebfb501f358fa51f08f969e1fa8deaa43f8438384d8f40c3379fe50e1e", "", "5e116b90f8f6bc829b0e6fa7f6f29570940becd0a9ec128f499cecc28017addbbbda04288c1855644a552ed4953213dfcfac23d59e25748a28bd708c657a116a06aa98ab59046aebcbbce2793c3dd084"},
	{"64 bytes with AD", "15abdb1d197c1a65e84672532f5d789feb75b14a4f8051f1a47fab9c2257ae18", "df85e6304957b03b2ecf0729", "1e8a6569666f466149a8d2574735fec604689244612a928cc72e5dc50458bd6faad608e332bcf3851cabeb09c688e478569d794cb0c291449b0501e02e48f42f", "a3f1df0b565eaac62d30d557", "e0785437dcf61e04986e8a69be68508ff70a7ce0076d2da916c5af14ac630fde404b2655208e198ab0f61c5a69c32e5e2471d039834f56831f84f1c8104c54ea1866719ee3c33d7d0905ce0f7f17beca"},
	{"65 bytes", "c958eb29996e557ce65517238b8ece0a51f5e290379f30273067e440c613f116", "19f84a142e7c6c170e614a12", "99c61f75c4add4c44de0b46893a83c04619985ddc1aac44c9b56a125aecf2a1f3750d58cee90cb9f8f04a4f7b15e9696a7a307300e2d913d0df4105a0dacd3edbb", "1e", "c2f449ecd1a63f3fc2f7edb6610903ba7bcb101748df78856cb8a3c46e9f95ffb6a52d515e51a574eb0fec31eb4ab8512176a090451cc50f83df1510c0b5ed9ced000c845875cd351138f76f5c680eb26a"},
	{"129 bytes", "4dc574e7b2ea5e0fbc808dd85e6205a4a9943faf07dee8370c053a174f3c8d1a", "b649d3c3d220480ee589d63d", "d4a3e0c5ce7a3af30a85a7ec87b97ba1d2f0e00e8b9671ddc1d4f3559cd86b872dda7848aee5657aa8bcbc6c75b48dcd9d439ac95277443aabbd884e77945a556406b2f3bb5e462f4da5b0f82c86891c6699806d428884b7fa7bfdca44e4776aa3aefa7e3bf45b71eb5acbad93bc525b2cb0f4d2725f9b7474ecd20be906faf540", "62ad37e4e9756565fbbe7c33f6840757de5c32010c7cf386afff8477be70c387", "a2ecd8ebf2b7ef4557acaf7f0addf93ccef56680b1e93cf0e6ae50556b3d16cf2671440aa88c3a1de771ebac69c5901a38824dc181b05fa252e840340357d4dba74745057977e75bb2fdbd30ec315404e5c87dbd6455f796d2e203073d70f6a183756166012a54fa8ff9d01746eec3b944977864f783291386ea251d66bcd35abdf1b223eff0fa5294c27b087019c24e33"},
	{"1000 bytes", "cc14c341362ba001846ab5fe8399b1543ab0205dff5988b112f5a8a901910bf1", "b96e2de2a435be65700ac012", "e2dd99085a6fa4c00a601733054c96777aac79059b600fb47964b3005774c052a6ac5200fc3e59f64e84835cb6d14baf8155c18e1dd47e2b844a5eb2cf6246d3e94bf0cb29f1347de97362584334bc1deaf11a9afbc385762f92b59c48ff2cd78e936cd4fa684f55fc24f300399eea6b86cc696f934b61b9548be3878519ef6231a04d2ae360daf8ffda9fbfb5ac93be2016181ba5429c0e30026146ab8e5cf401e00c32a0f63093c15b02da178d0f22004d2e3eabf0fccbfca6c3e508c2b9cb7ab20226dd425da9544c3ba1be1eafb007a83f91db3169a83101720228fa9ab9d4ad9b151a9c1ffcb07fbba467cf7e8afc00aee245ec8e830490999c1977130e905bdba71c1e6b5514cdaeef4a317cc144e885c74b1f8e88b2a32ad58b7c70bbe11965447b965e865a78910d59614293c3a4ea13323d416e5e7deb926183b786d25362d9b644aef161b2579ac0f1138ffd8565d5444bdf9897352f4fd7c7102e53dd69f9f0afacff7551ad3b69c7312d8b142c7ce9d48cc65c209df9b6c0b862a36185d7579dbe0486b22bbb48a0eee6bd57da24c764b1f5ad984c2da191539c8f67e07507f49032b31ae36df6abd8b05ec1ecc00d427f8c7280c34545d55cf830ed70e10be9d1bc3c7b4f69686f6d7f6a3cc20ce405dab29380902b1690a5fd908de858e39b10921d21efbcfaec91c19c99d5f0233fc30bbc79fe28de1c11c08c9315946a91069ac1aba643228d97174fa0f45628859f7e6ae36832cc0e519aa1c2f3e595012f6e71f7ba866e8e3e889558857e65a15016999ab96fca4195a62f4cb0d8b5dd3e5a30c95710faf9262b3e8f4b257e73fc26b9afba9186c5b2d30e6349ee0e9559964a413d6c8873f410a257a3f66eb7ff6ecd384962e215ae530687ec34c92eaa2308b971ae0679310bb2f069716cb7872fab931ba16b78726d57eff0f5770ee64d399aeeb9a7ba8db63c06c1f02a0e600640680c0acf849e45e21506ec1bc4fcb860a6795d6a0cd888483f34b2d6e03ac3ff3defa9998d52285face96efe151cec8e03ed09b100ecfb25566e630eb07b276e04f573ac3339adc0ef6eefe9dbe19e84186b33382b4b81f53f34122ff68082d304f404f9e768d3038120cdaf3ce8fca92dbc0abc981fcdbe23d1d810f884741f5299934c4a20b65ab261b3427ea042d6e42de1dda81d8ac6c6cb38f4f430aea8dd5beed6336a707643feef897a660823eedcf73caa1beb33de17568a99390d432c9e027ea3adcc4bb3a9ae834e9a5b75d88c022b5c721e491007df477a3433a2070447dd78c16e4e9252e11543a4daebec3322c6485612345c44a800761d81b2b33d32f771122a533315b1eff05ea3b121cb781a13df209c1cc9701b50feb4df27ae632a424d9ffaa89927ce2c5112", "", "1583b66ce2454deb8568f71e7a992dd3755480f2b9f6c868ecccc4f2847aef563084d1225266b81ab627b95f37b6e7d9a749a0b22ab25087e408b41aa5bf48025ce16acf020366307c917f071604c6c3bdc24331955515ebcbd8d1b69a20a70366f64f41c46590cdb111ea3ce1a8dec924f798d27794633f85e10353ba8f0487d24316f26a24bdcb5e677c1c466127f8eec13f220477536bf2b7f6982d28f70693e66ee6fd330c503d1a08fca91875670f98fb81ea0d933f7e1c545d4e576217c915ddc27b84c604df183c587281ff6eb2d215a84ce1d80140f1e3466c2331484560bce3ac79132599a51a6e5c61d6408670e85d7a5e43c95a607803cb87f4112ab3782039e6dcbf94677450a9571d0fed1d806138cb3b825e1ff637f25163e9900f444b692fa98cccb201fed8544131e514ae537fc45c0a9dbebc15f4be8fc64cd3674d4269cefd220bfb7e667e2df4d4298838ce5fa2c76009c425dce89c75a46fc5f1d5d89a78051783ff6f6474a1221ef94a6ee9fef6b00dd73cffc4d0fdd7faaaa9d10290095df187201f8c61b38b2506cb1340940c9576c77926fcbc20ce1e4747fbd5e4be04c2bbe10e8dca99a4f6a0d94f2c4b2645986e248d6c8e27494d24cf0b52d4002e3b43088b88ed6adafc4280f7abc63fb465e384d4a0571c29238e7658442e37becb23d3538b7c2ec91c1ce7c0a3d16eef066425245a708f53f54d869f726ca9b2fdce836e25d4cb5044a2d59f4a3efdea4bcb3a03fe7a18b526ae41c4a7c1a5672d5f6b87970192e1446c5e9843593344008d499e4af526f2215a81e813842bb7dbfa1d8996712adbc493c600c718e446f5b897069f342d1024a284a1392e40659c86947f8a138e7194b1a6d5ebdc02c8d30947fe594fc4263172ab1325972569b8d71cbb1968a352fc939b78a2b9879d13a6c6cb920fac4614a4dbf659f41d0b53be604694b84ff7ad2d425080c67ba9aa44aafcfff0f642495bbfc1c72628fe744649f6661c2c8f8b4a427e734ad20fef43f52250fce00cafe314d3035a05c48a6c5614a507b8b998ee884b515c198ea1deea9d32fbfb2014ff10832ca6b700922cf4097e0ec5f09024f9d20c2b9fedaf994d700cc6ae2753f56016cd0a031a4561b60d3c5c6d585fc783de68918484da0b5f5da3cc604e6fa21579cfbe2f1c8fb3553bb8cfb0591495d7d212f2b5b7717c84410a959ea6937efe2cc45d0ce4b36d63dd57e91c477fdaf6a41a92307389ddd087390802c2c9da0dcb489bf56f7577831d684d0fddfde73102a8de09b5caf09ddfff8c18f67d838d63c2c2b2097ffc8ce0bece0a528e7efc3f4ff6242674b9954ff0b4579437e5c9932703e76ba476d3da8da8450f32e9dd191b6890ed3128789e3de07e09db03216c995dc4b64744b0e657457d0f8658e3fc55d7c7"},
	{"1000 bytes with AD", "fb67769c1c8176514f644e352e707564b37682b7bc2899bed40284ff5b45a46b", "f22b1e4ad55b3f37cb16f0d2", "fd6813e72247b6eeac32a79337b5e319fc69630498e9ad80bf80d5233f08a3b875fbfbd1d8090c181dba4e7d2f494fd6c9eb93ef1b00e3f429e50877c4acd749a4e72678c7c828f324f9ea4db98e99a80fcd2104d63aa384e181ba9ac72ac6b54fda5d71add01f5adef32a33eb476238dbae340f890a5a1cf91d448fb67b5e7438ad7a8bbe3b8b50409de83b8d9693ece5d5472c5076743d9871322fe37b517ece9f53cc9fd8a6f4e94114c300f19b566905cacdb6d942f0229566e146436795aacc9906e6c2f7032d710462a9205b8a73ffc767e0e72a8d81dd833d7e1d35a5416e646acba47160a037c16af8f91c1312ec1875c5d5d34501b593f041cbc5fb52ed19b772d4b2351deed2fe5a452e88adf52b36339c4c318e4ee9f98334a581648939ca6131e8d066968d6097c08f682d0c71d3e03564e5d3f436d6c0b2b6698a2dc88dce41ce0bbc2cb8db70cdb848613d982685cfd2051f2b52ec2d6f944864c921094ff88b8472ee0fa188caf038238da565bf19a39eb983fd51d1ef88b6e072694a06f92bcee11c101a01f08e390808454ebf7ee06c5bad1da9f0f046aeffb8b12bebbd7a518a7e14031080a04fe4afaf6d9ed39b1a9c691de45f951dcc9b82676987e101fd491619e74e94f15ed905ad9c2965647213a43d6d52e6107343ea6089db09ad6de9eef6c88da149e3fe5891df11a76a4d23554d179854360a635d63fa7ab3fe4224c568fb3e4a22a836e64bea663b0bcb28ad1af641d5f19365ec73e5e1bce9a11b991218f55892f327c33456b5d46deec45e51c93a17166c94e1db610d057d57d29620c5881c3ae009ef4ae98b54a8b85e1d90228780ada28643f055c05e72d9332d95389485727a31ab032c40836eca32d67861ae4f948835f67c3eebbe507b5aa8be6ffa1f93e0de97d235f7e22338dc4ad2a9481ebd645d7c304f2e466cc48852769033a22b43a918c47dbfbe101e9fbfcbb57a0781aabc4c7c180d6b145d5d508263e9631adcfb0749a9feb3d1f5c19195a792c45fb0c70c5b1af5724ef5a12efec8030bf38bd8fb5addaf801b81d1b339aad1b569213543e62565c323f538ab718809aef99c8c1735a10474d9a970e93d831ca0b5fa1c6af21b7da855f006349965af38b040ca2e5b44d64fc61718a7b989b3eb46af5e1705083717aa50d1ba37ca508b72760a564225600052b8370211b5db210efae4821d77a7b7d858abf4062d2be13843fcaa428deb0feb756a1d25fa3b14029a5e38b8d3bead5944e77f0f0a72b6381929cfec2363296afac94bcfc284a4a8c181742d3ac27d3a70b6b47b5d61a2106d61821079803a2ebe6de8905e35aad190c87c51e1d4ff4b088bcab5908d669e693f8576ae318c108258e2ac8dcbd33ab7977186eafc5c5bf4", "054c85ada458e98799c31cf6", "bc4abf596f413da5a9f9b79622b348aaca37188e22efd78206b5fbf7cb2a1ac7df9b624a2f0ad1c951eefff574754495519966b2ac60633d9615fa18c0c805771e821e1acd91620602c5127d6b8343b657df1dd9e2cd413aeadfb288c626223383075b95247fc22f3460e46c1e5cbdbb51b112c7475652f5c00711794ae1a855ecfdc051eb9f251d56ccb392cd1e77e538b945728d0534a1d6695422a0c301065b59751f65c0e350589a134860fc43832149271c0fae488886b8fd70fa33f405932fe9a96fd4b3ec6bdd47d8f1fc0b6ee67ae3b5a4892212a1abd169c543b0a58ebbb4fb0b76c7a9978c23bef0d436c61c364edd8938bc16c55d08839862c8b8dfefac0a59be5bc20ccd42d9ca3b864e5e188a52d2aee423593fdf6b9c49e5267078e7997945a12806467822438982f6be5b5b5c86cd41654e2fc31f99be60e1c4cec9737f496d2b7bfc0f4abc53183e4022a4f54c226a01448b3a899f4e9f92c79c62e5ff158de5260f40ce17c557b85d798bf438ad39adf582209b5f3f2eb0b829541b54797a008fc2c0a44e35067791eab8103a9ffe9ac9fc0301a0f45c9ffe003ae8ee66991108d5e52cb2d519a9216176bc20ad6a38ac58f842e961eec4174091330115a415a0300e886dac931863aa3d4174479f1c97adca8a2e88f0067e9206773990bbe29b189c40a9a85aa0d72a63e5767fb7b5904f20de200ee8b66336bb9972d5d62ffe4b78566b6ec45505ad92d6e3a98511138df8fb10013e120753127c2bb1ff9d254b5fd4ccd49328d0c383cbf4b46b34c86020e5c41183cad014a67e877b305888aeb6270ecae66d891be8878fdac9dd599dcb8fed7f035b8cd31726977a02c9308d4c625cd62211faf14c0f8baffa47b8d8c10ec081c17f28e2819cd95c684aa8c4ed1100af894b81df6652237460ea6e569b7799e18fcb509172b5a6f6a57768467fd3b114d6c96aa931cfa62fd71bea20869150a80643a198858519051bdd7d6b72a6a590c98c2da9e008ca27a5c6acb1b6e8fa8d5f9a04c30b8844ca0b46d25dfb85c92dc650847bc86ecd5de5bdf175356a4dd6c547c3cef9be93a0d37ce3a43e86e7655dd71d606650b51a75aca14ef0379084a46547381ce372642b91a41dbaf5df9e2bcc784274d7e7c3a99091d2592c0bac18da5d9c7fd0de589b094eee2891366d963c36cc58883516d0440db21870442e8ec6e96febc1062c25c13137ea24d5663d8b2864ec2c7cd61a08a3db78f1b214fd356d549c532f784b3ec1929d6f952bcd20abbcf69c50692cc6eed6aaaf8a7f4b87c5058ddade5aa666358bbbdbfe58a78f34d735200081de321356b525450c24cdfd4b26a43657e953a3a4ee830560464fd84686263f38403733f51f87f9c0567ab7a4c839251a37e8b7ec2ea1f5871fcf77dcafb875342c1b"},
	{"RFC 8439 2.8.2", "808182838485868788898a8b8c8d8e8f909192939495969798999a9b9c9d9e9f", "070000004041424344454647", "4c616469657320616e642047656e746c656d656e206f662074686520636c617373206f66202739393a204966204920636f756c64206f6666657220796f75206f6e6c79206f6e652074697020666f7220746865206675747572652c2073756e73637265656e20776f756c642062652069742e", "50515253c0c1c2c3c4c5c6c7", "d31a8d34648e60db7b86afbc53ef7ec2a4aded51296e08fea9e2b5a736ee62d63dbea45e8ca9671282fafb69da92728b1a71de0a9e060b2905d6a5b67ecd3b3692ddbd7f2d778b8c9803aee328091b58fab324e4fad675945585808b4831d7bc3ff4def08e4b7a9de576d26586cec64b61161ae10b594f09e26a7e902ecbd0600691"},
}

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type vector struct {
	key, nonce, plain, ad, sealed []byte
}

func decode(t *testing.T, i int) vector {
	v := vectors[i]
	return vector{unhex(t, v.key), unhex(t, v.nonce), unhex(t, v.plain), unhex(t, v.ad), unhex(t, v.sealed)}
}

// TestVectors seals and opens each vector into a new slice, after a prefix
// in dst, and in place.
func TestVectors(t *testing.T) {
	for i, tt := range vectors {
		t.Run(tt.name, func(t *testing.T) {
			v := decode(t, i)
			a, err := New(v.key)
			if err != nil {
				t.Fatal(err)
			}
			if got := a.Seal(nil, v.nonce, v.plain, v.ad); !bytes.Equal(got, v.sealed) {
				t.Fatalf("Seal = %x, want %x", got, v.sealed)
			}
			prefix := []byte{1, 2, 3}
			if got := a.Seal(prefix, v.nonce, v.plain, v.ad); !bytes.Equal(got[:3], prefix) || !bytes.Equal(got[3:], v.sealed) {
				t.Fatalf("Seal after prefix = %x", got)
			}
			got, err := a.Open(nil, v.nonce, v.sealed, v.ad)
			if err != nil || !bytes.Equal(got, v.plain) {
				t.Fatalf("Open = %x, %v, want %x", got, err, v.plain)
			}
			buf := make([]byte, len(v.plain), len(v.plain)+Overhead)
			copy(buf, v.plain)
			sealed := a.Seal(buf[:0], v.nonce, buf, v.ad)
			if !bytes.Equal(sealed, v.sealed) || &sealed[0] != &buf[:1][0] {
				t.Fatalf("Seal in place = %x", sealed)
			}
			opened, err := a.Open(sealed[:0], v.nonce, sealed, v.ad)
			if err != nil || !bytes.Equal(opened, v.plain) {
				t.Fatalf("Open in place = %x, %v", opened, err)
			}
		})
	}
}

// TestOpenFailures changes one input at a time. Open must fail with ErrAuth
// and leave dst unchanged.
func TestOpenFailures(t *testing.T) {
	v := decode(t, len(vectors)-1)
	other := append([]byte(nil), v.key...)
	other[0] ^= 1
	flip := func(b []byte, i int) []byte {
		c := append([]byte(nil), b...)
		c[i] ^= 0x80
		return c
	}
	tests := []struct {
		name                   string
		key, nonce, sealed, ad []byte
	}{
		{"wrong key", other, v.nonce, v.sealed, v.ad},
		{"wrong nonce", v.key, flip(v.nonce, 11), v.sealed, v.ad},
		{"tampered ciphertext first byte", v.key, v.nonce, flip(v.sealed, 0), v.ad},
		{"tampered ciphertext last byte", v.key, v.nonce, flip(v.sealed, len(v.plain)-1), v.ad},
		{"tampered tag", v.key, v.nonce, flip(v.sealed, len(v.sealed)-1), v.ad},
		{"tampered AD", v.key, v.nonce, v.sealed, flip(v.ad, 0)},
		{"missing AD", v.key, v.nonce, v.sealed, nil},
		{"extra AD", v.key, v.nonce, v.sealed, append(append([]byte(nil), v.ad...), 0)},
		{"truncated", v.key, v.nonce, v.sealed[:len(v.sealed)-1], v.ad},
		{"shorter than tag", v.key, v.nonce, v.sealed[:Overhead-1], v.ad},
		{"empty", v.key, v.nonce, nil, v.ad},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := New(tt.key)
			if err != nil {
				t.Fatal(err)
			}
			dst := make([]byte, 0, len(tt.sealed))
			got, err := a.Open(dst, tt.nonce, tt.sealed, tt.ad)
			if err != ErrAuth || got != nil {
				t.Fatalf("Open = %x, %v, want ErrAuth", got, err)
			}
			for _, b := range dst[:cap(dst)] {
				if b != 0 {
					t.Fatal("Open wrote to dst")
				}
			}
			in := append([]byte(nil), tt.sealed...)
			if _, err := a.Open(in[:0], tt.nonce, in, tt.ad); err != ErrAuth || !bytes.Equal(in, tt.sealed) {
				t.Fatalf("Open in place = %v, or changed the ciphertext", err)
			}
		})
	}
}

// TestSizes checks the key size, the nonce size and the constants.
func TestSizes(t *testing.T) {
	for _, n := range []int{0, 16, 31, 33, 64} {
		if a, err := New(make([]byte, n)); err != ErrKeySize || a != nil {
			t.Errorf("New with %d-byte key = %v, %v", n, a, err)
		}
	}
	a, err := New(make([]byte, KeySize))
	if err != nil {
		t.Fatal(err)
	}
	if a.NonceSize() != 12 || a.Overhead() != 16 || KeySize != 32 {
		t.Errorf("sizes %d %d %d", a.NonceSize(), a.Overhead(), KeySize)
	}
	sealed := a.Seal(nil, make([]byte, NonceSize), []byte("x"), nil)
	for _, n := range []int{0, 8, 11, 13, 24} {
		if _, err := a.Open(nil, make([]byte, n), sealed, nil); err != ErrNonceSize {
			t.Errorf("Open with %d-byte nonce = %v", n, err)
		}
		func() {
			defer func() {
				if r := recover(); r != ErrNonceSize {
					t.Errorf("Seal with %d-byte nonce panicked with %v", n, r)
				}
			}()
			a.Seal(nil, make([]byte, n), []byte("x"), nil)
		}()
	}
}

// TestOverlap checks that Seal panics and Open fails when dst overlaps the
// input but does not start at the same byte.
func TestOverlap(t *testing.T) {
	a, _ := New(make([]byte, KeySize))
	nonce := make([]byte, NonceSize)
	buf := make([]byte, 100)
	func() {
		defer func() {
			if r := recover(); r != ErrOverlap {
				t.Errorf("Seal panicked with %v", r)
			}
		}()
		a.Seal(buf[1:1], nonce, buf[:50], nil)
	}()
	sealed := a.Seal(buf[:0], nonce, buf[:50], nil)
	if _, err := a.Open(sealed[1:1], nonce, sealed, nil); err != ErrOverlap {
		t.Errorf("Open = %v", err)
	}
}

// TestAllocs requires Seal and Open with capacity in dst, in place or not,
// to allocate nothing.
func TestAllocs(t *testing.T) {
	v := decode(t, len(vectors)-2) // 1000 bytes with AD
	a, _ := New(v.key)
	buf := make([]byte, len(v.plain)+Overhead)
	out := make([]byte, 0, len(v.plain)+Overhead)
	tests := []struct {
		name string
		f    func()
	}{
		{"Seal", func() { a.Seal(out[:0], v.nonce, v.plain, v.ad) }},
		{"Open", func() {
			if _, err := a.Open(out[:0], v.nonce, v.sealed, v.ad); err != nil {
				t.Fatal(err)
			}
		}},
		{"Seal and Open in place", func() {
			copy(buf, v.plain)
			s := a.Seal(buf[:0], v.nonce, buf[:len(v.plain)], v.ad)
			if _, err := a.Open(s[:0], v.nonce, s, v.ad); err != nil {
				t.Fatal(err)
			}
		}},
		{"Open failure", func() { a.Open(out[:0], v.nonce, v.sealed[1:], v.ad) }},
	}
	for _, tt := range tests {
		if n := testing.AllocsPerRun(20, tt.f); n != 0 {
			t.Errorf("%s: %v allocations, want 0", tt.name, n)
		}
	}
	// The count works: without capacity, Seal allocates the result.
	if n := testing.AllocsPerRun(20, func() { a.Seal(nil, v.nonce, v.plain, v.ad) }); n < 1 {
		t.Errorf("Seal with nil dst: %v allocations, want 1", n)
	}
}
