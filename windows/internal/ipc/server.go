package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"

	"github.com/nqmgaming/blockads-windows/windows/internal/controller"
	"github.com/nqmgaming/blockads-windows/windows/internal/protocol"
)

// Handler dispatches versioned IPC methods to the single Controller.
type Handler struct {
	Ctrl *controller.Controller
}

func (h *Handler) ServeConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	codec := protocol.NewCodec(conn, conn)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		req, err := codec.ReadRequest()
		if err != nil {
			if errors.Is(err, protocol.ErrTooLarge) {
				_ = codec.WriteResponse(protocol.Fail("", protocol.CodeProtocolTooLarge, err.Error()))
			} else if errors.Is(err, protocol.ErrVersion) {
				_ = codec.WriteResponse(protocol.Fail(req.ID, protocol.CodeProtocolVersion, err.Error()))
			}
			return
		}
		resp := h.dispatch(ctx, req)
		if err := codec.WriteResponse(resp); err != nil {
			return
		}
	}
}

func (h *Handler) dispatch(ctx context.Context, req protocol.Request) protocol.Response {
	switch req.Method {
	case protocol.MethodPing:
		return protocol.OK(req.ID, map[string]bool{"pong": true})
	case protocol.MethodStatus:
		return protocol.OK(req.ID, h.Ctrl.Status())
	case protocol.MethodEnable:
		if err := h.Ctrl.Enable(ctx); err != nil {
			return protocol.Fail(req.ID, protocol.CodeEngineError, err.Error())
		}
		return protocol.OK(req.ID, h.Ctrl.Status())
	case protocol.MethodDisable:
		if err := h.Ctrl.Disable(ctx); err != nil {
			return protocol.Fail(req.ID, protocol.CodeDNSError, err.Error())
		}
		return protocol.OK(req.ID, h.Ctrl.Status())
	case protocol.MethodRecover:
		results, err := h.Ctrl.Recover(ctx)
		if err != nil {
			return protocol.Fail(req.ID, protocol.CodeDNSError, err.Error())
		}
		return protocol.OK(req.ID, map[string]any{"results": results})
	case protocol.MethodTestDNS:
		var p protocol.TestDNSParams
		if len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params, &p)
		}
		res, err := h.Ctrl.TestDNS(ctx, p.Domain)
		if err != nil {
			return protocol.Fail(req.ID, protocol.CodeEngineError, err.Error())
		}
		return protocol.OK(req.ID, res)
	case protocol.MethodReloadFilters:
		if err := h.Ctrl.ReloadFilters(ctx); err != nil {
			return protocol.Fail(req.ID, protocol.CodeFilterError, err.Error())
		}
		return protocol.OK(req.ID, map[string]any{"loaded": true, "lists": h.Ctrl.Status().Filters.ListIDs})
	case protocol.MethodGetStats:
		return protocol.OK(req.ID, h.Ctrl.GetStats())
	default:
		return protocol.Fail(req.ID, protocol.CodeMethodUnknown, fmt.Sprintf("unknown method %q", req.Method))
	}
}

// Server accepts connections.
type Server struct {
	Handler *Handler
}

func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		go s.Handler.ServeConn(ctx, conn)
	}
}
