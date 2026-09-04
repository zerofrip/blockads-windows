package dnsconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// ComputeChecksum fingerprints DNS server lists for recovery records.
func ComputeChecksum(ipv4, ipv6 DNSServerList, ipv4DHCP, ipv6DHCP bool) string {
	h := sha256.New()
	fmt.Fprintf(h, "v4=%s;v6=%s;d4=%v;d6=%v",
		strings.Join(ipv4, ","), strings.Join(ipv6, ","), ipv4DHCP, ipv6DHCP)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// FillChecksum sets Checksum on a snapshot.
func FillChecksum(s *AdapterDNSSnapshot) {
	s.Checksum = ComputeChecksum(s.IPv4Servers, s.IPv6Servers, s.IPv4DHCP, s.IPv6DHCP)
}
