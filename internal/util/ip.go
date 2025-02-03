package util

import (
	"encoding/binary"
	"fmt"
	"net"
)

// IPToInt converts an IP address string to its integer representation
func IPToInt(ipStr string) (uint32, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return 0, fmt.Errorf("invalid IP address: %s", ipStr)
	}
	ip = ip.To4()
	if ip == nil {
		return 0, fmt.Errorf("not an IPv4 address: %s", ipStr)
	}
	return binary.BigEndian.Uint32(ip), nil
}

// IPVersion returns the IP version (4 or 6)
func IPVersion(ipStr string) (int, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return 0, fmt.Errorf("invalid IP address: %s", ipStr)
	}
	if ip.To4() != nil {
		return 4, nil
	}
	return 6, nil
}
