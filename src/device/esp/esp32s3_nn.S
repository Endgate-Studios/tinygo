// ESP32-S3 PIE kernels for package device/esp/nn. The Go code in
// src/device/esp/nn prepares the arguments of each call.
//
// These are the ESP32-S3 kernels of esp-nn, https://github.com/espressif/esp-nn,
// changed for the LLVM assembler. Literal loads are movi and slli, each
// function has its own section, and nop instructions align the loop bodies as
// the GNU assembler does. The last three functions are the inline asm
// blocks of the esp-nn C code. The esp-nn license:
//
// SPDX-FileCopyrightText: 2020-2026 Espressif Systems (Shanghai) CO LTD
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// activation_functions/esp_nn_relu_s8_esp32s3.S
.section .text.tinygo_nn_relu6_s8,"ax",@progbits
	.global tinygo_nn_relu6_s8
	.type tinygo_nn_relu6_s8, @function
	.p2align 4
tinygo_nn_relu6_s8:
	entry a1, 48
	extui a5, a2, 0, 4
	beqz.n a5, .Lnn_relu6_s8_Lt_head_aligned
	movi.n a6, 16
	sub a5, a6, a5
	min a5, a5, a3
	sub a3, a3, a5
	movi.n a6, 0
	movi.n a4, 6
	loopgtz a5, .Lnn_relu6_s8_Lt_head_done
	l8ui a8, a2, 0
	sext a8, a8, 7
	max a8, a6, a8
	min a8, a4, a8
	s8i a8, a2, 0
	addi.n a2, a2, 1
.Lnn_relu6_s8_Lt_head_done:
.Lnn_relu6_s8_Lt_head_aligned:
	mov.n a9, a2
	mov.n a7, a3
	movi.n a4, 6
	s8i a4, a1, 0
	addi a10, a3, -7
	ee.vldbc.8 q1, a1
	blti a3, 16, .Lnn_relu6_s8_Lt_0_5634
	srai a8, a3, 4
	ee.zero.q q2
	loopgtz a8, .Lnn_relu6_s8_LBB37_esp_nn_relu6_s8_esp32s3
	ee.vld.128.ip q0, a2, 0
	ee.vmax.s8 q0, q0, q2
	ee.vmin.s8 q0, q0, q1
	ee.vst.128.ip q0, a2, 16
.Lnn_relu6_s8_LBB37_esp_nn_relu6_s8_esp32s3:
	slli a8, a8, 4
	bge a8, a10, .Lnn_relu6_s8_Lt_0_3586
.Lnn_relu6_s8_Lt_0_3842:
	sub a6, a7, a8
	srai a6, a6, 3
	loopgtz a6, .Lnn_relu6_s8_LBB52_esp_nn_relu6_s8_esp32s3
	ee.vld.l.64.ip q0, a2, 0
	ee.vmax.s8 q0, q0, q2
	ee.vmin.s8 q0, q0, q1
	ee.vst.l.64.ip q0, a2, 8
.Lnn_relu6_s8_LBB52_esp_nn_relu6_s8_esp32s3:
	addx8 a8, a6, a8
.Lnn_relu6_s8_Lt_0_3586:
	bge a8, a7, .Lnn_relu6_s8_Lt_0_6402
.Lnn_relu6_s8_Lt_0_4866:
	movi.n a5, 0
	sub a3, a7, a8
	add.n a2, a8, a9
	l8ui a6, a2, 0
	addi.n a3, a3, -1
	sext a6, a6, 7
	max a6, a5, a6
	min a6, a4, a6
	s8i a6, a2, 0
	nop.n
	loopgtz a3, .Lnn_relu6_s8_LBB67_esp_nn_relu6_s8_esp32s3
	l8ui a3, a2, 1
	addi.n a2, a2, 1
	sext a3, a3, 7
	max a3, a5, a3
	min a3, a4, a3
	s8i a3, a2, 0
.Lnn_relu6_s8_LBB67_esp_nn_relu6_s8_esp32s3:
.Lnn_relu6_s8_Lt_0_6402:
	retw.n
.Lnn_relu6_s8_Lt_0_5634:
	blti a10, 1, .Lnn_relu6_s8_Lt_0_5890
	movi.n a8, 0
	ee.zero.q q2
	j .Lnn_relu6_s8_Lt_0_3842
.Lnn_relu6_s8_Lt_0_5890:
	beqz a3, .Lnn_relu6_s8_Lt_0_6402
	movi.n a8, 0
	j .Lnn_relu6_s8_Lt_0_4866
	.size tinygo_nn_relu6_s8, . - tinygo_nn_relu6_s8

// basic_math/esp_nn_add_s8_esp32s3.S
.section .text.tinygo_nn_add_elementwise_s8,"ax",@progbits
	.global tinygo_nn_add_elementwise_s8
	.type tinygo_nn_add_elementwise_s8, @function
	.p2align 4
tinygo_nn_add_elementwise_s8:
	entry a1, 80
	s32i.n a4, a1, 48
	s32i.n a5, a1, 52
	s32i.n a2, a1, 32
	s32i.n a3, a1, 12
	l32i a12, a1, 116
	mov.n a14, a2
	mov.n a10, a3
	bgei a12, 1, .Lnn_far_254
	j .Lnn_add_elementwise_s8_exit
.Lnn_far_254:
	l32i a3, a1, 80
	l32i a13, a1, 84
	l32i a2, a1, 104
	l32i a8, a1, 100
	neg a3, a3
	neg a13, a13
	neg a2, a2
	s32i.n a3, a1, 8
	s32i.n a13, a1, 4
	s32i.n a2, a1, 0
	movi.n a5, 1
	addi a9, a3, -1
	ssl a9
	sll a15, a5
	s32i.n a15, a1, 16
	addi a9, a13, -1
	ssl a9
	sll a15, a5
	s32i.n a15, a1, 20
	addi a9, a2, -1
	ssl a9
	sll a15, a5
	s32i.n a15, a1, 24
	movi.n a2, 0
	bgei a12, 12, .Lnn_far_253
	j .Lnn_add_elementwise_s8_process_leftover
.Lnn_far_253:
	or a9, a14, a10
	extui a9, a9, 0, 4
	bnez a9, .Lnn_add_elementwise_s8_process_leftover
	l32i a9, a1, 92
	l32i a13, a1, 116
	srai a13, a13, 3
	s32i.n a13, a1, 56
	movi.n a13, 8
	s32i.n a13, a1, 28
	ee.zero.q q6
.Lnn_add_elementwise_s8_vector_loop:
	l32i a15, a1, 88
	ee.vld.l.64.ip q0, a14, 8
	s32i.n a9, a1, 44
	s32i.n a14, a1, 40
	wsr.sar a15
	addi a15, a1, 48
	ee.vldbc.16 q7, a15
	ee.vcmp.lt.s8 q5, q0, q6
	ee.vzip.8 q0, q5
	ee.vadds.s16 q0, q0, q7
	ee.vcmp.lt.s16 q2, q0, q6
	ee.vzip.16 q0, q2
	ee.vsl.32 q0, q0
	ee.vsl.32 q2, q2
	movi a9, 1
	slli a9, a9, 30
	ee.movi.32.a q0, a3, 2
	ee.movi.32.a q0, a4, 3
	ee.movi.32.a q0, a14, 1
	ee.movi.32.a q0, a5, 0
	mulsh a13, a6, a3
	mull a3, a6, a3
	mulsh a12, a6, a4
	mull a4, a6, a4
	mulsh a15, a6, a14
	mull a14, a6, a14
	ssai 31
	add a3, a3, a9
	.byte 0x90, 0x23, 0x62 // saltu a2, a3, a9
	add.n a13, a13, a2
	src a13, a13, a3
	add a4, a4, a9
	.byte 0x90, 0x24, 0x62 // saltu a2, a4, a9
	add.n a12, a12, a2
	src a12, a12, a4
	ee.movi.32.q q0, a13, 2
	add a14, a14, a9
	.byte 0x90, 0x2e, 0x62 // saltu a2, a14, a9
	add.n a15, a15, a2
	src a15, a15, a14
	ee.movi.32.q q0, a12, 3
	mulsh a13, a6, a5
	mull a5, a6, a5
	ee.movi.32.q q0, a15, 1
	add a5, a5, a9
	.byte 0x90, 0x25, 0x62 // saltu a2, a5, a9
	add.n a13, a13, a2
	src a13, a13, a5
	ee.movi.32.q q0, a13, 0
	ee.movi.32.a q2, a3, 2
	ee.movi.32.a q2, a4, 3
	ee.movi.32.a q2, a14, 1
	ee.movi.32.a q2, a5, 0
	mulsh a13, a6, a3
	mull a3, a6, a3
	mulsh a12, a6, a4
	mull a4, a6, a4
	mulsh a15, a6, a14
	mull a14, a6, a14
	ssai 31
	add a3, a3, a9
	.byte 0x90, 0x23, 0x62 // saltu a2, a3, a9
	add.n a13, a13, a2
	src a13, a13, a3
	add a4, a4, a9
	.byte 0x90, 0x24, 0x62 // saltu a2, a4, a9
	add.n a12, a12, a2
	src a12, a12, a4
	ee.movi.32.q q2, a13, 2
	add a14, a14, a9
	.byte 0x90, 0x2e, 0x62 // saltu a2, a14, a9
	add.n a15, a15, a2
	src a15, a15, a14
	ee.movi.32.q q2, a12, 3
	mulsh a13, a6, a5
	mull a5, a6, a5
	ee.movi.32.q q2, a15, 1
	l32i a3, a1, 8
	add a5, a5, a9
	.byte 0x90, 0x25, 0x62 // saltu a2, a5, a9
	add.n a13, a13, a2
	src a13, a13, a5
	ee.movi.32.q q2, a13, 0
	blti a3, 1, .Lnn_add_elementwise_s8_skip_div_by2_in0
	addi a13, a1, 16
	ee.vcmp.lt.s32 q1, q0, q6
	ee.vcmp.lt.s32 q3, q2, q6
	ee.vldbc.32 q5, a13
	wsr.sar a3
	ee.vadds.s32 q0, q0, q1
	ee.vadds.s32 q2, q2, q3
	ee.vadds.s32 q0, q0, q5
	ee.vadds.s32 q2, q2, q5
	ee.vsr.32 q0, q0
	ee.vsr.32 q2, q2
.Lnn_add_elementwise_s8_skip_div_by2_in0:
	ee.vld.l.64.ip q1, a10, 8
	addi a15, a1, 52
	ee.vldbc.16 q7, a15
	l32i a15, a1, 88
	s32i a10, a1, 36
	ee.vcmp.lt.s8 q3, q1, q6
	wsr.sar a15
	ee.vzip.8 q1, q3
	ee.vadds.s16 q1, q1, q7
	ee.vcmp.lt.s16 q3, q1, q6
	ee.vzip.16 q1, q3
	ee.vsl.32 q1, q1
	ee.vsl.32 q3, q3
	ee.movi.32.a q1, a3, 2
	ee.movi.32.a q1, a4, 3
	ee.movi.32.a q1, a14, 1
	ee.movi.32.a q1, a5, 0
	mulsh a13, a7, a3
	mull a3, a7, a3
	mulsh a12, a7, a4
	mull a4, a7, a4
	mulsh a15, a7, a14
	mull a14, a7, a14
	ssai 31
	add a3, a3, a9
	.byte 0x90, 0x23, 0x62 // saltu a2, a3, a9
	add.n a13, a13, a2
	src a13, a13, a3
	add a4, a4, a9
	.byte 0x90, 0x24, 0x62 // saltu a2, a4, a9
	add.n a12, a12, a2
	src a12, a12, a4
	ee.movi.32.q q1, a13, 2
	add a14, a14, a9
	.byte 0x90, 0x2e, 0x62 // saltu a2, a14, a9
	add.n a15, a15, a2
	src a15, a15, a14
	ee.movi.32.q q1, a12, 3
	mulsh a13, a7, a5
	mull a5, a7, a5
	ee.movi.32.q q1, a15, 1
	add a5, a5, a9
	.byte 0x90, 0x25, 0x62 // saltu a2, a5, a9
	add.n a13, a13, a2
	src a13, a13, a5
	ee.movi.32.q q1, a13, 0
	ee.movi.32.a q3, a3, 2
	ee.movi.32.a q3, a4, 3
	ee.movi.32.a q3, a14, 1
	ee.movi.32.a q3, a5, 0
	mulsh a13, a7, a3
	mull a3, a7, a3
	mulsh a12, a7, a4
	mull a4, a7, a4
	mulsh a15, a7, a14
	mull a14, a7, a14
	ssai 31
	add a3, a3, a9
	.byte 0x90, 0x23, 0x62 // saltu a2, a3, a9
	add.n a13, a13, a2
	src a13, a13, a3
	add a4, a4, a9
	.byte 0x90, 0x24, 0x62 // saltu a2, a4, a9
	add.n a12, a12, a2
	src a12, a12, a4
	ee.movi.32.q q3, a13, 2
	add a14, a14, a9
	.byte 0x90, 0x2e, 0x62 // saltu a2, a14, a9
	add.n a15, a15, a2
	src a15, a15, a14
	ee.movi.32.q q3, a12, 3
	mulsh a13, a7, a5
	mull a5, a7, a5
	ee.movi.32.q q3, a15, 1
	l32i a14, a1, 4
	add a5, a5, a9
	.byte 0x90, 0x25, 0x62 // saltu a2, a5, a9
	add.n a13, a13, a2
	src a13, a13, a5
	ee.movi.32.q q3, a13, 0
	blti a14, 1, .Lnn_add_elementwise_s8_skip_div_by2_in1
	addi a5, a1, 20
	ee.vcmp.lt.s32 q4, q1, q6
	ee.vcmp.lt.s32 q5, q3, q6
	ee.vldbc.32 q7, a5
	wsr.sar a14
	ee.vadds.s32 q4, q4, q7
	ee.vadds.s32 q5, q5, q7
	ee.vadds.s32 q1, q1, q4
	ee.vadds.s32 q3, q3, q5
	ee.vsr.32 q1, q1
	ee.vsr.32 q3, q3
.Lnn_add_elementwise_s8_skip_div_by2_in1:
	ee.vadds.s32 q0, q0, q1
	ee.vadds.s32 q1, q2, q3
	ee.movi.32.a q0, a3, 2
	ee.movi.32.a q0, a4, 3
	ee.movi.32.a q0, a14, 1
	ee.movi.32.a q0, a5, 0
	mulsh a13, a8, a3
	mull a3, a8, a3
	mulsh a12, a8, a4
	mull a4, a8, a4
	mulsh a15, a8, a14
	mull a14, a8, a14
	ssai 31
	add a3, a3, a9
	.byte 0x90, 0x23, 0x62 // saltu a2, a3, a9
	add.n a13, a13, a2
	src a13, a13, a3
	add a4, a4, a9
	.byte 0x90, 0x24, 0x62 // saltu a2, a4, a9
	add.n a12, a12, a2
	src a12, a12, a4
	ee.movi.32.q q0, a13, 2
	add a14, a14, a9
	.byte 0x90, 0x2e, 0x62 // saltu a2, a14, a9
	add.n a15, a15, a2
	src a15, a15, a14
	ee.movi.32.q q0, a12, 3
	mulsh a13, a8, a5
	mull a5, a8, a5
	ee.movi.32.q q0, a15, 1
	add a5, a5, a9
	.byte 0x90, 0x25, 0x62 // saltu a2, a5, a9
	add.n a13, a13, a2
	src a13, a13, a5
	ee.movi.32.q q0, a13, 0
	ee.movi.32.a q1, a3, 2
	ee.movi.32.a q1, a4, 3
	ee.movi.32.a q1, a14, 1
	ee.movi.32.a q1, a5, 0
	mulsh a13, a8, a3
	mull a3, a8, a3
	mulsh a12, a8, a4
	mull a4, a8, a4
	mulsh a15, a8, a14
	mull a14, a8, a14
	ssai 31
	add a3, a3, a9
	.byte 0x90, 0x23, 0x62 // saltu a2, a3, a9
	add.n a13, a13, a2
	src a13, a13, a3
	add a4, a4, a9
	.byte 0x90, 0x24, 0x62 // saltu a2, a4, a9
	add.n a12, a12, a2
	src a12, a12, a4
	ee.movi.32.q q1, a13, 2
	add a14, a14, a9
	.byte 0x90, 0x2e, 0x62 // saltu a2, a14, a9
	add.n a15, a15, a2
	src a15, a15, a14
	ee.movi.32.q q1, a12, 3
	mulsh a13, a8, a5
	mull a5, a8, a5
	ee.movi.32.q q1, a15, 1
	l32i a14, a1, 0
	add a5, a5, a9
	.byte 0x90, 0x25, 0x62 // saltu a2, a5, a9
	add.n a13, a13, a2
	src a13, a13, a5
	ee.movi.32.q q1, a13, 0
	blti a14, 1, .Lnn_add_elementwise_s8_skip_div_by2_out
	addi a5, a1, 24
	ee.vcmp.lt.s32 q2, q0, q6
	ee.vcmp.lt.s32 q3, q1, q6
	ee.vldbc.32 q5, a5
	wsr.sar a14
	ee.vadds.s32 q0, q0, q2
	ee.vadds.s32 q1, q1, q3
	ee.vadds.s32 q0, q0, q5
	ee.vadds.s32 q1, q1, q5
	ee.vsr.32 q0, q0
	ee.vsr.32 q1, q1
.Lnn_add_elementwise_s8_skip_div_by2_out:
	addi a15, a1, 96
	ee.vldbc.32 q3, a15
	ee.vadds.s32 q0, q0, q3
	ee.vadds.s32 q1, q1, q3
	addi a13, a1, 108
	addi a14, a1, 112
	ee.vldbc.32 q3, a14
	ee.vmin.s32 q0, q0, q3
	ee.vmin.s32 q1, q1, q3
	ee.vldbc.32 q3, a13
	l32i a13, a1, 4
	ee.vmax.s32 q1, q1, q3
	ee.vmax.s32 q0, q0, q3
	l32i.n a9, a1, 44
	ee.vunzip.16 q0, q1
	ee.vunzip.8 q0, q1
	l32i.n a13, a1, 28
	ee.vst.l.64.ip q0, a9, 8
	l32i a15, a1, 116
	l32i.n a14, a1, 40
	l32i.n a10, a1, 36
	addi a13, a13, 8
	s32i.n a13, a1, 28
	blt a15, a13, .Lnn_far_252
	j .Lnn_add_elementwise_s8_vector_loop
.Lnn_far_252:
	l32i.n a2, a1, 56
	l32i a10, a1, 116
	slli a2, a2, 3
	blt a2, a10, .Lnn_far_251
	j .Lnn_add_elementwise_s8_exit
.Lnn_far_251:
.Lnn_add_elementwise_s8_process_leftover:
	l32i.n a3, a1, 48
	l32i.n a12, a1, 52
	l32i.n a10, a1, 12
	l32i.n a14, a1, 32
	add.n a10, a2, a10
	add.n a14, a2, a14
	l8ui a14, a14, 0
	l8ui a10, a10, 0
	sext a14, a14, 7
	sext a10, a10, 7
	add.n a10, a10, a12
	add.n a14, a14, a3
	l32i a12, a1, 88
	ssl a12
	sll a10, a10
	sll a14, a14
	movi a12, 1
	slli a12, a12, 30
	mulsh a13, a14, a6
	mull a9, a14, a6
	ssai 31
	add a9, a9, a12
	.byte 0xc0, 0x39, 0x62 // saltu a3, a9, a12
	add.n a13, a13, a3
	src a14, a13, a9
	mulsh a13, a10, a7
	mull a9, a10, a7
	ssai 31
	add a9, a9, a12
	.byte 0xc0, 0x39, 0x62 // saltu a3, a9, a12
	add.n a13, a13, a3
	src a10, a13, a9
	l32i.n a12, a1, 8
	l32i.n a13, a1, 4
	blti a12, 1, .Lnn_add_elementwise_s8_skip_div_by2_in0_remain
	l32i.n a3, a1, 16
	extui a9, a14, 31, 1
	ssr a12
	sub a3, a3, a9
	add a14, a14, a3
	sra a14, a14
.Lnn_add_elementwise_s8_skip_div_by2_in0_remain:
	blti a13, 1, .Lnn_add_elementwise_s8_skip_div_by2_in1_remain
	l32i.n a3, a1, 20
	extui a9, a10, 31, 1
	ssr a13
	sub a3, a3, a9
	add a10, a10, a3
	sra a10, a10
.Lnn_add_elementwise_s8_skip_div_by2_in1_remain:
	movi a12, 1
	slli a12, a12, 30
	l32i a13, a1, 0
	add.n a10, a10, a14
	mulsh a3, a10, a8
	mull a10, a10, a8
	ssai 31
	add a10, a10, a12
	.byte 0xc0, 0x9a, 0x62 // saltu a9, a10, a12
	add a12, a3, a9
	src a12, a12, a10
	l32i a9, a1, 96
	blti a13, 1, .Lnn_add_elementwise_s8_skip_div_by2_out_remain
	l32i.n a3, a1, 24
	extui a14, a12, 31, 1
	ssr a13
	sub a3, a3, a14
	add a12, a12, a3
	sra a12, a12
.Lnn_add_elementwise_s8_skip_div_by2_out_remain:
	add.n a9, a9, a12
	l32i a13, a1, 112
	l32i a12, a1, 108
	min a13, a13, a9
	l32i a9, a1, 92
	max a13, a13, a12
	add.n a9, a2, a9
	s8i a13, a9, 0
	l32i a12, a1, 116
	addi.n a2, a2, 1
	bge a2, a12, .Lnn_far_250
	j .Lnn_add_elementwise_s8_process_leftover
.Lnn_far_250:
.Lnn_add_elementwise_s8_exit:
	retw.n
	.size tinygo_nn_add_elementwise_s8, . - tinygo_nn_add_elementwise_s8

// basic_math/esp_nn_mul_broadcast_s8_esp32s3.S
.section .text.tinygo_nn_mul_broadcast_channel_s8,"ax",@progbits
	.global tinygo_nn_mul_broadcast_channel_s8
	.type tinygo_nn_mul_broadcast_channel_s8, @function
	.p2align 4
tinygo_nn_mul_broadcast_channel_s8:
	entry a1, 120
	s32i.n a3, a1, 4
	s32i.n a6, a1, 8
	s32i.n a2, a1, 16
	s32i.n a4, a1, 24
	s32i.n a5, a1, 28
	s32i a7, a1, 40
	l32i a8, a1, 136
	l32i a9, a1, 140
	s32i.n a9, a1, 12
	bgei a8, 1, .Lnn_far_249
	j .Lnn_mul_broadcast_channel_s8_Lexit
.Lnn_far_249:
	bgei a9, 1, .Lnn_far_248
	j .Lnn_mul_broadcast_channel_s8_Lexit
.Lnn_far_248:
	l32i a15, a1, 124
	movi.n a11, 0
	max a14, a15, a11
	sub a4, a14, a15
	s32i.n a4, a1, 20
	l32i a13, a1, 120
	movi a4, 1
	slli a4, a4, 30
	l32i a8, a1, 136
	s32i a5, a1, 48
	l32i.n a5, a1, 24
	s32i a5, a1, 44
	movi.n a10, 0
	s32i a10, a1, 32
.Lnn_mul_broadcast_channel_s8_Lspatial_loop:
	l32i a8, a1, 136
	l32i a10, a1, 32
	blt a10, a8, .Lnn_far_247
	j .Lnn_mul_broadcast_channel_s8_Lexit
.Lnn_far_247:
	l32i.n a3, a1, 4
	l32i.n a9, a1, 12
	movi.n a11, 0
	bgei a9, 8, .Lnn_far_246
	j .Lnn_mul_broadcast_channel_s8_Lchannel_leftover
.Lnn_far_246:
	or a8, a2, a3
	or a8, a8, a6
	extui a8, a8, 0, 4
	bnez a8, .Lnn_mul_broadcast_channel_s8_Lchannel_leftover
	ee.zero.q q1
	addi a8, a1, 44
	ee.vldbc.16 q0, a8
	addi a8, a1, 48
	ee.vldbc.16 q7, a8
	.byte 0x14, 0x64, 0xcd // st.qr q0, a1, 64
.Lnn_mul_broadcast_channel_s8_Lchannel_simd_loop:
	addi a8, a9, -7
	blt a11, a8, .Lnn_mul_broadcast_channel_s8_Lchannel_simd_body
	j .Lnn_mul_broadcast_channel_s8_Lchannel_leftover
.Lnn_mul_broadcast_channel_s8_Lchannel_simd_body:
	.byte 0x14, 0x24, 0xed // ld.qr q4, a1, 64
	ee.vld.l.64.ip q2, a2, 8
	movi.n a7, 16
	ee.vld.h.64.ip q2, a3, 8
	wsr.sar a7
	ee.vcmp.lt.s8 q5, q2, q1
	ee.vzip.8 q2, q5
	ee.vadds.s16 q5, q5, q7
	ee.vadds.s16 q4, q2, q4
	ee.vmul.s16 q3, q4, q5
	ssai 0
	ee.vmul.s16 q2, q4, q5
	wsr.sar a14
	ee.vzip.16 q2, q3
	ee.vsl.32 q6, q2
	ssai 31
	ee.movi.32.a q6, a8, 2
	mulsh a7, a13, a8
	mull a8, a13, a8
	add.n a8, a4, a8
	.byte 0x40, 0x58, 0x62 // saltu a5, a8, a4
	add.n a5, a5, a7
	src a5, a5, a8
	ee.movi.32.a q6, a8, 3
	mulsh a7, a13, a8
	mull a8, a13, a8
	add.n a8, a4, a8
	.byte 0x40, 0xc8, 0x62 // saltu a12, a8, a4
	add.n a12, a12, a7
	src a12, a12, a8
	ee.movi.32.q q2, a5, 2
	ee.movi.32.q q2, a12, 3
	ee.movi.32.a q6, a8, 1
	mulsh a7, a13, a8
	mull a8, a13, a8
	add.n a8, a4, a8
	.byte 0x40, 0x58, 0x62 // saltu a5, a8, a4
	add.n a5, a5, a7
	src a5, a5, a8
	ee.movi.32.a q6, a8, 0
	mulsh a7, a13, a8
	mull a8, a13, a8
	add.n a8, a4, a8
	.byte 0x40, 0xc8, 0x62 // saltu a12, a8, a4
	add.n a12, a12, a7
	src a12, a12, a8
	ee.movi.32.q q2, a5, 1
	ee.movi.32.q q2, a12, 0
	wsr.sar a14
	ee.vsl.32 q4, q3
	ssai 31
	ee.movi.32.a q4, a8, 2
	mulsh a7, a13, a8
	mull a8, a13, a8
	add.n a8, a4, a8
	.byte 0x40, 0x58, 0x62 // saltu a5, a8, a4
	add.n a5, a5, a7
	src a5, a5, a8
	ee.movi.32.a q4, a8, 3
	mulsh a7, a13, a8
	mull a8, a13, a8
	add.n a8, a4, a8
	.byte 0x40, 0xc8, 0x62 // saltu a12, a8, a4
	add.n a12, a12, a7
	src a12, a12, a8
	ee.movi.32.q q0, a5, 2
	ee.movi.32.q q0, a12, 3
	ee.movi.32.a q4, a8, 1
	mulsh a7, a13, a8
	mull a8, a13, a8
	add.n a8, a4, a8
	.byte 0x40, 0x58, 0x62 // saltu a5, a8, a4
	add.n a5, a5, a7
	src a5, a5, a8
	ee.movi.32.a q4, a8, 0
	mulsh a7, a13, a8
	mull a8, a13, a8
	add.n a8, a4, a8
	.byte 0x40, 0xc8, 0x62 // saltu a12, a8, a4
	add.n a12, a12, a7
	src a12, a12, a8
	ee.movi.32.q q0, a5, 1
	ee.movi.32.q q0, a12, 0
	l32i.n a5, a1, 20
	movi.n a7, 1
	blti a5, 1, .Lnn_mul_broadcast_channel_s8_Lskip_div
	ee.vcmp.lt.s32 q5, q2, q1
	ee.vcmp.lt.s32 q6, q0, q1
	addi.n a8, a5, -1
	ssl a8
	sll a7, a7
	s32i.n a7, a1, 0
	ee.vldbc.32 q4, a1
	wsr.sar a5
	ee.vadds.s32 q5, q4, q5
	ee.vadds.s32 q5, q2, q5
	ee.vsr.32 q2, q5
	wsr.sar a5
	ee.vadds.s32 q5, q4, q6
	ee.vadds.s32 q5, q0, q5
	ee.vsr.32 q0, q5
