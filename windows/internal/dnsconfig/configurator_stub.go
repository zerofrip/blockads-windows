//go:build !windows

package dnsconfig

// NewPlatformConfigurator returns a stub on non-Windows hosts.
func NewPlatformConfigurator() DnsConfigurator {
	return &stubConfigurator{}
}

type stubConfigurator struct{}

func (s *stubConfigurator) ListAdapters() ([]NetworkAdapter, error) {
	return nil, ErrNotSupported
}
func (s *stubConfigurator) Snapshot(AdapterKey) (AdapterDNSSnapshot, error) {
	return AdapterDNSSnapshot{}, ErrNotSupported
}
func (s *stubConfigurator) ApplyLocalhost(AdapterKey) error { return ErrNotSupported }
func (s *stubConfigurator) Restore(AdapterKey, AdapterDNSSnapshot) error {
	return ErrNotSupported
}
func (s *stubConfigurator) Status() (string, error) {
	return "platform=non-windows unsupported", nil
}
