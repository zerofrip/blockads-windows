//go:build windows

package ipc

import (
	"fmt"
	"net"

	"github.com/Microsoft/go-winio"
	"github.com/nqmgaming/blockads-windows/windows/internal/protocol"
)

// Pipe SDDL: LocalSystem + Admins full; Interactive + Authenticated Users RW; no Everyone.
const pipeSDDL = "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;IU)(A;;GRGW;;;AU)"

func ListenPipe(name string) (net.Listener, error) {
	if name == "" {
		name = protocol.PipeName
	}
	cfg := &winio.PipeConfig{
		SecurityDescriptor: pipeSDDL,
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

func DialPipe(name string) (net.Conn, error) {
	if name == "" {
		name = protocol.PipeName
	}
	return winio.DialPipe(name, nil)
}
