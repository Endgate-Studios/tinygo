//go:build bcm2710

package machine

import (
	"device/bcm2710"
	"machine/usb"
	"runtime/interrupt"
	"runtime/volatile"
	"sync/atomic"
	"unsafe"
)

// USB device driver for the BCM2710's DWC OTG core, the Pi Zero 2 W's USB data port. It's the
// ESP32-P4's DWC2 driver at full speed in slave (FIFO) mode, with the core at 0x3F980000.

const NumberOfUSBEndpoints = 8

// Physical endpoints in use: TinyGo's classes use 0 to 3, and raw HID 4.
const otgNumEndpoints = 5

// The pid.codes test VID and PID (pid.codes/1209/0001), until the board has its own.
const (
	usb_VID                 = uint16(0x1209)
	usb_PID                 = uint16(0x0001)
	usb_STRING_MANUFACTURER = "TinyGo"
	usb_STRING_PRODUCT      = "Raspberry Pi Zero 2 W"
)

// Full speed on the UTMI+ high speed PHY is DCFG.DSPD 1 (DWC OTG databook, DCFG).
const (
	dcfgDevSpeed = 1
	otgHighSpeed = false
)

// otgEP0PacketSize is endpoint 0's at either speed, which is what DIEPCTL0's reset value of 0 means.
const otgEP0PacketSize = 64

var otgQualifier [10]byte

// deviceQualifier is USB 2.0 section 9.6.2 from the device descriptor, with no other speed configuration.
func deviceQualifier() []byte {
	d := usbDescriptor.Device
	otgQualifier = [10]byte{10, 6, d[2], d[3], d[4], d[5], d[6], d[7], 0, 0}
	return otgQualifier[:]
}

// otgRegisters is the DWC2 core, as in ESP-IDF soc/esp32p4/include/soc/usb_dwc_struct.h.
type otgRegisters struct {
	GOTGCTL   volatile.Register32 // 0x00
	_         uint32
	GAHBCFG   volatile.Register32 // 0x08
	GUSBCFG   volatile.Register32 // 0x0C
	GRSTCTL   volatile.Register32 // 0x10
	GINTSTS   volatile.Register32 // 0x14
	GINTMSK   volatile.Register32 // 0x18
	_         uint32
	GRXSTSP   volatile.Register32 // 0x20
	GRXFSIZ   volatile.Register32 // 0x24
	GNPTXFSIZ volatile.Register32 // 0x28
	_         [9]uint32
	GHWCFG4   volatile.Register32 // 0x50
	_         [44]uint32
	DIEPTXF1  volatile.Register32 // 0x104
	DIEPTXF2  volatile.Register32 // 0x108
	DIEPTXF3  volatile.Register32 // 0x10C
	DIEPTXF4  volatile.Register32 // 0x110
	_         [443]uint32
	DCFG      volatile.Register32 // 0x800
	DCTL      volatile.Register32 // 0x804
	_         [2]uint32
	DIEPMSK   volatile.Register32 // 0x810
	DOEPMSK   volatile.Register32 // 0x814
	DAINT     volatile.Register32 // 0x818
	DAINTMSK  volatile.Register32 // 0x81C
	_         [56]uint32
	DIEPCTL0  volatile.Register32 // 0x900
	_         [127]uint32
	DOEPCTL0  volatile.Register32 // 0xB00
	_         [191]uint32
	PCGCCTL   volatile.Register32 // 0xE00
}

// The USB block in Linux arch/arm/boot/dts/broadcom/bcm283x.dtsi (usb@7e980000).
var otg = (*otgRegisters)(unsafe.Pointer(uintptr(bcm2710.PeripheralBase + 0x980000)))

const (
	gotgctlVBVALOEN  = 1 << 2
	gotgctlVBVALOVAL = 1 << 3
	gotgctlBVALOEN   = 1 << 6
	gotgctlBVALOVAL  = 1 << 7

	gusbcfgPHYIF16 = 1 << 3
	gusbcfgULPI    = 1 << 4

	ghwcfg4PHYWidth = 3 << 14
)

type otgInEndpoint struct {
	CTL    volatile.Register32
	_      uint32
	INT    volatile.Register32
	_      uint32
	TSIZ   volatile.Register32
	_      uint32
	TXFSTS volatile.Register32
	_      uint32
}

type otgOutEndpoint struct {
	CTL  volatile.Register32
	_    uint32
	INT  volatile.Register32
	_    uint32
	TSIZ volatile.Register32
	_    [3]uint32
}

