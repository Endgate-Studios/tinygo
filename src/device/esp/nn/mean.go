package nn

// MeanScratchSize returns the scratch size in bytes for MeanInt8.
func MeanScratchSize(height, width, channels int) int {
	return meanScratchSize(height, width, channels)
}

// MeanInt8 sets out[c] to the mean of channel c over the height and width of
// the NHWC tensor in, as esp-nn esp_nn_mean_nhwc_s8.
func MeanInt8(out, in []int8, height, width int, inZeroPoint, outZeroPoint, mult, shift int32, scratch []byte) {
	checkLen(height*width*len(out), len(in))
	meanInt8(out, in[:height*width*len(out)], inZeroPoint, outZeroPoint, mult, shift, scratch)
}

func meanInt8Go(out, in []int8, inZeroPoint, outZeroPoint, mult, shift int32) {
	ch := len(out)
	n := int32(len(in) / ch)
	for c := range out {
		var sum int32
		for i := c; i < len(in); i += ch {
			sum += int32(in[i])
		}
		v := MultiplyByQuantizedMult(sum-n*inZeroPoint, mult, shift) + outZeroPoint
		out[c] = sat8(v)
	}
}