.Lnn_mul_broadcast_channel_s8_Lskip_div:
	addi a8, a1, 132
	ee.vldbc.32 q4, a8
	addi a5, a1, 40
	ee.vldbc.32 q6, a5
	addi a7, a1, 128
	ee.vadds.s32 q0, q0, q6
	ee.vadds.s32 q2, q2, q6
	ee.vldbc.32 q6, a7
	ee.vmin.s32 q0, q0, q4
	ee.vmin.s32 q2, q2, q4
	ee.vmax.s32 q0, q0, q6
	ee.vmax.s32 q2, q2, q6
	ee.vunzip.16 q2, q0
	ee.vunzip.8 q2, q0
	ee.vst.l.64.ip q2, a6, 8
	addi a11, a11, 8
	j .Lnn_mul_broadcast_channel_s8_Lchannel_simd_loop
.Lnn_mul_broadcast_channel_s8_Lchannel_leftover:
	l32i.n a9, a1, 12
	bge a11, a9, .Lnn_mul_broadcast_channel_s8_Lspatial_next
	ssl a14
	l32i.n a8, a1, 24
	l8ui a10, a2, 0
	sext a10, a10, 7
	add.n a10, a10, a8
	l32i.n a8, a1, 28
	l8ui a12, a3, 0
	sext a12, a12, 7
	add.n a12, a12, a8
	mull a10, a10, a12
	sll a10, a10
	l32i.n a9, a1, 20
	mulsh a8, a10, a13
	mull a12, a10, a13
	ssai 31
	add.n a12, a4, a12
	.byte 0x40, 0xac, 0x62 // saltu a10, a12, a4
	add.n a10, a10, a8
	src a10, a10, a12
	blti a9, 1, .Lnn_mul_broadcast_channel_s8_Lskip_div_scalar
	addi a8, a9, -1
	ssl a8
	movi a7, 1
	sll a7, a7
	extui a8, a10, 31, 1
	sub a10, a10, a8
	add a10, a10, a7
	ssr a9
	sra a10, a10
.Lnn_mul_broadcast_channel_s8_Lskip_div_scalar:
	l32i a8, a1, 40
	l32i a7, a1, 128
	l32i a12, a1, 132
	add.n a10, a10, a8
	min a10, a10, a12
	max a10, a10, a7
	s8i a10, a6, 0
	addi a2, a2, 1
	addi a3, a3, 1
	addi a6, a6, 1
	addi a11, a11, 1
	j .Lnn_mul_broadcast_channel_s8_Lchannel_leftover
.Lnn_mul_broadcast_channel_s8_Lspatial_next:
	l32i a10, a1, 32
	addi a10, a10, 1
	s32i a10, a1, 32
	j .Lnn_mul_broadcast_channel_s8_Lspatial_loop
.Lnn_mul_broadcast_channel_s8_Lexit:
	retw.n
	.size tinygo_nn_mul_broadcast_channel_s8, . - tinygo_nn_mul_broadcast_channel_s8

// basic_math/esp_nn_mul_s8_esp32s3.S
.section .text.tinygo_nn_mul_elementwise_s8,"ax",@progbits
	.global tinygo_nn_mul_elementwise_s8
	.type tinygo_nn_mul_elementwise_s8, @function
	.p2align 4
tinygo_nn_mul_elementwise_s8:
	entry a1, 120
	s32i.n a4, a1, 24
	s32i.n a5, a1, 28
	s32i.n a3, a1, 4
	mov.n a10, a3
	l32i a3, a1, 136
	mov.n a9, a6
	bgei a3, 1, .Lnn_far_245
	j .Lnn_mul_elementwise_s8_exit
.Lnn_far_245:
	s32i.n a2, a1, 16
	s32i a7, a1, 40
	movi.n a11, 0
	mov.n a12, a2
	s32i a4, a1, 44
	s32i a5, a1, 48
	movi.n a2, 1
	l32i a15, a1, 124
	l32i a13, a1, 120
	s32i.n a6, a1, 8
	max a14, a15, a11
	sub a4, a14, a15
	s32i.n a4, a1, 20
	movi a4, 1
	slli a4, a4, 30
	bgei a3, 8, .Lnn_far_244
	j .Lnn_mul_elementwise_s8_process_leftover
.Lnn_far_244:
	or a6, a12, a10
	extui a6, a6, 0, 4
	bnez a6, .Lnn_mul_elementwise_s8_process_leftover
	ee.zero.q q1
	addi a4, a1, 44
	addi a8, a1, 48
	ee.vldbc.16 q0, a4
	ee.vldbc.16 q7, a8
	movi a4, 1
	slli a4, a4, 30
	movi a8, 8
	.byte 0x14, 0x64, 0xcd // st.qr q0, a1, 64
	s32i.n a8, a1, 12
.Lnn_mul_elementwise_s8_Lt_0_7682:
	s32i a9, a1, 36
	.byte 0x14, 0x24, 0xed // ld.qr q4, a1, 64
	ee.vld.l.64.ip q2, a12, 8
	movi.n a7, 16
	ee.vld.h.64.ip q2, a10, 8
	wsr.sar a7
	ee.vcmp.lt.s8 q5, q2, q1
	ee.vzip.8 q2, q5
	ee.vadds.s16 q5, q5, q7
	ee.vadds.s16 q4, q2, q4
	ee.vmul.s16 q3, q4, q5
	wsr.sar a11
	ee.vmul.s16 q2, q4, q5
	wsr.sar a14
	ee.vzip.16 q2, q3
	ee.vsl.32 q6, q2
	ssai 31
	ee.movi.32.a q6, a3, 2
	ee.movi.32.a q6, a8, 3
	mulsh a6, a13, a3
	mull a3, a13, a3
	mulsh a7, a13, a8
	add.n a3, a4, a3
	.byte 0x40, 0x23, 0x62 // saltu a2, a3, a4
	add.n a2, a2, a6
	src a2, a2, a3
	mull a6, a13, a8
	add.n a6, a4, a6
	.byte 0x40, 0x96, 0x62 // saltu a9, a6, a4
	add.n a9, a9, a7
	src a9, a9, a6
	ee.movi.32.q q2, a2, 2
	ee.movi.32.q q2, a9, 3
	ee.movi.32.a q6, a6, 1
	mulsh a7, a13, a6
	mull a6, a13, a6
	add.n a6, a4, a6
	.byte 0x40, 0x36, 0x62 // saltu a3, a6, a4
	add.n a3, a3, a7
	src a3, a3, a6
	ee.movi.32.a q6, a2, 0
	mulsh a8, a13, a2
	mull a7, a13, a2
	add.n a7, a4, a7
	.byte 0x40, 0x67, 0x62 // saltu a6, a7, a4
	add.n a6, a6, a8
	src a6, a6, a7
	ee.movi.32.q q2, a3, 1
	ee.movi.32.q q2, a6, 0
	wsr.sar a14
	ee.vsl.32 q4, q3
	ee.movi.32.a q4, a2, 2
	mulsh a3, a13, a2
	mull a2, a13, a2
	ssai 31
	add.n a2, a4, a2
	.byte 0x40, 0x52, 0x62 // saltu a5, a2, a4
	add.n a5, a5, a3
	src a5, a5, a2
	ee.movi.32.a q4, a3, 3
	mulsh a6, a13, a3
	mull a3, a13, a3
	add.n a3, a4, a3
	.byte 0x40, 0x83, 0x62 // saltu a8, a3, a4
	add.n a8, a8, a6
	src a8, a8, a3
	ee.movi.32.q q0, a5, 2
	ee.movi.32.q q0, a8, 3
	ee.movi.32.a q4, a7, 1
	mulsh a6, a13, a7
	mull a3, a13, a7
	add.n a3, a4, a3
	.byte 0x40, 0x23, 0x62 // saltu a2, a3, a4
	add.n a2, a2, a6
	src a2, a2, a3
	ee.movi.32.a q4, a6, 0
	mulsh a7, a13, a6
	mull a6, a13, a6
	add.n a6, a4, a6
	.byte 0x40, 0x36, 0x62 // saltu a3, a6, a4
	add.n a3, a3, a7
	src a3, a3, a6
	ee.movi.32.q q0, a2, 1
	ee.movi.32.q q0, a3, 0
	l32i.n a5, a1, 20
	movi.n a7, 1
	blti a5, 1, .Lnn_mul_elementwise_s8_skip_div_by_pow_of_2
	ee.vcmp.lt.s32 q5, q2, q1
	ee.vcmp.lt.s32 q6, q0, q1
	addi.n a8, a5, -1
	ssl a8
	sll a7, a7
	s32i.n a7, a1, 0
	ee.vldbc.32 q4, a1
	wsr.sar a5
	ee.vadds.s32 q5, q4, q5
	ee.vadds.s32 q5, q2, q5
	ee.vsr.32 q2, q5
	wsr.sar a5
	ee.vadds.s32 q5, q4, q6
	ee.vadds.s32 q5, q0, q5
	ee.vsr.32 q0, q5
.Lnn_mul_elementwise_s8_skip_div_by_pow_of_2:
	addi a8, a1, 132
	ee.vldbc.32 q4, a8
	addi a5, a1, 40
	ee.vldbc.32 q6, a5
	addi a7, a1, 128
	ee.vadds.s32 q0, q0, q6
	ee.vadds.s32 q2, q2, q6
	ee.vldbc.32 q6, a7
	ee.vmin.s32 q0, q0, q4
	ee.vmin.s32 q2, q2, q4
	ee.vmax.s32 q0, q0, q6
	ee.vmax.s32 q2, q2, q6
	ee.vunzip.16 q2, q0
	ee.vunzip.8 q2, q0
	l32i.n a7, a1, 12
	l32i a9, a1, 36
	l32i a3, a1, 136
	ee.vst.l.64.ip q2, a9, 8
	addi a7, a7, 8
	s32i.n a7, a1, 12
	blt a3, a7, .Lnn_far_243
	j .Lnn_mul_elementwise_s8_Lt_0_7682
.Lnn_far_243:
	addi a11, a7, -8
	bge a11, a3, .Lnn_mul_elementwise_s8_exit
.Lnn_mul_elementwise_s8_process_leftover:
	sub a8, a3, a11
	loopgtz a8, .Lnn_mul_elementwise_s8_LBB33_esp_nn_mul_elementwise_s8_esp32s3
	ssl a14
	l32i.n a8, a1, 24
	l32i.n a10, a1, 4
	l32i.n a12, a1, 16
	add.n a10, a11, a10
	add.n a12, a11, a12
	l8ui a12, a12, 0
	l8ui a10, a10, 0
	sext a12, a12, 7
	add.n a12, a12, a8
	l32i.n a8, a1, 28
	sext a10, a10, 7
	add.n a10, a10, a8
	mull a10, a12, a10
	l32i.n a9, a1, 20
	sll a10, a10
	mulsh a3, a10, a13
	mull a8, a10, a13
	ssai 31
	add.n a6, a8, a4
	.byte 0x80, 0x86, 0x62 // saltu a8, a6, a8
	add.n a8, a8, a3
	src a3, a8, a6
	blti a9, 1, .Lnn_mul_elementwise_s8_skip_div_by_pow_of_2_remains
	addi a6, a9, -1
	ssl a6
	movi a7, 1
	sll a7, a7
	extui a8, a3, 31, 1
	sub a3, a3, a8
	add a3, a3, a7
	ssr a9
	sra a3, a3
.Lnn_mul_elementwise_s8_skip_div_by_pow_of_2_remains:
	l32i.n a6, a1, 40
	l32i a8, a1, 132
	l32i a7, a1, 128
	add.n a3, a3, a6
	min a8, a8, a3
	l32i.n a3, a1, 8
	max a8, a8, a7
	add.n a3, a11, a3
	s8i a8, a3, 0
	addi.n a11, a11, 1
.Lnn_mul_elementwise_s8_LBB33_esp_nn_mul_elementwise_s8_esp32s3:
.Lnn_mul_elementwise_s8_exit:
	retw.n
	.size tinygo_nn_mul_elementwise_s8, . - tinygo_nn_mul_elementwise_s8

// common/esp_nn_common_functions_esp32s3.S
.section .text.tinygo_nn_aligned_s8_to_s16_with_offset,"ax",@progbits
	.global tinygo_nn_aligned_s8_to_s16_with_offset
	.type tinygo_nn_aligned_s8_to_s16_with_offset, @function
	.p2align 4
tinygo_nn_aligned_s8_to_s16_with_offset:
	entry a1, 48
	mov.n a10, a2
	mov.n a9, a3
	mov.n a8, a4
	s32i.n a5, a1, 12
	addi.n a2, a1, 12
	bgei a4, 32, .Lnn_far_242
	j .Lnn_aligned_s8_to_s16_with_offset_Lt_2_6402
.Lnn_far_242:
	addi.n a6, a8, -1
	ee.zero.q q5
	ee.vldbc.16 q4, a2
	mov.n a3, a10
	mov.n a2, a9
	ee.vld.128.ip q0, a3, 16
	ee.vld.128.ip q1, a3, 16
	ee.vcmp.lt.s8 q2, q0, q5
	ee.vzip.8 q0, q2
	ee.vadds.s16 q0, q0, q4
	ee.vadds.s16.st.incp q0, a2, q0, q2, q4
	bgei a4, 64, .Lnn_far_241
	j .Lnn_aligned_s8_to_s16_with_offset_Lt_2_7170
.Lnn_far_241:
	addi a5, a4, -32
	srai a5, a5, 5
	slli a4, a5, 5
	loopgtz a5, .Lnn_aligned_s8_to_s16_with_offset_LBB37_esp_nn_aligned_s8_to_s16_with_offset_esp32s3
	ee.vst.128.ip q0, a2, 16
	ee.vcmp.lt.s8 q0, q1, q5
	ee.vzip.8 q1, q0
	ee.vadds.s16.ld.incp q2, a3, q3, q1, q4
	ee.vadds.s16.st.incp q3, a2, q0, q0, q4
	ee.vcmp.lt.s8 q3, q2, q5
	ee.vst.128.ip q0, a2, 16
	ee.vzip.8 q2, q3
	ee.vadds.s16.ld.incp q1, a3, q0, q2, q4
	ee.vadds.s16.st.incp q0, a2, q0, q3, q4
.Lnn_aligned_s8_to_s16_with_offset_LBB37_esp_nn_aligned_s8_to_s16_with_offset_esp32s3:
	addi a4, a4, 32
.Lnn_aligned_s8_to_s16_with_offset_Lt_2_3842:
	ee.vst.128.ip q0, a2, 16
	ee.vcmp.lt.s8 q2, q1, q5
	ee.vzip.8 q1, q2
	ee.vadds.s16 q2, q2, q4
	ee.vadds.s16 q3, q1, q4
	ee.vst.128.ip q3, a2, 16
	ee.vst.128.ip q2, a2, 16
	bge a4, a6, .Lnn_aligned_s8_to_s16_with_offset_Lt_2_4866
	l32i.n a5, a1, 12
.Lnn_aligned_s8_to_s16_with_offset_Lt_2_5122:
	mov.n a11, a4
	add.n a2, a4, a10
	l8ui a7, a2, 0
	addx2 a6, a4, a9
	sext a7, a7, 7
	add.n a7, a7, a5
	s16i a7, a6, 0
	l8ui a3, a2, 1
	sub a7, a8, a4
	addi.n a2, a2, 2
	srai a7, a7, 1
	sext a3, a3, 7
	add.n a3, a3, a5
	s16i a3, a6, 2
	addi.n a3, a7, -1
	loopgtz a3, .Lnn_aligned_s8_to_s16_with_offset_LBB52_esp_nn_aligned_s8_to_s16_with_offset_esp32s3
	l8ui a3, a2, 0
	addi.n a6, a6, 4
	sext a3, a3, 7
	add.n a3, a3, a5
	s16i a3, a6, 0
	l8ui a3, a2, 1
	addi.n a2, a2, 2
	sext a3, a3, 7
	add.n a3, a3, a5
	s16i a3, a6, 2
.Lnn_aligned_s8_to_s16_with_offset_LBB52_esp_nn_aligned_s8_to_s16_with_offset_esp32s3:
	addx2 a4, a7, a11
.Lnn_aligned_s8_to_s16_with_offset_Lt_2_4866:
	bge a4, a8, .Lnn_aligned_s8_to_s16_with_offset_Lt_2_7682
	addx2 a11, a4, a9
	add.n a8, a4, a10
	l8ui a8, a8, 0
	l32i.n a12, a1, 12
	sext a8, a8, 7
	add.n a8, a8, a12
	s16i a8, a11, 0
	retw.n
.Lnn_aligned_s8_to_s16_with_offset_Lt_2_6402:
	blti a4, 2, .Lnn_aligned_s8_to_s16_with_offset_Lt_2_6658
	movi.n a4, 0
	j .Lnn_aligned_s8_to_s16_with_offset_Lt_2_5122
.Lnn_aligned_s8_to_s16_with_offset_Lt_2_7682:
	retw.n
.Lnn_aligned_s8_to_s16_with_offset_Lt_2_6658:
	blti a4, 1, .Lnn_aligned_s8_to_s16_with_offset_Lt_2_7682
	l8ui a11, a10, 0
	sext a11, a11, 7
	add.n a11, a11, a5
	s16i a11, a3, 0
	retw.n
.Lnn_aligned_s8_to_s16_with_offset_Lt_2_7170:
	movi.n a4, 32
	j .Lnn_aligned_s8_to_s16_with_offset_Lt_2_3842
	.size tinygo_nn_aligned_s8_to_s16_with_offset, . - tinygo_nn_aligned_s8_to_s16_with_offset

// common/esp_nn_common_functions_esp32s3.S
.section .text.tinygo_nn_s8_to_s16,"ax",@progbits
	.global tinygo_nn_s8_to_s16
	.type tinygo_nn_s8_to_s16, @function
	.p2align 4
tinygo_nn_s8_to_s16:
	entry a1, 32
	mov.n a9, a2
	mov.n a8, a3
	mov.n a7, a4
	bgei a4, 1, .Lnn_far_240
	j .Lnn_s8_to_s16_Lt_3_4866
.Lnn_far_240:
	bgei a4, 16, .Lnn_far_239
	j .Lnn_s8_to_s16_Lt_3_4610
.Lnn_far_239:
	extui a2, a2, 0, 4
	wur.sar_byte a2
	mov.n a2, a9
	ee.vld.128.ip q0, a2, 16
	ee.vld.128.ip q1, a2, 16
	ee.zero.q q4
	bgei a4, 32, .Lnn_far_238
	j .Lnn_s8_to_s16_Lt_3_5378
.Lnn_far_238:
	addi a6, a4, -16
	srai a6, a6, 4
	slli a4, a6, 4
	loopgtz a6, .Lnn_s8_to_s16_LBB35_esp_nn_s8_to_s16_esp32s3
	ee.src.q.qup q2, q0, q1
	ee.vcmp.lt.s8 q3, q2, q4
	ee.vld.128.ip q1, a2, 16
	ee.vzip.8 q2, q3
	ee.vst.128.ip q2, a3, 16
	ee.vst.128.ip q3, a3, 16
.Lnn_s8_to_s16_LBB35_esp_nn_s8_to_s16_esp32s3:
	addi a4, a4, 16
.Lnn_s8_to_s16_Lt_3_2050:
	ee.src.q.qup q5, q0, q1
	ee.vcmp.lt.s8 q3, q5, q4
	ee.vzip.8 q5, q3
	ee.vst.128.ip q5, a3, 16
	ee.vst.128.ip q3, a3, 16
	bge a4, a7, .Lnn_s8_to_s16_Lt_3_4866
.Lnn_s8_to_s16_Lt_3_3330:
	mov.n a11, a4
	add.n a2, a4, a9
	l8ui a10, a2, 0
	addx2 a5, a4, a8
	sext a10, a10, 7
	s16i a10, a5, 0
	l8ui a3, a2, 1
	sub a10, a7, a4
	addi.n a2, a2, 2
	addi.n a10, a10, 1
	srai a10, a10, 1
	sext a3, a3, 7
	s16i a3, a5, 2
	addi.n a3, a10, -1
	loopgtz a3, .Lnn_s8_to_s16_LBB50_esp_nn_s8_to_s16_esp32s3
	l8ui a3, a2, 0
	addi.n a5, a5, 4
	sext a3, a3, 7
	s16i a3, a5, 0
	l8ui a3, a2, 1
	addi.n a2, a2, 2
	sext a3, a3, 7
	s16i a3, a5, 2
.Lnn_s8_to_s16_LBB50_esp_nn_s8_to_s16_esp32s3:
	addx2 a4, a10, a11
	bge a4, a7, .Lnn_s8_to_s16_Lt_3_4866
	add.n a11, a4, a9
	l8ui a11, a11, 0
	addx2 a12, a4, a8
	sext a11, a11, 7
	s16i a11, a12, 0
	retw.n
.Lnn_s8_to_s16_Lt_3_4610:
	movi.n a4, 0
	j .Lnn_s8_to_s16_Lt_3_3330
.Lnn_s8_to_s16_Lt_3_4866:
	retw.n
.Lnn_s8_to_s16_Lt_3_5378:
	movi.n a4, 16
	j .Lnn_s8_to_s16_Lt_3_2050
	.size tinygo_nn_s8_to_s16, . - tinygo_nn_s8_to_s16

// common/esp_nn_common_functions_esp32s3.S
.section .text.tinygo_nn_s8_to_s16_with_offset_row,"ax",@progbits
	.global tinygo_nn_s8_to_s16_with_offset_row
	.type tinygo_nn_s8_to_s16_with_offset_row, @function
	.p2align 4
tinygo_nn_s8_to_s16_with_offset_row:
	entry a1, 48
	s32i.n a5, a1, 12
	addi.n a7, a1, 12
	ee.zero.q q5
	ee.vldbc.16 q4, a7
	mov.n a10, a2
	mov.n a11, a3
	srai a6, a4, 4
	addi.n a6, a6, -1
	srai a9, a6, 1
	slli a6, a9, 1
	slli a6, a6, 4
	ee.ld.128.usar.ip q0, a2, 16
	ee.ld.128.usar.ip q1, a2, 16
	loopgtz a9, .Lnn_s8_to_s16_with_offset_row_Lrow_vec_end
	ee.src.q q2, q0, q1
	ee.vld.128.ip q0, a2, 16
	ee.vcmp.lt.s8 q3, q2, q5
	ee.vzip.8 q2, q3
	ee.vadds.s16 q2, q2, q4
	ee.vadds.s16 q3, q3, q4
	ee.vst.128.ip q2, a3, 16
	ee.vst.128.ip q3, a3, 16
	ee.src.q q2, q1, q0
	ee.vld.128.ip q1, a2, 16
	ee.vcmp.lt.s8 q3, q2, q5
	ee.vzip.8 q2, q3
	ee.vadds.s16 q2, q2, q4
	ee.vadds.s16 q3, q3, q4
	ee.vst.128.ip q2, a3, 16
	ee.vst.128.ip q3, a3, 16
.Lnn_s8_to_s16_with_offset_row_Lrow_vec_end:
	add.n a2, a10, a6
	add.n a3, a11, a6
	add.n a3, a3, a6
	sub a4, a4, a6
	l32i.n a5, a1, 12
	beqz a4, .Lnn_s8_to_s16_with_offset_row_Lrow_done
	loopgtz a4, .Lnn_s8_to_s16_with_offset_row_Lrow_tail_end
	l8ui a8, a2, 0
	sext a8, a8, 7
	add.n a8, a8, a5
	s16i a8, a3, 0
	addi.n a2, a2, 1
	addi.n a3, a3, 2
.Lnn_s8_to_s16_with_offset_row_Lrow_tail_end:
.Lnn_s8_to_s16_with_offset_row_Lrow_done:
	retw.n
	.size tinygo_nn_s8_to_s16_with_offset_row, . - tinygo_nn_s8_to_s16_with_offset_row

// common/esp_nn_dot_s8_esp32s3.S
.section .text.tinygo_nn_dot_s8_aligned,"ax",@progbits
	.global tinygo_nn_dot_s8_aligned
	.type tinygo_nn_dot_s8_aligned, @function
	.p2align 4
tinygo_nn_dot_s8_aligned:
	entry a1, 32
	ee.zero.accx
	beqz a4, .Lnn_dot_s8_aligned_Lalign_done
	srli a5, a4, 4
	beqz a5, .Lnn_dot_s8_aligned_Lalign_done
	ee.vld.128.ip q0, a2, 16
	ee.vld.128.ip q1, a3, 16
	addi a5, a5, -1
	beqz a5, .Lnn_dot_s8_aligned_Lalign_last
	nop.n
	loopgtz a5, .Lnn_dot_s8_aligned_Lalign_loop_end
	ee.vmulas.s8.accx.ld.ip q0, a2, 16, q0, q1
	ee.vld.128.ip q1, a3, 16
.Lnn_dot_s8_aligned_Lalign_loop_end:
.Lnn_dot_s8_aligned_Lalign_last:
	ee.vmulas.s8.accx q0, q1
.Lnn_dot_s8_aligned_Lalign_done:
	nop
	nop
	rur.accx_0 a2
	retw.n
	.size tinygo_nn_dot_s8_aligned, . - tinygo_nn_dot_s8_aligned

// common/esp_nn_dot_s8_esp32s3.S
.section .text.tinygo_nn_dot_s8_unaligned,"ax",@progbits
	.global tinygo_nn_dot_s8_unaligned
	.type tinygo_nn_dot_s8_unaligned, @function
	.p2align 4
tinygo_nn_dot_s8_unaligned:
	entry a1, 32
	ee.zero.accx
	extui a7, a2, 0, 4
	extui a6, a3, 0, 4
	bnez a7, .Lnn_dot_s8_unaligned_Lunalign_scalar
	beqz a4, .Lnn_dot_s8_unaligned_Lunalign_done
	ee.ld.128.usar.ip q0, a3, 16
	addi a5, a4, -1
	beqz a5, .Lnn_dot_s8_unaligned_Lunalign_last
	nop.n
	loopgtz a5, .Lnn_dot_s8_unaligned_Lunalign_loop_end
	ee.vld.128.ip q1, a2, 16
	ee.ld.128.usar.ip q2, a3, 16
	ee.src.q.qup q4, q0, q2
	ee.vmulas.s8.accx q4, q1
.Lnn_dot_s8_unaligned_Lunalign_loop_end:
.Lnn_dot_s8_unaligned_Lunalign_last:
	ee.vld.128.ip q1, a2, 16
	beqz a6, .Lnn_dot_s8_unaligned_Lunalign_pass
	ee.ld.128.usar.ip q2, a3, 16
.Lnn_dot_s8_unaligned_Lunalign_pass:
	ee.src.q.qup q4, q0, q2
	ee.vmulas.s8.accx q4, q1
.Lnn_dot_s8_unaligned_Lunalign_done:
	movi.n a3, 0
	nop
	ee.srs.accx a2, a3, 0
	retw.n
