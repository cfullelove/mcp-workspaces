package mcp

import "encoding/json"

// Request represents a generic MCP request.
type Request struct {
	ID     json.RawMessage `json:"id"`
	Tool   string          `json:"tool"`
	Params json.RawMessage `json:"params"`
}

// Response represents a generic MCP response.
type Response struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// Error represents a structured MCP error.
type Error struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// NewError creates a new MCP error.
func NewError(code, message string, details interface{}) *Error {
	return &Error{Code: code, Message: message, Details: details}
}