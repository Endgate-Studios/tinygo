package ieee80211

import "encoding/binary"

// Suite is a cipher or AKM suite selector: an OUI and a type (IEEE 802.11-2020 9.4.2.24.2).
type Suite uint32

// Suites of the IEEE 802.11 OUI 00-0F-AC (IEEE 802.11-2020 tables 9-149 and 9-151).
const (
	CipherTKIP   Suite = 0x000fac02
	CipherCCMP   Suite = 0x000fac04
	CipherGCMP   Suite = 0x000fac08
	AKMPSK       Suite = 0x000fac02
	AKMPSKSHA256 Suite = 0x000fac06
	AKMSAE       Suite = 0x000fac08
)

const maxSuites = 4

// RSN is a parsed RSN element with at most 4 pairwise and 4 AKM suites.
type RSN struct {
	Version      uint16
	Group        Suite
	Pairwise     [maxSuites]Suite
	PairwiseLen  int
	AKM          [maxSuites]Suite
	AKMLen       int
	Capabilities uint16
}

// HasPairwise and HasAKM report whether the element offers s.
func (r *RSN) HasPairwise(s Suite) bool { return has(r.Pairwise[:r.PairwiseLen], s) }
func (r *RSN) HasAKM(s Suite) bool      { return has(r.AKM[:r.AKMLen], s) }

func has(list []Suite, s Suite) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// ParseRSN decodes the data of an RSN element. Missing optional fields keep their defaults:
// CCMP group and pairwise, and PSK (IEEE 802.11-2020 9.4.2.24.1).
func ParseRSN(data []byte) (RSN, error) {
	r := RSN{Group: CipherCCMP}
	if len(data) < 2 {
		return r, ErrRSN
	}
	r.Version = binary.LittleEndian.Uint16(data)
	data = data[2:]
	if len(data) < 4 {
		r.Pairwise[0], r.PairwiseLen = CipherCCMP, 1
		r.AKM[0], r.AKMLen = AKMPSK, 1
		return r, nil
	}
	r.Group = Suite(binary.BigEndian.Uint32(data))
	data = data[4:]
	var err error
	if data, r.PairwiseLen, err = parseSuites(data, r.Pairwise[:], CipherCCMP); err != nil {
		return r, err
	}
	if data, r.AKMLen, err = parseSuites(data, r.AKM[:], AKMPSK); err != nil {
		return r, err
	}
	if len(data) >= 2 {
		r.Capabilities = binary.LittleEndian.Uint16(data)
	}
	return r, nil
}

func parseSuites(data []byte, out []Suite, def Suite) ([]byte, int, error) {
	if len(data) < 2 {
		out[0] = def
		return data, 1, nil
	}
	n := int(binary.LittleEndian.Uint16(data))
	data = data[2:]
	if n > len(out) {
		return data, 0, ErrTooMany
	}
	if len(data) < 4*n {
		return data, 0, ErrRSN
	}
	for i := 0; i < n; i++ {
		out[i] = Suite(binary.BigEndian.Uint32(data[4*i:]))
	}
	return data[4*n:], n, nil
}

// AppendRSN writes an RSN element with one group cipher, one pairwise cipher and one AKM.
// This is the element that a station sends in its association request.
func AppendRSN(dst []byte, pos int, group, pairwise, akm Suite, capabilities uint16) (int, error) {
	var d [20]byte
	binary.LittleEndian.PutUint16(d[0:], 1)
	binary.BigEndian.PutUint32(d[2:], uint32(group))
	binary.LittleEndian.PutUint16(d[6:], 1)
	binary.BigEndian.PutUint32(d[8:], uint32(pairwise))
	binary.LittleEndian.PutUint16(d[12:], 1)
	binary.BigEndian.PutUint32(d[14:], uint32(akm))
	binary.LittleEndian.PutUint16(d[18:], capabilities)
	return AppendElement(dst, pos, EIDRSN, d[:])
}