.Lnn_dot_s8_unaligned_Lunalign_scalar:
	slli a4, a4, 4
	movi.n a7, 0
	beqz a4, .Lnn_dot_s8_unaligned_Lunalign_scalar_done
	loopgtz a4, .Lnn_dot_s8_unaligned_Lunalign_scalar_loop_end
	l8ui a5, a2, 0
	l8ui a6, a3, 0
	sext a5, a5, 7
	sext a6, a6, 7
	mull a5, a5, a6
	add a7, a7, a5
	addi.n a2, a2, 1
	addi.n a3, a3, 1
.Lnn_dot_s8_unaligned_Lunalign_scalar_loop_end:
.Lnn_dot_s8_unaligned_Lunalign_scalar_done:
	mov.n a2, a7
	retw.n
	.size tinygo_nn_dot_s8_unaligned, . - tinygo_nn_dot_s8_unaligned

// common/esp_nn_multiply_by_quantized_mult_esp32s3.S
.section .text.tinygo_nn_multiply_by_quantized_mult_asm,"ax",@progbits
	.global tinygo_nn_multiply_by_quantized_mult_asm
	.type tinygo_nn_multiply_by_quantized_mult_asm, @function
	.p2align 4
tinygo_nn_multiply_by_quantized_mult_asm:
	entry a1, 32
	wsr.sar a3
	ee.zero.q q2
	bltz a3, .Lnn_multiply_by_quantized_mult_asm_skip_left_shift
	ee.vsl.32 q0, q0
.Lnn_multiply_by_quantized_mult_asm_skip_left_shift:
	ssai 31
	ee.movi.32.a q0, a12, 0
	ee.movi.32.a q0, a13, 1
	ee.movi.32.a q0, a14, 2
	ee.movi.32.a q0, a15, 3
	movi a6, 1
	slli a6, a6, 30
	mulsh a4, a2, a12
	mulsh a11, a2, a13
	mulsh a10, a2, a14
	mulsh a8, a2, a15
	mull a12, a2, a12
	mull a13, a2, a13
	mull a14, a2, a14
	mull a15, a2, a15
	add.n a14, a6, a14
	.byte 0x60, 0x2e, 0x62 // saltu a2, a14, a6
	add.n a10, a10, a2
	add.n a13, a6, a13
	.byte 0x60, 0x9d, 0x62 // saltu a9, a13, a6
	add.n a11, a11, a9
	src a10, a10, a14
	src a11, a11, a13
	ee.movi.32.q q0, a10, 2
	ee.movi.32.q q0, a11, 1
	add.n a15, a6, a15
	.byte 0x60, 0x2f, 0x62 // saltu a2, a15, a6
	add.n a8, a8, a2
	add.n a12, a6, a12
	.byte 0x60, 0xac, 0x62 // saltu a10, a12, a6
	add.n a4, a4, a10
	src a8, a8, a15
	src a4, a4, a12
	ee.movi.32.q q0, a8, 3
	ee.movi.32.q q0, a4, 0
	bgez a3, .Lnn_multiply_by_quantized_mult_asm_skip_div_by_power_of_2
	neg a5, a3
	ee.vcmp.lt.s32 q2, q0, q2
	addi.n a7, a5, -1
	ssl a7
	movi.n a6, 1
	sll a6, a6
	s32i.n a6, a1, 4
	addi.n a4, a1, 4
	ee.vldbc.32 q1, a4
	wsr.sar a5
	ee.vadds.s32 q1, q1, q2
	ee.vadds.s32 q0, q0, q1
	ee.vsr.32 q0, q0
.Lnn_multiply_by_quantized_mult_asm_skip_div_by_power_of_2:
	retw.n
	.size tinygo_nn_multiply_by_quantized_mult_asm, . - tinygo_nn_multiply_by_quantized_mult_asm

// common/esp_nn_multiply_by_quantized_mult_ver1_esp32s3.S
.section .text.tinygo_nn_multiply_by_quantized_mult_ver1,"ax",@progbits
	.global tinygo_nn_multiply_by_quantized_mult_ver1
	.type tinygo_nn_multiply_by_quantized_mult_ver1, @function
	.p2align 4
tinygo_nn_multiply_by_quantized_mult_ver1:
	entry a1, 32
	ee.zero.q q3
	l32i.n a8, a3, 0
	l32i.n a7, a3, 4
	l32i.n a12, a2, 0
	l32i.n a15, a2, 4
	movi.n a10, 0
	max a6, a10, a8
	max a5, a10, a7
	sub a8, a6, a8
	sub a7, a5, a7
	ee.movi.32.a q0, a9, 0
	ee.movi.32.a q0, a11, 1
	ssl a6
	sll a9, a9
	mulsh a4, a12, a9
	mull a12, a12, a9
	ssl a5
	sll a11, a11
	mulsh a14, a15, a11
	mull a15, a15, a11
	movi a13, 1
	slli a13, a13, 30
	ee.movi.32.q q0, a9, 0
	ee.movi.32.q q0, a11, 1
	l32i.n a6, a3, 8
	l32i.n a9, a2, 8
	max a5, a10, a6
	sub a6, a5, a6
	ee.movi.32.a q0, a11, 2
	ssl a5
	sll a11, a11
	ee.movi.32.q q0, a11, 2
	mulsh a5, a9, a11
	mull a9, a9, a11
	mov a11, a5
	add.n a12, a13, a12
	.byte 0xd0, 0x5c, 0x62 // saltu a5, a12, a13
	add.n a15, a13, a15
	add.n a5, a5, a4
	.byte 0xd0, 0x4f, 0x62 // saltu a4, a15, a13
	add.n a4, a4, a14
	l32i.n a14, a3, 12
	add.n a9, a13, a9
	max a10, a10, a14
	sub a14, a10, a14
	ssl a10
	ee.movi.32.a q0, a10, 3
	sll a10, a10
	ssai 31
	src a5, a5, a12
	src a4, a4, a15
	movi.n a12, 1
	ee.movi.32.q q0, a5, 0
	.byte 0xd0, 0xf9, 0x62 // saltu a15, a9, a13
	add.n a15, a15, a11
	ee.movi.32.q q0, a4, 1
	l32i.n a11, a2, 12
	src a15, a15, a9
	ee.movi.32.q q0, a15, 2
	mull a9, a11, a10
	mulsh a11, a11, a10
	add.n a9, a13, a9
	.byte 0xd0, 0xd9, 0x62 // saltu a13, a9, a13
	add.n a13, a13, a11
	src a13, a13, a9
	ee.movi.32.q q0, a13, 3
	ssl a8
	sll a9, a12
	ssl a7
	addi.n a9, a9, -1
	ee.movi.32.q q2, a9, 0
	sll a11, a12
	addi.n a11, a11, -1
	ssl a6
	sll a10, a12
	ee.movi.32.q q2, a11, 1
	ssl a14
	addi.n a10, a10, -1
	ee.movi.32.q q2, a10, 2
	sll a9, a12
	addi.n a9, a9, -1
	ee.movi.32.q q2, a9, 3
	ee.andq q1, q0, q2
	ssr a8
	sra a5, a5
	ssr a7
	sra a4, a4
	ssr a6
	sra a15, a15
	ssr a14
	sra a13, a13
	wsr.sar a12
	ee.movi.32.q q7, a5, 0
	ee.movi.32.q q7, a4, 1
	ee.movi.32.q q7, a15, 2
	ee.movi.32.q q7, a13, 3
	ee.vcmp.lt.s32 q3, q7, q3
	ee.vsr.32 q2, q2
	ee.vsubs.s32 q2, q2, q3
	ee.vcmp.gt.s32 q1, q1, q2
	ee.vsubs.s32 q0, q7, q1
	retw.n
	.size tinygo_nn_multiply_by_quantized_mult_ver1, . - tinygo_nn_multiply_by_quantized_mult_ver1

// convolution/esp_nn_conv_s8_filter_aligned_input_padded_esp32s3.S
.section .text.tinygo_nn_conv_s8_filter_aligned_input_padded,"ax",@progbits
	.global tinygo_nn_conv_s8_filter_aligned_input_padded
	.type tinygo_nn_conv_s8_filter_aligned_input_padded, @function
	.p2align 4
tinygo_nn_conv_s8_filter_aligned_input_padded:
	entry sp, 80
	s32i.n a2, sp, 40
	mov a11, a6
	l16ui a2, sp, 88
	l32i a8, sp, 100
	l16ui a6, sp, 80
	mov.n a15, a5
	mull a4, a2, a15
	s32i.n a8, sp, 24
	movi.n a9, 0
	s32i.n a9, sp, 36
	addi.n a4, a4, 15
	srli a2, a4, 4
	slli a12, a2, 4
	mull a4, a6, a3
	mull a5, a3, a15
	l32i a10, sp, 112
	mull a9, a7, a15
	mull a4, a4, a15
	slli a3, a10, 2
	s32i.n a3, sp, 32
	s32i.n a5, sp, 12
	s32i.n a9, sp, 52
	s32i a4, sp, 56
	l32i a3, sp, 92
	l32i a13, sp, 136
	l32i a5, sp, 84
	mull a4, a12, a3
	srai a4, a4, 1
	addx4 a10, a10, a13
	l32i a3, sp, 96
	beqz a11, .Lnn_conv_s8_filter_aligned_input_padded_L_skip_acc_loop
.Lnn_conv_s8_filter_aligned_input_padded_L_acc_out_channel_loop:
	movi.n a9, 0
	loop a4, .Lnn_conv_s8_filter_aligned_input_padded_L_acc_filter_size_loop
	l8ui a14, a5, 0
	l8ui a7, a5, 1
	addi.n a5, a5, 2
	sext a14, a14, 7
	sext a7, a7, 7
	add a9, a9, a14
	add a9, a9, a7
.Lnn_conv_s8_filter_aligned_input_padded_L_acc_filter_size_loop:
	mull a9, a9, a11
	beqz.n a3, .Lnn_conv_s8_filter_aligned_input_padded_L_skip_bias
	l32i a8, a3, 0
	addi a3, a3, 4
	add a9, a9, a8
.Lnn_conv_s8_filter_aligned_input_padded_L_skip_bias:
	s32i a9, a13, 0
	addi.n a13, a13, 4
	blt a13, a10, .Lnn_conv_s8_filter_aligned_input_padded_L_acc_out_channel_loop
	j .Lnn_conv_s8_filter_aligned_input_padded_L_acc_done
.Lnn_conv_s8_filter_aligned_input_padded_L_skip_acc_loop:
	beqz.n a3, .Lnn_conv_s8_filter_aligned_input_padded_L_skip_acc_zero_bias
.Lnn_conv_s8_filter_aligned_input_padded_L_copy_bias_loop:
	l32i a8, a3, 0
	s32i a8, a13, 0
	addi a3, a3, 4
	addi.n a13, a13, 4
	blt a13, a10, .Lnn_conv_s8_filter_aligned_input_padded_L_copy_bias_loop
	j .Lnn_conv_s8_filter_aligned_input_padded_L_acc_done
.Lnn_conv_s8_filter_aligned_input_padded_L_skip_acc_zero_bias:
.Lnn_conv_s8_filter_aligned_input_padded_L_zero_scratch_loop:
	movi.n a8, 0
	s32i a8, a13, 0
	addi.n a13, a13, 4
	blt a13, a10, .Lnn_conv_s8_filter_aligned_input_padded_L_zero_scratch_loop
.Lnn_conv_s8_filter_aligned_input_padded_L_acc_done:
	movi.n a4, 0
.Lnn_conv_s8_filter_aligned_input_padded_L_height_loop:
	l32i.n a8, sp, 40
	movi.n a9, 0
	l32i a10, sp, 104
	s32i.n a8, sp, 28
	s32i.n a9, sp, 44
.Lnn_conv_s8_filter_aligned_input_padded_L_width_loop:
	movi.n a9, 0
	l32i a5, sp, 84
	s32i.n a9, sp, 20
	l32i a3, sp, 136
.Lnn_conv_s8_filter_aligned_input_padded_L_out_ch_loop:
	movi.n a6, 0
	l32i.n a9, sp, 28
	mov.n a10, a6
.Lnn_conv_s8_filter_aligned_input_padded_L_filter_ht_loop:
	add.n a8, a5, a12
	mov.n a13, a9
	ee.zero.accx
	ee.ld.128.usar.ip q0, a13, 16
	ee.vld.128.ip q4, a13, 16
	ee.vld.128.ip q1, a5, 16
	sub a15, a8, a5
	extui a14, a15, 4, 1
	srai a15, a15, 5
	ee.src.q.qup q2, q0, q4
	beqz a15, .Lnn_conv_s8_filter_aligned_input_padded_L_vector_32_loop_end
	loop a15, .Lnn_conv_s8_filter_aligned_input_padded_L_vector_32_loop_end
	ee.vld.128.ip q4, a13, 16
	ee.vmulas.s8.accx.ld.ip.qup q3, a5, 16, q2, q1, q0, q4
	ee.vld.128.ip q2, a13, 16
	ee.vmulas.s8.accx.ld.ip.qup q1, a5, 16, q0, q3, q4, q2
	ee.orq q0, q2, q2
	ee.orq q2, q4, q4
.Lnn_conv_s8_filter_aligned_input_padded_L_vector_32_loop_end:
	beqz a14, .Lnn_conv_s8_filter_aligned_input_padded_L_vector_loop_end
	ee.vmulas.s8.accx.ld.ip q4, a13, 16, q2, q1
	ee.src.q.ld.ip q1, a5, 16, q0, q4
	ee.orq q2, q0, q0
.Lnn_conv_s8_filter_aligned_input_padded_L_vector_loop_end:
	ee.vmulas.s8.accx q2, q1
	addi a13, a13, -16
	movi a15, 0
	ee.srs.accx a14, a15, 0
	mov.n a5, a8
	add.n a6, a6, a14
.Lnn_conv_s8_filter_aligned_input_padded_L7:
	l32i.n a8, sp, 12
	l32i a2, sp, 92
	addi.n a10, a10, 1
	add.n a9, a9, a8
	blt a10, a2, .Lnn_conv_s8_filter_aligned_input_padded_L_filter_ht_loop
.Lnn_conv_s8_filter_aligned_input_padded_L9:
	l32i a7, a3, 0
	addi a3, a3, 4
	l32i.n a8, sp, 20
	add.n a6, a6, a7
.Lnn_conv_s8_filter_aligned_input_padded_L_multiply_by_quant_mult:
	l32i a10, sp, 120
	l32i a9, sp, 124
	add.n a2, a10, a8
	l32i.n a2, a2, 0
	add.n a7, a9, a8
	l32i.n a7, a7, 0
	max a8, a2, a4
	ssl a8
	sll a6, a6
	mull a9, a6, a7
	movi a10, 1
	slli a10, a10, 30
	sub a2, a8, a2
	add.n a8, a9, a10
	mulsh a6, a6, a7
	movi.n a7, 1
	bltu a8, a9, .Lnn_conv_s8_filter_aligned_input_padded_L13
	movi.n a7, 0
.Lnn_conv_s8_filter_aligned_input_padded_L13:
	add.n a6, a7, a6
	slli a6, a6, 1
	extui a8, a8, 31, 1
	or a6, a6, a8
	beqz.n a2, .Lnn_conv_s8_filter_aligned_input_padded_L_skip_div_by_pow_of_2
	addi.n a7, a2, -1
	movi.n a9, 1
	extui a8, a6, 31, 1
	ssl a7
	sll a7, a9
	sub a7, a7, a8
	add.n a6, a7, a6
	ssr a2
	sra a6, a6
.Lnn_conv_s8_filter_aligned_input_padded_L_skip_div_by_pow_of_2:
	l32i a10, sp, 116
	l32i a8, sp, 128
	add.n a2, a10, a6
	l32i a9, sp, 132
	l32i.n a10, sp, 24
	max a2, a2, a8
	min a2, a2, a9
	s8i a2, a10, 0
	l32i.n a2, sp, 20
	addi.n a10, a10, 1
	addi.n a2, a2, 4
	l32i.n a6, sp, 32
	s32i.n a2, sp, 20
	s32i.n a10, sp, 24
	beq a6, a2, .Lnn_far_237
	j .Lnn_conv_s8_filter_aligned_input_padded_L_out_ch_loop
.Lnn_far_237:
.Lnn_conv_s8_filter_aligned_input_padded_L4:
	l32i.n a5, sp, 44
	l32i.n a6, sp, 28
	l32i.n a8, sp, 52
	addi.n a5, a5, 1
	add.n a6, a6, a8
	l32i a9, sp, 104
	s32i.n a5, sp, 44
	s32i.n a6, sp, 28
	beq a9, a5, .Lnn_far_236
	j .Lnn_conv_s8_filter_aligned_input_padded_L_width_loop
.Lnn_far_236:
	l32i.n a10, sp, 36
	l32i.n a2, sp, 40
	l32i a5, sp, 56
	l32i a6, sp, 108
	addi.n a10, a10, 1
	add.n a2, a2, a5
	s32i.n a10, sp, 36
	s32i.n a2, sp, 40
	bge a10, a6, .Lnn_far_235
	j .Lnn_conv_s8_filter_aligned_input_padded_L_height_loop
.Lnn_far_235:
	retw.n
	.size tinygo_nn_conv_s8_filter_aligned_input_padded, . - tinygo_nn_conv_s8_filter_aligned_input_padded

// convolution/esp_nn_conv_s8_mult8_1x1_esp32s3.S
.section .text.tinygo_nn_conv_s8_mult8_1x1,"ax",@progbits
	.global tinygo_nn_conv_s8_mult8_1x1
	.type tinygo_nn_conv_s8_mult8_1x1, @function
	.p2align 4
tinygo_nn_conv_s8_mult8_1x1:
	entry a1, 160
	s32i a5, a1, 44
	s32i a6, a1, 64
	s32i a7, a1, 60
	mul16u a8, a3, a4
	s32i a2, a1, 56
	l32i a4, a1, 164
	mov.n a3, a1
	s32i a8, a1, 84
	bgei a8, 8, .Lnn_far_234
	j .Lnn_conv_s8_mult8_1x1_prepare_leftover
.Lnn_far_234:
	addi a9, a8, -7
	s32i a9, a1, 80
	s32i a2, a1, 68
	srai a15, a5, 3
	movi.n a11, 0
	s32i a11, a1, 40
	addi a15, a15, -1
	s32i a15, a1, 48
	slli a9, a5, 3
	s32i a9, a1, 76
	addi a15, a5, -7
	s32i a15, a1, 52
.Lnn_conv_s8_mult8_1x1_outer_loop:
	l32i a10, a1, 200
	l32i.n a11, a1, 44
	l32i a8, a1, 68
	srai a9, a11, 3
	ee.zero.q q7
	addi a12, a1, 64
	ee.vldbc.16 q5, a12
	loopgtz a9, .Lnn_conv_s8_mult8_1x1_transpose_loop_end
	mov.n a9, a8
	ee.vld.l.64.xp q0, a9, a11
	ee.vld.l.64.xp q1, a9, a11
	ee.vld.h.64.xp q0, a9, a11
	ee.vld.h.64.xp q1, a9, a11
	ee.vld.l.64.xp q2, a9, a11
	ee.vzip.8 q0, q1
	ee.vld.l.64.xp q3, a9, a11
	ee.vld.h.64.xp q2, a9, a11
	ee.vld.h.64.ip q3, a9, 0
	ee.vzip.16 q0, q1
	ee.vzip.8 q2, q3
	ee.vzip.16 q2, q3
	ee.vzip.32 q0, q2
	ee.vcmp.lt.s8 q4, q2, q7
	ee.vzip.8 q2, q4
	ee.vcmp.lt.s8 q6, q0, q7
	ee.vzip.8 q0, q6
	ee.vadds.s16 q0, q0, q5
	ee.vadds.s16.st.incp q0, a10, q6, q6, q5
	ee.vadds.s16.st.incp q6, a10, q2, q2, q5
	ee.vadds.s16.st.incp q2, a10, q4, q4, q5
	ee.vst.128.ip q4, a10, 16
	ee.vzip.32 q1, q3
	ee.vcmp.lt.s8 q4, q3, q7
	ee.vzip.8 q3, q4
	ee.vcmp.lt.s8 q6, q1, q7
	ee.vzip.8 q1, q6
	ee.vadds.s16 q1, q1, q5
	ee.vadds.s16.st.incp q1, a10, q6, q6, q5
	ee.vadds.s16.st.incp q6, a10, q3, q3, q5
	ee.vadds.s16.st.incp q3, a10, q4, q4, q5
	ee.vst.128.ip q4, a10, 16
	addi.n a8, a8, 8
.Lnn_conv_s8_mult8_1x1_transpose_loop_end:
	l32i a6, a1, 184
	l32i a2, a1, 188
	l32i a5, a1, 60
	l32i a9, a1, 160
	l16ui a8, a1, 176
	s32i a9, a1, 72
	bgei a8, 1, .Lnn_far_233
	j .Lnn_conv_s8_mult8_1x1_outer_ch_loop_end
.Lnn_far_233:
	movi.n a7, 0
.Lnn_conv_s8_mult8_1x1_out_ch_loop:
	l32i a8, a1, 200
	ee.zero.qacc
	ee.zero.q q5
	l32i a10, a1, 52
	l32i a9, a1, 48
	ee.ld.128.usar.ip q7, a5, 16
	ee.ld.128.usar.ip q6, a5, 0
	addi a5, a5, -8
	ee.src.q q7, q7, q6
	ee.vld.128.ip q0, a8, 16
	ee.vld.128.ip q1, a8, 16
	ee.vcmp.lt.s8 q6, q7, q5
	ee.vzip.8 q7, q6
	ee.vsmulas.s16.qacc.ld.incp q2, a8, q0, q7, 0
	ee.vsmulas.s16.qacc.ld.incp q3, a8, q1, q7, 1
	ee.vsmulas.s16.qacc.ld.incp q0, a8, q2, q7, 2
	ee.vsmulas.s16.qacc.ld.incp q1, a8, q3, q7, 3
	ee.vsmulas.s16.qacc.ld.incp q2, a8, q0, q7, 4
	ee.vsmulas.s16.qacc.ld.incp q3, a8, q1, q7, 5
	blti a10, 8, .Lnn_conv_s8_mult8_1x1_inner_loop_end
	loopgtz a9, .Lnn_conv_s8_mult8_1x1_inner_loop_end
	ee.vsmulas.s16.qacc.ld.incp q0, a8, q2, q7, 6
	ee.vsmulas.s16.qacc.ld.incp q1, a8, q3, q7, 7
	ee.ld.128.usar.ip q7, a5, 16
	ee.ld.128.usar.ip q6, a5, 0
	addi a5, a5, -8
	ee.src.q q7, q7, q6
	ee.vcmp.lt.s8 q6, q7, q5
	ee.vzip.8 q7, q6
	ee.vsmulas.s16.qacc.ld.incp q2, a8, q0, q7, 0
	ee.vsmulas.s16.qacc.ld.incp q3, a8, q1, q7, 1
	ee.vsmulas.s16.qacc.ld.incp q0, a8, q2, q7, 2
	ee.vsmulas.s16.qacc.ld.incp q1, a8, q3, q7, 3
	ee.vsmulas.s16.qacc.ld.incp q2, a8, q0, q7, 4
	ee.vsmulas.s16.qacc.ld.incp q3, a8, q1, q7, 5
.Lnn_conv_s8_mult8_1x1_inner_loop_end:
	ee.vsmulas.s16.qacc q2, q7, 6
	ee.vsmulas.s16.qacc q3, q7, 7
	ee.st.qacc_l.l.128.ip a3, 16
	ee.st.qacc_l.h.32.ip a3, -16
	l32i.n a10, a1, 0
	addi a11, a1, 5
	l32i a11, a11, 0
	addi a12, a1, 10
	l32i a12, a12, 0
	addi a13, a1, 15
	l32i a13, a13, 0
	ee.movi.32.q q0, a10, 0
	ee.movi.32.q q0, a11, 1
	ee.movi.32.q q0, a12, 2
	ee.movi.32.q q0, a13, 3
	ee.st.qacc_h.l.128.ip a3, 16
	ee.st.qacc_h.h.32.ip a3, -16
	l32i.n a10, a1, 0
	addi a11, a1, 5
	l32i a11, a11, 0
	addi a12, a1, 10
	l32i a12, a12, 0
	addi a13, a1, 15
	l32i a13, a13, 0
	ee.movi.32.q q4, a10, 0
	ee.movi.32.q q4, a11, 1
	ee.movi.32.q q4, a12, 2
	ee.movi.32.q q4, a13, 3
	l32i a9, a1, 160
	l32i a10, a1, 72
	beqz.n a9, .Lnn_conv_s8_mult8_1x1_no_bias
	ee.vldbc.32.ip q6, a10, 4
	s32i a10, a1, 72
	ee.vadds.s32 q0, q0, q6
	ee.vadds.s32 q4, q4, q6
.Lnn_conv_s8_mult8_1x1_no_bias:
	l32i.n a11, a6, 0
	l32i.n a10, a2, 0
	call8 tinygo_nn_multiply_by_quantized_mult_asm
	l32i.n a10, a2, 0
	l32i.n a11, a6, 0
	mv.qr q5, q0
	mv.qr q0, q4
	call8 tinygo_nn_multiply_by_quantized_mult_asm
	addi.n a6, a6, 4
	addi.n a2, a2, 4
	addi a9, a1, 180
	addi a10, a1, 192
	addi a8, a1, 196
	ee.vldbc.32 q4, a9
	ee.vldbc.32 q2, a10
	ee.vldbc.32 q3, a8
	ee.vadds.s32 q1, q0, q4
	ee.vadds.s32 q0, q5, q4
	ee.vmin.s32 q1, q1, q3
	ee.vmax.s32 q1, q1, q2
	ee.vmin.s32 q0, q0, q3
	ee.vmax.s32 q0, q0, q2
	l16ui a9, a1, 176
	ee.vunzip.16 q0, q1
	ee.vst.128.ip q0, a3, 0
	l8ui a14, a1, 0
	l8ui a11, a1, 2
	add a10, a4, a9
	s8i a14, a4, 0
	s8i a11, a10, 0
	l8ui a14, a1, 4
	l8ui a11, a1, 6
	add a12, a10, a9
	add a10, a12, a9
	s8i a14, a12, 0
	s8i a11, a10, 0
	l8ui a14, a1, 8
	l8ui a11, a1, 10
	add a12, a10, a9
	add a10, a12, a9
	s8i a14, a12, 0
	s8i a11, a10, 0
	l8ui a14, a1, 12
	l8ui a11, a1, 14
	add a12, a10, a9
	add a10, a12, a9
	s8i a14, a12, 0
	s8i a11, a10, 0
	addi.n a4, a4, 1
	addi.n a7, a7, 1
	beq a7, a9, .Lnn_far_232
	j .Lnn_conv_s8_mult8_1x1_out_ch_loop
.Lnn_far_232:
.Lnn_conv_s8_mult8_1x1_outer_ch_loop_end:
	subx8 a11, a9, a9
	l32i a10, a1, 76
	l32i a15, a1, 40
	l32i a9, a1, 68
	l32i a8, a1, 80
	add.n a4, a4, a11
	addi.n a15, a15, 8
	s32i a15, a1, 40
	add.n a9, a9, a10
	s32i a9, a1, 68
	bge a15, a8, .Lnn_far_231
	j .Lnn_conv_s8_mult8_1x1_outer_loop
.Lnn_far_231:
	l32i a15, a1, 40
	l32i a13, a1, 84
	l32i a8, a1, 44
	blt a15, a13, .Lnn_far_230
	j .Lnn_conv_s8_mult8_1x1_return_function
.Lnn_far_230:
.Lnn_conv_s8_mult8_1x1_process_leftover:
	l32i a15, a1, 40
	l32i a14, a1, 56
	mull a15, a15, a8
	addi.n a8, a8, -1
	add.n a14, a14, a15
	srai a8, a8, 3
	s32i a8, a1, 36
	s32i a14, a1, 68
	addi a12, a1, 64
	ee.vldbc.16 q4, a12
