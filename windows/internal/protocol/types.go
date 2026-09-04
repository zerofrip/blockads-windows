package protocol

import "encoding/json"

const Version = 1

// MaxMessageBytes caps a single framed IPC line (including newline).
const MaxMessageBytes = 256 * 1024

// PipeName is the production Windows Named Pipe path.
const PipeName = `\\.\pipe\BlockAdsService`

// Methods
const (
	MethodPing          = "ping"
	MethodStatus        = "status"
	MethodEnable        = "enable"
	MethodDisable       = "disable"
	MethodRecover       = "recover"
	MethodTestDNS       = "test_dns"
	MethodReloadFilters = "reload_filters"
	MethodGetStats      = "get_stats"
)

// Error codes
const (
	CodeProtocolError      = "PROTOCOL_ERROR"
	CodeProtocolVersion    = "PROTOCOL_VERSION"
	CodeProtocolTooLarge   = "PROTOCOL_TOO_LARGE"
	CodeMethodUnknown      = "METHOD_UNKNOWN"
	CodeServiceUnavailable = "SERVICE_UNAVAILABLE"
	CodeEngineError        = "ENGINE_ERROR"
	CodeDNSError           = "DNS_ERROR"
	CodeFilterError        = "FILTER_ERROR"
	CodeConflict           = "CONFLICT"
	CodeInternal           = "INTERNAL"
)

// Request is the versioned IPC request envelope.
type Request struct {
	Version int             `json:"version"`
	ID      string          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// ErrorBody is a machine-readable error.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Response is the versioned IPC response envelope.
type Response struct {
	Version int             `json:"version"`
	ID      string          `json:"id"`
	OK      bool            `json:"ok"`
	Result  json.RawMessage `json:"result"`
	Error   *ErrorBody      `json:"error"`
}

// TestDNSParams is params for test_dns.
type TestDNSParams struct {
	Domain string `json:"domain"`
}

// TestDNSResult is result for test_dns.
type TestDNSResult struct {
	Domain  string   `json:"domain"`
	RCode   string   `json:"rcode"`
	Answers []string `json:"answers"`
	Blocked bool     `json:"blocked"`
}

// StatsResult is result for get_stats.
type StatsResult struct {
	TotalQueries   int64 `json:"totalQueries"`
	BlockedQueries int64 `json:"blockedQueries"`
}
