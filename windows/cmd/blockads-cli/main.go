package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nqmgaming/blockads-windows/windows/internal/client"
	"github.com/nqmgaming/blockads-windows/windows/internal/controller"
	"github.com/nqmgaming/blockads-windows/windows/internal/dnsconfig"
	"github.com/nqmgaming/blockads-windows/windows/internal/protocol"
)

func main() {
	args := os.Args[1:]
	devDirect := false
	filtered := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--dev-direct" {
			devDirect = true
			continue
		}
		filtered = append(filtered, a)
	}
	args = filtered
	if len(args) < 1 {
		usage()
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if devDirect {
		runDevDirect(ctx, args)
		return
	}
	runIPC(ctx, args)
}

func runIPC(ctx context.Context, args []string) {
	cli := client.New()
	switch args[0] {
	case "status":
		st, err := cli.Status(ctx)
		exitJSON(st, err)
	case "enable":
		st, err := cli.Enable(ctx)
		exitJSON(st, err)
	case "disable":
		st, err := cli.Disable(ctx)
		exitJSON(st, err)
	case "recover":
		raw, err := cli.Recover(ctx)
		if err != nil {
			fail(err)
		}
		fmt.Println(string(raw))
	case "test-dns":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: blockads-cli test-dns <domain>")
			os.Exit(2)
		}
		res, err := cli.TestDNS(ctx, args[1])
		exitJSON(res, err)
	case "reload-filters":
		raw, err := cli.ReloadFilters(ctx)
		if err != nil {
			fail(err)
		}
		fmt.Println(string(raw))
	case "stats":
		res, err := cli.GetStats(ctx)
		exitJSON(res, err)
	default:
		usage()
		os.Exit(2)
	}
}

func runDevDirect(ctx context.Context, args []string) {
	fmt.Fprintln(os.Stderr, "WARNING: --dev-direct bypasses the service; privileged ops run in-process")
	paths := defaultPaths()
	ctrl, err := controller.New(paths, dnsconfig.NewPlatformConfigurator())
	if err != nil {
		fail(err)
	}
	defer ctrl.Close()
	switch args[0] {
	case "status":
		exitJSON(ctrl.Status(), nil)
	case "enable":
		if err := ctrl.Enable(ctx); err != nil {
			fail(err)
		}
		exitJSON(ctrl.Status(), nil)
	case "disable":
		if err := ctrl.Disable(ctx); err != nil {
			fail(err)
		}
		exitJSON(ctrl.Status(), nil)
	case "recover":
		res, err := ctrl.Recover(ctx)
		exitJSON(res, err)
	case "test-dns":
		if len(args) < 2 {
			os.Exit(2)
		}
		res, err := ctrl.TestDNS(ctx, args[1])
		exitJSON(res, err)
	default:
		usage()
		os.Exit(2)
	}
}

func exitJSON(v any, err error) {
	if err != nil {
		fail(err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func fail(err error) {
	if err == nil {
		return
	}
	code := protocol.CodeInternal
	if strings.Contains(err.Error(), protocol.CodeServiceUnavailable) {
		code = protocol.CodeServiceUnavailable
	}
	fmt.Fprintf(os.Stderr, "%s: %v\n", code, err)
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

func usage() {
	fmt.Fprintf(os.Stderr, `blockads-cli — IPC client for BlockAdsService

Usage:
  blockads-cli status|enable|disable|recover|stats|reload-filters
  blockads-cli test-dns <domain>
  blockads-cli --dev-direct <cmd>   # explicit in-process (dev/test only)

Production commands use the Named Pipe. No silent privileged fallback.
`)
}