.Lnn_conv_s8_mult8_1x1_leftover_outer_loop:
	l32i a15, a1, 184
	l32i a2, a1, 188
	l32i a8, a1, 60
	l32i a5, a1, 160
	movi.n a11, 0
.Lnn_conv_s8_mult8_1x1_leftover_out_ch_loop:
	ee.zero.qacc
	ee.zero.q q3
	l32i a9, a1, 68
	l32i a10, a1, 36
	ee.vld.l.64.ip q0, a9, 8
	ee.ld.128.usar.ip q1, a8, 16
	ee.ld.128.usar.ip q7, a8, 0
	addi a8, a8, -8
	ee.src.q q1, q1, q7
	ee.vcmp.lt.s8 q6, q0, q3
	ee.vcmp.lt.s8 q7, q1, q3
	ee.vzip.8 q0, q6
	ee.vzip.8 q1, q7
	ee.vadds.s16 q0, q0, q4
	nop.n
	loopgtz a10, .Lnn_conv_s8_mult8_1x1_leftover_inner_loop_end
	ee.vmulas.s16.qacc q0, q1
	ee.vld.l.64.ip q0, a9, 8
	ee.ld.128.usar.ip q1, a8, 16
	ee.ld.128.usar.ip q7, a8, 0
	addi a8, a8, -8
	ee.src.q q1, q1, q7
	ee.vcmp.lt.s8 q2, q0, q3
	ee.vcmp.lt.s8 q7, q1, q3
	ee.vzip.8 q0, q2
	ee.vzip.8 q1, q7
	ee.vadds.s16 q0, q0, q4
.Lnn_conv_s8_mult8_1x1_leftover_inner_loop_end:
	ee.vmulas.s16.qacc q0, q1
	ee.st.qacc_l.l.128.ip a3, 16
	ee.st.qacc_l.h.32.ip a3, 0
	l8ui a10, a1, 5
	l8ui a12, a1, 6
	l16ui a14, a1, 10
	l8ui a9, a1, 15
	l8ui a13, a1, 16
	s8i a10, a1, 2
	s8i a12, a1, 3
	s16i a14, a1, 4
	s8i a9, a1, 6
	s8i a13, a1, 7
	ee.st.qacc_h.l.128.ip a3, 16
	ee.st.qacc_h.h.32.ip a3, -32
	l16ui a13, a1, 16
	l8ui a14, a1, 21
	l8ui a9, a1, 22
	l16ui a10, a1, 26
	s16i a13, a1, 8
	l8ui a12, a1, 31
	l8ui a13, a1, 32
	s8i a14, a1, 10
	s8i a9, a1, 11
	s16i a10, a1, 12
	s8i a12, a1, 14
	s8i a13, a1, 15
	movi.n a12, 16
	ee.vld.128.ip q0, a3, 0
	ee.srcmb.s16.qacc q1, a12, 0
	ee.vzip.16 q0, q1
	ee.vadds.s32 q0, q0, q1
	ee.movi.32.a q0, a10, 3
	ee.movi.32.a q0, a9, 2
	ee.movi.32.a q0, a14, 0
	add a9, a9, a10
	ee.movi.32.a q0, a10, 1
	add a14, a14, a10
	add a14, a14, a9
	l32i a9, a1, 160
	l32i.n a6, a15, 0
	beqz.n a9, .Lnn_conv_s8_mult8_1x1_leftover_multiply_by_quant_mult
	l32i.n a9, a5, 0
	add.n a14, a14, a9
.Lnn_conv_s8_mult8_1x1_leftover_multiply_by_quant_mult:
	l32i.n a9, a2, 0
	movi.n a10, 0
	max a10, a6, a10
	ssl a10
	sll a14, a14
	sub a7, a10, a6
	movi a13, 1
	slli a13, a13, 30
	mulsh a12, a9, a14
	mull a14, a9, a14
	ssai 31
	addi.n a2, a2, 4
	addi.n a15, a15, 4
	addi.n a5, a5, 4
	addi.n a11, a11, 1
	add a13, a14, a13
	.byte 0xe0, 0xed, 0x62 // saltu a14, a13, a14
	add a9, a12, a14
	src a12, a9, a13
	blti a7, 1, .Lnn_conv_s8_mult8_1x1_leftover_skip_div_by2
	addi.n a14, a7, -1
	ssl a14
	movi.n a10, 1
	sll a10, a10
	extui a14, a12, 31, 1
	ssr a7
	sub a10, a10, a14
	add a12, a12, a10
	sra a12, a12
.Lnn_conv_s8_mult8_1x1_leftover_skip_div_by2:
	l32i a10, a1, 180
	l32i a9, a1, 192
	l16ui a13, a1, 176
	l32i a14, a1, 196
	add.n a10, a10, a12
	max a9, a9, a10
	min a14, a14, a9
	s8i a14, a4, 0
	addi.n a4, a4, 1
	beq a11, a13, .Lnn_far_229
	j .Lnn_conv_s8_mult8_1x1_leftover_out_ch_loop
.Lnn_far_229:
	l32i a15, a1, 44
	l32i a14, a1, 68
	l32i a13, a1, 40
	l32i a12, a1, 84
	addi.n a13, a13, 1
	s32i a13, a1, 40
	add a14, a14, a15
	s32i a14, a1, 68
	bge a13, a12, .Lnn_far_228
	j .Lnn_conv_s8_mult8_1x1_leftover_outer_loop
.Lnn_far_228:
.Lnn_conv_s8_mult8_1x1_return_function:
	retw.n
.Lnn_conv_s8_mult8_1x1_prepare_leftover:
	l32i a8, a1, 44
	movi.n a15, 0
	s32i a15, a1, 40
	j .Lnn_conv_s8_mult8_1x1_process_leftover
	.size tinygo_nn_conv_s8_mult8_1x1, . - tinygo_nn_conv_s8_mult8_1x1

// convolution/esp_nn_depthwise_conv_s16_mult1_3x3_esp32s3.S
.section .text.tinygo_nn_depthwise_conv_s16_mult1_3x3,"ax",@progbits
	.global tinygo_nn_depthwise_conv_s16_mult1_3x3
	.type tinygo_nn_depthwise_conv_s16_mult1_3x3, @function
	.p2align 4
tinygo_nn_depthwise_conv_s16_mult1_3x3:
	entry a1, 288
	s32i a2, a1, 104
	s32i a3, a1, 112
	s32i a5, a1, 116
	s32i.n a6, a1, 56
	addi a14, a1, 112
	addmi a11, a1, 256
	addmi a13, a1, 256
	addmi a15, a1, 256
	l32i a9, a1, 304
	l16ui a8, a1, 312
	s32i a8, a1, 64
	s32i a9, a1, 156
	addi a15, a15, 60
	addi a13, a13, 72
	addi a11, a11, 76
	ee.vldbc.32 q0, a11
	ee.vldbc.32 q1, a13
	ee.vldbc.32 q2, a15
	.byte 0xe4, 0x65, 0xdd // st.qr q2, a14, 80
	.byte 0xe4, 0xe6, 0xcd // st.qr q1, a14, 96
	.byte 0xe4, 0x67, 0xcd // st.qr q0, a14, 112
	beqz a8, .Lnn_depthwise_conv_s16_mult1_3x3_Lt_5_7426
.Lnn_depthwise_conv_s16_mult1_3x3_LBB3_esp_nn_depthwise_conv_s16_mult1_3x3:
	s32i a1, a1, 160
	s32i a7, a1, 72
	mul16u a6, a3, a5
	l32i a14, a1, 296
	l32i a15, a1, 300
	l16ui a9, a1, 308
	l16ui a13, a1, 288
	neg a8, a7
	l16ui a10, a1, 292
	l32i a11, a1, 324
	l32i a12, a1, 320
	s32i a12, a1, 84
	s32i a11, a1, 88
	s32i.n a10, a1, 60
	s32i a8, a1, 124
	s32i a13, a1, 80
	s32i a9, a1, 92
	s32i a15, a1, 140
	s32i a14, a1, 108
	slli a6, a6, 1
	movi.n a14, 16
	extui a15, a15, 0, 4
	addi a9, a5, -7
	movi.n a13, 0
	sub a8, a4, a8
	addx2 a7, a5, a5
	slli a7, a7, 1
	slli a4, a5, 1
	s32i a13, a1, 68
	s32i a9, a1, 144
	s32i a15, a1, 132
	l32i.n a9, a1, 56
	s32i a8, a1, 76
	neg a9, a9
	s32i.n a9, a1, 48
	sub a8, a3, a9
	s32i.n a8, a1, 52
.Lnn_depthwise_conv_s16_mult1_3x3_Lt_5_7938:
	l32i a10, a1, 92
	beqz a10, .Lnn_depthwise_conv_s16_mult1_3x3_Lt_5_8194
.Lnn_depthwise_conv_s16_mult1_3x3_LBB6_esp_nn_depthwise_conv_s16_mult1_3x3:
	l32i.n a5, a1, 52
	l32i a11, a1, 76
	movi.n a13, 0
	l32i a12, a1, 72
	movi.n a15, 0
	l32i.n a8, a1, 48
	l32i.n a9, a1, 56
	s32i a9, a1, 100
	s32i a8, a1, 128
	s32i a15, a1, 96
	max a12, a12, a13
	s32i a12, a1, 152
	movi.n a13, 3
	min a11, a11, a13
	s32i a11, a1, 136
	sub a11, a11, a12
	s32i a11, a1, 120
.Lnn_depthwise_conv_s16_mult1_3x3_Lt_5_8706:
	l32i a2, a1, 84
	l32i a10, a1, 144
	l32i a11, a1, 140
	l32i a12, a1, 88
	s32i a12, a1, 168
	s32i a11, a1, 148
	bgei a10, 1, .Lnn_far_225
	j .Lnn_depthwise_conv_s16_mult1_3x3_Lt_5_8962
.Lnn_far_225:
	movi.n a8, 0
	movi.n a13, 0
	l32i a3, a1, 100
	s32i a13, a1, 164
	max a3, a3, a8
.Lnn_depthwise_conv_s16_mult1_3x3_Lt_5_9474:
	l32i a10, a1, 136
	l32i a9, a1, 152
	ee.zero.qacc
	bge a9, a10, .Lnn_depthwise_conv_s16_mult1_3x3_Lt_5_9730
.Lnn_depthwise_conv_s16_mult1_3x3_LBB12_esp_nn_depthwise_conv_s16_mult1_3x3:
	l32i a12, a1, 128
	l32i a15, a1, 112
	l32i a10, a1, 116
	l32i a13, a1, 124
	mull a11, a9, a10
	add.n a13, a13, a9
	mull a13, a13, a15
	addx2 a11, a11, a11
	l32i a9, a1, 164
	add.n a12, a12, a13
	mull a10, a10, a12
	add.n a11, a9, a11
	l32i a12, a1, 108
	add.n a9, a9, a10
	l32i a10, a1, 104
	addx2 a11, a11, a12
	l32i a12, a1, 120
	addx2 a9, a9, a10
	loopgtz a12, .Lnn_depthwise_conv_s16_mult1_3x3_LBB32_esp_nn_depthwise_conv_s16_mult1_3x3
	mov.n a13, a9
	mov.n a12, a11
	mov.n a9, a11
	mov.n a11, a13
	beqz.n a3, .Lnn_depthwise_conv_s16_mult1_3x3_Lt_5_10498
	add.n a11, a4, a13
	add.n a9, a4, a12
.Lnn_depthwise_conv_s16_mult1_3x3_Lt_5_10498:
	ee.vld.128.xp q0, a11, a4
	ee.vld.128.xp q1, a9, a4
	bnez.n a3, .Lnn_depthwise_conv_s16_mult1_3x3_Lt_5_11010
	ee.vmulas.s16.qacc q0, q1
	ee.vld.128.xp q0, a11, a4
	ee.vld.128.xp q1, a9, a4
.Lnn_depthwise_conv_s16_mult1_3x3_Lt_5_11010:
	ee.vmulas.s16.qacc q0, q1
	ee.vld.128.xp q0, a11, a4
	ee.vld.128.xp q1, a9, a4
	add.n a9, a6, a13
	blti a5, 3, .Lnn_depthwise_conv_s16_mult1_3x3_Lt_5_11522
	ee.vmulas.s16.qacc q0, q1
.Lnn_depthwise_conv_s16_mult1_3x3_Lt_5_11522:
	add.n a11, a7, a12
.Lnn_depthwise_conv_s16_mult1_3x3_LBB32_esp_nn_depthwise_conv_s16_mult1_3x3:
.Lnn_depthwise_conv_s16_mult1_3x3_Lt_5_9730:
	l32i a9, a1, 160
	ee.st.qacc_l.l.128.ip a9, 16
	ee.st.qacc_l.h.32.ip a9, 0
	l8ui a11, a1, 15
	l16ui a10, a1, 10
	l8ui a15, a1, 16
	l8ui a13, a1, 6
	l8ui a12, a1, 5
	s8i a12, a1, 2
	s8i a13, a1, 3
	s8i a15, a1, 7
	s16i a10, a1, 4
	s8i a11, a1, 6
	ee.st.qacc_h.l.128.ip a9, 16
	ee.st.qacc_h.h.32.ip a9, -32
	ee.srcmb.s16.qacc q1, a14, 0
	l8ui a15, a1, 31
	l8ui a8, a1, 32
	l16ui a13, a1, 26
	l8ui a12, a1, 22
	l8ui a11, a1, 21
	l16ui a10, a1, 16
	s16i a10, a1, 8
	s8i a11, a1, 10
	s8i a12, a1, 11
	s16i a13, a1, 12
	s8i a8, a1, 15
	s8i a15, a1, 14
	l32i a8, a1, 140
	ee.vld.128.ip q0, a9, 0
	s32i a9, a1, 160
	ee.vzip.16 q0, q1
	beqz.n a8, .Lnn_depthwise_conv_s16_mult1_3x3_Lt_5_12290
	addi a8, a1, 112
	l32i a10, a1, 132
	l32i a9, a1, 148
	wur.sar_byte a10
	ee.vld.128.ip q4, a9, 16
	ee.vld.128.ip q7, a9, 16
	ee.vld.128.ip q5, a9, 0
	s32i a9, a1, 148
	ee.src.q.qup q6, q4, q7
	ee.vadds.s32 q0, q0, q6
	ee.src.q.qup q3, q4, q5
	ee.vadds.s32 q1, q1, q3
	.byte 0x84, 0xe4, 0xcd // st.qr q1, a8, 64
.Lnn_depthwise_conv_s16_mult1_3x3_Lt_5_12290:
	addi a11, a1, 112
	l32i a10, a1, 168
	.byte 0xb4, 0xe4, 0xcd // st.qr q1, a11, 64
	mov.n a11, a2
	call8 tinygo_nn_multiply_by_quantized_mult_ver1
	l32i a10, a1, 168
	addmi a12, a1, 256
	addi a11, a1, 112
	.byte 0xc4, 0x6f, 0xcd // st.qr q0, a12, -16
	.byte 0xb4, 0x24, 0xcd // ld.qr q0, a11, 64
	addi a10, a10, 16
	addi a11, a2, 16
	call8 tinygo_nn_multiply_by_quantized_mult_ver1
.Lnn_depthwise_conv_s16_mult1_3x3_LBB25_esp_nn_depthwise_conv_s16_mult1_3x3:
	movi.n a14, 16
	addi a2, a2, 32
	l32i a15, a1, 144
	l32i a9, a1, 156
	l32i a8, a1, 168
	addmi a12, a1, 256
	addi a13, a1, 112
	.byte 0xd4, 0xa7, 0xdd // ld.qr q3, a13, 112
	.byte 0xd4, 0xa5, 0xcd // ld.qr q1, a13, 80
	.byte 0xc4, 0x2f, 0xdd // ld.qr q2, a12, -16
	addi a8, a8, 32
	s32i a8, a1, 168
	ee.vadds.s32 q2, q2, q1
	ee.vadds.s32 q1, q0, q1
	ee.vmin.s32 q0, q2, q3
	ee.vmin.s32 q1, q1, q3
	.byte 0xd4, 0x26, 0xdd // ld.qr q2, a13, 96
	l32i a13, a1, 164
	ee.vmax.s32 q1, q1, q2
	ee.vmax.s32 q0, q0, q2
	addi.n a13, a13, 8
	s32i a13, a1, 164
	ee.vunzip.16 q0, q1
	ee.vunzip.8 q0, q1
	ee.vst.l.64.ip q0, a9, 8
	s32i a9, a1, 156
	bge a13, a15, .Lnn_far_224
	j .Lnn_depthwise_conv_s16_mult1_3x3_Lt_5_9474
.Lnn_far_224:
.Lnn_depthwise_conv_s16_mult1_3x3_Lt_5_8962:
	l32i a8, a1, 92
	l32i a11, a1, 100
	l32i a10, a1, 128
	l32i a9, a1, 80
	l32i a15, a1, 96
	sub a5, a5, a9
	addi.n a15, a15, 1
	s32i a15, a1, 96
	add.n a10, a10, a9
	sub a11, a11, a9
	s32i a11, a1, 100
	s32i a10, a1, 128
	sub a15, a15, a8
	bnez a15, .Lnn_depthwise_conv_s16_mult1_3x3_Lt_5_8706
.Lnn_depthwise_conv_s16_mult1_3x3_Lt_5_8194:
	l32i a13, a1, 64
	l32i a10, a1, 72
	l32i a9, a1, 124
	l32i.n a8, a1, 60
	l32i a12, a1, 68
	l32i a15, a1, 76
	addi.n a12, a12, 1
	s32i a12, a1, 68
	sub a15, a15, a8
	add.n a9, a9, a8
	sub a10, a10, a8
	s32i a10, a1, 72
	s32i a9, a1, 124
	s32i a15, a1, 76
	sub a12, a12, a13
	bnez a12, .Lnn_depthwise_conv_s16_mult1_3x3_Lt_5_7938
.Lnn_depthwise_conv_s16_mult1_3x3_Lt_5_7426:
	retw.n
	.size tinygo_nn_depthwise_conv_s16_mult1_3x3, . - tinygo_nn_depthwise_conv_s16_mult1_3x3

// convolution/esp_nn_depthwise_conv_s16_mult1_esp32s3.S
.section .text.tinygo_nn_depthwise_conv_s16_mult1,"ax",@progbits
	.global tinygo_nn_depthwise_conv_s16_mult1
	.type tinygo_nn_depthwise_conv_s16_mult1, @function
	.p2align 4
tinygo_nn_depthwise_conv_s16_mult1:
	entry a1, 160
	l32i a9, a1, 184
	l16ui a8, a1, 192
	s32i a2, a1, 52
	s32i.n a4, a1, 56
	s32i a5, a1, 60
	s32i a9, a1, 112
	beqz a8, .Lnn_depthwise_conv_s16_mult1_Lt_4_7170
.Lnn_depthwise_conv_s16_mult1_LBB3_esp_nn_depthwise_conv_s16_mult1:
	l16ui a4, a1, 172
	neg a13, a7
	neg a12, a6
	sext a12, a12, 15
	sext a13, a13, 15
	s32i a13, a1, 92
	s32i.n a12, a1, 48
	movi.n a8, 0
	slli a9, a5, 1
	addi a10, a5, -7
	s32i a10, a1, 100
	s32i a9, a1, 64
	s32i a8, a1, 68
	j .Lnn_depthwise_conv_s16_mult1_Lt_4_7682
.Lnn_depthwise_conv_s16_mult1_Lt_4_7938:
	l32i a15, a1, 192
	l32i a9, a1, 164
	l32i a14, a1, 68
	l32i a8, a1, 92
	addi.n a14, a14, 1
	s32i a14, a1, 68
	add.n a9, a8, a9
	sub a14, a14, a15
	sext a8, a9, 15
	s32i a8, a1, 92
	beqz a14, .Lnn_depthwise_conv_s16_mult1_Lt_4_7170
.Lnn_depthwise_conv_s16_mult1_Lt_4_7682:
	l32i a10, a1, 188
	beqz a10, .Lnn_depthwise_conv_s16_mult1_Lt_4_7938
.Lnn_depthwise_conv_s16_mult1_LBB6_esp_nn_depthwise_conv_s16_mult1:
	movi.n a14, 0
	l32i a7, a1, 176
	l32i a13, a1, 92
	l32i.n a8, a1, 56
	movi.n a11, 0
	l32i.n a12, a1, 48
	s32i a12, a1, 84
	s32i a11, a1, 88
	sub a8, a8, a13
	min a7, a7, a8
	neg a13, a13
	max a13, a13, a14
	s32i a13, a1, 96
	j .Lnn_depthwise_conv_s16_mult1_Lt_4_8450
.Lnn_depthwise_conv_s16_mult1_Lt_4_8706:
	l32i a10, a1, 188
	l32i a12, a1, 160
	l32i a9, a1, 88
	l32i a11, a1, 84
	addi.n a9, a9, 1
	s32i a9, a1, 88
	add.n a12, a11, a12
	sext a11, a12, 15
	s32i a11, a1, 84
	beq a9, a10, .Lnn_depthwise_conv_s16_mult1_Lt_4_7938
.Lnn_depthwise_conv_s16_mult1_Lt_4_8450:
	l32i a13, a1, 100
	l32i a14, a1, 180
	l32i a15, a1, 204
	l32i a8, a1, 200
	s32i a8, a1, 104
	s32i a15, a1, 116
	s32i a14, a1, 108
	blti a13, 1, .Lnn_depthwise_conv_s16_mult1_Lt_4_8706
.Lnn_depthwise_conv_s16_mult1_LBB9_esp_nn_depthwise_conv_s16_mult1:
	movi.n a2, 0
	l32i a5, a1, 84
	movi.n a8, 0
	neg a6, a5
	max a6, a6, a8
	sub a5, a3, a5
	min a5, a4, a5
	sub a9, a5, a6
	s32i a9, a1, 72
	j .Lnn_depthwise_conv_s16_mult1_Lt_4_9218
.Lnn_depthwise_conv_s16_mult1_Lt_4_9474:
	mov a11, a1
	ee.st.qacc_l.l.128.ip a11, 16
	ee.st.qacc_l.h.32.ip a11, 0
	l8ui a12, a1, 15
	l16ui a10, a1, 10
	l8ui a13, a1, 5
	l8ui a14, a1, 6
	l8ui a15, a1, 16
	s8i a13, a1, 2
	s8i a14, a1, 3
	s8i a15, a1, 7
	s16i a10, a1, 4
	s8i a12, a1, 6
	movi.n a10, 16
	ee.st.qacc_h.l.128.ip a11, 16
	ee.st.qacc_h.h.32.ip a11, -32
	ee.srcmb.s16.qacc q1, a10, 0
	l8ui a8, a1, 31
	l8ui a9, a1, 32
	l16ui a12, a1, 16
	l8ui a13, a1, 21
	l8ui a14, a1, 22
	l16ui a15, a1, 26
	s8i a13, a1, 10
	s8i a14, a1, 11
	s16i a15, a1, 12
	s16i a12, a1, 8
	s8i a9, a1, 15
	s8i a8, a1, 14
	l32i a9, a1, 180
	ee.vld.128.ip q0, a11, 0
	ee.vzip.16 q0, q1
	beqz.n a9, .Lnn_depthwise_conv_s16_mult1_Lt_4_11522
	l32i a9, a1, 108
	addi a8, a1, 112
	extui a10, a9, 0, 4
	wur.sar_byte a10
	ee.vld.128.ip q4, a9, 16
	ee.vld.128.ip q7, a9, 16
	ee.vld.128.ip q5, a9, 0
	s32i a9, a1, 108
	ee.src.q q4, q4, q7
	ee.src.q q7, q7, q5
	ee.vadds.s32 q0, q0, q4
	ee.vadds.s32 q1, q1, q7
	.byte 0x14, 0xe0, 0xcd // st.qr q1, a1, 0
.Lnn_depthwise_conv_s16_mult1_Lt_4_11522:
	l32i a10, a1, 116
	l32i a11, a1, 104
	.byte 0x14, 0xe0, 0xcd // st.qr q1, a1, 0
	call8 tinygo_nn_multiply_by_quantized_mult_ver1
	l32i a10, a1, 116
	l32i a11, a1, 104
	.byte 0x14, 0x61, 0xcd // st.qr q0, a1, 16
	.byte 0x14, 0x20, 0xcd // ld.qr q0, a1, 0
	addi a10, a10, 16
	addi a11, a11, 16
	call8 tinygo_nn_multiply_by_quantized_mult_ver1
	l32i a13, a1, 100
	addi.n a2, a2, 8
	l32i a8, a1, 112
	l32i a15, a1, 116
	l32i a14, a1, 104
	addi a12, a1, 212
	ee.vldbc.32 q3, a12
	addi a12, a1, 196
	ee.vldbc.32 q1, a12
	addi a12, a1, 208
	.byte 0x14, 0x21, 0xdd // ld.qr q2, a1, 16
	addi a14, a14, 32
	addi a15, a15, 32
	s32i a15, a1, 116
	ee.vadds.s32 q2, q2, q1
	s32i a14, a1, 104
	ee.vadds.s32 q1, q0, q1
	ee.vmin.s32 q0, q2, q3
	ee.vldbc.32 q2, a12
	ee.vmin.s32 q1, q1, q3
	ee.vmax.s32 q1, q1, q2
	ee.vmax.s32 q0, q0, q2
	ee.vunzip.16 q0, q1
	ee.vunzip.8 q0, q1
	ee.vst.l.64.ip q0, a8, 8
	s32i a8, a1, 112
	blt a2, a13, .Lnn_far_221
	j .Lnn_depthwise_conv_s16_mult1_Lt_4_8706
.Lnn_far_221:
.Lnn_depthwise_conv_s16_mult1_Lt_4_9218:
	ee.zero.qacc
	l32i a13, a1, 96
	s32i a13, a1, 80
	blt a13, a7, .Lnn_far_220
	j .Lnn_depthwise_conv_s16_mult1_Lt_4_9474
.Lnn_far_220:
.Lnn_depthwise_conv_s16_mult1_LBB12_esp_nn_depthwise_conv_s16_mult1:
	mull a15, a13, a4
	l32i a14, a1, 92
	add.n a8, a15, a5
	add.n a14, a14, a13
	mull a14, a3, a14
	s32i a8, a1, 76
	bge a6, a5, .Lnn_depthwise_conv_s16_mult1_Lt_4_10242
.Lnn_depthwise_conv_s16_mult1_LBB15_esp_nn_depthwise_conv_s16_mult1:
	l32i a12, a1, 64
	l32i a9, a1, 168
	l32i a10, a1, 60
	l32i a11, a1, 84
	add.n a8, a15, a6
	add.n a11, a11, a6
	mull a8, a8, a10
	add.n a11, a14, a11
	mull a10, a10, a11
	add.n a8, a2, a8
	l32i a11, a1, 52
	addx2 a8, a8, a9
	add.n a10, a2, a10
	l32i a9, a1, 72
	addx2 a10, a10, a11
	loopgtz a9, .Lnn_depthwise_conv_s16_mult1_LBB41_esp_nn_depthwise_conv_s16_mult1
	ee.vld.128.xp q0, a10, a12
	ee.vld.128.xp q1, a8, a12
	ee.vmulas.s16.qacc q0, q1
.Lnn_depthwise_conv_s16_mult1_LBB41_esp_nn_depthwise_conv_s16_mult1:
.Lnn_depthwise_conv_s16_mult1_Lt_4_10242:
	add.n a14, a14, a3
	add.n a15, a15, a4
	l32i a9, a1, 80
	l32i a10, a1, 76
	addi.n a9, a9, 1
	add.n a10, a10, a4
	s32i a10, a1, 76
	s32i a9, a1, 80
	sub a9, a7, a9
	beqz a9, .Lnn_depthwise_conv_s16_mult1_Lt_4_9474
	blt a6, a5, .Lnn_depthwise_conv_s16_mult1_LBB15_esp_nn_depthwise_conv_s16_mult1
	j .Lnn_depthwise_conv_s16_mult1_Lt_4_10242
