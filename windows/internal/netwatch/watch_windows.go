//go:build windows

package netwatch

import (
	"context"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modIphlpapi              = windows.NewLazySystemDLL("iphlpapi.dll")
	procNotifyIpInterfaceChange = modIphlpapi.NewProc("NotifyIpInterfaceChange")
	procCancelMibChangeNotify2  = modIphlpapi.NewProc("CancelMibChangeNotify2")
)

// startPlatform registers NotifyIpInterfaceChange with debounce.
func startPlatform(ctx context.Context, h Handler) error {
	if err := procNotifyIpInterfaceChange.Find(); err != nil {
		// API unavailable — fall back to no-op watcher
		<-ctx.Done()
		return nil
	}

	var (
		mu       sync.Mutex
		timer    *time.Timer
		handle   uintptr
	)
	cb := windows.NewCallback(func(callerContext uintptr, row uintptr, notificationType uint32) uintptr {
		mu.Lock()
		defer mu.Unlock()
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(750*time.Millisecond, func() {
			_ = h.ReevaluateAdapters(context.Background())
		})
		return 0
	})

	r1, _, err := procNotifyIpInterfaceChange.Call(
		uintptr(windows.AF_UNSPEC),
		cb,
		0,
		0, // initialNotification = FALSE
		uintptr(unsafe.Pointer(&handle)),
	)
	if r1 != 0 {
		// Failed to register; soft-fail
		_ = err
		<-ctx.Done()
		return nil
	}

	<-ctx.Done()
	if handle != 0 {
		_, _, _ = procCancelMibChangeNotify2.Call(handle)
	}
	return nil
}