func otgInEP(ep uint32) *otgInEndpoint {
	return (*otgInEndpoint)(unsafe.Add(unsafe.Pointer(&otg.DIEPCTL0), uintptr(ep)*0x20))
}

func otgOutEP(ep uint32) *otgOutEndpoint {
	return (*otgOutEndpoint)(unsafe.Add(unsafe.Pointer(&otg.DOEPCTL0), uintptr(ep)*0x20))
}

// otgFIFO returns the data FIFO of an endpoint. Reads from any of them pop
// the shared RX FIFO.
func otgFIFO(ep uint32) *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Add(unsafe.Pointer(otg), 0x1000+uintptr(ep)*0x1000))
}

// GUSBCFG bits.
const (
	gusbcfgPHYSEL   = 1 << 6
	gusbcfgTRDT_Pos = 10
	gusbcfgTRDT_Msk = 0xF << 10
	gusbcfgFHMOD    = 1 << 29
	gusbcfgFDMOD    = 1 << 30
)

// GRSTCTL bits.
const (
	grstCSRST      = 1 << 0
	grstRXFFLSH    = 1 << 4
	grstTXFFLSH    = 1 << 5
	grstTXFNUM_Pos = 6
	grstAHBIDL     = 1 << 31
)

// GINTSTS and GINTMSK bits.
const (
	gintCMOD      = 1 << 0
	gintRXFLVL    = 1 << 4
	gintGONAKEFF  = 1 << 7
	gintUSBSUSP   = 1 << 11
	gintUSBRST    = 1 << 12
	gintENUMDNE   = 1 << 13
	gintIEPINT    = 1 << 18
	gintOEPINT    = 1 << 19
	gintWKUPINT   = 1 << 31
	gahbcfgGINT   = 1 << 0
	dcfgDAD_Pos   = 4
	dcfgDAD_Msk   = 0x7F << 4
	dctlSDIS      = 1 << 1
	dctlCGINAK    = 1 << 8
	dctlSGONAK    = 1 << 9
	dctlCGONAK    = 1 << 10
	pcgcctlStopCK = 1 << 0
)

// DIEPCTLn and DOEPCTLn bits.
const (
	depctlUSBAEP     = 1 << 15
	depctlETYP_Pos   = 18
	depctlSTALL      = 1 << 21
	depctlTXFNUM_Pos = 22
	depctlCNAK       = 1 << 26
	depctlSNAK       = 1 << 27
	depctlSD0PID     = 1 << 28
	depctlEPDIS      = 1 << 30
	depctlEPENA      = 1 << 31
)

// DIEPINTn and DOEPINTn bits.
const (
	depintXFRC   = 1 << 0
	depintEPDISD = 1 << 1
	depintSTUP   = 1 << 3
	depintINEPNE = 1 << 6
)

// GRXSTSP packet status values.
const (
	rxPktstsOUTData   = 2
	rxPktstsSetupData = 6
)

// FIFO sizes in words. They fit in the 1024 words the P4's core has; the BCM2835's has more.
const (
	rxFIFOWords  = 400
	tx0FIFOWords = 32
	tx1FIFOWords = 32
	tx2FIFOWords = 256
	tx3FIFOWords = 256
	tx4FIFOWords = 32
)

var (
	sendOnEP0DATADONE struct {
		offset int
		data   []byte
	}

	usbSetupBuf [8]byte
	usbRxBufLen [NumberOfUSBEndpoints]uint32
	usbRxReady  [NumberOfUSBEndpoints]bool
)

// usbRegLock serializes FIFO and endpoint register access between the USB
// interrupt and senders on the other core with scheduler=cores.
var usbRegLock atomic.Uint32

func lockUSBRegs() interrupt.State {
	for {
		state := interrupt.Disable()
		if usbRegLock.CompareAndSwap(0, 1) {
			return state
		}
		interrupt.Restore(state)
	}
}

func unlockUSBRegs(state interrupt.State) {
	usbRegLock.Store(0)
	interrupt.Restore(state)
}

