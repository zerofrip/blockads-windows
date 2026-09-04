package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Request is a single JSON line from UI/CLI to the service.
type Request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response is a single JSON line reply.
type Response struct {
	ID     string          `json:"id"`
	OK     bool            `json:"ok"`
	Error  string          `json:"error,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
}

const (
	MethodStatus  = "status"
	MethodEnable  = "enable"
	MethodDisable = "disable"
	MethodRecover = "recover"
	MethodPing    = "ping"
)

// Codec reads/writes newline-delimited JSON messages.
type Codec struct {
	mu sync.Mutex
	rw *bufio.ReadWriter
}

func NewCodec(r io.Reader, w io.Writer) *Codec {
	return &Codec{rw: bufio.NewReadWriter(bufio.NewReader(r), bufio.NewWriter(w))}
}

func (c *Codec) ReadRequest() (Request, error) {
	line, err := c.rw.ReadBytes('\n')
	if err != nil {
		return Request{}, err
	}
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return Request{}, fmt.Errorf("bad request: %w", err)
	}
	return req, nil
}

func (c *Codec) WriteResponse(resp Response) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	if _, err := c.rw.Write(append(b, '\n')); err != nil {
		return err
	}
	return c.rw.Flush()
}

func (c *Codec) WriteRequest(req Request) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if _, err := c.rw.Write(append(b, '\n')); err != nil {
		return err
	}
	return c.rw.Flush()
}

func (c *Codec) ReadResponse() (Response, error) {
	line, err := c.rw.ReadBytes('\n')
	if err != nil {
		return Response{}, err
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return Response{}, fmt.Errorf("bad response: %w", err)
	}
	return resp, nil
}
