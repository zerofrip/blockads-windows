//go:build !windows

package ipc

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

func defaultSock() string {
	return filepath.Join(os.TempDir(), "blockads.sock")
}

func ListenPipe(name string) (net.Listener, error) {
	if name == "" {
		name = defaultSock()
	}
	_ = os.Remove(name)
	ln, err := net.Listen("unix", name)
	if err != nil {
		return nil, fmt.Errorf("listen unix %s: %w", name, err)
	}
	return ln, nil
}

func DialPipe(name string) (net.Conn, error) {
	if name == "" {
		name = defaultSock()
	}
	d := net.Dialer{Timeout: 3 * time.Second}
	return d.Dial("unix", name)
}