// Configure starts the USB-OTG peripheral in device mode, at full speed, or at high speed with the
// usbhs tag. A usbhs build doesn't work on a full speed host, as its descriptors say 512 byte packets.
// The config is here for compatibility with the UART interface.
// The VideoCore powers the USB block through the mailbox first.
func (dev *USBDevice) Configure(config UARTConfig) {
	if dev.initcomplete {
		return
	}

	SetPowerState(PowerUSB, true)

	// Select the PHY, and then reset the core. The reset clears the turnaround time, which the PHY's data
	// width sets. See phy_hs_init in TinyUSB src/portable/synopsys/dwc2/dwc2_common.c.
	wide := otg.GHWCFG4.Get()&ghwcfg4PHYWidth != 0
	gusbcfg := otg.GUSBCFG.Get() &^ (gusbcfgPHYSEL | gusbcfgULPI | gusbcfgPHYIF16)
	if wide {
		gusbcfg |= gusbcfgPHYIF16
	}
	otg.GUSBCFG.Set(gusbcfg)

	waitUSBBits(&otg.GRSTCTL, grstAHBIDL, grstAHBIDL)
	otg.GRSTCTL.SetBits(grstCSRST)
	waitUSBBits(&otg.GRSTCTL, grstCSRST, 0)
	waitUSBBits(&otg.GRSTCTL, grstAHBIDL, grstAHBIDL)

	gusbcfg = otg.GUSBCFG.Get() &^ (gusbcfgFHMOD | gusbcfgTRDT_Msk)
	if wide {
		gusbcfg |= 5 << gusbcfgTRDT_Pos
	} else {
		gusbcfg |= 9 << gusbcfgTRDT_Pos
	}
	otg.GUSBCFG.Set(gusbcfg | gusbcfgFDMOD)
	waitUSBBits(&otg.GINTSTS, gintCMOD, 0)

	// There may be no VBUS sense, so say the session is valid, as the S3's driver does by its GPIO matrix.
	otg.GOTGCTL.SetBits(gotgctlVBVALOEN | gotgctlVBVALOVAL | gotgctlBVALOEN | gotgctlBVALOVAL)

	otg.PCGCCTL.Set(0)
	dev.Detach()

	otg.GRXFSIZ.Set(rxFIFOWords)
	start := uint32(rxFIFOWords)
	otg.GNPTXFSIZ.Set(tx0FIFOWords<<16 | start)
	start += tx0FIFOWords
	otg.DIEPTXF1.Set(tx1FIFOWords<<16 | start)
	start += tx1FIFOWords
	otg.DIEPTXF2.Set(tx2FIFOWords<<16 | start)
	start += tx2FIFOWords
	otg.DIEPTXF3.Set(tx3FIFOWords<<16 | start)
	start += tx3FIFOWords
	otg.DIEPTXF4.Set(tx4FIFOWords<<16 | start)
	flushRxFIFO()
	flushTxFIFO(0x10)

	otg.DCFG.Set(dcfgDevSpeed)
	if otgHighSpeed {
		usbDeviceQualifier = deviceQualifier
	}

	otg.GINTSTS.Set(0xFFFFFFFF)
	otg.GINTMSK.Set(gintUSBRST | gintENUMDNE | gintRXFLVL | gintIEPINT | gintOEPINT |
		gintUSBSUSP | gintWKUPINT)
	otg.DIEPMSK.Set(depintXFRC)
	otg.DOEPMSK.Set(depintXFRC | depintSTUP)
	otg.GAHBCFG.SetBits(gahbcfgGINT)

	interrupt.New(bcm2710.IRQ_USB, handleUSBIRQ).Enable()

	dev.Attach()
	dev.initcomplete = true
}

// waitUSBBits waits until reg&mask equals value, with a limit so that a
// missing PHY clock does not hang the program.
func waitUSBBits(reg *volatile.Register32, mask, value uint32) bool {
	for i := 0; i < 1000000; i++ {
		if reg.Get()&mask == value {
			return true
		}
	}
	return false
}

// Attach connects the device to the USB bus by releasing soft disconnect.
func (dev *USBDevice) Attach() {
	otg.DCTL.ClearBits(dctlSDIS)
}

// Detach disconnects the device from the USB bus with soft disconnect.
func (dev *USBDevice) Detach() {
	otg.DCTL.SetBits(dctlSDIS)
}

