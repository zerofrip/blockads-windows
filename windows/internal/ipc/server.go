package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"

	"github.com/nqmgaming/blockads-windows/windows/internal/controller"
)

// Handler dispatches IPC methods to the Controller.
type Handler struct {
	Ctrl *controller.Controller
}

func (h *Handler) ServeConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	codec := NewCodec(conn, conn)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		req, err := codec.ReadRequest()
		if err != nil {
			return
		}
		resp := h.dispatch(ctx, req)
		if err := codec.WriteResponse(resp); err != nil {
			return
		}
	}
}

func (h *Handler) dispatch(ctx context.Context, req Request) Response {
	resp := Response{ID: req.ID}
	switch req.Method {
	case MethodPing:
		resp.OK = true
		resp.Result = json.RawMessage(`{"pong":true}`)
	case MethodStatus:
		b, _ := json.Marshal(h.Ctrl.Status())
		resp.OK = true
		resp.Result = b
	case MethodEnable:
		if err := h.Ctrl.Enable(ctx); err != nil {
			resp.Error = err.Error()
			return resp
		}
		resp.OK = true
		b, _ := json.Marshal(h.Ctrl.Status())
		resp.Result = b
	case MethodDisable:
		if err := h.Ctrl.Disable(); err != nil {
			resp.Error = err.Error()
			return resp
		}
		resp.OK = true
	case MethodRecover:
		results, err := h.Ctrl.RecoverIfNeeded()
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
		b, _ := json.Marshal(results)
		resp.OK = true
		resp.Result = b
	default:
		resp.Error = fmt.Sprintf("unknown method %q", req.Method)
	}
	return resp
}

// Server accepts connections and serves Handler.
type Server struct {
	Handler *Handler
	ln      net.Listener
	mu      sync.Mutex
}

func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	s.mu.Lock()
	s.ln = ln
	s.mu.Unlock()
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