.Lnn_depthwise_conv_s16_mult1_Lt_4_7170:
	retw.n
	.size tinygo_nn_depthwise_conv_s16_mult1, . - tinygo_nn_depthwise_conv_s16_mult1

// convolution/esp_nn_depthwise_conv_s16_mult4_esp32s3.S
.section .text.tinygo_nn_depthwise_conv_s16_mult4,"ax",@progbits
	.global tinygo_nn_depthwise_conv_s16_mult4
	.type tinygo_nn_depthwise_conv_s16_mult4, @function
	.p2align 4
tinygo_nn_depthwise_conv_s16_mult4:
	entry a1, 288
	s32i a2, a1, 136
	s32i.n a4, a1, 40
	s32i a5, a1, 164
	addi a12, a1, 112
	addmi a10, a1, 256
	addmi a11, a1, 256
	addmi a13, a1, 256
	l16ui a8, a1, 324
	s32i.n a8, a1, 48
	addi a13, a13, 72
	addi a11, a11, 88
	addi a10, a10, 84
	ee.vldbc.32 q0, a10
	ee.vldbc.32 q1, a11
	ee.vldbc.32 q2, a13
	.byte 0xc4, 0x65, 0xdd // st.qr q2, a12, 80
	.byte 0xc4, 0xe6, 0xcd // st.qr q1, a12, 96
	.byte 0xc4, 0x67, 0xcd // st.qr q0, a12, 112
	beqz a8, .Lnn_depthwise_conv_s16_mult4_Lt_10_8450
	s32i a1, a1, 112
	neg a15, a6
	neg a4, a7
	addmi a8, a1, 256
	movi.n a9, 0
	movi.n a11, 0
	slli a14, a5, 1
	l16ui a13, a1, 296
	l16ui a10, a1, 308
	s32i.n a10, a1, 36
	s32i a13, a1, 76
	s32i a14, a1, 148
	s32i.n a11, a1, 52
	s32i a9, a1, 116
	.byte 0x84, 0x6f, 0xed // st.qr q4, a8, -16
	sext a4, a4, 15
	sext a15, a15, 15
	s32i.n a15, a1, 32
	mul16u a12, a5, a13
	s32i a4, a1, 92
	l16ui a8, a1, 320
	l16ui a9, a1, 292
	l32i a11, a1, 336
	s32i a11, a1, 64
	s32i.n a9, a1, 44
	s32i a8, a1, 68
	l32i a4, a1, 300
	s32i a12, a1, 140
	l32i a15, a1, 316
	s32i a15, a1, 96
	slli a12, a12, 1
	s32i a4, a1, 152
	addi a14, a13, -3
	l16ui a4, a1, 304
	s32i a14, a1, 108
	s32i a12, a1, 144
	slli a13, a13, 2
	s32i a13, a1, 80
	l32i a12, a1, 332
	l32i a14, a1, 312
	s32i a14, a1, 104
	s32i.n a12, a1, 60
	l16ui a13, a1, 288
	s32i.n a13, a1, 56
	j .Lnn_depthwise_conv_s16_mult4_Lt_10_8962
.Lnn_depthwise_conv_s16_mult4_Lt_10_9218:
	l32i.n a9, a1, 48
	l32i.n a11, a1, 44
	l32i.n a8, a1, 52
	l32i a10, a1, 92
	addi.n a8, a8, 1
	s32i.n a8, a1, 52
	add.n a11, a10, a11
	sub a8, a8, a9
	sext a10, a11, 15
	s32i a10, a1, 92
	beqz a8, .Lnn_depthwise_conv_s16_mult4_Lt_10_8450
.Lnn_depthwise_conv_s16_mult4_Lt_10_8962:
	l32i a12, a1, 68
	beqz a12, .Lnn_depthwise_conv_s16_mult4_Lt_10_9218
.Lnn_depthwise_conv_s16_mult4_LBB6_esp_nn_depthwise_conv_s16_mult4:
	l32i.n a7, a1, 36
	movi.n a11, 0
	l32i.n a8, a1, 40
	l32i a9, a1, 92
	movi.n a13, 0
	l32i.n a14, a1, 32
	s32i a14, a1, 160
	s32i a13, a1, 72
	neg a10, a9
	sub a8, a8, a9
	max a10, a10, a11
	s32i a10, a1, 100
	min a7, a7, a8
	j .Lnn_depthwise_conv_s16_mult4_Lt_10_9730
.Lnn_depthwise_conv_s16_mult4_Lt_10_9986:
	l32i a13, a1, 68
	l32i.n a15, a1, 56
	l32i a12, a1, 72
	l32i a14, a1, 160
	addi.n a12, a12, 1
	s32i a12, a1, 72
	add.n a15, a14, a15
	sext a14, a15, 15
	s32i a14, a1, 160
	beq a12, a13, .Lnn_depthwise_conv_s16_mult4_Lt_10_9218
.Lnn_depthwise_conv_s16_mult4_Lt_10_9730:
	l32i a8, a1, 164
	l32i a9, a1, 64
	l32i.n a10, a1, 60
	s32i a10, a1, 132
	s32i a9, a1, 128
	beqz a8, .Lnn_depthwise_conv_s16_mult4_Lt_10_9986
	movi.n a8, 0
	l32i a5, a1, 160
	movi.n a12, 0
	movi.n a13, 0
	movi.n a14, 0
	s32i a14, a1, 84
	s32i a13, a1, 88
	s32i a12, a1, 176
	neg a6, a5
	max a6, a6, a8
	sub a5, a3, a5
	min a5, a4, a5
	sub a11, a5, a6
	s32i a11, a1, 156
	j .Lnn_depthwise_conv_s16_mult4_Lt_10_10498
.Lnn_depthwise_conv_s16_mult4_Lt_10_10754:
	l32i a10, a1, 164
	l32i a14, a1, 76
	l32i a13, a1, 84
	l32i a12, a1, 80
	l32i a9, a1, 176
	l32i a11, a1, 88
	addi.n a9, a9, 1
	s32i a9, a1, 176
	add.n a11, a11, a12
	add.n a13, a13, a14
	s32i a13, a1, 84
	s32i a11, a1, 88
	beq a9, a10, .Lnn_depthwise_conv_s16_mult4_Lt_10_9986
.Lnn_depthwise_conv_s16_mult4_Lt_10_10498:
	l32i a15, a1, 108
	blti a15, 1, .Lnn_depthwise_conv_s16_mult4_Lt_10_10754
	l32i a2, a1, 84
	l32i a10, a1, 104
	l32i a9, a1, 88
	movi.n a8, 0
	s32i a8, a1, 120
	add.n a9, a9, a10
	s32i a9, a1, 124
	j .Lnn_depthwise_conv_s16_mult4_Lt_10_11266
.Lnn_depthwise_conv_s16_mult4_Lt_10_11522:
	addmi a12, a1, 256
	l32i a14, a1, 112
	movi.n a13, 16
	ee.st.qacc_l.l.128.ip a14, 16
	ee.st.qacc_l.h.32.ip a14, -16
	ee.srcmb.s16.qacc q5, a13, 0
	l16ui a15, a1, 10
	l8ui a8, a1, 15
	l8ui a9, a1, 5
	l8ui a11, a1, 16
	l8ui a10, a1, 6
	s8i a10, a1, 3
	s8i a11, a1, 7
	s8i a9, a1, 2
	l32i a11, a1, 104
	s8i a8, a1, 6
	s16i a15, a1, 4
	ee.vld.l.64.ip q0, a14, 0
	s32i a14, a1, 112
	ee.vzip.16 q0, q5
	.byte 0xc4, 0xef, 0xed // st.qr q5, a12, -16
	beqz.n a11, .Lnn_depthwise_conv_s16_mult4_Lt_10_13570
	l32i a13, a1, 124
	extui a12, a13, 0, 4
	ee.vld.128.ip q7, a13, 16
	ee.vld.128.ip q1, a13, 0
	wur.sar_byte a12
	ee.src.q.qup q6, q7, q1
	ee.vadds.s32 q0, q0, q6
.Lnn_depthwise_conv_s16_mult4_Lt_10_13570:
	l32i a10, a1, 128
	l32i a11, a1, 132
	call8 tinygo_nn_multiply_by_quantized_mult_ver1
	addi.n a2, a2, 4
	l32i a13, a1, 96
	l32i a11, a1, 128
	l32i a10, a1, 132
	addi a8, a1, 112
	.byte 0x84, 0xa6, 0xcd // ld.qr q1, a8, 96
	.byte 0x84, 0x25, 0xdd // ld.qr q2, a8, 80
	addi a10, a10, 16
	addi a11, a11, 16
	s32i a11, a1, 128
	ee.vadds.s32 q0, q0, q2
	s32i a10, a1, 132
	ee.vmin.s32 q0, q0, q1
	.byte 0x84, 0xa7, 0xcd // ld.qr q1, a8, 112
	l32i a8, a1, 116
	ee.vmax.s32 q0, q0, q1
	ee.movi.32.a q0, a14, 2
	ee.movi.32.a q0, a15, 1
	ee.movi.32.a q0, a9, 0
	add.n a13, a8, a13
	ee.movi.32.a q0, a12, 3
	addi.n a8, a8, 4
	s8i a12, a13, 3
	s32i a8, a1, 116
	s8i a9, a13, 0
	s8i a15, a13, 1
	s8i a14, a13, 2
	l32i a15, a1, 108
	l32i a14, a1, 120
	l32i a9, a1, 124
	addi.n a14, a14, 4
	addi a9, a9, 16
	s32i a9, a1, 124
	s32i a14, a1, 120
	blt a14, a15, .Lnn_far_216
	j .Lnn_depthwise_conv_s16_mult4_Lt_10_10754
.Lnn_far_216:
.Lnn_depthwise_conv_s16_mult4_Lt_10_11266:
	ee.zero.qacc
	l32i a9, a1, 100
	s32i a9, a1, 172
	blt a9, a7, .Lnn_far_215
	j .Lnn_depthwise_conv_s16_mult4_Lt_10_11522
.Lnn_far_215:
	mull a15, a9, a4
	l32i a14, a1, 92
	add.n a11, a15, a5
	add.n a14, a14, a9
	mull a14, a3, a14
	s32i a11, a1, 168
	bge a6, a5, .Lnn_depthwise_conv_s16_mult4_Lt_10_12290
.Lnn_depthwise_conv_s16_mult4_LBB18_esp_nn_depthwise_conv_s16_mult4:
	l32i a10, a1, 176
	l32i a11, a1, 164
	l32i a12, a1, 160
	add.n a9, a15, a6
	l32i a8, a1, 140
	addmi a13, a1, 256
	.byte 0xd4, 0xaf, 0xcd // ld.qr q1, a13, -16
	mull a8, a8, a9
	add.n a12, a12, a6
	l32i a9, a1, 152
	add.n a12, a14, a12
	mull a11, a11, a12
	add.n a8, a2, a8
	l32i a12, a1, 148
	addx2 a8, a8, a9
	add.n a10, a10, a11
	l32i a11, a1, 136
	l32i a9, a1, 156
	addx2 a10, a10, a11
	l32i a11, a1, 144
	loopgtz a9, .Lnn_depthwise_conv_s16_mult4_LBB45_esp_nn_depthwise_conv_s16_mult4
	mov.n a9, a8
	ee.vldbc.16 q0, a10
	add.n a10, a10, a12
	ee.vld.l.64.ip q1, a9, 0
	add.n a8, a8, a11
	ee.vmulas.s16.qacc q0, q1
.Lnn_depthwise_conv_s16_mult4_LBB45_esp_nn_depthwise_conv_s16_mult4:
	addmi a10, a1, 256
	.byte 0xa4, 0xef, 0xcd // st.qr q1, a10, -16
.Lnn_depthwise_conv_s16_mult4_Lt_10_12290:
	add.n a14, a14, a3
	add.n a15, a15, a4
	l32i a11, a1, 172
	l32i a12, a1, 168
	addi.n a11, a11, 1
	add.n a12, a12, a4
	s32i a12, a1, 168
	s32i a11, a1, 172
	sub a11, a7, a11
	beqz a11, .Lnn_depthwise_conv_s16_mult4_Lt_10_11522
	blt a6, a5, .Lnn_depthwise_conv_s16_mult4_LBB18_esp_nn_depthwise_conv_s16_mult4
	j .Lnn_depthwise_conv_s16_mult4_Lt_10_12290
.Lnn_depthwise_conv_s16_mult4_Lt_10_8450:
	retw.n
	.size tinygo_nn_depthwise_conv_s16_mult4, . - tinygo_nn_depthwise_conv_s16_mult4

// convolution/esp_nn_depthwise_conv_s16_mult8_3x3_esp32s3.S
.section .text.tinygo_nn_depthwise_conv_s16_mult8_3x3,"ax",@progbits
	.global tinygo_nn_depthwise_conv_s16_mult8_3x3
	.type tinygo_nn_depthwise_conv_s16_mult8_3x3, @function
	.p2align 4
tinygo_nn_depthwise_conv_s16_mult8_3x3:
	entry a1, 304
	s32i a2, a1, 116
	s32i a3, a1, 120
	s32i a5, a1, 144
	s32i.n a6, a1, 60
	addmi a9, a1, 256
	addi a12, a1, 112
	addmi a10, a1, 256
	addmi a11, a1, 256
	addmi a13, a1, 256
	l16ui a8, a1, 332
	l32i a14, a1, 324
	s32i a14, a1, 176
	s32i a8, a1, 68
	addi a13, a13, 80
	addi a11, a11, 96
	addi a10, a10, 92
	ee.vldbc.32 q0, a10
	ee.vldbc.32 q1, a11
	ee.vldbc.32 q2, a13
	.byte 0xc4, 0x66, 0xdd // st.qr q2, a12, 96
	.byte 0xc4, 0xe7, 0xcd // st.qr q1, a12, 112
	.byte 0x94, 0x6f, 0xcd // st.qr q0, a9, -16
	beqz a8, .Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_8194
.Lnn_depthwise_conv_s16_mult8_3x3_LBB3_esp_nn_depthwise_conv_s16_mult8_3x3:
	s32i a1, a1, 180
	mul16u a6, a3, a5
	s32i a7, a1, 76
	l32i a9, a1, 316
	l32i a15, a1, 320
	l16ui a10, a1, 312
	slli a11, a5, 1
	l16ui a12, a1, 308
	l32i a13, a1, 344
	l32i a14, a1, 340
	s32i a14, a1, 88
	s32i a13, a1, 92
	s32i a12, a1, 64
	s32i a11, a1, 124
	s32i a10, a1, 108
	s32i a15, a1, 160
	s32i a9, a1, 128
	neg a7, a7
	slli a6, a6, 1
	s32i a7, a1, 136
	movi.n a9, 0
	extui a15, a15, 0, 4
	s32i a15, a1, 152
	s32i a9, a1, 72
	sub a7, a4, a7
	l32i.n a9, a1, 60
	s32i a7, a1, 80
	l16ui a4, a1, 328
	s32i a4, a1, 96
	l16ui a7, a1, 304
	s32i a7, a1, 84
	mul16u a4, a5, a10
	neg a9, a9
	s32i.n a9, a1, 52
	sub a8, a3, a9
	addi a10, a10, -7
	s32i a10, a1, 164
	s32i.n a8, a1, 56
	addx2 a7, a4, a4
	slli a7, a7, 1
	j .Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_8706
.Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_8962:
	l32i a10, a1, 68
	l32i a14, a1, 76
	l32i a13, a1, 136
	l32i a12, a1, 64
	l32i a9, a1, 72
	l32i a11, a1, 80
	addi.n a9, a9, 1
	s32i a9, a1, 72
	sub a11, a11, a12
	add.n a13, a13, a12
	sub a14, a14, a12
	s32i a14, a1, 76
	s32i a13, a1, 136
	s32i a11, a1, 80
	sub a9, a9, a10
	beqz a9, .Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_8194
.Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_8706:
	l32i a15, a1, 96
	beqz a15, .Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_8962
.Lnn_depthwise_conv_s16_mult8_3x3_LBB6_esp_nn_depthwise_conv_s16_mult8_3x3:
	l32i.n a3, a1, 56
	l32i a8, a1, 80
	movi.n a10, 0
	l32i a9, a1, 76
	movi.n a11, 0
	l32i.n a12, a1, 52
	l32i.n a13, a1, 60
	s32i a13, a1, 104
	s32i a12, a1, 140
	s32i a11, a1, 100
	max a9, a9, a10
	movi.n a10, 3
	s32i a9, a1, 172
	min a8, a8, a10
	s32i a8, a1, 156
	sub a8, a8, a9
	s32i a8, a1, 132
	j .Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_9474
.Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_9730:
	l32i a15, a1, 96
	l32i a10, a1, 104
	l32i a9, a1, 140
	l32i a8, a1, 84
	l32i a14, a1, 100
	sub a3, a3, a8
	addi.n a14, a14, 1
	s32i a14, a1, 100
	add.n a9, a9, a8
	sub a10, a10, a8
	s32i a10, a1, 104
	s32i a9, a1, 140
	bne a14, a15, .Lnn_far_212
	j .Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_8962
.Lnn_far_212:
.Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_9474:
	l32i a2, a1, 88
	l32i a10, a1, 92
	l32i a12, a1, 160
	l32i a11, a1, 144
	s32i a12, a1, 168
	beqz a11, .Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_9730
.Lnn_depthwise_conv_s16_mult8_3x3_LBB9_esp_nn_depthwise_conv_s16_mult8_3x3:
	movi.n a8, 0
	l32i a5, a1, 104
	movi.n a13, 0
	movi.n a9, 0
	s32i a9, a1, 112
	s32i a13, a1, 148
	max a5, a5, a8
	j .Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_10242
.Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_10498:
	l32i a12, a1, 144
	l32i a14, a1, 108
	l32i a11, a1, 148
	l32i a13, a1, 112
	addi.n a11, a11, 1
	s32i a11, a1, 148
	add.n a13, a13, a14
	s32i a13, a1, 112
	beq a11, a12, .Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_9730
.Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_10242:
	l32i a15, a1, 164
	blti a15, 1, .Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_10498
	movi.n a8, 0
	l32i a9, a1, 112
	s32i a9, a1, 188
	s32i a8, a1, 184
	j .Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_11010
.Lnn_depthwise_conv_s16_mult8_3x3_LBB23_esp_nn_depthwise_conv_s16_mult8_3x3:
	s32i.n a10, a1, 48
	addi a11, a1, 112
	l32i a13, a1, 152
	l32i a12, a1, 168
	wur.sar_byte a13
	ee.vld.128.ip q4, a12, 16
	ee.vld.128.ip q7, a12, 16
	ee.vld.128.ip q5, a12, 0
	s32i a12, a1, 168
	ee.src.q.qup q6, q4, q7
	ee.vadds.s32 q0, q0, q6
	ee.src.q.qup q3, q4, q5
	ee.vadds.s32 q1, q1, q3
	.byte 0xb4, 0xe5, 0xcd // st.qr q1, a11, 80
.Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_13314:
	l32i.n a10, a1, 48
	mov.n a11, a2
	call8 tinygo_nn_multiply_by_quantized_mult_ver1
	l32i.n a10, a1, 48
	addmi a12, a1, 256
	addi a11, a1, 112
	.byte 0xc4, 0x60, 0xcd // st.qr q0, a12, 0
	.byte 0xb4, 0x25, 0xcd // ld.qr q0, a11, 80
	addi a10, a10, 16
	addi a11, a2, 16
	call8 tinygo_nn_multiply_by_quantized_mult_ver1
	addi a2, a2, 32
	l32i a14, a1, 164
	l32i a8, a1, 176
	l32i a15, a1, 188
	l32i a13, a1, 184
	l32i.n a10, a1, 48
	addmi a11, a1, 256
	addi a12, a1, 112
	.byte 0xc4, 0xa7, 0xdd // ld.qr q3, a12, 112
	.byte 0xc4, 0xa6, 0xcd // ld.qr q1, a12, 96
	.byte 0xb4, 0x20, 0xdd // ld.qr q2, a11, 0
	addi a10, a10, 32
	addi.n a13, a13, 8
	addi.n a15, a15, 8
	s32i a15, a1, 188
	ee.vadds.s32 q2, q2, q1
	s32i a13, a1, 184
	ee.vadds.s32 q1, q0, q1
	ee.vmin.s32 q0, q2, q3
	.byte 0xb4, 0x2f, 0xdd // ld.qr q2, a11, -16
	ee.vmin.s32 q1, q1, q3
	ee.vmax.s32 q1, q1, q2
	ee.vmax.s32 q0, q0, q2
	ee.vunzip.16 q0, q1
	ee.vunzip.8 q0, q1
	ee.vst.l.64.ip q0, a8, 8
	s32i a8, a1, 176
	blt a13, a14, .Lnn_far_210
	j .Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_10498
.Lnn_far_210:
.Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_11010:
	l32i a14, a1, 156
	l32i a13, a1, 172
	ee.zero.qacc
	bge a13, a14, .Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_11266
.Lnn_depthwise_conv_s16_mult8_3x3_LBB15_esp_nn_depthwise_conv_s16_mult8_3x3:
	l32i a12, a1, 124
	l32i a8, a1, 140
	l32i a11, a1, 120
	l32i a14, a1, 188
	l32i a9, a1, 136
	mull a15, a4, a13
	add.n a9, a9, a13
	addx2 a15, a15, a15
	l32i a13, a1, 148
	add.n a14, a14, a15
	mull a9, a9, a11
	l32i a15, a1, 144
	add.n a8, a8, a9
	mull a15, a15, a8
	l32i a8, a1, 128
	add.n a13, a13, a15
	l32i a15, a1, 116
	addx2 a14, a14, a8
	addx2 a13, a13, a15
	add.n a11, a12, a13
	l32i a15, a1, 132
	add.n a12, a12, a11
	loopgtz a15, .Lnn_depthwise_conv_s16_mult8_3x3_LBB34_esp_nn_depthwise_conv_s16_mult8_3x3
.Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_11778:
	mov.n a15, a14
	mov.n a9, a14
	bnez.n a5, .Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_12034
	ee.vldbc.16 q3, a13
	mov.n a9, a14
	ee.vld.128.ip q4, a9, 0
	ee.vmulas.s16.qacc q3, q4
.Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_12034:
	ee.vldbc.16 q5, a11
	addx2 a9, a4, a9
	ee.vld.128.ip q6, a9, 0
	add.n a13, a13, a6
	ee.vmulas.s16.qacc q5, q6
	blti a3, 3, .Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_12546
	ee.vldbc.16 q7, a12
	addx2 a14, a4, a9
	ee.vld.128.ip q0, a14, 0
	ee.vmulas.s16.qacc q7, q0
.Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_12546:
	add.n a11, a11, a6
	add.n a12, a12, a6
	add.n a14, a7, a15
.Lnn_depthwise_conv_s16_mult8_3x3_LBB34_esp_nn_depthwise_conv_s16_mult8_3x3:
.Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_11266:
	l32i a8, a1, 180
	ee.st.qacc_l.l.128.ip a8, 16
	ee.st.qacc_l.h.32.ip a8, 0
	l16ui a9, a1, 10
	l8ui a11, a1, 15
	l8ui a12, a1, 5
	l8ui a13, a1, 6
	l8ui a14, a1, 16
	s8i a14, a1, 7
	s8i a13, a1, 3
	s8i a12, a1, 2
	s8i a11, a1, 6
	s16i a9, a1, 4
	ee.st.qacc_h.l.128.ip a8, 16
	ee.st.qacc_h.h.32.ip a8, -32
	l16ui a9, a1, 16
	l8ui a15, a1, 32
	l8ui a12, a1, 22
	l8ui a11, a1, 21
	l8ui a14, a1, 31
	l16ui a13, a1, 26
	s16i a13, a1, 12
	s8i a14, a1, 14
	s8i a11, a1, 10
	s8i a12, a1, 11
	s8i a15, a1, 15
	s16i a9, a1, 8
	l32i a15, a1, 160
	movi.n a9, 16
	ee.srcmb.s16.qacc q1, a9, 0
	ee.vld.128.ip q0, a8, 0
	s32i a8, a1, 180
	ee.vzip.16 q0, q1
	bnez a15, .Lnn_depthwise_conv_s16_mult8_3x3_LBB23_esp_nn_depthwise_conv_s16_mult8_3x3
	s32i.n a10, a1, 48
	addi a15, a1, 112
	.byte 0xf4, 0xe5, 0xcd // st.qr q1, a15, 80
	j .Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_13314
.Lnn_depthwise_conv_s16_mult8_3x3_Lt_8_8194:
	retw.n
	.size tinygo_nn_depthwise_conv_s16_mult8_3x3, . - tinygo_nn_depthwise_conv_s16_mult8_3x3

// convolution/esp_nn_depthwise_conv_s16_mult8_esp32s3.S
.section .text.tinygo_nn_depthwise_conv_s16_mult8,"ax",@progbits
	.global tinygo_nn_depthwise_conv_s16_mult8
	.type tinygo_nn_depthwise_conv_s16_mult8, @function
	.p2align 4
tinygo_nn_depthwise_conv_s16_mult8:
	entry a1, 256
	s32i a2, a1, 144
	s32i.n a4, a1, 56
	s32i a5, a1, 172
	l32i a9, a1, 284
	l16ui a8, a1, 292
	s32i a8, a1, 64
	s32i a9, a1, 124
	beqz a8, .Lnn_depthwise_conv_s16_mult8_Lt_9_8450
	s32i a1, a1, 128
	neg a13, a7
	movi.n a4, 0
	neg a12, a6
	l32i a9, a1, 280
	slli a11, a5, 1
	l16ui a10, a1, 264
	l32i a14, a1, 268
	s32i a14, a1, 160
	s32i a10, a1, 92
	s32i a11, a1, 156
	s32i a9, a1, 112
	sext a12, a12, 15
	s32i a4, a1, 68
	sext a13, a13, 15
	l16ui a4, a1, 272
	s32i a13, a1, 100
	s32i.n a12, a1, 48
	mul16u a8, a5, a10
	extui a9, a9, 0, 4
	l32i a11, a1, 304
	s32i a11, a1, 80
	s32i a9, a1, 104
	s32i a8, a1, 148
	addi a10, a10, -7
	l32i a12, a1, 300
	l16ui a13, a1, 256
	s32i a13, a1, 72
	s32i a12, a1, 76
	s32i a10, a1, 116
	slli a8, a8, 1
	l16ui a9, a1, 260
	s32i.n a9, a1, 60
	s32i a8, a1, 152
	l16ui a10, a1, 276
	s32i.n a10, a1, 52
	l16ui a8, a1, 288
	s32i a8, a1, 84
	j .Lnn_depthwise_conv_s16_mult8_Lt_9_8962
.Lnn_depthwise_conv_s16_mult8_Lt_9_9218:
	l32i a15, a1, 64
	l32i.n a9, a1, 60
	l32i a14, a1, 68
	l32i a8, a1, 100
	addi.n a14, a14, 1
	s32i a14, a1, 68
	add.n a9, a8, a9
	sub a14, a14, a15
	sext a8, a9, 15
	s32i a8, a1, 100
	beqz a14, .Lnn_depthwise_conv_s16_mult8_Lt_9_8450
.Lnn_depthwise_conv_s16_mult8_Lt_9_8962:
	l32i a10, a1, 84
	beqz a10, .Lnn_depthwise_conv_s16_mult8_Lt_9_9218
	l32i.n a7, a1, 52
	movi.n a11, 0
	l32i.n a8, a1, 56
	l32i a9, a1, 100
	l32i.n a12, a1, 48
	s32i a12, a1, 168
	neg a10, a9
	sub a8, a8, a9
	max a10, a10, a11
	s32i a10, a1, 108
	min a7, a7, a8
	movi.n a11, 0
	s32i a11, a1, 88
	j .Lnn_depthwise_conv_s16_mult8_Lt_9_9730