func handleUSBIRQ(interrupt.Interrupt) {
	status := otg.GINTSTS.Get() & otg.GINTMSK.Get()

	if status&gintUSBSUSP != 0 {
		otg.GINTSTS.Set(gintUSBSUSP)
	}
	if status&gintWKUPINT != 0 {
		otg.GINTSTS.Set(gintWKUPINT)
		otg.PCGCCTL.ClearBits(pcgcctlStopCK)
	}
	if status&gintUSBRST != 0 {
		otg.GINTSTS.Set(gintUSBRST)
		handleUSBReset()
	}
	if status&gintENUMDNE != 0 {
		otg.GINTSTS.Set(gintENUMDNE)
		// EP0 max packet size field 0 is 64 bytes.
		otgInEP(0).CTL.ClearBits(3)
		otg.DCTL.SetBits(dctlCGINAK)
	}
	if status&gintRXFLVL != 0 {
		handleRxFIFO()
	}
	if status&gintOEPINT != 0 {
		handleOutEndpoints()
	}
	if status&gintIEPINT != 0 {
		handleInEndpoints()
	}
}

func handleUSBReset() {
	for ep := uint32(0); ep < otgNumEndpoints; ep++ {
		otgOutEP(ep).CTL.SetBits(depctlSNAK)
		otgInEP(ep).CTL.SetBits(depctlSNAK)
		usbRxReady[ep] = false
	}
	otg.DCFG.ClearBits(dcfgDAD_Msk)
	flushRxFIFO()
	flushTxFIFO(0x10)

	otg.DAINT.Set(0xFFFFFFFF)
	otg.DAINTMSK.Set(1<<0 | 1<<16)
	sendOnEP0DATADONE.data = nil
	armEP0Out()

	usbConfiguration = 0
	USBDev.InitEndpointComplete = false
}

// handleRxFIFO drains the shared RX FIFO. The data of each OUT packet goes to
// the cache buffer of its endpoint.
func handleRxFIFO() {
	state := lockUSBRegs()
	for otg.GINTSTS.Get()&gintRXFLVL != 0 {
		status := otg.GRXSTSP.Get()
		ep := status & 0xF
		bcnt := (status >> 4) & 0x7FF

		switch (status >> 17) & 0xF {
		case rxPktstsSetupData:
			w0 := otgFIFO(0).Get()
			w1 := otgFIFO(0).Get()
			for i := 0; i < 4; i++ {
				usbSetupBuf[i] = byte(w0 >> (8 * i))
				usbSetupBuf[i+4] = byte(w1 >> (8 * i))
			}
		case rxPktstsOUTData:
			readUSBFIFO(ep, bcnt)
		}
	}
	unlockUSBRegs(state)
}

func readUSBFIFO(ep, bcnt uint32) {
	words := (bcnt + 3) / 4
	if ep >= NumberOfUSBEndpoints || bcnt > uint32(len(udd_ep_out_cache_buffer[0])) {
		for i := uint32(0); i < words; i++ {
			otgFIFO(0).Get()
		}
		return
	}
	buf := udd_ep_out_cache_buffer[ep][:]
	for i := uint32(0); i < words; i++ {
		w := otgFIFO(0).Get()
		for j := uint32(0); j < 4 && i*4+j < bcnt; j++ {
			buf[i*4+j] = byte(w >> (8 * j))
		}
	}
	usbRxBufLen[ep] = bcnt
	usbRxReady[ep] = true
}

func handleOutEndpoints() {
	active := (otg.DAINT.Get() & otg.DAINTMSK.Get()) >> 16
	for ep := uint32(0); ep < otgNumEndpoints; ep++ {
		if active&(1<<ep) == 0 {
			continue
		}
		out := otgOutEP(ep)
		flags := out.INT.Get()
		out.INT.Set(flags)

		if ep == 0 {
			if flags&depintSTUP != 0 {
				handleUSBSetup()
			} else if flags&depintXFRC != 0 {
				usbRxReady[0] = false
				armEP0Out()
			}
			continue
		}
		if flags&depintXFRC != 0 {
			usbRxReady[ep] = false
			buf := udd_ep_out_cache_buffer[ep][:usbRxBufLen[ep]]
			if usbRxHandler[ep] == nil || usbRxHandler[ep](buf) {
				AckUsbOutTransfer(ep)
			}
		}
	}
}

func handleUSBSetup() {
	setup := usb.NewSetup(usbSetupBuf[:])
	sendOnEP0DATADONE.data = nil

	ok := false
	if setup.BmRequestType&usb.REQUEST_TYPE == usb.REQUEST_STANDARD {
		ok = handleStandardSetup(setup)
	} else if setup.WIndex < uint16(len(usbSetupHandler)) && usbSetupHandler[setup.WIndex] != nil {
		ok = usbSetupHandler[setup.WIndex](setup)
	}
	if !ok {
		// The core clears an EP0 stall on the next SETUP.
		otgInEP(0).CTL.SetBits(depctlSTALL)
		otgOutEP(0).CTL.SetBits(depctlSTALL)
	}
	armEP0Out()
}

