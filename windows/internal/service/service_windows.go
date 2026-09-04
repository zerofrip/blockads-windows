//go:build windows

package service

import (
	"context"
	"fmt"
	"time"

	"github.com/nqmgaming/blockads-windows/windows/internal/controller"
	"github.com/nqmgaming/blockads-windows/windows/internal/dnsconfig"
	"github.com/nqmgaming/blockads-windows/windows/internal/ipc"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
)

const ServiceName = "BlockAdsService"
const ServiceDisplayName = "BlockAds DNS Filter"

type blockAdsService struct {
	paths controller.Paths
}

func (s *blockAdsService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dnsCfg := dnsconfig.NewPlatformConfigurator()
	ctrl := controller.New(s.paths, dnsCfg)
	_, _ = ctrl.RecoverIfNeeded()

	ln, err := ipc.ListenPipe("")
	if err != nil {
		changes <- svc.Status{State: svc.StopPending}
		return true, 1
	}
	defer ln.Close()

	srv := &ipc.Server{Handler: &ipc.Handler{Ctrl: ctrl}}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx, ln) }()

	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for {
		select {
		case err := <-errCh:
			if err != nil {
				_ = ctrl.Disable()
				return true, 1
			}
			return false, 0
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				_ = ctrl.Disable()
				time.Sleep(200 * time.Millisecond)
				changes <- svc.Status{State: svc.Stopped}
				return false, 0
			default:
				// ignore
			}
		}
	}
}

// Run runs as a Windows Service when launched by SCM; otherwise as console debug.
func Run(paths controller.Paths) error {
	isSvc, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	s := &blockAdsService{paths: paths}
	if isSvc {
		return svc.Run(ServiceName, s)
	}
	return debug.Run(ServiceName, s)
}

// InstallHints returns sc.exe-style guidance without shelling out.
func InstallHints() string {
	return fmt.Sprintf(`Install (elevated PowerShell / SCM):
  sc.exe create %s binPath= "C:\\Path\\To\\BlockAdsService.exe" start= auto
  sc.exe description %s "BlockAds system DNS filtering service"
  sc.exe start %s
`, ServiceName, ServiceName, ServiceName)
}
