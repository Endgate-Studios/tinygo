package ieee80211

// Element IDs (IEEE 802.11-2020 table 9-92).
const (
	EIDSSID        = 0
	EIDRates       = 1
	EIDDSParams    = 3
	EIDTIM         = 5
	EIDCountry     = 7
	EIDRSN         = 48
	EIDExtRates    = 50
	EIDHTCaps      = 45
	EIDHTOperation = 61
	EIDVendor      = 221
)

// Elements is a sequence of information elements (IEEE 802.11-2020 9.4.2.1).
type Elements []byte

// Element is one element. Data is a view into the frame.
type Element struct {
	ID   uint8
	Data []byte
}

// Next returns the first element and the rest. ok is false at the end, and err is set on a bad length.
func (e Elements) Next() (el Element, rest Elements, ok bool, err error) {
	if len(e) == 0 {
		return el, nil, false, nil
	}
	if len(e) < 2 || len(e) < 2+int(e[1]) {
		return el, nil, false, ErrIE
	}
	n := int(e[1])
	return Element{ID: e[0], Data: e[2 : 2+n]}, e[2+n:], true, nil
}

// Find returns the first element with id.
func (e Elements) Find(id uint8) (Element, bool) {
	for {
		el, rest, ok, err := e.Next()
		if !ok || err != nil {
			return Element{}, false
		}
		if el.ID == id {
			return el, true
		}
		e = rest
	}
}

// Validate checks that every element length stays inside the buffer.
func (e Elements) Validate() error {
	for {
		_, rest, ok, err := e.Next()
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		e = rest
	}
}

// SSID returns the SSID element, or nil when there is none.
func (e Elements) SSID() []byte {
	el, ok := e.Find(EIDSSID)
	if !ok {
		return nil
	}
	return el.Data
}

// Channel returns the channel from the DS Parameter Set element, or 0 when there is none.
func (e Elements) Channel() uint8 {
	el, ok := e.Find(EIDDSParams)
	if !ok || len(el.Data) < 1 {
		return 0
	}
	return el.Data[0]
}

// AppendElement writes one element into dst at pos and returns the new position.
func AppendElement(dst []byte, pos int, id uint8, data []byte) (int, error) {
	if len(data) > 255 {
		return pos, ErrIE
	}
	if len(dst) < pos+2+len(data) {
		return pos, ErrBuffer
	}
	dst[pos], dst[pos+1] = id, byte(len(data))
	copy(dst[pos+2:], data)
	return pos + 2 + len(data), nil
}

// AppendRates writes the Supported Rates element and, for more than 8 rates, the Extended Supported Rates element.
// Rates are in units of 500 kbit/s, and bit 7 marks a basic rate (IEEE 802.11-2020 9.4.2.3).
func AppendRates(dst []byte, pos int, rates []byte) (int, error) {
	first := rates
	if len(first) > 8 {
		first = rates[:8]
	}
	pos, err := AppendElement(dst, pos, EIDRates, first)
	if err != nil || len(rates) <= 8 {
		return pos, err
	}
	return AppendElement(dst, pos, EIDExtRates, rates[8:])
}
