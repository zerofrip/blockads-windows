package protocol

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

var (
	ErrTooLarge = errors.New("ipc message too large")
	ErrVersion  = errors.New("unsupported protocol version")
)

// Codec frames newline-delimited JSON with a hard size bound.
type Codec struct {
	mu   sync.Mutex
	r    *bufio.Reader
	w    *bufio.Writer
	scan *bufio.Scanner
}

func NewCodec(r io.Reader, w io.Writer) *Codec {
	br := bufio.NewReaderSize(r, 64*1024)
	sc := bufio.NewScanner(br)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, MaxMessageBytes)
	return &Codec{
		r:    br,
		w:    bufio.NewWriterSize(w, 64*1024),
		scan: sc,
	}
}

func (c *Codec) readLine() ([]byte, error) {
	if !c.scan.Scan() {
		if err := c.scan.Err(); err != nil {
			if errors.Is(err, bufio.ErrTooLong) {
				return nil, ErrTooLarge
			}
			return nil, err
		}
		return nil, io.EOF
	}
	// Scanner.Bytes is valid until next Scan; copy.
	b := append([]byte(nil), c.scan.Bytes()...)
	return b, nil
}

func (c *Codec) writeLine(b []byte) error {
	if len(b)+1 > MaxMessageBytes {
		return ErrTooLarge
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.w.Write(b); err != nil {
		return err
	}
	if err := c.w.WriteByte('\n'); err != nil {
		return err
	}
	return c.w.Flush()
}

func (c *Codec) ReadRequest() (Request, error) {
	line, err := c.readLine()
	if err != nil {
		return Request{}, err
	}
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return Request{}, fmt.Errorf("%w: %v", errors.New(CodeProtocolError), err)
	}
	if req.Version != Version {
		return req, ErrVersion
	}
	if req.Method == "" {
		return req, fmt.Errorf("%s: missing method", CodeProtocolError)
	}
	return req, nil
}

func (c *Codec) WriteResponse(resp Response) error {
	if resp.Version == 0 {
		resp.Version = Version
	}
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return c.writeLine(b)
}

func (c *Codec) WriteRequest(req Request) error {
	if req.Version == 0 {
		req.Version = Version
	}
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return c.writeLine(b)
}

func (c *Codec) ReadResponse() (Response, error) {
	line, err := c.readLine()
	if err != nil {
		return Response{}, err
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return Response{}, fmt.Errorf("%s: %v", CodeProtocolError, err)
	}
	return resp, nil
}

func Fail(id, code, msg string) Response {
	return Response{
		Version: Version,
		ID:      id,
		OK:      false,
		Result:  nil,
		Error:   &ErrorBody{Code: code, Message: msg},
	}
}

func OK(id string, result any) Response {
	var raw json.RawMessage
	if result != nil {
		b, err := json.Marshal(result)
		if err != nil {
			return Fail(id, CodeInternal, err.Error())
		}
		raw = b
	} else {
		raw = json.RawMessage("null")
	}
	return Response{Version: Version, ID: id, OK: true, Result: raw, Error: nil}
}
