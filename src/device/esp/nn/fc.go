package nn

// FullyConnectedParams are the offsets and the output range of the fully
// connected functions.
type FullyConnectedParams struct {
	InOffset     int32
	FilterOffset int32
	OutOffset    int32
	Activation
}

// FullyConnectedInt8 sets out[o] to the dot product of in and row o of
// filter, with one output multiplier, as esp-nn esp_nn_fully_connected_s8.
// The row length is len(in). The bias can be nil.
func FullyConnectedInt8(out, in, filter []int8, bias []int32, p *FullyConnectedParams, shift, mult int32) {
	checkFullyConnected(out, in, filter, bias)
	fullyConnectedInt8(out, in, filter, bias, p, shift, mult)
}

func fullyConnectedInt8Go(out, in, filter []int8, bias []int32, p *FullyConnectedParams, shift, mult int32) {
	for o := range out {
		acc := dotOffset(in, filter[o*len(in):], p)
		if bias != nil {
			acc += bias[o]
		}
		acc = MultiplyByQuantizedMult(acc, mult, shift) + p.OutOffset
		out[o] = int8(p.clamp(acc))
	}
}

// FullyConnectedPerChannelInt8 is FullyConnectedInt8 with one output
// multiplier for each output, as esp-nn esp_nn_fully_connected_per_ch_s8.
func FullyConnectedPerChannelInt8(out, in, filter []int8, bias []int32, p *FullyConnectedParams, q Quant) {
	checkFullyConnected(out, in, filter, bias)
	checkLen(len(out), len(q.Shift), len(q.Mult))
	fullyConnectedPerChannelInt8(out, in, filter, bias, p, q)
}

func fullyConnectedPerChannelInt8Go(out, in, filter []int8, bias []int32, p *FullyConnectedParams, q Quant) {
	for o := range out {
		acc := dotOffset(in, filter[o*len(in):], p)
		if bias != nil {
			acc += bias[o]
		}
		acc = MultiplyByQuantizedMult(acc, q.Mult[o], q.Shift[o]) + p.OutOffset
		out[o] = int8(p.clamp(acc))
	}
}

// FullyConnectedPerChannelBatchInt8 runs FullyConnectedPerChannelInt8 on
// batches rows of in and out, as esp-nn esp_nn_fully_connected_per_ch_s8_batch.
func FullyConnectedPerChannelBatchInt8(out, in, filter []int8, bias []int32, p *FullyConnectedParams, q Quant, batches int) {
	if batches <= 0 || len(in)%batches != 0 || len(out)%batches != 0 {
		panic("nn: bad batch count")
	}
	rowLen, outCh := len(in)/batches, len(out)/batches
	for b := 0; b < batches; b++ {
		FullyConnectedPerChannelInt8(out[b*outCh:(b+1)*outCh], in[b*rowLen:(b+1)*rowLen], filter, bias, p, q)
	}
}

func checkFullyConnected(out, in, filter []int8, bias []int32) {
	checkLen(len(out)*len(in), len(filter))
	if bias != nil {
		checkLen(len(out), len(bias))
	}
}

func dotOffset(in, row []int8, p *FullyConnectedParams) int32 {
	row = row[:len(in)]
	var acc int32
	for i, v := range in {
		acc += (int32(row[i]) + p.FilterOffset) * (int32(v) + p.InOffset)
	}
	return acc
}
