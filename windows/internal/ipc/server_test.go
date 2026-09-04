package ipc_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nqmgaming/blockads-windows/windows/internal/client"
	"github.com/nqmgaming/blockads-windows/windows/internal/controller"
	"github.com/nqmgaming/blockads-windows/windows/internal/dnsconfig"
	"github.com/nqmgaming/blockads-windows/windows/internal/ipc"
	"github.com/nqmgaming/blockads-windows/windows/internal/protocol"
)

func TestIPCStatusOverUnixPipe(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "test.sock")
	cfgPath := filepath.Join(dir, "config.json")
	_ = os.WriteFile(cfgPath, []byte(fmt.Sprintf(`{
	  "version":1,"enabled":false,
	  "dns":{"listenPort":%d,"protocol":"udp","primary":"1.1.1.1","fallback":"1.0.0.1"},
	  "filters":{"catalogUrl":"http://127.0.0.1:1/x","enabledListIds":[],"autoUpdate":false}
	}`, 1855)), 0o600)

	paths := controller.Paths{
		DataDir: dir, StateFile: filepath.Join(dir, "recovery.json"),
		ConfigFile: cfgPath, FilterDir: filepath.Join(dir, "filters"),
	}

	key := dnsconfig.AdapterKey{GUID: "{DDDDDDDD-BBBB-CCCC-DDDD-EEEEEEEEEEEE}"}
	orig := dnsconfig.AdapterDNSSnapshot{Key: key, IPv4Servers: dnsconfig.DNSServerList{"1.1.1.1"}}
	dnsconfig.FillChecksum(&orig)
	mem := dnsconfig.NewMemoryConfigurator([]dnsconfig.NetworkAdapter{{
		Key: key, FriendlyName: "Eth", Description: "NIC",
		IfType: dnsconfig.IfTypeEthernetCSMACD, OperStatus: dnsconfig.OperStatusUp,
		IPv4Addrs: []string{"10.0.0.1"},
	}}, map[string]dnsconfig.AdapterDNSSnapshot{key.GUID: orig})

	ctrl, err := controller.New(paths, mem)
	if err != nil {
		t.Fatal(err)
	}
	defer ctrl.Close()

	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := &ipc.Server{Handler: &ipc.Handler{Ctrl: ctrl}}
	go func() { _ = srv.Serve(ctx, ln) }()

	cli := &client.Client{Dial: func() (net.Conn, error) {
		return net.DialTimeout("unix", sock, 2*time.Second)
	}}
	st, err := cli.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Service.Version == "" {
		t.Fatalf("empty status: %+v", st)
	}

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	codec := protocol.NewCodec(conn, conn)
	_ = codec.WriteRequest(protocol.Request{Version: 1, ID: "x", Method: "explode"})
	resp, err := codec.ReadResponse()
	if err != nil {
		t.Fatal(err)
	}
	if resp.OK || resp.Error == nil || resp.Error.Code != protocol.CodeMethodUnknown {
		t.Fatalf("%+v", resp)
	}
	_ = conn.Close()
}
