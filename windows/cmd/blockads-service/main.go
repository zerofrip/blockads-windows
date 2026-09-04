package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nqmgaming/blockads-windows/windows/internal/controller"
	"github.com/nqmgaming/blockads-windows/windows/internal/service"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "install-hints" {
		fmt.Print(service.InstallHints())
		return
	}
	paths := defaultPaths()
	if err := service.Run(paths); err != nil {
		fmt.Fprintf(os.Stderr, "service error: %v\n", err)
		os.Exit(1)
	}
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
		DataDir:   base,
		StateFile: filepath.Join(base, "state", "recovery.json"),
		FilterDir: filepath.Join(base, "filters"),
	}
}