.Lnn_depthwise_conv_s16_mult8_Lt_9_9986:
	l32i a13, a1, 84
	l32i a15, a1, 72
	l32i a12, a1, 88
	l32i a14, a1, 168
	addi.n a12, a12, 1
	s32i a12, a1, 88
	add.n a15, a14, a15
	sext a14, a15, 15
	s32i a14, a1, 168
	beq a12, a13, .Lnn_depthwise_conv_s16_mult8_Lt_9_9218
.Lnn_depthwise_conv_s16_mult8_Lt_9_9730:
	l32i a8, a1, 172
	l32i a9, a1, 80
	l32i a10, a1, 76
	l32i a11, a1, 112
	s32i a11, a1, 120
	s32i a10, a1, 140
	s32i a9, a1, 136
	beqz a8, .Lnn_depthwise_conv_s16_mult8_Lt_9_9986
.Lnn_depthwise_conv_s16_mult8_LBB9_esp_nn_depthwise_conv_s16_mult8:
	movi.n a8, 0
	l32i a5, a1, 168
	movi.n a13, 0
	movi.n a14, 0
	s32i a14, a1, 96
	s32i a13, a1, 184
	neg a6, a5
	max a6, a6, a8
	sub a5, a3, a5
	min a5, a4, a5
	sub a12, a5, a6
	s32i a12, a1, 164
	j .Lnn_depthwise_conv_s16_mult8_Lt_9_10498
.Lnn_depthwise_conv_s16_mult8_Lt_9_10754:
	l32i a10, a1, 172
	l32i a12, a1, 92
	l32i a9, a1, 184
	l32i a11, a1, 96
	addi.n a9, a9, 1
	s32i a9, a1, 184
	add.n a11, a11, a12
	s32i a11, a1, 96
	beq a9, a10, .Lnn_depthwise_conv_s16_mult8_Lt_9_9986
.Lnn_depthwise_conv_s16_mult8_Lt_9_10498:
	l32i a13, a1, 116
	blti a13, 1, .Lnn_depthwise_conv_s16_mult8_Lt_9_10754
.Lnn_depthwise_conv_s16_mult8_LBB12_esp_nn_depthwise_conv_s16_mult8:
	l32i a2, a1, 96
	movi.n a14, 0
	s32i a14, a1, 132
	j .Lnn_depthwise_conv_s16_mult8_Lt_9_11266
.Lnn_depthwise_conv_s16_mult8_Lt_9_11522:
	l32i a9, a1, 128
	ee.st.qacc_l.l.128.ip a9, 16
	ee.st.qacc_l.h.32.ip a9, 0
	l8ui a10, a1, 15
	l16ui a8, a1, 10
	l8ui a13, a1, 16
	l8ui a12, a1, 6
	l8ui a11, a1, 5
	s8i a11, a1, 2
	s8i a12, a1, 3
	s8i a13, a1, 7
	s16i a8, a1, 4
	s8i a10, a1, 6
	movi.n a8, 16
	ee.st.qacc_h.l.128.ip a9, 16
	ee.st.qacc_h.h.32.ip a9, -32
	ee.srcmb.s16.qacc q1, a8, 0
	l16ui a13, a1, 26
	l8ui a15, a1, 32
	l8ui a12, a1, 22
	l8ui a11, a1, 21
	l16ui a10, a1, 16
	l8ui a14, a1, 31
	s8i a14, a1, 14
	s16i a10, a1, 8
	s8i a11, a1, 10
	s8i a12, a1, 11
	s8i a15, a1, 15
	s16i a13, a1, 12
	l32i a15, a1, 112
	ee.vld.128.ip q0, a9, 0
	s32i a9, a1, 128
	ee.vzip.16 q0, q1
	beqz.n a15, .Lnn_depthwise_conv_s16_mult8_Lt_9_13570
.Lnn_depthwise_conv_s16_mult8_LBB23_esp_nn_depthwise_conv_s16_mult8:
	addi a14, a1, 112
	l32i a8, a1, 104
	l32i a15, a1, 120
	wur.sar_byte a8
	ee.vld.128.ip q3, a15, 16
	ee.vld.128.ip q6, a15, 16
	ee.vld.128.ip q4, a15, 0
	s32i a15, a1, 120
	ee.src.q.qup q5, q3, q6
	ee.vadds.s32 q0, q0, q5
	ee.src.q.qup q2, q3, q4
	ee.vadds.s32 q1, q1, q2
	.byte 0xe4, 0xe6, 0xcd // st.qr q1, a14, 96
.Lnn_depthwise_conv_s16_mult8_Lt_9_13570:
	l32i a10, a1, 136
	l32i a11, a1, 140
	addi a9, a1, 112
	.byte 0x94, 0xe6, 0xcd // st.qr q1, a9, 96
	call8 tinygo_nn_multiply_by_quantized_mult_ver1
	l32i a11, a1, 140
	addi a12, a1, 112
	l32i a10, a1, 136
	.byte 0xc4, 0x65, 0xcd // st.qr q0, a12, 80
	.byte 0xc4, 0x26, 0xcd // ld.qr q0, a12, 96
	addi a10, a10, 16
	addi a11, a11, 16
	call8 tinygo_nn_multiply_by_quantized_mult_ver1
	addi.n a2, a2, 8
	l32i a14, a1, 116
	l32i a15, a1, 124
	l32i a13, a1, 132
	l32i a10, a1, 140
	l32i a11, a1, 136
	addmi a9, a1, 256
	addi a8, a1, 112
	.byte 0x84, 0xa5, 0xfd // ld.qr q7, a8, 80
	addi a9, a9, 56
	ee.vldbc.32 q2, a9
	addi a11, a11, 32
	addi a10, a10, 32
	addi.n a13, a13, 8
	s32i a13, a1, 132
	s32i a10, a1, 140
	s32i a11, a1, 136
	addmi a10, a1, 256
	addmi a11, a1, 256
	addi a11, a11, 52
	addi a10, a10, 40
	ee.vldbc.32 q3, a10
	ee.vldbc.32 q1, a11
	ee.vadds.s32 q0, q0, q3
	ee.vadds.s32 q7, q7, q3
	ee.vmin.s32 q7, q7, q2
	ee.vmin.s32 q0, q0, q2
	ee.vmax.s32 q0, q0, q1
	ee.vmax.s32 q7, q7, q1
	ee.vunzip.16 q7, q0
	ee.vunzip.8 q7, q0
	ee.vst.l.64.ip q7, a15, 8
	s32i a15, a1, 124
	blt a13, a14, .Lnn_far_205
	j .Lnn_depthwise_conv_s16_mult8_Lt_9_10754
.Lnn_far_205:
.Lnn_depthwise_conv_s16_mult8_Lt_9_11266:
	ee.zero.qacc
	l32i a12, a1, 108
	s32i a12, a1, 180
	blt a12, a7, .Lnn_far_204
	j .Lnn_depthwise_conv_s16_mult8_Lt_9_11522
.Lnn_far_204:
	mull a15, a12, a4
	l32i a14, a1, 100
	add.n a8, a15, a5
	add.n a14, a14, a12
	mull a14, a3, a14
	s32i a8, a1, 176
	bge a6, a5, .Lnn_depthwise_conv_s16_mult8_Lt_9_12290
.Lnn_depthwise_conv_s16_mult8_LBB18_esp_nn_depthwise_conv_s16_mult8:
	l32i a10, a1, 184
	l32i a11, a1, 172
	l32i a12, a1, 168
	l32i a8, a1, 148
	add.n a9, a15, a6
	mull a8, a8, a9
	add.n a12, a12, a6
	l32i a9, a1, 160
	add.n a12, a14, a12
	mull a11, a11, a12
	add.n a8, a2, a8
	l32i a12, a1, 156
	addx2 a8, a8, a9
	add.n a10, a10, a11
	l32i a11, a1, 144
	l32i a9, a1, 164
	addx2 a10, a10, a11
	l32i a11, a1, 152
	loopgtz a9, .Lnn_depthwise_conv_s16_mult8_LBB45_esp_nn_depthwise_conv_s16_mult8
	mov.n a9, a8
	ee.vldbc.16 q0, a10
	ee.vld.128.ip q1, a9, 0
	add.n a10, a10, a12
	add.n a8, a8, a11
	ee.vmulas.s16.qacc q0, q1
.Lnn_depthwise_conv_s16_mult8_LBB45_esp_nn_depthwise_conv_s16_mult8:
.Lnn_depthwise_conv_s16_mult8_Lt_9_12290:
	add.n a14, a14, a3
	add.n a15, a15, a4
	l32i a10, a1, 180
	l32i a11, a1, 176
	addi.n a10, a10, 1
	add.n a11, a11, a4
	s32i a11, a1, 176
	s32i a10, a1, 180
	sub a10, a7, a10
	beqz a10, .Lnn_depthwise_conv_s16_mult8_Lt_9_11522
.Lnn_depthwise_conv_s16_mult8_Lt_9_12034:
	blt a6, a5, .Lnn_depthwise_conv_s16_mult8_LBB18_esp_nn_depthwise_conv_s16_mult8
	j .Lnn_depthwise_conv_s16_mult8_Lt_9_12290
.Lnn_depthwise_conv_s16_mult8_Lt_9_8450:
	retw.n
	.size tinygo_nn_depthwise_conv_s16_mult8, . - tinygo_nn_depthwise_conv_s16_mult8

// convolution/esp_nn_depthwise_conv_s8_kx1_esp32s3.S
.section .text.tinygo_nn_dw_s8_kx1,"ax",@progbits
	.global tinygo_nn_dw_s8_kx1
	.type tinygo_nn_dw_s8_kx1, @function
	.p2align 4
tinygo_nn_dw_s8_kx1:
	entry a1, 160
	ee.zero.qacc
	mov.n a8, a2
	mov.n a9, a5
	loopgtz a4, .Lnn_dw_s8_kx1_Lcl_mac_done
	ee.vld.128.xp q0, a8, a3
	ee.vld.128.xp q1, a9, a3
	ee.vmulas.s8.qacc q0, q1
.Lnn_dw_s8_kx1_Lcl_mac_done:
	mov a10, a1
	ee.st.qacc_l.l.128.ip a10, 16
	ee.st.qacc_l.h.32.ip a10, -16
	addi a10, a1, 32
	ee.st.qacc_h.l.128.ip a10, 16
	ee.st.qacc_h.h.32.ip a10, -16
	l32i.n a12, a1, 0
	l32i.n a13, a1, 4
	sext a8, a12, 19
	extui a12, a12, 20, 12
	slli a14, a13, 24
	srai a14, a14, 12
	or a14, a14, a12
	slli a15, a13, 4
	srai a15, a15, 12
	ee.movi.32.q q0, a8, 0
	ee.movi.32.q q0, a14, 1
	ee.movi.32.q q0, a15, 2
	l32i.n a12, a1, 8
	extui a13, a13, 28, 4
	slli a8, a12, 16
	srai a8, a8, 12
	or a8, a8, a13
	ee.movi.32.q q0, a8, 3
	l32i.n a13, a1, 12
	extui a14, a12, 16, 16
	slli a8, a13, 28
	srai a8, a8, 12
	or a8, a8, a14
	slli a15, a13, 8
	srai a15, a15, 12
	l32i.n a12, a1, 16
	ee.movi.32.q q3, a8, 0
	ee.movi.32.q q3, a15, 1
	extui a13, a13, 24, 8
	slli a8, a12, 20
	srai a8, a8, 12
	or a8, a8, a13
	srai a15, a12, 12
	ee.movi.32.q q3, a8, 2
	ee.movi.32.q q3, a15, 3
	l32i.n a12, a1, 32
	l32i.n a13, a1, 36
	sext a8, a12, 19
	extui a12, a12, 20, 12
	slli a14, a13, 24
	srai a14, a14, 12
	or a14, a14, a12
	slli a15, a13, 4
	srai a15, a15, 12
	ee.movi.32.q q1, a8, 0
	ee.movi.32.q q1, a14, 1
	ee.movi.32.q q1, a15, 2
	l32i.n a12, a1, 40
	extui a13, a13, 28, 4
	slli a8, a12, 16
	srai a8, a8, 12
	or a8, a8, a13
	ee.movi.32.q q1, a8, 3
	l32i.n a13, a1, 44
	extui a14, a12, 16, 16
	slli a8, a13, 28
	srai a8, a8, 12
	or a8, a8, a14
	slli a15, a13, 8
	srai a15, a15, 12
	l32i.n a12, a1, 48
	ee.movi.32.q q2, a8, 0
	ee.movi.32.q q2, a15, 1
	extui a13, a13, 24, 8
	slli a8, a12, 20
	srai a8, a8, 12
	or a8, a8, a13
	srai a15, a12, 12
	ee.movi.32.q q2, a8, 2
	ee.movi.32.q q2, a15, 3
	.byte 0x64, 0x20, 0xed // ld.qr q4, a6, 0
	ee.vadds.s32 q0, q0, q4
	.byte 0x64, 0x21, 0xed // ld.qr q4, a6, 16
	ee.vadds.s32 q3, q3, q4
	.byte 0x64, 0x22, 0xed // ld.qr q4, a6, 32
	ee.vadds.s32 q1, q1, q4
	.byte 0x64, 0x23, 0xed // ld.qr q4, a6, 48
	ee.vadds.s32 q2, q2, q4
	.byte 0x14, 0x64, 0xcd // st.qr q0, a1, 64
	.byte 0x14, 0xe5, 0xdd // st.qr q3, a1, 80
	.byte 0x14, 0xe6, 0xcd // st.qr q1, a1, 96
	.byte 0x14, 0x67, 0xdd // st.qr q2, a1, 112
	s32i a7, a1, 56
	l32i a6, a7, 4
	l32i a7, a7, 8
	mov.n a10, a6
	mov.n a11, a7
	call8 tinygo_nn_multiply_by_quantized_mult_ver1
	.byte 0x14, 0x64, 0xcd // st.qr q0, a1, 64
	.byte 0x14, 0x25, 0xcd // ld.qr q0, a1, 80
	addi a10, a6, 16
	addi a11, a7, 16
	call8 tinygo_nn_multiply_by_quantized_mult_ver1
	.byte 0x14, 0x65, 0xcd // st.qr q0, a1, 80
	.byte 0x14, 0x26, 0xcd // ld.qr q0, a1, 96
	addi a10, a6, 32
	addi a11, a7, 32
	call8 tinygo_nn_multiply_by_quantized_mult_ver1
	.byte 0x14, 0x66, 0xcd // st.qr q0, a1, 96
	.byte 0x14, 0x27, 0xcd // ld.qr q0, a1, 112
	addi a10, a6, 48
	addi a11, a7, 48
	call8 tinygo_nn_multiply_by_quantized_mult_ver1
	.byte 0x14, 0x67, 0xcd // st.qr q0, a1, 112
	l32i a7, a1, 56
	addi a8, a7, 12
	ee.vldbc.32 q5, a8
	addi a8, a7, 16
	ee.vldbc.32 q6, a8
	addi a8, a7, 20
	ee.vldbc.32 q7, a8
	l32i a8, a7, 0
	.byte 0x14, 0x24, 0xcd // ld.qr q0, a1, 64
	.byte 0x14, 0xa5, 0xcd // ld.qr q1, a1, 80
	.byte 0x14, 0x26, 0xdd // ld.qr q2, a1, 96
	.byte 0x14, 0xa7, 0xdd // ld.qr q3, a1, 112
	ee.vadds.s32 q0, q0, q5
	ee.vadds.s32 q1, q1, q5
	ee.vadds.s32 q2, q2, q5
	ee.vadds.s32 q3, q3, q5
	ee.vmin.s32 q0, q0, q7
	ee.vmin.s32 q1, q1, q7
	ee.vmin.s32 q2, q2, q7
	ee.vmin.s32 q3, q3, q7
	ee.vmax.s32 q0, q0, q6
	ee.vmax.s32 q1, q1, q6
	ee.vmax.s32 q2, q2, q6
	ee.vmax.s32 q3, q3, q6
	.byte 0x84, 0x60, 0xcd // st.qr q0, a8, 0
	.byte 0x84, 0xe1, 0xcd // st.qr q1, a8, 16
	.byte 0x84, 0x62, 0xdd // st.qr q2, a8, 32
	.byte 0x84, 0xe3, 0xdd // st.qr q3, a8, 48
	retw.n
	.size tinygo_nn_dw_s8_kx1, . - tinygo_nn_dw_s8_kx1

// convolution/esp_nn_depthwise_conv_s8_mult1_3x3_padded_esp32s3.S
.section .text.tinygo_nn_depthwise_conv_s8_mult1_3x3_padded,"ax",@progbits
	.global tinygo_nn_depthwise_conv_s8_mult1_3x3_padded
	.type tinygo_nn_depthwise_conv_s8_mult1_3x3_padded, @function
	.p2align 4
tinygo_nn_depthwise_conv_s8_mult1_3x3_padded:
	entry a1, 320
	mul16u a7, a7, a5
	s32i a3, a1, 116
	s32i a6, a1, 48
	s32i a7, a1, 40
	addi a8, a5, -15
	s32i a2, a1, 76
	l32i a9, a1, 328
	mov.n a2, a5
	s32i a8, a1, 64
	s32i a9, a1, 68
	bgei a8, 1, .Lnn_far_203
	j .Lnn_depthwise_conv_s8_mult1_3x3_padded_Lt_7_4610
.Lnn_far_203:
	l32i a12, a1, 348
	mul16u a15, a3, a5
	movi.n a9, 0
	s32i a12, a1, 56
	s32i a9, a1, 60
	s32i a15, a1, 112
	l32i a9, a1, 352
	slli a15, a15, 1
	s32i a15, a1, 72
	s32i a9, a1, 52
.Lnn_depthwise_conv_s8_mult1_3x3_padded_Lt_7_5122:
	l32i a13, a1, 324
	l32i a6, a1, 60
	l32i a9, a1, 48
	ee.zero.q q2
	add.n a13, a6, a13
	s32i a13, a1, 108
	ee.vld.128.xp q1, a13, a2
	ee.vld.128.xp q3, a13, a2
	ee.vcmp.lt.s8 q0, q1, q2
	ee.vcmp.lt.s8 q4, q3, q2
	ee.vzip.8 q1, q0
	ee.vzip.8 q3, q4
	ee.vadds.s16 q0, q0, q4
	ee.vld.128.xp q4, a13, a2
	ee.vadds.s16 q1, q1, q3
	ee.vcmp.lt.s8 q3, q4, q2
	ee.vzip.8 q4, q3
	ee.vadds.s16 q1, q1, q4
	ee.vld.128.xp q4, a13, a2
	ee.vadds.s16 q0, q0, q3
	ee.vcmp.lt.s8 q3, q4, q2
	ee.vzip.8 q4, q3
	ee.vadds.s16 q0, q0, q3
	ee.vld.128.xp q3, a13, a2
	ee.vadds.s16 q1, q1, q4
	ee.vcmp.lt.s8 q4, q3, q2
	ee.vzip.8 q3, q4
	ee.vadds.s16 q1, q1, q3
	ee.vld.128.xp q3, a13, a2
	ee.vadds.s16 q0, q0, q4
	ee.vcmp.lt.s8 q4, q3, q2
	ee.vzip.8 q3, q4
	ee.vadds.s16 q0, q0, q4
	ee.vld.128.xp q4, a13, a2
	ee.vadds.s16 q1, q1, q3
	ee.vcmp.lt.s8 q3, q4, q2
	ee.vzip.8 q4, q3
	ee.vadds.s16 q1, q1, q4
	ee.vld.128.xp q4, a13, a2
	ee.vadds.s16 q0, q0, q3
	ee.vcmp.lt.s8 q3, q4, q2
	ee.vzip.8 q4, q3
	ee.vadds.s16 q0, q0, q3
	ee.vld.128.xp q3, a13, a2
	ee.vadds.s16 q1, q1, q4
	ee.vcmp.lt.s8 q2, q3, q2
	ee.vzip.8 q3, q2
	ee.vadds.s16 q0, q0, q2
	ee.vadds.s16 q1, q1, q3
	ee.movi.32.a q1, a15, 1
	ee.movi.32.a q1, a8, 3
	ee.movi.32.a q0, a10, 3
	ee.movi.32.a q0, a13, 1
	srai a11, a10, 16
	srai a12, a8, 16
	mull a12, a9, a12
	mull a11, a9, a11
	sext a8, a8, 15
	sext a10, a10, 15
	srai a14, a13, 16
	mull a14, a9, a14
	mull a10, a9, a10
	mull a8, a9, a8
	sext a13, a13, 15
	mull a13, a9, a13
	ee.movi.32.q q3, a11, 3
	ee.movi.32.q q4, a12, 3
	ee.movi.32.q q4, a8, 2
	ee.movi.32.q q3, a10, 2
	ee.movi.32.a q1, a11, 2
	srai a12, a11, 16
	srai a8, a15, 16
	mull a8, a9, a8
	mull a12, a9, a12
	sext a15, a15, 15
	sext a11, a11, 15
	mull a11, a9, a11
	mull a15, a9, a15
	ee.movi.32.q q4, a12, 1
	ee.movi.32.q q1, a8, 3
	ee.movi.32.q q1, a15, 2
	ee.movi.32.q q4, a11, 0
	ee.movi.32.a q0, a15, 2
	ee.movi.32.q q0, a14, 3
	ee.movi.32.q q0, a13, 2
	srai a8, a15, 16
	mull a8, a9, a8
	sext a15, a15, 15
	mull a15, a9, a15
	ee.movi.32.a q0, a11, 0
	srai a13, a11, 16
	ee.movi.32.q q3, a8, 1
	ee.movi.32.q q3, a15, 0
	sext a11, a11, 15
	mull a13, a9, a13
	l32i a8, a1, 332
	ee.movi.32.a q1, a10, 0
	ee.movi.32.q q0, a13, 1
	srai a12, a10, 16
	sext a10, a10, 15
	mull a12, a9, a12
	mull a10, a9, a10
	mull a9, a9, a11
	ee.movi.32.q q1, a12, 1
	ee.movi.32.q q1, a10, 0
	l32i a11, a1, 328
	add.n a6, a6, a8
	ee.movi.32.q q0, a9, 0
	addmi a10, a1, 256
	beqz.n a11, .Lnn_depthwise_conv_s8_mult1_3x3_padded_Lt_7_5378
	l32i a8, a1, 68
	extui a11, a11, 0, 4
	wur.sar_byte a11
	ee.vld.128.ip q5, a8, 16
	ee.vld.128.ip q6, a8, 16
	ee.vld.128.ip q7, a8, 16
	ee.src.q.ld.ip q2, a8, 16, q5, q6
	ee.vadds.s32 q1, q1, q5
	ee.src.q.ld.ip q5, a8, 0, q6, q7
	s32i a8, a1, 68
	ee.vadds.s32 q4, q4, q6
	ee.src.q q7, q7, q2
	ee.src.q q2, q2, q5
	ee.vadds.s32 q0, q0, q7
	ee.vadds.s32 q3, q3, q2
.Lnn_depthwise_conv_s8_mult1_3x3_padded_Lt_7_5378:
	.byte 0xa4, 0x69, 0xed // st.qr q4, a10, -112
	.byte 0xa4, 0xe8, 0xdd // st.qr q3, a10, -128
	.byte 0xa4, 0xea, 0xcd // st.qr q1, a10, -96
	.byte 0xa4, 0x6b, 0xcd // st.qr q0, a10, -80
	movi.n a15, 0
	movi.n a8, 0
	movi.n a9, 0
	s32i a9, a1, 100
	s32i a8, a1, 104
	s32i a15, a1, 96
.Lnn_depthwise_conv_s8_mult1_3x3_padded_Lt_7_6402:
	l32i a4, a1, 104
	l32i a8, a1, 100
	l32i a5, a1, 76
	l32i a3, a1, 60
	l32i a7, a1, 112
	l32i a10, a1, 336
	add.n a4, a4, a5
	mov.n a5, a8
	add.n a3, a3, a4
	l32i a4, a1, 72
	add.n a7, a7, a3
	add.n a8, a8, a10
	s32i a8, a1, 120
	add.n a4, a4, a3