func handleInEndpoints() {
	active := otg.DAINT.Get() & otg.DAINTMSK.Get() & 0xFFFF
	for ep := uint32(0); ep < otgNumEndpoints; ep++ {
		if active&(1<<ep) == 0 {
			continue
		}
		in := otgInEP(ep)
		flags := in.INT.Get()
		in.INT.Set(flags)
		if flags&depintXFRC == 0 {
			continue
		}

		if ep != 0 {
			if usbTxHandler[ep] != nil {
				usbTxHandler[ep]()
			}
			continue
		}
		if data := sendOnEP0DATADONE.data; data != nil {
			offset := sendOnEP0DATADONE.offset
			count := min(len(data)-offset, otgEP0PacketSize)
			sendOnEP0DATADONE.offset += count
			if sendOnEP0DATADONE.offset >= len(data) {
				sendOnEP0DATADONE.data = nil
			}
			sendViaEPIn(0, data[offset:offset+count])
		} else {
			armEP0Out()
		}
	}
}

func initEndpoint(ep, config uint32) {
	if ep == 0 || ep >= otgNumEndpoints {
		return
	}
	epType := config &^ usb.EndpointIn
	packet := uint32(usb.EndpointPacketSize)
	if epType != usb.ENDPOINT_TYPE_BULK {
		packet = otgEP0PacketSize
	}
	ctl := packet | depctlUSBAEP | depctlSD0PID | epType<<depctlETYP_Pos

	if config&usb.EndpointIn != 0 {
		otgInEP(ep).CTL.Set(ctl | ep<<depctlTXFNUM_Pos | depctlSNAK)
		otg.DAINTMSK.SetBits(1 << ep)
		return
	}
	otgOutEP(ep).CTL.Set(ctl)
	otg.DAINTMSK.SetBits(1 << (ep + 16))
	AckUsbOutTransfer(ep)
}

// SendUSBInPacket sends a packet for USB (interrupt in / bulk in).
func SendUSBInPacket(ep uint32, data []byte) bool {
	sendUSBPacket(ep, data)
	return true
}

//go:noinline
func sendUSBPacket(ep uint32, data []byte) {
	count := len(data)
	if ep == 0 {
		sendOnEP0DATADONE.data = nil
		if count > otgEP0PacketSize {
			count = otgEP0PacketSize
			sendOnEP0DATADONE.offset = count
			sendOnEP0DATADONE.data = data
		}
	}
	sendViaEPIn(ep, data[:count])
}

// sendViaEPIn starts a one packet IN transfer and pushes the data into the
// TX FIFO of the endpoint.
func sendViaEPIn(ep uint32, data []byte) {
	state := lockUSBRegs()
	in := otgInEP(ep)
	in.TSIZ.Set(1<<19 | uint32(len(data)))
	in.CTL.SetBits(depctlEPENA | depctlCNAK)

	fifo := otgFIFO(ep)
	for i := 0; i < len(data); i += 4 {
		var w uint32
		for j := 0; j < 4 && i+j < len(data); j++ {
			w |= uint32(data[i+j]) << (8 * j)
		}
		fifo.Set(w)
	}
	unlockUSBRegs(state)
}

// SendZlp sends a zero length packet on EP0 IN.
func SendZlp() {
	sendUSBPacket(0, nil)
}

// AckUsbOutTransfer re-arms an OUT endpoint for the next packet.
func AckUsbOutTransfer(ep uint32) {
	ep &= 0x7F
	state := lockUSBRegs()
	usbRxBufLen[ep] = 0
	out := otgOutEP(ep)
	out.TSIZ.Set(1<<19 | usb.EndpointPacketSize)
	out.CTL.SetBits(depctlEPENA | depctlCNAK)
	unlockUSBRegs(state)
}

// armEP0Out enables EP0 OUT for a data or status packet and up to 3
// back to back SETUP packets.
func armEP0Out() {
	state := lockUSBRegs()
	otgOutEP(0).TSIZ.Set(3<<29 | 1<<19 | otgEP0PacketSize)
	otgOutEP(0).CTL.SetBits(depctlEPENA | depctlCNAK)
	unlockUSBRegs(state)
}

// handleUSBSetAddress stores the new address before the status ZLP goes out.
// The core still answers the status stage on address 0.
func handleUSBSetAddress(setup usb.Setup) bool {
	dcfg := otg.DCFG.Get() &^ dcfgDAD_Msk
	otg.DCFG.Set(dcfg | uint32(setup.WValueL&0x7F)<<dcfgDAD_Pos)
	SendZlp()
	return true
}

