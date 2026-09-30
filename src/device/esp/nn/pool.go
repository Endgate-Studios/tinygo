package nn

// PoolParams are the values of MaxPoolInt8 and AvgPoolInt8.
type PoolParams struct {
	Filter  Size2D
	Stride  Size2D
	Padding Size2D
	Activation
}

// MaxPoolInt8 sets each output value to the largest input value in the
// filter window, as esp-nn esp_nn_max_pool_s8. The tensors are NHWC and
// inDims.Channels is the channel count of both.
func MaxPoolInt8(outDims Dims, out []int8, inDims Dims, in []int8, p *PoolParams) {
	checkPool(&outDims, out, &inDims, in)
	maxPoolInt8(&outDims, out, &inDims, in, p)
}

// AvgPoolInt8 sets each output value to the rounded mean of the input values
// in the filter window, as esp-nn esp_nn_avg_pool_s8.
func AvgPoolInt8(outDims Dims, out []int8, inDims Dims, in []int8, p *PoolParams) {
	checkPool(&outDims, out, &inDims, in)
	avgPoolInt8(&outDims, out, &inDims, in, p)
}

func checkPool(outDims *Dims, out []int8, inDims *Dims, in []int8) {
	checkLen(inDims.Width*inDims.Height*inDims.Channels, len(in))
	checkLen(outDims.Width*outDims.Height*inDims.Channels, len(out))
}

func maxPoolInt8Go(outDims *Dims, out []int8, inDims *Dims, in []int8, p *PoolParams) {
	poolGo(outDims, out, inDims, in, p, false)
}

func avgPoolInt8Go(outDims *Dims, out []int8, inDims *Dims, in []int8, p *PoolParams) {
	poolGo(outDims, out, inDims, in, p, true)
}

func poolGo(outDims *Dims, out []int8, inDims *Dims, in []int8, p *PoolParams, avg bool) {
	inW, inH, ch := inDims.Width, inDims.Height, inDims.Channels
	i := 0
	for outY := 0; outY < outDims.Height; outY++ {
		baseY := outY*p.Stride.Height - p.Padding.Height
		for outX := 0; outX < outDims.Width; outX++ {
			baseX := outX*p.Stride.Width - p.Padding.Width
			yStart, yEnd := max(0, -baseY), min(p.Filter.Height, inH-baseY)
			xStart, xEnd := max(0, -baseX), min(p.Filter.Width, inW-baseX)
			for c := 0; c < ch; c++ {
				var acc, count int32
				if !avg {
					acc = -128
				}
				for fy := yStart; fy < yEnd; fy++ {
					for fx := xStart; fx < xEnd; fx++ {
						v := int32(in[((baseY+fy)*inW+baseX+fx)*ch+c])
						if avg {
							acc += v
							count++
						} else if v > acc {
							acc = v
						}
					}
				}
				if avg {
					if acc > 0 {
						acc = (acc + count/2) / count
					} else {
						acc = (acc - count/2) / count
					}
				}
				out[i] = int8(p.clamp(acc))
				i++
			}
		}
	}
}