.Lnn_depthwise_conv_s8_mult1_3x3_padded_Lt_7_7170:
	l32i a9, a1, 108
	ee.zero.qacc
	mov.n a12, a3
	mov.n a11, a7
	mov.n a10, a4
	ee.vld.128.xp q0, a12, a2
	ee.vld.128.xp q6, a12, a2
	ee.vld.128.xp q1, a9, a2
	ee.vld.128.xp q7, a9, a2
	ee.vld.128.xp q5, a9, a2
	ee.vld.128.xp q3, a9, a2
	ee.vmulas.s8.qacc.ld.xp q4, a12, a2, q0, q1
	ee.vmulas.s8.qacc.ld.xp q2, a11, a2, q6, q7
	ee.vld.128.xp q1, a9, a2
	ee.vmulas.s8.qacc.ld.xp q0, a11, a2, q4, q5
	ee.vmulas.s8.qacc.ld.xp q6, a11, a2, q2, q3
	ee.vld.128.xp q7, a9, a2
	ee.vld.128.xp q3, a9, a2
	ee.vmulas.s8.qacc.ld.xp q0, a10, a2, q0, q1
	ee.vmulas.s8.qacc.ld.xp q1, a10, a2, q6, q7
	ee.vmulas.s8.qacc.ld.xp q4, a10, a2, q0, q3
	ee.vld.128.xp q6, a9, a2
	ee.vld.128.xp q5, a9, a2
	ee.vmulas.s8.qacc q1, q6
	ee.vmulas.s8.qacc q4, q5
	mov a12, a1
	ee.st.qacc_l.l.128.ip a12, 16
	ee.st.qacc_l.h.32.ip a12, -16
	l32i.n a9, a1, 8
	l32i.n a11, a1, 4
	l32i.n a15, a1, 0
	slli a14, a11, 24
	sext a8, a15, 19
	slli a10, a9, 16
	slli a13, a11, 4
	extui a9, a9, 16, 16
	srai a13, a13, 12
	extui a15, a15, 20, 12
	srai a14, a14, 12
	srai a10, a10, 12
	extui a11, a11, 28, 4
	or a10, a10, a11
	or a14, a14, a15
	ee.movi.32.q q0, a8, 0
	ee.movi.32.q q0, a14, 1
	ee.movi.32.q q0, a13, 2
	ee.movi.32.q q0, a10, 3
	l32i.n a11, a1, 16
	l32i.n a14, a1, 12
	slli a13, a11, 20
	ee.st.qacc_h.l.128.ip a12, 16
	ee.st.qacc_h.h.32.ip a12, -16
	srai a11, a11, 12
	srai a13, a13, 12
	slli a8, a14, 28
	slli a15, a14, 8
	srai a15, a15, 12
	srai a8, a8, 12
	l32i.n a12, a1, 8
	or a8, a8, a9
	extui a14, a14, 24, 8
	l32i.n a9, a1, 0
	or a13, a13, a14
	ee.movi.32.q q3, a8, 0
	ee.movi.32.q q3, a15, 1
	ee.movi.32.q q3, a13, 2
	ee.movi.32.q q3, a11, 3
	l32i.n a14, a1, 4
	sext a10, a9, 19
	extui a9, a9, 20, 12
	slli a13, a12, 16
	slli a8, a14, 24
	extui a12, a12, 16, 16
	srai a13, a13, 12
	srai a8, a8, 12
	slli a15, a14, 4
	srai a15, a15, 12
	or a8, a8, a9
	extui a14, a14, 28, 4
	l32i.n a9, a1, 12
	or a13, a13, a14
	ee.movi.32.q q1, a10, 0
	ee.movi.32.q q1, a8, 1
	ee.movi.32.q q1, a15, 2
	ee.movi.32.q q1, a13, 3
	addmi a14, a1, 256
	.byte 0xe4, 0xa8, 0xfd // ld.qr q7, a14, -128
	.byte 0xe4, 0x29, 0xed // ld.qr q4, a14, -112
	l32i.n a15, a1, 16
	.byte 0xe4, 0x2a, 0xdd // ld.qr q2, a14, -96
	slli a11, a9, 28
	slli a10, a9, 8
	srai a10, a10, 12
	srai a11, a11, 12
	extui a9, a9, 24, 8
	or a11, a11, a12
	ee.vadds.s32 q0, q0, q2
	slli a8, a15, 20
	ee.vadds.s32 q3, q3, q4
	.byte 0x14, 0xe5, 0xdd // st.qr q3, a1, 80
	srai a15, a15, 12
	.byte 0xe4, 0x2b, 0xdd // ld.qr q2, a14, -80
	srai a8, a8, 12
	or a8, a8, a9
	ee.movi.32.q q6, a11, 0
	ee.movi.32.q q6, a10, 1
	ee.movi.32.q q6, a8, 2
	ee.movi.32.q q6, a15, 3
	ee.vadds.s32 q1, q1, q2
	ee.vadds.s32 q6, q6, q7
	.byte 0x14, 0xe1, 0xcd // st.qr q1, a1, 16
	s32i.n a7, a1, 32
	s32i.n a6, a1, 36
	l32i a7, a1, 52
	l32i a6, a1, 56
	mov a10, a7
	mov a11, a6
	call8 tinygo_nn_multiply_by_quantized_mult_ver1
	mv.qr q5, q0
	.byte 0x14, 0x25, 0xcd // ld.qr q0, a1, 80
	addi a10, a7, 16
	addi a11, a6, 16
	call8 tinygo_nn_multiply_by_quantized_mult_ver1
	mv.qr q4, q0
	.byte 0x14, 0x21, 0xcd // ld.qr q0, a1, 16
	addi a10, a7, 32
	addi a11, a6, 32
	call8 tinygo_nn_multiply_by_quantized_mult_ver1
	.byte 0x14, 0x60, 0xcd // st.qr q0, a1, 0
	mv.qr q0, q6
	addi a10, a7, 48
	addi a11, a6, 48
	call8 tinygo_nn_multiply_by_quantized_mult_ver1
	l32i.n a6, a1, 36
	l32i.n a7, a1, 32
	l32i a15, a1, 40
	l32i a11, a1, 120
	add.n a3, a3, a15
	add.n a4, a4, a15
	add.n a7, a7, a15
	addi.n a5, a5, 1
	addmi a13, a1, 256
	.byte 0x14, 0xa0, 0xdd // ld.qr q3, a1, 0
	mv.qr q2, q5
	addi a8, a13, 88
	addi a9, a13, 100
	addi a15, a13, 104
	ee.vldbc.32 q6, a9
	ee.vldbc.32 q1, a8
	ee.vldbc.32 q7, a15
	ee.vadds.s32 q4, q4, q1
	ee.vadds.s32 q2, q2, q1
	ee.vadds.s32 q5, q0, q1
	ee.vadds.s32 q3, q3, q1
	ee.vmin.s32 q3, q3, q7
	ee.vmin.s32 q5, q5, q7
	ee.vmin.s32 q2, q2, q7
	ee.vmin.s32 q4, q4, q7
	ee.vmax.s32 q4, q4, q6
	ee.vmax.s32 q2, q2, q6
	ee.vmax.s32 q5, q5, q6
	ee.vmax.s32 q3, q3, q6
	ee.vunzip.16 q3, q5
	ee.vunzip.16 q2, q4
	ee.vunzip.8 q2, q3
	ee.vst.128.xp q2, a6, a2
	beq a5, a11, .Lnn_far_202
	j .Lnn_depthwise_conv_s8_mult1_3x3_padded_Lt_7_7170
.Lnn_far_202:
.Lnn_depthwise_conv_s8_mult1_3x3_padded_Lt_7_6658:
	l32i a15, a1, 112
	l32i a10, a1, 320
	l32i a13, a1, 340
	l32i a9, a1, 116
	l32i a12, a1, 96
	mull a15, a10, a15
	l32i a14, a1, 104
	l32i a8, a1, 100
	addi.n a12, a12, 1
	s32i a12, a1, 96
	add.n a14, a14, a15
	add.n a8, a8, a9
	s32i a8, a1, 100
	s32i a14, a1, 104
	beq a12, a13, .Lnn_far_201
	j .Lnn_depthwise_conv_s8_mult1_3x3_padded_Lt_7_6402
.Lnn_far_201:
	l32i a11, a1, 56
	l32i a15, a1, 52
	l32i a10, a1, 60
	addi a11, a11, 64
	addi a15, a15, 64
	s32i a11, a1, 56
	s32i a15, a1, 52
	l32i a11, a1, 64
	addi a10, a10, 16
	s32i a10, a1, 60
	bge a10, a11, .Lnn_far_200
	j .Lnn_depthwise_conv_s8_mult1_3x3_padded_Lt_7_5122
.Lnn_far_200:
.Lnn_depthwise_conv_s8_mult1_3x3_padded_Lt_7_4610:
	retw.n
	.size tinygo_nn_depthwise_conv_s8_mult1_3x3_padded, . - tinygo_nn_depthwise_conv_s8_mult1_3x3_padded

// fully_connected/esp_nn_fully_connected_per_ch_s8_esp32s3.S
.section .text.tinygo_nn_fc_per_ch_s16,"ax",@progbits
	.global tinygo_nn_fc_per_ch_s16
	.type tinygo_nn_fc_per_ch_s16, @function
	.p2align 4
tinygo_nn_fc_per_ch_s16:
	entry a1, 112
	s32i.n a5, a1, 60
	s32i a7, a1, 48
	s32i a6, a1, 40
	s32i a3, a1, 44
	mov.n a13, a2
	mov.n a12, a4
	l16ui a2, a1, 116
	addi a4, a1, 40
	addi a8, a1, 44
	ee.vldbc.16 q5, a8
	ee.vldbc.16 q6, a4
	beqz a2, .Lnn_fc_per_ch_s16_Lt_0_7938
	ee.zero.q q7
	srai a11, a12, 3
	l32i a8, a1, 112
	addi a9, a12, -7
	s32i a9, a1, 76
	s32i a8, a1, 72
	s32i a11, a1, 64
	slli a11, a11, 3
	s32i a11, a1, 68
	movi.n a15, 0
	mov.n a14, a7
	mov.n a11, a5
	l32i a10, a1, 124
	l32i a2, a1, 128
	s32i a10, a1, 80
	s32i a2, a1, 84
	movi.n a10, 0
	mov.n a2, a11
.Lnn_fc_per_ch_s16_Lt_0_8450:
	l32i a9, a1, 76
	extui a5, a11, 0, 3
	ee.zero.accx
	slli a5, a5, 1
	bgez a9, .Lnn_fc_per_ch_s16_LBB6_esp_nn_fc_per_ch_s16_esp32s3
	mov.n a5, a10
	movi.n a2, 0
	j .Lnn_fc_per_ch_s16_Lt_0_8706
.Lnn_fc_per_ch_s16_LBB6_esp_nn_fc_per_ch_s16_esp32s3:
	wur.sar_byte a5
	ee.vld.l.64.ip q4, a2, 8
	l32i a4, a1, 64
	mov.n a3, a13
	addx8 a5, a4, a10
	ee.vcmp.lt.s8 q2, q4, q7
	ee.vzip.8 q4, q2
	loopgtz a4, .Lnn_fc_per_ch_s16_LBB45_esp_nn_fc_per_ch_s16_esp32s3
	ee.vld.l.64.ip q0, a2, 8
	ee.vld.l.64.ip q1, a3, 8
	ee.vcmp.lt.s8 q2, q0, q7
	ee.vcmp.lt.s8 q3, q1, q7
	ee.vzip.8 q0, q2
	ee.vzip.8 q1, q3
	ee.vadds.s16 q1, q1, q5
	ee.src.q.qup q2, q4, q0
	ee.vadds.s16 q2, q2, q6
	ee.vmulas.s16.accx q1, q2
.Lnn_fc_per_ch_s16_LBB45_esp_nn_fc_per_ch_s16_esp32s3:
	l32i a2, a1, 68
.Lnn_fc_per_ch_s16_Lt_0_8706:
	movi a9, 0
	ee.srs.accx a6, a9, 0
	bge a2, a12, .Lnn_fc_per_ch_s16_Lt_0_9730
	l32i a8, a1, 44
	l32i a7, a1, 40
	sub a3, a12, a2
	l32i.n a4, a1, 60
	add.n a2, a2, a13
	add.n a4, a4, a5
	nop.n
	loopgtz a3, .Lnn_fc_per_ch_s16_LBB60_esp_nn_fc_per_ch_s16_esp32s3
	l8ui a3, a2, 0
	l8ui a5, a4, 0
	sext a3, a3, 7
	sext a5, a5, 7
	add.n a5, a5, a7
	add.n a3, a3, a8
	mull a3, a3, a5
	addi.n a2, a2, 1
	addi.n a4, a4, 1
	add.n a6, a6, a3
.Lnn_fc_per_ch_s16_LBB60_esp_nn_fc_per_ch_s16_esp32s3:
.Lnn_fc_per_ch_s16_Lt_0_9730:
	l32i a8, a1, 48
	beqz.n a8, .Lnn_fc_per_ch_s16_Lt_0_10754
	l32i.n a9, a14, 0
	add.n a6, a6, a9
.Lnn_fc_per_ch_s16_Lt_0_10754:
	movi a4, 0
	l32i a5, a1, 80
	l32i a5, a5, 0
	max a2, a5, a4
	sub a5, a2, a5
	ssl a2
	sll a6, a6
	l32i a4, a1, 84
	movi a3, 1
	slli a3, a3, 30
	add.n a10, a10, a12
	addi.n a14, a14, 4
	l32i a4, a4, 0
	add.n a11, a11, a12
	ssai 31
	mulsh a7, a4, a6
	mull a4, a4, a6
	mov.n a2, a11
	add a4, a4, a3
	.byte 0x30, 0x84, 0x62 // saltu a8, a4, a3
	add.n a7, a7, a8
	src a3, a7, a4
	blti a5, 1, .Lnn_fc_per_ch_s16_skip_divide_by2
	movi.n a8, 1
	addi a4, a5, -1
	ssl a4
	sll a8, a8
	extui a6, a3, 31, 1
	sub a8, a8, a6
	add a3, a3, a8
	ssr a5
	sra a3, a3
.Lnn_fc_per_ch_s16_skip_divide_by2:
	l32i a8, a1, 120
	l32i a7, a1, 132
	l32i a4, a1, 136
	add.n a8, a8, a3
	l32i a6, a1, 72
	l32i a3, a1, 116
	max a7, a7, a8
	add.n a6, a15, a6
	min a4, a4, a7
	addi.n a15, a15, 1
	l32i a7, a1, 84
	l32i a8, a1, 80
	s8i a4, a6, 0
	addi.n a7, a7, 4
	addi.n a8, a8, 4
	s32i a7, a1, 84
	s32i a8, a1, 80
	beq a3, a15, .Lnn_far_198
	j .Lnn_fc_per_ch_s16_Lt_0_8450
.Lnn_far_198:
.Lnn_fc_per_ch_s16_Lt_0_7938:
	retw.n
	.size tinygo_nn_fc_per_ch_s16, . - tinygo_nn_fc_per_ch_s16

// fully_connected/esp_nn_fully_connected_s8_esp32s3.S
.section .text.tinygo_nn_fc_s16,"ax",@progbits
	.global tinygo_nn_fc_s16
	.type tinygo_nn_fc_s16, @function
	.p2align 4
tinygo_nn_fc_s16:
	entry a1, 112
	s32i.n a5, a1, 60
	s32i a7, a1, 48
	s32i a6, a1, 40
	s32i a3, a1, 44
	mov.n a13, a2
	mov.n a12, a4
	l16ui a2, a1, 116
	addi a4, a1, 40
	addi a8, a1, 44
	ee.vldbc.16 q5, a8
	ee.vldbc.16 q6, a4
	beqz a2, .Lnn_fc_s16_Lt_0_7938
	ee.zero.q q7
	srai a11, a12, 3
	l32i a10, a1, 128
	l32i a8, a1, 112
	addi a9, a12, -7
	s32i a9, a1, 76
	s32i a8, a1, 72
	s32i a11, a1, 64
	slli a11, a11, 3
	s32i a11, a1, 68
	l32i a10, a1, 124
	movi.n a15, 0
	mov.n a14, a7
	max a11, a10, a15
	s32i a11, a1, 52
	sub a10, a11, a10
	s32i.n a10, a1, 56
	mov.n a11, a5
	movi.n a10, 0
	mov.n a2, a11
.Lnn_fc_s16_Lt_0_8450:
	l32i a9, a1, 76
	extui a5, a11, 0, 3
	ee.zero.accx
	slli a5, a5, 1
	bgez a9, .Lnn_fc_s16_LBB6_esp_nn_fc_s16_esp32s3
	mov.n a5, a10
	movi.n a2, 0
	j .Lnn_fc_s16_Lt_0_8706
.Lnn_fc_s16_LBB6_esp_nn_fc_s16_esp32s3:
	wur.sar_byte a5
	ee.vld.l.64.ip q4, a2, 8
	l32i a4, a1, 64
	mov.n a3, a13
	addx8 a5, a4, a10
	ee.vcmp.lt.s8 q2, q4, q7
	ee.vzip.8 q4, q2
	loopgtz a4, .Lnn_fc_s16_LBB45_esp_nn_fc_s16_esp32s3
	ee.vld.l.64.ip q0, a2, 8
	ee.vld.l.64.ip q1, a3, 8
	ee.vcmp.lt.s8 q2, q0, q7
	ee.vcmp.lt.s8 q3, q1, q7
	ee.vzip.8 q0, q2
	ee.vzip.8 q1, q3
	ee.vadds.s16 q1, q1, q5
	ee.src.q.qup q2, q4, q0
	ee.vadds.s16 q2, q2, q6
	ee.vmulas.s16.accx q1, q2
.Lnn_fc_s16_LBB45_esp_nn_fc_s16_esp32s3:
	l32i a2, a1, 68
.Lnn_fc_s16_Lt_0_8706:
	movi a9, 0
	ee.srs.accx a6, a9, 0
	bge a2, a12, .Lnn_fc_s16_Lt_0_9730
	l32i a8, a1, 44
	l32i a7, a1, 40
	sub a3, a12, a2
	l32i.n a4, a1, 60
	add.n a2, a2, a13
	add.n a4, a4, a5
	loopgtz a3, .Lnn_fc_s16_LBB60_esp_nn_fc_s16_esp32s3
	l8ui a3, a2, 0
	l8ui a5, a4, 0
	sext a3, a3, 7
	sext a5, a5, 7
	add.n a5, a5, a7
	add.n a3, a3, a8
	mull a3, a3, a5
	addi.n a2, a2, 1
	addi.n a4, a4, 1
	add.n a6, a6, a3
.Lnn_fc_s16_LBB60_esp_nn_fc_s16_esp32s3:
.Lnn_fc_s16_Lt_0_9730:
	l32i a8, a1, 48
	beqz.n a8, .Lnn_fc_s16_Lt_0_10754
	l32i.n a9, a14, 0
	add.n a6, a6, a9
.Lnn_fc_s16_Lt_0_10754:
	l32i a2, a1, 52
	l32i a5, a1, 56
	ssl a2
	sll a6, a6
	movi a3, 1
	slli a3, a3, 30
	add.n a10, a10, a12
	addi.n a14, a14, 4
	l32i a4, a1, 128
	add.n a11, a11, a12
	ssai 31
	mulsh a7, a4, a6
	mull a4, a4, a6
	mov.n a2, a11
	add a4, a4, a3
	.byte 0x30, 0x84, 0x62 // saltu a8, a4, a3
	add.n a7, a7, a8
	src a3, a7, a4
	blti a5, 1, .Lnn_fc_s16_skip_divide_by2
	movi.n a8, 1
	addi a4, a5, -1
	ssl a4
	sll a8, a8
	extui a6, a3, 31, 1
	sub a8, a8, a6
	add a3, a3, a8
	ssr a5
	sra a3, a3
.Lnn_fc_s16_skip_divide_by2:
	l32i a8, a1, 120
	l32i a7, a1, 132
	l32i a4, a1, 136
	add.n a8, a8, a3
	l32i a6, a1, 72
	l32i a3, a1, 116
	max a7, a7, a8
	add.n a6, a15, a6
	min a4, a4, a7
	addi.n a15, a15, 1
	s8i a4, a6, 0
	beq a3, a15, .Lnn_far_196
	j .Lnn_fc_s16_Lt_0_8450
.Lnn_far_196:
.Lnn_fc_s16_Lt_0_7938:
	retw.n
	.size tinygo_nn_fc_s16, . - tinygo_nn_fc_s16

// pooling/esp_nn_avg_pool_s8_esp32s3.S

// The output row stride used output_ht, and a spill slot at a1+280 was in the
// frame of the caller. The fix uses output_wd and a1+220.
.section .text.tinygo_nn_avg_pool_s8,"ax",@progbits
	.global tinygo_nn_avg_pool_s8
	.type tinygo_nn_avg_pool_s8, @function
	.p2align 4
tinygo_nn_avg_pool_s8:
	entry a1, 240
	mov.n a11, a3
	mov.n a12, a2
	s32i a5, a1, 136
	s32i a6, a1, 128
	l16ui a5, a1, 272
	s32i a7, a1, 72
	l32i a9, a1, 264
	l32i a10, a1, 268
	s32i.n a9, a1, 0
	s32i.n a10, a1, 4
	addi.n a8, a1, 4
	ee.vldbc.32 q7, a1
	ee.vldbc.32 q6, a8
	ee.zero.q q4
	extui a10, a5, 0, 3
	beqz.n a10, .Lnn_avg_pool_s8_LBB3_esp_nn_avg_pool_s8_esp32s3_asm
	extui a13, a5, 0, 2
	beqz a13, .Lnn_avg_pool_s8_LBB52_esp_nn_avg_pool_s8_esp32s3_asm
.Lnn_avg_pool_s8_Lt_0_44546:
	retw.n
.Lnn_avg_pool_s8_LBB3_esp_nn_avg_pool_s8_esp32s3_asm:
	l16ui a7, a1, 256
	l16ui a10, a1, 260
	l32i a15, a1, 128
	movi.n a14, 0
	movi.n a8, 0
	neg a10, a10
	s32i a10, a1, 56
	s32i a8, a1, 44
	s32i.n a14, a1, 20
	sub a9, a4, a10
	s32i a9, a1, 40
	mul16u a15, a15, a5
	neg a13, a7
	s32i a13, a1, 104
	s32i.n a15, a1, 16
	sub a13, a3, a13
	s32i.n a13, a1, 12
	j .Lnn_avg_pool_s8_Lt_0_28162
.Lnn_avg_pool_s8_Lt_0_28418:
	l32i a15, a1, 260
	l32i a14, a1, 56
	l32i.n a9, a1, 16
	l32i a13, a1, 244
	l32i a10, a1, 40
	l32i a8, a1, 44
	sub a10, a10, a13
	add.n a8, a8, a9
	add.n a14, a14, a13
	sub a15, a15, a13
	s32i a15, a1, 260
	s32i a14, a1, 56
	s32i a8, a1, 44
	s32i a10, a1, 40
	l32i.n a8, a1, 20
	l32i a9, a1, 72
	addi.n a8, a8, 1
	s32i.n a8, a1, 20
	beq a8, a9, .Lnn_avg_pool_s8_Lt_0_44546
.Lnn_avg_pool_s8_Lt_0_28162:
	l32i a10, a1, 128
	beqz a10, .Lnn_avg_pool_s8_Lt_0_28418
.Lnn_avg_pool_s8_LBB7_esp_nn_avg_pool_s8_esp32s3_asm:
	s32i a7, a1, 112
	movi.n a10, 0
	l32i a9, a1, 260
	l32i.n a6, a1, 12
	l32i a8, a1, 44
	movi.n a13, 0
	l32i a15, a1, 104
	s32i a15, a1, 116
	s32i a13, a1, 48
	s32i a8, a1, 124
	s32i a6, a1, 120
	l32i a8, a1, 40
	l32i a6, a1, 252
	movi.n a13, 0
	max a9, a9, a10
	s32i a9, a1, 160
	s32i a13, a1, 92
	min a6, a6, a8
	bnez.n a5, .Lnn_avg_pool_s8_LBB10_esp_nn_avg_pool_s8_esp32s3_asm
.Lnn_avg_pool_s8_Lt_0_29186:
	l32i a8, a1, 116
	l32i a15, a1, 120
	l32i a9, a1, 48
	l32i a14, a1, 240
	l32i a10, a1, 124
	l32i a13, a1, 112
	add.n a10, a10, a5
	s32i a10, a1, 124
	sub a13, a13, a14
	add.n a9, a9, a14
	sub a15, a15, a14
	add.n a8, a8, a14
	s32i a8, a1, 116
	s32i a15, a1, 120
	s32i a9, a1, 48
	s32i a13, a1, 112
	l32i a9, a1, 92
	l32i a10, a1, 128
	addi.n a9, a9, 1
	s32i a9, a1, 92
	bne a9, a10, .Lnn_far_193
	j .Lnn_avg_pool_s8_Lt_0_28418
.Lnn_far_193:
.Lnn_avg_pool_s8_Lt_0_28930:
	beqz a5, .Lnn_avg_pool_s8_Lt_0_29186
.Lnn_avg_pool_s8_LBB10_esp_nn_avg_pool_s8_esp32s3_asm:
	l32i a14, a1, 120
	l32i a13, a1, 248
	l32i a9, a1, 136
	l32i a8, a1, 124
	movi.n a15, 0
	s32i a15, a1, 24
	add.n a10, a8, a5
	movi.n a15, 0
	add.n a8, a8, a9
	min a13, a13, a14
	add.n a10, a9, a10
	s32i a10, a1, 180
	s32i a13, a1, 76
	l32i a14, a1, 112
	s32i a8, a1, 148
	max a14, a14, a15
	l32i a15, a1, 116
	s32i a14, a1, 152
	add.n a8, a15, a14
	s32i a8, a1, 36
	add.n a15, a15, a13
	s32i a15, a1, 204
	sub a13, a13, a14
	s32i a13, a1, 220
	j .Lnn_avg_pool_s8_Lt_0_29698
.Lnn_avg_pool_s8_LBB13_esp_nn_avg_pool_s8_esp32s3_asm:
	l32i a10, a1, 56
	l32i a14, a1, 204
	add.n a10, a10, a15
	mull a10, a11, a10
	movi.n a15, 0
	add.n a14, a10, a14
.Lnn_avg_pool_s8_Lt_0_30466:
	l32i a9, a1, 76
	l32i a8, a1, 152
	add.n a14, a14, a11
	bge a8, a9, .Lnn_avg_pool_s8_Lt_0_30722
.Lnn_avg_pool_s8_LBB16_esp_nn_avg_pool_s8_esp32s3_asm:
	l32i a3, a1, 36
	l32i a2, a1, 24
	add.n a3, a3, a10
	mull a3, a3, a5
	movi.n a8, 0
	add.n a2, a2, a3
	l32i a3, a1, 220
	add.n a2, a12, a2
	loopgtz a3, .Lnn_avg_pool_s8_LBB140_esp_nn_avg_pool_s8_esp32s3_asm
	ee.vld.l.64.xp q0, a2, a5
	ee.vcmp.lt.s8 q1, q0, q4
	ee.vzip.8 q0, q1
	ee.vcmp.lt.s16 q1, q0, q4
	ee.vzip.16 q0, q1
	ee.vadds.s32 q2, q2, q1
	ee.vadds.s32 q3, q3, q0
.Lnn_avg_pool_s8_LBB140_esp_nn_avg_pool_s8_esp32s3_asm:
	l32i a2, a1, 48
	sub a9, a7, a2
	sub a2, a2, a7
	max a9, a9, a8
	l32i a8, a1, 248
	sub a2, a11, a2
	min a8, a8, a2
	sub a8, a8, a9
	add.n a15, a15, a8
.Lnn_avg_pool_s8_Lt_0_30722:
	add.n a10, a10, a11
	addi.n a13, a13, 1
	bne a6, a13, .Lnn_avg_pool_s8_Lt_0_30466
.Lnn_avg_pool_s8_Lt_0_29954:
	srai a2, a15, 1
	ee.movi.32.a q3, a9, 0
	ee.movi.32.a q3, a4, 1
	blti a9, 1, .Lnn_avg_pool_s8_Lt_0_32258
	add.n a9, a9, a2
	j .Lnn_avg_pool_s8_Lt_0_32002
.Lnn_avg_pool_s8_Lt_0_32258:
	sub a9, a9, a2
.Lnn_avg_pool_s8_Lt_0_32002:
	blti a4, 1, .Lnn_avg_pool_s8_Lt_0_32770
	add.n a4, a2, a4
	j .Lnn_avg_pool_s8_Lt_0_32514
.Lnn_avg_pool_s8_Lt_0_32770:
	sub a4, a4, a2
.Lnn_avg_pool_s8_Lt_0_32514:
	quos a9, a9, a15
	quos a4, a4, a15
	ee.movi.32.q q3, a9, 0
	ee.movi.32.q q3, a4, 1
	ee.movi.32.a q3, a9, 2
	ee.movi.32.a q3, a14, 3
	blti a9, 1, .Lnn_avg_pool_s8_Lt_0_33282
	add.n a9, a9, a2
	j .Lnn_avg_pool_s8_Lt_0_33026
.Lnn_avg_pool_s8_Lt_0_33282:
	sub a9, a9, a2
.Lnn_avg_pool_s8_Lt_0_33026:
	blti a14, 1, .Lnn_avg_pool_s8_Lt_0_33794
	add.n a14, a2, a14
	j .Lnn_avg_pool_s8_Lt_0_33538
.Lnn_avg_pool_s8_Lt_0_33794:
	sub a14, a14, a2
.Lnn_avg_pool_s8_Lt_0_33538:
	quos a9, a9, a15
	quos a14, a14, a15
	ee.movi.32.q q3, a9, 2
	ee.movi.32.q q3, a14, 3
	ee.movi.32.a q2, a9, 0
	ee.movi.32.a q2, a4, 1
	blti a9, 1, .Lnn_avg_pool_s8_Lt_0_34306
	add.n a9, a9, a2
	j .Lnn_avg_pool_s8_Lt_0_34050
.Lnn_avg_pool_s8_Lt_0_34306:
	sub a9, a9, a2
.Lnn_avg_pool_s8_Lt_0_34050:
	blti a4, 1, .Lnn_avg_pool_s8_Lt_0_34818
	add.n a4, a2, a4
	j .Lnn_avg_pool_s8_Lt_0_34562
.Lnn_avg_pool_s8_Lt_0_34818:
	sub a4, a4, a2
.Lnn_avg_pool_s8_Lt_0_34562:
	quos a9, a9, a15
	quos a4, a4, a15
	ee.movi.32.q q2, a9, 0
	ee.movi.32.q q2, a4, 1
	ee.movi.32.a q2, a9, 2
	ee.movi.32.a q2, a14, 3
	blti a9, 1, .Lnn_avg_pool_s8_Lt_0_35330
	add.n a9, a9, a2
	j .Lnn_avg_pool_s8_Lt_0_35074
.Lnn_avg_pool_s8_Lt_0_35330:
	sub a9, a9, a2
.Lnn_avg_pool_s8_Lt_0_35074:
	blti a14, 1, .Lnn_avg_pool_s8_Lt_0_35842
	add.n a14, a2, a14
	j .Lnn_avg_pool_s8_Lt_0_35586
