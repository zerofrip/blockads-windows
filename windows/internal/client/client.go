package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	"github.com/nqmgaming/blockads-windows/windows/internal/ipc"
	"github.com/nqmgaming/blockads-windows/windows/internal/protocol"
	"github.com/nqmgaming/blockads-windows/windows/internal/statusdto"
)

// Client talks to BlockAdsService over the Named Pipe / Unix socket.
type Client struct {
	Dial func() (net.Conn, error)
}

func New() *Client {
	return &Client{Dial: func() (net.Conn, error) {
		return ipc.DialPipe("")
	}}
}

func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	conn, err := c.Dial()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", protocol.CodeServiceUnavailable, err)
	}
	defer conn.Close()

	codec := protocol.NewCodec(conn, conn)
	var raw json.RawMessage
	if params != nil {
		raw, err = json.Marshal(params)
		if err != nil {
			return nil, err
		}
	}
	id := fmt.Sprintf("%d", ctx.Value("id"))
	if id == "%!d(<nil>)" || id == "0" {
		id = "1"
	}
	req := protocol.Request{Version: protocol.Version, ID: id, Method: method, Params: raw}
	if err := codec.WriteRequest(req); err != nil {
		return nil, err
	}
	resp, err := codec.ReadResponse()
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("%s: request failed", protocol.CodeInternal)
	}
	return resp.Result, nil
}

func (c *Client) Status(ctx context.Context) (statusdto.Status, error) {
	raw, err := c.call(ctx, protocol.MethodStatus, nil)
	if err != nil {
		return statusdto.Status{}, err
	}
	var st statusdto.Status
	return st, json.Unmarshal(raw, &st)
}

func (c *Client) Enable(ctx context.Context) (statusdto.Status, error) {
	raw, err := c.call(ctx, protocol.MethodEnable, nil)
	if err != nil {
		return statusdto.Status{}, err
	}
	var st statusdto.Status
	return st, json.Unmarshal(raw, &st)
}

func (c *Client) Disable(ctx context.Context) (statusdto.Status, error) {
	raw, err := c.call(ctx, protocol.MethodDisable, nil)
	if err != nil {
		return statusdto.Status{}, err
	}
	var st statusdto.Status
	return st, json.Unmarshal(raw, &st)
}

func (c *Client) Recover(ctx context.Context) (json.RawMessage, error) {
	return c.call(ctx, protocol.MethodRecover, nil)
}

func (c *Client) TestDNS(ctx context.Context, domain string) (protocol.TestDNSResult, error) {
	raw, err := c.call(ctx, protocol.MethodTestDNS, protocol.TestDNSParams{Domain: domain})
	if err != nil {
		return protocol.TestDNSResult{}, err
	}
	var r protocol.TestDNSResult
	return r, json.Unmarshal(raw, &r)
}

func (c *Client) ReloadFilters(ctx context.Context) (json.RawMessage, error) {
	return c.call(ctx, protocol.MethodReloadFilters, nil)
}

func (c *Client) GetStats(ctx context.Context) (protocol.StatsResult, error) {
	raw, err := c.call(ctx, protocol.MethodGetStats, nil)
	if err != nil {
		return protocol.StatsResult{}, err
	}
	var r protocol.StatsResult
	return r, json.Unmarshal(raw, &r)
}
