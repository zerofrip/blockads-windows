//go:build windows

package dnsconfig

import (
	"fmt"
	"net"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	dnsInterfaceSettingsVersion1 = 1
	dnsSettingIPv6               = 0x0001
	dnsSettingNameServer         = 0x0002

	gAAIncludePrefix   = 0x0010
	gAAIncludeGateways = 0x0080
)

var (
	modIphlpapi                  = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetInterfaceDnsSettings  = modIphlpapi.NewProc("GetInterfaceDnsSettings")
	procSetInterfaceDnsSettings  = modIphlpapi.NewProc("SetInterfaceDnsSettings")
	procFreeInterfaceDnsSettings = modIphlpapi.NewProc("FreeInterfaceDnsSettings")
)

// dnsInterfaceSettings matches DNS_INTERFACE_SETTINGS (64-bit layout).
type dnsInterfaceSettings struct {
	Version             uint32
	_                   uint32
	Flags               uint64
	Domain              *uint16
	NameServer          *uint16
	SearchList          *uint16
	RegistrationEnabled uint32
	RegisterAdapterName uint32
	EnableLLMNR         uint32
	QueryAdapterName    uint32
	ProfileNameServer   *uint16
}

type winConfigurator struct{}

// NewPlatformConfigurator returns the native Windows DNS configurator.
// Requires Windows 10 Build 19041+ for Get/SetInterfaceDnsSettings.
func NewPlatformConfigurator() DnsConfigurator {
	return &winConfigurator{}
}

func (w *winConfigurator) apiAvailable() error {
	if err := procGetInterfaceDnsSettings.Find(); err != nil {
		return fmt.Errorf("%w: GetInterfaceDnsSettings requires Windows 10 2004+ (build 19041): %v", ErrNotSupported, err)
	}
	if err := procSetInterfaceDnsSettings.Find(); err != nil {
		return fmt.Errorf("%w: SetInterfaceDnsSettings unavailable: %v", ErrNotSupported, err)
	}
	return nil
}

func (w *winConfigurator) ListAdapters() ([]NetworkAdapter, error) {
	var size uint32 = 16 * 1024
	var buf []byte
	for attempt := 0; attempt < 5; attempt++ {
		buf = make([]byte, size)
		err := windows.GetAdaptersAddresses(
			windows.AF_UNSPEC,
			gAAIncludePrefix|gAAIncludeGateways,
			0,
			(*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])),
			&size,
		)
		if err == nil {
			return parseAdapters((*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))), nil
		}
		if err != windows.ERROR_BUFFER_OVERFLOW {
			return nil, fmt.Errorf("GetAdaptersAddresses: %w", err)
		}
	}
	return nil, fmt.Errorf("GetAdaptersAddresses: buffer overflow retries exhausted")
}

func parseAdapters(head *windows.IpAdapterAddresses) []NetworkAdapter {
	var out []NetworkAdapter
	for a := head; a != nil; a = a.Next {
		na := NetworkAdapter{
			Key: AdapterKey{
				GUID: formatGUID(a.NetworkGuid),
				LUID: a.Luid,
			},
			FriendlyName: windows.UTF16PtrToString(a.FriendlyName),
			Description:  windows.UTF16PtrToString(a.Description),
			IfType:       a.IfType,
			OperStatus:   a.OperStatus,
		}
		if na.Key.GUID == "{00000000-0000-0000-0000-000000000000}" {
			// Fallback to AdapterName (ASCII GUID string)
			na.Key.GUID = normalizeGUID(windows.BytePtrToString(a.AdapterName))
		}
		for u := a.FirstUnicastAddress; u != nil; u = u.Next {
			ip := sockaddrToIP(u.Address.Sockaddr)
			if ip == nil {
				continue
			}
			if v4 := ip.To4(); v4 != nil {
				na.IPv4Addrs = append(na.IPv4Addrs, v4.String())
			} else {
				na.IPv6Addrs = append(na.IPv6Addrs, ip.String())
			}
		}
		out = append(out, na)
	}
	return out
}

func formatGUID(g windows.GUID) string {
	return fmt.Sprintf("{%08X-%04X-%04X-%02X%02X-%02X%02X%02X%02X%02X%02X}",
		g.Data1, g.Data2, g.Data3,
		g.Data4[0], g.Data4[1], g.Data4[2], g.Data4[3],
		g.Data4[4], g.Data4[5], g.Data4[6], g.Data4[7])
}

func normalizeGUID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if !strings.HasPrefix(s, "{") {
		s = "{" + s
	}
	if !strings.HasSuffix(s, "}") {
		s += "}"
	}
	return strings.ToUpper(s)
}