.Lnn_avg_pool_s8_Lt_0_35842:
	sub a14, a14, a2
.Lnn_avg_pool_s8_Lt_0_35586:
	quos a9, a9, a15
	quos a14, a14, a15
	ee.movi.32.q q2, a9, 2
	ee.movi.32.q q2, a14, 3
	l32i a9, a1, 180
	l32i a14, a1, 24
	l32i a13, a1, 148
	ee.vmin.s32 q1, q3, q6
	ee.vmax.s32 q1, q1, q7
	ee.vmin.s32 q5, q2, q6
	addi.n a14, a14, 8
	s32i a14, a1, 24
	ee.vmax.s32 q5, q5, q7
	addi.n a8, a13, 8
	s32i a8, a1, 148
	ee.vunzip.16 q1, q5
	ee.vunzip.8 q1, q5
	ee.vst.l.64.ip q1, a13, 0
	blt a8, a9, .Lnn_far_191
	j .Lnn_avg_pool_s8_Lt_0_29186
.Lnn_far_191:
.Lnn_avg_pool_s8_Lt_0_29698:
	mv.qr q3, q4
	l32i a15, a1, 160
	mv.qr q2, q4
	mov.n a13, a15
	bge a15, a6, .Lnn_far_190
	j .Lnn_avg_pool_s8_LBB13_esp_nn_avg_pool_s8_esp32s3_asm
.Lnn_far_190:
.Lnn_avg_pool_s8_Lt_0_51458:
	movi.n a15, 0
	j .Lnn_avg_pool_s8_Lt_0_29954
.Lnn_avg_pool_s8_LBB52_esp_nn_avg_pool_s8_esp32s3_asm:
	l16ui a7, a1, 256
	l16ui a13, a1, 260
	s32i a13, a1, 64
	l32i a8, a1, 128
	movi.n a15, 0
	movi.n a9, 0
	neg a13, a13
	s32i a13, a1, 192
	s32i a9, a1, 32
	s32i.n a15, a1, 8
	sub a10, a4, a13
	s32i a10, a1, 28
	mul16u a8, a8, a5
	neg a14, a7
	s32i a14, a1, 104
	s32i.n a8, a1, 16
	sub a14, a3, a14
	s32i.n a14, a1, 12
	j .Lnn_avg_pool_s8_Lt_0_37890
.Lnn_avg_pool_s8_Lt_0_38146:
	l32i a15, a1, 64
	l32i a14, a1, 192
	l32i.n a9, a1, 16
	l32i a13, a1, 244
	l32i a10, a1, 28
	l32i a8, a1, 32
	sub a10, a10, a13
	add.n a8, a8, a9
	add.n a14, a14, a13
	sub a15, a15, a13
	s32i a15, a1, 64
	s32i a14, a1, 192
	s32i a8, a1, 32
	s32i a10, a1, 28
	l32i.n a8, a1, 8
	l32i a9, a1, 72
	addi.n a8, a8, 1
	s32i.n a8, a1, 8
	sub a8, a8, a9
	beqz a8, .Lnn_avg_pool_s8_Lt_0_44546
.Lnn_avg_pool_s8_Lt_0_37890:
	l32i a10, a1, 128
	beqz a10, .Lnn_avg_pool_s8_Lt_0_38146
	s32i a7, a1, 96
	movi.n a10, 0
	l32i a9, a1, 64
	l32i.n a6, a1, 12
	l32i a8, a1, 32
	movi.n a13, 0
	l32i a15, a1, 104
	s32i a15, a1, 100
	s32i a13, a1, 148
	s32i a8, a1, 108
	s32i a6, a1, 144
	l32i a8, a1, 28
	l32i a6, a1, 252
	max a9, a9, a10
	s32i a9, a1, 168
	s32i a13, a1, 88
	min a6, a6, a8
	bnez.n a5, .Lnn_avg_pool_s8_LBB59_esp_nn_avg_pool_s8_esp32s3_asm
.Lnn_avg_pool_s8_Lt_0_38914:
	l32i a8, a1, 100
	l32i a15, a1, 144
	l32i a9, a1, 148
	l32i a14, a1, 240
	l32i a10, a1, 108
	l32i a13, a1, 96
	add.n a10, a10, a5
	s32i a10, a1, 108
	sub a13, a13, a14
	add.n a9, a9, a14
	sub a15, a15, a14
	add.n a8, a8, a14
	s32i a8, a1, 100
	s32i a15, a1, 144
	s32i a9, a1, 148
	s32i a13, a1, 96
	l32i a9, a1, 88
	l32i a10, a1, 128
	addi.n a9, a9, 1
	s32i a9, a1, 88
	bne a9, a10, .Lnn_far_188
	j .Lnn_avg_pool_s8_Lt_0_38146
.Lnn_far_188:
	beqz a5, .Lnn_avg_pool_s8_Lt_0_38914
.Lnn_avg_pool_s8_LBB59_esp_nn_avg_pool_s8_esp32s3_asm:
	l32i a14, a1, 144
	l32i a13, a1, 248
	l32i a9, a1, 136
	l32i a8, a1, 108
	movi.n a15, 0
	s32i a15, a1, 216
	add.n a10, a8, a5
	add.n a8, a8, a9
	min a13, a13, a14
	add.n a10, a9, a10
	s32i a10, a1, 172
	s32i a13, a1, 132
	l32i a14, a1, 96
	s32i a8, a1, 164
	max a14, a14, a15
	l32i a15, a1, 100
	s32i a14, a1, 208
	add.n a8, a15, a14
	s32i a8, a1, 60
	add.n a15, a15, a13
	s32i a15, a1, 196
	sub a13, a13, a14
	s32i a13, a1, 52
	j .Lnn_avg_pool_s8_Lt_0_39426
.Lnn_avg_pool_s8_LBB62_esp_nn_avg_pool_s8_esp32s3_asm:
	l32i a10, a1, 192
	l32i a14, a1, 196
	add.n a10, a10, a15
	mull a10, a11, a10
	movi.n a15, 0
	add.n a14, a10, a14
.Lnn_avg_pool_s8_Lt_0_40194:
	l32i a9, a1, 132
	l32i a8, a1, 208
	add.n a14, a14, a11
	bge a8, a9, .Lnn_avg_pool_s8_Lt_0_40450
.Lnn_avg_pool_s8_LBB65_esp_nn_avg_pool_s8_esp32s3_asm:
	l32i a3, a1, 60
	l32i a2, a1, 216
	add.n a3, a3, a10
	mull a3, a3, a5
	l32i a4, a1, 52
	add.n a2, a2, a3
	add.n a2, a12, a2
	loopgtz a4, .Lnn_avg_pool_s8_LBB155_esp_nn_avg_pool_s8_esp32s3_asm
	ee.vldbc.32.xp q0, a2, a5
	ee.vcmp.lt.s8 q1, q0, q4
	ee.vzip.8 q0, q1
	ee.vcmp.lt.s16 q1, q0, q4
	ee.vzip.16 q0, q1
	ee.vadds.s32 q2, q2, q0
.Lnn_avg_pool_s8_LBB155_esp_nn_avg_pool_s8_esp32s3_asm:
	l32i a2, a1, 148
	movi.n a8, 0
	sub a9, a7, a2
	sub a2, a2, a7
	max a9, a9, a8
	l32i a8, a1, 248
	sub a2, a11, a2
	min a8, a8, a2
	sub a8, a8, a9
	add.n a15, a15, a8
.Lnn_avg_pool_s8_Lt_0_40450:
	add.n a10, a10, a11
	addi.n a13, a13, 1
	bne a6, a13, .Lnn_avg_pool_s8_Lt_0_40194
.Lnn_avg_pool_s8_Lt_0_39682:
	srai a2, a15, 1
	ee.movi.32.a q2, a9, 0
	ee.movi.32.a q2, a4, 1
	blti a9, 1, .Lnn_avg_pool_s8_Lt_0_41986
	add.n a9, a9, a2
	j .Lnn_avg_pool_s8_Lt_0_41730
.Lnn_avg_pool_s8_Lt_0_41986:
	sub a9, a9, a2
.Lnn_avg_pool_s8_Lt_0_41730:
	blti a4, 1, .Lnn_avg_pool_s8_Lt_0_42498
	add.n a4, a2, a4
	j .Lnn_avg_pool_s8_Lt_0_42242
.Lnn_avg_pool_s8_Lt_0_42498:
	sub a4, a4, a2
.Lnn_avg_pool_s8_Lt_0_42242:
	quos a9, a9, a15
	quos a4, a4, a15
	ee.movi.32.q q2, a9, 0
	ee.movi.32.q q2, a4, 1
	ee.movi.32.a q2, a9, 2
	ee.movi.32.a q2, a14, 3
	blti a9, 1, .Lnn_avg_pool_s8_Lt_0_43010
	add.n a9, a9, a2
	j .Lnn_avg_pool_s8_Lt_0_42754
.Lnn_avg_pool_s8_Lt_0_43010:
	sub a9, a9, a2
.Lnn_avg_pool_s8_Lt_0_42754:
	blti a14, 1, .Lnn_avg_pool_s8_Lt_0_43522
	add.n a14, a2, a14
	j .Lnn_avg_pool_s8_Lt_0_43266
.Lnn_avg_pool_s8_Lt_0_43522:
	sub a14, a14, a2
.Lnn_avg_pool_s8_Lt_0_43266:
	quos a9, a9, a15
	quos a14, a14, a15
	ee.movi.32.q q2, a9, 2
	ee.movi.32.q q2, a14, 3
	l32i a9, a1, 172
	l32i a8, a1, 164
	l32i a14, a1, 216
	addi.n a14, a14, 4
	ee.vmin.s32 q2, q2, q6
	s32i a14, a1, 216
	ee.vmax.s32 q2, q2, q7
	ee.vunzip.16 q2, q1
	ee.vunzip.8 q2, q1
	ee.vst.l.64.ip q2, a1, 0
	l32i.n a13, a1, 0
	s32i.n a13, a8, 0
	addi.n a8, a8, 4
	s32i a8, a1, 164
	blt a8, a9, .Lnn_far_186
	j .Lnn_avg_pool_s8_Lt_0_38914
.Lnn_far_186:
.Lnn_avg_pool_s8_Lt_0_39426:
	l32i a15, a1, 168
	mv.qr q2, q4
	mov.n a13, a15
	bge a15, a6, .Lnn_far_185
	j .Lnn_avg_pool_s8_LBB62_esp_nn_avg_pool_s8_esp32s3_asm
.Lnn_far_185:
.Lnn_avg_pool_s8_Lt_0_52738:
	movi.n a15, 0
	j .Lnn_avg_pool_s8_Lt_0_39682
	.size tinygo_nn_avg_pool_s8, . - tinygo_nn_avg_pool_s8

// pooling/esp_nn_max_pool_s8_esp32s3.S

// The 4 channel path clamped into q5, the activation minimum. The fix uses q2.
.section .text.tinygo_nn_max_pool_s8,"ax",@progbits
	.global tinygo_nn_max_pool_s8
	.type tinygo_nn_max_pool_s8, @function
	.p2align 4
tinygo_nn_max_pool_s8:
	entry a1, 120
	mov.n a12, a2
	s32i a6, a1, 4
	s32i a7, a1, 68
	mov.n a11, a3
	s32i a5, a1, 96
	l16ui a5, a1, 152
	movi a3, -128
	s32i.n a3, a1, 0
	addmi a9, a1, 256
	addi a9, a9, -108
	addmi a15, a1, 256
	addi a15, a15, -112
	ee.vldbc.8 q3, a1
	ee.vldbc.8 q5, a15
	ee.vldbc.8 q4, a9
	extui a8, a5, 0, 3
	beqz.n a8, .Lnn_max_pool_s8_LBB3_esp_nn_max_pool_s8_esp32s3
	extui a14, a5, 0, 2
	beqz a14, .Lnn_max_pool_s8_LBB25_esp_nn_max_pool_s8_esp32s3
	retw.n
.Lnn_max_pool_s8_LBB3_esp_nn_max_pool_s8_esp32s3:
	l16ui a15, a1, 136
	l16ui a14, a1, 140
	movi.n a8, 0
	movi.n a10, 0
	s32i a14, a1, 44
	neg a15, a15
	mul16u a9, a6, a5
	neg a14, a14
	s32i a14, a1, 92
	s32i a10, a1, 52
	s32i a9, a1, 60
	s32i.n a8, a1, 36
	s32i a15, a1, 56
	sub a13, a4, a14
	s32i a13, a1, 48
	sub a15, a11, a15
	s32i.n a15, a1, 40
.Lnn_max_pool_s8_Lt_0_21506:
	l32i a8, a1, 4
	beqz a8, .Lnn_max_pool_s8_Lt_0_21762
	movi.n a10, 0
	l32i a9, a1, 44
	l32i.n a15, a1, 40
	l32i a8, a1, 52
	l32i a13, a1, 136
	l32i a14, a1, 56
	s32i a14, a1, 80
	s32i a13, a1, 76
	s32i a8, a1, 88
	s32i a15, a1, 84
	l32i a8, a1, 48
	max a9, a9, a10
	l32i a15, a1, 132
	s32i a9, a1, 8
	movi.n a9, 0
	min a15, a15, a8
	s32i a9, a1, 64
.Lnn_max_pool_s8_Lt_0_22274:
	beqz a5, .Lnn_max_pool_s8_Lt_0_22530
.Lnn_max_pool_s8_LBB10_esp_nn_max_pool_s8_esp32s3:
	l32i a6, a1, 76
	l32i a13, a1, 96
	l32i a8, a1, 84
	l32i a7, a1, 128
	l32i a10, a1, 88
	movi.n a9, 0
	s32i a9, a1, 20
	add.n a14, a10, a5
	min a7, a7, a8
	add.n a10, a10, a13
	add.n a14, a13, a14
	s32i a14, a1, 12
	s32i a10, a1, 16
	movi.n a8, 0
	l32i a10, a1, 80
	max a6, a6, a8
	sub a9, a7, a6
	s32i a9, a1, 28
	add.n a13, a10, a6
	s32i a13, a1, 24
	add.n a10, a10, a7
	s32i a10, a1, 72
.Lnn_max_pool_s8_Lt_0_23042:
	l32i a8, a1, 8
	mv.qr q1, q3
	mov.n a13, a8
	bge a8, a15, .Lnn_max_pool_s8_Lt_0_23298
.Lnn_max_pool_s8_LBB13_esp_nn_max_pool_s8_esp32s3:
	l32i a10, a1, 92
	l32i a14, a1, 72
	add.n a10, a10, a8
	mull a10, a11, a10
	add.n a14, a10, a14
.Lnn_max_pool_s8_Lt_0_23810:
	add.n a14, a14, a11
	addi.n a13, a13, 1
	bge a6, a7, .Lnn_max_pool_s8_Lt_0_24066
.Lnn_max_pool_s8_LBB16_esp_nn_max_pool_s8_esp32s3:
	l32i a3, a1, 24
	l32i a2, a1, 20
	add.n a3, a3, a10
	mull a3, a3, a5
	add.n a2, a2, a3
	l32i a3, a1, 28
	add.n a2, a12, a2
	loopgtz a3, .Lnn_max_pool_s8_LBB93_esp_nn_max_pool_s8_esp32s3
	ee.vld.l.64.ip q0, a2, 0
	add.n a2, a2, a5
	ee.vmax.s8 q1, q1, q0
.Lnn_max_pool_s8_LBB93_esp_nn_max_pool_s8_esp32s3:
.Lnn_max_pool_s8_Lt_0_24066:
	add.n a10, a10, a11
	bne a15, a13, .Lnn_max_pool_s8_Lt_0_23810
.Lnn_max_pool_s8_Lt_0_23298:
	l32i a9, a1, 12
	l32i a13, a1, 20
	l32i a8, a1, 16
	ee.vmin.s8 q2, q1, q4
	ee.vmax.s8 q2, q2, q5
	mov.n a10, a8
	addi.n a13, a13, 8
	s32i a13, a1, 20
	ee.vst.l.64.ip q2, a10, 0
	addi.n a8, a8, 8
	s32i a8, a1, 16
	blt a8, a9, .Lnn_max_pool_s8_Lt_0_23042
.Lnn_max_pool_s8_Lt_0_22530:
	l32i a13, a1, 84
	l32i a14, a1, 80
	l32i a10, a1, 120
	l32i a8, a1, 88
	l32i a9, a1, 76
	add.n a8, a8, a5
	s32i a8, a1, 88
	sub a9, a9, a10
	add.n a14, a14, a10
	sub a13, a13, a10
	s32i a13, a1, 84
	s32i a14, a1, 80
	s32i a9, a1, 76
	l32i a14, a1, 64
	l32i a8, a1, 4
	addi.n a14, a14, 1
	s32i a14, a1, 64
	sub a14, a14, a8
	bnez a14, .Lnn_max_pool_s8_Lt_0_22274
.Lnn_max_pool_s8_Lt_0_21762:
	l32i a8, a1, 44
	l32i a15, a1, 92
	l32i a10, a1, 60
	l32i a14, a1, 124
	l32i a13, a1, 48
	l32i a9, a1, 52
	sub a13, a13, a14
	add.n a9, a9, a10
	add.n a15, a15, a14
	sub a8, a8, a14
	s32i a8, a1, 44
	s32i a15, a1, 92
	s32i a9, a1, 52
	s32i a13, a1, 48
	l32i.n a9, a1, 36
	l32i a10, a1, 68
	addi.n a9, a9, 1
	s32i.n a9, a1, 36
	sub a9, a9, a10
	bnez a9, .Lnn_max_pool_s8_Lt_0_21506
	retw.n
.Lnn_max_pool_s8_LBB25_esp_nn_max_pool_s8_esp32s3:
	l16ui a10, a1, 136
	l16ui a9, a1, 140
	movi.n a13, 0
	movi.n a15, 0
	neg a10, a10
	s32i a9, a1, 44
	mul16u a14, a6, a5
	neg a9, a9
	s32i a9, a1, 92
	s32i a15, a1, 52
	s32i a14, a1, 60
	s32i.n a13, a1, 36
	s32i a10, a1, 56
	sub a8, a4, a9
	s32i a8, a1, 48
	sub a10, a11, a10
	s32i.n a10, a1, 40
.Lnn_max_pool_s8_Lt_0_27138:
	l32i a13, a1, 4
	beqz a13, .Lnn_max_pool_s8_Lt_0_27394
.Lnn_max_pool_s8_LBB29_esp_nn_max_pool_s8_esp32s3:
	movi.n a10, 0
	l32i a9, a1, 44
	l32i.n a15, a1, 40
	l32i a8, a1, 52
	l32i a14, a1, 56
	l32i a13, a1, 136
	s32i a13, a1, 76
	s32i a14, a1, 80
	s32i a8, a1, 88
	s32i a15, a1, 84
	l32i a8, a1, 48
	l32i a15, a1, 132
	movi.n a14, 0
	max a9, a9, a10
	s32i a9, a1, 8
	s32i a14, a1, 64
	min a15, a15, a8
.Lnn_max_pool_s8_Lt_0_27906:
	beqz a5, .Lnn_max_pool_s8_Lt_0_28162
.Lnn_max_pool_s8_LBB32_esp_nn_max_pool_s8_esp32s3:
	l32i a6, a1, 76
	l32i a13, a1, 96
	l32i a8, a1, 84
	l32i a7, a1, 128
	l32i a10, a1, 88
	movi.n a9, 0
	s32i a9, a1, 32
	add.n a14, a10, a5
	min a7, a7, a8
	add.n a10, a10, a13
	add.n a14, a13, a14
	s32i a14, a1, 12
	s32i a10, a1, 16
	movi.n a8, 0
	l32i a10, a1, 80
	max a6, a6, a8
	sub a9, a7, a6
	s32i a9, a1, 28
	add.n a13, a10, a6
	s32i a13, a1, 24
	add.n a10, a10, a7
	s32i a10, a1, 72
.Lnn_max_pool_s8_Lt_0_28674:
	l32i a8, a1, 8
	mv.qr q1, q3
	mov.n a13, a8
	bge a8, a15, .Lnn_max_pool_s8_Lt_0_28930
.Lnn_max_pool_s8_LBB35_esp_nn_max_pool_s8_esp32s3:
	l32i a10, a1, 92
	l32i a14, a1, 72
	add.n a10, a10, a8
	mull a10, a11, a10
	add.n a14, a10, a14
.Lnn_max_pool_s8_Lt_0_29442:
	add.n a14, a14, a11
	addi.n a13, a13, 1
	bge a6, a7, .Lnn_max_pool_s8_Lt_0_29698
.Lnn_max_pool_s8_LBB38_esp_nn_max_pool_s8_esp32s3:
	l32i a3, a1, 24
	l32i a2, a1, 32
	add.n a3, a3, a10
	mull a3, a3, a5
	l32i a4, a1, 28
	add.n a2, a2, a3
	add.n a2, a12, a2
	loopgtz a4, .Lnn_max_pool_s8_LBB108_esp_nn_max_pool_s8_esp32s3
	ee.vldbc.32 q0, a2
	add.n a2, a2, a5
	ee.vmax.s8 q1, q1, q0
.Lnn_max_pool_s8_LBB108_esp_nn_max_pool_s8_esp32s3:
.Lnn_max_pool_s8_Lt_0_29698:
	add.n a10, a10, a11
	bne a15, a13, .Lnn_max_pool_s8_Lt_0_29442
.Lnn_max_pool_s8_Lt_0_28930:
	l32i a9, a1, 12
	l32i a8, a1, 16
	l32i a10, a1, 32
	ee.vmin.s8 q2, q1, q4
	ee.vmax.s8 q2, q2, q5
	addi.n a10, a10, 4
	ee.movi.32.a q2, a13, 0
	s32i a10, a1, 32
	s32i.n a13, a8, 0
	addi.n a8, a8, 4
	s32i a8, a1, 16
	blt a8, a9, .Lnn_max_pool_s8_Lt_0_28674
.Lnn_max_pool_s8_Lt_0_28162:
	l32i a13, a1, 84
	l32i a14, a1, 80
	l32i a10, a1, 120
	l32i a8, a1, 88
	l32i a9, a1, 76
	add.n a8, a8, a5
	s32i a8, a1, 88
	sub a9, a9, a10
	add.n a14, a14, a10
	sub a13, a13, a10
	s32i a13, a1, 84
	s32i a14, a1, 80
	s32i a9, a1, 76
	l32i a14, a1, 64
	l32i a8, a1, 4
	addi.n a14, a14, 1
	s32i a14, a1, 64
	sub a14, a14, a8
	bnez a14, .Lnn_max_pool_s8_Lt_0_27906
.Lnn_max_pool_s8_Lt_0_27394:
	l32i a8, a1, 44
	l32i a15, a1, 92
	l32i a10, a1, 60
	l32i a14, a1, 124
	l32i a13, a1, 48
	l32i a9, a1, 52
	sub a13, a13, a14
	add.n a9, a9, a10
	add.n a15, a15, a14
	sub a8, a8, a14
	s32i a8, a1, 44
	s32i a15, a1, 92
	s32i a9, a1, 52
	s32i a13, a1, 48
	l32i.n a9, a1, 36
	l32i a10, a1, 68
	addi.n a9, a9, 1
	s32i.n a9, a1, 36
	sub a9, a9, a10
	bnez a9, .Lnn_max_pool_s8_Lt_0_27138
	retw.n
	.size tinygo_nn_max_pool_s8, . - tinygo_nn_max_pool_s8

// Inline asm of the esp-nn C code
.section .text.tinygo_nn_sum_blocks16_s8,"ax",@progbits
	.global tinygo_nn_sum_blocks16_s8
	.type tinygo_nn_sum_blocks16_s8, @function
	.p2align 4
tinygo_nn_sum_blocks16_s8:
	entry a1, 32
	movi.n a4, 1
	s8i a4, a1, 0
	mov.n a4, a1
	ee.vldbc.8.ip q1, a4, 0
	ee.zero.accx
	loopgtz a3, .Lnn_sum_blocks16_s8_ens_end
	ee.vld.128.ip q0, a2, 16
	ee.vmulas.s8.accx q0, q1
.Lnn_sum_blocks16_s8_ens_end:
	nop
	nop
	rur.accx_0 a2
	retw.n
	.size tinygo_nn_sum_blocks16_s8, . - tinygo_nn_sum_blocks16_s8

// Inline asm of the esp-nn C code
.section .text.tinygo_nn_max_s8_blocks,"ax",@progbits
	.global tinygo_nn_max_s8_blocks
	.type tinygo_nn_max_s8_blocks, @function
	.p2align 4
tinygo_nn_max_s8_blocks:
	entry a1, 32
	ee.vld.128.ip q0, a2, 16
	movi.n a5, 16
	j .Lnn_max_s8_blocks_max_test
.Lnn_max_s8_blocks_max_loop:
	ee.vld.128.ip q1, a2, 16
	ee.vmax.s8 q0, q0, q1
	addi a5, a5, 16
.Lnn_max_s8_blocks_max_test:
	blt a5, a3, .Lnn_max_s8_blocks_max_loop
	ee.vst.128.ip q0, a4, 16
	retw.n
	.size tinygo_nn_max_s8_blocks, . - tinygo_nn_max_s8_blocks

// Inline asm of the esp-nn C code
.section .text.tinygo_nn_conv_s8_1x1_mac,"ax",@progbits
	.global tinygo_nn_conv_s8_1x1_mac
	.type tinygo_nn_conv_s8_1x1_mac, @function
	.p2align 4
tinygo_nn_conv_s8_1x1_mac:
	entry a1, 48
	ee.zero.qacc
	beqz a4, .Lnn_conv_s8_1x1_mac_mac_done
.Lnn_conv_s8_1x1_mac_mac_group:
	l8ui a6, a3, 0
	s8i a6, a1, 0
	l8ui a6, a3, 1
	s8i a6, a1, 1
	l8ui a6, a3, 2
	s8i a6, a1, 2
	l8ui a6, a3, 3
	s8i a6, a1, 3
	l8ui a6, a3, 4
	s8i a6, a1, 4
	l8ui a6, a3, 5
	s8i a6, a1, 5
	l8ui a6, a3, 6
	s8i a6, a1, 6
	l8ui a6, a3, 7
	s8i a6, a1, 7
	addi a3, a3, 8
	mov.n a7, a1
	mov.n a8, a2
	ee.zero.q q5
	ee.vld.l.64.ip q7, a7, 0
	ee.vld.128.ip q0, a8, 16
	ee.vld.128.ip q1, a8, 16
	ee.vcmp.lt.s8 q6, q7, q5
	ee.vzip.8 q7, q6
	ee.vsmulas.s16.qacc.ld.incp q2, a8, q0, q7, 0
	ee.vsmulas.s16.qacc.ld.incp q3, a8, q1, q7, 1
	ee.vsmulas.s16.qacc.ld.incp q0, a8, q2, q7, 2
	ee.vsmulas.s16.qacc.ld.incp q1, a8, q3, q7, 3
	ee.vsmulas.s16.qacc.ld.incp q2, a8, q0, q7, 4
	ee.vsmulas.s16.qacc.ld.incp q3, a8, q1, q7, 5
	ee.vsmulas.s16.qacc q2, q7, 6
	ee.vsmulas.s16.qacc q3, q7, 7
	addi a2, a2, 128
	addi a4, a4, -1
	bnez a4, .Lnn_conv_s8_1x1_mac_mac_group
.Lnn_conv_s8_1x1_mac_mac_done:
	mov.n a6, a5
	ee.st.qacc_l.l.128.ip a6, 16
	ee.st.qacc_l.h.32.ip a6, -16
	addi a6, a5, 32
	ee.st.qacc_h.l.128.ip a6, 16
	ee.st.qacc_h.h.32.ip a6, -16
	retw.n
	.size tinygo_nn_conv_s8_1x1_mac, . - tinygo_nn_conv_s8_1x1_mac
