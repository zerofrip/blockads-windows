//go:build windows

package ipc

import (
	"fmt"
	"net"

	"github.com/Microsoft/go-winio"
)

const DefaultPipeName = `\\.\pipe\BlockAdsService`

// ListenPipe creates a local Named Pipe listener.
// Security: default winio SD allowing Administrators and the creating user.
func ListenPipe(name string) (net.Listener, error) {
	if name == "" {
		name = DefaultPipeName
	}
	cfg := &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;BA)(A;;GA;;;SY)(A;;GRGW;;;IU)",
		MessageMode:        false,
		InputBufferSize:    65536,
		OutputBufferSize:   65536,
	}
	ln, err := winio.ListenPipe(name, cfg)
	if err != nil {
		return nil, fmt.Errorf("listen pipe %s: %w", name, err)
	}
	return ln, nil
}
