package protocol

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestCodecRoundTrip(t *testing.T) {
	pr1, pw1 := io.Pipe()
	pr2, pw2 := io.Pipe()
	srv := NewCodec(pr1, pw2)
	cli := NewCodec(pr2, pw1)

	done := make(chan struct{})
	go func() {
		defer close(done)
		req, err := srv.ReadRequest()
		if err != nil {
			t.Errorf("read: %v", err)
			return
		}
		if req.Method != MethodPing || req.Version != Version {
			t.Errorf("bad req %+v", req)
		}
		_ = srv.WriteResponse(OK(req.ID, map[string]bool{"pong": true}))
	}()

	if err := cli.WriteRequest(Request{Version: Version, ID: "1", Method: MethodPing}); err != nil {
		t.Fatal(err)
	}
	resp, err := cli.ReadResponse()
	if err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.ID != "1" || resp.Version != Version {
		t.Fatalf("%+v", resp)
	}
	<-done
	_ = pw1.Close()
	_ = pw2.Close()
}

func TestCodecRejectsOversize(t *testing.T) {
	pr, pw := io.Pipe()
	c := NewCodec(pr, pw)
	go func() {
		huge := strings.Repeat("a", MaxMessageBytes+10)
		_, _ = pw.Write([]byte(huge + "\n"))
		_ = pw.Close()
	}()
	_, err := c.ReadRequest()
	if err == nil {
		t.Fatal("expected too large")
	}
}

func TestCodecRejectsBadVersion(t *testing.T) {
	pr, pw := io.Pipe()
	c := NewCodec(pr, pw)
	go func() {
		b, _ := json.Marshal(Request{Version: 99, ID: "x", Method: MethodPing})
		_, _ = pw.Write(append(b, '\n'))
		_ = pw.Close()
	}()
	_, err := c.ReadRequest()
	if err == nil {
		t.Fatal("expected version error")
	}
}
