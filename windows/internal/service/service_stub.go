//go:build !windows

package service

import (
	"context"
	"fmt"

	"github.com/nqmgaming/blockads-windows/windows/internal/controller"
	"github.com/nqmgaming/blockads-windows/windows/internal/dnsconfig"
	"github.com/nqmgaming/blockads-windows/windows/internal/ipc"
)

const ServiceName = "BlockAdsService"

// Run starts a foreground IPC server for non-Windows development.
func Run(paths controller.Paths) error {
	dnsCfg := dnsconfig.NewPlatformConfigurator()
	ctrl := controller.New(paths, dnsCfg)
	_, _ = ctrl.RecoverIfNeeded()
	ln, err := ipc.ListenPipe("")
	if err != nil {
		return err
	}
	defer ln.Close()
	fmt.Println("BlockAds IPC listening (dev):", ln.Addr())
	srv := &ipc.Server{Handler: &ipc.Handler{Ctrl: ctrl}}
	return srv.Serve(context.Background(), ln)
}

func InstallHints() string {
	return "Windows Service installation is only available on Windows."
}
