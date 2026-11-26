// Package nn has the int8 neural network functions of esp-nn
// (https://github.com/espressif/esp-nn) for TinyGo. The functions use the
// TensorFlow Lite int8 quantization and give the same results as the esp-nn
// ansi C code. On the ESP32-S3 the functions use the PIE SIMD kernels from
// src/device/esp/esp32s3_nn.S. On other targets all functions are plain Go.
//
// Some functions take a scratch buffer. Get its size from the ScratchSize
// function of the operation. If the buffer is too small, the function uses
// the plain Go code. The convolutions also use the plain Go code if the
// input, filter or output slice is not 16 byte aligned. AlignedInt8 makes
// such slices.
//
// esp-nn is Copyright 2020-2026 Espressif Systems (Shanghai) CO LTD, under
// the Apache License, Version 2.0.
package nn

import "unsafe"

// Dims is the shape of a tensor in NHWC order, as esp-nn data_dims_t.
type Dims struct {
	Width, Height, Channels int
}

// Size2D is a width and height, as esp-nn data_2d_t.
type Size2D struct {
	Width, Height int
}

// Activation is the output range after the quantization, as esp-nn
// act_params_t.
type Activation struct {
	Min, Max int32
}

// Quant has one output multiplier and shift for each output channel, as
// esp-nn quant_data_t.
type Quant struct {
	Shift, Mult []int32
}

// AlignedInt8 returns a slice of n values that starts at a 16 byte boundary.
func AlignedInt8(n int) []int8 {
	s := make([]int8, n+16)
	off := int(-uintptr(unsafe.Pointer(&s[0])) % 16)
	return s[off : off+n : off+n]
}

func (a Activation) clamp(v int32) int32 {
	if v < a.Min {
		v = a.Min
	}
	if v > a.Max {
		v = a.Max
	}
	return v
}

func sat8(v int32) int8 {
	if v > 127 {
		return 127
	}
	if v < -128 {
		return -128
	}
	return int8(v)
}

func checkLen(n int, s ...int) {
	for _, l := range s {
		if l < n {
			panic("nn: slice too short")
		}
	}
}
