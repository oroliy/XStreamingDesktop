package streaming

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"
)

type ICECandidate struct {
	Candidate     string `json:"candidate"`
	MessageType   string `json:"messageType"`
	SDPMLineIndex string `json:"sdpMLineIndex"`
	SDPMid        string `json:"sdpMid"`
}

// UnmarshalJSON accepts both browser numeric and service string m-line indexes.
func (c *ICECandidate) UnmarshalJSON(data []byte) error {
	var wire struct {
		Candidate   string          `json:"candidate"`
		MessageType string          `json:"messageType"`
		Index       json.RawMessage `json:"sdpMLineIndex"`
		Mid         string          `json:"sdpMid"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	index := "0"
	if len(wire.Index) > 0 && !bytes.Equal(wire.Index, []byte("null")) {
		if wire.Index[0] == '"' {
			if err := json.Unmarshal(wire.Index, &index); err != nil {
				return err
			}
		} else {
			index = string(wire.Index)
		}
		if _, err := strconv.ParseUint(index, 10, 16); err != nil {
			return fmt.Errorf("invalid ICE m-line index: %w", err)
		}
	}
	*c = ICECandidate{Candidate: wire.Candidate, MessageType: wire.MessageType, SDPMLineIndex: index, SDPMid: wire.Mid}
	return nil
}

// MarshalJSON sends the m-line index as a number, as browser ICE candidates do.
func (c ICECandidate) MarshalJSON() ([]byte, error) {
	index, err := strconv.ParseUint(c.SDPMLineIndex, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid ICE m-line index: %w", err)
	}
	return json.Marshal(struct {
		Candidate   string `json:"candidate"`
		MessageType string `json:"messageType,omitempty"`
		Index       uint16 `json:"sdpMLineIndex"`
		Mid         string `json:"sdpMid"`
	}{c.Candidate, c.MessageType, uint16(index), c.SDPMid})
}

type parsedCandidate struct {
	ip   netip.Addr
	port string
	rest string
}

// NormalizeICE applies the original client's Teredo, IPv6 preference and priority rules.
func NormalizeICE(input []ICECandidate, preferIPv6 bool) ([]ICECandidate, error) {
	var parsed []parsedCandidate
	for _, item := range input {
		if item.Candidate == "a=end-of-candidates" || item.Candidate == "end-of-candidates" {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(item.Candidate, "a="))
		if len(fields) < 8 || !strings.HasPrefix(fields[0], "candidate:") || fields[6] != "typ" {
			return nil, fmt.Errorf("invalid ICE candidate")
		}
		ip, err := netip.ParseAddr(fields[4])
		if err != nil {
			return nil, fmt.Errorf("invalid ICE address: %w", err)
		}
		port, err := strconv.ParseUint(fields[5], 10, 16)
		if err != nil || port == 0 {
			return nil, fmt.Errorf("invalid ICE port")
		}
		rest := strings.Join(fields[6:], " ")
		if ip.Is6() {
			a := ip.As16()
			if a[0] == 0x20 && a[1] == 0x01 && a[2] == 0 && a[3] == 0 {
				v4 := netip.AddrFrom4([4]byte{^a[12], ^a[13], ^a[14], ^a[15]})
				teredoPort := uint16(^a[10])<<8 | uint16(^a[11])
				parsed = append(parsed, parsedCandidate{v4, "9002", "typ host"}, parsedCandidate{v4, strconv.Itoa(int(teredoPort)), "typ host"})
			}
		}
		parsed = append(parsed, parsedCandidate{ip, fields[5], rest})
	}
	if preferIPv6 {
		sort.SliceStable(parsed, func(i, j int) bool { return parsed[i].ip.Is6() && !parsed[j].ip.Is6() })
	}
	result := make([]ICECandidate, 0, len(parsed)+1)
	for i, item := range parsed {
		priority := 1
		if i == 0 {
			priority = 2130706431
		}
		result = append(result, ICECandidate{Candidate: fmt.Sprintf("a=candidate:%d 1 UDP %d %s %s %s", i+1, priority, item.ip, item.port, item.rest), MessageType: "iceCandidate", SDPMLineIndex: "0", SDPMid: "0"})
	}
	return append(result, ICECandidate{Candidate: "a=end-of-candidates", MessageType: "iceCandidate", SDPMLineIndex: "0", SDPMid: "0"}), nil
}
