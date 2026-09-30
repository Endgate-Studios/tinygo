package nn

// AddParams are the quantization values of AddInt8 and AddUint8, as in the
// TensorFlow Lite ArithmeticParams.
type AddParams struct {
	In1Offset, In2Offset int32
	In1Mult, In2Mult     int32
	In1Shift, In2Shift   int32
	LeftShift            int32
	OutOffset            int32
	OutMult, OutShift    int32
	Activation
}

// AddInt8 adds two quantized tensors, as esp-nn esp_nn_add_elementwise_s8.
func AddInt8(out, in1, in2 []int8, p *AddParams) {
	checkLen(len(out), len(in1), len(in2))
	addInt8(out, in1[:len(out)], in2[:len(out)], p)
}

func addInt8Go(out, in1, in2 []int8, p *AddParams) {
	for i := range out {
		out[i] = int8(p.add(int32(in1[i]), int32(in2[i])))
	}
}

// AddUint8 adds two quantized tensors, as esp-nn esp_nn_add_elementwise_u8.
func AddUint8(out, in1, in2 []uint8, p *AddParams) {
	checkLen(len(out), len(in1), len(in2))
	for i := range out {
		out[i] = uint8(p.add(int32(in1[i]), int32(in2[i])))
	}
}

func (p *AddParams) add(a, b int32) int32 {
	a = (a + p.In1Offset) << uint(p.LeftShift)
	b = (b + p.In2Offset) << uint(p.LeftShift)
	a = DivByPowerOfTwo(SatRoundDoublingHighMul(a, p.In1Mult), -p.In1Shift)
	b = DivByPowerOfTwo(SatRoundDoublingHighMul(b, p.In2Mult), -p.In2Shift)
	v := DivByPowerOfTwo(SatRoundDoublingHighMul(a+b, p.OutMult), -p.OutShift) + p.OutOffset
	return max(p.Min, min(v, p.Max))
}

// MulParams are the quantization values of MulInt8 and
// MulBroadcastChannelInt8.
type MulParams struct {
	In1Offset, In2Offset int32
	OutOffset            int32
	OutMult, OutShift    int32
	Activation
}

// MulInt8 multiplies two quantized tensors, as esp-nn
// esp_nn_mul_elementwise_s8.
func MulInt8(out, in1, in2 []int8, p *MulParams) {
	checkLen(len(out), len(in1), len(in2))
	mulInt8(out, in1[:len(out)], in2[:len(out)], p)
}

func mulInt8Go(out, in1, in2 []int8, p *MulParams) {
	for i := range out {
		v := (int32(in1[i]) + p.In1Offset) * (int32(in2[i]) + p.In2Offset)
		v = MultiplyByQuantizedMult(v, p.OutMult, p.OutShift) + p.OutOffset
		out[i] = int8(max(p.Min, min(v, p.Max)))
	}
}

// MulBroadcastChannelInt8 multiplies each pixel of the NHWC tensor in by the
// channel vector perChannel, as esp-nn esp_nn_mul_broadcast_channel_s8.
func MulBroadcastChannelInt8(out, in, perChannel []int8, p *MulParams) {
	c := len(perChannel)
	if c == 0 || len(out)%c != 0 {
		panic("nn: bad channel count")
	}
	checkLen(len(out), len(in))
	mulBroadcastChannelInt8(out, in[:len(out)], perChannel, p)
}

func mulBroadcastChannelInt8Go(out, in, perChannel []int8, p *MulParams) {
	c := len(perChannel)
	for i := range out {
		v := (int32(in[i]) + p.In1Offset) * (int32(perChannel[i%c]) + p.In2Offset)
		v = MultiplyByQuantizedMult(v, p.OutMult, p.OutShift) + p.OutOffset
		out[i] = int8(p.clamp(v))
	}
}
