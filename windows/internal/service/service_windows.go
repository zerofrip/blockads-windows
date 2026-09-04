//go:build windows

package service

import (
	"context"
	"path/filepath"
	"time"

	"github.com/nqmgaming/blockads-windows/windows/internal/controller"
	"github.com/nqmgaming/blockads-windows/windows/internal/dnsconfig"
	"github.com/nqmgaming/blockads-windows/windows/internal/ipc"
	"github.com/nqmgaming/blockads-windows/windows/internal/netwatch"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
)

type blockAdsService struct {
	paths controller.Paths
}

func (s *blockAdsService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const accepts = svc.AcceptStop | svc.AcceptShutdown

	changes <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dnsCfg := dnsconfig.NewPlatformConfigurator()
	ctrl, err := controller.New(s.paths, dnsCfg)
	if err != nil {
		changes <- svc.Status{State: svc.StopPending}
		return true, 1
	}
	defer ctrl.Close()

	_, _ = ctrl.Recover(ctx)

	ln, err := ipc.ListenPipe("")
	if err != nil {
		changes <- svc.Status{State: svc.StopPending}
		return true, 1
	}
	defer ln.Close()

	srv := &ipc.Server{Handler: &ipc.Handler{Ctrl: ctrl}}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx, ln) }()
	go func() { _ = netwatch.Start(ctx, ctrl) }()

	changes <- svc.Status{State: svc.Running, Accepts: accepts}

	for {
		select {
		case err := <-errCh:
			cancel()
			_ = ctrl.Disable(context.Background())
			if err != nil {
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
				_ = ctrl.Disable(context.Background())
				time.Sleep(100 * time.Millisecond)
				changes <- svc.Status{State: svc.Stopped}
				return false, 0
			}
		}
	}
}

// Run hosts the service under SCM or as a console debug process.
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

func InstallHints() string {
	exe, _ := filepath.Abs("BlockAdsService.exe")
	return "Use: blockads-service install|uninstall|start|stop|status\nDefault exe: " + exe
}
