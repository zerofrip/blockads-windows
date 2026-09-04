package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/miekg/dns"
	"github.com/nqmgaming/blockads-windows/windows/internal/controller"
	"github.com/nqmgaming/blockads-windows/windows/internal/dnsconfig"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	paths := defaultPaths()
	cfg := dnsconfig.NewPlatformConfigurator()
	c := controller.New(paths, cfg)
	c.SetConfig(controller.Config{
		ListenPort:  53,
		DNSProtocol: envOr("BLOCKADS_DNS_PROTOCOL", "udp"),
		PrimaryDNS:  envOr("BLOCKADS_PRIMARY_DNS", "1.1.1.1"),
		FallbackDNS: envOr("BLOCKADS_FALLBACK_DNS", "1.0.0.1"),
		DoHURL:      os.Getenv("BLOCKADS_DOH_URL"),
		AdTrieCSV:   os.Getenv("BLOCKADS_AD_TRIE"),
		AdBloomCSV:  os.Getenv("BLOCKADS_AD_BLOOM"),
		SecTrieCSV:  os.Getenv("BLOCKADS_SEC_TRIE"),
		SecBloomCSV: os.Getenv("BLOCKADS_SEC_BLOOM"),
	})

	switch cmd {
	case "status":
		st := c.Status()
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(st)
		dnsSt, _ := cfg.Status()
		fmt.Println("dnsConfigurator:", dnsSt)
	case "enable":
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := c.Enable(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "enable failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("enabled")
		_ = json.NewEncoder(os.Stdout).Encode(c.Status())
	case "disable":
		if err := c.Disable(); err != nil {
			fmt.Fprintf(os.Stderr, "disable failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("disabled")
	case "recover":
		results, err := c.RecoverIfNeeded()
		if err != nil {
			fmt.Fprintf(os.Stderr, "recover failed: %v\n", err)
			os.Exit(1)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(results)
	case "test-dns":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: blockads-cli test-dns <domain>")
			os.Exit(2)
		}
		domain := os.Args[2]
		if err := testDNS(domain); err != nil {
			fmt.Fprintf(os.Stderr, "test-dns failed: %v\n", err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func testDNS(domain string) error {
	client := &dns.Client{Net: "udp", Timeout: 5 * time.Second}
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	r, _, err := client.Exchange(m, "127.0.0.1:53")
	if err != nil {
		return err
	}
	fmt.Printf("rcode=%s answers=%d\n", dns.RcodeToString[r.Rcode], len(r.Answer))
	for _, rr := range r.Answer {
		fmt.Println(rr.String())
	}
	return nil
}

func defaultPaths() controller.Paths {
	base := os.Getenv("BLOCKADS_DATA_DIR")
	if base == "" {
		programData := os.Getenv("PROGRAMDATA")
		if programData == "" {
			programData = filepath.Join(os.TempDir(), "BlockAds")
		}
		base = filepath.Join(programData, "BlockAds")
	}
	return controller.Paths{
		DataDir:   base,
		StateFile: filepath.Join(base, "state", "recovery.json"),
		FilterDir: filepath.Join(base, "filters"),
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func usage() {
	fmt.Fprintf(os.Stderr, `blockads-cli — BlockAds Windows DNS MVP

Usage:
  blockads-cli status
  blockads-cli enable
  blockads-cli disable
  blockads-cli recover
  blockads-cli test-dns <domain>

Environment:
  BLOCKADS_DATA_DIR       data root (default %%PROGRAMDATA%%\BlockAds)
  BLOCKADS_DNS_PROTOCOL   udp|doh|... (default udp)
  BLOCKADS_PRIMARY_DNS    upstream (default 1.1.1.1)
  BLOCKADS_DOH_URL        DoH endpoint when protocol=doh
  BLOCKADS_AD_TRIE        CSV of ad trie paths
  BLOCKADS_AD_BLOOM       CSV of ad bloom paths
`)
}
