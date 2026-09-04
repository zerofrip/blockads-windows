package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nqmgaming/blockads-windows/windows/internal/controller"
	"github.com/nqmgaming/blockads-windows/windows/internal/service"
)

func main() {
	if len(os.Args) < 2 {
		runService()
		return
	}
	switch os.Args[1] {
	case "install":
		exe := ""
		if len(os.Args) > 2 {
			exe = os.Args[2]
		}
		if err := service.Install(exe); err != nil {
			fatal(err)
		}
		fmt.Println("installed", service.ServiceName)
		fmt.Println("Note: service installed/running is distinct from filtering enabled.")
	case "uninstall":
		if err := service.Uninstall(); err != nil {
			fatal(err)
		}
		fmt.Println("uninstalled", service.ServiceName)
	case "start":
		if err := service.Start(); err != nil {
			fatal(err)
		}
		fmt.Println("started", service.ServiceName)
	case "stop":
		if err := service.Stop(); err != nil {
			fatal(err)
		}
		fmt.Println("stopped", service.ServiceName)
	case "status":
		st, err := service.QueryState()
		if err != nil {
			fatal(err)
		}
		fmt.Println(st)
	case "install-hints":
		fmt.Print(service.InstallHints())
	case "run":
		runService()
	default:
		fmt.Fprintf(os.Stderr, "usage: blockads-service [install|uninstall|start|stop|status|run]\n")
		os.Exit(2)
	}
}

func runService() {
	if err := service.Run(defaultPaths()); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

func defaultPaths() controller.Paths {
	base := os.Getenv("BLOCKADS_DATA_DIR")
	if base == "" {
		pd := os.Getenv("PROGRAMDATA")
		if pd == "" {
			pd = filepath.Join(os.TempDir(), "BlockAds")
		}
		base = filepath.Join(pd, "BlockAds")
	}
	return controller.Paths{
		DataDir:    base,
		StateFile:  filepath.Join(base, "state", "recovery.json"),
		ConfigFile: filepath.Join(base, "config.json"),
		FilterDir:  filepath.Join(base, "filters"),
	}
}