// ReceiveUSBControlPacket receives the data stage of a control OUT request.
// It runs inside the SETUP handler, so it drains the RX FIFO itself.
func ReceiveUSBControlPacket() ([cdcLineInfoSize]byte, error) {
	var b [cdcLineInfoSize]byte
	usbRxReady[0] = false
	armEP0Out()

	for i := 0; i < 2000000; i++ {
		if otg.GINTSTS.Get()&gintRXFLVL != 0 {
			handleRxFIFO()
		}
		if usbRxReady[0] {
			usbRxReady[0] = false
			copy(b[:], udd_ep_out_cache_buffer[0][:usbRxBufLen[0]])
			return b, nil
		}
	}
	return b, ErrUSBReadTimeout
}

// SetStallEPIn stalls an IN endpoint.
func (dev *USBDevice) SetStallEPIn(ep uint32) {
	ep &= 0x7F
	state := lockUSBRegs()
	in := otgInEP(ep)
	if ep == 0 || in.CTL.Get()&depctlEPENA == 0 {
		in.CTL.SetBits(depctlSNAK | depctlSTALL)
	} else {
		in.CTL.SetBits(depctlSNAK)
		waitUSBBits(&in.INT, depintINEPNE, depintINEPNE)
		in.CTL.SetBits(depctlEPDIS | depctlSTALL)
		waitUSBBits(&in.INT, depintEPDISD, depintEPDISD)
		in.INT.Set(depintEPDISD | depintINEPNE)
	}
	flushTxFIFO(ep)
	unlockUSBRegs(state)
}

// SetStallEPOut stalls an OUT endpoint.
func (dev *USBDevice) SetStallEPOut(ep uint32) {
	ep &= 0x7F
	state := lockUSBRegs()
	out := otgOutEP(ep)
	if ep == 0 || out.CTL.Get()&depctlEPENA == 0 {
		out.CTL.SetBits(depctlSTALL)
		unlockUSBRegs(state)
		return
	}
	// Disabling an enabled OUT endpoint needs a global OUT NAK, as in
	// TinyUSB src/portable/synopsys/dwc2/dcd_dwc2.c edpt_disable.
	otg.DCTL.SetBits(dctlSGONAK)
	waitUSBBits(&otg.GINTSTS, gintGONAKEFF, gintGONAKEFF)
	out.CTL.SetBits(depctlEPDIS | depctlSTALL)
	waitUSBBits(&out.INT, depintEPDISD, depintEPDISD)
	out.INT.Set(depintEPDISD)
	otg.DCTL.SetBits(dctlCGONAK)
	unlockUSBRegs(state)
}

// ClearStallEPIn clears the stall on an IN endpoint and resets it to DATA0.
func (dev *USBDevice) ClearStallEPIn(ep uint32) {
	ep &= 0x7F
	state := lockUSBRegs()
	ctl := &otgInEP(ep).CTL
	ctl.ClearBits(depctlSTALL)
	ctl.SetBits(depctlSD0PID)
	unlockUSBRegs(state)
}

// ClearStallEPOut clears the stall on an OUT endpoint, resets it to DATA0
// and re-arms it.
func (dev *USBDevice) ClearStallEPOut(ep uint32) {
	ep &= 0x7F
	state := lockUSBRegs()
	ctl := &otgOutEP(ep).CTL
	ctl.ClearBits(depctlSTALL)
	ctl.SetBits(depctlSD0PID)
	unlockUSBRegs(state)
	AckUsbOutTransfer(ep)
}

func flushTxFIFO(txfnum uint32) {
	otg.GRSTCTL.Set(grstTXFFLSH | txfnum<<grstTXFNUM_Pos)
	waitUSBBits(&otg.GRSTCTL, grstTXFFLSH, 0)
}

func flushRxFIFO() {
	otg.GRSTCTL.Set(grstRXFFLSH)
	waitUSBBits(&otg.GRSTCTL, grstRXFFLSH, 0)
}

// FlushSerial is a no-op, the CDC class sends from its own ring buffer.
func FlushSerial() {}

// usbDelayMs spins without timers or interrupts, at about 100 ns for a register read.
func usbDelayMs(ms int) {
	for i := 0; i < ms*10000; i++ {
		_ = otg.GINTSTS.Get()
	}
}
