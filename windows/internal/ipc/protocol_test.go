package ipc

import (
	"encoding/json"
	"io"
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
		if req.Method != MethodPing {
			t.Errorf("method=%s", req.Method)
		}
		_ = srv.WriteResponse(Response{ID: req.ID, OK: true, Result: json.RawMessage(`{"pong":true}`)})
	}()

	if err := cli.WriteRequest(Request{ID: "1", Method: MethodPing}); err != nil {
		t.Fatal(err)
	}
	resp, err := cli.ReadResponse()
	if err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.ID != "1" {
		t.Fatalf("%+v", resp)
	}
	<-done
	_ = pw1.Close()
	_ = pw2.Close()
}