func sockaddrToIP(sa *syscall.RawSockaddrAny) net.IP {
	if sa == nil {
		return nil
	}
	switch sa.Addr.Family {
	case windows.AF_INET:
		a := (*windows.RawSockaddrInet4)(unsafe.Pointer(sa))
		return net.IPv4(a.Addr[0], a.Addr[1], a.Addr[2], a.Addr[3])
	case windows.AF_INET6:
		a := (*windows.RawSockaddrInet6)(unsafe.Pointer(sa))
		ip := make(net.IP, net.IPv6len)
		copy(ip, a.Addr[:])
		return ip
	default:
		return nil
	}
}

func (w *winConfigurator) readNameServers(guid windows.GUID, ipv6 bool) (DNSServerList, string, error) {
	var settings dnsInterfaceSettings
	settings.Version = dnsInterfaceSettingsVersion1
	if ipv6 {
		settings.Flags = dnsSettingIPv6
	}
	r1, _, _ := procGetInterfaceDnsSettings.Call(
		uintptr(unsafe.Pointer(&guid)),
		uintptr(unsafe.Pointer(&settings)),
	)
	if r1 != 0 {
		return nil, "", fmt.Errorf("GetInterfaceDnsSettings: %w", syscall.Errno(r1))
	}
	defer procFreeInterfaceDnsSettings.Call(uintptr(unsafe.Pointer(&settings)))

	raw := ""
	if settings.NameServer != nil {
		raw = windows.UTF16PtrToString(settings.NameServer)
	}
	return parseServerList(raw), raw, nil
}

func parseServerList(raw string) DNSServerList {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	raw = strings.ReplaceAll(raw, ",", " ")
	fields := strings.Fields(raw)
	out := make(DNSServerList, 0, len(fields))
	out = append(out, fields...)
	return out
}

func joinServers(list DNSServerList) string {
	return strings.Join(list, ",")
}

func (w *winConfigurator) Snapshot(key AdapterKey) (AdapterDNSSnapshot, error) {
	if err := w.apiAvailable(); err != nil {
		return AdapterDNSSnapshot{}, err
	}
	guid, err := windows.GUIDFromString(normalizeGUID(key.GUID))
	if err != nil {
		return AdapterDNSSnapshot{}, fmt.Errorf("GUID: %w", err)
	}
	v4, raw4, err := w.readNameServers(guid, false)
	if err != nil {
		return AdapterDNSSnapshot{}, err
	}
	v6, raw6, err := w.readNameServers(guid, true)
	if err != nil {
		v6, raw6 = nil, ""
	}
	snap := AdapterDNSSnapshot{
		Key:         key,
		IPv4Servers: v4,
		IPv6Servers: v6,
		IPv4DHCP:    raw4 == "",
		IPv6DHCP:    raw6 == "",
	}
	FillChecksum(&snap)
	return snap, nil
}

func (w *winConfigurator) setNameServers(guid windows.GUID, servers DNSServerList, ipv6 bool) error {
	var settings dnsInterfaceSettings
	settings.Version = dnsInterfaceSettingsVersion1
	settings.Flags = dnsSettingNameServer
	if ipv6 {
		settings.Flags |= dnsSettingIPv6
	}
	joined := joinServers(servers)
	if joined != "" {
		p, err := windows.UTF16PtrFromString(joined)
		if err != nil {
			return err
		}
		settings.NameServer = p
	}
	r1, _, _ := procSetInterfaceDnsSettings.Call(
		uintptr(unsafe.Pointer(&guid)),
		uintptr(unsafe.Pointer(&settings)),
	)
	if r1 != 0 {
		return fmt.Errorf("SetInterfaceDnsSettings: %w", syscall.Errno(r1))
	}
	return nil
}

func (w *winConfigurator) ApplyLocalhost(key AdapterKey) error {
	if err := w.apiAvailable(); err != nil {
		return err
	}
	guid, err := windows.GUIDFromString(normalizeGUID(key.GUID))
	if err != nil {
		return err
	}
	if err := w.setNameServers(guid, LocalhostApplied.IPv4Servers, false); err != nil {
		return err
	}
	_ = w.setNameServers(guid, LocalhostApplied.IPv6Servers, true)
	return nil
}

func (w *winConfigurator) Restore(key AdapterKey, original AdapterDNSSnapshot) error {
	if err := w.apiAvailable(); err != nil {
		return err
	}
	guid, err := windows.GUIDFromString(normalizeGUID(key.GUID))
	if err != nil {
		return err
	}
	if err := w.setNameServers(guid, original.IPv4Servers, false); err != nil {
		return err
	}
	_ = w.setNameServers(guid, original.IPv6Servers, true)
	return nil
}

func (w *winConfigurator) Status() (string, error) {
	if err := w.apiAvailable(); err != nil {
		return err.Error(), nil
	}
	ads, err := w.ListAdapters()
	if err != nil {
		return "", err
	}
	elig := FilterEligible(ads)
	return fmt.Sprintf("platform=windows adapters=%d eligible=%d api=Get/SetInterfaceDnsSettings minBuild=19041", len(ads), len(elig)), nil
}
