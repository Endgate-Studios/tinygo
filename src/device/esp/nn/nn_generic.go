//go:build !esp32s3

package nn

func enableDualCore() {}

func relu6Int8(data []int8) { relu6Int8Go(data) }

func hardSwishScratchSize() int { return 0 }

func hardSwishInt8(out, in []int8, p *HardSwishParams, scratch []byte) {
	hardSwishInt8Go(out, in, p)
}

func addInt8(out, in1, in2 []int8, p *AddParams) { addInt8Go(out, in1, in2, p) }

func mulInt8(out, in1, in2 []int8, p *MulParams) { mulInt8Go(out, in1, in2, p) }

func mulBroadcastChannelInt8(out, in, perChannel []int8, p *MulParams) {
	mulBroadcastChannelInt8Go(out, in, perChannel, p)
}

func convScratchSize(inDims, filterDims, outDims *Dims, p *ConvParams) int { return 0 }

func convInt8(inDims *Dims, in []int8, filterDims *Dims, filter []int8, bias []int32,
	outDims *Dims, out []int8, p *ConvParams, q Quant, scratch []byte) {
	convInt8Go(inDims, in, filterDims, filter, bias, outDims, out, p, q)
}

func depthwiseConvScratchSize(inDims, filterDims, outDims *Dims, p *DepthwiseConvParams) int {
	return 0
}

func depthwiseConvInt8(inDims *Dims, in []int8, filterDims *Dims, filter []int8, bias []int32,
	outDims *Dims, out []int8, p *DepthwiseConvParams, q Quant, scratch []byte) {
	depthwiseConvInt8Go(inDims, in, filterDims, filter, bias, outDims, out, p, q)
}

func fullyConnectedInt8(out, in, filter []int8, bias []int32, p *FullyConnectedParams, shift, mult int32) {
	fullyConnectedInt8Go(out, in, filter, bias, p, shift, mult)
}

func fullyConnectedPerChannelInt8(out, in, filter []int8, bias []int32, p *FullyConnectedParams, q Quant) {
	fullyConnectedPerChannelInt8Go(out, in, filter, bias, p, q)
}

func maxPoolInt8(outDims *Dims, out []int8, inDims *Dims, in []int8, p *PoolParams) {
	maxPoolInt8Go(outDims, out, inDims, in, p)
}

func avgPoolInt8(outDims *Dims, out []int8, inDims *Dims, in []int8, p *PoolParams) {
	avgPoolInt8Go(outDims, out, inDims, in, p)
}

func softmaxScratchSize(width, height int) int { return 0 }

func softmaxInt8(out, in []int8, width int, mult, shift, diffMin int32, scratch []byte) {
	softmaxInt8Go(out, in, width, mult, shift, diffMin)
}

func meanScratchSize(height, width, channels int) int { return 0 }

func meanInt8(out, in []int8, inZeroPoint, outZeroPoint, mult, shift int32, scratch []byte) {
	meanInt8Go(out, in, inZeroPoint, outZeroPoint, mult, shift)
}
