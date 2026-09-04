//go:build !windows

package service

import (
	"context"
	"fmt"

	"github.com/nqmgaming/blockads-windows/windows/internal/controller"
	"github.com/nqmgaming/blockads-windows/windows/internal/dnsconfig"
	"github.com/nqmgaming/blockads-windows/windows/internal/ipc"
	"github.com/nqmgaming/blockads-windows/windows/internal/netwatch"
)

func Run(paths controller.Paths) error {
	dnsCfg := dnsconfig.NewPlatformConfigurator()
	ctrl, err := controller.New(paths, dnsCfg)
	if err != nil {
		return err
	}
	defer ctrl.Close()
	_ = ctrl.Startup(context.Background())
	ln, err := ipc.ListenPipe("")
	if err != nil {
		return err
	}
	defer ln.Close()
	fmt.Println("BlockAds IPC listening (dev):", ln.Addr())
	srv := &ipc.Server{Handler: &ipc.Handler{Ctrl: ctrl}}
	ctx := context.Background()
	go func() { _ = netwatch.Start(ctx, ctrl) }()
	return srv.Serve(ctx, ln)
}

func InstallHints() string {
	return "Windows Service installation is only available on Windows."
}

