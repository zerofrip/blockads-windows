//go:build windows

package netwatch

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modIphlpapi                 = windows.NewLazySystemDLL("iphlpapi.dll")
	procNotifyIpInterfaceChange = modIphlpapi.NewProc("NotifyIpInterfaceChange")
	procCancelMibChangeNotify2  = modIphlpapi.NewProc("CancelMibChangeNotify2")

	callbackCount atomic.Uint64
	reevalCount   atomic.Uint64
)

// Stats returns NotifyIpInterfaceChange callback and debounced reevaluation counts.
func Stats() (callbacks, reevaluations uint64) {
	return callbackCount.Load(), reevalCount.Load()
}

// ResetStats clears counters (tests/validation only).
func ResetStats() {
	callbackCount.Store(0)
	reevalCount.Store(0)
}

// startPlatform registers NotifyIpInterfaceChange with debounce.
func startPlatform(ctx context.Context, h Handler) error {
	if err := procNotifyIpInterfaceChange.Find(); err != nil {
		<-ctx.Done()
		return nil
	}

	var (
		mu     sync.Mutex
		timer  *time.Timer
		handle uintptr
	)
	cb := windows.NewCallback(func(callerContext uintptr, row uintptr, notificationType uint32) uintptr {
		callbackCount.Add(1)
		mu.Lock()
		defer mu.Unlock()
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(750*time.Millisecond, func() {
			reevalCount.Add(1)
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
