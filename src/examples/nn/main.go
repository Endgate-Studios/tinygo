// This example runs a small int8 convolutional network with the esp-nn
// functions of package device/esp/nn and prints the time of each layer. The
// weights are pseudo random, so the output class has no meaning.
//
// On the ESP32-S3 the layers use the PIE SIMD kernels. Build with
// -scheduler=cores to also use the second core.
package main

import (
	"device/esp/nn"
	"time"
)

var seed uint32 = 1

func random(s []int8) []int8 {
	for i := range s {
		seed = seed*1664525 + 1013904223
		s[i] = int8(seed >> 24)
	}
	return s
}

// quant returns a per channel multiplier of 0.5 * 2^shift.
func quant(n int, shift int32) nn.Quant {
	q := nn.Quant{Shift: make([]int32, n), Mult: make([]int32, n)}
	for i := range q.Mult {
		q.Shift[i], q.Mult[i] = shift, 1<<30
	}
	return q
}

func main() {
	time.Sleep(time.Second)
	nn.EnableDualCore()
	all := nn.Activation{Min: -128, Max: 127}

	image := random(nn.AlignedInt8(32 * 32 * 3))

	// Layer 1: 3x3 convolution, stride 2, 3 to 16 channels.
	d1 := nn.Dims{Width: 16, Height: 16, Channels: 16}
	f1 := random(nn.AlignedInt8(16 * 3 * 3 * 3))
	p1 := nn.ConvParams{InOffset: 128, OutOffset: -128, Stride: nn.Size2D{Width: 2, Height: 2},
		Padding: nn.Size2D{Width: 1, Height: 1}, Activation: all}
	in1 := nn.Dims{Width: 32, Height: 32, Channels: 3}
	k1 := nn.Dims{Width: 3, Height: 3}
	out1 := nn.AlignedInt8(16 * 16 * 16)
	s1 := make([]byte, nn.ConvScratchSize(in1, k1, d1, &p1))

	// Layer 2: 3x3 depthwise convolution.
	f2 := random(nn.AlignedInt8(3 * 3 * 16))
	p2 := nn.DepthwiseConvParams{InOffset: 128, OutOffset: -128, ChMult: 1, Stride: nn.Size2D{Width: 1, Height: 1},
		Padding: nn.Size2D{Width: 1, Height: 1}, Activation: all}
	out2 := nn.AlignedInt8(16 * 16 * 16)
	s2 := make([]byte, nn.DepthwiseConvScratchSize(d1, k1, d1, &p2))

	// Layer 3: 1x1 convolution, 16 to 32 channels.
	d3 := nn.Dims{Width: 16, Height: 16, Channels: 32}
	k3 := nn.Dims{Width: 1, Height: 1}
	f3 := random(nn.AlignedInt8(32 * 16))
	p3 := nn.ConvParams{InOffset: 128, OutOffset: -128, Stride: nn.Size2D{Width: 1, Height: 1}, Activation: all}
	out3 := nn.AlignedInt8(16 * 16 * 32)
	s3 := make([]byte, nn.ConvScratchSize(d1, k3, d3, &p3))

	// Layer 4: 2x2 average pool, then the mean of each channel.
	d4 := nn.Dims{Width: 8, Height: 8}
	pp := nn.PoolParams{Filter: nn.Size2D{Width: 2, Height: 2}, Stride: nn.Size2D{Width: 2, Height: 2}, Activation: all}
	out4 := nn.AlignedInt8(8 * 8 * 32)
	mean := nn.AlignedInt8(32)
	s4 := make([]byte, nn.MeanScratchSize(8, 8, 32))

	// Layer 5: fully connected, 32 to 10 classes, and softmax.
	f5 := random(nn.AlignedInt8(10 * 32))
	p5 := nn.FullyConnectedParams{InOffset: 128, OutOffset: 0, Activation: all}
	logits := nn.AlignedInt8(10)
	probs := nn.AlignedInt8(10)
	s5 := make([]byte, nn.SoftmaxScratchSize(10, 1))

	for {
		t := time.Now()
		lap := func(name string) {
			now := time.Now()
			println(" ", name, now.Sub(t).Microseconds(), "us")
			t = now
		}
		nn.ConvInt8(in1, image, k1, f1, nil, d1, out1, &p1, quant(16, -6), s1)
		lap("conv 3x3")
		nn.DepthwiseConvInt8(d1, out1, k1, f2, nil, d1, out2, &p2, quant(16, -4), s2)
		lap("depthwise 3x3")
		nn.ConvInt8(d1, out2, k3, f3, nil, d3, out3, &p3, quant(32, -5), s3)
		lap("conv 1x1")
		nn.AvgPoolInt8(d4, out4, d3, out3, &pp)
		nn.MeanInt8(mean, out4, 8, 8, -128, -128, 1<<30, 1, s4)
		lap("pool and mean")
		nn.FullyConnectedInt8(logits, mean, f5, nil, &p5, -6, 1<<30)
		nn.SoftmaxInt8(probs, logits, 10, 1<<30, 0, -64, s5)
		lap("fully connected and softmax")
		best := 0
		for i, v := range probs {
			if v > probs[best] {
				best = i
			}
		}
		println("class", best)
		time.Sleep(2 * time.Second)
	}
}
