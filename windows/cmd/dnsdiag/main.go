//go:build windows

package main

import (
	"fmt"
	"os"

	"github.com/nqmgaming/blockads-windows/windows/internal/dnsconfig"
)

func main() {
	cfg := dnsconfig.NewPlatformConfigurator()
	st, err := cfg.Status()
	fmt.Println("status:", st, err)
	ads, err := cfg.ListAdapters()
	if err != nil {
		fmt.Println("list err:", err)
		os.Exit(1)
	}
	for _, a := range ads {
		elig := dnsconfig.IsEligible(a)
		fmt.Printf("ADAPTER name=%q desc=%q ifType=%d oper=%d guid=%s luid=%d ipv4=%v eligible=%v\n",
			a.FriendlyName, a.Description, a.IfType, a.OperStatus, a.Key.GUID, a.Key.LUID, a.IPv4Addrs, elig)
		snap, err := cfg.Snapshot(a.Key)
		if err != nil {
			fmt.Printf("  SNAPSHOT_ERR: %v\n", err)
			continue
		}
		fmt.Printf("  snapshot v4=%v dhcp4=%v v6=%v dhcp6=%v\n", snap.IPv4Servers, snap.IPv4DHCP, snap.IPv6Servers, snap.IPv6DHCP)
	}
}
